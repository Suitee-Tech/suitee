package main

// Testes de regressão do achado C7 (docs/reviews/2026-09-19-permissoes-acesso-review.md):
// a superfície MCP não tinha autenticação nenhuma, o tenant vinha do header
// X-Tenant-Id e o papel vinha de um payload de JWT decodificado sem verificação
// de assinatura.
//
// Os testes abaixo exercitam a fiação REAL: newRouter + newMCPSSEServer +
// pkg/auth.Middleware contra um JWKS servido localmente.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
)

const kidDeTeste = "test-key-1"

type assinadorDeTeste struct {
	key *rsa.PrivateKey
	kid string
}

// iniciarJWKSDeTeste publica a chave pública num JWKS local e aponta
// pkg/auth para ele — é o mesmo caminho de produção (InitJWKS + Middleware),
// só trocando o provedor Supabase pelo servidor de teste.
func iniciarJWKSDeTeste(t *testing.T) *assinadorDeTeste {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gerar chave RSA: %v", err)
	}

	jwks, err := json.Marshal(map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kidDeTeste,
			"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}},
	})
	if err != nil {
		t.Fatalf("montar JWKS: %v", err)
	}

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	t.Cleanup(jwksSrv.Close)

	if err := pkgAuth.InitJWKS(jwksSrv.URL); err != nil {
		t.Fatalf("InitJWKS: %v", err)
	}

	return &assinadorDeTeste{key: key, kid: kidDeTeste}
}

// token assina claims no formato do Supabase/toca-auth.
func (a *assinadorDeTeste) token(t *testing.T, ajustar func(*pkgAuth.Claims)) string {
	t.Helper()

	claims := pkgAuth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "usuario-1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: "usuario@toca.test",
		Role:  "authenticated",
		AppMetadata: &pkgAuth.AppMetadata{
			TenantID:       "tenant-a",
			FunctionalRole: "corretor",
			Permission:     "member",
		},
	}
	if ajustar != nil {
		ajustar(&claims)
	}
	return a.assinarClaims(t, &claims)
}

// tokenCru assina um payload JSON arbitrário — usado para reproduzir o ataque
// do C7 (app_metadata.user_role forjado) com assinatura legítima.
func (a *assinadorDeTeste) tokenCru(t *testing.T, payload map[string]any) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(payload))
	tok.Header["kid"] = a.kid
	signed, err := tok.SignedString(a.key)
	if err != nil {
		t.Fatalf("assinar token cru: %v", err)
	}
	return signed
}

func (a *assinadorDeTeste) assinarClaims(t *testing.T, claims jwt.Claims) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = a.kid
	signed, err := tok.SignedString(a.key)
	if err != nil {
		t.Fatalf("assinar token: %v", err)
	}
	return signed
}

// tokenForjadoSemChave é o ataque do C7: payload dizendo o que o atacante
// quiser, assinatura que não corresponde a chave nenhuma.
func tokenForjadoSemChave(t *testing.T) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"role":         "authenticated",
		"app_metadata": map[string]any{"user_role": "admin", "tenant_id": "victim"},
	})
	if err != nil {
		t.Fatalf("montar payload forjado: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".assinatura-invalida"
}

// adulterarAssinatura mantém o token sintaticamente válido e troca um byte da
// assinatura: exatamente o "token adulterado" que a versão antiga aceitava.
func adulterarAssinatura(t *testing.T, token string) string {
	t.Helper()

	partes := strings.Split(token, ".")
	if len(partes) != 3 {
		t.Fatalf("token inesperado: %q", token)
	}
	sig, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil {
		t.Fatalf("decodificar assinatura: %v", err)
	}
	sig[len(sig)-1] ^= 0x01
	partes[2] = base64.RawURLEncoding.EncodeToString(sig)
	return strings.Join(partes, ".")
}

// novoServidorDeTeste sobe o router de produção. O Close vai para t.Cleanup
// (LIFO) para que o cliente SSE registrado depois seja encerrado ANTES do
// servidor — httptest.Server.Close espera os requests em voo, e o stream SSE
// só termina quando o contexto do cliente é cancelado.
func novoServidorDeTeste(t *testing.T, s *server.MCPServer) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(newRouter(newMCPSSEServer(s)))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// Probes de infraestrutura continuam públicos.
// ---------------------------------------------------------------------------

func TestProbesDeInfraContinuamPublicos(t *testing.T) {
	srv := novoServidorDeTeste(t, server.NewMCPServer("toca-mcp-test", "1.0.0"))

	for _, path := range []string{"/health", "/live", "/ready", "/metrics"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, esperado 200 (probe de infraestrutura não pode exigir token)", path, resp.StatusCode)
		}
	}
}

// O preflight CORS não pode ser barrado pelo middleware de auth (o navegador
// não envia Authorization no OPTIONS).
func TestPreflightContinuaLiberado(t *testing.T) {
	srv := novoServidorDeTeste(t, server.NewMCPServer("toca-mcp-test", "1.0.0"))

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/mcp/sse", nil)
	if err != nil {
		t.Fatalf("montar request: %v", err)
	}
	req.Header.Set("Origin", "https://app.toca.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization,x-tenant-id")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /mcp/sse: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight = %d, esperado 204", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// O teste de regressão do C7: token forjado/ausente é 401.
// ---------------------------------------------------------------------------

func TestMCPSSERejeitaTokenNaoVerificado(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)

	// Chave estranha: assinatura bem formada, chave que o JWKS não conhece.
	chaveEstranha, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gerar chave estranha: %v", err)
	}
	tokenChaveEstranha, err := jwt.NewWithClaims(jwt.SigningMethodRS256, pkgAuth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "usuario-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role:        "authenticated",
		AppMetadata: &pkgAuth.AppMetadata{TenantID: "tenant-a", FunctionalRole: "admin"},
	}).SignedString(chaveEstranha)
	if err != nil {
		t.Fatalf("assinar com chave estranha: %v", err)
	}

	expirado := assinador.token(t, func(c *pkgAuth.Claims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	})
	semTenant := assinador.token(t, func(c *pkgAuth.Claims) {
		c.AppMetadata = &pkgAuth.AppMetadata{FunctionalRole: "admin", Permission: "owner"}
		c.TenantID = ""
	})
	valido := assinador.token(t, nil)

	casos := []struct {
		nome   string
		header string
	}{
		{"sem token", ""},
		{"token que não é JWT", "Bearer nao-e-um-jwt"},
		{"payload forjado com papel admin e assinatura lixo", "Bearer " + tokenForjadoSemChave(t)},
		{"assinatura adulterada de token legítimo", "Bearer " + adulterarAssinatura(t, valido)},
		{"token assinado por chave desconhecida", "Bearer " + tokenChaveEstranha},
		{"token expirado", "Bearer " + expirado},
		{"token verificado sem tenant", "Bearer " + semTenant},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			srv := novoServidorDeTeste(t, server.NewMCPServer("toca-mcp-test", "1.0.0"))

			req, err := http.NewRequest(http.MethodGet, srv.URL+"/mcp/sse", nil)
			if err != nil {
				t.Fatalf("montar request: %v", err)
			}
			req.Header.Set("X-Tenant-Id", "victim")
			if caso.header != "" {
				req.Header.Set("Authorization", caso.header)
			}

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("GET /mcp/sse: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("GET /mcp/sse = %d, esperado 401 (C7: token não verificado não pode abrir a superfície MCP)", resp.StatusCode)
			}
		})
	}
}

func TestMCPMessagesRejeitaTokenNaoVerificado(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)

	casos := []struct {
		nome  string
		token string
	}{
		{"sem token", ""},
		{"payload forjado", tokenForjadoSemChave(t)},
		{"assinatura adulterada", adulterarAssinatura(t, assinador.token(t, nil))},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			srv := novoServidorDeTeste(t, server.NewMCPServer("toca-mcp-test", "1.0.0"))

			req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp/messages?sessionId=qualquer", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_company_settings"}}`))
			if err != nil {
				t.Fatalf("montar request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-Id", "victim")
			if caso.token != "" {
				req.Header.Set("Authorization", "Bearer "+caso.token)
			}

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("POST /mcp/messages: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("POST /mcp/messages = %d, esperado 401 do middleware (e não o 400 do SDK)", resp.StatusCode)
			}
		})
	}
}

func TestMCPSSEAceitaTokenVerificado(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)
	srv := novoServidorDeTeste(t, server.NewMCPServer("toca-mcp-test", "1.0.0"))

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/mcp/sse", nil)
	if err != nil {
		t.Fatalf("montar request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+assinador.token(t, nil))

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /mcp/sse: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mcp/sse com token verificado = %d, esperado 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, esperado text/event-stream", ct)
	}
	cancel()
}

// ---------------------------------------------------------------------------
// End-to-end: o tenant e o papel vêm das claims verificadas, não do header.
// ---------------------------------------------------------------------------

const nomeToolLeitura = "read_probe"
const nomeToolEscrita = "write_probe"

// novoServidorMCPComProbes registra duas ferramentas com os mesmos metadados
// dos tools reais (x-min-role "user" e "broker") e o mesmo gate requireMinRole
// usado pelos 22 tools de escrita.
func novoServidorMCPComProbes() *server.MCPServer {
	s := server.NewMCPServer(
		"toca-mcp-test",
		"1.0.0",
		server.WithToolCapabilities(false),
		// Mesmo hook de produção: a listagem é filtrada pelo rank verificado.
		server.WithHooks(mcpServerHooks()),
	)

	s.AddTool(
		toolWithMeta(mcp.NewTool(nomeToolLeitura, mcp.WithDescription("probe de leitura")), "teste", "user", true, false),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(identidadeDaSessao(ctx)), nil
		},
	)

	s.AddTool(
		toolWithMeta(mcp.NewTool(nomeToolEscrita, mcp.WithDescription("probe de escrita")), "teste", "broker", false, true),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := requireMinRole(ctx, "broker"); err != nil {
				return mcp.NewToolResultError("insufficient role"), nil
			}
			return mcp.NewToolResultText(identidadeDaSessao(ctx)), nil
		},
	)

	return s
}

func identidadeDaSessao(ctx context.Context) string {
	tenant, _ := ctx.Value(tenantIDKey).(string)
	papel, _ := ctx.Value(userRoleKey).(string)
	authz, _ := ctx.Value(authKey).(string)
	return fmt.Sprintf("tenant=%s papel=%s rank=%d authz=%s", tenant, papel, sessionRank(ctx), authz)
}

func conectarClienteMCP(t *testing.T, srv *httptest.Server, token string) (*client.Client, context.Context) {
	t.Helper()

	c, err := client.NewSSEMCPClient(srv.URL+"/mcp/sse", client.WithHeaders(map[string]string{
		"Authorization": "Bearer " + token,
		// Header hostil: era a fonte do tenant na versão antiga.
		"X-Tenant-Id": "victim",
	}))
	if err != nil {
		t.Fatalf("criar cliente SSE: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	// Cancelar antes de fechar: é o cancelamento que encerra o stream SSE no
	// servidor (e portanto o Close do httptest).
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
	})

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo:   mcp.Implementation{Name: "toca-mcp-test", Version: "1.0.0"},
			Capabilities: mcp.ClientCapabilities{},
		},
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c, ctx
}

func textoDoResultado(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	var b strings.Builder
	for _, c := range res.Content {
		if txt, ok := c.(mcp.TextContent); ok {
			b.WriteString(txt.Text)
		}
	}
	return b.String()
}

func nomesDasTools(res *mcp.ListToolsResult) map[string]bool {
	nomes := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		nomes[tl.Name] = true
	}
	return nomes
}

func TestTenantEPapelVemDasClaimsVerificadas(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)
	srv := novoServidorDeTeste(t, novoServidorMCPComProbes())

	// Token legítimo do tenant-a com papel funcional corretor; o request
	// declara X-Tenant-Id: victim.
	token := assinador.token(t, nil)
	c, ctx := conectarClienteMCP(t, srv, token)

	res, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: nomeToolEscrita}})
	if err != nil {
		t.Fatalf("CallTool %s: %v", nomeToolEscrita, err)
	}
	texto := textoDoResultado(t, res)

	if res.IsError {
		t.Fatalf("%s negou um corretor (falha funcional do C7): %s", nomeToolEscrita, texto)
	}
	if !strings.Contains(texto, "tenant=tenant-a") {
		t.Fatalf("tenant resolvido = %q, esperado tenant-a (das claims), não o header X-Tenant-Id", texto)
	}
	if strings.Contains(texto, "victim") {
		t.Fatalf("tenant veio do header X-Tenant-Id: %s", texto)
	}
	if !strings.Contains(texto, "papel=corretor") || !strings.Contains(texto, "rank=2") {
		t.Fatalf("papel/rank não vieram de functional_role: %s", texto)
	}
	// O gateway recebe o MESMO header Authorization que foi verificado
	// (com o prefixo Bearer que makeGatewayRequest repassa).
	if !strings.Contains(texto, "authz=Bearer "+token) {
		t.Fatalf("token repassado ao gateway não é o header verificado: %s", texto)
	}

	// A listagem (hook OnAfterListTools) também passa a usar o rank
	// verificado: corretor enxerga os dois probes.
	listados, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	nomes := nomesDasTools(listados)
	if !nomes[nomeToolEscrita] || !nomes[nomeToolLeitura] {
		t.Fatalf("corretor deveria ver os dois probes, viu %v", nomes)
	}
}

func TestPapelSemEscritaNaoAlcancaToolsDeEscrita(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)
	srv := novoServidorDeTeste(t, novoServidorMCPComProbes())

	// Token legítimo, papel funcional cliente: rank 1 (somente leitura).
	c, ctx := conectarClienteMCP(t, srv, assinador.token(t, func(cl *pkgAuth.Claims) {
		cl.AppMetadata = &pkgAuth.AppMetadata{TenantID: "tenant-a", FunctionalRole: "cliente"}
	}))

	resLeitura, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: nomeToolLeitura}})
	if err != nil {
		t.Fatalf("CallTool %s: %v", nomeToolLeitura, err)
	}
	if resLeitura.IsError {
		t.Fatalf("ferramenta de leitura negada para cliente: %s", textoDoResultado(t, resLeitura))
	}

	resEscrita, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: nomeToolEscrita}})
	if err != nil {
		t.Fatalf("CallTool %s: %v", nomeToolEscrita, err)
	}
	if !resEscrita.IsError {
		t.Fatalf("ferramenta de escrita liberada para papel sem escrita: %s", textoDoResultado(t, resEscrita))
	}

	// tools/list não pode nem anunciar o probe de escrita (hook
	// OnAfterListTools filtrando pelo rank verificado).
	listados, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	nomes := nomesDasTools(listados)
	if nomes[nomeToolEscrita] {
		t.Fatalf("cliente não deveria ver %s na listagem: %v", nomeToolEscrita, nomes)
	}
	if !nomes[nomeToolLeitura] {
		t.Fatalf("cliente deveria ver %s na listagem: %v", nomeToolLeitura, nomes)
	}
}

// app_metadata.user_role era a claim que o decodificador antigo lia. Ela não
// existe no toca-auth e não pode conceder rank: um token legítimo que ainda a
// carregue vale pelo functional_role/permission verificados.
func TestAppMetadataUserRoleNaoConcedeRank(t *testing.T) {
	assinador := iniciarJWKSDeTeste(t)
	srv := novoServidorDeTeste(t, novoServidorMCPComProbes())

	token := assinador.tokenCru(t, map[string]any{
		"sub":   "usuario-1",
		"email": "usuario@toca.test",
		"role":  "authenticated",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"app_metadata": map[string]any{
			"tenant_id": "tenant-a",
			"user_role": "admin", // campo do ataque antigo
		},
	})

	c, ctx := conectarClienteMCP(t, srv, token)

	res, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: nomeToolEscrita}})
	if err != nil {
		t.Fatalf("CallTool %s: %v", nomeToolEscrita, err)
	}
	if !res.IsError {
		t.Fatalf("app_metadata.user_role concedeu rank de escrita: %s", textoDoResultado(t, res))
	}
}

func TestRankDosPapeisFuncionais(t *testing.T) {
	casos := []struct {
		nome           string
		userRole       string
		permission     string
		functionalRole string
		esperado       int
	}{
		{"admin funcional alcança escrita", "authenticated", "member", "admin", 4},
		{"gestor alcança escrita", "authenticated", "member", "gestor", 3},
		{"operacoes alcança escrita", "authenticated", "member", "operacoes", 2},
		{"corretor alcança escrita", "authenticated", "member", "corretor", 2},
		{"perfil owner é root", "authenticated", "owner", "", 4},
		{"perfil admin é root", "authenticated", "admin", "", 4},
		{"cliente fica em leitura", "authenticated", "", "cliente", 1},
		{"auditor fica em leitura", "authenticated", "member", "auditor", 1},
		{"financeiro fica em leitura", "authenticated", "member", "financeiro", 1},
		{"token só com role authenticated", "authenticated", "", "", 1},
		{"service_role", "service_role", "", "", 4},
		{"anon não tem rank", "anon", "", "", 0},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			got := rankFromVerifiedClaims(caso.userRole, caso.permission, caso.functionalRole)
			if got != caso.esperado {
				t.Fatalf("rankFromVerifiedClaims(%q, %q, %q) = %d, esperado %d",
					caso.userRole, caso.permission, caso.functionalRole, got, caso.esperado)
			}
		})
	}
}

func TestRequireMinRoleUsaRankVerificado(t *testing.T) {
	ctxComRank := func(functionalRole string) context.Context {
		rank := rankFromVerifiedClaims("authenticated", "member", functionalRole)
		return context.WithValue(context.Background(), userRankKey, rank)
	}

	for _, papel := range []string{"admin", "gestor", "operacoes", "corretor"} {
		if err := requireMinRole(ctxComRank(papel), "broker"); err != nil {
			t.Errorf("papel %q deveria alcançar os tools de escrita (x-min-role broker): %v", papel, err)
		}
	}

	for _, papel := range []string{"cliente", "auditor", "parceiro", "financeiro", "marketing"} {
		if err := requireMinRole(ctxComRank(papel), "broker"); err == nil {
			t.Errorf("papel %q não deveria alcançar os tools de escrita", papel)
		}
		if err := requireMinRole(ctxComRank(papel), "user"); err != nil {
			t.Errorf("papel %q deveria alcançar os tools de leitura: %v", papel, err)
		}
	}

	// Contexto sem rank verificado (nenhuma claim verificada): nega tudo.
	if err := requireMinRole(context.Background(), "user"); err == nil {
		t.Error("contexto sem rank verificado não pode liberar nem os tools de leitura")
	}
}

// verifiedSessionContext não pode ler tenant de header nenhum.
func TestVerifiedSessionContextIgnoraHeaderDeTenant(t *testing.T) {
	ctx := verifiedSessionContext(context.Background(), httptest.NewRequest(http.MethodGet, "/mcp/sse", nil))
	if tenant, _ := ctx.Value(tenantIDKey).(string); tenant != "" {
		t.Fatalf("sem claims verificadas o tenant deveria ser vazio, veio %q", tenant)
	}
	if rank := sessionRank(ctx); rank != 0 {
		t.Fatalf("sem claims verificadas o rank deveria ser 0, veio %d", rank)
	}
}

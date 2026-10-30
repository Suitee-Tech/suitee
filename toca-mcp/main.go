package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/tracing"
	"toca-mcp/internal/helpcorpus"
)

type contextKey string

const (
	tenantIDKey contextKey = "tenant_id"
	authKey     contextKey = "authorization"
	userRoleKey contextKey = "user_role"
	userPermKey contextKey = "user_permission"
	// userRankKey guarda o rank derivado das claims VERIFICADAS. É gravado
	// exclusivamente por verifiedSessionContext — nada mais no serviço pode
	// escrever nele, e é ele que requireMinRole consulta (achado C7).
	userRankKey contextKey = "user_rank"
)

// gatewayHTTPClient is the HTTP client used by every MCP tool to reach the
// gateway. It has an explicit timeout so a slow/unresponsive backend cannot
// hang a tool call forever — which previously blocked the parent SSE stream
// in toca-ai (`wg.Wait()` would never return when N parallel calls fan out).
var gatewayHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

func makeGatewayRequest(ctx context.Context, gatewayURL, method, endpoint string, body []byte) (*mcp.CallToolResult, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, gatewayURL+endpoint, reqBody)
	if err != nil {
		logger.Error("Failed to create request", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to create request: %v", err)), nil
	}

	// Propaga credenciais da sessão MCP para o Backend Gateway
	if tenantID, ok := ctx.Value(tenantIDKey).(string); ok && tenantID != "" {
		req.Header.Set("X-Tenant-Id", tenantID)
	}
	if auth, ok := ctx.Value(authKey).(string); ok && auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := gatewayHTTPClient.Do(req)
	if err != nil {
		logger.Error("Failed to execute request", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to execute request: %v", err)), nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		logger.Warn("API returned error", "status", resp.StatusCode, "body", string(respBody))
		return mcp.NewToolResultError(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, string(respBody))), nil
	}

	// Sanitiza ANTES de formatar: o que o modelo recebe define quanto ele demora
	// para responder.
	respBody = sanitizarRespostaParaModelo(respBody)

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, respBody, "", "  "); err == nil {
		return mcp.NewToolResultText(prettyJSON.String()), nil
	}

	return mcp.NewToolResultText(string(respBody)), nil
}

// camposDeDocumento são os campos que carregam o ARQUIVO em si (base64), e não
// dados sobre ele. Medido: `get_contracts` devolvia 40.053 caracteres quase
// inteiramente de base64 do DOCX de cada contrato — o documento completo dentro
// da resposta da lista.
//
// O modelo não lê DOCX: para raciocinar sobre contratos ele precisa de id,
// status, datas, valores e partes. Carregar o arquivo só infla o prompt e faz a
// inferência demorar — era essa a causa das respostas de 10 a 36 segundos, que
// estouravam o watchdog do cliente e faziam a resposta nunca ser persistida.
//
// A troca preserva o campo como marcador legível, para o modelo saber que o
// documento existe e cujo tamanho, sem carregar o conteúdo.
var camposDeDocumento = map[string]bool{
	"content":      true, // contrato/documento renderizado
	"file_content": true,
	"base64":       true,
	"signed_file":  true,
	"pdf_base64":   true,
}

// limiarDeDocumento evita mexer em campos pequenos que por acaso tenham esses
// nomes: só trata como arquivo o que é grande o bastante para ser um.
const limiarDeDocumento = 2000

// limiarDeURLAssinada: URLs pré-assinadas de storage (S3 e afins) carregam
// credenciais TEMPORÁRIAS na query string. Medido num `signed_document_url`:
// 1.870 caracteres, dos quais 1.316 são o `X-Amz-Security-Token` — credencial
// da AWS, ao lado de `X-Amz-Credential` e `X-Amz-Signature`.
//
// Enviar isso ao modelo significa (a) gastar contexto com credenciais que não
// respondem nada, e (b) gravar segredo no histórico da conversa, nos logs e no
// prompt do provedor. O modelo não precisa do token para nada: para raciocinar
// basta saber QUE o documento existe. A query é removida e o caminho fica.
const limiarDeURLAssinada = 300

// parametrosDeAssinatura identificam URL pré-assinada. Só removemos a query
// quando ela tem cara de credencial — URL comum com filtros continua inteira.
var parametrosDeAssinatura = []string{
	"X-Amz-Signature", "X-Amz-Credential", "X-Amz-Security-Token",
	"X-Goog-Signature", "X-Goog-Credential", "sig=", "se=", "sp=", "sv=",
	"token=", "access_token=",
}

func ehURLAssinada(s string) bool {
	if len(s) < limiarDeURLAssinada || !strings.Contains(s, "://") {
		return false
	}
	for _, p := range parametrosDeAssinatura {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func podarURLCredenciada(s string) (string, bool) {
	if !ehURLAssinada(s) {
		return s, false
	}
	if i := strings.IndexByte(s, '?'); i > 0 {
		return s[:i] + "?[credenciais da URL removidas do contexto]", true
	}
	return s, false
}

func sanitizarRespostaParaModelo(raw []byte) []byte {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw // não é JSON: devolve como veio
	}
	limpo := podarCamposDeDocumento(doc)
	out, err := json.Marshal(limpo)
	if err != nil {
		return raw
	}
	if len(out) > 0 && len(out) < len(raw) {
		logger.Info("resposta sanitizada para o modelo",
			"antes_bytes", len(raw), "depois_bytes", len(out))
		return out
	}
	return raw
}

func podarCamposDeDocumento(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok {
				if camposDeDocumento[k] && len(s) >= limiarDeDocumento {
					t[k] = fmt.Sprintf("[documento omitido do contexto: %d caracteres]", len(s))
					continue
				}
				// URL com credencial é podada em QUALQUER campo: o segredo não
				// pode ir para o prompt nem para o histórico da conversa.
				if podada, mudou := podarURLCredenciada(s); mudou {
					t[k] = podada
					continue
				}
			}
			t[k] = podarCamposDeDocumento(val)
		}
		return t
	case []any:
		for i := range t {
			t[i] = podarCamposDeDocumento(t[i])
		}
		return t
	default:
		return v
	}
}

func doGatewayRequest(ctx context.Context, gatewayURL, method, endpoint string, body []byte) (int, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, gatewayURL+endpoint, reqBody)
	if err != nil {
		return 0, nil, err
	}
	if tenantID, ok := ctx.Value(tenantIDKey).(string); ok && tenantID != "" {
		req.Header.Set("X-Tenant-Id", tenantID)
	}
	if auth, ok := ctx.Value(authKey).(string); ok && auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := gatewayHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func applyDryRun(endpoint string, dryRun bool) string {
	if !dryRun {
		return endpoint
	}
	if strings.Contains(endpoint, "?") {
		return endpoint + "&dry_run=true"
	}
	return endpoint + "?dry_run=true"
}

func getStringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func roleRank(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "authenticated":
		return 1
	case "user":
		return 1
	case "broker":
		return 2
	case "owner":
		return 3
	case "service_role":
		return 4
	case "admin":
		return 4
	default:
		return 0
	}
}

// requireMinRole compara o rank VERIFICADO da sessão com o mínimo da tool.
//
// Antes da correção (achado C7) o rank vinha do papel lido de um JWT decodificado
// SEM verificação de assinatura — bastava forjar `app_metadata.user_role` para
// alcançar os tools de escrita. Agora o único rank considerado é o gravado por
// verifiedSessionContext a partir das claims verificadas pelo pkg/auth.
func requireMinRole(ctx context.Context, minRole string) error {
	rank := sessionRank(ctx)
	if rank <= 0 || rank < roleRank(minRole) {
		return fmt.Errorf("insufficient role")
	}
	return nil
}

// sessionRank devolve o rank verificado da sessão (0 quando não há).
func sessionRank(ctx context.Context) int {
	rank, _ := ctx.Value(userRankKey).(int)
	return rank
}

// rankFunctionalRole mapeia o papel funcional VERIFICADO
// (app_metadata.functional_role) para o mesmo rank de roleRank.
//
// Sem esta tradução o rank de escrita era inalcançável: o papel vinha do
// payload não verificado e o toca-auth grava `functional_role`, nunca
// `app_metadata.user_role` — então o único valor real era
// role:"authenticated" (rank 1) e os 22 tools de escrita (x-min-role "broker")
// ficavam negados para todo mundo, inclusive admin.
//
// Só os papéis que docs/features/ROLES-MATRIX.md descreve com escrita em
// CRM/operações alcançam o rank de escrita. financeiro, marketing, auditor,
// parceiro e cliente ficam em rank 1 (somente leitura): o MCP não tem
// granularidade para representar os writes parciais desses perfis, e negar é a
// falha segura.
func rankFunctionalRole(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return 4
	case "gestor":
		return 3
	case "corretor", "operacoes":
		return 2
	case "financeiro", "marketing", "auditor", "parceiro", "cliente":
		return 1
	default:
		return 0
	}
}

// rankFromVerifiedClaims é a ÚNICA derivação de papel do serviço. Recebe apenas
// valores extraídos de um token cuja assinatura foi verificada contra o JWKS do
// Supabase por pkg/auth.
func rankFromVerifiedClaims(userRole, permission, functionalRole string) int {
	// Perfil root (owner/admin): mesma decisão de pkg/auth.IsRootProfile, que
	// libera tudo nos demais serviços do monorepo.
	if pkgAuth.IsRootProfile(permission) {
		return roleRank("admin")
	}
	if r := rankFunctionalRole(functionalRole); r > 0 {
		return r
	}
	return roleRank(userRole)
}

// verifiedSessionContext monta o contexto que as tools recebem a partir das
// claims VERIFICADAS pelo middleware (pkg/auth), e não de headers.
//
// O tenant sai de appCtx.GetTenantID — valor que o middleware gravou a partir de
// `app_metadata.tenant_id` (com o fallback da claim legada `tenant_id`). O header
// X-Tenant-Id NÃO é lido aqui: era exatamente o vetor do achado C7.
func verifiedSessionContext(ctx context.Context, r *http.Request) context.Context {
	tenantID, _ := pkgcontext.GetTenantID(r.Context())
	functionalRole, _ := pkgcontext.GetFunctionalRole(r.Context())
	permission, _ := pkgcontext.GetUserPermission(r.Context())
	userRole, _ := pkgcontext.GetUserRole(r.Context())

	rank := rankFromVerifiedClaims(userRole, permission, functionalRole)

	ctx = context.WithValue(ctx, tenantIDKey, tenantID)
	ctx = context.WithValue(ctx, userRoleKey, strings.ToLower(strings.TrimSpace(functionalRole)))
	ctx = context.WithValue(ctx, userPermKey, permission)
	ctx = context.WithValue(ctx, userRankKey, rank)
	if token, err := pkgcontext.GetAuthToken(r.Context()); err == nil && token != "" {
		ctx = context.WithValue(ctx, authKey, "Bearer "+token)
	}
	return ctx
}

// requireVerifiedTenant devolve 401 quando o token é válido mas não carrega
// tenant: sem tenant não há escopo para nenhuma tool.
func requireVerifiedTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
		if strings.TrimSpace(tenantID) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token sem tenant"})
			return
		}
		c.Next()
	}
}

// filtrarToolsPorRank remove da listagem as tools que o rank VERIFICADO da
// sessão não alcança (o hook OnAfterListTools não tem acesso ao request, então
// usa o rank gravado no contexto da mensagem).
func filtrarToolsPorRank(ctx context.Context, result *mcp.ListToolsResult) {
	rank := sessionRank(ctx)
	if rank <= 0 || result == nil {
		return
	}
	filtered := make([]mcp.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		minRole := ""
		if t.Meta != nil && t.Meta.AdditionalFields != nil {
			if v, ok := t.Meta.AdditionalFields["x-min-role"].(string); ok {
				minRole = v
			}
		}
		if minRole != "" && rank < roleRank(minRole) {
			continue
		}
		filtered = append(filtered, t)
	}
	result.Tools = filtered
}

// instrumentarFerramenta registra o ciclo de cada chamada de ferramenta.
//
// Quando o cliente reporta "timeout waiting for SSE response", há duas causas
// possíveis e indistinguíveis sem log: a ferramenta não rodou, ou rodou e a
// resposta não chegou ao stream SSE. Este log separa as duas.
func instrumentarFerramenta(nome string, h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inicio := time.Now()
		logger.Info("ferramenta iniciando", "tool", nome)
		res, err := h(ctx, req)
		duracao := time.Since(inicio)
		if err != nil {
			logger.Error("ferramenta falhou", "tool", nome, "ms", duracao.Milliseconds(), "erro", err.Error())
			return res, err
		}
		logger.Info("ferramenta concluiu", "tool", nome, "ms", duracao.Milliseconds())
		return res, nil
	}
}

func toolWithMeta(t mcp.Tool, domain, minRole string, readOnly bool, destructive bool) mcp.Tool {
	if t.Annotations.ReadOnlyHint == nil || *t.Annotations.ReadOnlyHint != readOnly {
		t.Annotations.ReadOnlyHint = mcp.ToBoolPtr(readOnly)
	}
	if t.Annotations.DestructiveHint == nil || *t.Annotations.DestructiveHint != destructive {
		t.Annotations.DestructiveHint = mcp.ToBoolPtr(destructive)
	}
	t.Meta = mcp.NewMetaFromMap(map[string]any{
		"x-domain":   domain,
		"x-min-role": minRole,
	})
	return t
}

func main() {
	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://gateway:8080"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	logger.Setup(logger.Config{
		Level:  "info",
		Format: "json",
	})
	logger.Info("Starting toca-mcp service...")

	shutdown, err := tracing.InitProvider("toca-mcp")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		logger.Error("Failed to initialize OpenTelemetry", "error", err)
	}

	// P0/C7: o MESMO middleware de JWT dos outros serviços valida as rotas MCP,
	// então o JWKS é inicializado no boot. Reusa a precedência de pkg/config:
	// SUPABASE_JWKS_URL e, na ausência, SUPABASE_URL + o caminho padrão do
	// Supabase — que é o que o compose já injeta neste serviço.
	if err := pkgAuth.InitJWKS(jwksURLFromEnv()); err != nil {
		logger.Error("Failed to initialize JWKS — rotas MCP vão negar todo acesso", "error", err)
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	pkgAuth.LogServiceKeyRollout()

	s := server.NewMCPServer(
		"toca-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithHooks(mcpServerHooks()),
		server.WithLogging(),
	)

	// ==========================================
	// READ TOOLS (Consultas)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("get_company_settings",
		mcp.WithDescription("Fetch the active company settings and white-label preferences (Theme, Appearance) for the current tenant."),
	), "settings", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return makeGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/company-settings/appearance", nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_leads",
		mcp.WithDescription("Search for leads in the CRM based on specific criteria. Respects tenant RBAC policies."),
		mcp.WithString("status", mcp.Description("Filter by lead status (e.g. NEW, CONTACTED, QUALIFIED)")),
		mcp.WithString("status_in", mcp.Description("CSV list of lead statuses (e.g. NEW,CONTACTED,QUALIFIED)")),
		mcp.WithString("search", mcp.Description("Free-text search (name, email, phone, display_id, preferred_location)")),
		mcp.WithString("role", mcp.Description("Filter by lead role/type (buyer, seller, tenant, landlord)")),
		mcp.WithBoolean("is_client", mcp.Description("Filter by whether the lead is already a client")),
		mcp.WithString("display_id", mcp.Description("Exact match by DisplayID")),
		mcp.WithString("email", mcp.Description("Exact match by email")),
		mcp.WithString("phone", mcp.Description("Exact match by phone")),
		mcp.WithString("account_id", mcp.Description("Filter by AccountID")),
		mcp.WithString("contract_id", mcp.Description("Filter by ContractID")),
		mcp.WithString("source", mcp.Description("Filter by Source")),
		mcp.WithString("source_in", mcp.Description("CSV list of sources")),
		mcp.WithString("tag", mcp.Description("Filter by derived tag")),
		mcp.WithBoolean("qualified", mcp.Description("Shortcut for qualified statuses")),
		mcp.WithString("assigned_broker_id", mcp.Description("Filter by assigned broker ID")),
		mcp.WithBoolean("unassigned", mcp.Description("Filter for unassigned leads")),
		mcp.WithString("assigned_from", mcp.Description("AssignedAt >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("assigned_to", mcp.Description("AssignedAt <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("expires_before", mcp.Description("ExpiresAt <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("expires_after", mcp.Description("ExpiresAt >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithNumber("escalation_level_min", mcp.Description("EscalationLevel >= N")),
		mcp.WithNumber("roulette_retries_min", mcp.Description("RouletteRetries >= N")),
		mcp.WithString("tier_id", mcp.Description("Filter by SLARetries tier key")),
		mcp.WithString("created_from", mcp.Description("CreatedAt >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("created_to", mcp.Description("CreatedAt <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("updated_from", mcp.Description("UpdatedAt >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("updated_to", mcp.Description("UpdatedAt <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("last_interaction_from", mcp.Description("LastInteraction >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("last_interaction_to", mcp.Description("LastInteraction <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithNumber("stale_days", mcp.Description("LastInteraction < NOW() - N days")),
		mcp.WithString("property_id", mcp.Description("Filter by PropertyID in PropertyIDs array")),
		mcp.WithString("captacao_property_id", mcp.Description("Filter by CaptacaoPropertyID")),
		mcp.WithString("owner_lead_id", mcp.Description("Filter by OwnerLeadID")),
		mcp.WithBoolean("is_captacao", mcp.Description("CaptacaoPropertyID != ''")),
		mcp.WithString("interest_type", mcp.Description("Filter by property interest type")),
		mcp.WithString("interest_location", mcp.Description("ILIKE match on preferred location")),
		mcp.WithNumber("interest_min_price", mcp.Description("Min interest price")),
		mcp.WithNumber("interest_max_price", mcp.Description("Max interest price")),
		mcp.WithNumber("interest_bedrooms_min", mcp.Description("Min bedrooms")),
		mcp.WithNumber("interest_bathrooms_min", mcp.Description("Min bathrooms")),
		mcp.WithNumber("interest_parking_min", mcp.Description("Min parking")),
		mcp.WithNumber("interest_area_min", mcp.Description("Min area")),
		mcp.WithNumber("interest_area_max", mcp.Description("Max area")),
		mcp.WithString("note_search", mcp.Description("ILIKE match on history/details")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of leads to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		status, _ := args["status"].(string)
		statusIn, _ := args["status_in"].(string)
		search, _ := args["search"].(string)
		role, _ := args["role"].(string)
		_, hasIsClient := args["is_client"].(bool)
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/leads?limit=%v&offset=%v", limit, offset)
		addStr := func(k, v string) {
			if strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addBool := func(k string) {
			if v, ok := args[k].(bool); ok {
				if v {
					endpoint += "&" + k + "=true"
				} else {
					endpoint += "&" + k + "=false"
				}
			}
		}
		if strings.TrimSpace(status) != "" {
			endpoint += "&status=" + url.QueryEscape(strings.TrimSpace(status))
		}
		addStr("status_in", statusIn)
		if strings.TrimSpace(search) != "" {
			endpoint += "&search=" + url.QueryEscape(strings.TrimSpace(search))
		}
		if strings.TrimSpace(role) != "" {
			endpoint += "&role=" + url.QueryEscape(strings.TrimSpace(role))
		}
		if hasIsClient {
			addBool("is_client")
		}
		addStr("display_id", getStringArg(args, "display_id"))
		addStr("email", getStringArg(args, "email"))
		addStr("phone", getStringArg(args, "phone"))
		addStr("account_id", getStringArg(args, "account_id"))
		addStr("contract_id", getStringArg(args, "contract_id"))
		addStr("source", getStringArg(args, "source"))
		addStr("source_in", getStringArg(args, "source_in"))
		addStr("tag", getStringArg(args, "tag"))
		addBool("qualified")
		addStr("assigned_broker_id", getStringArg(args, "assigned_broker_id"))
		addBool("unassigned")
		addStr("assigned_from", getStringArg(args, "assigned_from"))
		addStr("assigned_to", getStringArg(args, "assigned_to"))
		addStr("expires_before", getStringArg(args, "expires_before"))
		addStr("expires_after", getStringArg(args, "expires_after"))
		addNum("escalation_level_min")
		addNum("roulette_retries_min")
		addStr("tier_id", getStringArg(args, "tier_id"))
		addStr("created_from", getStringArg(args, "created_from"))
		addStr("created_to", getStringArg(args, "created_to"))
		addStr("updated_from", getStringArg(args, "updated_from"))
		addStr("updated_to", getStringArg(args, "updated_to"))
		addStr("last_interaction_from", getStringArg(args, "last_interaction_from"))
		addStr("last_interaction_to", getStringArg(args, "last_interaction_to"))
		addNum("stale_days")
		addStr("property_id", getStringArg(args, "property_id"))
		addStr("captacao_property_id", getStringArg(args, "captacao_property_id"))
		addStr("owner_lead_id", getStringArg(args, "owner_lead_id"))
		addBool("is_captacao")
		addStr("interest_type", getStringArg(args, "interest_type"))
		addStr("interest_location", getStringArg(args, "interest_location"))
		addNum("interest_min_price")
		addNum("interest_max_price")
		addNum("interest_bedrooms_min")
		addNum("interest_bathrooms_min")
		addNum("interest_parking_min")
		addNum("interest_area_min")
		addNum("interest_area_max")
		addStr("note_search", getStringArg(args, "note_search"))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_properties",
		mcp.WithDescription("Search for properties in the inventory. Returns available real estate units based on filters."),
		mcp.WithString("q", mcp.Description("Free-text query (bairro, endereço, título, etc.)")),
		mcp.WithString("address", mcp.Description("ILIKE match on address/street")),
		mcp.WithString("neighborhood", mcp.Description("ILIKE match on neighborhood")),
		mcp.WithString("city", mcp.Description("ILIKE match on city")),
		mcp.WithString("state", mcp.Description("ILIKE match on state")),
		mcp.WithString("zip_code", mcp.Description("Exact match on zip code/postal code")),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. available, sold, rented)")),
		mcp.WithString("type", mcp.Description("Filter by property type (e.g. apartment, house, commercial)")),
		mcp.WithString("listing_type", mcp.Description("Filter by listing type (rent, sale)")),
		mcp.WithNumber("min_price", mcp.Description("Minimum price")),
		mcp.WithNumber("max_price", mcp.Description("Maximum price")),
		mcp.WithNumber("min_bedrooms", mcp.Description("Minimum bedrooms")),
		mcp.WithNumber("min_bathrooms", mcp.Description("Minimum bathrooms")),
		mcp.WithNumber("min_area", mcp.Description("Minimum area (m²)")),
		mcp.WithNumber("max_area", mcp.Description("Maximum area (m²)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of properties to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "inventory", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		q, _ := args["q"].(string)
		address, _ := args["address"].(string)
		neighborhood, _ := args["neighborhood"].(string)
		city, _ := args["city"].(string)
		state, _ := args["state"].(string)
		zipCode, _ := args["zip_code"].(string)
		status, _ := args["status"].(string)
		pType, _ := args["type"].(string)
		listingType, _ := args["listing_type"].(string)
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/properties?limit=%v&offset=%v", limit, offset)
		if strings.TrimSpace(q) != "" {
			endpoint += "&q=" + url.QueryEscape(strings.TrimSpace(q))
		}
		if strings.TrimSpace(address) != "" {
			endpoint += "&address=" + url.QueryEscape(strings.TrimSpace(address))
		}
		if strings.TrimSpace(neighborhood) != "" {
			endpoint += "&neighborhood=" + url.QueryEscape(strings.TrimSpace(neighborhood))
		}
		if strings.TrimSpace(city) != "" {
			endpoint += "&city=" + url.QueryEscape(strings.TrimSpace(city))
		}
		if strings.TrimSpace(state) != "" {
			endpoint += "&state=" + url.QueryEscape(strings.TrimSpace(state))
		}
		if strings.TrimSpace(zipCode) != "" {
			endpoint += "&zip_code=" + url.QueryEscape(strings.TrimSpace(zipCode))
		}
		if strings.TrimSpace(status) != "" {
			endpoint += "&status=" + url.QueryEscape(strings.TrimSpace(status))
		}
		if strings.TrimSpace(pType) != "" {
			endpoint += "&type=" + url.QueryEscape(strings.TrimSpace(pType))
		}
		if strings.TrimSpace(listingType) != "" {
			endpoint += "&listing_type=" + url.QueryEscape(strings.TrimSpace(listingType))
		}
		if v, ok := args["min_price"].(float64); ok {
			endpoint += fmt.Sprintf("&min_price=%v", v)
		}
		if v, ok := args["max_price"].(float64); ok {
			endpoint += fmt.Sprintf("&max_price=%v", v)
		}
		if v, ok := args["min_bedrooms"].(float64); ok {
			endpoint += fmt.Sprintf("&min_bedrooms=%d", int(v))
		}
		if v, ok := args["min_bathrooms"].(float64); ok {
			endpoint += fmt.Sprintf("&min_bathrooms=%d", int(v))
		}
		if v, ok := args["min_area"].(float64); ok {
			endpoint += fmt.Sprintf("&min_area=%v", v)
		}
		if v, ok := args["max_area"].(float64); ok {
			endpoint += fmt.Sprintf("&max_area=%v", v)
		}
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_brokers",
		mcp.WithDescription("Search for brokers in the CRM based on specific criteria. Respects tenant RBAC policies."),
		mcp.WithString("search", mcp.Description("Free-text search (name, email)")),
		mcp.WithString("specialty", mcp.Description("CSV list of specialties (array contains)")),
		mcp.WithString("region", mcp.Description("CSV list of regions (array contains)")),
		mcp.WithBoolean("active", mcp.Description("Filter by Active")),
		mcp.WithBoolean("paused", mcp.Description("Filter by Paused")),
		mcp.WithString("role", mcp.Description("Filter by Role")),
		mcp.WithBoolean("is_priority", mcp.Description("Filter by IsPriority")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of brokers to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/brokers?limit=%v&offset=%v", limit, offset)
		if v, ok := args["search"].(string); ok && strings.TrimSpace(v) != "" {
			endpoint += "&search=" + url.QueryEscape(strings.TrimSpace(v))
		}
		if v, ok := args["specialty"].(string); ok && strings.TrimSpace(v) != "" {
			endpoint += "&specialty=" + url.QueryEscape(strings.TrimSpace(v))
		}
		if v, ok := args["region"].(string); ok && strings.TrimSpace(v) != "" {
			endpoint += "&region=" + url.QueryEscape(strings.TrimSpace(v))
		}
		if v, ok := args["role"].(string); ok && strings.TrimSpace(v) != "" {
			endpoint += "&role=" + url.QueryEscape(strings.TrimSpace(v))
		}
		if v, ok := args["active"].(bool); ok {
			if v {
				endpoint += "&active=true"
			} else {
				endpoint += "&active=false"
			}
		}
		if v, ok := args["paused"].(bool); ok {
			if v {
				endpoint += "&paused=true"
			} else {
				endpoint += "&paused=false"
			}
		}
		if v, ok := args["is_priority"].(bool); ok {
			if v {
				endpoint += "&is_priority=true"
			} else {
				endpoint += "&is_priority=false"
			}
		}
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_broker_by_id",
		mcp.WithDescription("Fetch a single broker by its ID."),
		mcp.WithString("broker_id", mcp.Required(), mcp.Description("Broker ID")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := request.Params.Arguments.(map[string]interface{})
		brokerID := getStringArg(args, "broker_id")
		if brokerID == "" {
			return mcp.NewToolResultError("broker_id is required"), nil
		}
		endpoint := fmt.Sprintf("/api/v1/brokers/%s", url.PathEscape(brokerID))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_lead_by_email",
		mcp.WithDescription("Fetch a single lead by its email (exact match)."),
		mcp.WithString("email", mcp.Required(), mcp.Description("Lead email")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		email := args["email"].(string)
		endpoint := fmt.Sprintf("/api/v1/leads/by-email/%s", url.PathEscape(email))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_lead_by_phone",
		mcp.WithDescription("Fetch a single lead by its phone (exact match)."),
		mcp.WithString("phone", mcp.Required(), mcp.Description("Lead phone")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		phone := args["phone"].(string)
		endpoint := fmt.Sprintf("/api/v1/leads/by-phone/%s", url.PathEscape(phone))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_lead_by_display_id",
		mcp.WithDescription("Fetch a single lead by its display_id (exact match)."),
		mcp.WithString("display_id", mcp.Required(), mcp.Description("Lead display ID")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		displayID := args["display_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/leads/by-display-id/%s", url.PathEscape(displayID))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_lead_by_id",
		mcp.WithDescription("Fetch a single lead by its ID (includes full details and history when available)."),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("Lead ID")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		leadID := args["lead_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/leads/%s", leadID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_property_by_id",
		mcp.WithDescription("Fetch a single property by its ID (includes owners and related entities when available)."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("Property ID")),
	), "inventory", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		propertyID := args["property_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/properties/%s", propertyID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_deals",
		mcp.WithDescription("Search for deals/negotiations based on specific criteria. Respects tenant RBAC policies."),
		mcp.WithString("status", mcp.Description("Filter by deal status")),
		mcp.WithString("type", mcp.Description("Filter by deal type (sale/rent)")),
		mcp.WithNumber("min_value", mcp.Description("Minimum deal value")),
		mcp.WithNumber("max_value", mcp.Description("Maximum deal value")),
		mcp.WithString("property_id", mcp.Description("Filter by property_id")),
		mcp.WithString("property_address", mcp.Description("ILIKE match on property address")),
		mcp.WithString("party_name", mcp.Description("ILIKE match on party name")),
		mcp.WithString("party_email", mcp.Description("ILIKE match on party/signer email")),
		mcp.WithString("lead_id", mcp.Description("Filter by party lead_id")),
		mcp.WithString("broker_id", mcp.Description("Filter by broker_id")),
		mcp.WithString("created_from", mcp.Description("CreatedAt >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("created_to", mcp.Description("CreatedAt <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("search", mcp.Description("Free-text search (property address + party name)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of deals to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "deals", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/deals?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addStr("status")
		addStr("type")
		addNum("min_value")
		addNum("max_value")
		addStr("property_id")
		addStr("property_address")
		addStr("party_name")
		addStr("party_email")
		addStr("lead_id")
		addStr("broker_id")
		addStr("created_from")
		addStr("created_to")
		addStr("search")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_deal_by_id",
		mcp.WithDescription("Fetch a deal/negotiation by its ID, including current stage and key fields."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
	), "deals", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/deals/%s", dealID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_contract_by_id",
		mcp.WithDescription("Fetch a contract by its ID."),
		mcp.WithString("contract_id", mcp.Required(), mcp.Description("Contract ID")),
	), "contracts", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		contractID := args["contract_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/contracts/%s", contractID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("list_contract_templates",
		mcp.WithDescription("List all contract templates available for the current tenant."),
	), "contracts", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return makeGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/contract-templates", nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_contract_template_by_id",
		mcp.WithDescription("Fetch a contract template by its ID (including variables and base64 content)."),
		mcp.WithString("template_id", mcp.Required(), mcp.Description("Contract template ID")),
	), "contracts", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		templateID := args["template_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/contract-templates/%s", templateID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_calendar_events",
		mcp.WithDescription("Fetch upcoming appointments, visits, and meetings from the calendar."),
		mcp.WithNumber("limit", mcp.Description("Maximum number of events to return")),
	), "calendar", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		endpoint := fmt.Sprintf("/api/v1/appointments?limit=%v", limit)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_appointments",
		mcp.WithDescription("Search calendar appointments using filters and free-text search."),
		mcp.WithString("agent_id", mcp.Description("Filter by agent_id")),
		mcp.WithString("lead_id", mcp.Description("Filter by lead_id")),
		mcp.WithString("property_id", mcp.Description("Filter by property_id")),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithString("priority", mcp.Description("Filter by priority")),
		mcp.WithString("client_name", mcp.Description("ILIKE match on client")),
		mcp.WithString("agent_name", mcp.Description("ILIKE match on agent")),
		mcp.WithString("start_date", mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("end_date", mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("search", mcp.Description("Free-text search (title, client, agent)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of appointments to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "calendar", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/appointments?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addStr("agent_id")
		addStr("lead_id")
		addStr("property_id")
		addStr("status")
		addStr("type")
		addStr("priority")
		addStr("client_name")
		addStr("agent_name")
		addStr("start_date")
		addStr("end_date")
		addStr("search")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_appointment_by_id",
		mcp.WithDescription("Fetch a single appointment by its ID."),
		mcp.WithString("appointment_id", mcp.Required(), mcp.Description("Appointment ID")),
	), "calendar", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		appointmentID := args["appointment_id"].(string)
		endpoint := fmt.Sprintf("/api/v1/appointments/%s", appointmentID)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_deals_pipeline",
		mcp.WithDescription("Fetch the active deals (Esteira Digital) in the negotiation pipeline."),
		mcp.WithString("status", mcp.Description("Filter by deal stage (e.g. proposal, due_diligence, contract)")),
	), "deals", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		status, _ := args["status"].(string)
		endpoint := fmt.Sprintf("/api/v1/deals?status=%s", status)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	// ==========================================
	// READ TOOLS (Financeiro)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("search_charges",
		mcp.WithDescription("Search accounts receivable (charges) such as rent, installments, or fees."),
		mcp.WithString("customer_id", mcp.Description("Filter by customer/party ID")),
		mcp.WithString("customer_name", mcp.Description("ILIKE match on customer name")),
		mcp.WithString("deal_id", mcp.Description("Filter by deal ID")),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. PENDING, PAID, OVERDUE)")),
		mcp.WithNumber("min_amount", mcp.Description("Minimum amount")),
		mcp.WithNumber("max_amount", mcp.Description("Maximum amount")),
		mcp.WithString("due_from", mcp.Description("Due date >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("due_to", mcp.Description("Due date <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("paid_from", mcp.Description("Paid at >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("paid_to", mcp.Description("Paid at <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("payment_method", mcp.Description("Filter by payment method")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of charges to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/charges?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addStr("customer_id")
		addStr("customer_name")
		addStr("deal_id")
		addStr("status")
		addNum("min_amount")
		addNum("max_amount")
		addStr("due_from")
		addStr("due_to")
		addStr("paid_from")
		addStr("paid_to")
		addStr("payment_method")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_transactions",
		mcp.WithDescription("Search financial transactions (cash flow, income, expenses) for the active tenant."),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("type", mcp.Description("Filter by type (income/expense)")),
		mcp.WithNumber("min_amount", mcp.Description("Minimum amount")),
		mcp.WithNumber("max_amount", mcp.Description("Maximum amount")),
		mcp.WithString("date_from", mcp.Description("Date >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("date_to", mcp.Description("Date <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithString("description", mcp.Description("ILIKE match on description")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of transactions to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/transactions?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addStr("category")
		addStr("type")
		addNum("min_amount")
		addNum("max_amount")
		addStr("date_from")
		addStr("date_to")
		addStr("status")
		addStr("description")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_transfers",
		mcp.WithDescription("Search transfers/repasses to property owners."),
		mcp.WithString("broker_id", mcp.Description("Filter by broker/owner ID")),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithNumber("min_amount", mcp.Description("Minimum amount")),
		mcp.WithNumber("max_amount", mcp.Description("Maximum amount")),
		mcp.WithString("date_from", mcp.Description("Created date >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("date_to", mcp.Description("Created date <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of transfers to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/transfers?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addStr("broker_id")
		addStr("status")
		addNum("min_amount")
		addNum("max_amount")
		addStr("date_from")
		addStr("date_to")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("search_invoices",
		mcp.WithDescription("Search financial invoices (faturas)."),
		mcp.WithString("customer_id", mcp.Description("Filter by customer/payer ID")),
		mcp.WithString("customer_name", mcp.Description("ILIKE match on customer name")),
		mcp.WithString("status", mcp.Description("Filter by invoice status")),
		mcp.WithNumber("min_amount", mcp.Description("Minimum amount")),
		mcp.WithNumber("max_amount", mcp.Description("Maximum amount")),
		mcp.WithString("issued_from", mcp.Description("Issued date >= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithString("issued_to", mcp.Description("Issued date <= (RFC3339 or YYYY-MM-DD)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of invoices to return")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination (default 0)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		offset, ok := args["offset"].(float64)
		if !ok {
			offset = 0
		}
		endpoint := fmt.Sprintf("/api/v1/invoices?limit=%v&offset=%v", limit, offset)
		addStr := func(k string) {
			if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
				endpoint += "&" + k + "=" + url.QueryEscape(strings.TrimSpace(v))
			}
		}
		addNum := func(k string) {
			if v, ok := args[k].(float64); ok {
				endpoint += fmt.Sprintf("&%s=%v", k, v)
			}
		}
		addStr("customer_id")
		addStr("customer_name")
		addStr("status")
		addNum("min_amount")
		addNum("max_amount")
		addStr("issued_from")
		addStr("issued_to")
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_financial_transactions",
		mcp.WithDescription("Fetch financial transactions (cash flow, income, expenses) for the active tenant."),
		mcp.WithString("contract_id", mcp.Description("Filter by a specific contract ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of transactions to return (default 50)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 50
		}

		endpoint := fmt.Sprintf("/api/v1/transactions?limit=%v", limit)
		if contractID, ok := args["contract_id"].(string); ok && contractID != "" {
			endpoint += fmt.Sprintf("&contract_id=%s", contractID)
		}

		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_financial_charges",
		mcp.WithDescription("Fetch accounts receivable (charges) such as rent, installments, or fees."),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. PENDING, PAID, OVERDUE)")),
		mcp.WithString("contract_id", mcp.Description("Filter by contract ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of charges to return (default 50)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 50
		}

		endpoint := fmt.Sprintf("/api/v1/charges?limit=%v", limit)
		if status, ok := args["status"].(string); ok && status != "" {
			endpoint += fmt.Sprintf("&status=%s", status)
		}
		if contractID, ok := args["contract_id"].(string); ok && contractID != "" {
			endpoint += fmt.Sprintf("&contract_id=%s", contractID)
		}

		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_financial_transfers",
		mcp.WithDescription("Fetch accounts payable (transfers/repasses) to property owners or broker commissions."),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. PENDING, TRANSFERRED, FAILED)")),
		mcp.WithString("owner_id", mcp.Description("Filter by property owner ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of transfers to return (default 50)")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 50
		}

		endpoint := fmt.Sprintf("/api/v1/transfers?limit=%v", limit)
		if status, ok := args["status"].(string); ok && status != "" {
			endpoint += fmt.Sprintf("&status=%s", status)
		}
		if ownerID, ok := args["owner_id"].(string); ok && ownerID != "" {
			endpoint += fmt.Sprintf("&owner_id=%s", ownerID)
		}

		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_financial_dashboard",
		mcp.WithDescription("Fetch the financial dashboard containing high-level cash flow metrics and delinquency (inadimplência) indicators."),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return makeGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/delinquency/dashboard", nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_financial_invoices",
		mcp.WithDescription("Fetch financial invoices (faturas)."),
		mcp.WithString("status", mcp.Description("Filter by invoice status")),
		mcp.WithString("reference_id", mcp.Description("Filter by a specific reference ID")),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		endpoint := "/api/v1/invoices?"
		if status, ok := args["status"].(string); ok && status != "" {
			endpoint += fmt.Sprintf("status=%s&", status)
		}
		if refID, ok := args["reference_id"].(string); ok && refID != "" {
			endpoint += fmt.Sprintf("reference_id=%s", refID)
		}
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_fiscal_reports",
		mcp.WithDescription("Fetch the consolidated fiscal and tax reports history."),
		mcp.WithNumber("limit", mcp.Description("Maximum number of reports to return (default 20)")),
	), "fiscal", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		limit, ok := args["limit"].(float64)
		if !ok {
			limit = 20
		}
		endpoint := fmt.Sprintf("/api/v1/fiscal/reports?limit=%v", limit)
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_delinquency_aging",
		mcp.WithDescription("Fetch the detailed aging report of overdue debts (inadimplência por período)."),
	), "finance", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return makeGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/delinquency/aging", nil)
	})

	// ==========================================
	// READ TOOLS (Contratos e Documentos - Esteira Digital)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("get_contracts",
		mcp.WithDescription("Fetch the list of contracts, optionally filtering by deal ID."),
		mcp.WithString("deal_id", mcp.Description("Filter contracts by a specific deal/negotiation ID")),
	), "contracts", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		endpoint := "/api/v1/contracts"
		if dealID, ok := args["deal_id"].(string); ok && dealID != "" {
			endpoint = fmt.Sprintf("/api/v1/deals/%s/contracts", dealID)
		}
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_documents",
		mcp.WithDescription("Fetch the central document repository files, optionally filtering by entity."),
		mcp.WithString("entity_type", mcp.Description("Filter by entity type (e.g. property, lead, deal)")),
		mcp.WithString("entity_id", mcp.Description("Filter by specific entity ID")),
	), "documents", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments.(map[string]interface{})
		endpoint := "/api/v1/documents"
		eType, typeOk := args["entity_type"].(string)
		eID, idOk := args["entity_id"].(string)

		if typeOk && idOk && eType != "" && eID != "" {
			endpoint = fmt.Sprintf("/api/v1/documents/entity/%s/%s", eType, eID)
		}
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	// ==========================================
	// READ TOOLS (CRM - Corretores)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("get_brokers",
		mcp.WithDescription("Fetch the list of brokers (corretores) registered in the real estate agency."),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return makeGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/brokers", nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_broker_performance",
		mcp.WithDescription("Fetch performance metrics and statistics for a specific broker."),
		mcp.WithString("broker_id", mcp.Required(), mcp.Description("The ID of the broker")),
	), "crm", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := request.Params.Arguments.(map[string]interface{})
		brokerID := getStringArg(args, "broker_id")
		if brokerID == "" {
			return mcp.NewToolResultError("broker_id is required"), nil
		}
		endpoint := fmt.Sprintf("/api/v1/brokers/%s/performance", url.PathEscape(brokerID))
		return makeGatewayRequest(ctx, gatewayURL, "GET", endpoint, nil)
	})

	// ==========================================
	// WRITE TOOLS (Ações de Negócios e Documentos)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("create_property",
		mcp.WithDescription("Add a new property to the real estate inventory."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Property title or short description")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Property type (apartment, house, commercial, land)")),
		mcp.WithString("status", mcp.Required(), mcp.Description("Current status (available, sold, rented)")),
		mcp.WithNumber("price", mcp.Required(), mcp.Description("Listing price")),
		mcp.WithNumber("bedrooms", mcp.Description("Number of bedrooms")),
		mcp.WithNumber("area", mcp.Description("Total area in square meters")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "inventory", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		endpoint := applyDryRun("/api/v1/properties", dryRun)
		return makeGatewayRequest(ctx, gatewayURL, "POST", endpoint, body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("add_property_owner",
		mcp.WithDescription("Link an existing lead/contact as the owner of a specific property."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("The ID of the property")),
		mcp.WithString("owner_id", mcp.Required(), mcp.Description("The ID of the lead who owns the property")),
		mcp.WithNumber("percentage", mcp.Description("Ownership percentage (default 100)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "inventory", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		propertyID := args["property_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		payload := map[string]interface{}{
			"owner_id": args["owner_id"],
		}
		if pct, ok := args["percentage"]; ok {
			payload["percentage"] = pct
		} else {
			payload["percentage"] = 100
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/properties/%s/owners", propertyID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_deal",
		mcp.WithDescription("Start a new deal/negotiation in the Esteira Digital pipeline."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Title for the deal (e.g. Sale of Apt 101)")),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("The ID of the property being negotiated")),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("The ID of the buyer/tenant lead")),
		mcp.WithNumber("value", mcp.Required(), mcp.Description("Negotiated value")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Deal type (sale, rent)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun("/api/v1/deals", dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("add_deal_checklist_item",
		mcp.WithDescription("Add a single item to the deal checklist (document requirement)."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Checklist item name (e.g. RG/CPF, comprovante de renda)")),
		mcp.WithString("category", mcp.Required(), mcp.Description("Category (e.g. identity, income, property, contract, other)")),
		mcp.WithString("party_id", mcp.Description("Optional party/signer ID to request from (if applicable)")),
		mcp.WithBoolean("required", mcp.Description("Whether it is required to complete the deal")),
		mcp.WithString("description", mcp.Description("Optional description/instructions")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		delete(args, "deal_id")
		body, _ := json.Marshal(args)
		endpoint := fmt.Sprintf("/api/v1/deals/%s/checklist", dealID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("init_deal_checklist",
		mcp.WithDescription("Replace the entire deal checklist (apply a template). Provide items_json as a JSON array of checklist items."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
		mcp.WithString("items_json", mcp.Required(), mcp.Description("JSON array of items: [{\"name\":\"...\",\"category\":\"...\",\"required\":true,\"description\":\"...\",\"party_id\":\"...\"}]")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		itemsStr := args["items_json"].(string)
		var items any
		if err := json.Unmarshal([]byte(itemsStr), &items); err != nil {
			return mcp.NewToolResultError("invalid items_json"), nil
		}
		body, _ := json.Marshal(map[string]any{"items": items})
		endpoint := fmt.Sprintf("/api/v1/deals/%s/checklist", dealID)
		return makeGatewayRequest(ctx, gatewayURL, "PUT", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("update_deal_checklist_item_status",
		mcp.WithDescription("Update a checklist item status (approve/reject/upload/etc.)."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("Checklist item ID")),
		mcp.WithString("status", mcp.Required(), mcp.Description("New status (e.g. pending, solicited, uploaded, approved, rejected)")),
		mcp.WithString("rejection_reason", mcp.Description("Reason when status is rejected")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		itemID := args["item_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		payload := map[string]any{
			"status": args["status"],
		}
		if v, ok := args["rejection_reason"].(string); ok && strings.TrimSpace(v) != "" {
			payload["rejection_reason"] = strings.TrimSpace(v)
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/deals/%s/checklist/%s", dealID, itemID)
		return makeGatewayRequest(ctx, gatewayURL, "PATCH", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("remove_deal_checklist_item",
		mcp.WithDescription("Remove an item from the deal checklist."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("Checklist item ID")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		itemID := args["item_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		endpoint := fmt.Sprintf("/api/v1/deals/%s/checklist/%s", dealID, itemID)
		return makeGatewayRequest(ctx, gatewayURL, "DELETE", applyDryRun(endpoint, dryRun), nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("solicit_deal_checklist_item",
		mcp.WithDescription("Mark a checklist item as solicited (requested from client)."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("Deal ID")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("Checklist item ID")),
		mcp.WithString("party_id", mcp.Description("Optional party/signer ID to request from")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		itemID := args["item_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		payload := map[string]any{}
		if v, ok := args["party_id"].(string); ok && strings.TrimSpace(v) != "" {
			payload["party_id"] = strings.TrimSpace(v)
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/deals/%s/checklist/%s/solicit", dealID, itemID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_contract",
		mcp.WithDescription("Create a new contract for a deal/negotiation."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("The ID of the deal")),
		mcp.WithString("template_id", mcp.Required(), mcp.Description("The ID of the contract template to use")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Contract title")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "contracts", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun("/api/v1/contracts", dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_lead",
		mcp.WithDescription("Create a new lead/client in the CRM."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Lead's full name")),
		mcp.WithString("email", mcp.Description("Lead's email address")),
		mcp.WithString("phone", mcp.Description("Lead's phone number")),
		mcp.WithString("type", mcp.Description("Lead type (buyer, seller, tenant, landlord)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "crm", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun("/api/v1/leads", dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("add_lead_note",
		mcp.WithDescription("Add a history note or comment to a specific lead."),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("The ID of the lead")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The content of the note")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "crm", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		leadID := args["lead_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		body, _ := json.Marshal(map[string]string{"content": args["content"].(string)})
		endpoint := fmt.Sprintf("/api/v1/leads/%s/notes", leadID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("add_lead_note_from_screen",
		mcp.WithDescription("Add a note to the lead currently in context (tela atual). lead_id can be omitted when the caller can infer it from screen context."),
		mcp.WithString("lead_id", mcp.Description("Lead ID (optional when inferred from screen context)")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The content of the note")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "crm", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		leadID, _ := args["lead_id"].(string)
		leadID = strings.TrimSpace(leadID)
		if leadID == "" {
			return mcp.NewToolResultError("missing lead_id"), nil
		}
		dryRun, _ := args["dry_run"].(bool)
		body, _ := json.Marshal(map[string]string{"content": args["content"].(string)})
		endpoint := fmt.Sprintf("/api/v1/leads/%s/notes", leadID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("update_lead",
		mcp.WithDescription("Update an existing lead (status, assigned broker, contact info, role, qualification)."),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("Lead ID")),
		mcp.WithString("name", mcp.Description("Lead name")),
		mcp.WithString("email", mcp.Description("Lead email")),
		mcp.WithString("phone", mcp.Description("Lead phone")),
		mcp.WithString("status", mcp.Description("Lead status (e.g. novo, contato, agendado)")),
		mcp.WithString("assigned_broker_id", mcp.Description("Assign a broker (account ID) to this lead")),
		mcp.WithString("role", mcp.Description("Client role (owner, buyer, tenant, guarantor)")),
		mcp.WithString("captacao_property_id", mcp.Description("Link this lead as captação to a property (property ID)")),
		mcp.WithString("owner_lead_id", mcp.Description("Link this lead as owner lead ID (lead ID)")),
		mcp.WithString("qualification_json", mcp.Description("JSON object with qualification fields")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "crm", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		leadID := args["lead_id"].(string)
		dryRun, _ := args["dry_run"].(bool)

		payload := map[string]interface{}{}
		if v, ok := args["name"].(string); ok && strings.TrimSpace(v) != "" {
			payload["name"] = strings.TrimSpace(v)
		}
		if v, ok := args["email"].(string); ok && strings.TrimSpace(v) != "" {
			payload["email"] = strings.TrimSpace(v)
		}
		if v, ok := args["phone"].(string); ok && strings.TrimSpace(v) != "" {
			payload["phone"] = strings.TrimSpace(v)
		}
		if v, ok := args["status"].(string); ok && strings.TrimSpace(v) != "" {
			payload["status"] = strings.TrimSpace(v)
		}
		if v, ok := args["assigned_broker_id"].(string); ok && strings.TrimSpace(v) != "" {
			payload["assigned_broker_id"] = strings.TrimSpace(v)
		}
		if v, ok := args["role"].(string); ok && strings.TrimSpace(v) != "" {
			payload["role"] = strings.TrimSpace(v)
		}
		if v, ok := args["captacao_property_id"].(string); ok && strings.TrimSpace(v) != "" {
			payload["captacao_property_id"] = strings.TrimSpace(v)
		}
		if v, ok := args["owner_lead_id"].(string); ok && strings.TrimSpace(v) != "" {
			payload["owner_lead_id"] = strings.TrimSpace(v)
		}
		if v, ok := args["qualification_json"].(string); ok && strings.TrimSpace(v) != "" {
			var q any
			if err := json.Unmarshal([]byte(v), &q); err != nil {
				return mcp.NewToolResultError("invalid qualification_json"), nil
			}
			payload["qualification"] = q
		}

		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/leads/%s", leadID)
		return makeGatewayRequest(ctx, gatewayURL, "PATCH", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("link_lead_to_property",
		mcp.WithDescription("Link a lead to a property as 'interested lead' (economiza cliques de vínculo)."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("Property ID")),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("Lead ID")),
		mcp.WithString("source", mcp.Description("Optional source (e.g. whatsapp, portal, indicação)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "inventory", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		propertyID := args["property_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		payload := map[string]interface{}{
			"lead_id": args["lead_id"],
		}
		if v, ok := args["source"].(string); ok && strings.TrimSpace(v) != "" {
			payload["source"] = strings.TrimSpace(v)
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/properties/%s/interested-leads", propertyID)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("unlink_lead_from_property",
		mcp.WithDescription("Remove an interested lead from a property."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("Property ID")),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("Lead ID")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "inventory", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		propertyID := args["property_id"].(string)
		leadID := args["lead_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		endpoint := fmt.Sprintf("/api/v1/properties/%s/interested-leads/%s", propertyID, leadID)
		return makeGatewayRequest(ctx, gatewayURL, "DELETE", applyDryRun(endpoint, dryRun), nil)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("set_property_active_deal",
		mcp.WithDescription("Set or clear the active deal linked to a property (setar deal ativo no imóvel)."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("Property ID")),
		mcp.WithString("deal_id", mcp.Description("Deal ID. Use empty string to clear.")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "inventory", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		propertyID := args["property_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		payload := map[string]interface{}{}
		if v, ok := args["deal_id"].(string); ok {
			payload["deal_id"] = strings.TrimSpace(v)
		} else {
			payload["deal_id"] = ""
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/properties/%s/active-deal", propertyID)
		return makeGatewayRequest(ctx, gatewayURL, "PATCH", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_appointment",
		mcp.WithDescription("Schedule a new appointment, meeting, or visit in the calendar."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Title of the event")),
		mcp.WithString("date", mcp.Required(), mcp.Description("Date and time of the event in ISO format (e.g. 2026-04-10T14:00:00Z)")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Type of event (visit, meeting, call)")),
		mcp.WithString("lead_id", mcp.Description("Associated lead ID (optional)")),
		mcp.WithString("property_id", mcp.Description("Associated property ID (optional)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "calendar", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun("/api/v1/appointments", dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("update_deal_status",
		mcp.WithDescription("Move a deal to a new stage in the negotiation pipeline."),
		mcp.WithString("deal_id", mcp.Required(), mcp.Description("The ID of the deal")),
		mcp.WithString("status", mcp.Required(), mcp.Description("New status: NEGOTIATION | PROPOSAL | ANALYSIS | CONTRACT | CONVERTED | CANCELLED")),
		mcp.WithString("notes", mcp.Description("Optional notes explaining why the deal moved to this status")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dealID := args["deal_id"].(string)
		dryRun, _ := args["dry_run"].(bool)
		rawStatus := strings.TrimSpace(args["status"].(string))
		status := strings.ToUpper(rawStatus)
		switch {
		case strings.HasPrefix(status, "NEG"):
			status = "NEGOTIATION"
		case strings.HasPrefix(status, "PRO"):
			status = "PROPOSAL"
		case strings.HasPrefix(status, "ANA"):
			status = "ANALYSIS"
		case strings.HasPrefix(status, "CONTR"):
			status = "CONTRACT"
		case strings.HasPrefix(status, "CONVE") || strings.HasPrefix(status, "WON"):
			status = "CONVERTED"
		case strings.HasPrefix(status, "CANC") || strings.HasPrefix(status, "LOST"):
			status = "CANCELLED"
		}
		payload := map[string]any{"status": status}
		if v, ok := args["notes"].(string); ok && strings.TrimSpace(v) != "" {
			payload["notes"] = strings.TrimSpace(v)
		}
		body, _ := json.Marshal(payload)
		endpoint := fmt.Sprintf("/api/v1/deals/%s/status", dealID)
		return makeGatewayRequest(ctx, gatewayURL, "PATCH", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_deal_from_lead_and_property",
		mcp.WithDescription("Create a deal for a lead + property, auto-filling broker_id from /auth/me and owner_id from the property. Optionally links the lead as interested and sets the active deal on the property."),
		mcp.WithString("lead_id", mcp.Required(), mcp.Description("Lead ID (customer)")),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("Property ID")),
		mcp.WithString("type", mcp.Description("Deal type: SALE or RENT (optional; defaults from property.listing_type)")),
		mcp.WithNumber("value", mcp.Description("Deal value (optional; defaults from property pricing)")),
		mcp.WithBoolean("link_interested", mcp.Description("When true, links lead to property as interested (default true)")),
		mcp.WithBoolean("set_active_deal", mcp.Description("When true, sets the created deal as property active_deal (default true)")),
		mcp.WithString("source", mcp.Description("Source for interested link (default mcp)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, returns a preview without persisting changes.")),
	), "deals", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		leadID := strings.TrimSpace(args["lead_id"].(string))
		propertyID := strings.TrimSpace(args["property_id"].(string))
		dryRun, _ := args["dry_run"].(bool)

		status, b, err := doGatewayRequest(ctx, gatewayURL, "GET", "/api/v1/auth/me", nil)
		if err != nil {
			return mcp.NewToolResultError("failed to fetch broker identity"), nil
		}
		if status >= 400 {
			return mcp.NewToolResultError(fmt.Sprintf("API Error (%d): %s", status, string(b))), nil
		}
		var meWrap struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(b, &meWrap)
		brokerID := strings.TrimSpace(meWrap.Data.ID)
		if brokerID == "" {
			return mcp.NewToolResultError("broker id missing"), nil
		}

		status, pb, err := doGatewayRequest(ctx, gatewayURL, "GET", fmt.Sprintf("/api/v1/properties/%s", propertyID), nil)
		if err != nil {
			return mcp.NewToolResultError("failed to fetch property"), nil
		}
		if status >= 400 {
			return mcp.NewToolResultError(fmt.Sprintf("API Error (%d): %s", status, string(pb))), nil
		}
		var propWrap struct {
			Data struct {
				ListingType string `json:"listing_type"`
				Owner       struct {
					ID string `json:"id"`
				} `json:"owner"`
				Owners []struct {
					ID string `json:"id"`
				} `json:"owners"`
				Pricing struct {
					SalePrice *struct {
						CurrentAmount float64 `json:"current_amount"`
					} `json:"sale_price"`
					RentPrice *struct {
						CurrentAmount float64 `json:"current_amount"`
					} `json:"rent_price"`
				} `json:"pricing"`
			} `json:"data"`
		}
		_ = json.Unmarshal(pb, &propWrap)

		ownerID := strings.TrimSpace(propWrap.Data.Owner.ID)
		if ownerID == "" && len(propWrap.Data.Owners) > 0 {
			ownerID = strings.TrimSpace(propWrap.Data.Owners[0].ID)
		}
		if ownerID == "" {
			return mcp.NewToolResultError("owner id missing on property"), nil
		}

		dealType := ""
		if v, ok := args["type"].(string); ok {
			dealType = strings.ToUpper(strings.TrimSpace(v))
		}
		if dealType == "" {
			if strings.EqualFold(strings.TrimSpace(propWrap.Data.ListingType), "rent") {
				dealType = "RENT"
			} else {
				dealType = "SALE"
			}
		}

		value := 0.0
		if v, ok := args["value"].(float64); ok && v > 0 {
			value = v
		} else {
			if dealType == "RENT" && propWrap.Data.Pricing.RentPrice != nil {
				value = propWrap.Data.Pricing.RentPrice.CurrentAmount
			}
			if dealType == "SALE" && propWrap.Data.Pricing.SalePrice != nil {
				value = propWrap.Data.Pricing.SalePrice.CurrentAmount
			}
		}
		if value <= 0 {
			return mcp.NewToolResultError("deal value missing (provide value or set pricing on property)"), nil
		}

		linkInterested := true
		if v, ok := args["link_interested"].(bool); ok {
			linkInterested = v
		}
		setActive := true
		if v, ok := args["set_active_deal"].(bool); ok {
			setActive = v
		}
		source := "mcp"
		if v, ok := args["source"].(string); ok && strings.TrimSpace(v) != "" {
			source = strings.TrimSpace(v)
		}

		createPayload := map[string]any{
			"type":        dealType,
			"property_id": propertyID,
			"owner_id":    ownerID,
			"customer_id": leadID,
			"lead_id":     leadID,
			"broker_id":   brokerID,
			"value":       value,
		}
		createBody, _ := json.Marshal(createPayload)

		if dryRun {
			preview, _ := json.MarshalIndent(map[string]any{
				"create_deal":     createPayload,
				"link_interested": linkInterested,
				"set_active_deal": setActive,
				"source":          source,
			}, "", "  ")
			return mcp.NewToolResultText(string(preview)), nil
		}

		status, created, err := doGatewayRequest(ctx, gatewayURL, "POST", "/api/v1/deals", createBody)
		if err != nil {
			return mcp.NewToolResultError("failed to create deal"), nil
		}
		if status >= 400 {
			return mcp.NewToolResultError(fmt.Sprintf("API Error (%d): %s", status, string(created))), nil
		}

		var dealWrap struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(created, &dealWrap)
		dealID := strings.TrimSpace(dealWrap.Data.ID)
		if dealID == "" {
			return mcp.NewToolResultError("created deal id missing"), nil
		}

		ops := []map[string]any{
			{"op": "create_deal", "deal_id": dealID},
		}

		if linkInterested {
			body, _ := json.Marshal(map[string]any{"lead_id": leadID, "source": source})
			_, _, _ = doGatewayRequest(ctx, gatewayURL, "POST", fmt.Sprintf("/api/v1/properties/%s/interested-leads", propertyID), body)
			ops = append(ops, map[string]any{"op": "link_interested", "property_id": propertyID, "lead_id": leadID})
		}
		if setActive {
			body, _ := json.Marshal(map[string]any{"deal_id": dealID})
			_, _, _ = doGatewayRequest(ctx, gatewayURL, "PATCH", fmt.Sprintf("/api/v1/properties/%s/active-deal", propertyID), body)
			ops = append(ops, map[string]any{"op": "set_active_deal", "property_id": propertyID, "deal_id": dealID})
		}

		out, _ := json.MarshalIndent(map[string]any{
			"deal_id": dealID,
			"ops":     ops,
		}, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})

	// ==========================================
	// WRITE TOOLS (Agenda e Vistorias)
	// ==========================================

	s.AddTool(toolWithMeta(mcp.NewTool("update_appointment",
		mcp.WithDescription("Update an existing appointment/event in the calendar (e.g. reschedule, change status to Cancelled or Confirmed)."),
		mcp.WithString("appointment_id", mcp.Required(), mcp.Description("The ID of the appointment to update")),
		mcp.WithString("title", mcp.Description("New title of the event")),
		mcp.WithString("date", mcp.Description("New date of the event (YYYY-MM-DD)")),
		mcp.WithString("time", mcp.Description("New start time (HH:MM)")),
		mcp.WithString("status", mcp.Description("New status (e.g. Agendado, Confirmado, Cancelado, Concluido)")),
		mcp.WithString("description", mcp.Description("New description or notes for the event")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "calendar", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		appointmentID := args["appointment_id"].(string)
		dryRun, _ := args["dry_run"].(bool)

		// Remove the ID from the payload so we only send the fields to update
		delete(args, "appointment_id")
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		endpoint := fmt.Sprintf("/api/v1/appointments/%s", appointmentID)
		return makeGatewayRequest(ctx, gatewayURL, "PUT", applyDryRun(endpoint, dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("create_inspection",
		mcp.WithDescription("Schedule a new property inspection (vistoria) in the operations module."),
		mcp.WithString("property_id", mcp.Required(), mcp.Description("The ID of the property to be inspected")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Type of inspection (e.g. Entrada, Saida, Manutencao)")),
		mcp.WithString("scheduled_date", mcp.Required(), mcp.Description("Scheduled date and time in ISO format (RFC3339)")),
		mcp.WithString("inspector_id", mcp.Required(), mcp.Description("The ID of the user/broker conducting the inspection")),
		mcp.WithString("inspector_name", mcp.Required(), mcp.Description("The name of the inspector")),
		mcp.WithString("contract_id", mcp.Description("Associated contract ID (optional)")),
		mcp.WithString("notes", mcp.Description("Initial notes or instructions for the inspection (optional)")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "operations", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		dryRun, _ := args["dry_run"].(bool)
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		return makeGatewayRequest(ctx, gatewayURL, "POST", applyDryRun("/api/v1/inspections", dryRun), body)
	})

	s.AddTool(toolWithMeta(mcp.NewTool("update_inspection",
		mcp.WithDescription("Update an existing property inspection (vistoria), like changing its status, date, or adding findings."),
		mcp.WithString("inspection_id", mcp.Required(), mcp.Description("The ID of the inspection to update")),
		mcp.WithString("status", mcp.Description("New status (e.g. Agendada, Em Andamento, Concluida, Cancelada)")),
		mcp.WithString("scheduled_date", mcp.Description("Rescheduled date and time (ISO format)")),
		mcp.WithString("completed_date", mcp.Description("Date and time when the inspection was completed (ISO format)")),
		mcp.WithString("notes", mcp.Description("Updated notes or final remarks")),
		mcp.WithBoolean("dry_run", mcp.Description("When true, validates the action and returns a preview without persisting changes.")),
	), "operations", "broker", false, true), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireMinRole(ctx, "broker"); err != nil {
			return mcp.NewToolResultError("forbidden"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		inspectionID := args["inspection_id"].(string)
		dryRun, _ := args["dry_run"].(bool)

		delete(args, "inspection_id")
		delete(args, "dry_run")
		body, _ := json.Marshal(args)
		endpoint := fmt.Sprintf("/api/v1/inspections/%s", inspectionID)
		return makeGatewayRequest(ctx, gatewayURL, "PUT", applyDryRun(endpoint, dryRun), body)
	})

	// ==========================================
	// RESOURCES
	// ==========================================

	s.AddResource(
		mcp.NewResource("toca://crm/dashboard", "CRM Dashboard Summary", mcp.WithMIMEType("application/json"), mcp.WithResourceDescription("High-level dashboard metrics for the active tenant")),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", gatewayURL+"/api/v1/dashboard", nil)
			if err != nil {
				return nil, err
			}

			if tenantID, ok := ctx.Value(tenantIDKey).(string); ok && tenantID != "" {
				req.Header.Set("X-Tenant-Id", tenantID)
			}
			if auth, ok := ctx.Value(authKey).(string); ok && auth != "" {
				req.Header.Set("Authorization", auth)
			}

			resp, err := gatewayHTTPClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/json",
					Text:     string(body),
				},
			}, nil
		},
	)

	// Instrumenta TODAS as ferramentas registradas de uma vez.
	//
	// Feito aqui, e não em cada um dos 56 registros, para não poluir o código e
	// para que ferramentas adicionadas depois também sejam cobertas. Quando o
	// cliente reporta "timeout waiting for SSE response", há duas causas
	// possíveis e indistinguíveis sem log: a ferramenta não rodou, ou rodou e a
	// Central de Ajuda (docs/help-center) como tools locais para a Suitee IA.
	// Registra ANTES da instrumentação para seguirem o mesmo caminho das demais.
	if corpus, err := helpcorpus.Load(helpCorpusDir()); err != nil {
		logger.Error("Central de Ajuda indisponível — search_help/get_help_topic vão falhar", "error", err)
	} else {
		registerHelpTools(s, corpus)
		logger.Info("Central de Ajuda carregada", "topicos", len(corpus.Topics))
	}

	// resposta não voltou pelo stream SSE.
	ferramentasRegistradas := s.ListTools()
	logger.Info("instrumentando ferramentas", "total", len(ferramentasRegistradas))
	for nome, st := range ferramentasRegistradas {
		handlerOriginal := st.Handler
		nome := nome
		s.AddTools(server.ServerTool{
			Tool:    st.Tool,
			Handler: instrumentarFerramenta(nome, handlerOriginal),
		})
	}

	// Set up Gin Router
	router := newRouter(newMCPSSEServer(s))

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		logger.Info("Server started", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down toca-mcp server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("toca-mcp server exiting")
}

// jwksURLFromEnv resolve a URL do JWKS com a mesma precedência do pkg/config:
// SUPABASE_JWKS_URL explícito e, na ausência, SUPABASE_URL + o caminho padrão do
// Supabase. Sem nenhum dos dois o middleware nega tudo (falha fechada).
func jwksURLFromEnv() string {
	if url := strings.TrimSpace(os.Getenv("SUPABASE_JWKS_URL")); url != "" {
		return url
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/")
	if base == "" {
		return ""
	}
	return base + "/auth/v1/.well-known/jwks.json"
}

// helpCorpusDir devolve o diretório do corpus da Central de Ajuda
// (docs/help-center). Em produção o Dockerfile copia o corpus para
// /help-corpus; HELP_CORPUS_DIR permite override local.
func helpCorpusDir() string {
	if dir := strings.TrimSpace(os.Getenv("HELP_CORPUS_DIR")); dir != "" {
		return dir
	}
	return "/help-corpus"
}

// registerHelpTools expõe a Central de Ajuda como tools MCP locais (sem
// gateway): search_help ranqueia os tópicos e get_help_topic devolve o
// markdown completo. A IA usa essas tools para responder dúvidas de uso com o
// MESMO conteúdo da página /ajuda do aplicativo.
func registerHelpTools(s *server.MCPServer, corpus *helpcorpus.Corpus) {
	s.AddTool(toolWithMeta(mcp.NewTool("search_help",
		mcp.WithDescription("Search the Suitee help center (usage documentation) for topics about how to use the platform. Use it when the user asks how to do something in the app (e.g. how to create a lead, register a payment, generate DIMOB, configure the lead roulette). Returns matching topics with id, title and summary."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search terms in Portuguese (e.g. 'como registrar recebimento', 'gerar DIMOB', 'roleta de leads')")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of topics to return (default 5)")),
	), "help", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if corpus == nil {
			return mcp.NewToolResultError("central de ajuda indisponível neste ambiente"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		query, _ := args["query"].(string)
		limit, _ := args["limit"].(float64)
		if limit <= 0 {
			limit = 5
		}
		if limit > 10 {
			limit = 10
		}
		hits := corpus.Search(query, int(limit))
		if len(hits) == 0 {
			return mcp.NewToolResultError("nenhum tópico da central de ajuda encontrado para: " + query), nil
		}
		var sb strings.Builder
		sb.WriteString("Tópicos encontrados na Central de Ajuda:\n")
		for _, h := range hits {
			sb.WriteString(fmt.Sprintf("\n- id: %s\n  título: %s\n  resumo: %s\n", h.Topic.ID, h.Topic.Title, h.Topic.Summary))
		}
		sb.WriteString("\nPara responder, chame get_help_topic com o id do tópico mais relevante.")
		return mcp.NewToolResultText(sb.String()), nil
	})

	s.AddTool(toolWithMeta(mcp.NewTool("get_help_topic",
		mcp.WithDescription("Return the full help center topic (markdown) by topic id. Use after search_help to get the complete step-by-step instructions for the user's question."),
		mcp.WithString("topic_id", mcp.Required(), mcp.Description("Topic id returned by search_help (e.g. 'leads', 'financeiro', 'fiscal')")),
	), "help", "user", true, false), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if corpus == nil {
			return mcp.NewToolResultError("central de ajuda indisponível neste ambiente"), nil
		}
		args := request.Params.Arguments.(map[string]interface{})
		id, _ := args["topic_id"].(string)
		topic, ok := corpus.Topic(strings.TrimSpace(id))
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("tópico desconhecido: %q — use search_help para localizar o id", id)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("# %s\n\n%s", topic.Title, topic.Body)), nil
	})
}

// mcpServerHooks devolve os hooks de produção do servidor MCP. O único hook é a
// listagem: ela é filtrada pelo rank VERIFICADO da sessão (achado C7) — antes o
// filtro usava o header Authorization decodificado sem verificação.
func mcpServerHooks() *server.Hooks {
	return &server.Hooks{
		OnAfterListTools: []server.OnAfterListToolsFunc{
			func(ctx context.Context, _ any, _ *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
				filtrarToolsPorRank(ctx, result)
			},
		},
	}
}

// newMCPSSEServer monta o servidor SSE do SDK (extraído para os testes subirem o
// mesmo caminho de produção).
func newMCPSSEServer(s *server.MCPServer) *server.SSEServer {
	// O endpoint de mensagens é /mcp/messages (o default do SDK é /message, que
	// não existe neste router). O contexto da sessão NÃO vem mais de
	// WithSSEContextFunc: quem o monta é o middleware autenticado do newRouter, a
	// partir das claims verificadas (P0/C7).
	return server.NewSSEServer(s, server.WithMessageEndpoint("/mcp/messages"))
}

// newRouter monta o router HTTP do serviço, com a autenticação das rotas MCP.
//
// P0/C7: a superfície MCP não tinha autenticação nenhuma — o tenant vinha do
// header X-Tenant-Id e o papel, de um JWT decodificado sem verificação de
// assinatura. As rotas /mcp/* passam a viver num grupo com
// pkgAuth.Middleware (mesmo JWKS dos outros 12 serviços) + requireVerifiedTenant.
// Os probes de infraestrutura (/health, /live, /ready, /metrics) seguem públicos.
func newRouter(sseServer *server.SSEServer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(tracing.Middleware("toca-mcp"))
	router.Use(metrics.GinMiddleware("toca-mcp"))
	router.GET("/metrics", metrics.Handler())
	health.RegisterGin(router, "toca-mcp", nil)

	appEnv := os.Getenv("APP_ENV")
	appMode := os.Getenv("APP_MODE")
	// P2/M16: critério único de release (pkg/config), sem cópias divergentes.
	isRelease := config.IsReleaseEnv(appEnv, appMode)
	allowedOrigins := os.Getenv("MCP_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("CORS_ALLOWED_ORIGINS")
	}
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	}

	parsedOrigins := make([]string, 0)
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			parsedOrigins = append(parsedOrigins, o)
		}
	}
	if isRelease {
		if len(parsedOrigins) == 0 {
			logger.Error("CORS origins must be configured via MCP_ALLOWED_ORIGINS/CORS_ALLOWED_ORIGINS/ALLOWED_ORIGINS in release mode")
			os.Exit(1)
		}
		for _, o := range parsedOrigins {
			if o == "*" {
				logger.Error("CORS wildcard origin is not allowed in release mode")
				os.Exit(1)
			}
		}
	}

	corsConfig := cors.DefaultConfig()
	if len(parsedOrigins) > 0 {
		corsConfig.AllowOrigins = parsedOrigins
	} else {
		corsConfig.AllowAllOrigins = true
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Tenant-Id"}
	router.Use(cors.New(corsConfig))

	router.Use(func(c *gin.Context) {
		tenantId := c.GetHeader("X-Tenant-Id")
		if tenantId != "" {
			logger.Info("Incoming MCP request", "tenantId", tenantId, "path", c.Request.URL.Path)
		}
		c.Next()
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-mcp"})
	})

	// P0/C7 — grupo autenticado das rotas MCP.
	//
	//   * pkgAuth.Middleware valida assinatura (RS256/ES256), `exp` e `kid`
	//     contra o JWKS do Supabase — o mesmo middleware dos outros 12 serviços,
	//     nada de verificação caseira;
	//   * requireVerifiedTenant devolve 401 quando o token é válido mas não
	//     carrega tenant (sem tenant não há escopo para tool nenhuma).
	//
	// Sem JWKS configurado o middleware nega tudo (`auth jwks not initialized`):
	// falha fechada, sem derrubar /health, /live, /ready e /metrics.
	mcpGroup := router.Group("/mcp")
	mcpGroup.Use(pkgAuth.Middleware(pkgAuth.Config{SupabaseJWKSURL: jwksURLFromEnv()}))
	mcpGroup.Use(func(c *gin.Context) {
		// O contexto da sessão MCP é derivado das claims VERIFICADAS: as tools
		// recebem tenant/papel/rank daqui, e não de headers.
		ctx := verifiedSessionContext(c.Request.Context(), c.Request)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	mcpGroup.Use(requireVerifiedTenant())

	// Instrumentação do ciclo de sessão SSE.
	//
	// O SDK remove a sessão do mapa quando o handler do stream SSE retorna, e
	// qualquer POST posterior passa a receber 400 "Invalid session ID". Sem
	// registrar abertura e fechamento do stream, uma sessão que morre cedo é
	// indistinguível de um cliente que usa a sessão errada.
	sseHandler := sseServer.SSEHandler()
	mcpGroup.GET("/sse", func(c *gin.Context) {
		sessao := c.Query("sessionId")
		inicio := time.Now()
		logger.Info("sse stream abrindo", "session_id", sessao)
		defer func() {
			logger.Info("sse stream fechou",
				"session_id", sessao,
				"ms", time.Since(inicio).Milliseconds(),
			)
		}()
		sseHandler.ServeHTTP(c.Writer, c.Request)
	})

	mensagensHandler := sseServer.MessageHandler()
	mcpGroup.POST("/messages", func(c *gin.Context) {
		// Lê o corpo para registrar o método JSON-RPC e o nome da ferramenta, e
		// devolve o corpo ao request para o handler do SDK não ver um stream
		// vazio. É o que responde a pergunta que faltava: o cliente chega a
		// enviar `tools/call`, e com que nome?
		corpo, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(corpo))

		var envelope struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.Unmarshal(corpo, &envelope)

		logger.Info("post de mensagem",
			"session_id", c.Query("sessionId"),
			"metodo", envelope.Method,
			"tool", envelope.Params.Name,
			"tem_id", envelope.ID != nil,
			"bytes", len(corpo),
		)
		mensagensHandler.ServeHTTP(c.Writer, c.Request)
	})

	return router
}

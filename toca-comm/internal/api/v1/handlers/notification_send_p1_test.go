package handlers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// P1 / A3 — `POST /notifications/send` era um primitivo de envio arbitrário de
// e-mail/WhatsApp/SMS alcançável pela borda.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) registrou que o gateway proxia `/api/v1/notifications/*`
// (`gateway/internal/handlers/comm.go:19-20`) e que a única credencial da rota
// era a `INTERNAL_API_KEY` estática, compartilhada por ~10 serviços. Como o
// caminho não tem o segmento `internal`, a denylist da borda (P0/C10) não o
// cobria: quem tivesse a chave disparava mensagens em nome da plataforma pela
// internet, escolhendo destinatário, assunto e corpo.
//
// A correção moveu a rota para `/api/v1/comm/internal/notifications/send` —
// coberta pela denylist — e o tenant passou a ser resolvido pela credencial.
// Os testes abaixo travam as duas pontas: o caminho público não existe mais e o
// caminho interno não aceita tenant escolhido pelo chamador.

const (
	sendPath       = "/api/v1/comm/internal/notifications/send"
	legacySendPath = "/api/v1/notifications/send"
)

// O caminho antigo não é mais servido: mesmo com a chave interna válida, o
// primitivo não está no namespace público. Sem essa garantia, o gateway (que
// proxia `/api/v1/notifications/*`) voltaria a expô-lo.
func TestSendNotification_LegacyPublicPathIsNotServed(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	key := withNamedServiceKey(t, "toca-financial", "key-fin", "tenant-a")

	w := doCommRequest(t, r, http.MethodPost, legacySendPath,
		`{"type":"EMAIL","recipient":"vitima@example.com","subject":"oi","content":"click"}`,
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusNotFound {
		t.Fatalf("VAZAMENTO: %s ainda é servido pela borda (%d): %s — mova a rota para %s",
			legacySendPath, w.Code, w.Body.String(), sendPath)
	}
}

// A rota interna existe (não é 404) e exige identidade: sem chave e sem JWT a
// requisição não chega ao serviço de notificação.
func TestSendNotification_InternalPathRequiresIdentity(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)

	w := doCommRequest(t, r, http.MethodPost, sendPath,
		`{"type":"EMAIL","recipient":"vitima@example.com","subject":"oi","content":"click"}`, nil)

	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("rota interna respondeu %d sem identidade de serviço/usuário: %s", w.Code, w.Body.String())
	}
}

// Credencial de serviço com allowlist de tenant-a pedindo tenant-vitima: 403.
// É o cenário do A3 — antes, a chave compartilhada aceitava qualquer tenant.
func TestSendNotification_ServiceCredentialCannotPickForeignTenant(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	key := withNamedServiceKey(t, "toca-financial", "key-fin", "tenant-a")

	w := doCommRequest(t, r, http.MethodPost, sendPath,
		`{"type":"EMAIL","recipient":"vitima@example.com","subject":"oi","content":"click","tenant_id":"tenant-vitima"}`,
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a enviou mensagem em tenant-vitima (%d): %s",
			w.Code, w.Body.String())
	}
}

// Envio de SISTEMA: sem tenant no corpo e com credencial de serviço, a mensagem
// é aceita (o e-mail de recuperação de senha é pedido por quem ainda não tem
// tenant). Aqui o serviço de e-mail não está configurado — o 500 "not
// configured" é justamente a prova de que a requisição PASSOU pelo gate de
// tenant e chegou ao NotificationService; o que não pode acontecer é o 400 de
// "tenant_id is required".
func TestSendNotification_SystemSendWithoutTenantIsNotBlocked(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	key := withNamedServiceKey(t, "toca-auth", "key-auth")

	w := doCommRequest(t, r, http.MethodPost, sendPath,
		`{"type":"EMAIL","recipient":"vitima@example.com","subject":"Recuperação","content":"link"}`,
		map[string]string{"X-Internal-Key": key})

	if w.Code == http.StatusBadRequest {
		t.Fatalf("envio de sistema foi recusado por falta de tenant (%d): %s — isto quebra a recuperação de senha",
			w.Code, w.Body.String())
	}
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("credencial de serviço válida recusada (%d): %s", w.Code, w.Body.String())
	}
	// A requisição passou pelo guarda de tenant e chegou ao NotificationService —
	// que neste harness não tem sender configurado e por isso responde 500.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("envio de sistema deveria chegar ao serviço (%d): %s", w.Code, w.Body.String())
	}
	// M17: o detalhe do serviço ("not configured") NÃO pode voltar ao cliente.
	if strings.Contains(w.Body.String(), "not configured") {
		t.Fatalf("resposta vazou o detalhe interno do serviço: %s", w.Body.String())
	}
}

// JWT de usuário do tenant-b pedindo tenant-vitima no corpo: 403 — com JWT, o
// tenant do token manda (mesma regra das demais rotas internas).
func TestSendNotification_UserJWTCannotPickAnotherTenant(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	jwks := newCommTestJWKS(t)
	r := commRouter(t, jwks.server.URL, msgRepo, inAppRepo)
	token := jwks.token(t, "tenant-b")

	w := doCommRequest(t, r, http.MethodPost, sendPath,
		`{"type":"EMAIL","recipient":"vitima@example.com","subject":"oi","content":"click","tenant_id":"tenant-vitima"}`,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: JWT de tenant-b enviou mensagem em tenant-vitima (%d): %s",
			w.Code, w.Body.String())
	}
}

// ───────────── contrato pkg/clients ↔ rotas do toca-comm ─────────────
//
// Todo caminho que o CommClient chama precisa existir no router. O teste lê a
// fonte do cliente (não o binário) porque a divergência que interessa é de
// contrato: renomear a rota aqui sem renomear o cliente lá compila nos dois
// lados e só quebra em runtime, no caminho de cobrança/assinatura.
//
// Mesma varredura garante que o cliente NÃO usa mais caminho público para as
// rotas internas — se alguém reintroduzir `/api/v1/notifications/send`, este
// teste falha antes do deploy.
func TestCommClientPathsAreRegisteredInternalRoutes(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "pkg", "clients", "comm_client.go"))
	if err != nil {
		t.Fatalf("ler comm_client.go: %v", err)
	}

	literal := regexp.MustCompile(`"(/api/v1/[^"]*)"`)
	type call struct{ method, pattern string }
	var calls []call
	for _, m := range literal.FindAllStringSubmatch(string(src), -1) {
		path := m[1]
		// `?participant=` e `%s` são preenchidos pelo cliente: aqui vira o
		// parâmetro do gin correspondente.
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}
		path = regexp.MustCompile(`%s`).ReplaceAllString(path, ":param")
		calls = append(calls, call{pattern: path})
	}
	if len(calls) == 0 {
		t.Fatal("nenhum caminho encontrado em pkg/clients/comm_client.go — o teste perdeu a referência e não protege mais nada")
	}

	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	registered := map[string]bool{}
	for _, rt := range r.Routes() {
		registered[rt.Path] = true
	}

	for _, c := range calls {
		if !strings.Contains(c.pattern, "/internal") {
			t.Errorf("CommClient usa caminho PÚBLICO %q: as rotas internas do toca-comm ficam sob /api/v1/comm/internal (P1/A3)", c.pattern)
			continue
		}
		want := regexp.MustCompile(`:[a-zA-Z_]+`).ReplaceAllString(c.pattern, ":param")
		found := false
		for path := range registered {
			if regexp.MustCompile(`:[a-zA-Z_]+`).ReplaceAllString(path, ":param") == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CommClient chama %q, que não existe no router do toca-comm", c.pattern)
		}
	}
}

// repoRoot sobe até o diretório que contém go.work (raiz do monorepo). O teste
// roda dentro de toca-comm/internal/api/v1/handlers.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 12; i++ {
		// go.work é gerado localmente e não é versionado; o checkout público
		// tem .git como marcador estável da raiz do monorepo.
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.work não encontrado a partir de %s", dir)
	return ""
}

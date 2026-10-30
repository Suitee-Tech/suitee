package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// P0 / C5 — webhook público de leads do portal.
//
// Antes da correção: `if signature != "" && !ValidateHMAC(...)` deixava uma
// requisição SEM assinatura passar direto, sem checar status e sem limite.
// Estes testes usam os repos falsos (nenhum Postgres, nenhuma rede) e provam
// pelo lado do HTTP o que o auditor verificou em runtime.

const p0WebhookBody = `{"name":"Invasor","email":"invasor@example.com","phone":"11999999999","message":"lead forjado"}`

func p0Signature(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// newP0WebhookEngine monta só a rota pública (sem JWT), igual ao router real.
func newP0WebhookEngine(t *testing.T, integRepo *fakeIntegrationRepo, leadRepo *fakeLeadRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	p0InitValidation(t)
	svc := service.NewWebhookService(leadRepo, integRepo, nil, "", "test-internal-key")
	h := NewWebhookHandler(svc)
	r := gin.New()
	r.POST("/api/v1/portals/webhook/:integrationID", h.ReceiveLead)
	return r
}

func p0PostWebhook(t *testing.T, engine *gin.Engine, integrationID, body, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portals/webhook/"+integrationID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Hub-Signature", signature)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func p0ActiveIntegration(id, tenantID, secret string) *integration.PortalIntegration {
	return &integration.PortalIntegration{
		ID:            id,
		TenantID:      tenantID,
		Portal:        integration.PortalZAP,
		Status:        integration.IntegrationActive,
		WebhookSecret: secret,
		ContactName:   "Titular",
	}
}

// TestWebhookP0_MissingSignatureIsRejected: requisição anônima sem o header
// X-Hub-Signature deve receber 401 e NADA pode ser gravado em portal.leads.
func TestWebhookP0_MissingSignatureIsRejected(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integRepo.seed(p0ActiveIntegration("integ-vitima", "tenant-vitima", "segredo-real"))

	engine := newP0WebhookEngine(t, integRepo, leadRepo)
	w := p0PostWebhook(t, engine, "integ-vitima", p0WebhookBody, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("SEM ASSINATURA: esperava 401, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	if n := leadRepo.count(); n != 0 {
		t.Fatalf("SEM ASSINATURA: %d lead(s) foram persistidos em portal.leads: %+v", n, leadRepo.all())
	}
}

// TestWebhookP0_InvalidSignatureIsRejected: assinatura presente e errada → 401.
func TestWebhookP0_InvalidSignatureIsRejected(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integRepo.seed(p0ActiveIntegration("integ-vitima", "tenant-vitima", "segredo-real"))

	engine := newP0WebhookEngine(t, integRepo, leadRepo)
	w := p0PostWebhook(t, engine, "integ-vitima", p0WebhookBody, p0Signature(p0WebhookBody, "segredo-errado"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ASSINATURA INVÁLIDA: esperava 401, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	if n := leadRepo.count(); n != 0 {
		t.Fatalf("ASSINATURA INVÁLIDA: %d lead(s) persistidos", n)
	}
}

// TestWebhookP0_EmptySecretFailsClosed: integração sem webhook_secret não pode
// aceitar nada — nem requisição sem assinatura, nem assinatura calculada com
// chave vazia (que qualquer atacante consegue produzir).
func TestWebhookP0_EmptySecretFailsClosed(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integRepo.seed(p0ActiveIntegration("integ-sem-secret", "tenant-vitima", ""))

	engine := newP0WebhookEngine(t, integRepo, leadRepo)

	// assinatura vazia (sem header) e assinatura computada com chave vazia
	for _, sig := range []string{"", p0Signature(p0WebhookBody, "")} {
		w := p0PostWebhook(t, engine, "integ-sem-secret", p0WebhookBody, sig)
		if w.Code < 400 {
			t.Fatalf("SEGREDO VAZIO (sig=%q): esperava erro, veio %d (corpo=%s)", sig, w.Code, w.Body.String())
		}
	}
	if n := leadRepo.count(); n != 0 {
		t.Fatalf("SEGREDO VAZIO: %d lead(s) persistidos", n)
	}
}

// TestWebhookP0_InactiveIntegrationIsRejected: integração desconectada não
// ingere lead, mesmo com assinatura válida.
func TestWebhookP0_InactiveIntegrationIsRejected(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integ := p0ActiveIntegration("integ-desconectada", "tenant-vitima", "segredo-real")
	integ.Status = integration.IntegrationDisconnected
	integRepo.seed(integ)

	engine := newP0WebhookEngine(t, integRepo, leadRepo)
	w := p0PostWebhook(t, engine, "integ-desconectada", p0WebhookBody, p0Signature(p0WebhookBody, "segredo-real"))

	if w.Code < 400 {
		t.Fatalf("INTEGRAÇÃO INATIVA: esperava erro, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	if n := leadRepo.count(); n != 0 {
		t.Fatalf("INTEGRAÇÃO INATIVA: %d lead(s) persistidos", n)
	}
}

// TestWebhookP0_RateLimitedPerIntegration: o limite por integração precisa
// cortar rajadas antes de gravar o lead.
func TestWebhookP0_RateLimitedPerIntegration(t *testing.T) {
	t.Setenv("PORTAL_WEBHOOK_RATE_LIMIT_PER_MIN", "2")

	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integRepo.seed(p0ActiveIntegration("integ-limitada", "tenant-vitima", "segredo-real"))

	engine := newP0WebhookEngine(t, integRepo, leadRepo)
	sig := p0Signature(p0WebhookBody, "segredo-real")

	var got []int
	for i := 0; i < 3; i++ {
		w := p0PostWebhook(t, engine, "integ-limitada", p0WebhookBody, sig)
		got = append(got, w.Code)
	}

	if got[0] != http.StatusCreated || got[1] != http.StatusCreated {
		t.Fatalf("limite: as duas primeiras requisições deveriam ser aceitas, veio %v", got)
	}
	if got[2] != http.StatusTooManyRequests {
		t.Fatalf("limite por integração não aplicado: terceira requisição veio %d (esperado 429); códigos=%v", got[2], got)
	}
}

// TestWebhookP0_ValidSignatureStillAccepted: sanidade — assinatura correta
// continua ingerindo o lead (o harden não pode quebrar o portal de verdade).
func TestWebhookP0_ValidSignatureStillAccepted(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	leadRepo := newFakeLeadRepo()
	integRepo.seed(p0ActiveIntegration("integ-ok", "tenant-vitima", "segredo-real"))

	engine := newP0WebhookEngine(t, integRepo, leadRepo)
	w := p0PostWebhook(t, engine, "integ-ok", p0WebhookBody, p0Signature(p0WebhookBody, "segredo-real"))

	if w.Code != http.StatusCreated {
		t.Fatalf("ASSINATURA VÁLIDA: esperava 201, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	if n := leadRepo.count(); n != 1 {
		t.Fatalf("ASSINATURA VÁLIDA: esperava 1 lead persistido, veio %d", n)
	}
}

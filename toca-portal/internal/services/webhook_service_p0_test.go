package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/domain/lead"
)

func p0HMAC(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// P0 / C5 — contrato de erros do ProcessWebhook.
//
// Os testes de HTTP (internal/handlers/webhook_p0_test.go) provam status e
// persistência; aqui a prova é o contrato fino: qual sentinela sai em cada
// situação (é ele que o handler traduz para 401/403/404/429/500), que a
// checagem de assinatura acontece ANTES de qualquer consulta e que erro de
// driver não vira "não encontrado".

type p0IntegRepo struct {
	integration.Repository // métodos não exercitados aqui (embed nil = panic se chamados)
	integ                  *integration.PortalIntegration
	loadErr                error
	lookups                int
}

func (r *p0IntegRepo) GetByIDAnyTenant(_ context.Context, id string) (*integration.PortalIntegration, error) {
	r.lookups++
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	if r.integ == nil || r.integ.ID != id {
		return nil, integration.ErrNotFound
	}
	return r.integ, nil
}

type p0LeadRepo struct {
	lead.Repository // embed nil
	created         []*lead.PortalLead
	dupErr          error
}

func (r *p0LeadRepo) Create(_ context.Context, pl *lead.PortalLead) error {
	r.created = append(r.created, pl)
	return nil
}

func (r *p0LeadRepo) Update(_ context.Context, _ *lead.PortalLead) error { return nil }

func (r *p0LeadRepo) FindDuplicate(_ context.Context, _, _, _, _, _ string) (*lead.PortalLead, error) {
	return nil, r.dupErr
}

func p0WebhookService(t *testing.T, integ *integration.PortalIntegration, loadErr error) (*WebhookService, *p0LeadRepo, *p0IntegRepo) {
	t.Helper()
	ir := &p0IntegRepo{integ: integ, loadErr: loadErr}
	lr := &p0LeadRepo{}
	return NewWebhookService(lr, ir, nil, "", "test-internal-key"), lr, ir
}

func p0Body() []byte {
	return []byte(`{"name":"Invasor","email":"invasor@example.com","phone":"11999999999"}`)
}

func TestProcessWebhookP0_MissingSignatureFailsBeforeLookup(t *testing.T) {
	svc, leads, irepo := p0WebhookService(t, &integration.PortalIntegration{
		ID: "integ-1", TenantID: "tenant-1", Status: integration.IntegrationActive, WebhookSecret: "segredo",
	}, nil)

	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), "")
	if !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("assinatura ausente: esperava ErrSignatureRequired, veio %v", err)
	}
	if irepo.lookups != 0 {
		t.Fatalf("assinatura ausente não deveria nem consultar o banco (lookups=%d)", irepo.lookups)
	}
	if len(leads.created) != 0 {
		t.Fatalf("assinatura ausente persistiu %d lead(s)", len(leads.created))
	}
}

func TestProcessWebhookP0_InvalidSignatureIsRejected(t *testing.T) {
	svc, leads, _ := p0WebhookService(t, &integration.PortalIntegration{
		ID: "integ-1", TenantID: "tenant-1", Status: integration.IntegrationActive, WebhookSecret: "segredo",
	}, nil)

	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), "sha256=deadbeef")
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("assinatura inválida: esperava ErrSignatureInvalid, veio %v", err)
	}
	if len(leads.created) != 0 {
		t.Fatalf("assinatura inválida persistiu %d lead(s)", len(leads.created))
	}
}

func TestProcessWebhookP0_EmptySecretFailsClosed(t *testing.T) {
	svc, leads, _ := p0WebhookService(t, &integration.PortalIntegration{
		ID: "integ-1", TenantID: "tenant-1", Status: integration.IntegrationActive, WebhookSecret: "",
	}, nil)

	// Assinatura calculada com chave vazia: qualquer atacante consegue produzir.
	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), "sha256="+p0HMAC(p0Body(), ""))
	if !errors.Is(err, ErrWebhookSecretMissing) {
		t.Fatalf("segredo vazio: esperava ErrWebhookSecretMissing, veio %v", err)
	}
	if len(leads.created) != 0 {
		t.Fatalf("segredo vazio persistiu %d lead(s)", len(leads.created))
	}
}

func TestProcessWebhookP0_InactiveIntegrationIsRejected(t *testing.T) {
	svc, leads, _ := p0WebhookService(t, &integration.PortalIntegration{
		ID: "integ-1", TenantID: "tenant-1", Status: integration.IntegrationDisconnected, WebhookSecret: "segredo",
	}, nil)

	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), "sha256="+p0HMAC(p0Body(), "segredo"))
	if !errors.Is(err, ErrIntegrationInactive) {
		t.Fatalf("integração inativa: esperava ErrIntegrationInactive, veio %v", err)
	}
	if len(leads.created) != 0 {
		t.Fatalf("integração inativa persistiu %d lead(s)", len(leads.created))
	}
}

func TestProcessWebhookP0_UnknownIntegrationIsNotFound(t *testing.T) {
	svc, _, _ := p0WebhookService(t, nil, nil)

	_, err := svc.ProcessWebhook(context.Background(), "integ-inexistente", p0Body(), "sha256=deadbeef")
	if !errors.Is(err, integration.ErrNotFound) {
		t.Fatalf("integração inexistente: esperava integration.ErrNotFound (404), veio %v", err)
	}
}

func TestProcessWebhookP0_DriverErrorIsNotNotFound(t *testing.T) {
	svc, _, _ := p0WebhookService(t, nil, errors.New("load integration: sql: connection refused"))

	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), "sha256=deadbeef")
	if err == nil {
		t.Fatal("erro de banco deveria propagar")
	}
	if errors.Is(err, integration.ErrNotFound) {
		t.Fatal("erro de driver não pode virar 'não encontrado' (404) — tem de virar 500 genérico")
	}
}

func TestProcessWebhookP0_RateLimitIsPerIntegration(t *testing.T) {
	t.Setenv("PORTAL_WEBHOOK_RATE_LIMIT_PER_MIN", "2")
	limiter := newWebhookRateLimiter()

	if !limiter.allow("integ-A") || !limiter.allow("integ-A") {
		t.Fatal("as duas primeiras requisições da integração A deveriam passar")
	}
	if limiter.allow("integ-A") {
		t.Fatal("terceira requisição da integração A deveria ser limitada")
	}
	if !limiter.allow("integ-B") {
		t.Fatal("o bucket é por integração: B não pode ser afetado pelo consumo de A")
	}
}

func TestProcessWebhookP0_RateLimitedSentinel(t *testing.T) {
	t.Setenv("PORTAL_WEBHOOK_RATE_LIMIT_PER_MIN", "1")
	svc, _, _ := p0WebhookService(t, &integration.PortalIntegration{
		ID: "integ-1", TenantID: "tenant-1", Status: integration.IntegrationActive, WebhookSecret: "segredo",
	}, nil)
	sig := "sha256=" + p0HMAC(p0Body(), "segredo")

	if _, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), sig); err != nil {
		t.Fatalf("primeira requisição deveria ser aceita (o CRM falha depois, mas o erro é nil): %v", err)
	}
	_, err := svc.ProcessWebhook(context.Background(), "integ-1", p0Body(), sig)
	if !errors.Is(err, ErrWebhookRateLimited) {
		t.Fatalf("segunda requisição acima do limite: esperava ErrWebhookRateLimited (429), veio %v", err)
	}
}

func TestValidateHMACP0_EmptyInputsNeverValidate(t *testing.T) {
	body := p0Body()
	if ValidateHMAC(body, "", "segredo") {
		t.Fatal("assinatura vazia não pode validar")
	}
	if ValidateHMAC(body, "sha256="+p0HMAC(body, "outro-segredo"), "segredo") {
		t.Fatal("assinatura de outro segredo não pode validar")
	}
	if !ValidateHMAC(body, "sha256="+p0HMAC(body, "segredo"), "segredo") {
		t.Fatal("assinatura correta deveria validar")
	}
}

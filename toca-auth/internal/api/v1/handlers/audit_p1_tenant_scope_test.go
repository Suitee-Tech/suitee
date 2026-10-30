package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P1 / A4 — trilha de auditoria não é cross-tenant por omissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A4) mostrou que `GET /api/v1/audit-trail` repassava o `tenant_id` do
// query string cru ao `pkg/audit.List` e, quando ele vinha vazio, devolvia TODOS
// os tenants — o comentário do próprio código admitia isso ("se mandar
// tenant_id vazio, devolve cross-tenant mesmo"). O gate
// `system:view_audit_trail` inclui papéis DE TENANT (`auditor`, `operacoes`),
// então qualquer auditor lia eventos alheios (`auth.banking_updated`, `lgpd.*`).
//
// Regra nova: principal de tenant só lê o próprio tenant (pedir outro ou omitir
// é 403); principal de serviço (X-Internal-Key) precisa dizer o tenant.

func auditReq(tenantID, queryTenant, principalType, functionalRole string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/audit-trail", nil)
	ctx := context.Background()
	if tenantID != "" {
		ctx = pkgcontext.WithTenantID(ctx, tenantID)
	}
	c.Request = c.Request.WithContext(ctx)
	c.Set("type", principalType)
	c.Set("functional_role", functionalRole)
	if queryTenant != "" {
		q := c.Request.URL.Query()
		q.Set("tenant_id", queryTenant)
		c.Request.URL.RawQuery = q.Encode()
	}
	return c
}

func TestAuditFilters_TenantPrincipalStaysInItsOwnTenant(t *testing.T) {
	f, err := auditFiltersFromRequest(auditReq("tenant-a", "", "", "auditor"))
	if err != nil {
		t.Fatalf("auditor do próprio tenant deveria poder listar: %v", err)
	}
	if f.TenantID != "tenant-a" {
		t.Fatalf("TenantID = %q, want tenant-a", f.TenantID)
	}
}

func TestAuditFilters_TenantPrincipalCannotReadOtherTenant(t *testing.T) {
	_, err := auditFiltersFromRequest(auditReq("tenant-a", "tenant-b", "", "auditor"))
	if !errors.Is(err, ErrCrossTenantAuditDenied) {
		t.Fatalf("VAZAMENTO CROSS-TENANT: auditor de tenant-a pediu tenant-b e passou (err=%v)", err)
	}
}

// O caso que originava o vazamento: omitir `tenant_id` fazia o pkg/audit.List
// consultar TODOS os tenants. Agora a omissão resolve para o tenant do próprio
// chamador — nunca para "tudo".
func TestAuditFilters_OmittedTenantResolvesToCallerTenant(t *testing.T) {
	f, err := auditFiltersFromRequest(auditReq("tenant-a", "", "", "auditor"))
	if err != nil {
		t.Fatalf("omitir tenant_id deveria ser permitido (escopo = próprio tenant): %v", err)
	}
	if f.TenantID != "tenant-a" {
		t.Fatalf("TenantID = %q — a omissão precisa resolver para o tenant do chamador, nunca para vazio/cross-tenant", f.TenantID)
	}
}

func TestAuditFilters_EmptyContextTenantIsDenied(t *testing.T) {
	if _, err := auditFiltersFromRequest(auditReq("", "", "", "operacoes")); !errors.Is(err, ErrCrossTenantAuditDenied) {
		t.Fatalf("token sem tenant deveria falhar fechado (err=%v)", err)
	}
}

// O caminho do auditor de plataforma continua existindo, mas exige credencial de
// serviço E tenant explícito.
func TestAuditFilters_InternalServiceMustNameTenant(t *testing.T) {
	if _, err := auditFiltersFromRequest(auditReq("", "", "service", "service")); !errors.Is(err, ErrCrossTenantAuditDenied) {
		t.Fatalf("chamador interno sem tenant explícito deveria ser recusado (err=%v)", err)
	}

	f, err := auditFiltersFromRequest(auditReq("", "tenant-b", "service", "service"))
	if err != nil {
		t.Fatalf("chamador interno com tenant explícito deveria passar: %v", err)
	}
	if f.TenantID != "tenant-b" {
		t.Fatalf("TenantID = %q, want tenant-b", f.TenantID)
	}
}

// Filtros não relacionados a tenant continuam funcionando.
func TestAuditFilters_PreservesOtherFilters(t *testing.T) {
	c := auditReq("tenant-a", "", "", "auditor")
	q := c.Request.URL.Query()
	q.Set("action", "auth.login_success")
	q.Set("limit", "50")
	c.Request.URL.RawQuery = q.Encode()

	f, err := auditFiltersFromRequest(c)
	if err != nil {
		t.Fatal(err)
	}
	if f.Action != "auth.login_success" || f.Limit != 50 || f.TenantID != "tenant-a" {
		t.Fatalf("filtros perdidos: %+v", f)
	}
}

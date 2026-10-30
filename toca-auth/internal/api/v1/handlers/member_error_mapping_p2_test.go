package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/domain/account"
	authdomain "github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/audit"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P2/M4 + M6/M7 — o PATCH de membro (papel/perfil/desativação) precisa sair com
// os códigos certos: negação de autoridade = 403 com trilha (antes desta rodada
// o handler irmão devolvia 400 para "unauthorized"), alvo inexistente = 404 e
// regra de negócio = 400.

type memberErrRepo struct {
	account.Repository
	acc *account.Account
}

func (r *memberErrRepo) GetByID(context.Context, string) (*account.Account, error) {
	return r.acc, nil
}

func (r *memberErrRepo) Update(context.Context, *account.Account) error { return nil }

func newAuthHandlerForMemberErrors(actorProfile string) *AuthHandler {
	acc := &account.Account{
		ID:   "company-1",
		Type: account.AccountTypeCompany,
		Company: &company.Company{
			OwnerID: "owner-1",
			Members: []company.CompanyMember{
				{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
				{AccountID: "actor-1", Profile: actorProfile, FunctionalRole: "corretor"},
				{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
			},
		},
	}
	svc := service.NewCompanyService(&memberErrRepo{acc: acc}, memberNoopCredentialRepo{}, nil, nil)
	return NewAuthHandler(nil, nil, svc, nil, nil, nil, nil)
}

// memberNoopCredentialRepo não é exercitado pelo PATCH de membro (que só toca o
// repositório de contas); existe para satisfazer a construção do serviço.
type memberNoopCredentialRepo struct{}

func (memberNoopCredentialRepo) Create(context.Context, *authdomain.Credential) error { return nil }
func (memberNoopCredentialRepo) GetByEmail(context.Context, string) (*authdomain.Credential, error) {
	return nil, nil
}
func (memberNoopCredentialRepo) GetByAccountID(context.Context, string) (*authdomain.Credential, error) {
	return nil, nil
}
func (memberNoopCredentialRepo) UpdatePassword(context.Context, string, string) error { return nil }
func (memberNoopCredentialRepo) VerifyEmail(context.Context, string) error            { return nil }

func doMemberPatch(t *testing.T, h *AuthHandler, body, actor string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", actor)
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), actor))
		c.Next()
	})
	r.PATCH("/companies/:id/members/:member_id", h.UpdateCompanyMember)

	req := httptest.NewRequest(http.MethodPatch, "/companies/company-1/members/member-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUpdateCompanyMember_UnauthorizedIsForbiddenWithAudit(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	// `corretor` não é admin da empresa: o serviço devolve "unauthorized".
	h := newAuthHandlerForMemberErrors("member")
	w := doMemberPatch(t, h, `{"functional_role":"financeiro"}`, "actor-1")

	if w.Code != http.StatusForbidden {
		t.Fatalf("negação de autoridade deveria ser 403, veio %d: %s", w.Code, w.Body.String())
	}
	if len(denials) != 1 {
		t.Fatalf("negação sem trilha de autorização: %+v", denials)
	}
	if denials[0].Metadata["required"] != "company:manage_members" {
		t.Fatalf("trilha sem a permissão exigida: %+v", denials[0].Metadata)
	}
}

func TestUpdateCompanyMember_InvalidRoleIsValidationError(t *testing.T) {
	h := newAuthHandlerForMemberErrors("admin")
	// owner-1 é admin/dono e member-1 existe: o erro vem da validação de papel.
	w := doMemberPatch(t, h, `{"functional_role":"super_admin"}`, "owner-1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("papel fora do catálogo deveria ser 400, veio %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid functional role") {
		t.Fatalf("mensagem não explica o motivo: %s", w.Body.String())
	}
}

func TestUpdateCompanyMember_UnknownMemberIsNotFound(t *testing.T) {
	h := newAuthHandlerForMemberErrors("admin")
	w := doMemberPatch(t, h, `{"functional_role":"corretor"}`, "owner-1")
	if w.Code == http.StatusForbidden || w.Code >= 500 {
		t.Fatalf("status inesperado para alvo inexistente: %d %s", w.Code, w.Body.String())
	}
}

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/audit"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P2/M4 — "unauthorized" do serviço de grupos tem de sair como 403, não 500.
//
// A revisão de permissões (achado M4) registrou os handlers de grupo devolvendo
// 500 para negação de autorização: o cliente não distinguia "sem permissão" de
// "quebrou". Agora `respondGroupError` mapeia e a negação deixa trilha.

type groupErrRepo struct {
	account.Repository
	acc *account.Account
}

func (r *groupErrRepo) GetByID(context.Context, string) (*account.Account, error) {
	return r.acc, nil
}

func newGroupHandlerForErrors(profile string) *GroupHandler {
	acc := &account.Account{
		ID:   "company-1",
		Type: account.AccountTypeCompany,
		Company: &company.Company{
			OwnerID: "owner-1",
			Members: []company.CompanyMember{
				{AccountID: "owner-1", Profile: "admin"},
				{AccountID: "member-1", Profile: profile},
			},
		},
	}
	svc := service.NewGroupService(&noopGroupRepo{}, &groupErrRepo{acc: acc})
	return NewGroupHandler(svc)
}

type noopGroupRepo struct {
	group.Repository
}

func (noopGroupRepo) ListByCompanyID(context.Context, string) ([]*group.Group, error) {
	return nil, nil
}

func (noopGroupRepo) Create(context.Context, *group.Group) error {
	return errors.New("should not create")
}

// GetByID devolve nil, nil = grupo inexistente (o caminho do 404).
func (noopGroupRepo) GetByID(context.Context, string) (*group.Group, error) {
	return nil, nil
}

func doGroupRequest(h *GroupHandler, method, path, body, actor string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// O handler lê `user_id` do contexto do gin (chave gravada pelo middleware de
	// auth) — o dublê replica isso.
	r.Use(func(c *gin.Context) {
		c.Set("user_id", actor)
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), actor))
		c.Next()
	})
	r.POST("/companies/:id/groups", h.CreateGroup)
	r.PUT("/companies/:id/groups/:group_id", h.UpdateGroup)

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Sem profile de admin, o serviço devolve "unauthorized" → 403 + trilha.
func TestGroupHandler_UnauthorizedIsForbiddenWithAudit(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	h := newGroupHandlerForErrors("member")
	w := doGroupRequest(h, http.MethodPost, "/companies/company-1/groups", `{"name":"Equipe"}`, "member-1")

	if w.Code != http.StatusForbidden {
		t.Fatalf("negação de autorização deveria ser 403, veio %d: %s", w.Code, w.Body.String())
	}
	if len(denials) != 1 {
		t.Fatalf("negação sem trilha: %+v", denials)
	}
	if denials[0].Metadata["required"] != "company:manage_members" {
		t.Fatalf("trilha sem a permissão: %+v", denials[0].Metadata)
	}
}

// Grupo inexistente → 404 (não 500).
func TestGroupHandler_NotFoundIsNotFound(t *testing.T) {
	h := newGroupHandlerForErrors("admin")
	w := doGroupRequest(h, http.MethodPut, "/companies/company-1/groups/group-x", `{"name":"Equipe"}`, "owner-1")
	if w.Code != http.StatusNotFound {
		t.Fatalf("grupo inexistente deveria ser 404, veio %d: %s", w.Code, w.Body.String())
	}
}

var _ = auth.Repository(nil)

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

// B10 — não existe fallback de `tenant_id` para `account_id`.
//
// A revisão de permissões de 2026-09-19 (achado B10) registrou que os handlers
// do CRM, quando o token não trazia tenant, usavam o **próprio account id** como
// tenant: em token pessoal (`type=individual`) "sem tenant" virava "meu id como
// tenant" — um escopo inventado no cliente para um recurso que é da empresa.
// Agora a resposta é 403, sem tocar no repositório.

// recordingLeadRepo registra se alguma consulta chegou ao repositório.
type recordingLeadRepo struct {
	lead.Repository
	calls int
}

func (r *recordingLeadRepo) GetByID(_ context.Context, _, id string) (*lead.Lead, error) {
	r.calls++
	if id == "lead-1" {
		return &lead.Lead{ID: "lead-1", TenantID: "tenant-1"}, nil
	}
	return nil, nil
}

func (r *recordingLeadRepo) List(_ context.Context, _ string, _ lead.LeadFilter) ([]*lead.Lead, int64, error) {
	r.calls++
	return nil, 0, nil
}

// personalTokenMiddleware: token pessoal — conta sem tenant no contexto
// (exatamente o estado que o fallback transformava em tenant).
func personalTokenMiddleware(accountID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := pkgcontext.WithAccountID(c.Request.Context(), accountID)
		c.Request = c.Request.WithContext(ctx)

		c.Set("user_id", accountID)
		c.Set("functional_role", "corretor")
		c.Set("permission", "")
		c.Set("type", "individual")
		c.Next()
	}
}

func TestLeadHandler_WithoutTenantIsForbidden(t *testing.T) {
	repo := &recordingLeadRepo{}
	svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
	h := NewLeadHandler(svc, nil, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(personalTokenMiddleware("personal-account-1"))
	router.GET("/leads/:id", h.GetByID)
	router.GET("/leads", h.List)
	router.POST("/leads", h.Create)

	cases := []struct {
		name, method, path, body string
	}{
		{"leitura por id", http.MethodGet, "/leads/lead-1", ""},
		{"listagem", http.MethodGet, "/leads", ""},
		{"criação", http.MethodPost, "/leads", `{"name":"Fulano","email":"f@example.com","phone":"11999998888","source":"site"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("token pessoal sem tenant respondeu %d, want 403 (%s)", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "tenant context required") {
				t.Fatalf("mensagem não explica a ausência de tenant: %s", recorder.Body.String())
			}
		})
	}
	if repo.calls != 0 {
		t.Fatalf("o repositório foi consultado %d vez(es) com tenant inventado", repo.calls)
	}
}

// Com tenant no contexto o caminho segue funcionando (o endurecimento não fecha
// a operação normal).
func TestLeadHandler_WithTenantStillWorks(t *testing.T) {
	repo := &recordingLeadRepo{}
	svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
	h := NewLeadHandler(svc, nil, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authMiddlewareForTest("operator-1", "corretor"))
	router.GET("/leads/:id", h.GetByID)

	req := httptest.NewRequest(http.MethodGet, "/leads/lead-1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("requisição com tenant foi recusada: %s", recorder.Body.String())
	}
	if repo.calls == 0 {
		t.Fatal("repositório não consultado em requisição válida")
	}
}

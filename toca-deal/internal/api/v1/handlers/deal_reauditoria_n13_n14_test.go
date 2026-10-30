package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

// Reauditoria de 2026-09-20 — N13 (checklist por PUT) e N14 (divisão de comissão
// que exclui o responsável). Ambos falhavam ANTES e a mutação correspondente foi
// verificada.

func newChecklistRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDealHandler(services.NewDealService(deal.Repository(nil), nil, nil, nil, nil))
	r := gin.New()
	r.PUT("/deals/:id/checklist", h.InitChecklist)
	return r
}

func putChecklist(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/deals/deal-1/checklist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// N13 — o PUT substitui a checklist inteira e o domínio copia o `status` do
// cliente verbatim. Um `corretor` (deals:write, sem documents:validate) aplicava
// os itens obrigatórios já APPROVED e destravava o gate de contrato — o mesmo
// bypass que o A18 fechou no PATCH, pelo caminho irmão.
func TestInitChecklist_RejectsClientSuppliedReviewStatus(t *testing.T) {
	r := newChecklistRouter()

	cases := map[string]string{
		"APPROVED":    `{"items":[{"name":"RG","category":"DOCUMENTO","required":true,"status":"APPROVED"}]}`,
		"REJECTED":    `{"items":[{"name":"RG","category":"DOCUMENTO","required":true,"status":"REJECTED"}]}`,
		"UPLOADED":    `{"items":[{"name":"RG","category":"DOCUMENTO","required":true,"status":"UPLOADED"}]}`,
		"reviewed_at": `{"items":[{"name":"RG","category":"DOCUMENTO","required":true,"reviewed_at":"2026-09-20T12:00:00Z"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			w := putChecklist(r, body)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status de revisão sem documents:validate: HTTP %d, quer 403 (body=%s)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "documents:validate") {
				t.Fatalf("403 pelo motivo errado: %s", w.Body.String())
			}
		})
	}
}

// O fluxo legítimo (o que o frontend manda) não carrega status: a guarda não
// pode barrá-lo. Sem tenant no contexto ele para depois, por outro motivo.
func TestInitChecklist_AllowsPayloadWithoutStatus(t *testing.T) {
	r := newChecklistRouter()

	for name, body := range map[string]string{
		"sem status":     `{"items":[{"name":"RG","category":"DOCUMENTO","required":true}]}`,
		"status PENDING": `{"items":[{"name":"RG","category":"DOCUMENTO","required":true,"status":"PENDING"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := putChecklist(r, body)
			if strings.Contains(w.Body.String(), "documents:validate") {
				t.Fatalf("guarda barrou payload legítimo (%s): %s", name, w.Body.String())
			}
		})
	}
}

func newCreateDealRouter(tenantID, accountID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDealHandler(services.NewDealService(deal.Repository(nil), nil, nil, nil, nil))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), tenantID)
		ctx = pkgcontext.WithAccountID(ctx, accountID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/deals", h.Create)
	return r
}

func postDeal(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/deals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// N14 — quem NÃO precisou de `deals:approve` para atribuir (está criando o
// negócio no próprio nome) não pode sumir da divisão de comissão que ele mesmo
// define: sem isto, o corretor roteava 100% da comissão para ids arbitrários.
func TestCreate_RejectsCommissionSplitExcludingResponsibleBroker(t *testing.T) {
	const actor = "broker-1"
	r := newCreateDealRouter("tenant-1", actor)

	body := `{"type":"VENDA","property_id":"prop-1","owner_id":"owner-1","customer_id":"lead-1","value":100,"broker_id":"` + actor + `","broker_participations":[` +
		`{"broker_id":"outro-1","role":"captador","percentage":60},` +
		`{"broker_id":"outro-2","role":"fechamento","percentage":40}]}`

	w := postDeal(r, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("divisão sem o responsável: HTTP %d, quer 403 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "exclude the responsible broker") {
		t.Fatalf("403 pelo motivo errado: %s", w.Body.String())
	}
}

// A matriz "pode/não pode" é testada na função extraída: os casos PERMITIDOS
// seguiriam para o repositório (nulo neste harness) e o teste mediria um panic,
// não a guarda. Mesmo padrão de `actorCanApproveDeal`.
func TestCommissionSplitExcludesResponsibleBroker(t *testing.T) {
	const actor = "broker-1"

	cases := []struct {
		name             string
		items            []SetBrokerParticipationItem
		requiresApproval bool
		want             bool
	}{
		{
			name:  "sem participações não há divisão a checar",
			items: nil,
			want:  false,
		},
		{
			name: "divisão com terceiros mantendo o responsável (captador + co-captador)",
			items: []SetBrokerParticipationItem{
				{BrokerID: actor, Role: "captador", Percentage: 70},
				{BrokerID: "outro-1", Role: "co_captador", Percentage: 30},
			},
			want: false,
		},
		{
			name: "divisão que EXCLUI o responsável sem autoridade",
			items: []SetBrokerParticipationItem{
				{BrokerID: "outro-1", Role: "captador", Percentage: 60},
				{BrokerID: "outro-2", Role: "fechamento", Percentage: 40},
			},
			want: true,
		},
		{
			name: "gestor redistribuindo: tem autoridade, segue livre",
			items: []SetBrokerParticipationItem{
				{BrokerID: "outro-1", Role: "captador", Percentage: 60},
				{BrokerID: "outro-2", Role: "fechamento", Percentage: 40},
			},
			requiresApproval: true,
			want:             false,
		},
		{
			name: "imobiliária na divisão (negócio da própria agência)",
			items: []SetBrokerParticipationItem{
				{Role: "imobiliaria", Percentage: 100},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/deals", nil)
			c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), actor))

			if got := commissionSplitExcludesResponsibleBroker(c, tc.items, tc.requiresApproval); got != tc.want {
				t.Fatalf("exclui responsável = %v, quer %v", got, tc.want)
			}
		})
	}
}

// Atribuir a OUTRO corretor continua exigindo `deals:approve` (A24, preservado).
func TestCreate_StillRequiresApproveToAssignAnotherBroker(t *testing.T) {
	r := newCreateDealRouter("tenant-1", "broker-1")

	body := `{"type":"VENDA","property_id":"prop-1","owner_id":"owner-1","customer_id":"lead-1","value":100,"broker_id":"outro-1"}`
	w := postDeal(r, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("atribuição a terceiro: HTTP %d, quer 403 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "assign the deal to another broker") {
		t.Fatalf("403 pelo motivo errado: %s", w.Body.String())
	}
}

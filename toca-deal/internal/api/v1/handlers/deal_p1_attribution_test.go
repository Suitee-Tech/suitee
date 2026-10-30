package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P1 / A24 — atribuição de negócio a terceiro exige autoridade.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A24) mostrou que `POST /deals` usava `broker_id` do corpo sem validação:
// um corretor criava negócio atribuído a outro, e a atribuição dirige a
// distribuição de comissões (`broker_participations`) e o escopo de propriedade
// aplicado nas demais rotas — primitivo de fraude de comissionamento.

func attributionCtx(actorID string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/deals", nil)
	ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
	if actorID != "" {
		ctx = pkgcontext.WithAccountID(ctx, actorID)
	}
	c.Request = c.Request.WithContext(ctx)
	c.Set("tenant_id", "tenant-1")
	c.Set("user_id", actorID)
	return c
}

func TestDealAttributionRequiresAuthority(t *testing.T) {
	cases := []struct {
		name     string
		actor    string
		brokerID string
		want     bool
	}{
		{"atribuir a si mesmo não exige autoridade extra", "corretor-1", "corretor-1", false},
		{"atribuir a outro exige autoridade", "corretor-1", "corretor-2", true},
		{"sem broker_id não há atribuição", "corretor-1", "", false},
		{"broker_id em branco não conta", "corretor-1", "   ", false},
		{"sem conta no contexto exige autoridade (fail-closed)", "", "corretor-2", true},
		{"sem conta e sem broker_id não bloqueia (nada a atribuir)", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dealAttributionRequiresAuthority(attributionCtx(tc.actor), tc.brokerID); got != tc.want {
				t.Fatalf("dealAttributionRequiresAuthority = %v, want %v", got, tc.want)
			}
		})
	}
}

// Guarda estática: o gate precisa continuar no handler de criação (os testes de
// unidade acima cobrem a decisão, não o wiring).
func TestCreateDealHandlerKeepsAttributionGate(t *testing.T) {
	src, err := os.ReadFile("deal_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "dealAttributionRequiresAuthority(c, req.BrokerID)") {
		t.Fatal("P0/P1-A24: o handler Create deixou de checar a autoridade de atribuição")
	}
	if !strings.Contains(body, "deals:approve is required to assign the deal to another broker") {
		t.Fatal("P0/P1-A24: a recusa de atribuição sem permissão desapareceu")
	}
}

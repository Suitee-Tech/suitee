package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// settingsStub devolve uma configuração fixa por tenant. `fail` simula
// indisponibilidade da leitura.
type settingsStub struct {
	reqd bool
	fail bool
}

func (s settingsStub) Get(context.Context, string) (*tenant_financial_settings.Settings, error) {
	if s.fail {
		return nil, errors.New("settings unavailable")
	}
	return &tenant_financial_settings.Settings{PayoutProofRequired: s.reqd}, nil
}

func TestRequireManualProof(t *testing.T) {
	for _, tc := range []struct {
		name      string
		settings  proofPolicySettings
		proofURL  string
		wantError bool
	}{
		{name: "comprovante presente e dispensado", settings: settingsStub{reqd: false}, proofURL: "https://x/p.pdf"},
		{name: "comprovante presente e exigido", settings: settingsStub{reqd: true}, proofURL: "https://x/p.pdf"},
		{name: "sem comprovante e dispensado", settings: settingsStub{reqd: false}, proofURL: ""},
		{name: "sem comprovante e exigido", settings: settingsStub{reqd: true}, proofURL: "", wantError: true},
		{name: "espaços em branco não contam como comprovante", settings: settingsStub{reqd: true}, proofURL: "   ", wantError: true},
		// Sem configuração conectada vale o default do domínio (exigir): não
		// conseguir ler a política não pode liberar a baixa.
		{name: "sem settings exige", settings: nil, proofURL: "", wantError: true},
		{name: "falha de leitura exige", settings: settingsStub{fail: true}, proofURL: "", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireManualProof(context.Background(), tc.settings, "tenant-1", tc.proofURL)
			if tc.wantError && !errors.Is(err, ErrManualProofRequired) {
				t.Fatalf("err = %v, want ErrManualProofRequired", err)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}

// TestTransferProcessProofPolicy prova a regra ponta-a-ponta no endpoint de
// repasse: é a borda HTTP que precisa recusar a baixa manual sem evidência,
// independentemente do que a interface permite.
func TestTransferProcessProofPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name     string
		settings proofPolicySettings
		body     string
		wantCode int
	}{
		{
			name:     "política exigindo e sem comprovante é recusado",
			settings: settingsStub{reqd: true},
			body:     `{}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "política dispensando permite seguir sem comprovante",
			settings: settingsStub{reqd: false},
			body:     `{}`,
			// Conflito de CAS é o próximo erro: prova que o guard passou.
			wantCode: http.StatusConflict,
		},
		{
			name:     "política exigindo e com comprovante segue adiante",
			settings: settingsStub{reqd: true},
			body:     `{"proof_url":"https://files.example/repasse.pdf"}`,
			wantCode: http.StatusConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Stub já definido em transfer_lifecycle_conflict_test.go: o item
			// existe mas a versão não confere, então o fluxo termina em conflito
			// de CAS (409). O que importa aqui é distinguir o guard (400) de tudo
			// que comprova que a requisição passou por ele.
			item := transfer.New("contract-x", "owner-x", "property-x", "tenant-1", "2026-09", 100, 0, 0, 0, nil, "")
			item.ID = "transfer-x"
			item.Status = transfer.StatusApproved
			handler := NewTransferHandler(service.NewTransferService(&conflictingTransferLifecycleRepo{item: item}))
			handler.SetProofPolicy(tc.settings)

			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.POST("/transfers/:id/process", handler.Process)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/transfers/transfer-x/process",
				strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantCode {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), tc.wantCode)
			}
		})
	}
}

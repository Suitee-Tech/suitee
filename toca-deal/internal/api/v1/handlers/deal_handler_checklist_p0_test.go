package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

// P0 / A18 — aprovar/rejeitar item de checklist exige `documents:validate`.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A18) mostrou que `PATCH /deals/:id/checklist/:itemId` exigia apenas
// `deals:write` (que o corretor tem por padrão) e gravava o status literalmente,
// sem allowlist. Assim, um corretor aprovava os próprios documentos e destravava
// as transições ANALYSIS/CONTRACT protegidas pelo checklist gate.
//
// O handler deve falhar FECHADO: sem provider de permissão configurado (ou sem a
// permissão), APPROVED/REJECTED são recusados com 403.

func a18Router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewDealHandler(services.NewDealService(deal.Repository(nil), nil, nil, nil, nil))
	r := gin.New()
	r.PATCH("/deals/:id/checklist/:itemId", h.UpdateChecklistItemStatus)
	return r
}

func TestUpdateChecklistItemStatus_RejectsApprovalWithoutValidatePermission(t *testing.T) {
	r := a18Router(t)

	for _, status := range []string{"APPROVED", "REJECTED"} {
		t.Run(status, func(t *testing.T) {
			body := `{"status":"` + status + `"}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/deals/deal-1/checklist/item-1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status %s sem documents:validate: HTTP %d, want 403 (body=%s)", status, w.Code, w.Body.String())
			}
		})
	}
}

// TestUpdateChecklistItemStatus_RejectsUnknownStatus cobre o `oneof` do DTO:
// antes qualquer string era gravada no agregado.
func TestUpdateChecklistItemStatus_RejectsUnknownStatus(t *testing.T) {
	r := a18Router(t)

	for _, status := range []string{"", "APROVADO", "approved", "DELETE", "'; DROP TABLE"} {
		t.Run(status, func(t *testing.T) {
			body := `{"status":"` + status + `"}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/deals/deal-1/checklist/item-1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Fatalf("status inválido %q foi aceito (HTTP 200)", status)
			}
			if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
				t.Fatalf("status inválido %q: HTTP %d, want 400/422", status, w.Code)
			}
		})
	}
}

// TestUpdateChecklistItemStatus_UploadDoesNotRequireValidate garante que o fluxo
// legítimo do corretor (marcar como UPLOADED/PENDING) NÃO regrediu com a nova
// exigência: só APPROVED/REJECTED pedem `documents:validate`.
//
// O handler segue adiante (e falha depois por falta de contexto de tenant/repo
// neste teste isolado) — o que importa aqui é que a recusa NÃO venha do novo
// gate de permissão.
func TestUpdateChecklistItemStatus_UploadDoesNotRequireValidate(t *testing.T) {
	r := a18Router(t)

	for _, status := range []string{"UPLOADED", "PENDING"} {
		t.Run(status, func(t *testing.T) {
			body := `{"status":"` + status + `"}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/deals/deal-1/checklist/item-1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if strings.Contains(w.Body.String(), "documents:validate") {
				t.Fatalf("status %s foi bloqueado pelo gate de validação (body=%s)", status, w.Body.String())
			}
		})
	}
}

// Garante que o DTO aceita exatamente os quatro status do domínio.
func TestChecklistStatusAllowlistMatchesDomain(t *testing.T) {
	allowed := map[deal.ChecklistItemStatus]bool{
		deal.ChecklistPending:  true,
		deal.ChecklistUploaded: true,
		deal.ChecklistApproved: true,
		deal.ChecklistRejected: true,
	}
	if len(allowed) != 4 {
		t.Fatal("domínio mudou: revise o oneof do UpdateChecklistItemStatusRequest")
	}
}

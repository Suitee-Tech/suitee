package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// zeroBalanceTransferRepo persiste de verdade (UpdateIfVersion aplica o CAS) —
// o repo de conflitos do outro teste sempre perde o CAS, o que responderia 409
// antes de qualquer asserção de estado.
type zeroBalanceTransferRepo struct {
	item *transfer.OwnerTransfer
}

func (r *zeroBalanceTransferRepo) Create(_ context.Context, item *transfer.OwnerTransfer) error {
	r.item = item
	return nil
}

func (r *zeroBalanceTransferRepo) GetByID(_ context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	clone.History = append([]transfer.HistoryEntry(nil), r.item.History...)
	clone.DeductionDetails = append([]transfer.Deduction(nil), r.item.DeductionDetails...)
	if r.item.OwnerBalance != nil {
		balance := *r.item.OwnerBalance
		clone.OwnerBalance = &balance
	}
	return &clone, nil
}

func (r *zeroBalanceTransferRepo) GetByContractMonth(context.Context, string, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}

func (r *zeroBalanceTransferRepo) Update(_ context.Context, item *transfer.OwnerTransfer) error {
	r.item = item
	return nil
}

func (r *zeroBalanceTransferRepo) UpdateIfVersion(_ context.Context, item *transfer.OwnerTransfer, expected time.Time) (bool, error) {
	if r.item == nil || !r.item.UpdatedAt.Equal(expected) {
		return false, nil
	}
	r.item = item
	return true, nil
}

func (r *zeroBalanceTransferRepo) List(context.Context, string, string, string, string, string, int64, int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *zeroBalanceTransferRepo) ListByOwnerLeadIDs(context.Context, string, []string, transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *zeroBalanceTransferRepo) UpdateStatus(context.Context, string, string, transfer.Status) error {
	return nil
}

func (r *zeroBalanceTransferRepo) Search(context.Context, string, transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func newZeroBalanceTransfer(item *transfer.OwnerTransfer) *transfer.OwnerTransfer {
	if item.OwnerBalance != nil {
		return item
	}
	balance, err := transfer.NewOwnerBalance(0, 0, 0)
	if err != nil {
		panic(err)
	}
	item.OwnerBalance = &balance
	item.Calculate()
	return item
}

// A baixa de um repasse sem saldo a repassar precisa funcionar SEM comprovante:
// nenhum pagamento aconteceu, e a política de evidência do tenant (default
// exigindo comprovante) deixaria o registro travado para sempre.
func TestTransferProcessZeroBalanceSettlesWithoutProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		status transfer.Status
	}{
		{name: "aprovado", status: transfer.StatusApproved},
		{name: "pendente", status: transfer.StatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := newZeroBalanceTransfer(transfer.New(
				"contract-zero", "owner-zero", "property-zero", "tenant-zero", "2026-10", 0, 0, 0, 0, nil, ""))
			item.ID = "transfer-zero"
			item.Status = tc.status

			repo := &zeroBalanceTransferRepo{item: item}
			handler := NewTransferHandler(service.NewTransferService(repo))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
				ctx = pkgcontext.WithAccountID(ctx, "op-zero")
				ctx = pkgcontext.WithUserName(ctx, "Operadora Zero")
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.POST("/transfers/:id/process", handler.Process)

			// Sem corpo: sem comprovante e sem observação (o guard de política
			// exigiria comprovante em qualquer repasse com valor a pagar).
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/transfers/transfer-zero/process", strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Data transfer.OwnerTransfer `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("resposta inválida: %v body=%s", err, recorder.Body.String())
			}
			if payload.Data.Status != transfer.StatusProcessed || !payload.Data.SettledWithoutBalance {
				t.Fatalf("repasse zerado deveria concluir sem saldo: %+v", payload.Data)
			}
			if payload.Data.ProcessedBy != "op-zero" {
				t.Fatalf("operador que concluiu deve ficar no registro: %q", payload.Data.ProcessedBy)
			}
			if payload.Data.SettlementJournalSourceID != "" {
				t.Fatalf("conclusão sem caixa não pode ter lançamento: %q", payload.Data.SettlementJournalSourceID)
			}
			// A conclusão é reentrega-segura: o retry responde 200 novamente.
			retry := httptest.NewRecorder()
			router.ServeHTTP(retry, httptest.NewRequest(http.MethodPost, "/transfers/transfer-zero/process", strings.NewReader(`{}`)))
			if retry.Code != http.StatusOK {
				t.Fatalf("retry status=%d body=%s, want 200", retry.Code, retry.Body.String())
			}
		})
	}
}

// A política de comprovante continua valendo para repasses com valor a pagar.
func TestTransferProcessRejectsMissingProofForPayableTransfer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item := transfer.New("contract-paid", "owner-paid", "property-paid", "tenant-paid", "2026-10", 1000, 10, 0, 0, nil, "")
	item.ID = "transfer-paid"
	item.Status = transfer.StatusApproved
	balance, err := transfer.NewOwnerBalance(90000, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := item.SetAllocatedOwnerBalance(balance, 10000); err != nil {
		t.Fatal(err)
	}

	repo := &zeroBalanceTransferRepo{item: item}
	handler := NewTransferHandler(service.NewTransferService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
		ctx = pkgcontext.WithAccountID(ctx, "op-paid")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/transfers/:id/process", handler.Process)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/transfers/transfer-paid/process", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
	if item.Status != transfer.StatusApproved {
		t.Fatalf("repasse com valor a pagar não pode ser encerrado: %s", item.Status)
	}
}

func TestTransferProcessZeroBalanceCancelledIsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item := newZeroBalanceTransfer(transfer.New(
		"contract-zero", "owner-zero", "property-zero", "tenant-zero", "2026-10", 0, 0, 0, 0, nil, ""))
	item.ID = "transfer-zero"
	if err := item.Cancel("op-1", "Operadora", "competência errada"); err != nil {
		t.Fatal(err)
	}

	repo := &zeroBalanceTransferRepo{item: item}
	handler := NewTransferHandler(service.NewTransferService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
		ctx = pkgcontext.WithAccountID(ctx, "op-zero")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/transfers/:id/process", handler.Process)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/transfers/transfer-zero/process", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
}

package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type conflictingTransferLifecycleRepo struct {
	item *transfer.OwnerTransfer
}

func (r *conflictingTransferLifecycleRepo) Create(_ context.Context, item *transfer.OwnerTransfer) error {
	r.item = item
	return nil
}

func (r *conflictingTransferLifecycleRepo) GetByID(_ context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	clone.DeductionDetails = append([]transfer.Deduction(nil), r.item.DeductionDetails...)
	clone.History = append([]transfer.HistoryEntry(nil), r.item.History...)
	return &clone, nil
}

func (r *conflictingTransferLifecycleRepo) GetByContractMonth(context.Context, string, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}

func (r *conflictingTransferLifecycleRepo) Update(_ context.Context, item *transfer.OwnerTransfer) error {
	r.item = item
	return nil
}

func (r *conflictingTransferLifecycleRepo) UpdateIfVersion(context.Context, *transfer.OwnerTransfer, time.Time) (bool, error) {
	return false, nil
}

func (r *conflictingTransferLifecycleRepo) List(context.Context, string, string, string, string, string, int64, int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *conflictingTransferLifecycleRepo) ListByOwnerLeadIDs(context.Context, string, []string, transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *conflictingTransferLifecycleRepo) UpdateStatus(context.Context, string, string, transfer.Status) error {
	return nil
}

func (r *conflictingTransferLifecycleRepo) Search(context.Context, string, transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func TestTransferLifecycleConflictMapsToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name   string
		path   string
		status transfer.Status
		wire   func(*gin.Engine, *TransferHandler)
	}{
		{name: "approve", path: "/transfers/transfer-conflict/approve", status: transfer.StatusPending, wire: func(router *gin.Engine, handler *TransferHandler) {
			router.POST("/transfers/:id/approve", handler.Approve)
		}},
		{name: "process", path: "/transfers/transfer-conflict/process", status: transfer.StatusApproved, wire: func(router *gin.Engine, handler *TransferHandler) {
			router.POST("/transfers/:id/process", handler.Process)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := transfer.New("contract-conflict", "owner-conflict", "property-conflict", "tenant-conflict", "2026-08", 100, 0, 0, 0, nil, "")
			item.ID = "transfer-conflict"
			item.Status = tt.status
			repo := &conflictingTransferLifecycleRepo{item: item}
			handler := NewTransferHandler(service.NewTransferService(repo))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			tt.wire(router, handler)

			responseRecorder := httptest.NewRecorder()
			// /process exige comprovante (política do tenant): sem ele o guard
			// responde 400 antes de alcançar o conflito que este teste cobre.
			// /approve não é baixa manual e segue sem corpo.
			var body io.Reader
			if strings.HasSuffix(tt.path, "/process") {
				body = strings.NewReader(`{"proof_url":"https://files.example/repasse.pdf"}`)
			}
			request := httptest.NewRequest(http.MethodPost, tt.path, body)
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s, want 409", responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
}

type noOpSettlementJournalRepo struct{}

func (noOpSettlementJournalRepo) Create(context.Context, *journal.Entry) error { return nil }
func (noOpSettlementJournalRepo) GetByID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (noOpSettlementJournalRepo) GetBySourceEventID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (noOpSettlementJournalRepo) UpdateStatus(context.Context, *journal.Entry, time.Time) (bool, error) {
	return false, nil
}
func (noOpSettlementJournalRepo) List(context.Context, string, string, int64, int64) ([]*journal.Entry, int64, error) {
	return nil, 0, nil
}
func (noOpSettlementJournalRepo) AccountBalance(context.Context, string, journal.Account) (int64, error) {
	return 0, nil
}

func TestTransferSettlementValidationMapsToHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, *transfer.OwnerTransfer)
		want  int
	}{
		{name: "missing allocation coverage", want: http.StatusConflict},
		{name: "net differs from available", setup: func(t *testing.T, item *transfer.OwnerTransfer) {
			balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := item.SetAllocatedOwnerBalance(balance, 40000); err != nil {
				t.Fatal(err)
			}
			item.NetAmount = 5600
		}, want: http.StatusConflict},
		// Um repasse sem saldo a repassar NÃO é mais erro de validação: ele é
		// concluído sem perna de caixa (comprovante dispensado). A cobertura
		// desse caminho está em transfer_zero_balance_handler_test.go.
		{name: "zero settlement stays without journal validation", setup: func(t *testing.T, item *transfer.OwnerTransfer) {
			balance, err := transfer.NewOwnerBalance(0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := item.SetAllocatedOwnerBalance(balance, 0); err != nil {
				t.Fatal(err)
			}
		}, want: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := transfer.New("contract-coverage", "owner-coverage", "property-coverage", "tenant-coverage", "2026-08", 5000, 8, 0, 0, nil, "")
			item.ID = "transfer-coverage"
			item.Status = transfer.StatusApproved
			if tc.setup != nil {
				tc.setup(t, item)
			}

			repo := &conflictingTransferLifecycleRepo{item: item}
			transferService := service.NewTransferService(repo).
				WithJournalService(service.NewJournalService(noOpSettlementJournalRepo{}))
			handler := NewTransferHandler(transferService)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
				ctx = database.WithTx(ctx, &sqlx.Tx{})
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.POST("/transfers/:id/process", handler.Process)

			responseRecorder := httptest.NewRecorder()
			// Comprovante presente: o objetivo é a validação de liquidação, não o
			// guard de comprovante, que responderia 400 e mascararia o caso.
			request := httptest.NewRequest(http.MethodPost, "/transfers/transfer-coverage/process",
				strings.NewReader(`{"proof_url":"https://files.example/repasse.pdf"}`))
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tc.want {
				t.Fatalf("status=%d body=%s, want %d", responseRecorder.Code, responseRecorder.Body.String(), tc.want)
			}
		})
	}
}

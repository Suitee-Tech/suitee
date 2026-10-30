package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type transactionHandlerRepo struct {
	byID       map[string]*transaction.Transaction
	batchCalls int
}

type transactionHandlerCommissionRepo struct {
	service.CommissionRepository
	bySource map[string]*commission.Commission
}

func (r *transactionHandlerCommissionRepo) Create(_ context.Context, item *commission.Commission) error {
	clone := *item
	r.bySource[item.SourceEventID] = &clone
	return nil
}

func (r *transactionHandlerCommissionRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*commission.Commission, error) {
	item := r.bySource[sourceEventID]
	if item == nil || item.TenantID != tenantID {
		return nil, nil
	}
	clone := *item
	return &clone, nil
}

func newTransactionHandlerRepo() *transactionHandlerRepo {
	return &transactionHandlerRepo{byID: make(map[string]*transaction.Transaction)}
}

func (r *transactionHandlerRepo) Create(_ context.Context, tx *transaction.Transaction) error {
	r.byID[tx.ID] = tx
	return nil
}

func (r *transactionHandlerRepo) CreateBatch(_ context.Context, txs []*transaction.Transaction) error {
	r.batchCalls++
	for _, tx := range txs {
		if existing := r.findBySource(tx.TenantID, tx.SourceEventID); existing == nil {
			r.byID[tx.ID] = tx
		}
	}
	return nil
}

func (r *transactionHandlerRepo) GetByID(_ context.Context, tenantID, id string) (*transaction.Transaction, error) {
	tx := r.byID[id]
	if tx == nil || tx.TenantID != tenantID {
		return nil, nil
	}
	return tx, nil
}

func (r *transactionHandlerRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	return r.findBySource(tenantID, sourceEventID), nil
}

func (r *transactionHandlerRepo) findBySource(tenantID, sourceEventID string) *transaction.Transaction {
	for _, tx := range r.byID {
		if tx.TenantID == tenantID && tx.SourceEventID == sourceEventID {
			return tx
		}
	}
	return nil
}

func (r *transactionHandlerRepo) Update(_ context.Context, tx *transaction.Transaction) error {
	r.byID[tx.ID] = tx
	return nil
}

func (r *transactionHandlerRepo) Delete(_ context.Context, id, _ string) error {
	delete(r.byID, id)
	return nil
}

func (r *transactionHandlerRepo) List(_ context.Context, _, _ string, _, _ int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}

func (r *transactionHandlerRepo) Search(_ context.Context, _ string, _ transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}

func transactionHandlerRouter(repo *transactionHandlerRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewTransactionHandler(service.NewTransactionService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/transactions/batch", handler.CreateBatch)
	router.POST("/transactions/:id/settle", handler.Settle)
	return router
}

func TestTransactionHandler_CreateBatchRequiresIdempotencyKey(t *testing.T) {
	repo := newTransactionHandlerRepo()
	router := transactionHandlerRouter(repo)
	body := []byte(`{"items":[{"type":"receita","description":"Receita manual","value":100,"date":"2026-08-10"}]}`)

	request := httptest.NewRequest(http.MethodPost, "/transactions/batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "   ")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if repo.batchCalls != 0 {
		t.Fatalf("missing operation key must not write, got %d batch calls", repo.batchCalls)
	}
}

func TestTransactionHandler_CreateBatchAcceptsExplicitCommissionRetention(t *testing.T) {
	gin.SetMode(gin.TestMode)
	txRepo := newTransactionHandlerRepo()
	commissionRepo := &transactionHandlerCommissionRepo{bySource: make(map[string]*commission.Commission)}
	handler := NewTransactionHandler(service.NewTransactionService(txRepo).
		WithManualCommissionService(service.NewCommissionService(commissionRepo)))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
		c.Request = c.Request.WithContext(database.WithTx(ctx, &sqlx.Tx{}))
		c.Next()
	})
	router.POST("/transactions/batch", handler.CreateBatch)

	body := []byte(`{
		"items":[
			{"contract_id":"contract-1","type":"receita","description":"Receita","value":10000,"date":"2026-08-10"},
			{"contract_id":"contract-1","type":"comissao","description":"Captador","value":300,"date":"2026-08-10","broker_id":"broker-cap","role":"captador"},
			{"contract_id":"contract-1","type":"comissao","description":"Fechamento","value":500,"date":"2026-08-10","broker_id":"broker-close","role":"fechamento"}
		],
		"commissions":[
			{"deal_id":"deal-1","contract_id":"contract-1","broker_id":"broker-cap","role":"captador","percentage":30,"amount":300},
			{"deal_id":"deal-1","contract_id":"contract-1","broker_id":"broker-close","role":"fechamento","percentage":50,"amount":500}
		],
		"commission_retention":{"percentage":20,"amount":200}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/transactions/batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "handler-explicit-retention")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", responseRecorder.Code, responseRecorder.Body.String())
	}
	if txRepo.batchCalls != 1 || len(txRepo.byID) != 3 || len(commissionRepo.bySource) != 2 {
		t.Fatalf("posting = calls:%d transactions:%d commissions:%d, want 1/3/2", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.bySource))
	}
	for _, item := range commissionRepo.bySource {
		if item.Role == "imobiliaria" || item.BrokerID == "" {
			t.Fatalf("agency retention became a payable: %+v", item)
		}
	}
}

func TestTransactionHandler_SettleReversedReturnsConflict(t *testing.T) {
	repo := newTransactionHandlerRepo()
	entry := transaction.New("contract-1", transaction.TypeReceita, "Receita", 100, time.Now(), "tenant-1")
	entry.Status = transaction.StatusReversed
	repo.byID[entry.ID] = entry
	router := transactionHandlerRouter(repo)

	request := httptest.NewRequest(http.MethodPost, "/transactions/"+entry.ID+"/settle", bytes.NewBufferString(`{"proof_url":"https://files.example/baixa.pdf"}`))
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if entry.Status != transaction.StatusReversed {
		t.Fatalf("reversed transaction was reopened: %s", entry.Status)
	}
}

func TestTransactionHandler_SettleUnknownIDReturnsNotFound(t *testing.T) {
	repo := newTransactionHandlerRepo()
	router := transactionHandlerRouter(repo)

	request := httptest.NewRequest(http.MethodPost, "/transactions/not-a-transaction/settle", bytes.NewBufferString(`{"proof_url":"https://files.example/baixa.pdf"}`))
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// A simple mock repo for integration tests
type mockChargeRepo struct {
	updateCalls      int
	charges          map[string]*charge.Charge
	overdueCharges   []*charge.Charge
	overdueTenantID  string
	listCalls        int
	listOverdueCalls int
}

func (m *mockChargeRepo) Create(ctx context.Context, c *charge.Charge) error {
	m.charges[c.ID] = c
	return nil
}
func (m *mockChargeRepo) GetByID(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	c, ok := m.charges[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}
func (m *mockChargeRepo) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*charge.Charge, error) {
	if sourceEventID == "" {
		return nil, nil
	}
	for _, c := range m.charges {
		if c.TenantID == tenantID && c.SourceEventID == sourceEventID {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockChargeRepo) Update(ctx context.Context, c *charge.Charge) error {
	m.updateCalls++
	m.charges[c.ID] = c
	return nil
}
func (m *mockChargeRepo) List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	m.listCalls++
	return nil, 0, nil
}
func (m *mockChargeRepo) ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) Search(ctx context.Context, tenantID string, filter charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) ListOverdue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	m.listOverdueCalls++
	m.overdueTenantID = tenantID
	return m.overdueCharges, nil
}
func (m *mockChargeRepo) BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status charge.Status) error {
	return nil
}
func (m *mockChargeRepo) ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListPendingPastDue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListByDealAndCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) AgingReport(ctx context.Context, tenantID string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (m *mockChargeRepo) GetDelinquencyStats(ctx context.Context, tenantID string) (*charge.DelinquencyStats, error) {
	return nil, nil
}

// seedCharge inserts a charge with the given status into the mock repo. Used
// by Cancel tests to set up fixtures without going through the Create flow.
func seedCharge(repo *mockChargeRepo, tenantID, id string, status charge.Status) *charge.Charge {
	c, _ := charge.New("c-1", "t-1", "o-1", "p-1", tenantID, charge.TypeRent, "Aluguel", "2024-01", 1000.0, time.Now().Add(24*time.Hour))
	c.ID = id
	c.Status = status
	repo.charges[id] = c
	return c
}

func setupCancelRouter(repo *mockChargeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewChargeService(repo, nil)
	handler := NewChargeHandler(svc)
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/charges/:id/cancel", handler.CancelCharge)
	return r
}

func TestChargeHandler_ListOverdue_UsesRepositoryOverdueSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pastDue, err := charge.New(
		"contract-past-due",
		"tenant-party",
		"owner",
		"property",
		"tenant-123",
		charge.TypeRent,
		"Aluguel vencido ainda pendente",
		"2026-06",
		1000,
		time.Now().Add(-48*time.Hour),
	)
	if err != nil {
		t.Fatalf("create pending past-due charge: %v", err)
	}
	alreadyOverdue, err := charge.New(
		"contract-overdue",
		"tenant-party",
		"owner",
		"property",
		"tenant-123",
		charge.TypeRent,
		"Aluguel marcado como vencido",
		"2026-05",
		900,
		time.Now().Add(-72*time.Hour),
	)
	if err != nil {
		t.Fatalf("create overdue charge: %v", err)
	}
	alreadyOverdue.Status = charge.StatusOverdue

	repo := &mockChargeRepo{
		charges:        make(map[string]*charge.Charge),
		overdueCharges: []*charge.Charge{alreadyOverdue, pastDue},
	}
	handler := NewChargeHandler(service.NewChargeService(repo, nil))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/charges/overdue", handler.ListOverdue)

	req := httptest.NewRequest(http.MethodGet, "/charges/overdue", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if repo.listOverdueCalls != 1 {
		t.Fatalf("expected ListOverdue to be called once, got %d", repo.listOverdueCalls)
	}
	if repo.listCalls != 0 {
		t.Fatalf("generic List must not define overdue semantics, got %d calls", repo.listCalls)
	}
	if repo.overdueTenantID != "tenant-123" {
		t.Fatalf("expected tenant-123, got %q", repo.overdueTenantID)
	}

	var body struct {
		Data []charge.Charge `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected both repository-defined overdue charges, got %d", len(body.Data))
	}
	if body.Data[0].ID != alreadyOverdue.ID || body.Data[1].ID != pastDue.ID {
		t.Fatalf("handler changed repository ordering: got %q, %q", body.Data[0].ID, body.Data[1].ID)
	}
}

func TestChargeHandler_Cancel_Success_Pending(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	seedCharge(repo, "tenant-123", "ch-pending", charge.StatusPending)
	r := setupCancelRouter(repo)

	body, _ := json.Marshal(CancelChargeRequest{Reason: "Erro de digitação no valor"})
	req, _ := http.NewRequest("POST", "/charges/ch-pending/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if got := repo.charges["ch-pending"].Status; got != charge.StatusCancelled {
		t.Fatalf("expected status CANCELLED, got %s", got)
	}
}

func TestChargeHandler_Cancel_Success_Overdue(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	seedCharge(repo, "tenant-123", "ch-overdue", charge.StatusOverdue)
	r := setupCancelRouter(repo)

	body, _ := json.Marshal(CancelChargeRequest{Reason: "Renegociação acordada"})
	req, _ := http.NewRequest("POST", "/charges/ch-overdue/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if got := repo.charges["ch-overdue"].Status; got != charge.StatusCancelled {
		t.Fatalf("expected status CANCELLED, got %s", got)
	}
}

func TestChargeHandler_Cancel_AlreadyPaid_Returns409(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	seedCharge(repo, "tenant-123", "ch-paid", charge.StatusPaid)
	r := setupCancelRouter(repo)

	body, _ := json.Marshal(CancelChargeRequest{Reason: "tentativa indevida"})
	req, _ := http.NewRequest("POST", "/charges/ch-paid/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for paid charge, got %d. Body: %s", w.Code, w.Body.String())
	}
	if got := repo.charges["ch-paid"].Status; got != charge.StatusPaid {
		t.Fatalf("status should remain PAID, got %s", got)
	}
}

func TestChargeHandler_Cancel_AlreadyCancelled_Returns409(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	seedCharge(repo, "tenant-123", "ch-cancelled", charge.StatusCancelled)
	r := setupCancelRouter(repo)

	body, _ := json.Marshal(CancelChargeRequest{Reason: "duplicado"})
	req, _ := http.NewRequest("POST", "/charges/ch-cancelled/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for cancelled charge, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestChargeHandler_Cancel_ReversedOrPartiallyPaid_Returns409(t *testing.T) {
	for _, status := range []charge.Status{charge.StatusReversed, charge.StatusPartiallyPaid} {
		t.Run(string(status), func(t *testing.T) {
			repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
			seedCharge(repo, "tenant-123", "ch-protected", status)
			r := setupCancelRouter(repo)

			body, _ := json.Marshal(CancelChargeRequest{Reason: "tentativa indevida"})
			req, _ := http.NewRequest("POST", "/charges/ch-protected/cancel", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusConflict {
				t.Fatalf("expected 409 for %s charge, got %d. Body: %s", status, w.Code, w.Body.String())
			}
			if got := repo.charges["ch-protected"].Status; got != status {
				t.Fatalf("status should remain %s, got %s", status, got)
			}
		})
	}
}

func TestChargeHandler_Cancel_MissingReason_Returns400(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	seedCharge(repo, "tenant-123", "ch-pending", charge.StatusPending)
	r := setupCancelRouter(repo)

	// binding:"required" cobre o campo ausente; o handler também recusa
	// reason só com whitespace (regra adicional sobre o binding).
	body := []byte(`{"reason":"   "}`)
	req, _ := http.NewRequest("POST", "/charges/ch-pending/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty reason, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestChargeHandler_Cancel_NotFound_Returns404(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	r := setupCancelRouter(repo)

	body, _ := json.Marshal(CancelChargeRequest{Reason: "qualquer"})
	req, _ := http.NewRequest("POST", "/charges/does-not-exist/cancel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing charge, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestChargeHandler_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	svc := service.NewChargeService(repo, nil)
	handler := NewChargeHandler(svc)

	r := gin.Default()
	// Mock tenant middleware
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/charges", handler.Create)

	reqBody := CreateChargeRequest{
		ContractID:     "c-1",
		TenantPartyID:  "t-1",
		OwnerID:        "o-1",
		PropertyID:     "p-1",
		Type:           charge.TypeRent,
		Description:    "Aluguel",
		ReferenceMonth: "2024-01",
		Amount:         1000.0,
		DueDate:        time.Now().Add(time.Hour * 24),
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/charges", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestChargeHandler_GetByID_NotFound exercises the sentinel-error mapping:
// when the service returns charge.ErrNotFound, the handler must respond 404
// regardless of any wrapping the service applies to the message. Guards
// against the previous string-literal comparison silently flipping to 500.
func TestChargeHandler_GetByID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	svc := service.NewChargeService(repo, nil)
	handler := NewChargeHandler(svc)

	r := gin.Default()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/charges/:id", handler.GetByID)

	req, _ := http.NewRequest("GET", "/charges/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

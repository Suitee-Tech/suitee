package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"context"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// mockRentalRepo is an in-memory rental.Repository for handler tests. Lookups
// are keyed off the same metadata fields the Postgres repo queries, so the
// handler's multi-key resolution (contract_id -> deal_id -> rental.id) is
// exercised exactly as in production.
type mockRentalRepo struct {
	rentals map[string]*rental.Rental // keyed by rental.ID
}

func newMockRentalRepo() *mockRentalRepo {
	return &mockRentalRepo{rentals: make(map[string]*rental.Rental)}
}

func (m *mockRentalRepo) Create(_ context.Context, r *rental.Rental) error {
	m.rentals[r.ID] = r
	return nil
}
func (m *mockRentalRepo) CreateInTx(_ context.Context, _ *sqlx.Tx, r *rental.Rental) error {
	m.rentals[r.ID] = r
	return nil
}
func (m *mockRentalRepo) GetByID(_ context.Context, tenantID, id string) (*rental.Rental, error) {
	if r, ok := m.rentals[id]; ok && r.TenantID == tenantID {
		return r, nil
	}
	return nil, nil
}
func (m *mockRentalRepo) GetByDealID(_ context.Context, tenantID, dealID string) (*rental.Rental, error) {
	for _, r := range m.rentals {
		if r.TenantID == tenantID && r.DealID == dealID {
			return r, nil
		}
	}
	return nil, nil
}
func (m *mockRentalRepo) GetByContractID(_ context.Context, tenantID, contractID string) (*rental.Rental, error) {
	for _, r := range m.rentals {
		if r.TenantID == tenantID && r.ContractID == contractID {
			return r, nil
		}
	}
	return nil, nil
}
func (m *mockRentalRepo) ListByLeadID(_ context.Context, _, _ string, _ rental.LeadRole) ([]*rental.Rental, error) {
	return nil, nil
}
func (m *mockRentalRepo) ListForLeadIDs(_ context.Context, _ string, _ []string) ([]*rental.Rental, error) {
	return nil, nil
}
func (m *mockRentalRepo) ListByPropertyID(_ context.Context, _, _ string) ([]*rental.Rental, error) {
	return nil, nil
}
func (m *mockRentalRepo) Update(_ context.Context, r *rental.Rental) error {
	m.rentals[r.ID] = r
	return nil
}

func seedRental(repo *mockRentalRepo) *rental.Rental {
	r, _ := rental.New(
		"tenant-123", "deal-def", "contract-xyz", "prop-1",
		"tenant-lead-1", "owner-lead-1", "broker-1",
		time.Now(), time.Now().Add(24*time.Hour), 320000,
	)
	r.ID = "rental-abc"
	repo.rentals[r.ID] = r
	return r
}

func setupRentalRouter(repo *mockRentalRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewRentalService(repo, nil)
	handler := NewRentalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/rentals/:id", handler.GetByID)
	return r
}

// TestRentalHandler_GetByID_Resolution locks in that /rentals/:id resolves a
// rental by contract_id (the key the admin UI sends via /locacao/{contract.id}),
// as well as by deal_id and the rental's own id, and 404s otherwise.
func TestRentalHandler_GetByID_Resolution(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		wantCode int
	}{
		{"by contract_id", "contract-xyz", http.StatusOK},
		{"by deal_id", "deal-def", http.StatusOK},
		{"by rental id", "rental-abc", http.StatusOK},
		{"unknown id 404s", "does-not-exist", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRentalRepo()
			seedRental(repo)
			r := setupRentalRouter(repo)

			req, _ := http.NewRequest(http.MethodGet, "/rentals/"+tc.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantCode {
				t.Fatalf("GET /rentals/%s: expected %d, got %d. Body: %s", tc.id, tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

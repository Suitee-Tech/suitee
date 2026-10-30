package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// stubLeadRepo lets each test control which lead IDs the repo "knows" within
// a tenant. ANY id not in `existing` returns false.
type stubLeadRepo struct {
	existing map[string]map[string]bool // tenantID -> id -> true
	err      error                      // if set, Exists returns this error
	// leadTenantsByAccount alimenta ClientLeadTenants (imobiliárias em que o
	// cliente tem atendimento) — usado pelos testes da lista de canais do portal.
	leadTenantsByAccount map[string][]string
}

func (s *stubLeadRepo) GetInterest(ctx context.Context, tenantID, id string) (interface{}, error) {
	return nil, nil
}

func (s *stubLeadRepo) ClientLeadTenants(ctx context.Context, accountID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.leadTenantsByAccount[accountID], nil
}

func (s *stubLeadRepo) Exists(ctx context.Context, tenantID, id string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if s.existing == nil {
		return false, nil
	}
	tenantBucket, ok := s.existing[tenantID]
	if !ok {
		return false, nil
	}
	return tenantBucket[id], nil
}

// stubPropertyRepo returns a property summary only for the configured
// (tenantID, propertyID) pair; otherwise returns "not found".
type stubPropertyRepo struct {
	tenantID   string
	propertyID string
}

func (p *stubPropertyRepo) GetSummaryByID(ctx context.Context, tenantID, id string, dealType deal.Type) (*portal.PropertySummary, error) {
	if tenantID == p.tenantID && id == p.propertyID {
		return &portal.PropertySummary{ID: id, Title: "ok"}, nil
	}
	return nil, errors.New("property not found")
}

func (p *stubPropertyRepo) ListRecommendations(ctx context.Context, tenantID string, dealType deal.Type, excludePropertyID string, interest interface{}, limit int64) ([]*portal.PropertySummary, error) {
	return nil, nil
}

func (p *stubPropertyRepo) Search(ctx context.Context, tenantID string, dealType deal.Type, params repository.SearchParams) ([]*portal.PropertySummary, error) {
	return nil, nil
}

// TestCreate_RejectsForeignProperty ensures we cannot create a deal that
// references a property of another tenant — the gate uses propertyRepo's
// tenant-scoped lookup which returns "not found" for a foreign id.
func TestCreate_RejectsForeignProperty(t *testing.T) {
	tenantA := "tenant-A"
	tenantB := "tenant-B"

	propRepo := &stubPropertyRepo{tenantID: tenantA, propertyID: "prop-A"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		tenantB: {"owner-B": true, "buyer-B": true},
	}}

	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)

	// tenant B tries to create a deal pointing to prop-A (lives in tenant A).
	_, err := svc.Create(
		context.Background(),
		deal.TypeSale,
		"prop-A", "owner-B", "buyer-B", "broker-1", "",
		100000, tenantB,
		"", "", "", "",
		service.PartyProfileHints{},
		false,
	)
	if err == nil {
		t.Fatal("expected error rejecting foreign property_id, got nil")
	}
	if !strings.Contains(err.Error(), "invalid property_id") {
		t.Errorf("expected error to mention invalid property_id, got %q", err.Error())
	}
}

// TestCreate_RejectsForeignLead ensures we cannot create a deal that
// references an owner/customer/lead id that doesn't exist in the calling
// tenant — even if it exists in another tenant.
func TestCreate_RejectsForeignLead(t *testing.T) {
	tenantA := "tenant-A"
	tenantB := "tenant-B"

	propRepo := &stubPropertyRepo{tenantID: tenantB, propertyID: "prop-B"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		tenantA: {"owner-A": true},
		tenantB: {"buyer-B": true}, // owner-A is intentionally absent for tenant B
	}}

	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)

	// tenant B tries to use owner-A (lives only in tenant A).
	_, err := svc.Create(
		context.Background(),
		deal.TypeSale,
		"prop-B", "owner-A", "buyer-B", "broker-1", "",
		100000, tenantB,
		"", "", "", "",
		service.PartyProfileHints{},
		false,
	)
	if err == nil {
		t.Fatal("expected error rejecting foreign owner_id, got nil")
	}
	if !strings.Contains(err.Error(), "invalid owner_id") {
		t.Errorf("expected error to mention invalid owner_id, got %q", err.Error())
	}
}

// TestCreate_AcceptsValidIDs is the happy-path control — confirms the gate
// doesn't over-block when everything is in the same tenant.
func TestCreate_AcceptsValidIDs(t *testing.T) {
	tenantA := "tenant-A"
	propRepo := &stubPropertyRepo{tenantID: tenantA, propertyID: "prop-A"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		tenantA: {"owner-A": true, "buyer-A": true},
	}}

	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)

	d, err := svc.Create(
		context.Background(),
		deal.TypeSale,
		"prop-A", "owner-A", "buyer-A", "broker-1", "",
		100000, tenantA,
		"", "", "", "",
		service.PartyProfileHints{},
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error on valid ids: %v", err)
	}
	if d == nil {
		t.Fatal("expected a deal, got nil")
	}
	if d.TenantID != tenantA {
		t.Errorf("expected tenant %s, got %s", tenantA, d.TenantID)
	}
}

// TestCreate_NilReposSkipValidation preserves the test-friendly behavior of
// passing nil repos (existing tests rely on this); we only validate when
// repos are wired.
func TestCreate_NilReposSkipValidation(t *testing.T) {
	svc := service.NewDealService(&mockDealRepo{}, nil, nil, &mockEventBus{}, nil)
	_, err := svc.Create(
		context.Background(),
		deal.TypeSale,
		"prop-X", "owner-X", "buyer-X", "broker-1", "",
		100000, "tenant-X",
		"", "", "", "",
		service.PartyProfileHints{},
		false,
	)
	if err != nil {
		t.Fatalf("expected nil-repo path to skip validation, got: %v", err)
	}
}

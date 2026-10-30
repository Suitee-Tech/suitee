package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

type canonicalSalePlanRepo struct {
	mu sync.Mutex

	canonical       *sale_billing_plan.SaleBillingPlan
	createCalls     int
	candidateIDs    []string
	lookupBarrier   int
	lookupArrivals  int
	lookupsReleased chan struct{}
}

func newCanonicalSalePlanRepo(lookupBarrier int) *canonicalSalePlanRepo {
	r := &canonicalSalePlanRepo{lookupBarrier: lookupBarrier}
	if lookupBarrier > 0 {
		r.lookupsReleased = make(chan struct{})
	}
	return r
}

func cloneSaleBillingPlan(plan *sale_billing_plan.SaleBillingPlan) *sale_billing_plan.SaleBillingPlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	if plan.SaldoDueDate != nil {
		saldoDueDate := *plan.SaldoDueDate
		copy.SaldoDueDate = &saldoDueDate
	}
	return &copy
}

func (r *canonicalSalePlanRepo) Create(_ context.Context, candidate *sale_billing_plan.SaleBillingPlan) (*sale_billing_plan.SaleBillingPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.createCalls++
	r.candidateIDs = append(r.candidateIDs, candidate.ID)
	if r.canonical == nil {
		r.canonical = cloneSaleBillingPlan(candidate)
	}
	return cloneSaleBillingPlan(r.canonical), nil
}

func (r *canonicalSalePlanRepo) GetByID(_ context.Context, tenantID, id string) (*sale_billing_plan.SaleBillingPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canonical != nil && r.canonical.TenantID == tenantID && r.canonical.ID == id {
		return cloneSaleBillingPlan(r.canonical), nil
	}
	return nil, nil
}

func (r *canonicalSalePlanRepo) GetByContractID(ctx context.Context, tenantID, contractID string) (*sale_billing_plan.SaleBillingPlan, error) {
	r.mu.Lock()
	if r.canonical != nil {
		canonical := cloneSaleBillingPlan(r.canonical)
		r.mu.Unlock()
		if canonical.TenantID == tenantID && canonical.ContractID == contractID {
			return canonical, nil
		}
		return nil, nil
	}
	if r.lookupBarrier == 0 {
		r.mu.Unlock()
		return nil, nil
	}

	r.lookupArrivals++
	if r.lookupArrivals == r.lookupBarrier {
		close(r.lookupsReleased)
	}
	released := r.lookupsReleased
	r.mu.Unlock()

	select {
	case <-released:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *canonicalSalePlanRepo) Update(_ context.Context, plan *sale_billing_plan.SaleBillingPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canonical = cloneSaleBillingPlan(plan)
	return nil
}

func (r *canonicalSalePlanRepo) Delete(_ context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canonical != nil && r.canonical.TenantID == tenantID && r.canonical.ID == id {
		r.canonical = nil
	}
	return nil
}

func (r *canonicalSalePlanRepo) stats() (int, []string, *sale_billing_plan.SaleBillingPlan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createCalls, append([]string(nil), r.candidateIDs...), cloneSaleBillingPlan(r.canonical)
}

type concurrentSaleChargeRepo struct {
	mu       sync.Mutex
	bySource map[string]*charge.Charge
	failOn   string
	failLeft int
}

func newConcurrentSaleChargeRepo() *concurrentSaleChargeRepo {
	return &concurrentSaleChargeRepo{
		bySource: make(map[string]*charge.Charge),
	}
}

func (r *concurrentSaleChargeRepo) Create(_ context.Context, candidate *charge.Charge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn != "" && candidate.SourceEventID == r.failOn && r.failLeft > 0 {
		r.failLeft--
		return errors.New("injected sale charge create failure")
	}
	if _, exists := r.bySource[candidate.SourceEventID]; exists {
		return charge.ErrChargeAlreadyExists
	}
	r.bySource[candidate.SourceEventID] = candidate
	return nil
}

func (r *concurrentSaleChargeRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*charge.Charge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.bySource[sourceEventID]
	if existing != nil && existing.TenantID == tenantID {
		return existing, nil
	}
	return nil, nil
}

func (r *concurrentSaleChargeRepo) GetByID(_ context.Context, tenantID, id string) (*charge.Charge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.bySource {
		if existing.TenantID == tenantID && existing.ID == id {
			return existing, nil
		}
	}
	return nil, nil
}

func (r *concurrentSaleChargeRepo) Update(_ context.Context, candidate *charge.Charge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySource[candidate.SourceEventID] = candidate
	return nil
}

func (r *concurrentSaleChargeRepo) List(_ context.Context, _, _, _, _ string, _, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func (r *concurrentSaleChargeRepo) ListByPartyIDs(_ context.Context, _ string, _ []string, _, _ string, _, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func (r *concurrentSaleChargeRepo) ListByLeadIDs(_ context.Context, _ string, _ []string, _, _ string, _, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func (r *concurrentSaleChargeRepo) Search(_ context.Context, _ string, _ charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func (r *concurrentSaleChargeRepo) ListOverdue(_ context.Context, _ string) ([]*charge.Charge, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) BulkUpdateStatus(_ context.Context, _ string, _ []string, _ charge.Status) error {
	return nil
}

func (r *concurrentSaleChargeRepo) ListByIDs(_ context.Context, _ string, _ []string) ([]*charge.Charge, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) ListPendingPastDue(_ context.Context, _ string) ([]*charge.Charge, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) ListDistinctTenantIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) ListByDealAndCustomer(_ context.Context, _, _, _, _ string) ([]*charge.Charge, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) AgingReport(_ context.Context, _ string) ([]charge.AgingBucket, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) GetDelinquencyStats(_ context.Context, _ string) (*charge.DelinquencyStats, error) {
	return nil, nil
}

func (r *concurrentSaleChargeRepo) charges() map[string]*charge.Charge {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]*charge.Charge, len(r.bySource))
	for sourceEventID, ch := range r.bySource {
		copy[sourceEventID] = ch
	}
	return copy
}

func createSingleSalePlanForConcurrencyTest(ctx context.Context, service *SaleBillingPlanService) (*sale_billing_plan.SaleBillingPlan, int, error) {
	return service.CreateAndGenerateCharges(
		ctx,
		"tenant-1",
		"contract-1",
		"deal-1",
		"buyer-1",
		"owner-1",
		"property-1",
		sale_billing_plan.ModeSingle,
		100000,
		0,
		100000,
		1,
		time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC),
		30,
		sale_billing_plan.CompositeInput{},
	)
}

func TestSaleBillingPlanConcurrentDeliveriesUseCanonicalPlanForCharges(t *testing.T) {
	planRepo := newCanonicalSalePlanRepo(2)
	chargeRepo := newConcurrentSaleChargeRepo()
	service := NewSaleBillingPlanService(planRepo, NewChargeService(chargeRepo, nil))

	type result struct {
		plan    *sale_billing_plan.SaleBillingPlan
		created int
		err     error
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		go func() {
			plan, created, err := createSingleSalePlanForConcurrencyTest(ctx, service)
			results <- result{plan: plan, created: created, err: err}
		}()
	}

	var got [2]result
	for i := range got {
		select {
		case got[i] = <-results:
		case <-ctx.Done():
			t.Fatalf("concurrent materialization timed out: %v", ctx.Err())
		}
		if got[i].err != nil {
			t.Fatalf("concurrent materialization failed: %v", got[i].err)
		}
		if got[i].plan == nil {
			t.Fatal("concurrent materialization returned nil plan")
		}
	}

	if got[0].plan.ID != got[1].plan.ID {
		t.Fatalf("deliveries returned different plans: %q != %q", got[0].plan.ID, got[1].plan.ID)
	}
	if got[0].created+got[1].created != 1 {
		t.Fatalf("created charge count = %d + %d, want one canonical charge", got[0].created, got[1].created)
	}

	createCalls, candidateIDs, canonical := planRepo.stats()
	if createCalls != 2 {
		t.Fatalf("repository Create calls = %d, want both racing candidates", createCalls)
	}
	if len(candidateIDs) != 2 || candidateIDs[0] == candidateIDs[1] {
		t.Fatalf("expected distinct transient plan IDs, got %v", candidateIDs)
	}
	if canonical == nil || got[0].plan.ID != canonical.ID {
		t.Fatalf("service did not return repository winner: got=%q canonical=%v", got[0].plan.ID, canonical)
	}

	charges := chargeRepo.charges()
	wantSourceEventID := fmt.Sprintf("sale.%s.installment.1", canonical.ID)
	if len(charges) != 1 {
		t.Fatalf("persisted charges = %d, want 1: %#v", len(charges), charges)
	}
	createdCharge := charges[wantSourceEventID]
	if createdCharge == nil {
		t.Fatalf("charge source must derive from canonical plan %q; got %#v", wantSourceEventID, charges)
	}
	if createdCharge.ContractID != canonical.ContractID || createdCharge.TenantID != canonical.TenantID {
		t.Fatalf("charge scope differs from canonical plan: charge=%s/%s plan=%s/%s", createdCharge.TenantID, createdCharge.ContractID, canonical.TenantID, canonical.ContractID)
	}
}

func TestSaleBillingPlanRetryReturnsCanonicalPlanWithoutDuplicateCharges(t *testing.T) {
	planRepo := newCanonicalSalePlanRepo(0)
	chargeRepo := newConcurrentSaleChargeRepo()
	service := NewSaleBillingPlanService(planRepo, NewChargeService(chargeRepo, nil))

	first, firstCreated, err := createSingleSalePlanForConcurrencyTest(context.Background(), service)
	if err != nil {
		t.Fatalf("first materialization failed: %v", err)
	}
	second, secondCreated, err := createSingleSalePlanForConcurrencyTest(context.Background(), service)
	if err != nil {
		t.Fatalf("retry materialization failed: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("retry returned a different plan: first=%q second=%q", first.ID, second.ID)
	}
	if firstCreated != 1 || secondCreated != 0 {
		t.Fatalf("created counts = first:%d retry:%d, want 1/0", firstCreated, secondCreated)
	}
	createCalls, _, canonical := planRepo.stats()
	if createCalls != 1 {
		t.Fatalf("repository Create calls = %d, want 1 after retry lookup", createCalls)
	}
	wantSourceEventID := fmt.Sprintf("sale.%s.installment.1", canonical.ID)
	charges := chargeRepo.charges()
	if len(charges) != 1 || charges[wantSourceEventID] == nil {
		t.Fatalf("retry duplicated or changed canonical charge: %#v", charges)
	}
}

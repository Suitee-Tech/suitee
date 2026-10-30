package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// trackingContractRepo stores contracts in a map keyed by ID so we can test
// flows that create or look up multiple contracts (e.g. AmendContract).
// Implements contract.Repository. Not thread-safe (tests are sequential).
type trackingContractRepo struct {
	items map[string]*contract.Contract
}

func newTrackingContractRepo(initial ...*contract.Contract) *trackingContractRepo {
	r := &trackingContractRepo{items: make(map[string]*contract.Contract)}
	for _, c := range initial {
		if c != nil {
			r.items[c.ID] = c
		}
	}
	return r
}

func (m *trackingContractRepo) Create(ctx context.Context, c *contract.Contract) error {
	m.items[c.ID] = c
	return nil
}
func (m *trackingContractRepo) GetByID(ctx context.Context, tenantID, id string) (*contract.Contract, error) {
	return m.items[id], nil
}
func (m *trackingContractRepo) GetByDealID(ctx context.Context, tenantID, dealID string) ([]*contract.Contract, error) {
	out := []*contract.Contract{}
	for _, c := range m.items {
		if c.DealID == dealID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *trackingContractRepo) Update(ctx context.Context, c *contract.Contract) error {
	m.items[c.ID] = c
	return nil
}
func (m *trackingContractRepo) UpdateInTx(ctx context.Context, _ *sqlx.Tx, c *contract.Contract) error {
	return m.Update(ctx, c)
}
func (m *trackingContractRepo) Delete(ctx context.Context, id, tenantID string) error {
	delete(m.items, id)
	return nil
}
func (m *trackingContractRepo) List(ctx context.Context, tenantID string, status *contract.Status) ([]*contract.Contract, error) {
	return nil, nil
}
func (m *trackingContractRepo) ListByPropertyID(ctx context.Context, propertyID string) ([]*contract.Contract, error) {
	return nil, nil
}
func (m *trackingContractRepo) ListAutoAdjustDue(ctx context.Context, today time.Time) ([]*contract.Contract, error) {
	out := []*contract.Contract{}
	dateStr := today.Format("2006-01-02")
	for _, c := range m.items {
		if !c.AutoAdjust || c.NextAdjustment == "" {
			continue
		}
		if c.NextAdjustment > dateStr {
			continue
		}
		if c.Status != contract.StatusSigned && c.Status != contract.StatusActive {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (m *trackingContractRepo) ListMissingCRMSnapshot(ctx context.Context, tenantID string) ([]*contract.Contract, error) {
	return nil, nil
}

// capturingEventBus records every event published — both Publish and
// PublishReliable land in `events` so tests can assert what was emitted.
type capturingEventBus struct {
	mu     sync.Mutex
	events []events.Event
}

func (b *capturingEventBus) Publish(ctx context.Context, event events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}
func (b *capturingEventBus) PublishReliable(ctx context.Context, event events.Event) error {
	return b.Publish(ctx, event)
}
func (b *capturingEventBus) Subscribe(topic string, handler events.EventHandler) error { return nil }
func (b *capturingEventBus) SubscribeWithQueue(_, _ string, _ events.EventHandler) error {
	return nil
}

func (b *capturingEventBus) eventsByName(name string) []events.Event {
	out := []events.Event{}
	for _, e := range b.events {
		if e.Name() == name {
			out = append(out, e)
		}
	}
	return out
}

type stubExitChecker struct {
	ok  bool
	err error
}

func (s *stubExitChecker) HasCompletedExitInspection(_ context.Context, _, _ string) (bool, error) {
	return s.ok, s.err
}

// buildActiveContract returns a SIGNED contract ready for lifecycle action
// tests. Caller can mutate the returned pointer before persisting.
func buildActiveContract(t *testing.T) *contract.Contract {
	t.Helper()
	c := &contract.Contract{
		ID:        "contract-1",
		DealID:    "deal-1",
		TenantID:  "tenant-1",
		Status:    contract.StatusSigned,
		Type:      contract.TypeLocacao,
		StartDate: time.Now().AddDate(-1, 0, 0),
		Value:     1500.00,
		Signatories: []contract.Signatory{
			{ID: "sig-1", Name: "Inquilino", Email: "in@example.com", Role: contract.RoleLocatario},
			{ID: "sig-2", Name: "Locador", Email: "lo@example.com", Role: contract.RoleLocador},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	end := time.Now().AddDate(1, 0, 0)
	c.EndDate = &end
	return c
}

func TestTerminateContract_HappyPath(t *testing.T) {
	c := buildActiveContract(t)
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1500, "tenant-1")
	dealRepo := &mockDealRepo{d: d}

	svc := service.NewContractService(repo, dealRepo, bus, nil)
	updated, err := svc.TerminateContract(context.Background(), "tenant-1", c.ID, service.TerminateContractRequest{
		Reason:                    "rescisão acordada",
		EarlyTerminationFee:       500,
		SecurityDepositReturn:     1500,
		SecurityDepositDeductions: 200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != contract.StatusTerminated {
		t.Fatalf("status = %q; want TERMINATED", updated.Status)
	}
	if updated.Termination == nil {
		t.Fatal("Termination block missing")
	}
	if updated.Termination.Reason != "rescisão acordada" {
		t.Errorf("Termination.Reason = %q; want rescisão acordada", updated.Termination.Reason)
	}
	if updated.Termination.EarlyTerminationFee != 500 {
		t.Errorf("EarlyTerminationFee = %v; want 500", updated.Termination.EarlyTerminationFee)
	}
	if updated.Termination.SecurityDepositDeductions != 200 {
		t.Errorf("SecurityDepositDeductions = %v; want 200", updated.Termination.SecurityDepositDeductions)
	}
	evs := bus.eventsByName("contract.terminated")
	if len(evs) != 1 {
		t.Fatalf("got %d contract.terminated events; want 1", len(evs))
	}
}

func TestTerminateContract_NotFound(t *testing.T) {
	repo := newTrackingContractRepo()
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.TerminateContract(context.Background(), "tenant-1", "missing", service.TerminateContractRequest{Reason: "x"})
	if err == nil || err.Error() != "contract not found" {
		t.Fatalf("err = %v; want contract not found", err)
	}
}

func TestTerminateContract_AlreadyTerminated(t *testing.T) {
	c := buildActiveContract(t)
	c.Status = contract.StatusTerminated
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.TerminateContract(context.Background(), "tenant-1", c.ID, service.TerminateContractRequest{Reason: "x"})
	if err == nil || err.Error() != "contract already terminated" {
		t.Fatalf("err = %v; want contract already terminated", err)
	}
}

func TestTerminateContract_ExitInspectionGate(t *testing.T) {
	c := buildActiveContract(t)
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	svc.WithExitInspectionChecker(&stubExitChecker{ok: false})

	_, err := svc.TerminateContract(context.Background(), "tenant-1", c.ID, service.TerminateContractRequest{
		Reason:                "fim natural",
		RequireExitInspection: true,
	})
	if !errors.Is(err, service.ErrExitInspectionRequired) {
		t.Fatalf("err = %v; want ErrExitInspectionRequired", err)
	}

	// Now flip checker to ok=true — should proceed.
	svc.WithExitInspectionChecker(&stubExitChecker{ok: true})
	if _, err := svc.TerminateContract(context.Background(), "tenant-1", c.ID, service.TerminateContractRequest{
		Reason:                "fim natural",
		RequireExitInspection: true,
	}); err != nil {
		t.Fatalf("unexpected error after checker passed: %v", err)
	}
}

func TestReadjustContract_ByIndex(t *testing.T) {
	c := buildActiveContract(t)
	c.Value = 1000
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	svc := service.NewContractService(repo, &mockDealRepo{}, bus, nil)

	updated, err := svc.ReadjustContract(context.Background(), "tenant-1", c.ID, service.ReadjustContractRequest{
		IndexName:      "IGPM",
		IndexValue:     5, // 5%
		EffectiveFrom:  time.Now(),
		NextAdjustment: "2027-05-01",
		Mode:           "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := updated.Value; got != 1050 {
		t.Errorf("Value = %v; want 1050", got)
	}
	if len(updated.Adjustments) != 1 {
		t.Fatalf("Adjustments len = %d; want 1", len(updated.Adjustments))
	}
	entry := updated.Adjustments[0]
	if entry.PreviousValue != 1000 || entry.NewValue != 1050 {
		t.Errorf("Adjustment values: prev=%v new=%v; want 1000→1050", entry.PreviousValue, entry.NewValue)
	}
	if entry.Mode != "manual" {
		t.Errorf("Adjustment.Mode = %q; want manual", entry.Mode)
	}
	if updated.NextAdjustment != "2027-05-01" {
		t.Errorf("NextAdjustment = %q; want 2027-05-01", updated.NextAdjustment)
	}
	evs := bus.eventsByName("contract.readjusted")
	if len(evs) != 1 {
		t.Fatalf("got %d contract.readjusted events; want 1", len(evs))
	}
}

func TestReadjustContract_ByValue(t *testing.T) {
	c := buildActiveContract(t)
	c.Value = 1000
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)

	updated, err := svc.ReadjustContract(context.Background(), "tenant-1", c.ID, service.ReadjustContractRequest{
		NewValue: 1200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Value != 1200 {
		t.Errorf("Value = %v; want 1200", updated.Value)
	}
}

func TestReadjustContract_RequiresValueOrIndex(t *testing.T) {
	c := buildActiveContract(t)
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.ReadjustContract(context.Background(), "tenant-1", c.ID, service.ReadjustContractRequest{})
	if err == nil || err.Error() != "readjust requires new_value or index_value" {
		t.Fatalf("err = %v; want validation error", err)
	}
}

func TestReadjustContract_AutoModeIsRecorded(t *testing.T) {
	c := buildActiveContract(t)
	c.Value = 2000
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)

	updated, err := svc.ReadjustContract(context.Background(), "tenant-1", c.ID, service.ReadjustContractRequest{
		IndexName:  "IPCA",
		IndexValue: 3,
		Mode:       "auto",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Adjustments[0].Mode != "auto" {
		t.Errorf("Mode = %q; want auto", updated.Adjustments[0].Mode)
	}
	if updated.Adjustments[0].TriggeredBy != "scheduler" {
		t.Errorf("TriggeredBy = %q; want scheduler (auto mode falls back from system)", updated.Adjustments[0].TriggeredBy)
	}
}

func TestRunAutoReadjustments(t *testing.T) {
	today := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	dueContract := buildActiveContract(t)
	dueContract.ID = "due"
	dueContract.AutoAdjust = true
	dueContract.NextAdjustment = "2026-05-01" // past — due
	dueContract.AdjustIndex = "IGPM"
	dueContract.Value = 1000

	notDue := buildActiveContract(t)
	notDue.ID = "notdue"
	notDue.AutoAdjust = true
	notDue.NextAdjustment = "2027-01-01" // future — skip

	manualOnly := buildActiveContract(t)
	manualOnly.ID = "manual"
	manualOnly.AutoAdjust = false // skip
	manualOnly.NextAdjustment = "2026-05-01"

	repo := newTrackingContractRepo(dueContract, notDue, manualOnly)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)

	count, err := svc.RunAutoReadjustments(context.Background(), today, 4.5) // 4.5%
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d; want 1 (only the due contract)", count)
	}
	got, _ := repo.GetByID(context.Background(), "tenant-1", "due")
	if got.Value <= 1000 {
		t.Errorf("due contract value = %v; want > 1000 after readjustment", got.Value)
	}
	// NextAdjustment rolled forward by one year.
	if got.NextAdjustment != "2027-05-01" {
		t.Errorf("NextAdjustment = %q; want 2027-05-01", got.NextAdjustment)
	}
}

func TestRunAutoReadjustments_NoIndexConfigured(t *testing.T) {
	today := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	c := buildActiveContract(t)
	c.ID = "due"
	c.AutoAdjust = true
	c.NextAdjustment = "2026-05-01"
	c.Value = 1000

	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	count, err := svc.RunAutoReadjustments(context.Background(), today, 0) // unset
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d; want 0 when defaultIndexValue is zero", count)
	}
}

func TestAmendContract_InheritsFromOriginal(t *testing.T) {
	c := buildActiveContract(t)
	c.Value = 1500
	c.SecurityDeposit = 3000
	c.AdjustIndex = "IGPM"
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	svc := service.NewContractService(repo, &mockDealRepo{}, bus, nil)

	amendment, err := svc.AmendContract(context.Background(), "tenant-1", c.ID, service.AmendContractRequest{
		Reason: "extensão de prazo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amendment.Status != contract.StatusDraft {
		t.Errorf("Status = %q; want DRAFT", amendment.Status)
	}
	if amendment.AmendsContractID != c.ID {
		t.Errorf("AmendsContractID = %q; want %q", amendment.AmendsContractID, c.ID)
	}
	if amendment.Value != 1500 || amendment.SecurityDeposit != 3000 || amendment.AdjustIndex != "IGPM" {
		t.Errorf("amendment did not inherit unchanged fields: %+v", amendment)
	}
	if len(amendment.Signatories) != len(c.Signatories) {
		t.Errorf("Signatories cloned len = %d; want %d", len(amendment.Signatories), len(c.Signatories))
	}
	for _, s := range amendment.Signatories {
		if s.Status == contract.SignatoryStatusSigned {
			t.Error("cloned signatory should not carry signed status from original")
		}
	}

	original, _ := repo.GetByID(context.Background(), "tenant-1", c.ID)
	if len(original.Amendments) != 1 || original.Amendments[0].NewContractID != amendment.ID {
		t.Errorf("original.Amendments not updated correctly: %+v", original.Amendments)
	}

	evs := bus.eventsByName("contract.amended")
	if len(evs) != 1 {
		t.Fatalf("got %d contract.amended events; want 1", len(evs))
	}
}

func TestAmendContract_OverridesFromRequest(t *testing.T) {
	c := buildActiveContract(t)
	c.Value = 1500
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)

	auto := true
	amendment, err := svc.AmendContract(context.Background(), "tenant-1", c.ID, service.AmendContractRequest{
		Reason:     "reajuste e extensão",
		Value:      2000,
		AutoAdjust: &auto,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amendment.Value != 2000 {
		t.Errorf("Value = %v; want 2000", amendment.Value)
	}
	if !amendment.AutoAdjust {
		t.Errorf("AutoAdjust = false; want true")
	}
}

func TestAmendContract_RequiresActiveOriginal(t *testing.T) {
	c := buildActiveContract(t)
	c.Status = contract.StatusDraft
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.AmendContract(context.Background(), "tenant-1", c.ID, service.AmendContractRequest{Reason: "x"})
	if err == nil {
		t.Fatal("expected error amending DRAFT contract")
	}
}

func TestAmendContract_RequiresReason(t *testing.T) {
	c := buildActiveContract(t)
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.AmendContract(context.Background(), "tenant-1", c.ID, service.AmendContractRequest{})
	if err == nil || err.Error() != "amendment reason is required" {
		t.Fatalf("err = %v; want amendment reason is required", err)
	}
}

func TestAmendContract_CannotAmendAnAmendment(t *testing.T) {
	c := buildActiveContract(t)
	c.AmendsContractID = "another"
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	_, err := svc.AmendContract(context.Background(), "tenant-1", c.ID, service.AmendContractRequest{Reason: "x"})
	if err == nil {
		t.Fatal("expected error amending an amendment")
	}
}

func TestSupersedeOriginalOnAmendmentSigned(t *testing.T) {
	original := buildActiveContract(t)
	original.ID = "orig-1"
	original.Status = contract.StatusActive
	original.Amendments = []contract.AmendmentEntry{
		{NewContractID: "amend-1", CreatedAt: time.Now(), Reason: "x"},
	}
	amendment := buildActiveContract(t)
	amendment.ID = "amend-1"
	amendment.AmendsContractID = "orig-1"
	amendment.Status = contract.StatusSigned

	repo := newTrackingContractRepo(original, amendment)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)

	if err := svc.SupersedeOriginalOnAmendmentSigned(context.Background(), amendment); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := repo.GetByID(context.Background(), "tenant-1", "orig-1")
	if got.Status != contract.StatusExpired {
		t.Errorf("original status = %q; want EXPIRED", got.Status)
	}
	if got.Amendments[0].SupersededAt == nil {
		t.Error("Amendments[0].SupersededAt should be set")
	}
}

func TestSupersedeOriginalOnAmendmentSigned_NoopForNonAmendment(t *testing.T) {
	c := buildActiveContract(t)
	repo := newTrackingContractRepo(c)
	svc := service.NewContractService(repo, &mockDealRepo{}, &capturingEventBus{}, nil)
	// signed contract with no AmendsContractID — should be a no-op.
	if err := svc.SupersedeOriginalOnAmendmentSigned(context.Background(), c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func (m *trackingContractRepo) UpdateIfUnchanged(ctx context.Context, c *contract.Contract, _ time.Time) error {
	return m.Update(ctx, c)
}

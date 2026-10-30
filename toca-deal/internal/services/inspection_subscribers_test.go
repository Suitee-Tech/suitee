package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
	"github.com/stretchr/testify/assert"
)

func makeContractForInspectionTest() *contract.Contract {
	return &contract.Contract{
		ID:        "contract-7",
		DealID:    "deal-7",
		TenantID:  "tenant-1",
		Status:    contract.StatusSigned,
		Type:      contract.TypeLocacao,
		StartDate: time.Now(),
		Value:     2000,
		Signatories: []contract.Signatory{
			{ID: "s1", Name: "Inq", Email: "a@b.c", Role: contract.RoleLocatario},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestInspectionCompletedHandler_EntryPinsInspectionID(t *testing.T) {
	c := makeContractForInspectionTest()
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2000, "tenant-1")
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil)
	handler := service.NewInspectionCompletedHandler(svc)

	evt := contracts.InspectionCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID:  "insp-1",
			TenantID:      "tenant-1",
			ContractID:    "contract-7",
			Type:          "ENTRY",
			ReportURL:     "https://docs.example/laudo-entrada.pdf",
			CompletedAt:   time.Now().UTC().Format(time.RFC3339),
			FindingsCount: 5,
			HasDamage:     false,
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	updated, _ := repo.GetByID(context.Background(), "tenant-1", "contract-7")
	assert.NotNil(t, updated)
	assert.Equal(t, "insp-1", updated.EntryInspectionID)
}

func TestInspectionCompletedHandler_ExitIsAuditOnly(t *testing.T) {
	c := makeContractForInspectionTest()
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2000, "tenant-1")
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil)
	handler := service.NewInspectionCompletedHandler(svc)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-exit-1",
			TenantID:     "tenant-1",
			ContractID:   "contract-7",
			Type:         "EXIT",
			ReportURL:    "https://docs.example/laudo-saida.pdf",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	updated, _ := repo.GetByID(context.Background(), "tenant-1", "contract-7")
	// EXIT does not pin anything on the contract — the gate is HasCompletedExitInspection
	// queried by TerminateContract via toca-ops.
	assert.Empty(t, updated.EntryInspectionID, "EXIT events must not write EntryInspectionID")
	assert.Equal(t, contract.StatusSigned, updated.Status, "EXIT events must not transition the contract")
}

func TestInspectionCompletedHandler_Idempotent(t *testing.T) {
	c := makeContractForInspectionTest()
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2000, "tenant-1")
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil)
	handler := service.NewInspectionCompletedHandler(svc)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-1",
			TenantID:     "tenant-1",
			ContractID:   "contract-7",
			Type:         "ENTRY",
			ReportURL:    "https://docs.example/laudo.pdf",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.NoError(t, handler(context.Background(), evt))

	updated, _ := repo.GetByID(context.Background(), "tenant-1", "contract-7")
	assert.Equal(t, "insp-1", updated.EntryInspectionID)
}

func TestInspectionCompletedHandler_NoContractAck(t *testing.T) {
	// Inspections without contract_id (e.g. ad-hoc property inspections)
	// should ack and skip — not error out the broker.
	repo := newTrackingContractRepo()
	bus := &capturingEventBus{}
	svc := service.NewContractService(repo, &mockDealRepo{}, bus, nil)
	handler := service.NewInspectionCompletedHandler(svc)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-loose",
			TenantID:     "tenant-1",
			Type:         "PERIODIC",
			// ContractID intentionally empty
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
}

func TestInspectionCompletedHandler_AcceptsMapPayload(t *testing.T) {
	c := makeContractForInspectionTest()
	repo := newTrackingContractRepo(c)
	bus := &capturingEventBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2000, "tenant-1")
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil)
	handler := service.NewInspectionCompletedHandler(svc)

	raw := map[string]interface{}{
		"inspection_id": "insp-9",
		"tenant_id":     "tenant-1",
		"contract_id":   "contract-7",
		"type":          "ENTRY",
		"report_url":    "https://example.com/laudo.pdf",
	}
	// Reuse the existing mockEvent from contract_lifecycle_test.go (same package).
	assert.NoError(t, handler(context.Background(), mockInspectionEvent{raw}))

	updated, _ := repo.GetByID(context.Background(), "tenant-1", "contract-7")
	assert.Equal(t, "insp-9", updated.EntryInspectionID)
}

// mockInspectionEvent is a tiny Event wrapper for the map-payload test.
// Avoid colliding with mockEvent in case other tests already define one.
type mockInspectionEvent struct {
	payload interface{}
}

func (m mockInspectionEvent) EventID() string      { return "" }
func (m mockInspectionEvent) Name() string         { return "inspection.completed" }
func (m mockInspectionEvent) Payload() interface{} { return m.payload }

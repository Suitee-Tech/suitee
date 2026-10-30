package service

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// captureBus em events_test.go já existe? Não — vamos reusar
// inMemoryAcceptLeadRepo do lead_acceptance_service_test.go pra evitar
// repetição. O método HandlePropertyCapturedConvert só usa GetByID/Update,
// que aquele mock já implementa.

type propertyCapturedTestRepo = inMemoryAcceptLeadRepo

func newPropertyCapturedTestService(repo lead.Repository) *LeadService {
	return &LeadService{
		repo:          repo,
		bootstrapping: map[string]bool{},
	}
}

// mockEvent satisfaz events.Event pelos métodos EventID/Name/Payload.
type capturedMockEvent struct {
	id      string
	name    string
	payload interface{}
}

func (e capturedMockEvent) EventID() string      { return e.id }
func (e capturedMockEvent) Name() string         { return e.name }
func (e capturedMockEvent) Payload() interface{} { return e.payload }

func TestHandlePropertyCapturedConvert_Success(t *testing.T) {
	ctx := context.Background()
	ownerLead := &lead.Lead{
		ID:       "lead-owner-1",
		TenantID: "tenant-A",
		Name:     "Proprietário Teste",
		Email:    "owner@example.com",
		Status:   lead.LeadStatusNew,
		IsClient: false,
	}
	repo := newAcceptRepo(ownerLead)
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		id:   "evt-1",
		name: "property.captured",
		payload: contracts.PropertyCapturedEventPayload{
			PropertyID:         "prop-1",
			TenantID:           "tenant-A",
			OwnerLeadID:        ownerLead.ID,
			CapturingBrokerIDs: []string{"broker-1"},
			Status:             "prospecting",
			ListingType:        "rent",
			Timestamp:          time.Now().Format(time.RFC3339),
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	got, _ := repo.GetByID(ctx, ownerLead.TenantID, ownerLead.ID)
	if got == nil {
		t.Fatal("lead disappeared after handler")
	}
	if !got.IsClient {
		t.Errorf("expected IsClient=true, got false")
	}
	if got.Role != lead.ClientRoleOwner {
		t.Errorf("expected role=owner, got %q", got.Role)
	}
	if got.Status != lead.LeadStatusConverted {
		t.Errorf("expected status=CONVERTED, got %q", got.Status)
	}
	if got.CaptacaoPropertyID != "prop-1" {
		t.Errorf("expected CaptacaoPropertyID=prop-1, got %q", got.CaptacaoPropertyID)
	}
}

func TestHandlePropertyCapturedConvert_Idempotent_AlreadyClient(t *testing.T) {
	ctx := context.Background()
	ownerLead := &lead.Lead{
		ID:       "lead-owner-1",
		TenantID: "tenant-A",
		Status:   lead.LeadStatusConverted,
		IsClient: true,
		Role:     lead.ClientRoleOwner,
	}
	repo := newAcceptRepo(ownerLead)
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		name: "property.captured",
		payload: contracts.PropertyCapturedEventPayload{
			PropertyID:  "prop-2",
			TenantID:    "tenant-A",
			OwnerLeadID: ownerLead.ID,
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Errorf("idempotency: should not error on already-client, got %v", err)
	}
	// CaptacaoPropertyID NÃO deve ser sobrescrito pelo handler quando o
	// lead já é cliente — primeira captação fica autoritativa.
	got, _ := repo.GetByID(ctx, ownerLead.TenantID, ownerLead.ID)
	if got.CaptacaoPropertyID != "" {
		t.Errorf("expected CaptacaoPropertyID untouched, got %q", got.CaptacaoPropertyID)
	}
}

func TestHandlePropertyCapturedConvert_SkipsEmptyOwner(t *testing.T) {
	ctx := context.Background()
	repo := newAcceptRepo()
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		name: "property.captured",
		payload: contracts.PropertyCapturedEventPayload{
			PropertyID:  "prop-3",
			TenantID:    "tenant-A",
			OwnerLeadID: "", // captação direta sem owner cadastrado
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Errorf("empty owner is legitimate flow, should not error: %v", err)
	}
}

func TestHandlePropertyCapturedConvert_AcceptsMapPayload(t *testing.T) {
	// RabbitMQ entrega payload como map[string]interface{} após
	// deserialize. Handler deve marshal/unmarshal sem perder dados.
	ctx := context.Background()
	ownerLead := &lead.Lead{
		ID:       "lead-owner-2",
		TenantID: "tenant-A",
		Status:   lead.LeadStatusNew,
	}
	repo := newAcceptRepo(ownerLead)
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		name: "property.captured",
		payload: map[string]interface{}{
			"property_id":   "prop-4",
			"tenant_id":     "tenant-A",
			"owner_lead_id": ownerLead.ID,
			"status":        "prospecting",
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Fatalf("expected success with map payload, got %v", err)
	}
	got, _ := repo.GetByID(ctx, ownerLead.TenantID, ownerLead.ID)
	if !got.IsClient {
		t.Error("lead should be promoted to client from map payload")
	}
}

func TestHandlePropertyCapturedConvert_MissingLead(t *testing.T) {
	// Lead inexistente (ex.: race com cadastro). Handler não-op + log,
	// sem propagar erro para o broker entrar em redeliver loop infinito.
	ctx := context.Background()
	repo := newAcceptRepo()
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		name: "property.captured",
		payload: contracts.PropertyCapturedEventPayload{
			PropertyID:  "prop-5",
			TenantID:    "tenant-A",
			OwnerLeadID: "lead-missing",
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Errorf("missing lead should not error (avoid redeliver loop): %v", err)
	}
}

func TestHandlePropertyCapturedConvert_BackfillsTenantID(t *testing.T) {
	// Lead legacy criado sem tenant_id (caso conhecido J01 BDD). Handler
	// preenche a partir do tenant do evento.
	ctx := context.Background()
	ownerLead := &lead.Lead{
		ID:       "lead-owner-3",
		TenantID: "",
		Status:   lead.LeadStatusNew,
	}
	repo := newAcceptRepo(ownerLead)
	svc := newPropertyCapturedTestService(repo)

	evt := capturedMockEvent{
		name: "property.captured",
		payload: contracts.PropertyCapturedEventPayload{
			PropertyID:  "prop-6",
			TenantID:    "tenant-Z",
			OwnerLeadID: ownerLead.ID,
		},
	}

	if err := svc.HandlePropertyCapturedConvert(ctx, evt); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	got, _ := repo.GetByID(ctx, ownerLead.TenantID, ownerLead.ID)
	if got.TenantID != "tenant-Z" {
		t.Errorf("expected tenant_id backfilled to tenant-Z, got %q", got.TenantID)
	}
}

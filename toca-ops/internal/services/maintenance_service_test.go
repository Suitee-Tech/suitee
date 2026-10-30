package services_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	pkgevents "github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

type maintenanceAttachmentStorage struct{}

func (maintenanceAttachmentStorage) Upload(context.Context, string, string, string, io.Reader) (string, error) {
	return "", nil
}

func (maintenanceAttachmentStorage) Delete(context.Context, string, string) error { return nil }
func (maintenanceAttachmentStorage) GetURL(string, string) string                 { return "" }
func (maintenanceAttachmentStorage) GetPresignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	return "https://storage.example/" + bucket + "/" + key + "?signed=1", nil
}
func (maintenanceAttachmentStorage) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (maintenanceAttachmentStorage) IsPrivate(string) bool { return true }

type inMemoryMaintenanceRepo struct {
	mu      sync.Mutex
	tickets map[string]*maintenance.MaintenanceTicket
}

func newInMemoryMaintenanceRepo() *inMemoryMaintenanceRepo {
	return &inMemoryMaintenanceRepo{tickets: make(map[string]*maintenance.MaintenanceTicket)}
}

func (r *inMemoryMaintenanceRepo) Create(ctx context.Context, t *maintenance.MaintenanceTicket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *t
	r.tickets[t.ID] = &cp
	return nil
}

// GetByID espelha o repositório Postgres: filtra pelo tenant do chamador e
// falha fechado quando ele está vazio (auditoria A8/ops).
func (r *inMemoryMaintenanceRepo) GetByID(ctx context.Context, tenantID, id string) (*maintenance.MaintenanceTicket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(tenantID) == "" {
		return nil, nil
	}
	t, ok := r.tickets[id]
	if !ok || t.TenantID != tenantID {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *inMemoryMaintenanceRepo) Update(ctx context.Context, t *maintenance.MaintenanceTicket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *t
	r.tickets[t.ID] = &cp
	return nil
}

func (r *inMemoryMaintenanceRepo) Delete(ctx context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tickets, id)
	return nil
}

func (r *inMemoryMaintenanceRepo) List(ctx context.Context, tenantID, status string, requesterLeadID string, rentalID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	return nil, 0, nil
}

func (r *inMemoryMaintenanceRepo) ListByProperty(ctx context.Context, tenantID, propertyID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	return nil, 0, nil
}

type captureEventBus struct {
	mu     sync.Mutex
	events []pkgevents.Event
}

func (b *captureEventBus) Publish(ctx context.Context, e pkgevents.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
	return nil
}

func (b *captureEventBus) PublishReliable(ctx context.Context, e pkgevents.Event) error {
	return b.Publish(ctx, e)
}

func (b *captureEventBus) Subscribe(name string, h pkgevents.EventHandler) error { return nil }

func (b *captureEventBus) SubscribeWithQueue(queue, name string, h pkgevents.EventHandler) error {
	return nil
}

func (b *captureEventBus) byName(name string) []pkgevents.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]pkgevents.Event, 0)
	for _, e := range b.events {
		if e.Name() == name {
			out = append(out, e)
		}
	}
	return out
}

func TestMaintenanceService_Create_PublishesRequestedEventWithPayerRole(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	bus := &captureEventBus{}
	svc := services.NewMaintenanceService(repo, bus)

	ticket := maintenance.New(
		"hidraulica", "Reparo", "prop-1", "contract-1",
		"Casa do Joao", "Joao Inquilino", "tenant", "alta",
		"Vazamento no banheiro", "tenant-1",
	)
	ticket.RequesterLeadID = "lead-1"
	ticket.EstimatedCost = 500.0
	ticket.PayerRole = "tenant"

	assert.NoError(t, svc.Create(context.Background(), ticket))

	requested := bus.byName("maintenance.requested")
	if assert.Len(t, requested, 1, "should publish exactly one maintenance.requested event") {
		payload, ok := requested[0].Payload().(contracts.MaintenanceRequestedEventPayload)
		assert.True(t, ok, "payload should be the typed contracts.MaintenanceRequestedEventPayload")
		assert.Equal(t, "tenant", payload.PayerRole)
		assert.Equal(t, ticket.ID, payload.TicketID)
		assert.InDelta(t, 500.0, payload.EstimatedCost, 0.001)
		assert.Equal(t, "lead-1", payload.RequesterLeadID)
		assert.Equal(t, "contract-1", payload.ContractID)
	}
	assert.Empty(t, bus.byName("maintenance.completed"))
}

func TestMaintenanceService_GetByIDForOperator_PresignsAttachmentsWithoutPersistingURLs(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	ticket := maintenance.New("Manutenção", "Infiltração", "property-1", "contract-1", "Imóvel", "Cliente", "Inquilino", "Alta", "Há vazamento", "tenant-1")
	ticket.Attachments = []string{"tenant-1/maintenance/deal-1/photo.jpg"}
	if err := repo.Create(context.Background(), ticket); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	svc := services.NewMaintenanceService(repo, nil).WithStorage(maintenanceAttachmentStorage{})
	view, err := svc.GetByIDForOperator(context.Background(), "tenant-1", ticket.ID)
	if err != nil {
		t.Fatalf("GetByIDForOperator returned error: %v", err)
	}
	if got := view.Attachments[0]; got != "https://storage.example/toca-documents/tenant-1/maintenance/deal-1/photo.jpg?signed=1" {
		t.Fatalf("signed attachment = %q", got)
	}

	raw, err := svc.GetByID(context.Background(), "tenant-1", ticket.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if got := raw.Attachments[0]; got != "tenant-1/maintenance/deal-1/photo.jpg" {
		t.Fatalf("stored attachment was mutated to %q", got)
	}
}

func TestMaintenanceService_GetByIDForOperator_RecoversLegacyUploadURL(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	ticket := maintenance.New("Manutenção", "Infiltração", "property-1", "contract-1", "Imóvel", "Cliente", "Inquilino", "Alta", "Há vazamento", "tenant-1")
	ticket.Attachments = []string{"http://localhost:3000/uploads/toca-documents/tenant-1/tickets/ticket-1/Fundo.png"}
	if err := repo.Create(context.Background(), ticket); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	svc := services.NewMaintenanceService(repo, nil).WithStorage(maintenanceAttachmentStorage{})
	view, err := svc.GetByIDForOperator(context.Background(), "tenant-1", ticket.ID)
	if err != nil {
		t.Fatalf("GetByIDForOperator returned error: %v", err)
	}
	want := "https://storage.example/toca-documents/tenant-1/tickets/ticket-1/Fundo.png?signed=1"
	if got := view.Attachments[0]; got != want {
		t.Fatalf("legacy attachment = %q, want %q", got, want)
	}
}

func TestMaintenanceService_Create_QuickChargeTicketAlsoPublishesCompleted(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	bus := &captureEventBus{}
	svc := services.NewMaintenanceService(repo, bus)

	ticket := maintenance.New(
		"avulsa", "Cobrança avulsa", "prop-1", "contract-1",
		"", "Operator", "admin", "media",
		"Cobrança avulsa via UI", "tenant-1",
	)
	ticket.EstimatedCost = 200.0
	ticket.ActualCost = 200.0
	ticket.PayerRole = "owner"
	ticket.Status = maintenance.StatusConcluido

	assert.NoError(t, svc.Create(context.Background(), ticket))

	assert.Len(t, bus.byName("maintenance.requested"), 1)
	completed := bus.byName("maintenance.completed")
	if assert.Len(t, completed, 1) {
		p := completed[0].Payload().(contracts.MaintenanceCompletedEventPayload)
		assert.Equal(t, "owner", p.PayerRole)
		assert.InDelta(t, 200.0, p.ActualCost, 0.001)
		assert.InDelta(t, 200.0, p.EstimatedCost, 0.001)
	}
}

func TestMaintenanceService_ChangeStatus_ToConcluidoPublishesCompletedEvent(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	bus := &captureEventBus{}
	svc := services.NewMaintenanceService(repo, bus)

	ticket := maintenance.New(
		"eletrica", "Reparo", "prop-1", "contract-1",
		"", "Joao", "tenant", "alta",
		"", "tenant-1",
	)
	ticket.PayerRole = "tenant"
	ticket.EstimatedCost = 500.0
	assert.NoError(t, svc.Create(context.Background(), ticket))

	mid, err := svc.ChangeStatus(context.Background(), "tenant-1", ticket.ID, maintenance.StatusEmAndamento)
	assert.NoError(t, err)
	mid.ActualCost = 620.0
	assert.NoError(t, svc.Update(context.Background(), mid))

	final, err := svc.ChangeStatus(context.Background(), "tenant-1", ticket.ID, maintenance.StatusConcluido)
	assert.NoError(t, err)
	assert.NotNil(t, final.CompletedAt)
	assert.InDelta(t, 620.0, final.ActualCost, 0.001)

	completed := bus.byName("maintenance.completed")
	if assert.Len(t, completed, 1) {
		p := completed[0].Payload().(contracts.MaintenanceCompletedEventPayload)
		assert.InDelta(t, 620.0, p.ActualCost, 0.001)
		assert.Equal(t, "tenant", p.PayerRole)
	}
}

func TestMaintenanceService_Update_ImplicitTransitionPublishesCompleted(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	bus := &captureEventBus{}
	svc := services.NewMaintenanceService(repo, bus)

	ticket := maintenance.New(
		"pintura", "Reparo", "prop-1", "contract-1",
		"", "Joao", "tenant", "media",
		"", "tenant-1",
	)
	ticket.PayerRole = "owner"
	ticket.EstimatedCost = 800.0
	assert.NoError(t, svc.Create(context.Background(), ticket))
	bus.events = nil

	ticket.Status = maintenance.StatusConcluido
	ticket.ActualCost = 800.0
	assert.NoError(t, svc.Update(context.Background(), ticket))

	completed := bus.byName("maintenance.completed")
	assert.Len(t, completed, 1)
}

// Auditoria A8/ops: chamado de manutenção de outro tenant não pode ser lido
// (nem com anexo privado virando URL pré-assinada) apenas por conhecer o id.
func TestMaintenanceService_GetByIDForOperator_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	ticket := maintenance.New("Manutenção", "Infiltração", "property-1", "contract-1", "Imóvel", "Cliente", "Inquilino", "Alta", "Há vazamento", "tenant-B")
	ticket.Attachments = []string{"tenant-B/maintenance/deal-1/photo.jpg"}
	assert.NoError(t, repo.Create(context.Background(), ticket))

	svc := services.NewMaintenanceService(repo, nil).WithStorage(maintenanceAttachmentStorage{})

	view, err := svc.GetByIDForOperator(context.Background(), "tenant-A", ticket.ID)
	assert.Nil(t, view)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	own, err := svc.GetByIDForOperator(context.Background(), "tenant-B", ticket.ID)
	assert.NoError(t, err)
	assert.NotNil(t, own)
}

func TestMaintenanceService_ChangeStatus_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryMaintenanceRepo()
	bus := &captureEventBus{}
	svc := services.NewMaintenanceService(repo, bus)

	ticket := maintenance.New("eletrica", "Reparo", "prop-1", "contract-1", "", "Joao", "tenant", "alta", "", "tenant-B")
	ticket.Status = maintenance.StatusEmAnalise
	assert.NoError(t, svc.Create(context.Background(), ticket))
	bus.events = nil

	_, err := svc.ChangeStatus(context.Background(), "tenant-A", ticket.ID, maintenance.StatusCancelado)
	assert.Error(t, err)
	assert.Empty(t, bus.byName("maintenance.completed"))

	stored, err := repo.GetByID(context.Background(), "tenant-B", ticket.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, stored) {
		assert.Equal(t, maintenance.StatusEmAnalise, stored.Status, "status de outro tenant não pode mudar")
	}
}

package services_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	pkgevents "github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

type inMemoryInspectionRepo struct {
	mu          sync.Mutex
	inspections map[string]*inspection.Inspection
	lastTenant  string
}

func newInMemoryInspectionRepo() *inMemoryInspectionRepo {
	return &inMemoryInspectionRepo{inspections: make(map[string]*inspection.Inspection)}
}

func (r *inMemoryInspectionRepo) Create(ctx context.Context, i *inspection.Inspection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *i
	r.inspections[i.ID] = &cp
	return nil
}

// GetByID espelha o repositório Postgres: devolve a vistoria apenas quando ela
// pertence ao tenant pedido e falha fechado (nada encontrado) para tenant vazio
// ou estrangeiro. `lastTenant` registra o tenant que o serviço repassou, para os
// testes provarem que veio do CHAMADOR e não do registro.
func (r *inMemoryInspectionRepo) GetByID(ctx context.Context, tenantID, id string) (*inspection.Inspection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTenant = tenantID
	if len(strings.TrimSpace(tenantID)) == 0 {
		return nil, nil
	}
	t, ok := r.inspections[id]
	if !ok || t.TenantID != tenantID {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *inMemoryInspectionRepo) Update(ctx context.Context, i *inspection.Inspection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *i
	r.inspections[i.ID] = &cp
	return nil
}

func (r *inMemoryInspectionRepo) Delete(ctx context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inspections, id)
	return nil
}

func (r *inMemoryInspectionRepo) List(ctx context.Context, f inspection.ListFilter) ([]*inspection.Inspection, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*inspection.Inspection, 0)
	for _, i := range r.inspections {
		if f.TenantID != "" && i.TenantID != f.TenantID {
			continue
		}
		if f.ContractID != "" && i.ContractID != f.ContractID {
			continue
		}
		if f.Type != "" && i.Type != f.Type {
			continue
		}
		if f.Status != "" && i.Status != f.Status {
			continue
		}
		cp := *i
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func TestInspectionService_Create_PublishesScheduledEvent(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	scheduled := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	insp := inspection.New("prop-1", "contract-1", inspection.TypeEntry, "ins-1", "Maria", "tenant-1", scheduled)

	assert.NoError(t, svc.Create(context.Background(), insp))

	scheduledEvts := bus.byName("inspection.scheduled")
	if assert.Len(t, scheduledEvts, 1) {
		p := scheduledEvts[0].Payload().(contracts.InspectionScheduledEventPayload)
		assert.Equal(t, "contract-1", p.ContractID)
		assert.Equal(t, "ENTRY", p.Type)
		assert.Equal(t, "ins-1", p.InspectorID)
	}
	assert.Empty(t, bus.byName("inspection.completed"))
}

func TestInspectionService_CreateScheduled_DeterministicIDAndIdempotent(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	derivedID := pkgevents.DeriveNotificationID("contract-99", "inspection.entry")
	insp := inspection.New("prop-1", "contract-99", inspection.TypeEntry, "", "", "tenant-1", time.Now())
	assert.NoError(t, svc.CreateScheduled(context.Background(), derivedID, insp))

	insp2 := inspection.New("prop-1", "contract-99", inspection.TypeEntry, "", "", "tenant-1", time.Now())
	assert.NoError(t, svc.CreateScheduled(context.Background(), derivedID, insp2))

	assert.Len(t, bus.byName("inspection.scheduled"), 1, "second CreateScheduled should be idempotent")
	stored, err := repo.GetByID(context.Background(), "tenant-1", derivedID)
	assert.NoError(t, err)
	assert.NotNil(t, stored)
}

func TestInspectionService_Update_ToConcluidaPublishesCompleted(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	insp := inspection.New("prop-1", "contract-1", inspection.TypeExit, "ins-1", "Maria", "tenant-1", time.Now())
	assert.NoError(t, svc.Create(context.Background(), insp))
	bus.events = nil

	updated := *insp
	updated.Status = inspection.StatusConcluida
	updated.ReportURL = "https://docs.example/laudo-saida.pdf"
	updated.Findings = []inspection.Finding{
		{Room: "Sala", Description: "Tudo ok", Condition: inspection.ConditionBom, Photos: []string{}},
		{Room: "Cozinha", Description: "Pia trincada", Condition: inspection.ConditionRuim, Photos: []string{"https://photo"}},
	}
	assert.NoError(t, svc.Update(context.Background(), &updated))

	completed := bus.byName("inspection.completed")
	if assert.Len(t, completed, 1) {
		p := completed[0].Payload().(contracts.InspectionCompletedEventPayload)
		assert.Equal(t, "EXIT", p.Type)
		assert.Equal(t, "https://docs.example/laudo-saida.pdf", p.ReportURL)
		assert.Equal(t, 2, p.FindingsCount)
		assert.True(t, p.HasDamage, "HasDamage should be true when any finding is not Bom")
	}
}

func TestInspectionService_Update_NoReportURL_NoCompletedEvent(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	insp := inspection.New("prop-1", "contract-1", inspection.TypeEntry, "", "", "tenant-1", time.Now())
	assert.NoError(t, svc.Create(context.Background(), insp))
	bus.events = nil

	updated := *insp
	updated.Status = inspection.StatusConcluida
	assert.NoError(t, svc.Update(context.Background(), &updated))

	assert.Empty(t, bus.byName("inspection.completed"))
}

func TestInspectionService_GetByID_RejectsForeignTenantAsNotFound(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	svc := services.NewInspectionService(repo, &captureEventBus{})

	insp := inspection.New("prop-B", "contract-B", inspection.TypeEntry, "ins-1", "Maria", "tenant-B", time.Now())
	assert.NoError(t, repo.Create(context.Background(), insp))

	// tenant-A pedindo a vistoria de tenant-B: 404 (nada encontrado), sem vazar
	// o tenant do dono.
	got, err := svc.GetByID(context.Background(), "tenant-A", insp.ID)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, services.ErrInspectionNotFound)

	// O tenant dono continua lendo normalmente (sem regressão funcional).
	own, err := svc.GetByID(context.Background(), "tenant-B", insp.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, own) {
		assert.Equal(t, "prop-B", own.PropertyID)
	}
	assert.Equal(t, "tenant-B", repo.lastTenant, "o serviço deve repassar o tenant do chamador ao repositório")
}

func TestInspectionService_GetByID_FailsClosedWithoutTenant(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	svc := services.NewInspectionService(repo, &captureEventBus{})

	insp := inspection.New("prop-1", "contract-1", inspection.TypeEntry, "ins-1", "Maria", "tenant-1", time.Now())
	assert.NoError(t, repo.Create(context.Background(), insp))

	got, err := svc.GetByID(context.Background(), "", insp.ID)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, services.ErrInspectionNotFound)
}

func TestInspectionService_Update_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	svc := services.NewInspectionService(repo, &captureEventBus{})

	insp := inspection.New("prop-B", "contract-B", inspection.TypeExit, "ins-1", "Maria", "tenant-B", time.Now())
	assert.NoError(t, repo.Create(context.Background(), insp))

	forged := *insp
	forged.TenantID = "tenant-A" // registro forjado com o tenant do atacante
	forged.ReportURL = "https://evil.example/laudo.pdf"
	err := svc.Update(context.Background(), &forged)
	assert.ErrorIs(t, err, services.ErrInspectionNotFound)

	stored, err := repo.GetByID(context.Background(), "tenant-B", insp.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, stored) {
		assert.Empty(t, stored.ReportURL, "vistoria de outro tenant não pode ser sobrescrita")
	}
}

func TestAssignedInspectionUsesCompanySupplier(t *testing.T) {
	supplierRepo := &fakeSupplierRepo{items: []*supplier.Supplier{
		{ID: "inspector", TenantID: "company", Name: "Ana", ServiceType: "Vistoriador", Active: true},
		{ID: "electrician", TenantID: "company", Name: "Bia", ServiceType: "Elétrica", Active: true},
		{ID: "inactive", TenantID: "company", Name: "Cris", ServiceType: "Vistoria", Active: false},
		{ID: "foreign", TenantID: "other", Name: "Dora", ServiceType: "Vistoria", Active: true},
	}}
	repo := newInMemoryInspectionRepo()
	svc := services.NewInspectionService(repo, nil).WithSuppliers(supplierRepo)
	for _, id := range []string{"electrician", "inactive", "foreign", "manual"} {
		i := inspection.New("property", "contract", "ENTRY", id, "forged", "company", time.Now())
		assert.ErrorIs(t, svc.CreateAssigned(context.Background(), i), services.ErrInvalidInspector)
	}
	i := inspection.New("property", "contract", "ENTRY", "inspector", "forged", "company", time.Now())
	assert.NoError(t, svc.CreateAssigned(context.Background(), i))
	assert.Equal(t, "Ana", i.InspectorName)
	assert.Len(t, repo.inspections, 1)
}

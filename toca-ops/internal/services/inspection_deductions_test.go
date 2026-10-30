package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

func TestRecordDeductions_HappyPath(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	insp := inspection.New("prop-1", "contract-1", inspection.TypeExit, "ins-1", "Maria", "tenant-1", time.Now())
	insp.Status = inspection.StatusConcluida
	insp.ReportURL = "https://docs/laudo.pdf"
	assert.NoError(t, repo.Create(context.Background(), insp))
	bus.events = nil

	deductions := []inspection.Deduction{
		{Room: "Cozinha", Description: "Pia trincada", Amount: 250},
		{Room: "Banheiro", Description: "Box quebrado", Amount: 400},
	}
	updated, err := svc.RecordDeductions(context.Background(), "tenant-1", insp.ID, deductions)
	assert.NoError(t, err)
	assert.Len(t, updated.Deductions, 2)

	evts := bus.byName("inspection.deductions_recorded")
	if assert.Len(t, evts, 1) {
		p := evts[0].Payload().(contracts.InspectionDeductionsRecordedEventPayload)
		assert.InDelta(t, 650.0, p.TotalAmount, 0.001)
		assert.Len(t, p.Deductions, 2)
	}
}

func TestRecordDeductions_RejectsNonExit(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	insp := inspection.New("prop-1", "contract-1", inspection.TypeEntry, "ins-1", "Maria", "tenant-1", time.Now())
	insp.Status = inspection.StatusConcluida
	insp.ReportURL = "https://docs/laudo.pdf"
	assert.NoError(t, repo.Create(context.Background(), insp))

	_, err := svc.RecordDeductions(context.Background(), "tenant-1", insp.ID, []inspection.Deduction{
		{Room: "Sala", Description: "Pintura", Amount: 100},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EXIT")
}

func TestRecordDeductions_RejectsNotConcluded(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	insp := inspection.New("prop-1", "contract-1", inspection.TypeExit, "ins-1", "Maria", "tenant-1", time.Now())
	assert.NoError(t, repo.Create(context.Background(), insp))

	_, err := svc.RecordDeductions(context.Background(), "tenant-1", insp.ID, []inspection.Deduction{
		{Room: "Sala", Description: "Pintura", Amount: 100},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Concluída")
}

func TestRecordDeductions_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)

	insp := inspection.New("prop-1", "contract-1", inspection.TypeExit, "ins-1", "Maria", "tenant-B", time.Now())
	insp.Status = inspection.StatusConcluida
	insp.ReportURL = "https://docs/laudo.pdf"
	assert.NoError(t, repo.Create(context.Background(), insp))

	_, err := svc.RecordDeductions(context.Background(), "tenant-A", insp.ID, []inspection.Deduction{
		{Room: "Sala", Description: "Pintura", Amount: 100},
	})
	assert.ErrorIs(t, err, services.ErrInspectionNotFound)
	assert.Empty(t, bus.byName("inspection.deductions_recorded"), "tenant estrangeiro não gera evento no tenant da vítima")

	stored, err := repo.GetByID(context.Background(), "tenant-B", insp.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, stored) {
		assert.Empty(t, stored.Deductions, "deduções de outro tenant não podem ser gravadas")
	}
}

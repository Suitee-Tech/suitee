package services_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	pkgevents "github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
	"github.com/stretchr/testify/assert"
)

// reuses fakeDocumentRepo + fakeStorage from document_service_signed_pdf_test.go

func TestInspectionCompletedDocumentHandler_CreatesLaudo(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewInspectionCompletedDocumentHandler(docs)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-1",
			TenantID:     "tenant-1",
			ContractID:   "contract-1",
			Type:         "ENTRY",
			ReportURL:    "https://docs.example/laudo.pdf",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	if assert.Len(t, repo.creates, 1) {
		d := repo.creates[0]
		assert.Equal(t, document.CategoryLaudo, d.Category)
		assert.Equal(t, document.EntityInspection, d.EntityType)
		assert.Equal(t, "insp-1", d.EntityID)
		assert.Equal(t, "contract-1", d.ContractID)
		assert.Equal(t, "https://docs.example/laudo.pdf", d.FileURL)
		assert.Equal(t, document.SourceOps, d.Source)
	}
}

func TestInspectionCompletedDocumentHandler_Idempotent(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewInspectionCompletedDocumentHandler(docs)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-1",
			TenantID:     "tenant-1",
			ContractID:   "contract-1",
			Type:         "EXIT",
			ReportURL:    "https://docs.example/laudo-saida.pdf",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.NoError(t, handler(context.Background(), evt))
	assert.Len(t, repo.creates, 1, "second delivery must no-op")
}

func TestInspectionCompletedDocumentHandler_NoReportURL_NoCreate(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewInspectionCompletedDocumentHandler(docs)

	evt := contracts.InspectionCompletedEvent{
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID: "insp-1",
			TenantID:     "tenant-1",
			ContractID:   "contract-1",
			Type:         "ENTRY",
			ReportURL:    "",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.Empty(t, repo.creates)
}

// stubAttachmentLookup returns canned attachments for ticket->ticket testing.
type stubAttachmentLookup struct {
	byTicket map[string][]string
	err      error
}

func (s *stubAttachmentLookup) GetAttachments(_ context.Context, _, ticketID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byTicket[ticketID], nil
}

func TestMaintenanceCompletedDocumentHandler_OneDocumentPerAttachment(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	lookup := &stubAttachmentLookup{
		byTicket: map[string][]string{
			"ticket-7": {"https://photos.example/before.jpg", "https://photos.example/after.jpg"},
		},
	}
	handler := service.NewMaintenanceCompletedDocumentHandler(docs, lookup)

	evt := contracts.MaintenanceCompletedEvent{
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:   "ticket-7",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   "hidraulica",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	if assert.Len(t, repo.creates, 2) {
		for _, d := range repo.creates {
			assert.Equal(t, document.CategoryComprovante, d.Category)
			assert.Equal(t, document.EntityType("maintenance"), d.EntityType)
			assert.Equal(t, "ticket-7", d.EntityID)
		}
	}
}

func TestMaintenanceCompletedDocumentHandler_NoLookup_AcksAndSkips(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewMaintenanceCompletedDocumentHandler(docs, nil)

	evt := contracts.MaintenanceCompletedEvent{
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:   "ticket-7",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.Empty(t, repo.creates)
}

func TestMaintenanceCompletedDocumentHandler_NoAttachments_NoCreate(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	lookup := &stubAttachmentLookup{byTicket: map[string][]string{"ticket-7": nil}}
	handler := service.NewMaintenanceCompletedDocumentHandler(docs, lookup)

	evt := contracts.MaintenanceCompletedEvent{
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:   "ticket-7",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.Empty(t, repo.creates)
}

func TestMaintenanceCompletedDocumentHandler_Idempotent(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	lookup := &stubAttachmentLookup{
		byTicket: map[string][]string{"ticket-9": {"https://photos.example/A.png"}},
	}
	handler := service.NewMaintenanceCompletedDocumentHandler(docs, lookup)

	evt := contracts.MaintenanceCompletedEvent{
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:   "ticket-9",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.NoError(t, handler(context.Background(), evt))
	assert.Len(t, repo.creates, 1, "second delivery must dedupe")
}

func TestReclassifyContractAsAmendment(t *testing.T) {
	repo := &fakeDocumentRepo{}
	// Seed a "contrato" document linked to the contract.
	repo.docs = []*document.Document{
		{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusAssinado,
		},
	}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})

	assert.NoError(t, docs.ReclassifyContractAsAmendment(context.Background(), "tenant-1", "contract-1"))
	assert.Len(t, repo.updates, 1)
	assert.Equal(t, document.CategoryAditivo, repo.updates[0].Category)
}

// Compile-time check that the dummy event type satisfies events.Event.
var _ pkgevents.Event = contracts.InspectionCompletedEvent{}
var _ pkgevents.Event = contracts.MaintenanceCompletedEvent{}

// stubProofDownloader devolve bytes fixos para simular a leitura do bucket
// financial-proofs sem storage real.
type stubProofDownloader struct {
	body        []byte
	contentType string
	err         error
}

func (s *stubProofDownloader) Download(_ context.Context, _, _ string) (io.ReadCloser, string, int64, error) {
	if s.err != nil {
		return nil, "", 0, s.err
	}
	return io.NopCloser(bytes.NewReader(s.body)), s.contentType, int64(len(s.body)), nil
}

func TestFinancialProofDocumentHandler_CreatesComprovante(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewFinancialProofDocumentHandler(docs, &stubProofDownloader{
		body:        []byte("fake-pdf-bytes"),
		contentType: "application/pdf",
	})

	evt := contracts.FinancialProofUploadedEvent{
		Data: contracts.FinancialProofUploadedEventPayload{
			TenantID:    "tenant-1",
			Kind:        "payout",
			URL:         "/api/v1/proofs/content/abc",
			Filename:    "comprovante-pix.pdf",
			ContentType: "application/pdf",
			SizeBytes:   14,
			ObjectKey:   "tenant-1/payout/2026-09/uuid.pdf",
			UploadedBy:  "actor-1",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	if assert.Len(t, repo.creates, 1) {
		d := repo.creates[0]
		assert.Equal(t, document.CategoryComprovante, d.Category)
		assert.Equal(t, document.EntityFinancial, d.EntityType)
		assert.Equal(t, document.SourceFinancial, d.Source)
		assert.Equal(t, document.StatusPendente, d.Status)
		assert.Equal(t, []string{"financeiro", "payout"}, d.Tags)
		assert.Contains(t, d.Name, "Comprovante Payout")
	}
	// Com downloader, os bytes são copiados para o bucket de documentos.
	if assert.Len(t, repo.updates, 1) {
		assert.Equal(t, document.PersistenceStored, repo.updates[0].PersistenceStatus)
		assert.NotEmpty(t, repo.updates[0].StorageKey)
	}
}

func TestFinancialProofDocumentHandler_Idempotent(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewFinancialProofDocumentHandler(docs, nil)

	evt := contracts.FinancialProofUploadedEvent{
		Data: contracts.FinancialProofUploadedEventPayload{
			TenantID:  "tenant-1",
			Kind:      "charge",
			URL:       "/api/v1/proofs/content/abc",
			Filename:  "recibo.pdf",
			ObjectKey: "tenant-1/charge/2026-09/uuid.pdf",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.NoError(t, handler(context.Background(), evt))
	assert.Len(t, repo.creates, 1, "redelivery must no-op")
}

func TestFinancialProofDocumentHandler_NoDownloader_MetadataOnly(t *testing.T) {
	repo := &fakeDocumentRepo{}
	docs := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewFinancialProofDocumentHandler(docs, nil)

	evt := contracts.FinancialProofUploadedEvent{
		Data: contracts.FinancialProofUploadedEventPayload{
			TenantID:    "tenant-1",
			Kind:        "commission",
			URL:         "/api/v1/proofs/content/xyz",
			Filename:    "comissao.png",
			ContentType: "image/png",
			ObjectKey:   "tenant-1/commission/2026-09/uuid.png",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	if assert.Len(t, repo.creates, 1) {
		assert.Equal(t, "/api/v1/proofs/content/xyz", repo.creates[0].FileURL)
		assert.Equal(t, document.PersistencePending, repo.creates[0].PersistenceStatus)
	}
	assert.Empty(t, repo.updates, "sem downloader não há materialização de bytes")
}

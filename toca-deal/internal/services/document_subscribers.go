package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

// NewInspectionCompletedDocumentHandler subscribes the Document layer to
// inspection.completed so a Laudo Document is materialized as soon as the
// vistoria is closed with a report_url. Deterministic id keeps the writer
// idempotent across broker redeliveries.
func NewInspectionCompletedDocumentHandler(docService *DocumentService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeInspectionCompletedPayload(event)
		if err != nil {
			return err
		}
		if payload.ReportURL == "" || payload.ContractID == "" || payload.TenantID == "" {
			// No laudo URL → no Document worth creating. The contract-side
			// EntryInspectionID handler still fires in inspection_subscribers.go.
			return nil
		}

		id := events.DeriveNotificationID(payload.InspectionID, "document", "laudo")
		typeLabel := inspectionTypeLabel(payload.Type)
		name := fmt.Sprintf("Laudo Vistoria %s — Contrato %s", typeLabel, payload.ContractID)

		// Check for an existing row first so reentries no-op without
		// triggering a unique-constraint error in storage. The repo doesn't
		// expose GetByID with tenant scoping so we filter by entity.
		existing, err := docService.ListByEntity(ctx, payload.InspectionID, document.EntityInspection, payload.TenantID)
		if err == nil {
			for _, d := range existing {
				if d != nil && d.ID == id {
					return nil
				}
			}
		}

		now := time.Now()
		doc := &document.Document{
			ID:                id,
			TenantID:          payload.TenantID,
			Name:              name,
			FileURL:           payload.ReportURL,
			FileType:          "application/pdf",
			Category:          document.CategoryLaudo,
			Status:            document.StatusAssinado,
			EntityID:          payload.InspectionID,
			EntityType:        document.EntityInspection,
			ContractID:        payload.ContractID,
			Source:            document.SourceOps,
			UploadedBy:        "system:toca-ops",
			UploadDate:        now,
			PersistenceStatus: document.PersistencePending,
			Signers:           []document.DocumentSigner{},
			Tags:              []string{"vistoria", strings.ToLower(payload.Type)},
			Version:           1,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		return docService.Create(ctx, doc)
	}
}

// NewMaintenanceCompletedDocumentHandler creates a Comprovante Document for
// each attachment supplied with a completed maintenance ticket. Empty
// attachment lists ack without creating anything.
func NewMaintenanceCompletedDocumentHandler(docService *DocumentService, maintenanceLookup MaintenanceTicketLookup) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeMaintenanceCompletedPayloadForDocs(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" || payload.TicketID == "" {
			return nil
		}
		// Attachments are not in the typed event payload; fetch the ticket
		// via the lookup to keep the event surface stable. A nil lookup
		// skips the side effect entirely (useful in tests / phased rollout).
		if maintenanceLookup == nil {
			return nil
		}
		attachments, err := maintenanceLookup.GetAttachments(ctx, payload.TenantID, payload.TicketID)
		if err != nil {
			return fmt.Errorf("fetch maintenance attachments for ticket %s: %w", payload.TicketID, err)
		}
		if len(attachments) == 0 {
			return nil
		}

		now := time.Now()
		for idx, url := range attachments {
			if strings.TrimSpace(url) == "" {
				continue
			}
			id := events.DeriveNotificationID(payload.TicketID, "document", "attachment", fmt.Sprintf("%d", idx))
			// Idempotency check.
			existing, _ := docService.ListByEntity(ctx, payload.TicketID, "maintenance", payload.TenantID)
			alreadyExists := false
			for _, d := range existing {
				if d != nil && d.ID == id {
					alreadyExists = true
					break
				}
			}
			if alreadyExists {
				continue
			}
			doc := &document.Document{
				ID:                id,
				TenantID:          payload.TenantID,
				Name:              fmt.Sprintf("Comprovante Manutenção (#%s) %d", payload.TicketID, idx+1),
				FileURL:           url,
				FileType:          inferFileType(url),
				Category:          document.CategoryComprovante,
				Status:            document.StatusAssinado,
				EntityID:          payload.TicketID,
				EntityType:        "maintenance",
				ContractID:        payload.ContractID,
				Source:            document.SourceOps,
				UploadedBy:        "system:toca-ops",
				UploadDate:        now,
				PersistenceStatus: document.PersistencePending,
				Signers:           []document.DocumentSigner{},
				Tags:              []string{"manutencao", strings.ToLower(payload.Category)},
				Version:           1,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			if err := docService.Create(ctx, doc); err != nil {
				return fmt.Errorf("create comprovante document for ticket %s: %w", payload.TicketID, err)
			}
		}
		return nil
	}
}

// MaintenanceTicketLookup is the narrow surface a document-side subscriber
// needs from toca-ops to fetch the attachments list. Implemented by an
// HTTP client wrapper in cmd/server/main.go; defined as an interface so
// tests can stub it without spinning up the ops service.
type MaintenanceTicketLookup interface {
	GetAttachments(ctx context.Context, tenantID, ticketID string) ([]string, error)
}

func inspectionTypeLabel(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "ENTRY":
		return "Entrada"
	case "EXIT":
		return "Saída"
	case "PERIODIC":
		return "Periódica"
	default:
		return t
	}
}

func inferFileType(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".heic"):
		return "image/heic"
	default:
		return "application/octet-stream"
	}
}

// decodeMaintenanceCompletedPayloadForDocs mirrors the financial-side
// decoder but lives in this package to avoid a cross-package import for
// the helper. Same logic, same fallbacks.
func decodeMaintenanceCompletedPayloadForDocs(event events.Event) (contracts.MaintenanceCompletedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.MaintenanceCompletedEventPayload:
		return raw, nil
	case contracts.MaintenanceCompletedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.MaintenanceCompletedEventPayload{
			TicketID:        dealGetStr(raw, "ticket_id"),
			TenantID:        dealGetStr(raw, "tenant_id"),
			ContractID:      dealGetStr(raw, "contract_id"),
			RentalID:        dealGetStr(raw, "rental_id"),
			PropertyID:      dealGetStr(raw, "property_id"),
			RequesterLeadID: dealGetStr(raw, "requester_lead_id"),
			Category:        dealGetStr(raw, "category"),
			PayerRole:       dealGetStr(raw, "payer_role"),
			CompletedAt:     dealGetStr(raw, "completed_at"),
			Timestamp:       dealGetStr(raw, "timestamp"),
		}
		if v, ok := raw["actual_cost"].(float64); ok {
			out.ActualCost = v
		}
		if v, ok := raw["estimated_cost"].(float64); ok {
			out.EstimatedCost = v
		}
		return out, nil
	default:
		return contracts.MaintenanceCompletedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

// FinancialProofDownloader baixa os bytes de um comprovante do bucket
// `financial-proofs`. Implementado pelos providers de pkg/storage (Supabase/S3)
// que já satisfazem storage.DownloadService; definido como interface própria
// para os testes não precisarem subir storage.
type FinancialProofDownloader interface {
	Download(ctx context.Context, bucket, filename string) (io.ReadCloser, string, int64, error)
}

// NewFinancialProofDocumentHandler materializa um Document de categoria
// `comprovante` para cada comprovante financeiro enviado via /proofs. É o que
// faz o arquivo aparecer centralizado e classificado na Central de Documentos
// (/documentos) sob a origem "financeiro".
//
// Sem downloader (nil), o handler cria apenas o registro de metadados
// apontando para o caminho interno do comprovante; com downloader, os bytes
// são copiados para o bucket de documentos para que o preview/download da
// Central funcione sem depender do endpoint autenticado /proofs.
//
// Idempotente: o ID do documento é derivado do object_key, então uma
// redelivery do RabbitMQ (at-least-once) vira no-op.
func NewFinancialProofDocumentHandler(docService *DocumentService, proofDownloader FinancialProofDownloader) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeFinancialProofUploadedPayload(event)
		if err != nil {
			return err
		}
		if payload.TenantID == "" || payload.ObjectKey == "" || payload.URL == "" {
			return nil
		}

		id := events.DeriveNotificationID(payload.ObjectKey, "financial", "proof")
		if existing, gerr := docService.GetByID(ctx, payload.TenantID, id); gerr == nil && existing != nil {
			return nil // já materializado
		}

		kind := strings.ToLower(strings.TrimSpace(payload.Kind))
		if kind == "" {
			kind = "financeiro"
		}
		uploadedBy := strings.TrimSpace(payload.UploadedBy)
		if uploadedBy == "" {
			uploadedBy = "system:toca-financial"
		}
		fileType := strings.TrimSpace(payload.ContentType)
		if fileType == "" {
			fileType = inferFileType(payload.Filename)
		}

		now := time.Now()
		doc := &document.Document{
			ID:                id,
			TenantID:          payload.TenantID,
			Name:              fmt.Sprintf("Comprovante %s — %s", financialProofKindLabel(kind), payload.Filename),
			FileURL:           payload.URL,
			FileType:          fileType,
			FileSize:          payload.SizeBytes,
			Category:          document.CategoryComprovante,
			Status:            document.StatusPendente, // sem fluxo de assinatura → "não exige assinatura"
			EntityType:        document.EntityFinancial,
			Source:            document.SourceFinancial,
			UploadedBy:        uploadedBy,
			UploadDate:        now,
			PersistenceStatus: document.PersistencePending,
			Signers:           []document.DocumentSigner{},
			Tags:              []string{"financeiro", kind},
			Version:           1,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := docService.Create(ctx, doc); err != nil {
			return fmt.Errorf("create comprovante document for proof %s: %w", payload.ObjectKey, err)
		}

		// Materialização dos bytes: copia do bucket de comprovantes para o
		// bucket de documentos. Best-effort — sem downloader ou falha de
		// download/upload, o metadata permanece (referência pelo FileURL).
		if proofDownloader == nil {
			return nil
		}
		body, contentType, size, derr := proofDownloader.Download(ctx, storage.FinancialProofsBucket, payload.ObjectKey)
		if derr != nil {
			return nil
		}
		defer drainAndClose(body)
		if contentType == "" {
			contentType = fileType
		}
		if uerr := docService.Upload(ctx, doc, payload.Filename, contentType, size, body); uerr != nil {
			// Upload já carimbou PersistenceFailed/LastPersistError; persiste
			// o marcador para o backfill/operador enxergarem o documento.
			_ = docService.Update(ctx, doc)
			return nil
		}
		return docService.Update(ctx, doc)
	}
}

func financialProofKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "payout":
		return "Payout"
	case "charge":
		return "Cobrança"
	case "commission":
		return "Comissão"
	case "transaction":
		return "Lançamento"
	default:
		return kind
	}
}

func decodeFinancialProofUploadedPayload(event events.Event) (contracts.FinancialProofUploadedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.FinancialProofUploadedEventPayload:
		return raw, nil
	case contracts.FinancialProofUploadedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.FinancialProofUploadedEventPayload{
			TenantID:    dealGetStr(raw, "tenant_id"),
			Kind:        dealGetStr(raw, "kind"),
			URL:         dealGetStr(raw, "url"),
			Filename:    dealGetStr(raw, "filename"),
			ContentType: dealGetStr(raw, "content_type"),
			ObjectKey:   dealGetStr(raw, "object_key"),
			UploadedBy:  dealGetStr(raw, "uploaded_by"),
		}
		if v, ok := raw["size_bytes"].(float64); ok {
			out.SizeBytes = int64(v)
		}
		return out, nil
	default:
		return contracts.FinancialProofUploadedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

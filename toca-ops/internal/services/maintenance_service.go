package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
)

type MaintenanceService struct {
	repo     maintenance.Repository
	eventBus events.EventBus
	storage  storage.Service
}

func NewMaintenanceService(repo maintenance.Repository, eventBus events.EventBus) *MaintenanceService {
	return &MaintenanceService{repo: repo, eventBus: eventBus}
}

// WithStorage enables signed URLs for attachments returned to authenticated
// operators. Internal property projections remain raw for the client portal,
// which applies its own requester and visibility checks.
func (s *MaintenanceService) WithStorage(storageService storage.Service) *MaintenanceService {
	s.storage = storageService
	return s
}

func (s *MaintenanceService) Create(ctx context.Context, ticket *maintenance.MaintenanceTicket) error {
	if ticket.PropertyID == "" {
		return fmt.Errorf("property_id is required")
	}
	if err := s.repo.Create(ctx, ticket); err != nil {
		return err
	}
	s.publishRequested(ctx, ticket)
	if ticket.Status == maintenance.StatusConcluido {
		s.publishCompleted(ctx, ticket)
	}
	return nil
}

func (s *MaintenanceService) GetByID(ctx context.Context, tenantID, id string) (*maintenance.MaintenanceTicket, error) {
	ticket, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, fmt.Errorf("maintenance ticket not found")
	}
	return ticket, nil
}

// GetByIDForOperator returns a non-persisted view with private attachment
// keys replaced by short-lived URLs. The lookup is tenant-scoped, so the
// presigned URLs are only ever minted for the caller's own tenant.
func (s *MaintenanceService) GetByIDForOperator(ctx context.Context, tenantID, id string) (*maintenance.MaintenanceTicket, error) {
	ticket, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.withPresignedAttachments(ctx, ticket)
}

func (s *MaintenanceService) Update(ctx context.Context, ticket *maintenance.MaintenanceTicket) error {
	// ticket.TenantID vem do handler (que já leu o registro escopado pelo
	// tenant do chamador); a releitura repete o filtro para que a escrita
	// nunca use o tenant do próprio registro.
	existing, err := s.repo.GetByID(ctx, ticket.TenantID, ticket.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("maintenance ticket not found")
	}
	ticket.TenantID = existing.TenantID
	ticket.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(ctx, ticket); err != nil {
		return err
	}
	if existing.Status != maintenance.StatusConcluido && ticket.Status == maintenance.StatusConcluido {
		if ticket.CompletedAt == nil {
			now := time.Now()
			ticket.CompletedAt = &now
		}
		s.publishCompleted(ctx, ticket)
	}
	return nil
}

func (s *MaintenanceService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *MaintenanceService) ListByProperty(ctx context.Context, tenantID, propertyID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	return s.repo.ListByProperty(ctx, tenantID, propertyID, limit, offset)
}

func (s *MaintenanceService) List(ctx context.Context, tenantID, status string, requesterLeadID string, rentalID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	return s.repo.List(ctx, tenantID, status, requesterLeadID, rentalID, limit, offset)
}

// ListForOperator is the operator-facing counterpart of List. It leaves the
// stored metadata untouched while resolving the private attachment URLs for
// the browser response.
func (s *MaintenanceService) ListForOperator(ctx context.Context, tenantID, status string, requesterLeadID string, rentalID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	tickets, total, err := s.List(ctx, tenantID, status, requesterLeadID, rentalID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for i, ticket := range tickets {
		tickets[i], err = s.withPresignedAttachments(ctx, ticket)
		if err != nil {
			return nil, 0, err
		}
	}
	return tickets, total, nil
}

func (s *MaintenanceService) withPresignedAttachments(ctx context.Context, ticket *maintenance.MaintenanceTicket) (*maintenance.MaintenanceTicket, error) {
	if ticket == nil || s.storage == nil || len(ticket.Attachments) == 0 {
		return ticket, nil
	}

	view := *ticket
	keys := append([]string(nil), ticket.Attachments...)
	attachments := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		// Older records may contain the URL returned by local storage instead
		// of the object key. Recover the key before signing private documents.
		if legacyKey, ok := storage.ExtractUploadObjectKey("toca-documents", key); ok {
			key = legacyKey
		} else if strings.Contains(key, "://") || strings.HasPrefix(key, "/") {
			// Keep already resolved/external URLs backward-compatible and avoid
			// signing an URL a second time after a mixed-version deployment.
			attachments = append(attachments, key)
			continue
		}
		url, err := s.storage.GetPresignedURL(ctx, "toca-documents", key, 15*time.Minute)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, url)
	}
	view.Attachments = attachments
	return &view, nil
}

// validTransitions descreve o ciclo de vida do chamado. `Em Análise →
// Concluído` é permitido de propósito: o serviço pode ser resolvido sem uma
// passagem explícita por "Em Andamento" (era o 400 que impedia lançar o custo
// no financeiro a partir da tela de manutenção).
var validTransitions = map[string][]string{
	maintenance.StatusEmAnalise:   {maintenance.StatusEmAndamento, maintenance.StatusConcluido, maintenance.StatusCancelado},
	maintenance.StatusEmAndamento: {maintenance.StatusConcluido, maintenance.StatusCancelado},
	maintenance.StatusConcluido:   {},
	maintenance.StatusCancelado:   {},
}

func (s *MaintenanceService) ChangeStatus(ctx context.Context, tenantID, id, newStatus string) (*maintenance.MaintenanceTicket, error) {
	ticket, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, fmt.Errorf("maintenance ticket not found")
	}

	allowed, ok := validTransitions[ticket.Status]
	if !ok {
		return nil, fmt.Errorf("unknown current status: %s", ticket.Status)
	}

	valid := false
	for _, s := range allowed {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("invalid status transition from %s to %s", ticket.Status, newStatus)
	}

	previousStatus := ticket.Status
	ticket.Status = newStatus
	if newStatus == maintenance.StatusConcluido {
		now := time.Now()
		ticket.CompletedAt = &now
	}

	if err := s.repo.Update(ctx, ticket); err != nil {
		return nil, err
	}

	if previousStatus != maintenance.StatusConcluido && newStatus == maintenance.StatusConcluido {
		s.publishCompleted(ctx, ticket)
	}

	return ticket, nil
}

func (s *MaintenanceService) publishRequested(ctx context.Context, t *maintenance.MaintenanceTicket) {
	if s.eventBus == nil {
		return
	}
	evt := contracts.MaintenanceRequestedEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.MaintenanceRequestedEventPayload{
			TicketID:        t.ID,
			TenantID:        t.TenantID,
			PropertyID:      t.PropertyID,
			ContractID:      t.ContractID,
			RentalID:        t.RentalID,
			RequesterLeadID: t.RequesterLeadID,
			Category:        t.Category,
			Priority:        t.Priority,
			Description:     t.Description,
			EstimatedCost:   t.EstimatedCost,
			PayerRole:       t.PayerRole,
			BillingTarget:   t.BillingTarget,
			AdvancedBy:      t.AdvancedBy,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to publish maintenance.requested for ticket=%s: %v", t.ID, err)
	}
}

func (s *MaintenanceService) publishCompleted(ctx context.Context, t *maintenance.MaintenanceTicket) {
	if s.eventBus == nil {
		return
	}
	completedAt := ""
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}
	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:        t.ID,
			TenantID:        t.TenantID,
			ContractID:      t.ContractID,
			RentalID:        t.RentalID,
			PropertyID:      t.PropertyID,
			RequesterLeadID: t.RequesterLeadID,
			Category:        t.Category,
			PayerRole:       t.PayerRole,
			BillingTarget:   t.BillingTarget,
			AdvancedBy:      t.AdvancedBy,
			EstimatedCost:   t.EstimatedCost,
			ActualCost:      t.ActualCost,
			CompletedAt:     completedAt,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to publish maintenance.completed for ticket=%s: %v", t.ID, err)
	}
}

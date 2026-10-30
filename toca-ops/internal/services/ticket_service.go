package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-ops/internal/models/ticket"
)

type TicketService struct {
	repo     ticket.Repository
	eventBus events.EventBus
	storage  storage.Service
}

func NewTicketService(repo ticket.Repository, eventBus events.EventBus) *TicketService {
	return &TicketService{repo: repo, eventBus: eventBus}
}

// WithStorage enables signed URLs for attachments returned to authenticated
// operators. Internal callers keep using GetByIDInternal/ListByRequester so
// they continue receiving storage keys and can apply their own visibility
// rules before signing URLs for the client portal.
func (s *TicketService) WithStorage(storageService storage.Service) *TicketService {
	s.storage = storageService
	return s
}

type CreateTicketRequest struct {
	TenantID           string
	DealID             string
	PropertyID         string
	Type               ticket.TicketType
	Category           string
	Subject            string
	Description        string
	Priority           ticket.TicketPriority
	RequesterAccountID string
	RequesterName      string
	RequesterEmail     string
}

func (s *TicketService) Create(ctx context.Context, req CreateTicketRequest) (*ticket.Ticket, error) {
	if strings.TrimSpace(req.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(req.RequesterAccountID) == "" {
		return nil, fmt.Errorf("requester_account_id is required")
	}
	if strings.TrimSpace(req.Subject) == "" {
		return nil, fmt.Errorf("subject is required")
	}

	tType := req.Type
	if tType == "" {
		tType = ticket.TicketTypeGeneral
	}
	priority := req.Priority
	if priority == "" {
		priority = ticket.TicketPriorityMedium
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "general"
	}

	t := ticket.New(req.TenantID, strings.TrimSpace(req.DealID), strings.TrimSpace(req.PropertyID), tType, category, strings.TrimSpace(req.Subject), strings.TrimSpace(req.Description), priority, req.RequesterAccountID, strings.TrimSpace(req.RequesterName), strings.TrimSpace(req.RequesterEmail))
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "ticket.created",
		EntityType: "ticket",
		EntityID:   t.ID,
		After: audit.Snapshot(t,
			"id", "tenant_id", "deal_id", "property_id", "type", "category",
			"subject", "status", "priority", "requester_account_id",
		),
		Metadata: map[string]any{
			"requester_email": t.RequesterEmail,
		},
	})

	if s.eventBus != nil {
		evt := contracts.TicketCreatedEvent{
			Data: contracts.TicketCreatedEventPayload{
				TicketID:          t.ID,
				DealID:            t.DealID,
				TenantID:          t.TenantID,
				CustomerAccountID: t.RequesterAccountID,
				Type:              string(t.Type),
				Category:          t.Category,
				Subject:           t.Subject,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue ticket.created event for ticket=%s: %v", t.ID, err)
		}
	}

	return t, nil
}

func (s *TicketService) List(ctx context.Context, tenantID string, filters ticket.ListFilters) ([]*ticket.Ticket, int64, error) {
	if tenantID == "" {
		return nil, 0, fmt.Errorf("tenant_id is required")
	}
	return s.repo.List(ctx, tenantID, filters)
}

func (s *TicketService) GetByID(ctx context.Context, tenantID, id string) (*ticket.Ticket, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket not found")
	}
	return t, nil
}

// GetByIDForOperator returns a non-persisted view with private attachment
// keys replaced by short-lived URLs. Never mutate the repository projection:
// signed URLs must not be written back into ticket metadata.
func (s *TicketService) GetByIDForOperator(ctx context.Context, tenantID, id string) (*ticket.Ticket, error) {
	t, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.withPresignedAttachments(ctx, t)
}

// ListForOperator is the operator-facing counterpart of List. The internal
// requester projection intentionally remains raw for the portal service.
func (s *TicketService) ListForOperator(ctx context.Context, tenantID string, filters ticket.ListFilters) ([]*ticket.Ticket, int64, error) {
	tickets, total, err := s.List(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, err
	}
	for i, t := range tickets {
		tickets[i], err = s.withPresignedAttachments(ctx, t)
		if err != nil {
			return nil, 0, err
		}
	}
	return tickets, total, nil
}

func (s *TicketService) withPresignedAttachments(ctx context.Context, t *ticket.Ticket) (*ticket.Ticket, error) {
	if t == nil || s.storage == nil || len(t.Comments) == 0 {
		return t, nil
	}

	view := *t
	view.Comments = append([]ticket.Comment(nil), t.Comments...)
	for i := range view.Comments {
		keys := view.Comments[i].Attachments
		if len(keys) == 0 {
			continue
		}

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
		view.Comments[i].Attachments = attachments
	}
	return &view, nil
}

func (s *TicketService) GetByIDInternal(ctx context.Context, tenantID, id string) (*ticket.Ticket, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket not found")
	}
	return t, nil
}

func (s *TicketService) UpdateStatus(ctx context.Context, tenantID, id string, newStatus ticket.TicketStatus) (*ticket.Ticket, error) {
	t, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	oldStatus := t.Status
	t.Status = newStatus
	now := time.Now()
	if newStatus == ticket.TicketStatusResolved && t.ResolvedAt == nil {
		t.ResolvedAt = &now
	}
	if newStatus == ticket.TicketStatusClosed && t.ClosedAt == nil {
		t.ClosedAt = &now
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "ticket.status_changed",
		EntityType: "ticket",
		EntityID:   t.ID,
		Before:     map[string]any{"status": string(oldStatus)},
		After:      map[string]any{"status": string(newStatus)},
		Metadata: map[string]any{
			"resolved": newStatus == ticket.TicketStatusResolved,
			"closed":   newStatus == ticket.TicketStatusClosed,
		},
	})

	if s.eventBus != nil {
		evt := contracts.TicketStatusChangedEvent{
			Data: contracts.TicketStatusChangedEventPayload{
				TicketID:          t.ID,
				DealID:            t.DealID,
				TenantID:          t.TenantID,
				OldStatus:         string(oldStatus),
				NewStatus:         string(newStatus),
				CustomerAccountID: t.RequesterAccountID,
				AssignedToID:      t.AssignedToID,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue ticket.status_changed event for ticket=%s: %v", t.ID, err)
		}
	}

	return t, nil
}

func (s *TicketService) Assign(ctx context.Context, tenantID, id, assigneeID, assigneeName string) (*ticket.Ticket, error) {
	t, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	oldAssigneeID := t.AssignedToID
	oldStatus := t.Status
	t.AssignedToID = strings.TrimSpace(assigneeID)
	t.AssignedToName = strings.TrimSpace(assigneeName)
	if t.Status == ticket.TicketStatusOpen && t.AssignedToID != "" {
		t.Status = ticket.TicketStatusAssigned
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "ticket.assigned",
		EntityType: "ticket",
		EntityID:   t.ID,
		Before: map[string]any{
			"assigned_to_id": oldAssigneeID,
			"status":         string(oldStatus),
		},
		After: map[string]any{
			"assigned_to_id": t.AssignedToID,
			"status":         string(t.Status),
		},
		Metadata: map[string]any{
			"assignee_name": t.AssignedToName,
			"reassignment":  oldAssigneeID != "" && oldAssigneeID != t.AssignedToID,
		},
	})

	if s.eventBus != nil && t.AssignedToID != "" {
		evt := contracts.TicketAssignedEvent{
			Data: contracts.TicketAssignedEventPayload{
				TicketID:          t.ID,
				DealID:            t.DealID,
				TenantID:          t.TenantID,
				OldAssignedToID:   oldAssigneeID,
				AssignedToID:      t.AssignedToID,
				AssignedToName:    t.AssignedToName,
				CustomerAccountID: t.RequesterAccountID,
				Subject:           t.Subject,
				Priority:          string(t.Priority),
			},
		}
		if pubErr := s.eventBus.PublishReliable(ctx, evt); pubErr != nil {
			log.Printf("WARN: failed to enqueue ticket.assigned event for ticket=%s assignee=%s: %v", t.ID, t.AssignedToID, pubErr)
		}
	}

	return t, nil
}

type AddCommentRequest struct {
	AuthorID        string
	AuthorName      string
	AuthorType      string
	Content         string
	Attachments     []string
	Visibility      string
	ClientRequestID string
}

func (s *TicketService) AddComment(ctx context.Context, tenantID, id string, req AddCommentRequest) (*ticket.Comment, error) {
	t, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("content is required")
	}

	crid := strings.TrimSpace(req.ClientRequestID)
	if crid != "" {
		for i := range t.Comments {
			if t.Comments[i].ClientRequestID == crid {
				existing := t.Comments[i]
				return &existing, nil
			}
		}
	}

	authorType := strings.TrimSpace(req.AuthorType)
	if authorType == "" {
		authorType = "operator"
	}
	visibility := ticket.CommentVisibility(strings.TrimSpace(req.Visibility))
	if !visibility.IsValid() {
		visibility = ticket.CommentVisibilityPublic
	}
	c := ticket.NewCommentFull(
		strings.TrimSpace(req.AuthorID),
		strings.TrimSpace(req.AuthorName),
		authorType,
		strings.TrimSpace(req.Content),
		req.Attachments,
		visibility,
		crid,
	)
	t.Comments = append(t.Comments, c)

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "ticket.comment_added",
		EntityType: "ticket",
		EntityID:   t.ID,
		After: map[string]any{
			"comment_id":  c.ID,
			"author_id":   c.AuthorID,
			"author_type": c.AuthorType,
			"visibility":  string(c.Visibility),
		},
		Metadata: map[string]any{
			"attachments_count": len(c.Attachments),
			"is_internal":       c.EffectiveVisibility() == ticket.CommentVisibilityInternal,
		},
	})

	if s.eventBus != nil {
		evt := contracts.TicketCommentAddedEvent{
			Data: contracts.TicketCommentAddedEventPayload{
				TicketID:          t.ID,
				DealID:            t.DealID,
				TenantID:          t.TenantID,
				CommentID:         c.ID,
				AuthorID:          c.AuthorID,
				AuthorType:        c.AuthorType,
				CustomerAccountID: t.RequesterAccountID,
				AssignedToID:      t.AssignedToID,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue ticket.comment_added event for ticket=%s comment=%s: %v", t.ID, c.ID, err)
		}
	}

	return &c, nil
}

func (s *TicketService) ListByRequester(ctx context.Context, tenantID, accountID string) ([]*ticket.Ticket, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(accountID) == "" {
		return []*ticket.Ticket{}, nil
	}
	return s.repo.ListByRequester(ctx, tenantID, accountID)
}

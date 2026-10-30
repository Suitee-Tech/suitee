package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
)

// RentalService is the orchestration layer that creates the consolidated
// Rental aggregate when a RENT contract is signed. It DOES NOT replace
// RentalChargeConfigService — instead it composes with it: CreateFromDeal
// creates the Rental record (linking deal_id, owner_lead_id, tenant_lead_id,
// contract_id, property_id) and lets RentalChargeConfigService keep doing
// the recurring billing setup it already handles.
//
// Why a service vs adding to event_handler.go directly: the handler is
// already crowded and mixing transaction + transfer + charge + rental
// orchestration. The service keeps Rental's lifecycle isolated and
// testable, and makes future operations (close rental, rescind, transfer
// owner, etc.) straightforward to add.
type RentalService struct {
	repo     rental.Repository
	eventBus events.EventBus

	// db opt-in: when wired, CreateFromDeal uses a single transaction
	// to atomically insert the rental and the rental.created outbox
	// event row. With db == nil the legacy non-atomic path is used —
	// preserved for tests that don't stand up Postgres.
	db *database.PostgresDB
}

func NewRentalService(repo rental.Repository, eventBus events.EventBus) *RentalService {
	return &RentalService{repo: repo, eventBus: eventBus}
}

// WithDB opts into the atomic publish+commit path. Production wiring in
// cmd/server/main.go should call this so the rental.created event lives
// or dies with the rental row.
func (s *RentalService) WithDB(db *database.PostgresDB) *RentalService {
	s.db = db
	return s
}

// CreateFromDealInput collects every id the rental aggregate needs. Filled
// by the contract.signed event handler from the payload.
type CreateFromDealInput struct {
	TenantID, DealID, ContractID, PropertyID string
	TenantLeadID, OwnerLeadID, BrokerID      string
	StartDate, EndDate                       time.Time
	MonthlyValueCents                        int64
}

// CreateFromDeal creates a Rental and emits a `rental.created` event so
// downstream consumers (toca-comm portal notification, analytics, etc.) can
// react. Idempotent on (tenant_id, deal_id): if a Rental already exists for
// the deal, returns the existing one without re-emitting.
func (s *RentalService) CreateFromDeal(ctx context.Context, in CreateFromDealInput) (*rental.Rental, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("rental service not configured")
	}
	if existing, err := s.repo.GetByDealID(ctx, in.TenantID, in.DealID); err == nil && existing != nil {
		return existing, nil
	}

	r, err := rental.New(in.TenantID, in.DealID, in.ContractID, in.PropertyID, in.TenantLeadID, in.OwnerLeadID, in.BrokerID, in.StartDate, in.EndDate, in.MonthlyValueCents)
	if err != nil {
		return nil, fmt.Errorf("invalid rental: %w", err)
	}

	evt := contracts.RentalCreatedEvent{
		Data: contracts.RentalCreatedEventPayload{
			RentalID:     r.ID,
			TenantID:     r.TenantID,
			DealID:       r.DealID,
			ContractID:   r.ContractID,
			PropertyID:   r.PropertyID,
			TenantLeadID: r.TenantLeadID,
			OwnerLeadID:  r.OwnerLeadID,
			BrokerID:     r.BrokerID,
			Timestamp:    contracts.NowRFC3339(),
		},
	}

	// Atomic path (wired in production via WithDB): rental insert + outbox
	// row land in the same Postgres transaction. If RabbitMQ is later
	// unavailable the drainer picks the row up asynchronously, but at no
	// point can the system end up with a rental but no event (or vice
	// versa). See docs/architecture/data-consistency-audit.md §8.1.
	if s.db != nil {
		if rb, ok := s.eventBus.(*events.RabbitMQEventBus); ok && rb != nil {
			if err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
				if err := s.repo.CreateInTx(ctx, tx, r); err != nil {
					return fmt.Errorf("persist rental: %w", err)
				}
				if _, err := rb.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
					return fmt.Errorf("enqueue rental.created in tx: %w", err)
				}
				return nil
			}); err != nil {
				return nil, err
			}
			metrics.IncRentalCreated()
			audit.Record(ctx, audit.Entry{
				Action:     "create",
				EntityType: "Rental",
				EntityID:   r.ID,
				After: map[string]any{
					"deal_id":     r.DealID,
					"contract_id": r.ContractID,
					"property_id": r.PropertyID,
					"broker_id":   r.BrokerID,
				},
				Metadata: map[string]any{"path": "CreateFromDeal", "atomic": true},
			})
			return r, nil
		}
	}

	// Legacy non-atomic path (kept for InMemory bus and for tests that
	// don't wire a DB). The publish error is logged but doesn't fail the
	// orchestration — recoverable via reconciliation or polling.
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, fmt.Errorf("persist rental: %w", err)
	}
	if s.eventBus != nil {
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue rental.created event for rental=%s: %v", r.ID, err)
		}
	}
	metrics.IncRentalCreated()
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Rental",
		EntityID:   r.ID,
		After: map[string]any{
			"deal_id":     r.DealID,
			"contract_id": r.ContractID,
			"property_id": r.PropertyID,
			"broker_id":   r.BrokerID,
		},
		Metadata: map[string]any{"path": "CreateFromDeal", "atomic": false},
	})

	return r, nil
}

// GetByDealID is a thin pass-through used by handlers + tests.
func (s *RentalService) GetByDealID(ctx context.Context, tenantID, dealID string) (*rental.Rental, error) {
	return s.repo.GetByDealID(ctx, tenantID, dealID)
}

// GetByContractID resolves a rental by its contract_id. The admin UI links
// to /locacao/{contract.id}, so /rentals/:id must accept a contract id —
// see RentalHandler.GetByID for the multi-key resolution.
func (s *RentalService) GetByContractID(ctx context.Context, tenantID, contractID string) (*rental.Rental, error) {
	return s.repo.GetByContractID(ctx, tenantID, contractID)
}

// GetRentalByID returns the Rental by its own id (uuid). Used by the admin
// /rentals/:id detail endpoint; tenant-scoped at the repo level.
func (s *RentalService) GetRentalByID(ctx context.Context, tenantID, id string) (*rental.Rental, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// ListForLead returns rentals where the lead is the tenant or owner. Used by
// LeadDetails admin UX (Phase 4) and the portal (Phase 5).
func (s *RentalService) ListForLead(ctx context.Context, tenantID, leadID string, role rental.LeadRole) ([]*rental.Rental, error) {
	return s.repo.ListByLeadID(ctx, tenantID, leadID, role)
}

// ListForLeadIDs returns rentals where any of the supplied leadIDs is the
// tenant or owner. Used by the client-portal handler that converts the
// JWT's account_id into the set of leads the user can act as.
func (s *RentalService) ListForLeadIDs(ctx context.Context, tenantID string, leadIDs []string) ([]*rental.Rental, error) {
	return s.repo.ListForLeadIDs(ctx, tenantID, leadIDs)
}

// (RentalCreatedEvent removed — moved to pkg/events/contracts/rental_events.go
// in Phase 3 so subscribers in other modules can import the contract without
// pulling in toca-financial.)

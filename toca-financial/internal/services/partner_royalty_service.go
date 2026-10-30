package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
)

// PartnerRoyaltyRepository is the minimal persistence surface the
// contract.signed pipeline needs to record franchise royalty payables.
type PartnerRoyaltyRepository interface {
	Create(ctx context.Context, pr *partner_royalty.PartnerRoyalty) error
	GetByID(ctx context.Context, tenantID, id string) (*partner_royalty.PartnerRoyalty, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*partner_royalty.PartnerRoyalty, error)
	List(ctx context.Context, tenantID string, limit, offset int64) ([]*partner_royalty.PartnerRoyalty, int64, error)
}

type PartnerRoyaltyService struct {
	repo PartnerRoyaltyRepository
}

type partnerRoyaltyTransitionRepository interface {
	UpdateIfStatus(ctx context.Context, pr *partner_royalty.PartnerRoyalty, expected partner_royalty.Status, expectedUpdatedAt time.Time) error
}

type RoyaltyPayoutLink struct {
	RoyaltyID     string
	PayoutOrderID string
	PayerTenant   string
	PayeeTenant   string
	ContractID    string
	AmountCents   int64
}

func NewPartnerRoyaltyService(repo PartnerRoyaltyRepository) *PartnerRoyaltyService {
	return &PartnerRoyaltyService{repo: repo}
}

// Create records a PENDING royalty payable owed by the franchisee
// (payerTenant) to the franchisor (payeeTenant).
func (s *PartnerRoyaltyService) Create(ctx context.Context, partnershipID, dealID, contractID, payerTenant, payeeTenant string, baseAmount, percentage float64) (*partner_royalty.PartnerRoyalty, error) {
	pr := partner_royalty.New(partnershipID, dealID, contractID, payerTenant, payeeTenant, baseAmount, percentage)
	if err := s.repo.Create(ctx, pr); err != nil {
		return nil, err
	}
	return pr, nil
}

func (s *PartnerRoyaltyService) CreateFromEvent(ctx context.Context, sourceEventID, partnershipID, dealID, contractID, payerTenant, payeeTenant string, baseAmount, percentage float64, kind ...string) (*partner_royalty.PartnerRoyalty, error) {
	pr := partner_royalty.New(partnershipID, dealID, contractID, payerTenant, payeeTenant, baseAmount, percentage)
	pr.SourceEventID = sourceEventID
	if len(kind) > 0 {
		pr.Kind = kind[0]
		if pr.Kind != "admin_fee" && pr.Kind != "transaction_fee" {
			return nil, fmt.Errorf("invalid partnership obligation kind")
		}
	}
	if err := s.repo.Create(ctx, pr); err != nil {
		if errors.Is(err, partner_royalty.ErrPartnerRoyaltyAlreadyExists) && sourceEventID != "" {
			existing, gerr := s.repo.GetBySourceEventID(ctx, pr.TenantID, sourceEventID)
			if gerr != nil {
				return nil, gerr
			}
			if existing != nil {
				if pr.Kind != "" && (existing.Kind != pr.Kind || existing.PayerTenant != pr.PayerTenant || existing.PayeeTenant != pr.PayeeTenant || existing.DealID != pr.DealID || existing.PartnershipID != pr.PartnershipID || math.Round(existing.Amount*100) != math.Round(pr.Amount*100)) {
					return nil, fmt.Errorf("partnership fee conflicts with existing obligation")
				}
				return existing, nil
			}
		}
		return nil, err
	}
	return pr, nil
}

func (s *PartnerRoyaltyService) GetByID(ctx context.Context, tenantID, id string) (*partner_royalty.PartnerRoyalty, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *PartnerRoyaltyService) List(ctx context.Context, tenantID string, limit, offset int64) ([]*partner_royalty.PartnerRoyalty, int64, error) {
	return s.repo.List(ctx, tenantID, limit, offset)
}

func (s *PartnerRoyaltyService) transitionFromPayout(ctx context.Context, link RoyaltyPayoutLink, mutate func(*partner_royalty.PartnerRoyalty) error) (*partner_royalty.PartnerRoyalty, error) {
	link.RoyaltyID = strings.TrimSpace(link.RoyaltyID)
	link.PayoutOrderID = strings.TrimSpace(link.PayoutOrderID)
	if link.RoyaltyID == "" || link.PayoutOrderID == "" || link.PayeeTenant == "" {
		return nil, partner_royalty.ErrPayoutSourceMismatch
	}
	pr, err := s.repo.GetByID(ctx, link.PayeeTenant, link.RoyaltyID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, fmt.Errorf("partner royalty %s not found", link.RoyaltyID)
	}
	if pr.ID != link.RoyaltyID || pr.TenantID != link.PayeeTenant || pr.PayerTenant != link.PayerTenant || pr.PayeeTenant != link.PayeeTenant ||
		pr.ContractID != link.ContractID || int64(math.Round(pr.Amount*100)) != link.AmountCents {
		return nil, partner_royalty.ErrPayoutSourceMismatch
	}
	if pr.PayoutOrderID != "" && pr.PayoutOrderID != link.PayoutOrderID {
		return nil, partner_royalty.ErrPayoutLinkMismatch
	}
	expectedStatus := pr.Status
	expectedUpdatedAt := pr.UpdatedAt
	previousPayoutOrderID := pr.PayoutOrderID
	pr.PayoutOrderID = link.PayoutOrderID
	if err := mutate(pr); err != nil {
		return nil, err
	}
	if pr.Status == expectedStatus && pr.PayoutOrderID == previousPayoutOrderID {
		return pr, nil
	}
	repo, ok := s.repo.(partnerRoyaltyTransitionRepository)
	if !ok {
		return nil, errors.New("partner royalty repository does not support compare-and-set transitions")
	}
	if err := repo.UpdateIfStatus(ctx, pr, expectedStatus, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return pr, nil
}

func (s *PartnerRoyaltyService) ApproveFromPayout(ctx context.Context, link RoyaltyPayoutLink) (*partner_royalty.PartnerRoyalty, error) {
	return s.transitionFromPayout(ctx, link, func(pr *partner_royalty.PartnerRoyalty) error { return pr.Approve() })
}

func (s *PartnerRoyaltyService) MarkPaidFromPayout(ctx context.Context, link RoyaltyPayoutLink) (*partner_royalty.PartnerRoyalty, error) {
	return s.transitionFromPayout(ctx, link, func(pr *partner_royalty.PartnerRoyalty) error { return pr.MarkPaid() })
}

// ReopenFromPayout é o espelho de CancelFromPayout: devolve o royalty a
// PENDING e o religa à ordem regerada, para a obrigação poder ser lançada de
// novo.
func (s *PartnerRoyaltyService) ReopenFromPayout(ctx context.Context, link RoyaltyPayoutLink) (*partner_royalty.PartnerRoyalty, error) {
	return s.transitionFromPayout(ctx, link, func(pr *partner_royalty.PartnerRoyalty) error { return pr.Reopen() })
}

func (s *PartnerRoyaltyService) CancelFromPayout(ctx context.Context, link RoyaltyPayoutLink) (*partner_royalty.PartnerRoyalty, error) {
	return s.transitionFromPayout(ctx, link, func(pr *partner_royalty.PartnerRoyalty) error { return pr.Cancel() })
}

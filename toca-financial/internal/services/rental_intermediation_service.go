package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// rentalIntermediationAtomicRepository serializes funding for one contract.
// PostgreSQL implements it with SELECT ... FOR UPDATE so two monthly payment
// commands can never consume the same remaining commission target.
type rentalIntermediationAtomicRepository interface {
	AllocateIntermediationFunding(
		ctx context.Context,
		tenantID, contractID, paymentID, chargeID, referenceMonth string,
		availableCents int64,
		fundedAt time.Time,
	) (*rental_charge_config.RentalChargeConfig, rental_charge_config.IntermediationFundingEntry, bool, error)
}

// RentalIntermediationService coordinates the frozen rental plan, Commission
// aggregates and payout liquidity. Production calls FundPayment inside the
// same database transaction that persists the Charge payment and journal.
type RentalIntermediationService struct {
	configRepo       rental_charge_config.Repository
	commission       *CommissionService
	payoutRepo       payout_order.Repository
	requirePaymentTx bool
}

func NewRentalIntermediationService(configRepo rental_charge_config.Repository, commissionService *CommissionService, payoutRepo payout_order.Repository) *RentalIntermediationService {
	return &RentalIntermediationService{configRepo: configRepo, commission: commissionService, payoutRepo: payoutRepo}
}

// RequirePaymentTransaction makes funding fail closed unless it participates
// in the canonical Charge payment unit of work.
func (s *RentalIntermediationService) RequirePaymentTransaction() *RentalIntermediationService {
	s.requirePaymentTx = true
	return s
}

// FreezePlan persists the canonical first-rent intermediation target after the
// commission resolver has applied tenant defaults and participant rules.
func (s *RentalIntermediationService) FreezePlan(ctx context.Context, tenantID, contractID, dealID string, plan ResolvedCommissionPlan) error {
	if plan.TotalAmountCents <= 0 {
		return nil
	}
	if s == nil || s.configRepo == nil {
		return errors.New("rental intermediation config repository is required")
	}
	cfg, err := s.configRepo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("rental charge config not found for contract %s", contractID)
	}
	shares := make([]rental_charge_config.IntermediationShare, 0, len(plan.Shares))
	for _, share := range plan.Shares {
		if share.AmountCents <= 0 {
			continue
		}
		shares = append(shares, rental_charge_config.IntermediationShare{
			BrokerID: share.BrokerID, Role: share.Role, TargetAmountCents: share.AmountCents,
		})
	}
	changed, err := cfg.FreezeIntermediationPlan(rental_charge_config.IntermediationPlan{
		DealID: dealID, PlanHash: plan.PlanHash, TotalAmountCents: plan.TotalAmountCents, Shares: shares,
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return s.configRepo.Update(ctx, cfg)
}

func (s *RentalIntermediationService) LinkCommission(ctx context.Context, tenantID, contractID, brokerID, role, commissionID string) error {
	return s.linkAggregate(ctx, tenantID, contractID, brokerID, role, commissionID, true)
}

func (s *RentalIntermediationService) LinkPayout(ctx context.Context, tenantID, contractID, brokerID, role, payoutOrderID string) error {
	return s.linkAggregate(ctx, tenantID, contractID, brokerID, role, payoutOrderID, false)
}

func (s *RentalIntermediationService) linkAggregate(ctx context.Context, tenantID, contractID, brokerID, role, aggregateID string, commissionLink bool) error {
	if s == nil || s.configRepo == nil {
		return errors.New("rental intermediation config repository is required")
	}
	cfg, err := s.configRepo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("rental charge config not found for contract %s", contractID)
	}
	var changed bool
	if commissionLink {
		changed, err = cfg.LinkIntermediationCommission(brokerID, role, aggregateID)
	} else {
		changed, err = cfg.LinkIntermediationPayout(brokerID, role, aggregateID)
	}
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return s.configRepo.Update(ctx, cfg)
}

// FundPayment consumes the rent left after administration fees, applies the
// frozen first-rent waterfall and synchronizes Commission/Payout funding. The
// returned entry is also embedded into Charge allocations by the caller.
func (s *RentalIntermediationService) FundPayment(
	ctx context.Context,
	tenantID, contractID, paymentID, chargeID, referenceMonth string,
	availableCents int64,
	fundedAt time.Time,
) (rental_charge_config.IntermediationFundingEntry, error) {
	if s == nil || s.configRepo == nil {
		return rental_charge_config.IntermediationFundingEntry{}, nil
	}
	if s.requirePaymentTx && database.TxFromCtx(ctx) == nil {
		return rental_charge_config.IntermediationFundingEntry{}, errors.New("rental intermediation funding requires the charge payment transaction")
	}

	var cfg *rental_charge_config.RentalChargeConfig
	var entry rental_charge_config.IntermediationFundingEntry
	var err error
	if atomicRepo, ok := s.configRepo.(rentalIntermediationAtomicRepository); ok {
		cfg, entry, _, err = atomicRepo.AllocateIntermediationFunding(
			ctx, tenantID, contractID, paymentID, chargeID, referenceMonth, availableCents, fundedAt,
		)
	} else {
		cfg, err = s.configRepo.GetByContractID(ctx, tenantID, contractID)
		if err == nil && cfg != nil {
			var changed bool
			entry, changed, err = cfg.AllocateIntermediationFunding(paymentID, chargeID, referenceMonth, availableCents, fundedAt)
			if err == nil && changed {
				err = s.configRepo.Update(ctx, cfg)
			}
		}
	}
	if err != nil {
		return rental_charge_config.IntermediationFundingEntry{}, err
	}
	if cfg == nil || entry.AppliedCents <= 0 {
		return rental_charge_config.IntermediationFundingEntry{}, nil
	}
	if s.commission == nil || s.payoutRepo == nil {
		return rental_charge_config.IntermediationFundingEntry{}, errors.New("commission and payout funding dependencies are required")
	}

	for _, funded := range entry.BrokerFunding {
		if strings.TrimSpace(funded.CommissionID) == "" || strings.TrimSpace(funded.PayoutOrderID) == "" {
			return rental_charge_config.IntermediationFundingEntry{}, fmt.Errorf("rental intermediation share %s/%s is not linked to commission and payout", funded.BrokerID, funded.Role)
		}
		fundingEntry := commission.FundingEntry{
			PaymentID: entry.PaymentID, ChargeID: entry.ChargeID, ReferenceMonth: entry.ReferenceMonth,
			AmountCents: funded.AmountCents, FundedAt: entry.FundedAt,
		}
		if _, err := s.commission.RecordRentalFunding(ctx, tenantID, funded.CommissionID, fundingEntry); err != nil {
			return rental_charge_config.IntermediationFundingEntry{}, fmt.Errorf("fund rental commission %s: %w", funded.CommissionID, err)
		}
		if err := s.recordPayoutFunding(ctx, tenantID, funded.PayoutOrderID, payout_order.FundingEntry{
			PaymentID: entry.PaymentID, ChargeID: entry.ChargeID, ReferenceMonth: entry.ReferenceMonth,
			AmountCents: funded.AmountCents, FundedAt: entry.FundedAt,
		}); err != nil {
			return rental_charge_config.IntermediationFundingEntry{}, fmt.Errorf("fund rental payout %s: %w", funded.PayoutOrderID, err)
		}
	}
	return entry, nil
}

func (s *RentalIntermediationService) recordPayoutFunding(ctx context.Context, tenantID, payoutID string, entry payout_order.FundingEntry) error {
	order, err := s.payoutRepo.GetByID(ctx, tenantID, payoutID)
	if err != nil {
		return err
	}
	if order == nil {
		return errors.New("payout order not found")
	}
	expectedStatus := order.Status
	expectedUpdatedAt := order.UpdatedAt
	changed, err := order.RecordFunding(entry)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	repo, ok := s.payoutRepo.(payout_order.TransitionRepository)
	if !ok {
		return payout_order.ErrTransitionCompareAndSetMissing
	}
	return repo.UpdateIfStatus(ctx, order, expectedStatus, expectedUpdatedAt)
}

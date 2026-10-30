package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/utility_account"
)

type UtilityAccountService struct {
	repo utility_account.Repository
}

func NewUtilityAccountService(repo utility_account.Repository) *UtilityAccountService {
	return &UtilityAccountService{repo: repo}
}

type CreateUtilityAccountInput struct {
	ContractID       string
	PropertyID       string
	Type             utility_account.Type
	Provider         string
	AccountNumber    string
	ResponsibleParty utility_account.ResponsibleParty
	EstimatedAmount  int64
	PaymentMethod    string
	AutoPayEnabled   bool
	DueDay           int
	NextDueDate      *time.Time
	Notes            string
	BillingMode      utility_account.BillingMode
	BundledItemKey   string
}

func (s *UtilityAccountService) Create(ctx context.Context, tenantID string, in CreateUtilityAccountInput) (*utility_account.UtilityAccount, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id required")
	}
	a, err := utility_account.New(tenantID, in.ContractID, in.PropertyID, in.Type, in.Provider, in.AccountNumber, in.ResponsibleParty, in.EstimatedAmount)
	if err != nil {
		return nil, err
	}
	a.PaymentMethod = strings.TrimSpace(in.PaymentMethod)
	a.AutoPayEnabled = in.AutoPayEnabled
	if in.DueDay > 0 && in.DueDay <= 31 {
		a.DueDay = in.DueDay
	}
	a.NextDueDate = in.NextDueDate
	a.Notes = strings.TrimSpace(in.Notes)
	billingMode := in.BillingMode
	if billingMode == "" {
		billingMode = utility_account.BillingModeInformational
	}
	if err := utility_account.ValidateBillingMode(billingMode); err != nil {
		return nil, err
	}
	bundledItemKey := strings.TrimSpace(in.BundledItemKey)
	if billingMode == utility_account.BillingModeBundled && bundledItemKey == "" {
		return nil, errors.New("bundled_item_key is required for BUNDLED billing mode")
	}
	if billingMode != utility_account.BillingModeBundled {
		bundledItemKey = ""
	}
	a.BillingMode = billingMode
	a.BundledItemKey = bundledItemKey
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

type UpdateUtilityAccountInput struct {
	Provider         *string
	AccountNumber    *string
	ResponsibleParty *utility_account.ResponsibleParty
	EstimatedAmount  *int64
	PaymentMethod    *string
	AutoPayEnabled   *bool
	DueDay           *int
	NextDueDate      *time.Time
	LastPaymentDate  *time.Time
	Status           *utility_account.Status
	Notes            *string
	BillingMode      *utility_account.BillingMode
	BundledItemKey   *string
}

func (s *UtilityAccountService) Update(ctx context.Context, tenantID, id string, in UpdateUtilityAccountInput) (*utility_account.UtilityAccount, error) {
	a, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("utility account not found")
	}
	if in.Provider != nil && strings.TrimSpace(*in.Provider) != "" {
		a.Provider = strings.TrimSpace(*in.Provider)
	}
	if in.AccountNumber != nil {
		a.AccountNumber = strings.TrimSpace(*in.AccountNumber)
	}
	if in.ResponsibleParty != nil {
		if err := utility_account.ValidateResponsible(*in.ResponsibleParty); err != nil {
			return nil, err
		}
		a.ResponsibleParty = *in.ResponsibleParty
	}
	if in.EstimatedAmount != nil && *in.EstimatedAmount >= 0 {
		a.EstimatedAmount = *in.EstimatedAmount
	}
	if in.PaymentMethod != nil {
		a.PaymentMethod = strings.TrimSpace(*in.PaymentMethod)
	}
	if in.AutoPayEnabled != nil {
		a.AutoPayEnabled = *in.AutoPayEnabled
	}
	if in.DueDay != nil && *in.DueDay > 0 && *in.DueDay <= 31 {
		a.DueDay = *in.DueDay
	}
	if in.NextDueDate != nil {
		a.NextDueDate = in.NextDueDate
	}
	if in.LastPaymentDate != nil {
		a.LastPaymentDate = in.LastPaymentDate
	}
	if in.Status != nil && (*in.Status == utility_account.StatusActive || *in.Status == utility_account.StatusInactive) {
		a.Status = *in.Status
	}
	if in.Notes != nil {
		a.Notes = strings.TrimSpace(*in.Notes)
	}
	if a.BillingMode == "" {
		a.BillingMode = utility_account.BillingModeInformational
	}
	if in.BillingMode != nil {
		if err := utility_account.ValidateBillingMode(*in.BillingMode); err != nil {
			return nil, err
		}
		a.BillingMode = *in.BillingMode
		if a.BillingMode == "" {
			a.BillingMode = utility_account.BillingModeInformational
		}
	}
	if in.BundledItemKey != nil {
		a.BundledItemKey = strings.TrimSpace(*in.BundledItemKey)
	}
	if a.BillingMode == utility_account.BillingModeBundled && a.BundledItemKey == "" {
		return nil, errors.New("bundled_item_key is required for BUNDLED billing mode")
	}
	if a.BillingMode != utility_account.BillingModeBundled {
		a.BundledItemKey = ""
	}
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *UtilityAccountService) ListByContract(ctx context.Context, tenantID, contractID string) ([]*utility_account.UtilityAccount, error) {
	return s.repo.ListByContract(ctx, tenantID, contractID)
}

func (s *UtilityAccountService) GetByID(ctx context.Context, tenantID, id string) (*utility_account.UtilityAccount, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *UtilityAccountService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

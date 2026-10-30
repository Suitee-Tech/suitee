package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

var errFinancialSettingsUnavailable = errors.New("financial settings unavailable")

type failingFinancialSettingsRepo struct{}

func (failingFinancialSettingsRepo) Get(context.Context, string) (*tenant_financial_settings.Settings, error) {
	return nil, errFinancialSettingsUnavailable
}

func (failingFinancialSettingsRepo) Upsert(context.Context, *tenant_financial_settings.Settings) error {
	return nil
}

type fixedFinancialSettingsRepo struct {
	settings *tenant_financial_settings.Settings
}

func (r fixedFinancialSettingsRepo) Get(context.Context, string) (*tenant_financial_settings.Settings, error) {
	return r.settings, nil
}

func (fixedFinancialSettingsRepo) Upsert(context.Context, *tenant_financial_settings.Settings) error {
	return nil
}

type settingsFailureEvent struct {
	payload map[string]interface{}
}

func (settingsFailureEvent) EventID() string        { return "evt-settings-failure" }
func (settingsFailureEvent) Name() string           { return "contract.signed" }
func (e settingsFailureEvent) Payload() interface{} { return e.payload }

func TestContractSignedDoesNotFreezeFallbackWhenSettingsLookupFails(t *testing.T) {
	settings := NewTenantFinancialSettingsService(failingFinancialSettingsRepo{})
	handler := NewContractSignedEventHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settings, nil)

	err := handler(context.Background(), settingsFailureEvent{payload: map[string]interface{}{
		"contract_id":    "contract-1",
		"deal_id":        "deal-1",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          1000.0,
		"commission_pct": 0.0,
		"customer_id":    "customer-1",
		"owner_id":       "owner-1",
	}})

	if !errors.Is(err, errFinancialSettingsUnavailable) {
		t.Fatalf("contract.signed error = %v, want settings lookup failure", err)
	}
}

func TestContractSignedServiceRoleRejectsTenantHeaderMismatch(t *testing.T) {
	handler := NewContractSignedEventHandler(&database.PostgresDB{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := pkgctx.WithTenantID(context.Background(), "tenant-header")

	err := handler(ctx, settingsFailureEvent{payload: map[string]interface{}{
		"contract_id":    "contract-1",
		"deal_id":        "deal-1",
		"tenant_id":      "tenant-payload",
		"type":           "SALE",
		"value":          1000.0,
		"commission_pct": 6.0,
		"customer_id":    "customer-1",
		"owner_id":       "owner-1",
	}})

	if err == nil || !strings.Contains(err.Error(), "tenant mismatch") {
		t.Fatalf("contract.signed error = %v, want tenant mismatch", err)
	}
}

func TestFinancialPreviewDoesNotReturnFallbackWhenSettingsLookupFails(t *testing.T) {
	settings := NewTenantFinancialSettingsService(failingFinancialSettingsRepo{})
	svc := NewFinancialPreviewService(nil, nil, nil, nil, settings)

	_, err := svc.previewFromDeal(context.Background(), "tenant-1", &clients.DealDTO{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Type:     "SALE",
		Value:    1000,
	}, "contract-1")

	if !errors.Is(err, errFinancialSettingsUnavailable) {
		t.Fatalf("preview error = %v, want settings lookup failure", err)
	}
}

func TestDelinquencyDoesNotMutateChargeWhenSettingsLookupFails(t *testing.T) {
	c := makeChargeDue(t, "charge-1", time.Now().Add(-10*24*time.Hour), charge.StatusPending)
	repo := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}},
		tenantIDs:      []string{"tenant-1"},
	}
	svc := NewDelinquencyService(repo, &stubCollectionRepo{}, nil, DefaultDelinquencyConfig())
	svc.SetTenantSettings(NewTenantFinancialSettingsService(failingFinancialSettingsRepo{}))

	result, err := svc.ProcessOverdueCharges(context.Background())
	if err != nil {
		t.Fatalf("ProcessOverdueCharges() error = %v", err)
	}
	if c.Status != charge.StatusPending || c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 {
		t.Fatalf("charge mutated after lookup failure: status=%s late=%d interest=%d", c.Status, c.LateFee.Cents(), c.Interest.Cents())
	}
	if repo.updateCalls != 0 || result.LateFessCalculated != 0 {
		t.Fatalf("lookup failure persisted/counted: updates=%d result=%+v", repo.updateCalls, result)
	}
}

func TestDelinquencyHonorsExplicitZeroTenantRates(t *testing.T) {
	settings := tenant_financial_settings.New("tenant-1")
	settings.DefaultLateFeePct = 0
	settings.DefaultMonthlyInterestPct = 0
	svc := NewDelinquencyService(nil, nil, nil, DefaultDelinquencyConfig())
	svc.SetTenantSettings(NewTenantFinancialSettingsService(fixedFinancialSettingsRepo{settings: settings}))
	c := makeChargeDue(t, "charge-zero-rates", time.Now().Add(-10*24*time.Hour), charge.StatusPending)

	lateFee, interest, err := svc.rateForCharge(context.Background(), c)
	if err != nil {
		t.Fatalf("rateForCharge() error = %v", err)
	}
	if lateFee != 0 || interest != 0 {
		t.Fatalf("tenant zero rates = (%.2f, %.2f), want (0, 0)", lateFee, interest)
	}
}

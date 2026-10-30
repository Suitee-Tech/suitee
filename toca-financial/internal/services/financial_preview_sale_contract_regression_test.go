package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

type frozenValueSalePlanRepo struct {
	plan             *sale_billing_plan.SaleBillingPlan
	lookupTenantID   string
	lookupContractID string
}

type previewDealRoundTrip func(*http.Request) (*http.Response, error)

func (f previewDealRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (r *frozenValueSalePlanRepo) Create(_ context.Context, plan *sale_billing_plan.SaleBillingPlan) (*sale_billing_plan.SaleBillingPlan, error) {
	if r.plan != nil {
		return r.plan, nil
	}
	r.plan = plan
	return plan, nil
}

func (r *frozenValueSalePlanRepo) GetByID(_ context.Context, tenantID, id string) (*sale_billing_plan.SaleBillingPlan, error) {
	if r.plan != nil && r.plan.TenantID == tenantID && r.plan.ID == id {
		return r.plan, nil
	}
	return nil, nil
}

func (r *frozenValueSalePlanRepo) GetByContractID(_ context.Context, tenantID, contractID string) (*sale_billing_plan.SaleBillingPlan, error) {
	r.lookupTenantID = tenantID
	r.lookupContractID = contractID
	if r.plan != nil && r.plan.TenantID == tenantID && r.plan.ContractID == contractID {
		return r.plan, nil
	}
	return nil, nil
}

func (r *frozenValueSalePlanRepo) Update(_ context.Context, plan *sale_billing_plan.SaleBillingPlan) error {
	r.plan = plan
	return nil
}

func (r *frozenValueSalePlanRepo) Delete(_ context.Context, _, _ string) error { return nil }

func TestFinancialPreviewByContractUsesFrozenSaleValueAfterDealChanges(t *testing.T) {
	firstDue := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	plan, err := sale_billing_plan.New(
		"tenant-frozen",
		"contract-frozen",
		"deal-frozen",
		"buyer-frozen",
		"owner-frozen",
		"property-frozen",
		sale_billing_plan.ModeSingle,
		100000,
		0,
		100000,
		1,
		firstDue,
		30,
		sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		t.Fatalf("build materialized plan: %v", err)
	}
	planRepo := &frozenValueSalePlanRepo{plan: plan}

	transport := previewDealRoundTrip(func(r *http.Request) (*http.Response, error) {
		var data any
		switch r.URL.Path {
		case "/api/v1/deal/internal/contracts/contract-frozen":
			data = clients.ContractDTO{
				ID:         "contract-frozen",
				DealID:     "deal-frozen",
				TenantID:   "tenant-frozen",
				Type:       "venda",
				Value:      1000,
				StartDate:  firstDue,
				PropertyID: "property-frozen",
			}
		case "/api/v1/deal/internal/deals/deal-frozen":
			// The mutable deal has drifted after signature. It must not replace
			// the contract's frozen R$ 1.000,00 financial snapshot.
			data = clients.DealDTO{
				ID:         "deal-frozen",
				TenantID:   "tenant-frozen",
				Type:       "SALE",
				Value:      1200,
				CustomerID: "buyer-frozen",
				OwnerID:    "owner-frozen",
				PropertyID: "property-frozen",
			}
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    r,
			}, nil
		}
		payload, err := json.Marshal(map[string]any{"data": data})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Request:    r,
		}, nil
	})

	baseClient := clients.NewBaseClient("http://deal.test")
	baseClient.InternalAPIKey = "test-internal-key"
	baseClient.HTTPClient = &http.Client{Transport: transport}
	dealClient := &clients.DealClient{BaseClient: baseClient}
	previewService := NewFinancialPreviewService(
		dealClient,
		nil,
		NewSaleBillingPlanService(planRepo, nil),
		nil,
		nil,
	)

	preview, err := previewService.PreviewByContract(context.Background(), "tenant-frozen", "contract-frozen")
	if err != nil {
		t.Fatalf("preview frozen contract: %v", err)
	}
	if preview.TotalChargesCents != 100000 || len(preview.Charges) != 1 {
		t.Fatalf("expected materialized frozen plan totaling 100000 cents, got total=%d charges=%d", preview.TotalChargesCents, len(preview.Charges))
	}
	if planRepo.lookupTenantID != "tenant-frozen" || planRepo.lookupContractID != "contract-frozen" {
		t.Fatalf("materialized plan lookup lost tenant/contract scope: tenant=%q contract=%q", planRepo.lookupTenantID, planRepo.lookupContractID)
	}
}

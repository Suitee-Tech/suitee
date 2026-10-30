package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

type fakeApprovalPolicyProvider struct {
	policy clients.ApprovalPolicy
	err    error
	calls  int
}

func (f *fakeApprovalPolicyProvider) GetApprovalPolicy(context.Context, string) (clients.ApprovalPolicy, error) {
	f.calls++
	return f.policy, f.err
}

func tenantApprovalPolicy(threshold float64) clients.ApprovalPolicy {
	return clients.ApprovalPolicy{
		SchemaVersion:      1,
		Revision:           9,
		Enabled:            true,
		ThresholdBRL:       threshold,
		TargetStatuses:     []string{"ANALYSIS", "CONTRACT"},
		RequiredPermission: "deals:approve",
		Source:             "tenant_settings",
		ResolutionSource:   clients.ApprovalPolicyResolutionRemote,
	}
}

// TestRequiresApproval cobre a função pura que decide se um movimento
// para o status alvo demanda permission `deals:approve`. Sem tocar em
// I/O ela vai direto ao ponto.
func TestRequiresApproval(t *testing.T) {
	// Restore global após mexer (paralelo desligado nos subtests para
	// evitar race no env var leitura cache).
	originalThreshold := approvalThresholdBRL
	defer func() { approvalThresholdBRL = originalThreshold }()

	tests := []struct {
		name      string
		threshold float64
		dealValue float64
		target    deal.Status
		want      bool
	}{
		{
			name:      "high value to CONTRACT requires approval",
			threshold: 500_000,
			dealValue: 750_000,
			target:    deal.StatusContract,
			want:      true,
		},
		{
			name:      "high value to ANALYSIS requires approval",
			threshold: 500_000,
			dealValue: 750_000,
			target:    deal.StatusAnalysis,
			want:      true,
		},
		{
			name:      "low value never requires",
			threshold: 500_000,
			dealValue: 100_000,
			target:    deal.StatusContract,
			want:      false,
		},
		{
			name:      "exactly at threshold does NOT require (strict gt)",
			threshold: 500_000,
			dealValue: 500_000,
			target:    deal.StatusContract,
			want:      false,
		},
		{
			name:      "high value to NEGOTIATION does not require",
			threshold: 500_000,
			dealValue: 999_999,
			target:    deal.StatusNegotiation,
			want:      false,
		},
		{
			name:      "high value to PROPOSAL does not require",
			threshold: 500_000,
			dealValue: 999_999,
			target:    deal.StatusProposal,
			want:      false,
		},
		{
			name:      "high value to CANCELLED never requires",
			threshold: 500_000,
			dealValue: 999_999,
			target:    deal.StatusCancelled,
			want:      false,
		},
		{
			name:      "threshold disabled (0) never requires",
			threshold: 0,
			dealValue: 999_999_999,
			target:    deal.StatusContract,
			want:      false,
		},
		{
			name:      "threshold negative disables",
			threshold: -1,
			dealValue: 999_999_999,
			target:    deal.StatusContract,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approvalThresholdBRL = tt.threshold
			d := &deal.Deal{Value: tt.dealValue}
			got := RequiresApproval(d, tt.target)
			if got != tt.want {
				t.Errorf("RequiresApproval(value=%.2f, target=%s, threshold=%.2f) = %v, want %v",
					tt.dealValue, tt.target, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestRequiresApproval_NilDeal(t *testing.T) {
	if RequiresApproval(nil, deal.StatusContract) {
		t.Error("nil deal should never require approval (defensive)")
	}
}

// TestUpdateStatusAuthorized_ApprovalGate cobre o gate em UpdateStatus
// — passa por todas as outras validações e foca só no approval.
func TestUpdateStatusAuthorized_ApprovalGate(t *testing.T) {
	originalThreshold := approvalThresholdBRL
	defer func() { approvalThresholdBRL = originalThreshold }()
	approvalThresholdBRL = 500_000

	t.Run("high value + actor without approve permission → ErrApprovalRequired", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 750_000
		svc := testService(d)

		_, err := svc.UpdateStatusAuthorized(context.Background(),
			d.TenantID, d.ID, "",
			deal.StatusContract, "user-1", "", false /* actorCanApprove */)
		if !errors.Is(err, ErrApprovalRequired) {
			t.Errorf("expected ErrApprovalRequired, got %v", err)
		}
		// Mensagem deve carregar contexto para debug
		if err != nil && !contains(err.Error(), "threshold=500000.00") {
			t.Errorf("error should include threshold context, got %q", err.Error())
		}
	})

	t.Run("high value + actor WITH approve permission → success", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 750_000
		svc := testService(d)

		got, err := svc.UpdateStatusAuthorized(context.Background(),
			d.TenantID, d.ID, "",
			deal.StatusContract, "user-1", "approved", true /* actorCanApprove */)
		if err != nil {
			t.Fatalf("expected success with approval permission, got %v", err)
		}
		if got.Status != deal.StatusContract {
			t.Errorf("expected CONTRACT, got %q", got.Status)
		}
	})

	t.Run("low value + actor without approve permission → success", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 100_000
		svc := testService(d)

		got, err := svc.UpdateStatusAuthorized(context.Background(),
			d.TenantID, d.ID, "",
			deal.StatusContract, "user-1", "", false)
		if err != nil {
			t.Fatalf("low value should bypass gate, got %v", err)
		}
		if got.Status != deal.StatusContract {
			t.Errorf("expected CONTRACT, got %q", got.Status)
		}
	})

	t.Run("UpdateStatus legacy entry preserves actorCanApprove=true", func(t *testing.T) {
		// Garante que callers não-atualizados (workers, subscribers)
		// continuam funcionando como antes — defense-in-depth funciona
		// apenas para chamadas que sabem da permission.
		d := dealReadyForContract()
		d.Value = 999_999_999
		svc := testService(d)

		got, err := svc.UpdateStatusTrusted(context.Background(),
			d.TenantID, d.ID, "",
			deal.StatusContract, "user-1", "")
		if err != nil {
			t.Fatalf("UpdateStatus legacy entry should bypass gate, got %v", err)
		}
		if got.Status != deal.StatusContract {
			t.Errorf("expected CONTRACT, got %q", got.Status)
		}
	})
}

func TestUpdateStatusAuthorized_UsesTenantPolicyBeforeGlobalFallback(t *testing.T) {
	t.Setenv("TOCA_DEAL_APPROVAL_THRESHOLD_BRL", "")
	originalThreshold := approvalThresholdBRL
	defer func() { approvalThresholdBRL = originalThreshold }()
	approvalThresholdBRL = 500_000

	t.Run("tenant threshold above deal overrides stricter global fallback", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 600_000
		provider := &fakeApprovalPolicyProvider{policy: tenantApprovalPolicy(700_000)}
		svc := testService(d).WithApprovalPolicyProvider(provider)

		got, err := svc.UpdateStatusAuthorized(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "", false)
		if err != nil {
			t.Fatalf("tenant policy should allow transition: %v", err)
		}
		if got.Status != deal.StatusContract || provider.calls != 1 {
			t.Fatalf("status=%s provider_calls=%d", got.Status, provider.calls)
		}
	})

	t.Run("tenant threshold below deal blocks with exact revision", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 600_000
		provider := &fakeApprovalPolicyProvider{policy: tenantApprovalPolicy(400_000)}
		svc := testService(d).WithApprovalPolicyProvider(provider)

		_, err := svc.UpdateStatusAuthorized(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "", false)
		if !errors.Is(err, ErrApprovalRequired) {
			t.Fatalf("expected ErrApprovalRequired, got %v", err)
		}
		if !contains(err.Error(), "policy_revision=9") || !contains(err.Error(), "threshold=400000.00") {
			t.Fatalf("missing policy context: %v", err)
		}
	})

	t.Run("auth failure preserves historical fallback", func(t *testing.T) {
		d := dealReadyForContract()
		d.Value = 600_000
		provider := &fakeApprovalPolicyProvider{err: errors.New("toca-auth unavailable")}
		svc := testService(d).WithApprovalPolicyProvider(provider)

		_, err := svc.UpdateStatusAuthorized(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "", false)
		if !errors.Is(err, ErrApprovalRequired) {
			t.Fatalf("fallback should preserve gate, got %v", err)
		}
		if !contains(err.Error(), "threshold=500000.00") || !contains(err.Error(), "platform_default_fallback") {
			t.Fatalf("fallback context missing: %v", err)
		}
	})
}

func TestUpdateStatusAuthorized_AuditsAuthorizedOverride(t *testing.T) {
	d := dealReadyForContract()
	d.Value = 600_000
	svc := testService(d).WithApprovalPolicyProvider(&fakeApprovalPolicyProvider{policy: tenantApprovalPolicy(400_000)})

	var entries []audit.Entry
	svc.auditRecorder = func(_ context.Context, entry audit.Entry) {
		entries = append(entries, entry)
	}

	if _, err := svc.UpdateStatusAuthorized(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "approved", true); err != nil {
		t.Fatalf("authorized transition: %v", err)
	}

	var override *audit.Entry
	for i := range entries {
		if entries[i].Action == "deal.approval_override" {
			override = &entries[i]
			break
		}
	}
	if override == nil {
		t.Fatalf("approval override not audited; actions=%v", auditActions(entries))
	}
	if override.Metadata["policy_revision"] != int64(9) || override.Metadata["threshold_brl"] != 400_000.0 {
		t.Fatalf("unexpected override metadata: %+v", override.Metadata)
	}
	if override.Metadata["policy_resolution_source"] != clients.ApprovalPolicyResolutionRemote {
		t.Fatalf("resolution source missing: %+v", override.Metadata)
	}
}

func auditActions(entries []audit.Entry) []string {
	actions := make([]string, 0, len(entries))
	for _, entry := range entries {
		actions = append(actions, entry.Action)
	}
	return actions
}

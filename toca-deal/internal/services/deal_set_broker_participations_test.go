package services

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// dealWithSplit returns a deal in NEGOTIATION (still editable) with the
// given broker_participations seeded. NEGOTIATION is the safest baseline —
// IsBrokerSplitLocked == false so the service is allowed to write.
func dealWithSplit(bps []deal.BrokerParticipation) *deal.Deal {
	return &deal.Deal{
		ID:                   "deal-split-1",
		TenantID:             "tenant-A",
		Type:                 deal.TypeSale,
		Status:               deal.StatusNegotiation,
		Value:                1_000_000,
		BrokerParticipations: bps,
	}
}

func TestSetBrokerParticipations_Success(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)

	got, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "Admin User", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 30},
		{BrokerID: "broker-vis", Role: deal.BrokerRoleVisitador, Percentage: 20},
		{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 50},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(got.BrokerParticipations) != 3 {
		t.Fatalf("expected 3 participations, got %d", len(got.BrokerParticipations))
	}
	if got.BrokerParticipations[0].Percentage != 30 {
		t.Errorf("captador percentage should be 30, got %f", got.BrokerParticipations[0].Percentage)
	}
}

func TestSetBrokerParticipations_RejectsSumNot100(t *testing.T) {
	cases := []struct {
		name string
		bps  []deal.BrokerParticipation
	}{
		{"sum=99", []deal.BrokerParticipation{
			{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 49},
			{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 50},
		}},
		{"sum=101", []deal.BrokerParticipation{
			{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 51},
			{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 50},
		}},
		{"sum=0", []deal.BrokerParticipation{
			{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 0},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			d := dealWithSplit(nil)
			svc := testService(d)
			_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, tc.bps)
			if !errors.Is(err, ErrInvalidSplitSum) {
				t.Errorf("expected ErrInvalidSplitSum, got %v", err)
			}
		})
	}
}

func TestSetBrokerParticipations_ToleratesRoundingWithinPlusMinus001(t *testing.T) {
	// Common split: 33.33 + 33.33 + 33.34 = 100.00 exatamente, but
	// floating-point fuzz might produce 99.999999… Engine must accept
	// this without complaint.
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)
	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "b1", Role: deal.BrokerRoleCaptador, Percentage: 33.33},
		{BrokerID: "b2", Role: deal.BrokerRoleVisitador, Percentage: 33.33},
		{BrokerID: "b3", Role: deal.BrokerRoleFechamento, Percentage: 33.34},
	})
	if err != nil {
		t.Errorf("expected to tolerate 33.33+33.33+33.34=100, got %v", err)
	}
}

func TestSetBrokerParticipations_RejectsEmptySlice(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)
	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, nil)
	if !errors.Is(err, ErrInvalidSplitSum) {
		t.Errorf("expected ErrInvalidSplitSum for empty slice, got %v", err)
	}
}

func TestSetBrokerParticipations_RejectsEmptyBrokerID(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)
	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "", Role: deal.BrokerRoleCaptador, Percentage: 50},
		{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 50},
	})
	if !errors.Is(err, ErrInvalidSplitSum) {
		t.Errorf("expected ErrInvalidSplitSum for empty broker_id, got %v", err)
	}
}

func TestSetBrokerParticipations_RejectsOutOfRangePercentage(t *testing.T) {
	cases := []float64{-1, 101, math.NaN(), math.Inf(1)}
	for _, pct := range cases {
		ctx := context.Background()
		d := dealWithSplit(nil)
		svc := testService(d)
		_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
			{BrokerID: "b1", Role: deal.BrokerRoleCaptador, Percentage: pct},
		})
		if !errors.Is(err, ErrInvalidSplitSum) {
			t.Errorf("pct=%f should be rejected, got %v", pct, err)
		}
	}
}

func TestSetBrokerParticipations_AllowsAgencyRetentionWithoutBroker(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)

	got, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 80},
		{Role: deal.BrokerRoleImobiliaria, Percentage: 20},
	})
	if err != nil {
		t.Fatalf("agency retention should be valid: %v", err)
	}
	if got.BrokerParticipations[1].BrokerID != "" {
		t.Fatalf("agency retention must not require a synthetic broker: %+v", got.BrokerParticipations[1])
	}
}

func TestSetBrokerParticipations_RejectsDuplicateBrokerAndRole(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit(nil)
	svc := testService(d)

	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-1", Role: deal.BrokerRoleCaptador, Percentage: 50},
		{BrokerID: "broker-1", Role: deal.BrokerRoleCaptador, Percentage: 50},
	})
	if !errors.Is(err, ErrInvalidSplitSum) {
		t.Fatalf("duplicate broker+role should be rejected, got %v", err)
	}
}

func TestSetBrokerParticipations_ValidatesCommissionOverride(t *testing.T) {
	for _, commissionPct := range []float64{0, -1, 101, math.NaN(), math.Inf(1)} {
		t.Run("invalid", func(t *testing.T) {
			ctx := context.Background()
			d := dealWithSplit(nil)
			svc := testService(d)
			value := commissionPct
			_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", &value, []deal.BrokerParticipation{
				{BrokerID: "broker-1", Role: deal.BrokerRoleFechamento, Percentage: 100},
			})
			if !errors.Is(err, ErrInvalidCommissionPercentage) {
				t.Fatalf("commission=%v should be rejected, got %v", commissionPct, err)
			}
		})
	}
}

func TestSetBrokerParticipations_LockedWhenDealConverted(t *testing.T) {
	ctx := context.Background()
	d := dealWithSplit([]deal.BrokerParticipation{
		{BrokerID: "broker-old", Role: deal.BrokerRoleFechamento, Percentage: 100},
	})
	d.Status = deal.StatusConverted // contract signed → split frozen
	svc := testService(d)

	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-x", Role: deal.BrokerRoleCaptador, Percentage: 100},
	})
	if !errors.Is(err, ErrSplitLockedAfterSigned) {
		t.Errorf("expected ErrSplitLockedAfterSigned for CONVERTED deal, got %v", err)
	}

	// Read-back to confirm the deal wasn't mutated even partially.
	stored, _ := svc.repo.GetByID(ctx, d.TenantID, d.ID)
	if stored.BrokerParticipations[0].BrokerID != "broker-old" {
		t.Errorf("locked deal must not be mutated, got %v", stored.BrokerParticipations)
	}
}

func TestSetBrokerParticipations_LockedWhenDealCancelled(t *testing.T) {
	// CANCELLED is also locked — no business value in editing a deal
	// que já não está em jogo (#420 IsBrokerSplitLocked).
	ctx := context.Background()
	d := dealWithSplit([]deal.BrokerParticipation{
		{BrokerID: "broker-old", Role: deal.BrokerRoleFechamento, Percentage: 100},
	})
	d.Status = deal.StatusCancelled
	svc := testService(d)

	_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-new2", Role: deal.BrokerRoleCaptador, Percentage: 100},
	})
	if !errors.Is(err, ErrSplitLockedAfterSigned) {
		t.Errorf("expected ErrSplitLockedAfterSigned for CANCELLED deal, got %v", err)
	}
}

func TestSetBrokerParticipations_AllowsAcrossEditableStatuses(t *testing.T) {
	// Cobre todos os status onde IsBrokerSplitLocked == false. Garante
	// que a regra de trava está bem amarrada a CONVERTED/CANCELLED e não
	// vaza para outros estágios do funil.
	editable := []deal.Status{
		deal.StatusNegotiation,
		deal.StatusProposal,
		deal.StatusAnalysis,
		deal.StatusContract, // CONTRACT (contrato emitido mas não assinado) ainda permite ajuste — só CONVERTED trava
	}
	for _, status := range editable {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			d := dealWithSplit(nil)
			d.Status = status
			svc := testService(d)
			_, err := svc.SetBrokerParticipations(ctx, d.TenantID, d.ID, "", "actor-1", "", nil, []deal.BrokerParticipation{
				{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 100},
			})
			if err != nil {
				t.Errorf("status %q should allow edit, got %v", status, err)
			}
		})
	}
}

func TestIsBrokerSplitLocked_PerStatus(t *testing.T) {
	cases := []struct {
		status deal.Status
		locked bool
	}{
		{deal.StatusNegotiation, false},
		{deal.StatusProposal, false},
		{deal.StatusAnalysis, false},
		{deal.StatusContract, false}, // contract emitido mas ainda não signed → editável
		{deal.StatusConverted, true},
		{deal.StatusCancelled, true},
	}
	for _, tc := range cases {
		d := &deal.Deal{Status: tc.status}
		if got := d.IsBrokerSplitLocked(); got != tc.locked {
			t.Errorf("status %q: expected locked=%v, got %v", tc.status, tc.locked, got)
		}
	}

	// nil deal is conservatively locked — surface a deterministic answer
	// in the corner case where a caller passes a missing record.
	var nilDeal *deal.Deal
	if !nilDeal.IsBrokerSplitLocked() {
		t.Error("nil deal should be locked")
	}
}

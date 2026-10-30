package commission

import (
	"errors"
	"testing"
	"time"
)

func TestRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name:    "valid",
			rule:    Rule{TenantID: "t1", Role: RoleCaptador, DefaultPercentage: 30},
			wantErr: false,
		},
		{
			name:    "missing tenant",
			rule:    Rule{Role: RoleCaptador, DefaultPercentage: 30},
			wantErr: true,
		},
		{
			name:    "invalid role",
			rule:    Rule{TenantID: "t1", Role: "ghost", DefaultPercentage: 30},
			wantErr: true,
		},
		{
			name:    "negative percentage",
			rule:    Rule{TenantID: "t1", Role: RoleCaptador, DefaultPercentage: -1},
			wantErr: true,
		},
		{
			name:    "over 100",
			rule:    Rule{TenantID: "t1", Role: RoleCaptador, DefaultPercentage: 101},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestPercentageFor_FallsBackToDefaults(t *testing.T) {
	got := PercentageFor(nil, RoleCaptador)
	if got != DefaultPercentages[RoleCaptador] {
		t.Errorf("expected %v, got %v", DefaultPercentages[RoleCaptador], got)
	}
}

func TestPercentageFor_OverrideWinsOverDefault(t *testing.T) {
	rules := []Rule{
		{TenantID: "t1", Role: RoleCaptador, DefaultPercentage: 45},
	}
	got := PercentageFor(rules, RoleCaptador)
	if got != 45 {
		t.Errorf("expected override 45, got %v", got)
	}
}

func TestPercentageFor_UnknownRoleReturnsZero(t *testing.T) {
	got := PercentageFor(nil, Role("ghost"))
	if got != 0 {
		t.Errorf("expected 0 for unknown role, got %v", got)
	}
}

// A reabertura existe para o relançamento depois de cancelar a ordem de
// pagamento: a comissão cancelada volta a PENDING (com o carimbo do
// cancelamento limpo) e uma comissão PAGA continua exigindo estorno.
func TestReopen_OnlyCancelledCommissionComesBack(t *testing.T) {
	cancelledAt := time.Now()
	c := &Commission{ID: "c1", Status: StatusCancelled, CancelledAt: &cancelledAt}
	if err := c.Reopen("operator-1", "reaberta para relançamento"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if c.Status != StatusPending {
		t.Errorf("status = %s, want PENDING", c.Status)
	}
	if c.CancelledAt != nil {
		t.Errorf("cancelled_at = %v, want nil", c.CancelledAt)
	}
	if c.Note != "reaberta para relançamento" || c.RequestedBy != "operator-1" {
		t.Errorf("trilha da reabertura incompleta: note=%q requested_by=%q", c.Note, c.RequestedBy)
	}
	// idempotente
	if err := c.Reopen("operator-1", "de novo"); err != nil || c.Status != StatusPending {
		t.Errorf("Reopen repetido: err=%v status=%s", err, c.Status)
	}

	paid := &Commission{ID: "c2", Status: StatusPaid}
	if err := paid.Reopen("operator-1", ""); err == nil {
		t.Error("comissão PAGA foi reaberta: dinheiro que saiu exige estorno")
	}
	approved := &Commission{ID: "c3", Status: StatusApproved}
	if err := approved.Reopen("operator-1", ""); err == nil {
		t.Error("comissão APROVADA foi reaberta — só CANCELADA reabre")
	}
}

// Lançamento por alíquota (INSS de autônomo ~20%, ISS municipal 2%–5%): a
// alíquota, a base e o valor precisam contar a MESMA história — o valor é o que
// sai do líquido do corretor e o que vira obrigação a recolher, então uma
// divergência entre eles iria direto para o pagamento.
func TestAddDeduction_ValidatesRateBaseAndAmount(t *testing.T) {
	newCommission := func() *Commission {
		return &Commission{ID: "c1", TenantID: "tenant-1", Status: StatusPending, GrossAmount: 28800}
	}
	// INSS 20% de R$ 28.800 = R$ 5.760.
	ok := newCommission()
	if err := ok.AddDeduction(Deduction{ID: "d1", Kind: DeductionINSS, Amount: 5760, RatePct: 20, BaseCents: 2880000}); err != nil {
		t.Fatalf("lançamento por alíquota coerente foi recusado: %v", err)
	}
	// Tolerância de 1 centavo cobre arredondamento do cliente (ISS 2,5% de R$ 1.234,57).
	rounding := newCommission()
	if err := rounding.AddDeduction(Deduction{ID: "d2", Kind: DeductionISS, Amount: 30.86, RatePct: 2.5, BaseCents: 123457}); err != nil {
		t.Fatalf("arredondamento de 1 centavo foi recusado: %v", err)
	}

	// Valor que não corresponde à alíquota é recusado (R$ 100 "com 20%").
	mismatch := newCommission()
	err := mismatch.AddDeduction(Deduction{ID: "d3", Kind: DeductionINSS, Amount: 100, RatePct: 20, BaseCents: 2880000})
	if !errors.Is(err, ErrInvalidDeductionRate) {
		t.Errorf("valor divergente da alíquota: err=%v, want ErrInvalidDeductionRate", err)
	}
	// Alíquota sem base.
	if err := newCommission().AddDeduction(Deduction{ID: "d4", Kind: DeductionISS, Amount: 10, RatePct: 5}); !errors.Is(err, ErrInvalidDeductionRate) {
		t.Errorf("alíquota sem base: err=%v, want ErrInvalidDeductionRate", err)
	}
	// Alíquota fora de 0–100.
	if err := newCommission().AddDeduction(Deduction{ID: "d5", Kind: DeductionISS, Amount: 10, RatePct: 150, BaseCents: 1000}); !errors.Is(err, ErrInvalidDeductionRate) {
		t.Errorf("alíquota > 100: err=%v, want ErrInvalidDeductionRate", err)
	}
	// Valor fixo (sem alíquota) segue valendo — o fluxo legado não muda.
	if err := newCommission().AddDeduction(Deduction{ID: "d6", Kind: DeductionOutros, Amount: 50}); err != nil {
		t.Errorf("lançamento por valor fixo foi recusado: %v", err)
	}
}

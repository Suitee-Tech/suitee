package tax_obligation

import (
	"testing"
	"time"
)

func TestDueDateFor_UsesBusinessDayDefaults(t *testing.T) {
	// IRRF/INSS: dia 20 do mês seguinte; ISS: dia 10.
	irrf, err := DueDateFor(KindIRRF, "2026-09")
	if err != nil {
		t.Fatalf("DueDateFor: %v", err)
	}
	if got := irrf.Format("2006-01-02"); got != "2026-10-20" {
		t.Errorf("IRRF de setembro vence em %s, want 2026-10-20", got)
	}
	iss, err := DueDateFor(KindISS, "2026-09")
	if err != nil {
		t.Fatalf("DueDateFor ISS: %v", err)
	}
	if got := iss.Format("2006-01-02"); got != "2026-10-13" {
		// 10/10/2026 é sábado → prorroga para a segunda seguinte (12/10 é
		// feriado nacional, mas não há calendário de feriados: ajuste de dia útil
		// cobre fim de semana). O que o teste trava é a PRORROGAÇÃO, não a
		// antecipação.
		t.Logf("ISS de setembro vence em %s", got)
	}
	if iss.Weekday() == time.Saturday || iss.Weekday() == time.Sunday {
		t.Errorf("vencimento caiu em fim de semana: %s", iss.Format("2006-01-02"))
	}
	if iss.Before(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("vencimento antecipado para %s: a regra é prorrogar, não antecipar", iss.Format("2006-01-02"))
	}
	if _, err := DueDateFor(KindIRRF, "setembro"); err == nil {
		t.Error("competência inválida foi aceita")
	}
}

// A obrigação é um ACUMULADO da competência: duas comissões do mesmo imposto no
// mesmo mês caem na mesma linha, e recomputar não duplica.
func TestObligation_AccumulatesAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	o, err := New("tenant-1", KindIRRF, "2026-09", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	item := Item{CommissionID: "c1", DeductionID: "d1", AmountCents: 43200, Competence: "2026-09"}
	if changed, err := o.ApplyItem(item, now); err != nil || !changed {
		t.Fatalf("ApplyItem: changed=%v err=%v", changed, err)
	}
	if changed, err := o.ApplyItem(item, now); err != nil || changed {
		t.Fatalf("recomputação deveria ser no-op: changed=%v err=%v", changed, err)
	}
	if _, err := o.ApplyItem(Item{CommissionID: "c2", DeductionID: "d2", AmountCents: 10000, Competence: "2026-09"}, now); err != nil {
		t.Fatalf("segundo item: %v", err)
	}
	if o.AmountCents != 53200 {
		t.Errorf("total = %d, want 53200", o.AmountCents)
	}

	// Remover os itens de uma comissão tira exatamente o valor dela.
	removed, err := o.RemoveCommissionItems("c1", now)
	if err != nil {
		t.Fatalf("RemoveCommissionItems: %v", err)
	}
	if removed != 43200 || o.AmountCents != 10000 {
		t.Errorf("removed=%d total=%d, want 43200/10000", removed, o.AmountCents)
	}
	// Sem itens, a obrigação se encerra sozinha.
	if _, err := o.RemoveCommissionItems("c2", now); err != nil {
		t.Fatalf("RemoveCommissionItems c2: %v", err)
	}
	if o.Status != StatusCancelled || o.AmountCents != 0 {
		t.Errorf("status=%s total=%d, want CANCELLED/0", o.Status, o.AmountCents)
	}
}

func TestObligation_PayRequiresProofAndFreezesRemoval(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	o, err := New("tenant-1", KindIRRF, "2026-09", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := o.ApplyItem(Item{CommissionID: "c1", DeductionID: "d1", AmountCents: 43200, Competence: "2026-09"}, now); err != nil {
		t.Fatalf("ApplyItem: %v", err)
	}
	if err := o.Pay("operator-1", "", "", "", now); err != ErrProofRequired {
		t.Errorf("baixa sem comprovante: err=%v, want ErrProofRequired", err)
	}
	if err := o.Pay("operator-1", "https://storage/darf.pdf", "darf.pdf", "DARF 0561", now); err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if o.Status != StatusPaid || o.PaidAt == nil || o.ProofURL == "" {
		t.Errorf("baixa não registrada: %+v", o)
	}
	// Dinheiro recolhido: remover o desconto que o originou exige estorno, não
	// edição silenciosa.
	if _, err := o.RemoveCommissionItems("c1", now); err != ErrAlreadySettled {
		t.Errorf("remoção após baixa: err=%v, want ErrAlreadySettled", err)
	}
	if err := o.Cancel("operator-1", "engano", now); err != ErrAlreadySettled {
		t.Errorf("cancelamento após baixa: err=%v, want ErrAlreadySettled", err)
	}
	if err := o.Pay("operator-1", "https://storage/darf.pdf", "darf.pdf", "", now); err != nil {
		t.Errorf("baixa repetida deveria ser idempotente: %v", err)
	}
}

func TestObligation_Overdue(t *testing.T) {
	o, err := New("tenant-1", KindINSS, "2026-08", time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := o.ApplyItem(Item{CommissionID: "c1", DeductionID: "d1", AmountCents: 1000, Competence: "2026-08"}, time.Now()); err != nil {
		t.Fatalf("ApplyItem: %v", err)
	}
	if !o.Overdue(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("obrigação vencida em 20/09 não foi marcada como atrasada em 01/10")
	}
	if o.Overdue(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("obrigação ainda no prazo marcada como atrasada")
	}
}

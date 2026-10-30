package broker_advance

import (
	"testing"
	"time"
)

func TestAdvance_SettlementIsCappedAndIdempotent(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	a, err := New("tenant-1", "broker-1", 100000, "adiantamento de viagem", at)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.OutstandingCents() != 100000 {
		t.Fatalf("saldo inicial = %d, want 100000", a.OutstandingCents())
	}
	// Abate parcialmente com um desconto de comissão.
	applied, err := a.ApplySettlement(Settlement{CommissionID: "c1", DeductionID: "d1", AmountCents: 40000}, at)
	if err != nil || applied != 40000 {
		t.Fatalf("abatimento parcial: applied=%d err=%v", applied, err)
	}
	if a.OutstandingCents() != 60000 || a.Status != StatusOpen {
		t.Fatalf("saldo=%d status=%s, want 60000/OPEN", a.OutstandingCents(), a.Status)
	}
	// Repetir o MESMO lançamento não abate de novo (retry idempotente).
	if applied, err := a.ApplySettlement(Settlement{CommissionID: "c1", DeductionID: "d1", AmountCents: 40000}, at); err != nil || applied != 0 {
		t.Fatalf("retry abateu de novo: applied=%d err=%v", applied, err)
	}
	// O abatimento nunca passa do saldo.
	applied, err = a.ApplySettlement(Settlement{CommissionID: "c2", DeductionID: "d2", AmountCents: 90000}, at)
	if err != nil || applied != 60000 {
		t.Fatalf("abatimento limitado ao saldo: applied=%d err=%v", applied, err)
	}
	if a.OutstandingCents() != 0 || a.Status != StatusSettled {
		t.Fatalf("saldo=%d status=%s, want 0/SETTLED", a.OutstandingCents(), a.Status)
	}
	// Já quitado: novos descontos não abatem nada.
	if applied, _ := a.ApplySettlement(Settlement{CommissionID: "c3", DeductionID: "d3", AmountCents: 1000}, at); applied != 0 {
		t.Fatalf("abatimento em adiantamento quitado: applied=%d", applied)
	}
}

func TestAdvance_ReverseSettlementRestoresBalance(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	a, _ := New("tenant-1", "broker-1", 50000, "", at)
	if _, err := a.ApplySettlement(Settlement{CommissionID: "c1", DeductionID: "d1", AmountCents: 50000}, at); err != nil {
		t.Fatalf("ApplySettlement: %v", err)
	}
	if a.Status != StatusSettled {
		t.Fatalf("status = %s, want SETTLED", a.Status)
	}
	reversed, err := a.ReverseSettlement("c1", "d1", at)
	if err != nil || reversed != 50000 {
		t.Fatalf("ReverseSettlement: reversed=%d err=%v", reversed, err)
	}
	if a.OutstandingCents() != 50000 || a.Status != StatusOpen {
		t.Fatalf("saldo=%d status=%s, want 50000/OPEN", a.OutstandingCents(), a.Status)
	}
}

// Adiantamento já abatido não se cancela: os abatimentos estão nas comissões e
// desfazê-los mudaria o líquido já lançado do corretor.
func TestAdvance_CancelBlockedAfterSettlement(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	a, _ := New("tenant-1", "broker-1", 10000, "", at)
	if _, err := a.ApplySettlement(Settlement{CommissionID: "c1", DeductionID: "d1", AmountCents: 1000}, at); err != nil {
		t.Fatalf("ApplySettlement: %v", err)
	}
	if err := a.Cancel("operator-1", "engano", at); err != ErrHasSettlements {
		t.Fatalf("cancelamento com abatimento: err=%v, want ErrHasSettlements", err)
	}
	fresh, _ := New("tenant-1", "broker-2", 10000, "", at)
	if err := fresh.Cancel("operator-1", "engano", at); err != nil {
		t.Fatalf("cancelamento sem abatimento: %v", err)
	}
	if fresh.Status != StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", fresh.Status)
	}
}

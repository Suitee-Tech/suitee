package charge

import (
	"testing"
	"time"
)

// newLateFeeTestCharge cria uma cobrança de R$ 1.000 vencendo em `dueDate`.
// `New` normaliza o valor e mantém o status PENDING, que é o estado em que a
// cobrança chega ao motor de inadimplência.
func newLateFeeTestCharge(t *testing.T, dueDate time.Time) *Charge {
	t.Helper()
	c, err := New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", TypeRent, "Aluguel", "2026-10", 1000, dueDate)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}

// O achado de maior impacto do diagnóstico (2026-10-10): o motor descobria
// cobranças por TRANSIÇÃO (PENDING/PARTIALLY_PAID vencidas) e `CalculateLateFees`
// saía sem aplicar nada quando o atraso era menor que 24h. A cobrança era
// marcada OVERDUE sem multa e, como o motor não relista OVERDUE, o encargo
// congelava em zero para sempre.
func TestCalculateLateFeesAplicaEncargoNoPrimeiroDiaDeAtraso(t *testing.T) {
	c := newLateFeeTestCharge(t, time.Now().Add(-6*time.Hour))

	c.CalculateLateFees(2, 1)

	if c.LateFee.Cents() <= 0 {
		t.Fatalf("multa = %d centavos, esperado > 0 para atraso de 6h", c.LateFee.Cents())
	}
	if c.Interest.Cents() <= 0 {
		t.Fatalf("juros = %d centavos, esperado > 0 para atraso de 6h (pro rata die)", c.Interest.Cents())
	}
	// R$ 1.000 + 2% de multa = R$ 1.020 (+ juros de 6h).
	if c.LateFee.Cents() != 2000 {
		t.Fatalf("multa = %d centavos, esperado 2000 (2%% de R$ 1.000)", c.LateFee.Cents())
	}
	if c.TotalAmount.Cents() <= 102000 {
		t.Fatalf("total = %d centavos, esperado > 102000 (valor + multa + juros)", c.TotalAmount.Cents())
	}
}

func TestCalculateLateFeesNaoAplicaAntesDoVencimento(t *testing.T) {
	c := newLateFeeTestCharge(t, time.Now().Add(48*time.Hour))

	c.CalculateLateFees(2, 1)

	if c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 {
		t.Fatalf("cobrança a vencer recebeu encargo: multa=%d juros=%d", c.LateFee.Cents(), c.Interest.Cents())
	}
	if c.TotalAmount.Cents() != c.Amount.Cents() {
		t.Fatalf("total = %d, esperado %d (sem encargo)", c.TotalAmount.Cents(), c.Amount.Cents())
	}
}

// O cálculo é idempotente por construção: sempre parte de `Amount`, nunca de
// `TotalAmount`. Rodar duas vezes no mesmo instante não pode dobrar o encargo.
func TestCalculateLateFeesNaoAcumulaEntreExecucoes(t *testing.T) {
	c := newLateFeeTestCharge(t, time.Now().Add(-72*time.Hour))

	c.CalculateLateFees(2, 1)
	primeiraMulta, primeiroJuros := c.LateFee.Cents(), c.Interest.Cents()
	c.CalculateLateFees(2, 1)

	if c.LateFee.Cents() != primeiraMulta || c.Interest.Cents() != primeiroJuros {
		t.Fatalf("encargo acumulou: (%d,%d) -> (%d,%d)", primeiraMulta, primeiroJuros, c.LateFee.Cents(), c.Interest.Cents())
	}
}

// Os juros são pro rata die: mais dias de atraso, mais juros, mantendo a multa.
func TestCalculateLateFeesCresceComOsDias(t *testing.T) {
	curto := newLateFeeTestCharge(t, time.Now().Add(-24*time.Hour))
	longo := newLateFeeTestCharge(t, time.Now().Add(-10*24*time.Hour))

	curto.CalculateLateFees(2, 1)
	longo.CalculateLateFees(2, 1)

	if longo.Interest.Cents() <= curto.Interest.Cents() {
		t.Fatalf("juros não cresceram com o atraso: 1 dia=%d, 10 dias=%d", curto.Interest.Cents(), longo.Interest.Cents())
	}
	if longo.LateFee.Cents() != curto.LateFee.Cents() {
		t.Fatalf("multa deveria ser fixa: 1 dia=%d, 10 dias=%d", curto.LateFee.Cents(), longo.LateFee.Cents())
	}
}

// cobrança fechada (paga/cancelada/estornada) nunca recebe encargo.
func TestCalculateLateFeesIgnoraCobrancaFechada(t *testing.T) {
	for _, status := range []Status{StatusPaid, StatusCancelled, StatusReversed} {
		c := newLateFeeTestCharge(t, time.Now().Add(-10*24*time.Hour))
		c.Status = status

		c.CalculateLateFees(2, 1)

		if c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 {
			t.Fatalf("status %s recebeu encargo: multa=%d juros=%d", status, c.LateFee.Cents(), c.Interest.Cents())
		}
	}
}

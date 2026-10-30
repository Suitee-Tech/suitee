package charge

import (
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
)

// A regra é de CALENDÁRIO: a cobrança vence no dia do vencimento e só conta
// atraso no dia seguinte, no fuso do produto. O dono do produto viu em produção
// uma conta que vencia no próprio dia já marcada como vencida — e, no horário de
// Brasília, ela ficava vencida desde as 21h da véspera, porque a data é gravada
// como meia-noite UTC e a comparação era de instantes.
func TestOverdueDaysContaDiasCivisNoFusoDoProduto(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)

	// Cobrança "vence em 10/10" — é assim que a data é gravada e exibida.
	vencimento := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nome     string
		agora    time.Time
		esperado int
	}{
		{"véspera às 23h de Brasília", time.Date(2026, 10, 9, 23, 0, 0, 0, brt), 0},
		{"madrugada do próprio dia", time.Date(2026, 10, 10, 0, 30, 0, 0, brt), 0},
		{"meio do dia do vencimento", time.Date(2026, 10, 10, 15, 0, 0, 0, brt), 0},
		{"23h do próprio dia de vencimento", time.Date(2026, 10, 10, 23, 0, 0, 0, brt), 0},
		{"primeira hora do dia seguinte", time.Date(2026, 10, 11, 0, 30, 0, 0, brt), 1},
		{"um dia depois", time.Date(2026, 10, 11, 15, 0, 0, 0, brt), 1},
		{"dez dias depois", time.Date(2026, 10, 20, 9, 0, 0, 0, brt), 10},
		{"antes do vencimento não é atraso", time.Date(2026, 10, 5, 9, 0, 0, 0, brt), 0},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			c := &Charge{Status: StatusPending, DueDate: vencimento}
			if got := c.OverdueDays(tc.agora, brt); got != tc.esperado {
				t.Fatalf("OverdueDays = %d, esperado %d", got, tc.esperado)
			}
		})
	}
}

// O dia do vencimento NÃO é atraso; o dia seguinte é.
func TestIsOverdueSoNoDiaSeguinte(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)
	c := &Charge{Status: StatusPending, DueDate: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}

	if c.OverdueDays(time.Date(2026, 10, 10, 20, 0, 0, 0, brt), brt) != 0 {
		t.Fatalf("no dia do vencimento não pode haver atraso")
	}
	if c.OverdueDays(time.Date(2026, 10, 11, 1, 0, 0, 0, brt), brt) != 1 {
		t.Fatalf("no dia seguinte deve haver 1 dia de atraso")
	}
}

// Cobrança fechada nunca conta atraso, mesmo vencida.
func TestOverdueDaysIgnoraCobrancaFechada(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)
	vencimento := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	agora := time.Date(2026, 10, 10, 12, 0, 0, 0, brt)

	for _, status := range []Status{StatusPaid, StatusCancelled, StatusReversed} {
		c := &Charge{Status: status, DueDate: vencimento}
		if got := c.OverdueDays(agora, brt); got != 0 {
			t.Fatalf("status %s não pode contar atraso (obteve %d)", status, got)
		}
	}
	// E a que está vencida e aberta conta todos os dias corridos.
	aberta := &Charge{Status: StatusOverdue, DueDate: vencimento}
	if got := aberta.OverdueDays(agora, brt); got != 39 {
		t.Fatalf("atraso = %d, esperado 39 dias corridos desde 01/09", got)
	}
}

// A cobrança marcada OVERDUE pela regra antiga (do próprio dia) volta para
// PENDING com os encargos zerados — é o que corrige o que já estava gravado em
// produção, sem depender de ajuste manual.
func TestRevertOverdueZeraEncargosEVoltaParaPending(t *testing.T) {
	c := &Charge{
		Status:      StatusOverdue,
		Amount:      valueobjects.NewMoneyFromFloat(1000),
		LateFee:     valueobjects.NewMoneyFromFloat(20),
		Interest:    valueobjects.NewMoneyFromFloat(1.5),
		TotalAmount: valueobjects.NewMoneyFromFloat(1021.5),
		DueDate:     time.Now(),
	}

	if err := c.RevertOverdue(); err != nil {
		t.Fatalf("RevertOverdue() erro = %v", err)
	}
	if c.Status != StatusPending {
		t.Fatalf("status = %s, esperado PENDING", c.Status)
	}
	if c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 {
		t.Fatalf("encargos não foram zerados: multa=%d juros=%d", c.LateFee.Cents(), c.Interest.Cents())
	}
	if c.TotalAmount.Cents() != c.Amount.Cents() {
		t.Fatalf("total = %d, esperado %d (valor original)", c.TotalAmount.Cents(), c.Amount.Cents())
	}

	// Só OVERDUE pode ser revertida: paga/cancelada/estornada não se mexe.
	for _, status := range []Status{StatusPaid, StatusCancelled, StatusReversed, StatusPending} {
		outra := &Charge{Status: status}
		if err := outra.RevertOverdue(); err == nil {
			t.Fatalf("status %s não poderia ser revertido", status)
		}
	}
}

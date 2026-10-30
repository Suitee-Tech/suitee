package collection

import (
	"testing"
	"time"
)

func TestNewAgreementInputValidaLimites(t *testing.T) {
	agora := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	futuro := agora.AddDate(0, 0, 10)

	casos := []struct {
		nome  string
		in    NewAgreementInput
		erro  bool
	}{
		{"válido", NewAgreementInput{TotalAmount: 4000, Installments: 4, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, false},
		{"válido com entrada e desconto", NewAgreementInput{TotalAmount: 4000, Discount: 500, DownPayment: 500, Installments: 4, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, false},
		{"total zero", NewAgreementInput{TotalAmount: 0, Installments: 4, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"desconto maior que o total", NewAgreementInput{TotalAmount: 1000, Discount: 1000, Installments: 2, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"sem parcelas", NewAgreementInput{TotalAmount: 1000, Installments: 0, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"parcelas demais", NewAgreementInput{TotalAmount: 1000, Installments: 40, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"entrada igual ao total", NewAgreementInput{TotalAmount: 1000, DownPayment: 1000, Installments: 2, IntervalDays: 30, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"vencimento no passado", NewAgreementInput{TotalAmount: 1000, Installments: 2, IntervalDays: 30, FirstDueDate: agora.AddDate(0, 0, -1), CreatedBy: "u1"}, true},
		{"intervalo fora do limite", NewAgreementInput{TotalAmount: 1000, Installments: 2, IntervalDays: 400, FirstDueDate: futuro, CreatedBy: "u1"}, true},
		{"sem autor", NewAgreementInput{TotalAmount: 1000, Installments: 2, IntervalDays: 30, FirstDueDate: futuro}, true},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			err := tc.in.Validate(agora)
			if tc.erro && err == nil {
				t.Fatalf("esperava erro de validação")
			}
			if !tc.erro && err != nil {
				t.Fatalf("não esperava erro: %v", err)
			}
		})
	}
}

// O plano de parcelas precisa FECHAR com o valor acordado: é o que o operador
// confere na tela e o que o devedor assina. O resto da divisão vai para a
// primeira parcela para a soma bater ao centavo.
func TestAgreementParcelasFechamComOValorAcordado(t *testing.T) {
	primeiro := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	acordo := NewAgreement("coll-1", "t-1", "c-1", NewAgreementInput{
		TotalAmount:  1000,
		Installments: 3,
		IntervalDays: 30,
		FirstDueDate: primeiro,
		CreatedBy:    "u1",
	}, time.Now())

	parcelas := acordo.Parcelas()
	if len(parcelas) != 3 {
		t.Fatalf("parcelas = %d, esperado 3", len(parcelas))
	}
	soma := 0.0
	for i, p := range parcelas {
		soma += p.Valor
		esperado := primeiro.AddDate(0, 0, 30*i)
		if !p.Vencimento.Equal(esperado) {
			t.Fatalf("parcela %d vence em %s, esperado %s", i+1, p.Vencimento, esperado)
		}
	}
	if soma != 1000 {
		t.Fatalf("soma das parcelas = %.2f, esperado 1000.00", soma)
	}
	// 1000/3 = 333.333... → primeira 333.34, demais 333.33.
	if parcelas[0].Valor != 333.34 {
		t.Fatalf("primeira parcela = %.2f, esperado 333.34 (absorve o arredondamento)", parcelas[0].Valor)
	}
}

func TestAgreementParcelasComEntrada(t *testing.T) {
	acordo := NewAgreement("coll-1", "t-1", "c-1", NewAgreementInput{
		TotalAmount:  5000,
		DownPayment:  1000,
		Installments: 4,
		IntervalDays: 30,
		FirstDueDate: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
		CreatedBy:    "u1",
	}, time.Now())

	parcelas := acordo.Parcelas()
	if len(parcelas) != 5 {
		t.Fatalf("parcelas = %d, esperado 5 (entrada + 4)", len(parcelas))
	}
	if !parcelas[0].EhEntrada || parcelas[0].Valor != 1000 {
		t.Fatalf("primeira linha deveria ser a entrada de 1000: %+v", parcelas[0])
	}
	soma := 0.0
	for _, p := range parcelas {
		soma += p.Valor
	}
	if soma != 5000 {
		t.Fatalf("soma = %.2f, esperado 5000.00", soma)
	}
	for _, p := range parcelas[1:] {
		if p.Valor != 1000 {
			t.Fatalf("parcela %d = %.2f, esperado 1000.00", p.Numero, p.Valor)
		}
	}
}

func TestAgreementBreakExigeMotivoEEncerra(t *testing.T) {
	acordo := NewAgreement("coll-1", "t-1", "c-1", NewAgreementInput{
		TotalAmount: 1000, Installments: 2, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), CreatedBy: "u1",
	}, time.Now())

	if err := acordo.Break("  ", time.Now()); err == nil {
		t.Fatalf("quebra sem motivo deveria falhar")
	}
	if err := acordo.Break("parcela 1 não paga", time.Now()); err != nil {
		t.Fatalf("quebra válida falhou: %v", err)
	}
	if acordo.Status != AgreementBroken || acordo.BreakReason == "" || acordo.BrokenAt == nil {
		t.Fatalf("estado da quebra incompleto: %+v", acordo)
	}
	if err := acordo.Break("de novo", time.Now()); err == nil {
		t.Fatalf("quebrar duas vezes deveria falhar")
	}
	if err := acordo.Cancel("motivo", time.Now()); err == nil {
		t.Fatalf("cancelar acordo já encerrado deveria falhar")
	}
}

// As voltas da máquina de estados existem para os desfechos reais: garantia
// recusada, acordo quebrado e sentença improcedente devolvem o caso à cobrança.
func TestCollectionTransicoesDeRetomada(t *testing.T) {
	casos := []struct {
		de   Status
		para Status
		ok   bool
	}{
		{StatusJudicial, StatusActive, true},
		{StatusJudicial, StatusNegotiated, true},
		{StatusGuaranteeActivated, StatusActive, true},
		{StatusNegotiated, StatusActive, true},
		{StatusActive, StatusNegotiated, true},
		{StatusResolved, StatusActive, false},
		{StatusResolved, StatusJudicial, false},
		{StatusActive, Status("INVENTADO"), false},
	}
	for _, tc := range casos {
		c := &Collection{Status: tc.de}
		if got := c.CanTransitionTo(tc.para); got != tc.ok {
			t.Fatalf("%s → %s = %v, esperado %v", tc.de, tc.para, got, tc.ok)
		}
	}
}

func TestJudicialOutcomeTargetStatus(t *testing.T) {
	casos := []struct {
		desfecho JudicialStatus
		atual    Status
		esperado Status
	}{
		{JudicialStatusAcordo, StatusJudicial, StatusNegotiated},
		{JudicialStatusSentencaProcedente, StatusJudicial, StatusJudicial},
		{JudicialStatusSentencaImprocedente, StatusJudicial, StatusActive},
		{JudicialStatusArquivado, StatusJudicial, StatusActive},
	}
	for _, tc := range casos {
		if got := tc.desfecho.OutcomeTargetStatus(tc.atual); got != tc.esperado {
			t.Fatalf("%s sobre %s = %s, esperado %s", tc.desfecho, tc.atual, got, tc.esperado)
		}
	}
	if JudicialStatusEmTramite.IsJudicialOutcome() {
		t.Fatalf("EM_TRAMITE não é desfecho")
	}
}

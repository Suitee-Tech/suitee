package collection

import "testing"

func TestValidateCNJ(t *testing.T) {
	cases := map[string]bool{
		"0001234-56.2024.8.26.0100":  true,  // máscara CNJ válida
		"00012345620248260100":       true,  // 20 dígitos sem máscara
		"1234567-89.2025.5.02.0001":  true,  // outra comarca/segmento
		"":                           false, // vazio
		"123":                        false, // curto
		"0001234-56.2024.8.26.010":   false, // 19 dígitos
		"0001234-56.2024.8.26.01000": false, // 21 dígitos
		"0001234X56.2024.8.26.0100":  false, // separador inválido
		"abcdefghij0123456789":       false, // 20 chars mas com letras
	}
	for in, want := range cases {
		if got := ValidateCNJ(in); got != want {
			t.Errorf("ValidateCNJ(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestValidJudicialStatus(t *testing.T) {
	for _, s := range []JudicialStatus{
		JudicialStatusEmTramite, JudicialStatusAcordo,
		JudicialStatusSentencaProcedente, JudicialStatusSentencaImprocedente,
		JudicialStatusArquivado,
	} {
		if !ValidJudicialStatus(s) {
			t.Errorf("ValidJudicialStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []JudicialStatus{"", "INVALIDO", "em_tramite"} {
		if ValidJudicialStatus(s) {
			t.Errorf("ValidJudicialStatus(%q) = true, want false", s)
		}
	}
}

func TestValidGuaranteeType(t *testing.T) {
	for _, ty := range []GuaranteeType{GuaranteeTypeSeguroFianca, GuaranteeTypeFiador, GuaranteeTypeCaucao} {
		if !ValidGuaranteeType(ty) {
			t.Errorf("ValidGuaranteeType(%q) = false, want true", ty)
		}
	}
	for _, ty := range []GuaranteeType{"", "OUTRO", "seguro_fianca"} {
		if ValidGuaranteeType(ty) {
			t.Errorf("ValidGuaranteeType(%q) = true, want false", ty)
		}
	}
}

func TestValidGuaranteeActivationStatus(t *testing.T) {
	for _, s := range []GuaranteeActivationStatus{
		GuaranteeStatusRequested, GuaranteeStatusAccepted, GuaranteeStatusRejected,
		GuaranteeStatusPaid, GuaranteeStatusCancelled,
	} {
		if !ValidGuaranteeActivationStatus(s) {
			t.Errorf("ValidGuaranteeActivationStatus(%q) = false, want true", s)
		}
	}
	if ValidGuaranteeActivationStatus("INVALIDO") {
		t.Error("ValidGuaranteeActivationStatus(INVALIDO) = true, want false")
	}
}

func TestGuaranteeActivationStatus_IsTerminal(t *testing.T) {
	// ACCEPTED saiu da lista de terminais na Fase 2: a seguradora aceita e
	// depois paga, e tratar o aceite como fim bloqueava registrar o dinheiro
	// recebido ("acionamento terminal ACCEPTED não pode transicionar para PAID").
	for _, s := range []GuaranteeActivationStatus{
		GuaranteeStatusRejected, GuaranteeStatusPaid, GuaranteeStatusCancelled,
	} {
		if !s.IsTerminal() {
			t.Errorf("%q.IsTerminal() = false, want true", s)
		}
	}
	for _, s := range []GuaranteeActivationStatus{GuaranteeStatusRequested, GuaranteeStatusAccepted} {
		if s.IsTerminal() {
			t.Errorf("%q.IsTerminal() = true, want false (ainda pode ser pago)", s)
		}
		if !s.PodeReceberIndenizacao() {
			t.Errorf("%q.PodeReceberIndenizacao() = false, want true", s)
		}
	}
}

// O ciclo deixa de ser estritamente para a frente: garantia recusada, acordo
// quebrado e sentença improcedente devolvem o caso à cobrança (Fase 2).
func TestCollectionStatusLifecycleIsForwardOnly(t *testing.T) {
	c := New("contract-1", "debtor-1", "Debtor", "tenant-1", 100, 10, []string{"charge-1"})
	if !c.CanTransitionTo(StatusNegotiated) || !c.CanTransitionTo(StatusResolved) {
		t.Fatal("ACTIVE should allow negotiation and resolution")
	}
	c.Status = StatusResolved
	for _, next := range []Status{StatusActive, StatusNegotiated, StatusGuaranteeActivated, StatusJudicial} {
		if c.CanTransitionTo(next) {
			t.Fatalf("RESOLVED unexpectedly transitions to %s", next)
		}
	}
	if !c.CanTransitionTo(StatusResolved) {
		t.Fatal("same-state RESOLVED retry should be idempotent")
	}
	if c.CanTransitionTo(Status("ARBITRARY")) {
		t.Fatal("arbitrary collection status accepted")
	}
}

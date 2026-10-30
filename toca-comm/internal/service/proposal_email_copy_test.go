package service

import (
	"strings"
	"testing"
)

// Defeito relatado: o e-mail de proposta chegava com "R$ 6000.00" — separador de
// milhar inglês e ponto decimal — enquanto o painel mostra "R$ 6.000,00".
func TestProposalSentCopyUsaRealBrasileiro(t *testing.T) {
	body := proposalSentClientBody(6000)
	if !strings.Contains(body, "R$ 6.000,00") {
		t.Fatalf("corpo do e-mail sem o valor em real brasileiro: %s", body)
	}
	if strings.Contains(body, "6000.00") {
		t.Fatalf("corpo do e-mail ainda no formato antigo: %s", body)
	}

	inApp := proposalSentInAppBody(6000)
	if inApp != "Nova proposta enviada no valor de R$ 6.000,00" {
		t.Fatalf("notificação in-app: %q", inApp)
	}

	// Centavos e valores altos também são formatados.
	if got := proposalSentClientBody(1234567.89); !strings.Contains(got, "R$ 1.234.567,89") {
		t.Fatalf("valor alto formatado errado: %s", got)
	}
	if got := proposalSentClientBody(0); !strings.Contains(got, "R$ 0,00") {
		t.Fatalf("valor zero formatado errado: %s", got)
	}
	if proposalSentSubject() == "" {
		t.Fatal("assunto vazio")
	}
}

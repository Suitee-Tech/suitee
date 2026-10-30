package service

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/events"
)

func TestProposalFollowUpWhere(t *testing.T) {
	// Proposta do corretor aguardando o cliente: valor da proposta, em real
	// brasileiro — este texto vai no e-mail ("R$ 350.000,00", nunca
	// "R$ 350000.00").
	if got := proposalFollowUpWhere("client", 350000, nil, 3); got != "no valor de R$ 350.000,00 há 3 dia(s)" {
		t.Errorf("where (client): %q", got)
	}
	// Contraproposta: o valor relevante é o contra-ofertado.
	counter := 320000.0
	if got := proposalFollowUpWhere("broker", 350000, &counter, 5); got != "com contraproposta de R$ 320.000,00 há 5 dia(s)" {
		t.Errorf("where (broker): %q", got)
	}
	// Contraproposta zerada cai no valor da proposta (o texto continua o de
	// contraproposta — comportamento anterior a esta correção, preservado).
	zero := 0.0
	if got := proposalFollowUpWhere("broker", 350000, &zero, 5); got != "com contraproposta de R$ 350.000,00 há 5 dia(s)" {
		t.Errorf("where (counter 0): %q", got)
	}
	// Centavos não podem ser engolidos pelo arredondamento.
	if got := proposalFollowUpWhere("client", 1234.5, nil, 1); got != "no valor de R$ 1.234,50 há 1 dia(s)" {
		t.Errorf("where (centavos): %q", got)
	}
	// Sem valor nenhum, o texto não inventa número.
	if got := proposalFollowUpWhere("client", 0, nil, 4); got != "há 4 dia(s)" {
		t.Errorf("where (sem valor): %q", got)
	}
}

func TestProposalFollowUpCopy(t *testing.T) {
	if got := proposalFollowUpClientSubject(3); !strings.Contains(got, "Ainda aguardando sua resposta") || !strings.Contains(got, "3 dia") {
		t.Errorf("client subject: %q", got)
	}
	if got := proposalFollowUpBrokerSubject(7); !strings.Contains(got, "Contraproposta") {
		t.Errorf("broker subject: %q", got)
	}
	if got := proposalFollowUpCopySubject(); !strings.Contains(got, "Cópia") || !strings.Contains(got, "cliente") {
		t.Errorf("copy subject: %q", got)
	}

	body := proposalFollowUpClientBody("no valor de R$ 350.000,00 há 3 dia(s)")
	for _, want := range []string{"R$ 350.000,00", "ainda não recebemos sua resposta"} {
		if !strings.Contains(body, want) {
			t.Errorf("client body sem %q: %s", want, body)
		}
	}
	broker := proposalFollowUpBrokerBody("com contraproposta de R$ 320.000,00 há 5 dia(s)")
	if !strings.Contains(broker, "contraproposta") {
		t.Errorf("broker body: %q", broker)
	}
	copyBody := proposalFollowUpCopyBody(2, 3, "no valor de R$ 350.000,00 há 10 dia(s)")
	if !strings.Contains(copyBody, "2/3") {
		t.Errorf("copy body sem o estágio: %q", copyBody)
	}

	if got := proposalFollowUpClientInApp(3); !strings.Contains(got, "3 dia") {
		t.Errorf("client in-app: %q", got)
	}
	if got := proposalFollowUpBrokerInApp(7); !strings.Contains(got, "7 dia") {
		t.Errorf("broker in-app: %q", got)
	}
}

// A chave de dedup não pode depender do transporte: eventos que passam pelo
// outbox chegam com o id da linha do outbox como EventID (novo a cada
// publicação), então a chave precisa derivar do payload para que uma
// republicação do mesmo grupo não vire um segundo e-mail.
func TestProposalFollowUpGroupIDIsPayloadAnchored(t *testing.T) {
	a := proposalFollowUpGroupID("prop-1", 1)
	b := proposalFollowUpGroupID("prop-1", 1)
	if a != b {
		t.Fatalf("mesmo grupo deve ter a mesma chave: %s != %s", a, b)
	}
	if want := events.DeriveNotificationID("prop-1", "proposal_followup", "1"); a != want {
		t.Fatalf("chave inesperada: got %s want %s", a, want)
	}
	// Estágio novo (grupo novo) e proposta nova mudam a chave.
	if a == proposalFollowUpGroupID("prop-1", 2) {
		t.Fatal("estágios diferentes não podem compartilhar a chave")
	}
	if a == proposalFollowUpGroupID("prop-2", 1) {
		t.Fatal("propostas diferentes não podem compartilhar a chave")
	}
}

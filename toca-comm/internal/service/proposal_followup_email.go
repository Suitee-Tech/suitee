package service

import (
	"fmt"
	"strconv"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/money"
)

// proposalFollowUpGroupID é a chave de idempotência LÓGICA do lembrete:
// (proposta + estágio). Vale para todos os destinatários do mesmo grupo — o
// ledger comm.sent_externals é único por (event_id, channel, recipient_id),
// então o mesmo grupo chega uma vez em cada destinatário e nunca duas vezes no
// mesmo.
//
// Ela NÃO pode vir de evt.EventID(): eventos publicados pelo outbox saem com o
// id da linha do outbox como MessageId (`publishRaw(ctx, ..., doc.ID)`), um
// valor novo a cada publicação. Com ele, uma republicação do mesmo grupo
// (retry do drainer, replay da DLQ, queda entre publish e mark no produtor)
// viraria um segundo e-mail. Derivando do payload, a chave é a mesma em todas
// as republicações e o dedup colapsa.
func proposalFollowUpGroupID(proposalID string, stage int) string {
	return events.DeriveNotificationID(proposalID, "proposal_followup", strconv.Itoa(stage))
}

// Copy dos e-mails transacionais do lembrete de proposta (beta §F6).
//
// O lembrete existe para cobrar resposta de quem está com a bola: a versão
// "client" vai para o cliente que recebeu a proposta e não respondeu; a versão
// "broker" vai para o corretor quando o cliente devolveu uma contraproposta e
// ela ficou parada.

// proposalFollowUpWhere descreve o que está pendente sem depender do status
// cru do domínio: proposta do corretor aguardando o cliente ou contraproposta
// do cliente aguardando o corretor.
func proposalFollowUpWhere(awaitingFor string, value float64, counterValue *float64, daysPending int) string {
	amount := value
	if counterValue != nil && *counterValue > 0 {
		amount = *counterValue
	}
	noun := "no valor de"
	if awaitingFor == "broker" {
		noun = "com contraproposta de"
	}
	if amount <= 0 {
		return fmt.Sprintf("há %d dia(s)", daysPending)
	}
	return fmt.Sprintf("%s %s há %d dia(s)", noun, money.BRL(amount), daysPending)
}

func proposalFollowUpClientSubject(daysPending int) string {
	return fmt.Sprintf("Ainda aguardando sua resposta sobre a proposta (%d dia(s))", daysPending)
}

func proposalFollowUpClientBody(where string) string {
	return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Enviamos uma proposta %s e ainda não recebemos sua resposta. Se já respondeu, ignore este lembrete.</p>", where)
}

func proposalFollowUpClientInApp(daysPending int) string {
	return fmt.Sprintf("Sua resposta sobre a proposta está pendente há %d dia(s)", daysPending)
}

func proposalFollowUpBrokerSubject(daysPending int) string {
	return fmt.Sprintf("Contraproposta aguardando sua resposta há %d dia(s)", daysPending)
}

func proposalFollowUpBrokerBody(where string) string {
	return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>O cliente enviou uma contraproposta %s e ela ainda não foi respondida.</p>", where)
}

func proposalFollowUpBrokerInApp(daysPending int) string {
	return fmt.Sprintf("Contraproposta do cliente aguardando resposta há %d dia(s)", daysPending)
}

// proposalFollowUpCopySubject marca a cópia ao corretor: ela não pede ação
// dele sobre a proposta (a bola é do cliente), apenas registra que o cliente
// foi cobrado para ele agir no telefone.
func proposalFollowUpCopySubject() string {
	return "[Cópia] Follow-up de proposta enviado ao cliente"
}

func proposalFollowUpCopyBody(stage, maxStages int, where string) string {
	return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Enviamos ao cliente o lembrete %d/%d sobre a proposta %s. Ele ainda não respondeu.</p>", stage, maxStages, where)
}

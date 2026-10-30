package services

import (
	"context"
	"errors"
	"strings"
)

// separationOfDutiesKey marca, no contexto da requisição, que o ator está
// dispensado da segregação de funções entre quem aprova e quem paga.
//
// É um valor de contexto, e não um parâmetro novo em Pay/ReversePayment, porque
// a dispensa é do ATOR (quem está agindo) e não da operação: acrescentar um
// booleano à assinatura sugeriria que o cliente escolhe a política, quando quem
// decide é a autoridade do usuário autenticado.
type separationOfDutiesKey struct{}

// WithSeparationOfDutiesWaiver marca a requisição como dispensada da segregação.
//
// Só a borda HTTP deve chamá-la, e apenas depois de resolver que o ator responde
// pela operação financeira da imobiliária — ver actorHasFinanceAuthority nos
// handlers. O serviço confia na marca porque quem a escreve já autenticou o
// usuário; ela nunca vem do corpo da requisição.
func WithSeparationOfDutiesWaiver(ctx context.Context) context.Context {
	return context.WithValue(ctx, separationOfDutiesKey{}, true)
}

// SeparationOfDutiesWaived diz se a requisição traz a dispensa.
func SeparationOfDutiesWaived(ctx context.Context) bool {
	waived, _ := ctx.Value(separationOfDutiesKey{}).(bool)
	return waived
}

// ErrSegregationOfDutiesApproverCannotPay é devolvido quando o MESMO ator que
// aprovou uma obrigação tenta efetivar o pagamento dela.
//
// SECURITY (achado A13 da revisão de permissões de 2026-09-19): a segregação de
// funções existia apenas no fluxo de managed bills. Repasses, payouts, saques e
// comissões aceitavam o mesmo ator criando, aprovando e pagando — e o papel
// `financeiro` concentra `finance:write` e `finance:approve_transfer`. A
// dispensa explícita (`WithSeparationOfDutiesWaiver`) continua valendo para quem
// responde pela operação financeira da imobiliária.
var ErrSegregationOfDutiesApproverCannotPay = errors.New("segregation of duties: the approver cannot settle the payment")

// ErrSegregationOfDutiesRequesterCannotApprove é devolvido quando o MESMO ator
// que pediu a operação tenta aprová-la (criador ≠ aprovador).
var ErrSegregationOfDutiesRequesterCannotApprove = errors.New("segregation of duties: the requester cannot approve")

// requireDistinctRequesterAndApprover bloqueia a aprovação pelo próprio autor do
// pedido (criador ≠ aprovador). Vazio em qualquer das pontas não bloqueia: nem
// todo registro carrega o autor (imports, workers, dados legados), e inventar
// conflito a partir de campo vazio pararia operação legítima.
func requireDistinctRequesterAndApprover(ctx context.Context, requestedBy, actor string) error {
	requestedBy = strings.TrimSpace(requestedBy)
	actor = strings.TrimSpace(actor)
	if requestedBy == "" || actor == "" || requestedBy != actor {
		return nil
	}
	if SeparationOfDutiesWaived(ctx) {
		return nil
	}
	return ErrSegregationOfDutiesRequesterCannotApprove
}

// requireDistinctApproverAndPayer bloqueia o pagamento por quem aprovou.
func requireDistinctApproverAndPayer(ctx context.Context, approvedBy, actor string) error {
	approvedBy = strings.TrimSpace(approvedBy)
	actor = strings.TrimSpace(actor)
	if approvedBy == "" || actor == "" || approvedBy != actor {
		return nil
	}
	if SeparationOfDutiesWaived(ctx) {
		return nil
	}
	return ErrSegregationOfDutiesApproverCannotPay
}

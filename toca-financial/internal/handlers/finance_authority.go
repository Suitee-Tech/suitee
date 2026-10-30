package handlers

import (
	"context"
	"errors"
	"strings"

	appcontext "github.com/oginoino/toca/pkg/context"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// rolesWithFinanceAuthority são os papéis funcionais que respondem pela operação
// financeira da imobiliária e, por isso, podem aprovar e pagar sozinhos.
//
// Correção de premissa (achado A13 da revisão de permissões de 2026-09-19): o
// comentário anterior afirmava que admin e gestor recebem `finance:write` e
// `finance:approve_transfer` no catálogo — **não recebem**. No catálogo geral
// (`pkg/auth/permission_catalog.go`) os dois defaults são admin e financeiro;
// gestor só teria essas permissões por customização da matriz do tenant.
// "gestor" continua nesta lista por decisão de produto (gestor de operação
// responde pelo financeiro da imobiliária quando a matriz do tenant assim o
// define) e porque removê-lo apertaria o fluxo de quem hoje consegue operar; o
// que não pode continuar é o código afirmar um fato falso. `financeiro` fica
// FORA de propósito: é o operador que a segregação de funções precisa separar
// (criador ≠ aprovador, aprovador ≠ pagador).
var rolesWithFinanceAuthority = map[string]bool{
	"admin":  true,
	"gestor": true,
}

// actorHasFinanceAuthority informa se o ator pode aprovar e pagar sozinho.
//
// Lê o papel funcional do vínculo do usuário com a empresa, que o middleware de
// autenticação propaga do JWT. Sem papel identificado devolve FALSO: a
// segregação continua valendo, porque liberar por ausência de dado seria o pior
// dos defaults — e é a situação desta base enquanto os grupos não têm membros.
func actorHasFinanceAuthority(ctx context.Context) bool {
	role, err := appcontext.GetFunctionalRole(ctx)
	if err != nil {
		return false
	}
	return rolesWithFinanceAuthority[strings.ToLower(strings.TrimSpace(role))]
}

// withFinanceAuthorityWaiver aplica a dispensa da segregação de funções quando o
// ator responde pela operação financeira.
//
// Chamado apenas na borda HTTP, depois da autenticação: a dispensa nunca é
// derivada do corpo da requisição, então um cliente não pode concedê-la a si
// mesmo.
func withFinanceAuthorityWaiver(ctx context.Context) context.Context {
	if !actorHasFinanceAuthority(ctx) {
		return ctx
	}
	return service.WithSeparationOfDutiesWaiver(ctx)
}

// isSegregationOfDutiesError reconhece a recusa por segregação de funções.
//
// É recusa de AUTORIDADE, não de entrada: o pedido está bem formado e o estado
// admite a transição — o que não pode é o mesmo ator ocupar as duas pontas. Sem
// este reconhecimento, transfer e withdrawal respondiam **500** (e a mensagem
// ainda passava pelo sanitizador de erro interno, então o operador não via causa
// nenhuma), enquanto payout e comissão respondiam 400, culpando o corpo da
// requisição. O precedente da casa é 409: managed bills já respondem conflito
// para o mesmo caso e a interface já trata esse status.
func isSegregationOfDutiesError(err error) bool {
	return errors.Is(err, service.ErrSegregationOfDutiesRequesterCannotApprove) ||
		errors.Is(err, service.ErrSegregationOfDutiesApproverCannotPay)
}

// Package insurer abstrai a comunicação com seguradoras de fiança
// locatícia (Porto Seguro, Tokio Marine, Mapfre, …) usada no
// acionamento de garantia da inadimplência (#447).
//
// O layout e as credenciais das APIs de cada seguradora são definidos
// por contrato comercial e variam entre elas. Este pacote define a
// interface de integração (Insurer) e fornece o ManualInsurer — o
// adaptador MVP que representa o processo manual atual: o operador
// aciona a seguradora por fora e registra o protocolo/resposta depois.
//
// Um adaptador real (ex.: porto_seguro.go) implementa a mesma interface
// quando as credenciais estiverem disponíveis, sem mudar o serviço.
package insurer

import "context"

// Request — dados enviados à seguradora ao acionar o seguro fiança.
type Request struct {
	CollectionID string
	ContractID   string
	TenantID     string
	Amount       float64
	Reason       string
}

// Result — resposta da seguradora ao acionamento.
type Result struct {
	// InsurerName identifica a seguradora/adaptador que respondeu.
	InsurerName string
	// Protocol é o número de protocolo do sinistro/acionamento, quando
	// a seguradora já o devolve de imediato.
	Protocol string
	// Message é a resposta textual da seguradora.
	Message string
	// Accepted indica se a seguradora já aceitou o acionamento na
	// resposta síncrona. Para o ManualInsurer é sempre false — a
	// aceitação chega fora de banda e é registrada depois.
	Accepted bool
}

// Insurer aciona o seguro fiança junto a uma seguradora.
type Insurer interface {
	// Activate registra o acionamento e devolve a resposta da seguradora.
	Activate(ctx context.Context, req Request) (Result, error)
	// Name identifica a seguradora/adaptador (para logs e auditoria).
	Name() string
}

// ManualInsurer — adaptador MVP. Não fala com nenhuma API: devolve um
// resultado "protocolo pendente", representando que o operador aciona a
// seguradora manualmente e registra a resposta depois (via
// UpdateGuaranteeStatus). É o default e nunca falha, então o
// acionamento de garantia não trava por indisponibilidade de
// integração.
type ManualInsurer struct{}

// Name identifica o adaptador.
func (ManualInsurer) Name() string { return "manual" }

// Activate registra o acionamento manual — sempre sucede, sem rede.
func (ManualInsurer) Activate(_ context.Context, _ Request) (Result, error) {
	return Result{
		InsurerName: "manual",
		Message:     "Acionamento registrado. Confirme o protocolo junto à seguradora e atualize o status do acionamento.",
		Accepted:    false,
	}, nil
}

// Default devolve o adaptador padrão de seguradora (ManualInsurer).
// Ponto único de troca quando um adaptador real for configurado.
func Default() Insurer { return ManualInsurer{} }

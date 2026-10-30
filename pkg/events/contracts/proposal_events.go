package contracts

import "github.com/google/uuid"

type ProposalSentEventPayload struct {
	DealID             string   `json:"deal_id"`
	TenantID           string   `json:"tenant_id"`
	ProposalID         string   `json:"proposal_id"`
	Value              float64  `json:"value"`
	CustomerAccountIDs []string `json:"customer_account_ids"`
	// CustomerEmails carrega os e-mails das partes cliente (buyer/tenant)
	// que AINDA não têm conta de portal — o consumidor notifica por e-mail
	// direto, sem in-app. Aditivo (beta de e-mail transacional, F4).
	CustomerEmails []string `json:"customer_emails,omitempty"`
}

type ProposalSentEvent struct {
	EventIDVal string                   `json:"event_id"`
	Data       ProposalSentEventPayload `json:"payload"`
}

func (e ProposalSentEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ProposalSentEvent) Name() string { return "proposal.sent" }

func (e ProposalSentEvent) Payload() interface{} { return e.Data }

type ProposalRespondedEventPayload struct {
	DealID            string   `json:"deal_id"`
	TenantID          string   `json:"tenant_id"`
	ProposalID        string   `json:"proposal_id"`
	Action            string   `json:"action"`
	CustomerAccountID string   `json:"customer_account_id"`
	DealValue         *float64 `json:"deal_value,omitempty"`
}

type ProposalRespondedEvent struct {
	EventIDVal string                        `json:"event_id"`
	Data       ProposalRespondedEventPayload `json:"payload"`
}

func (e ProposalRespondedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ProposalRespondedEvent) Name() string { return "proposal.responded" }

func (e ProposalRespondedEvent) Payload() interface{} { return e.Data }

// ProposalFollowUpDueEventPayload — gatilho do lembrete temporal de proposta
// ainda sem resposta (plano beta §F6). Publicado pelo motor em toca-deal
// (varredura periódica) e consumido pelo toca-comm, que decide canal
// (e-mail) e aplica os gates de opt-out/cooldown/dedup.
//
// `stage` é o grupo do lembrete: cada proposta só dispara UM evento por
// estágio, com event_id estável derivado de (proposal_id, stage). Enquanto
// a proposta continuar aguardando a mesma resposta, as varreduras seguintes
// reproduzem o mesmo event_id e o dedup do consumidor as colapsa — o
// lembrete nunca se repete por passar o tempo.
type ProposalFollowUpDueEventPayload struct {
	DealID         string `json:"deal_id"`
	TenantID       string `json:"tenant_id"`
	ProposalID     string `json:"proposal_id"`
	Stage          int    `json:"stage"`
	MaxStages      int    `json:"max_stages"`
	DaysPending    int    `json:"days_pending"`
	ProposalStatus string `json:"proposal_status"`
	// AwaitingFor diz quem está com a resposta: "client" (proposta enviada
	// pelo corretor aguardando o cliente) ou "broker" (contraproposta do
	// cliente aguardando o corretor).
	AwaitingFor        string   `json:"awaiting_for"`
	Value              float64  `json:"value"`
	CounterValue       *float64 `json:"counter_value,omitempty"`
	BrokerID           string   `json:"broker_id"`
	CustomerAccountIDs []string `json:"customer_account_ids"`
	CustomerEmails     []string `json:"customer_emails,omitempty"`
	ProposalCreatedAt  string   `json:"proposal_created_at"`
}

type ProposalFollowUpDueEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       ProposalFollowUpDueEventPayload `json:"payload"`
}

func (e ProposalFollowUpDueEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ProposalFollowUpDueEvent) Name() string { return "proposal.followup_due" }

func (e ProposalFollowUpDueEvent) Payload() interface{} { return e.Data }

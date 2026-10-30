package contracts

import (
	"time"

	"github.com/google/uuid"
)

// FinancialProofUploadedEventPayload — emitido quando um comprovante
// financeiro (payout/charge/commission/transaction) é enviado ao bucket
// privado `financial-proofs`. O subscriber do toca-deal materializa um
// Document de categoria `comprovante` para que o arquivo apareça
// centralizado e classificado na Central de Documentos (/documentos), sob a
// origem "financeiro".
type FinancialProofUploadedEventPayload struct {
	TenantID    string `json:"tenant_id"`
	Kind        string `json:"kind"`         // payout | charge | commission | transaction
	URL         string `json:"url"`          // caminho interno /api/v1/proofs/content/<token>
	Filename    string `json:"filename"`     // nome original enviado pelo operador
	ContentType string `json:"content_type"` // MIME detectado (application/pdf, image/png...)
	SizeBytes   int64  `json:"size_bytes"`
	ObjectKey   string `json:"object_key"` // chave no bucket financial-proofs (fonte dos bytes)
	UploadedBy  string `json:"uploaded_by,omitempty"`
}

type FinancialProofUploadedEvent struct {
	EventIDVal string                             `json:"event_id"`
	Data       FinancialProofUploadedEventPayload `json:"payload"`
}

func (e FinancialProofUploadedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e FinancialProofUploadedEvent) Name() string         { return "financial.proof_uploaded" }
func (e FinancialProofUploadedEvent) Payload() interface{} { return e.Data }

// ChargePaidEventPayload represents the full settlement of the canonical
// receivable. It deliberately has no invoice_id: a Charge may be settled by
// several invoices, a manual reconciliation, or a future PSP webhook.
type ChargePaidEventPayload struct {
	ChargeID   string  `json:"charge_id"`
	TenantID   string  `json:"tenant_id"`
	ContractID string  `json:"contract_id"`
	DealID     string  `json:"deal_id,omitempty"`
	Type       string  `json:"type"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	PaidAt     string  `json:"paid_at"`
}

type ChargePaidEvent struct {
	EventIDVal string                 `json:"event_id"`
	Data       ChargePaidEventPayload `json:"payload"`
}

func (e ChargePaidEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ChargePaidEvent) Name() string         { return "charge.paid" }
func (e ChargePaidEvent) Payload() interface{} { return e.Data }

// ChargeOverdueEventPayload — fires the moment a Charge transitions from
// PENDING to OVERDUE (the delinquency engine in toca-financial runs this
// daily). Auditing subscribers in toca-deal and toca-comm use this to
// surface the timeline event and dispatch the late-payment notification
// flow respectively.
type ChargeOverdueEventPayload struct {
	ChargeID    string    `json:"charge_id"`
	ContractID  string    `json:"contract_id"`
	TenantID    string    `json:"tenant_id"`
	DebtorID    string    `json:"debtor_id"`
	Amount      float64   `json:"amount"`
	TotalAmount float64   `json:"total_amount"`
	DaysOverdue int       `json:"days_overdue"`
	DueDate     time.Time `json:"due_date"`
	Timestamp   string    `json:"timestamp"`
}

type ChargeOverdueEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       ChargeOverdueEventPayload `json:"payload"`
}

func (e ChargeOverdueEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ChargeOverdueEvent) Name() string         { return "charge.overdue" }
func (e ChargeOverdueEvent) Payload() interface{} { return e.Data }

// CollectionEscalatedEventPayload — emitted whenever a Collection moves
// between status buckets (ACTIVE → NEGOTIATED → GUARANTEE_ACTIVATED →
// JUDICIAL → RESOLVED). The contract-side subscriber (audit only) writes
// the transition to the audit log; toca-deal does not mutate Contract
// state — operators query the live collection status via the financial
// service when they need it.
type CollectionEscalatedEventPayload struct {
	CollectionID   string  `json:"collection_id"`
	ContractID     string  `json:"contract_id"`
	TenantID       string  `json:"tenant_id"`
	DebtorID       string  `json:"debtor_id"`
	PreviousStatus string  `json:"previous_status"`
	NewStatus      string  `json:"new_status"`
	TotalDebt      float64 `json:"total_debt"`
	EscalatedBy    string  `json:"escalated_by,omitempty"`
	Timestamp      string  `json:"timestamp"`
}

type CollectionEscalatedEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       CollectionEscalatedEventPayload `json:"payload"`
}

func (e CollectionEscalatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CollectionEscalatedEvent) Name() string         { return "collection.escalated" }
func (e CollectionEscalatedEvent) Payload() interface{} { return e.Data }

// InspectionDeductionsRecordedEventPayload — fires when the operator
// closes an EXIT inspection's deductions tab. Each Deduction becomes a
// Charge type=OTHER against the locatário in toca-financial so the
// security-deposit balance reflects the new debit before the operator
// calls TerminateContract.
type DeductionLine struct {
	Room        string  `json:"room"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

type InspectionDeductionsRecordedEventPayload struct {
	InspectionID string          `json:"inspection_id"`
	TenantID     string          `json:"tenant_id"`
	ContractID   string          `json:"contract_id"`
	Deductions   []DeductionLine `json:"deductions"`
	TotalAmount  float64         `json:"total_amount"`
	Timestamp    string          `json:"timestamp"`
}

type InspectionDeductionsRecordedEvent struct {
	EventIDVal string                                   `json:"event_id"`
	Data       InspectionDeductionsRecordedEventPayload `json:"payload"`
}

func (e InspectionDeductionsRecordedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e InspectionDeductionsRecordedEvent) Name() string {
	return "inspection.deductions_recorded"
}
func (e InspectionDeductionsRecordedEvent) Payload() interface{} { return e.Data }

// CollectionCreatedEventPayload — publicado pelo motor de inadimplência do
// toca-financial quando um caso de cobrança é aberto automaticamente.
//
// VivIA dentro do serviço (`toca-financial/internal/services`), o que impedia
// qualquer outro módulo de assinar o evento (pacotes `internal/` não são
// importáveis fora do módulo) — na prática o evento não tinha e não podia ter
// consumidor. Movido para cá, que é onde os contratos compartilhados moram.
type CollectionCreatedEventPayload struct {
	CollectionID string  `json:"collection_id"`
	ContractID   string  `json:"contract_id"`
	TenantID     string  `json:"tenant_id"`
	DebtorID     string  `json:"debtor_id"`
	TotalDebt    float64 `json:"total_debt"`
	DaysOverdue  int     `json:"days_overdue"`
	ChargeCount  int     `json:"charge_count"`
	Timestamp    string  `json:"timestamp,omitempty"`
}

type CollectionCreatedEvent struct {
	EventIDVal string                        `json:"event_id"`
	Data       CollectionCreatedEventPayload `json:"payload"`
}

// EventID é determinístico por caso: um consumidor que reprocesse a mensagem
// (retry, replay de DLQ) deduplica pelo par (consumer, message_id). Antes
// devolvia string vazia, o que quebrava a deduplicação de qualquer assinante.
func (e CollectionCreatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CollectionCreatedEvent) Name() string         { return "collection.created" }
func (e CollectionCreatedEvent) Payload() interface{} { return e.Data }

// CollectionAgreementEventPayload — mudanças do acordo formalizado com o devedor
// (Fase 2 do plano de inadimplência). Um único payload serve aos quatro eventos
// do ciclo (created/broken/cancelled/fulfilled) porque os campos são os mesmos e
// o que muda é o desfecho — criar quatro structs quase idênticas só multiplicaria
// o custo de manter contrato.
type CollectionAgreementEventPayload struct {
	AgreementID    string   `json:"agreement_id"`
	CollectionID   string   `json:"collection_id"`
	ContractID     string   `json:"contract_id"`
	TenantID       string   `json:"tenant_id"`
	TotalAmount    float64  `json:"total_amount"`
	Discount       float64  `json:"discount"`
	Installments   int      `json:"installments"`
	OriginalTotal  float64  `json:"original_total"`
	InstallmentIDs []string `json:"installment_charge_ids,omitempty"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason,omitempty"`
	Timestamp      string   `json:"timestamp,omitempty"`
}

// CollectionAgreementEvent carrega o nome do evento no próprio dado: o domínio
// emite quatro nomes (`collection.agreement_created|broken|cancelled|fulfilled`)
// com o mesmo formato, e o roteamento é por nome — sem isso seriam quatro tipos
// idênticos só para trocar a string.
type CollectionAgreementEvent struct {
	EventIDVal string                          `json:"event_id"`
	EventName  string                          `json:"-"`
	Data       CollectionAgreementEventPayload `json:"payload"`
}

func (e CollectionAgreementEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CollectionAgreementEvent) Name() string {
	if e.EventName == "" {
		return "collection.agreement_created"
	}
	return e.EventName
}

func (e CollectionAgreementEvent) Payload() interface{} { return e.Data }

// CollectionSettlementEventPayload — baixa por garantia (indenização recebida)
// e desfecho judicial. Registra quanto entrou, quanto foi baixado e o que
// sobrou, porque é esse trio que a auditoria e a conciliação precisam conferir.
type CollectionSettlementEventPayload struct {
	CollectionID       string   `json:"collection_id"`
	ContractID         string   `json:"contract_id"`
	TenantID           string   `json:"tenant_id"`
	GuaranteeType      string   `json:"guarantee_type,omitempty"`
	SettledAmount      float64  `json:"settled_amount"`
	AppliedAmount      float64  `json:"applied_amount"`
	CreditAmount       float64  `json:"credit_amount,omitempty"`
	SettledChargeIDs   []string `json:"settled_charge_ids,omitempty"`
	Reference          string   `json:"reference,omitempty"`
	ResolvedCollection bool     `json:"resolved_collection"`
	JudicialStatus     string   `json:"judicial_status,omitempty"`
	Timestamp          string   `json:"timestamp,omitempty"`
}

type CollectionSettledEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       CollectionSettlementEventPayload `json:"payload"`
}

func (e CollectionSettledEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CollectionSettledEvent) Name() string         { return "collection.guarantee_settled" }
func (e CollectionSettledEvent) Payload() interface{} { return e.Data }

type CollectionJudicialClosedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       CollectionSettlementEventPayload `json:"payload"`
}

func (e CollectionJudicialClosedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CollectionJudicialClosedEvent) Name() string         { return "collection.judicial_closed" }
func (e CollectionJudicialClosedEvent) Payload() interface{} { return e.Data }

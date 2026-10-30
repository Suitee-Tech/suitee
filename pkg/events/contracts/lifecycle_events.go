package contracts

import "github.com/google/uuid"

// ContractTerminatedEventPayload é publicado por toca-deal quando um contrato
// de locação é encerrado. Consumido por toca-financial para encerrar a
// RentalChargeConfig, marcar Rental como ENDED, lançar a multa por rescisão
// (quando aplicável) e processar o acerto de caução.
type ContractTerminatedEventPayload struct {
	TenantID            string  `json:"tenant_id"`
	ContractID          string  `json:"contract_id"`
	DealID              string  `json:"deal_id,omitempty"`
	PropertyID          string  `json:"property_id,omitempty"`
	OwnerID             string  `json:"owner_id,omitempty"`
	CustomerID          string  `json:"customer_id,omitempty"`
	TerminationDate     string  `json:"termination_date"`
	Reason              string  `json:"reason,omitempty"`
	// EarlyTerminationFee é o valor da multa por rescisão antecipada (R$).
	// Quando 0, nenhum charge de multa é gerado.
	EarlyTerminationFee float64 `json:"early_termination_fee,omitempty"`
	// SecurityDeposit é o valor da caução (R$) a ser devolvida ao locatário,
	// já descontando danos identificados na vistoria de saída.
	SecurityDeposit          float64 `json:"security_deposit,omitempty"`
	SecurityDepositDeductions float64 `json:"security_deposit_deductions,omitempty"`
	TriggeredBy              string  `json:"triggered_by,omitempty"`
	Timestamp                string  `json:"timestamp"`
}

type ContractTerminatedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       ContractTerminatedEventPayload `json:"payload"`
}

func (e ContractTerminatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ContractTerminatedEvent) Name() string         { return "contract.terminated" }
func (e ContractTerminatedEvent) Payload() interface{} { return e.Data }

// ContractReadjustedEventPayload é publicado por toca-deal quando o valor
// de aluguel é reajustado (manual ou pelo job de auto-reajuste).
// toca-financial atualiza RentalChargeConfig.MonthlyAmount e registra audit
// trail. Charges já emitidos não são alteradas.
type ContractReadjustedEventPayload struct {
	TenantID       string  `json:"tenant_id"`
	ContractID     string  `json:"contract_id"`
	DealID         string  `json:"deal_id,omitempty"`
	PreviousValue  float64 `json:"previous_value"`
	NewValue       float64 `json:"new_value"`
	IndexName      string  `json:"index_name,omitempty"`
	IndexValue     float64 `json:"index_value,omitempty"`
	EffectiveFrom  string  `json:"effective_from"`
	NextAdjustment string  `json:"next_adjustment,omitempty"`
	TriggeredBy    string  `json:"triggered_by,omitempty"`
	Timestamp      string  `json:"timestamp"`
}

type ContractReadjustedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       ContractReadjustedEventPayload `json:"payload"`
}

func (e ContractReadjustedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ContractReadjustedEvent) Name() string         { return "contract.readjusted" }
func (e ContractReadjustedEvent) Payload() interface{} { return e.Data }

// ContractAmendedEventPayload é publicado por toca-deal quando um aditivo
// (novo contrato DRAFT que substitui o vigente) é emitido. Consumido por
// toca-comm para notificar os signatários e por analytics. NÃO altera a
// RentalChargeConfig — isso só acontece quando o aditivo é assinado, momento
// em que o ciclo já segue por contract.signed do contrato novo.
type ContractAmendedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	OriginalContractID string `json:"original_contract_id"`
	NewContractID      string `json:"new_contract_id"`
	DealID             string `json:"deal_id,omitempty"`
	Reason             string `json:"reason,omitempty"`
	TriggeredBy        string `json:"triggered_by,omitempty"`
	Timestamp          string `json:"timestamp"`
}

type ContractAmendedEvent struct {
	EventIDVal string                      `json:"event_id"`
	Data       ContractAmendedEventPayload `json:"payload"`
}

func (e ContractAmendedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ContractAmendedEvent) Name() string         { return "contract.amended" }
func (e ContractAmendedEvent) Payload() interface{} { return e.Data }

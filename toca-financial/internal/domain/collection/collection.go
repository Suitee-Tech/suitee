package collection

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive             Status = "ACTIVE"
	StatusNegotiated         Status = "NEGOTIATED"
	StatusGuaranteeActivated Status = "GUARANTEE_ACTIVATED"
	StatusJudicial           Status = "JUDICIAL"
	StatusResolved           Status = "RESOLVED"
)

var (
	ErrInvalidStatusTransition  = errors.New("invalid collection status transition")
	ErrConcurrentModification   = errors.New("collection changed concurrently")
	ErrStatusTransitionRequired = errors.New("collection status must be changed through UpdateStatus")
)

// ValidStatus rejects arbitrary strings before they reach the reporting and
// delinquency state machines.
func ValidStatus(status Status) bool {
	switch status {
	case StatusActive, StatusNegotiated, StatusGuaranteeActivated, StatusJudicial, StatusResolved:
		return true
	default:
		return false
	}
}

// CanTransitionTo valida o ciclo de vida do caso.
//
// A máquina era estritamente para a frente e `RESOLVED` terminal — o que está
// certo para encerramento, mas não para a realidade do fluxo: garantia recusada,
// acordo quebrado e sentença improcedente/arquivada são desfechos que DEVOLVEM o
// caso à cobrança. Sem essas voltas, o caso ficava preso em
// GUARANTEE_ACTIVATED/JUDICIAL para sempre e a dívida saía do radar (o único
// caminho era RESOLVED, que é encerramento).
//
// As voltas são explícitas (não é um "edite o status livremente") e cada uma
// exige motivo na camada de serviço, com auditoria:
//
//	GUARANTEE_ACTIVATED → ACTIVE      garantia recusada/cancelada
//	GUARANTEE_ACTIVATED → NEGOTIATED  acordo fechado com a garantia em curso
//	NEGOTIATED          → ACTIVE      acordo quebrado ou cancelado
//	JUDICIAL            → ACTIVE      sentença improcedente / processo arquivado
//	JUDICIAL            → NEGOTIATED  acordo homologado em juízo
func (c *Collection) CanTransitionTo(next Status) bool {
	if c == nil || !ValidStatus(next) {
		return false
	}
	if c.Status == next {
		return true
	}
	switch c.Status {
	case StatusActive:
		return next == StatusNegotiated || next == StatusGuaranteeActivated || next == StatusJudicial || next == StatusResolved
	case StatusNegotiated:
		return next == StatusActive || next == StatusGuaranteeActivated || next == StatusJudicial || next == StatusResolved
	case StatusGuaranteeActivated:
		// NEGOTIATED entra aqui porque o devedor pode fechar acordo com a
		// garantia ainda em curso (o acordo supera o acionamento). A tela já
		// oferecia a ação nesse estado; o backend recusava com 409 e a validação
		// no beta esbarrou nisso.
		return next == StatusActive || next == StatusNegotiated || next == StatusJudicial || next == StatusResolved
	case StatusJudicial:
		return next == StatusActive || next == StatusNegotiated || next == StatusResolved
	case StatusResolved:
		return false
	default:
		return false
	}
}

// CollectionStats represents collection case statistics
type CollectionStats struct {
	Active             int     `json:"active"`
	Negotiated         int     `json:"negotiated"`
	GuaranteeActivated int     `json:"guarantee_activated"`
	Judicial           int     `json:"judicial"`
	Resolved           int     `json:"resolved"`
	TotalDebt          float64 `json:"total_debt"`
	RecoveredAmount    float64 `json:"recovered_amount"`
}

type Action struct {
	Date        time.Time `json:"date" bson:"date"`
	Type        string    `json:"type" bson:"type"`
	Description string    `json:"description" bson:"description"`
	UserID      string    `json:"user_id" bson:"user_id"`
}

type Collection struct {
	ID               string   `json:"id" bson:"_id"`
	TenantID         string   `json:"tenant_id" bson:"tenant_id"`
	ContractID       string   `json:"contract_id" bson:"contract_id"`
	DebtorID         string   `json:"debtor_id" bson:"debtor_id"`
	DebtorName       string   `json:"debtor_name" bson:"debtor_name"`
	TotalDebt        float64  `json:"total_debt" bson:"total_debt"`
	DaysOverdue      int      `json:"days_overdue" bson:"days_overdue"`
	ChargeIDs        []string `json:"charge_ids" bson:"charge_ids"`
	Status           Status   `json:"status" bson:"status"`
	Actions          []Action `json:"actions" bson:"actions"`
	NegotiatedAmount *float64 `json:"negotiated_amount,omitempty" bson:"negotiated_amount,omitempty"`
	// CreditBalance é o excedente recebido a maior (ex.: indenização de garantia
	// que supera a dívida). Fica registrado no caso em vez de ser descartado: o
	// dinheiro entrou e precisa ter destino declarado — abate a próxima cobrança
	// do mesmo contrato ou é devolvido, decisão do operador.
	CreditBalance float64 `json:"credit_balance,omitempty" bson:"credit_balance,omitempty"`
	// Agreement é o acordo vigente do caso, quando existe. Preenchido na leitura
	// (não é persistido no metadata do caso — a verdade está em
	// financial.collection_agreements).
	Agreement      *Agreement `json:"agreement,omitempty" bson:"-"`
	NegotiatedDate *time.Time `json:"negotiated_date,omitempty" bson:"negotiated_date,omitempty"`
	Notes          string     `json:"notes,omitempty" bson:"notes,omitempty"`
	// Judicial — metadados do processo judicial, presente quando a
	// cobrança escalou para JUDICIAL (#448). Embutido no metadata JSONB.
	Judicial *JudicialProcess `json:"judicial,omitempty" bson:"judicial,omitempty"`
	// Guarantee — registro do acionamento da garantia locatícia,
	// presente quando a cobrança escalou para GUARANTEE_ACTIVATED (#447).
	Guarantee *GuaranteeActivation `json:"guarantee,omitempty" bson:"guarantee,omitempty"`
	// SourceEventID — chave de idempotência derivada do header
	// Idempotency-Key. Backed por índice único parcial no PostgreSQL
	// (uq_financial_collections_source_event_id) para evitar duplicatas
	// em re-submissões de criação manual (FIN-COM-12).
	SourceEventID string    `json:"source_event_id,omitempty" bson:"source_event_id,omitempty"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// AddCharge adds a charge ID to the collection if not already present
func (c *Collection) AddCharge(chargeID string) {
	for _, id := range c.ChargeIDs {
		if id == chargeID {
			return
		}
	}
	c.ChargeIDs = append(c.ChargeIDs, chargeID)
	c.UpdatedAt = time.Now()
}

// RecalculateDebt updates total debt and days overdue
func (c *Collection) RecalculateDebt(totalDebt float64, maxDaysOverdue int) {
	c.TotalDebt = totalDebt
	c.DaysOverdue = maxDaysOverdue
	c.UpdatedAt = time.Now()
}

func New(contractID, debtorID, debtorName, tenantID string, totalDebt float64, daysOverdue int, chargeIDs []string) *Collection {
	now := time.Now()
	return &Collection{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		ContractID:  contractID,
		DebtorID:    debtorID,
		DebtorName:  debtorName,
		TotalDebt:   totalDebt,
		DaysOverdue: daysOverdue,
		ChargeIDs:   chargeIDs,
		Status:      StatusActive,
		Actions:     []Action{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ── Processo Judicial (#448) ──

// JudicialStatus — andamento da ação de cobrança no tribunal.
type JudicialStatus string

const (
	JudicialStatusEmTramite            JudicialStatus = "EM_TRAMITE"
	JudicialStatusAcordo               JudicialStatus = "ACORDO"
	JudicialStatusSentencaProcedente   JudicialStatus = "SENTENCA_PROCEDENTE"
	JudicialStatusSentencaImprocedente JudicialStatus = "SENTENCA_IMPROCEDENTE"
	JudicialStatusArquivado            JudicialStatus = "ARQUIVADO"
)

// ValidJudicialStatus reporta se s é um JudicialStatus reconhecido.
func ValidJudicialStatus(s JudicialStatus) bool {
	switch s {
	case JudicialStatusEmTramite, JudicialStatusAcordo,
		JudicialStatusSentencaProcedente, JudicialStatusSentencaImprocedente,
		JudicialStatusArquivado:
		return true
	}
	return false
}

// Lawyer — advogado responsável pela ação de cobrança.
type Lawyer struct {
	Name  string `json:"name" bson:"name"`
	OAB   string `json:"oab,omitempty" bson:"oab,omitempty"`
	Email string `json:"email,omitempty" bson:"email,omitempty"`
	Phone string `json:"phone,omitempty" bson:"phone,omitempty"`
}

// JudicialProcess — metadados do processo judicial aberto quando a
// cobrança escala para JUDICIAL (#448). Relação 1:1 com a Collection;
// embutido no metadata JSONB, sem tabela própria — consistente com o
// padrão de persistência de financial.collections.
type JudicialProcess struct {
	// CaseNumber — número único do processo no padrão CNJ
	// (NNNNNNN-DD.AAAA.J.TR.OOOO). Validar com ValidateCNJ.
	CaseNumber  string         `json:"case_number" bson:"case_number"`
	Court       string         `json:"court" bson:"court"` // vara / comarca
	Lawyer      Lawyer         `json:"lawyer" bson:"lawyer"`
	ClaimValue  float64        `json:"claim_value" bson:"claim_value"` // valor da causa
	Status      JudicialStatus `json:"status" bson:"status"`
	NextHearing *time.Time     `json:"next_hearing,omitempty" bson:"next_hearing,omitempty"`
	Notes       string         `json:"notes,omitempty" bson:"notes,omitempty"`
	// Costs são as custas e honorários do processo. Antes o sistema registrava
	// o processo e não registrava quanto ele custou: o valor gasto não entrava
	// em lugar nenhum (G12 do plano). Cada item vira uma despesa lançada no
	// financeiro no momento do desfecho.
	Costs []JudicialCost `json:"costs,omitempty" bson:"costs,omitempty"`
	// DecidedAt é a data da decisão/desfecho, informada pelo operador.
	DecidedAt *time.Time `json:"decided_at,omitempty" bson:"decided_at,omitempty"`
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" bson:"updated_at"`
}

// JudicialCost — uma despesa do processo (custas, honorários, perícia).
type JudicialCost struct {
	Description string  `json:"description" bson:"description"`
	Amount      float64 `json:"amount" bson:"amount"`
}

// IsJudicialOutcome reporta se o status encerra a fase judicial.
//
// EM_TRAMITE é andamento; os outros quatro são desfechos. A distinção importa
// porque cada desfecho tem um efeito diferente sobre o caso (ver
// judicialOutcomeTargetStatus).
func (s JudicialStatus) IsJudicialOutcome() bool {
	switch s {
	case JudicialStatusAcordo, JudicialStatusSentencaProcedente,
		JudicialStatusSentencaImprocedente, JudicialStatusArquivado:
		return true
	}
	return false
}

// judicialOutcomeTargetStatus traduz o desfecho judicial no estado do caso.
//
//	ACORDO                  → NEGOTIATED (o acordo é formalizado depois, em /agreement)
//	SENTENCA_PROCEDENTE     → JUDICIAL    (segue aguardando o recebimento)
//	SENTENCA_IMPROCEDENTE   → ACTIVE      (volta a cobrar; a régua retoma)
//	ARQUIVADO               → ACTIVE      (volta a cobrar; a régua retoma)
//
// Devolve o próprio status atual quando não há mudança.
func (s JudicialStatus) judicialOutcomeTargetStatus(atual Status) Status {
	switch s {
	case JudicialStatusAcordo:
		return StatusNegotiated
	case JudicialStatusSentencaImprocedente, JudicialStatusArquivado:
		return StatusActive
	default:
		return atual
	}
}

// OutcomeTargetStatus é a versão exportada usada pelos serviços.
func (s JudicialStatus) OutcomeTargetStatus(atual Status) Status {
	return s.judicialOutcomeTargetStatus(atual)
}

var cnjPattern = regexp.MustCompile(`^\d{7}-\d{2}\.\d{4}\.\d\.\d{2}\.\d{4}$`)

// ValidateCNJ confere se s é um número de processo no formato unificado
// do CNJ. Aceita tanto a forma com máscara (NNNNNNN-DD.AAAA.J.TR.OOOO)
// quanto 20 dígitos sem máscara — em ambos os casos o número tem
// exatamente 20 dígitos (Resolução CNJ 65/2008).
func ValidateCNJ(s string) bool {
	s = strings.TrimSpace(s)
	if cnjPattern.MatchString(s) {
		return true
	}
	// Sem máscara: exatamente 20 dígitos, nada mais.
	digits := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
		digits++
	}
	return digits == 20
}

// ── Acionamento de Garantia (#447) ──

// GuaranteeType — modalidade da garantia locatícia acionada.
type GuaranteeType string

const (
	GuaranteeTypeSeguroFianca GuaranteeType = "SEGURO_FIANCA"
	GuaranteeTypeFiador       GuaranteeType = "FIADOR"
	GuaranteeTypeCaucao       GuaranteeType = "CAUCAO"
)

// ValidGuaranteeType reporta se t é uma modalidade de garantia conhecida.
func ValidGuaranteeType(t GuaranteeType) bool {
	switch t {
	case GuaranteeTypeSeguroFianca, GuaranteeTypeFiador, GuaranteeTypeCaucao:
		return true
	}
	return false
}

// GuaranteeActivationStatus — estágio do acionamento da garantia.
type GuaranteeActivationStatus string

const (
	GuaranteeStatusRequested GuaranteeActivationStatus = "REQUESTED" // acionamento aberto
	GuaranteeStatusAccepted  GuaranteeActivationStatus = "ACCEPTED"  // garantidor aceitou
	GuaranteeStatusRejected  GuaranteeActivationStatus = "REJECTED"  // acionamento recusado
	GuaranteeStatusPaid      GuaranteeActivationStatus = "PAID"      // valor recuperado
	GuaranteeStatusCancelled GuaranteeActivationStatus = "CANCELLED" // acionamento cancelado
)

// ValidGuaranteeActivationStatus reporta se s é um status reconhecido.
func ValidGuaranteeActivationStatus(s GuaranteeActivationStatus) bool {
	switch s {
	case GuaranteeStatusRequested, GuaranteeStatusAccepted, GuaranteeStatusRejected,
		GuaranteeStatusPaid, GuaranteeStatusCancelled:
		return true
	}
	return false
}

// IsTerminal reporta se o acionamento chegou a um estado final — não há
// mais transições de status esperadas.
//
// ACCEPTED NÃO é terminal: é o meio do caminho. A seguradora/fiador aceita o
// acionamento e depois paga (ou desiste) — tratar o aceite como fim impedia
// registrar o dinheiro que efetivamente entrou, que é justamente o que a Fase 2
// veio corrigir. Terminais são os desfechos: pago, recusado, cancelado.
func (s GuaranteeActivationStatus) IsTerminal() bool {
	switch s {
	case GuaranteeStatusRejected, GuaranteeStatusPaid, GuaranteeStatusCancelled:
		return true
	}
	return false
}

// PodeReceberIndenizacao reporta se o acionamento ainda pode virar PAID.
func (s GuaranteeActivationStatus) PodeReceberIndenizacao() bool {
	return s == GuaranteeStatusRequested || s == GuaranteeStatusAccepted
}

// GuaranteeActivation — registro do acionamento da garantia locatícia,
// disparado quando a inadimplência ultrapassa o limite (#447). Relação
// 1:1 com a Collection; embutido no metadata JSONB, sem tabela própria.
type GuaranteeActivation struct {
	Type   GuaranteeType             `json:"type" bson:"type"`
	Amount float64                   `json:"amount" bson:"amount"` // valor acionado
	Reason string                    `json:"reason,omitempty" bson:"reason,omitempty"`
	Status GuaranteeActivationStatus `json:"status" bson:"status"`
	// InsurerName / InsurerProtocol / InsurerResponse — preenchidos
	// quando a garantia é Seguro Fiança (o adaptador da seguradora
	// responde com protocolo). Para Fiador/Caução ficam vazios.
	InsurerName     string     `json:"insurer_name,omitempty" bson:"insurer_name,omitempty"`
	InsurerProtocol string     `json:"insurer_protocol,omitempty" bson:"insurer_protocol,omitempty"`
	InsurerResponse string     `json:"insurer_response,omitempty" bson:"insurer_response,omitempty"`
	RequestedAt     time.Time  `json:"requested_at" bson:"requested_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty" bson:"resolved_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at" bson:"updated_at"`
}

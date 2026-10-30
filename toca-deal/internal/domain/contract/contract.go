package contract

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft           Status = "DRAFT"
	StatusPending         Status = "PENDING_SIGNATURE"
	StatusPartiallySigned Status = "PARTIALLY_SIGNED"
	StatusSigned          Status = "SIGNED"
	StatusActive          Status = "ACTIVE"
	StatusExpired         Status = "EXPIRED"
	StatusTerminated      Status = "TERMINATED"
)

// SignatoryStatus mirrors toca-signature/internal/domain/envelope.SignerStatus.
// Kept as a string-only enum here so toca-deal stays a pure consumer of the
// signature service's source of truth — no transitions implemented locally.
type SignatoryStatus string

const (
	SignatoryStatusPending SignatoryStatus = "PENDING"
	SignatoryStatusViewed  SignatoryStatus = "VIEWED"
	SignatoryStatusSigned  SignatoryStatus = "SIGNED"
	SignatoryStatusRefused SignatoryStatus = "REFUSED"
)

type ContractType string

const (
	TypeVenda   ContractType = "venda"
	TypeLocacao ContractType = "locacao"
)

var (
	ErrInvalidSignatoryRole  = errors.New("invalid signatory role")
	ErrInvalidContractType   = errors.New("invalid contract type")
	ErrInvalidContractPeriod = errors.New("invalid contract period")
)

const DefaultLeaseTermMonths = 30

type SignatoryRole string

const (
	RoleInquilino    SignatoryRole = "inquilino"
	RoleProprietario SignatoryRole = "proprietario"
	RoleFiador       SignatoryRole = "fiador"
	RoleLocador      SignatoryRole = "locador"
	RoleLocatario    SignatoryRole = "locatario"

	RoleComprador SignatoryRole = "comprador"
	RoleVendedor  SignatoryRole = "vendedor"
	RoleAvalista  SignatoryRole = "avalista"

	RoleTestemunha         SignatoryRole = "testemunha"
	RoleConjuge            SignatoryRole = "conjuge"
	RoleProcurador         SignatoryRole = "procurador"
	RoleRepresentanteLegal SignatoryRole = "representante_legal"
	RoleTutor              SignatoryRole = "tutor"
	RoleCurador            SignatoryRole = "curador"
)

// AdjustmentEntry registra um reajuste aplicado ao contrato (manual ou
// automático). Mantém histórico — não substitui valores anteriores.
type AdjustmentEntry struct {
	AppliedAt     time.Time `json:"applied_at" bson:"applied_at"`
	PreviousValue float64   `json:"previous_value" bson:"previous_value"`
	NewValue      float64   `json:"new_value" bson:"new_value"`
	IndexName     string    `json:"index_name,omitempty" bson:"index_name,omitempty"`
	IndexValue    float64   `json:"index_value,omitempty" bson:"index_value,omitempty"`
	EffectiveFrom time.Time `json:"effective_from" bson:"effective_from"`
	TriggeredBy   string    `json:"triggered_by,omitempty" bson:"triggered_by,omitempty"`
	// Mode é "manual" ou "auto"; preserva a origem do reajuste para auditoria.
	Mode string `json:"mode,omitempty" bson:"mode,omitempty"`
	Note string `json:"note,omitempty" bson:"note,omitempty"`
}

// AmendmentEntry registra um aditivo emitido a partir deste contrato — o
// aditivo é, na prática, um novo contrato (NewContractID) que substitui o
// vigente quando assinado. Mantemos histórico aqui para que o operador
// possa rastrear todos os aditivos pelo contrato original.
type AmendmentEntry struct {
	NewContractID string    `json:"new_contract_id" bson:"new_contract_id"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	Reason        string    `json:"reason,omitempty" bson:"reason,omitempty"`
	TriggeredBy   string    `json:"triggered_by,omitempty" bson:"triggered_by,omitempty"`
	// SupersededAt é preenchido quando o aditivo é assinado e o contrato
	// original passa para EXPIRED. Antes disso fica vazio.
	SupersededAt *time.Time `json:"superseded_at,omitempty" bson:"superseded_at,omitempty"`
}

// Termination consolida os campos de encerramento — preferimos um sub-objeto
// nullable a múltiplos ponteiros soltos. Populado por TerminateContract().
type Termination struct {
	TerminatedAt              time.Time `json:"terminated_at" bson:"terminated_at"`
	Reason                    string    `json:"reason,omitempty" bson:"reason,omitempty"`
	EarlyTerminationFee       float64   `json:"early_termination_fee,omitempty" bson:"early_termination_fee,omitempty"`
	SecurityDepositReturn     float64   `json:"security_deposit_return,omitempty" bson:"security_deposit_return,omitempty"`
	SecurityDepositDeductions float64   `json:"security_deposit_deductions,omitempty" bson:"security_deposit_deductions,omitempty"`
	ExitInspectionID          string    `json:"exit_inspection_id,omitempty" bson:"exit_inspection_id,omitempty"`
	TriggeredBy               string    `json:"triggered_by,omitempty" bson:"triggered_by,omitempty"`
}

type Contract struct {
	ID          string      `json:"id" bson:"_id"`
	DisplayID   string      `json:"display_id,omitempty" bson:"display_id,omitempty"`
	DealID      string      `json:"deal_id,omitempty" bson:"deal_id,omitempty"`
	Status      Status      `json:"status" bson:"status"`
	Content     string      `json:"content,omitempty" bson:"content,omitempty"` // Could be HTML or S3 URL
	Signatories []Signatory `json:"signatories" bson:"signatories"`
	StartDate   time.Time   `json:"start_date" bson:"start_date"`
	EndDate     *time.Time  `json:"end_date,omitempty" bson:"end_date,omitempty"`

	// CRM contract fields
	Type          ContractType `json:"type,omitempty" bson:"type,omitempty"`
	PropertyID    string       `json:"property_id,omitempty" bson:"property_id,omitempty"`
	PropertyTitle string       `json:"property_title,omitempty" bson:"property_title,omitempty"`
	// CustomerID mirrors Deal.CustomerID (the lead/customer party UUID) so
	// the listing UI can fall back to a CRM lookup when Client (the cached
	// display name) drifts after a lead rename. Snapshot-derived from the
	// Deal at CreateContract time.
	CustomerID    string  `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Client        string  `json:"client,omitempty" bson:"client,omitempty"`
	Value         float64 `json:"value,omitempty" bson:"value,omitempty"`
	CommissionPct float64 `json:"commission_pct,omitempty" bson:"commission_pct,omitempty"`
	// Per-contract financial rules. When set, override the global delinquency
	// defaults (2% multa / 1% juros mês) so the admin UI can tune them per
	// contract. Stored on the contract itself rather than on a separate config
	// so they survive renewals/re-signatures of the same lease.
	LateFeePct         float64 `json:"late_fee_pct,omitempty" bson:"late_fee_pct,omitempty"`
	MonthlyInterestPct float64 `json:"monthly_interest_pct,omitempty" bson:"monthly_interest_pct,omitempty"`
	AdjustIndex        string  `json:"adjust_index,omitempty" bson:"adjust_index,omitempty"`
	AutoAdjust         bool    `json:"auto_adjust" bson:"auto_adjust"`
	NextAdjustment     string  `json:"next_adjustment,omitempty" bson:"next_adjustment,omitempty"`

	// SecurityDeposit (caução) — valor depositado pelo locatário no início do
	// contrato. Usado pelo TerminateContract para acertar devolução.
	SecurityDeposit float64 `json:"security_deposit,omitempty" bson:"security_deposit,omitempty"`

	// EntryInspectionID is set when the ENTRY vistoria created at
	// contract.signed time is concluded with a laudo. Mirrors
	// Termination.ExitInspectionID — both are the inspection ids that
	// flank the rental's operational window. Empty until the operator
	// closes the entry inspection in toca-ops.
	EntryInspectionID string `json:"entry_inspection_id,omitempty" bson:"entry_inspection_id,omitempty"`

	// Adjustments mantém o histórico de reajustes aplicados (manual ou auto).
	Adjustments []AdjustmentEntry `json:"adjustments,omitempty" bson:"adjustments,omitempty"`

	// Amendments lista aditivos emitidos a partir deste contrato. Apenas no
	// contrato original — o aditivo (novo contrato) carrega apenas
	// AmendsContractID apontando de volta. Ordem cronológica.
	Amendments []AmendmentEntry `json:"amendments,omitempty" bson:"amendments,omitempty"`

	// AmendsContractID, quando preenchido, indica que ESTE contrato é um
	// aditivo de outro — substitui o original ao ser assinado. O original
	// passa para EXPIRED e ganha SupersededAt no AmendmentEntry correspondente.
	AmendsContractID string `json:"amends_contract_id,omitempty" bson:"amends_contract_id,omitempty"`

	// Termination é preenchido pelo TerminateContract — nil enquanto ativo.
	Termination *Termination `json:"termination,omitempty" bson:"termination,omitempty"`

	// External signature integration (Clicksign today; provider-agnostic).
	// Populated when SendForSignature is called; cleared after a successful
	// cancel/refusal so the contract is editable again.
	ExternalEnvelopeID string     `json:"external_envelope_id,omitempty" bson:"external_envelope_id,omitempty"`
	SignatureProvider  string     `json:"signature_provider,omitempty" bson:"signature_provider,omitempty"` // e.g. "clicksign"
	SentForSignatureAt *time.Time `json:"sent_for_signature_at,omitempty" bson:"sent_for_signature_at,omitempty"`
	// SignatureLockedAt blocks edits while an envelope is in flight. Cleared
	// when the envelope is canceled or refused. SIGNED contracts stay locked
	// permanently — that's intentional, you can't edit a signed contract.
	SignatureLockedAt *time.Time `json:"signature_locked_at,omitempty" bson:"signature_locked_at,omitempty"`
	// SignedDocumentURL is the URL of the final signed PDF, populated by
	// MarkSignedExternally from the SignatureEnvelopeFinishedEvent.
	SignedDocumentURL string `json:"signed_document_url,omitempty" bson:"signed_document_url,omitempty"`

	// DraftRevisions preserves the document replaced by each regeneration.
	// Signature attempts remain linked to this same contract.
	DraftRevisions []DraftRevision `json:"draft_revisions,omitempty" bson:"draft_revisions,omitempty"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
	TenantID  string    `json:"tenant_id" bson:"tenant_id"`
}

type DraftRevision struct {
	Content     string      `json:"content"`
	Signatories []Signatory `json:"signatories"`
	ReplacedAt  time.Time   `json:"replaced_at"`
	ReplacedBy  string      `json:"replaced_by"`
}

type CreateContractRequest struct {
	DealID      string      `json:"deal_id"`
	Content     string      `json:"content"`
	Signatories []Signatory `json:"signatories"`
	StartDate   time.Time   `json:"start_date"`
	EndDate     *time.Time  `json:"end_date"`
	Status      Status      `json:"status"`

	// CRM fields
	Type          ContractType `json:"type"`
	PropertyID    string       `json:"property_id"`
	PropertyTitle string       `json:"property_title"`
	// CustomerID is accepted but optional — CreateContract overrides it from
	// the linked Deal (authoritative source) so a stale/missing value from
	// the caller cannot poison the snapshot.
	CustomerID         string  `json:"customer_id"`
	Client             string  `json:"client"`
	Value              float64 `json:"value"`
	CommissionPct      float64 `json:"commission_pct"`
	LateFeePct         float64 `json:"late_fee_pct"`
	MonthlyInterestPct float64 `json:"monthly_interest_pct"`
	AdjustIndex        string  `json:"adjust_index"`
	AutoAdjust         bool    `json:"auto_adjust"`
	NextAdjustment     string  `json:"next_adjustment"`
}

type Signatory struct {
	ID        string          `json:"id" bson:"_id"`
	Name      string          `json:"name" bson:"name"`
	Email     string          `json:"email" bson:"email"`
	Role      SignatoryRole   `json:"role" bson:"role"`                               // Buyer, Seller, Witness
	Status    SignatoryStatus `json:"status,omitempty" bson:"status,omitempty"`       // Mirrors envelope SignerStatus — written by signature_subscribers
	ViewedAt  *time.Time      `json:"viewed_at,omitempty" bson:"viewed_at,omitempty"` // First time the signer opened the signing link
	SignedAt  *time.Time      `json:"signed_at,omitempty" bson:"signed_at,omitempty"`
	Signature string          `json:"signature,omitempty" bson:"signature,omitempty"` // Digital signature hash

	// Optional contact + identification used by the signature provider
	// (Clicksign requires CPF for some auth methods like icp_brasil/biometric).
	PhoneNumber   string `json:"phone_number,omitempty" bson:"phone_number,omitempty"`
	Documentation string `json:"documentation,omitempty" bson:"documentation,omitempty"` // CPF/CNPJ
	Birthday      string `json:"birthday,omitempty" bson:"birthday,omitempty"`           // YYYY-MM-DD

	// External signature integration. Populated by EnvelopeService.CreateForContract
	// (via the Clicksign client) and updated by MarkSignerSignedExternally.
	ClicksignSignerKey string   `json:"clicksign_signer_key,omitempty" bson:"clicksign_signer_key,omitempty"`
	AuthMethods        []string `json:"auth_methods,omitempty" bson:"auth_methods,omitempty"` // selected by the operator
	EvidenceURL        string   `json:"evidence_url,omitempty" bson:"evidence_url,omitempty"`

	// Group is the default Clicksign signing-order group for this signer.
	// 0 / unset keeps the legacy behavior (every signer notified together
	// on envelope activation). Per-envelope overrides at create time take
	// precedence — this is just the contract-level default.
	Group int `json:"group,omitempty" bson:"group,omitempty"`
}

// ValidateSignatories checks if the provided signatories have valid roles for the given contract type.
func ValidateSignatories(contractType ContractType, signatories []Signatory) error {
	if contractType != TypeVenda && contractType != TypeLocacao {
		return fmt.Errorf("%w '%s'", ErrInvalidContractType, contractType)
	}

	for _, sig := range signatories {
		switch contractType {
		case TypeVenda:
			switch sig.Role {
			case RoleComprador, RoleVendedor, RoleAvalista, RoleTestemunha, RoleProprietario,
				RoleConjuge, RoleProcurador, RoleRepresentanteLegal, RoleTutor, RoleCurador:
				// Valid
			default:
				return fmt.Errorf("%w '%s' for sale contract", ErrInvalidSignatoryRole, sig.Role)
			}
		case TypeLocacao:
			switch sig.Role {
			case RoleInquilino, RoleProprietario, RoleFiador, RoleTestemunha, RoleLocador, RoleLocatario,
				RoleConjuge, RoleProcurador, RoleRepresentanteLegal, RoleTutor, RoleCurador:
				// Valid
			default:
				return fmt.Errorf("%w '%s' for rental contract", ErrInvalidSignatoryRole, sig.Role)
			}
		}
	}
	return nil
}

func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidSignatoryRole) ||
		errors.Is(err, ErrInvalidContractType) ||
		errors.Is(err, ErrInvalidContractPeriod)
}

func CalculateDefaultEndDate(startDate time.Time) *time.Time {
	d := startDate.AddDate(0, DefaultLeaseTermMonths, 0)
	return &d
}

// IsActive informa se o contrato está em estado operacional — isto é, se o
// fluxo de gestão (cobranças recorrentes, vistorias, despesas) deve estar
// disponível. DRAFT/PENDING/PARTIAL ainda não geram operação; EXPIRED/TERMINATED
// já cessaram. SIGNED e ACTIVE são equivalentes para fins de operação.
func (c *Contract) IsActive() bool {
	switch c.Status {
	case StatusSigned, StatusActive:
		return true
	}
	return false
}

// ApplyAdjustment aplica um reajuste ao valor do aluguel. Retorna o entry
// resultante (já apendado em c.Adjustments). Não persiste — caller é
// responsável por chamar repo.Update.
//
// mode deve ser "manual" ou "auto". triggeredBy identifica o ator (usuário
// ou "scheduler"). effectiveFrom default = now.
func (c *Contract) ApplyAdjustment(newValue, indexValue float64, indexName, mode, triggeredBy, note string, effectiveFrom time.Time) (AdjustmentEntry, error) {
	if !c.IsActive() {
		return AdjustmentEntry{}, errors.New("contract is not active; cannot apply adjustment")
	}
	if newValue <= 0 {
		return AdjustmentEntry{}, errors.New("new_value must be positive")
	}
	if effectiveFrom.IsZero() {
		effectiveFrom = time.Now()
	}
	entry := AdjustmentEntry{
		AppliedAt:     time.Now(),
		PreviousValue: c.Value,
		NewValue:      newValue,
		IndexName:     indexName,
		IndexValue:    indexValue,
		EffectiveFrom: effectiveFrom,
		TriggeredBy:   triggeredBy,
		Mode:          mode,
		Note:          note,
	}
	c.Adjustments = append(c.Adjustments, entry)
	c.Value = newValue
	c.UpdatedAt = time.Now()
	return entry, nil
}

// Terminate marca o contrato como TERMINATED. Não valida pré-requisitos
// como vistoria de saída — o caller (service) faz isso para poder consultar
// outros bounded contexts (toca-ops).
func (c *Contract) Terminate(reason, triggeredBy string, fee, depositReturn, depositDeductions float64, exitInspectionID string) error {
	if c.Status == StatusTerminated {
		return errors.New("contract already terminated")
	}
	if c.Status == StatusDraft || c.Status == StatusPending {
		return errors.New("contract cannot be terminated before signature")
	}
	now := time.Now()
	c.Status = StatusTerminated
	c.Termination = &Termination{
		TerminatedAt:              now,
		Reason:                    reason,
		EarlyTerminationFee:       fee,
		SecurityDepositReturn:     depositReturn,
		SecurityDepositDeductions: depositDeductions,
		ExitInspectionID:          exitInspectionID,
		TriggeredBy:               triggeredBy,
	}
	c.UpdatedAt = now
	return nil
}

func New(dealID string, content string, signatories []Signatory, startDate time.Time, endDate *time.Time, tenantID string, contractType ContractType) (*Contract, error) {
	now := time.Now()
	if signatories == nil {
		signatories = []Signatory{}
	}

	if contractType == TypeLocacao && endDate == nil {
		endDate = CalculateDefaultEndDate(startDate)
	}
	if endDate != nil && endDate.Before(startDate) {
		return nil, fmt.Errorf("%w: end_date must be >= start_date", ErrInvalidContractPeriod)
	}

	return &Contract{
		ID:          uuid.New().String(),
		DealID:      dealID,
		Status:      StatusDraft,
		Content:     content,
		Signatories: signatories,
		StartDate:   startDate,
		EndDate:     endDate,
		Type:        contractType,
		CreatedAt:   now,
		UpdatedAt:   now,
		TenantID:    tenantID,
	}, nil
}

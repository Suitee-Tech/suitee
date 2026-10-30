package deal

import (
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Status string

const (
	StatusNegotiation Status = "NEGOTIATION"
	StatusProposal    Status = "PROPOSAL"
	StatusAnalysis    Status = "ANALYSIS"
	StatusContract    Status = "CONTRACT"
	StatusConverted   Status = "CONVERTED"
	StatusCancelled   Status = "CANCELLED"
)

type Type string

const (
	TypeSale Type = "SALE"
	TypeRent Type = "RENT"
)

// BrokerParticipationRole defines the role of a broker in a deal
type BrokerParticipationRole string

const (
	BrokerRoleCaptador    BrokerParticipationRole = "captador" // property capture broker
	BrokerRoleCoCaptador  BrokerParticipationRole = "co_captador"
	BrokerRoleVisitador   BrokerParticipationRole = "visitador" // visit broker
	BrokerRoleAtendente   BrokerParticipationRole = "atendente"
	BrokerRoleFechamento  BrokerParticipationRole = "fechamento" // deal closing broker
	BrokerRoleImobiliaria BrokerParticipationRole = "imobiliaria"
)

// BrokerParticipation tracks a broker's role in a deal for history and commission purposes
type BrokerParticipation struct {
	BrokerID   string                  `json:"broker_id" bson:"broker_id"`
	Role       BrokerParticipationRole `json:"role" bson:"role"`
	Commission float64                 `json:"commission,omitempty" bson:"commission,omitempty"` // commission amount
	Percentage float64                 `json:"percentage,omitempty" bson:"percentage,omitempty"` // commission split %
}

type PartyKind string

const (
	PartyKindPerson  PartyKind = "person"
	PartyKindCompany PartyKind = "company"
)

type PartyRole string

const (
	PartyRoleTenant    PartyRole = "tenant"
	PartyRoleBuyer     PartyRole = "buyer"
	PartyRoleOwner     PartyRole = "owner"
	PartyRoleGuarantor PartyRole = "guarantor"
	PartyRoleWitness   PartyRole = "witness"
	PartyRoleSpouse    PartyRole = "spouse"
	PartyRoleAttorney  PartyRole = "attorney"
	PartyRoleLegalRep  PartyRole = "legal_representative"
	PartyRoleGuardian  PartyRole = "guardian"
	PartyRoleCurator   PartyRole = "curator"
)

type RepresentationType string

const (
	RepresentationTypeAttorney RepresentationType = "attorney"
	RepresentationTypeLegalRep RepresentationType = "legal_representative"
	RepresentationTypeGuardian RepresentationType = "guardian"
	RepresentationTypeCurator  RepresentationType = "curator"
)

type PartyRepresentation struct {
	Type                        RepresentationType `json:"type" bson:"type"`
	RepresentativePartyID       string             `json:"representative_party_id" bson:"representative_party_id"`
	RequiresPrincipalSignature  bool               `json:"requires_principal_signature" bson:"requires_principal_signature"`
	AllowRepresentativeProposal bool               `json:"allow_representative_proposal" bson:"allow_representative_proposal"`
}

// Origem de um campo da party (backing do indicador de origem no front).
const (
	ProvenanceOCR       = "ocr"       // identificado pela IA a partir do OCR
	ProvenanceAI        = "ai"        // auto-aplicado pela IA (alta confiança)
	ProvenanceManual    = "manual"    // digitado por um humano
	ProvenanceClient    = "client"    // preenchido pelo cliente no Portal
	ProvenanceValidated = "validated" // confirmado/corrigido pelo operador
)

// FieldProvenance registra a origem e a confiança de um campo identificado pela
// IA. É indexado por nome de campo da party (ex.: "name", "document", "cep") em
// Party.FieldProvenance e alimenta o ícone + tooltip + cor no front.
type FieldProvenance struct {
	Source      string     `json:"source" bson:"source"`
	Confidence  float64    `json:"confidence,omitempty" bson:"confidence,omitempty"`
	DocumentID  string     `json:"document_id,omitempty" bson:"document_id,omitempty"`
	ValidatedBy string     `json:"validated_by,omitempty" bson:"validated_by,omitempty"`
	ValidatedAt *time.Time `json:"validated_at,omitempty" bson:"validated_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at"`
}

type Party struct {
	ID           string            `json:"id" bson:"_id"`
	LeadID       string            `json:"lead_id,omitempty" bson:"lead_id,omitempty"`
	AccountID    string            `json:"account_id,omitempty" bson:"account_id,omitempty"`
	Name         string            `json:"name" bson:"name" pii:"name"`
	Email        string            `json:"email" bson:"email" pii:"email"`
	Phone        string            `json:"phone,omitempty" bson:"phone,omitempty" pii:"phone"`
	Document     string            `json:"document,omitempty" bson:"document,omitempty" pii:"document"`
	BirthDate    string            `json:"birth_date,omitempty" bson:"birth_date,omitempty" pii:"birth_date"`
	CustomFields map[string]string `json:"custom_fields,omitempty" bson:"custom_fields,omitempty"`
	// FieldProvenance registra, por campo, a origem do dado (IA/manual/validado) e
	// a confiança. Preenchido pela extração OCR+IA (auto-apply) e pela validação do
	// operador. Reside na JSONB do Deal — sem migration.
	FieldProvenance  map[string]FieldProvenance `json:"field_provenance,omitempty" bson:"field_provenance,omitempty"`
	Kind             PartyKind                  `json:"kind,omitempty" bson:"kind,omitempty"`
	Role             PartyRole                  `json:"role" bson:"role"`
	SpouseOfPartyID  string                     `json:"spouse_of_party_id,omitempty" bson:"spouse_of_party_id,omitempty"`
	NoSpouseDeclared bool                       `json:"no_spouse_declared,omitempty" bson:"no_spouse_declared,omitempty"`
	Representation   *PartyRepresentation       `json:"representation,omitempty" bson:"representation,omitempty"`
	SharePct         float64                    `json:"share_pct,omitempty" bson:"share_pct,omitempty"`
	IsPrimary        bool                       `json:"is_primary" bson:"is_primary"`
	// SignatureRequired marks whether this party's acceptance is mandatory for
	// the deal to advance past the proposal stage. nil means "not configured" —
	// the helper IsSignatureRequired() falls back to IsPrimary so deals created
	// before this field existed keep the previous behavior.
	SignatureRequired *bool         `json:"signature_required,omitempty" bson:"signature_required,omitempty"`
	SignStatus        SignerStatus  `json:"sign_status,omitempty" bson:"sign_status,omitempty"`
	SignedAt          *time.Time    `json:"signed_at,omitempty" bson:"signed_at,omitempty"`
	SignatureURL      string        `json:"signature_url,omitempty" bson:"signature_url,omitempty"`
	DueDiligence      *DueDiligence `json:"due_diligence,omitempty" bson:"due_diligence,omitempty"`
	CreatedAt         time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at" bson:"updated_at"`
}

// IsSignatureRequired returns whether this party must accept the proposal for
// the deal to advance. Defaults to IsPrimary when unset (legacy parties).
func (p *Party) IsSignatureRequired() bool {
	if p == nil {
		return false
	}
	if p.SignatureRequired != nil {
		return *p.SignatureRequired
	}
	return p.IsPrimary
}

type Deal struct {
	ID        string `json:"id" bson:"_id"`
	DisplayID string `json:"display_id,omitempty" bson:"display_id,omitempty"`
	Type      Type   `json:"type" bson:"type" validate:"required"`
	Status    Status `json:"status" bson:"status" validate:"required"`

	PropertyID string `json:"property_id" bson:"property_id" validate:"required"`
	// OwnerID is the lead.id of the property owner (lead with role='owner').
	// It does NOT change after lead→client conversion — `account_id` is a
	// projection layered onto the same lead, not a replacement for the id.
	OwnerID string `json:"owner_id" bson:"owner_id" validate:"required"`
	// CustomerID is ALWAYS the lead.id of the buyer or tenant. Stays constant
	// across the lead→client lifecycle. To get the portal account, look up
	// the lead's `account_id` field via toca-crm — don't reuse this column
	// for that purpose.
	CustomerID string `json:"customer_id" bson:"customer_id" validate:"required"`
	// LeadID is kept for explicit lead↔customer separation. Defaults to
	// CustomerID; in pre-conversion deals they're identical. Useful when a
	// deal is later transferred between leads (rare).
	LeadID   string `json:"lead_id,omitempty" bson:"lead_id,omitempty"`
	BrokerID string `json:"broker_id" bson:"broker_id" validate:"required"` // Primary responsible broker
	// RecommendedPropertyID é o imóvel que o corretor recomenda explicitamente
	// ao cliente (exibido no portal como "Sugestão do seu corretor").
	RecommendedPropertyID string `json:"recommended_property_id,omitempty" bson:"recommended_property_id,omitempty"`

	Value      float64 `json:"value" bson:"value" validate:"gt=0"`
	Commission float64 `json:"commission" bson:"commission"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`

	TenantID string `json:"tenant_id" bson:"tenant_id" validate:"required"`

	// BrokerParticipations tracks all brokers involved (captador, visitador, fechamento)
	// for history and commission split purposes
	BrokerParticipations []BrokerParticipation `json:"broker_participations,omitempty" bson:"broker_participations,omitempty"`

	// IdempotencyKey identifica a INTENÇÃO de criação: o mesmo formulário
	// reenviado (duplo clique, retry de rede) usa a mesma chave e o backend
	// devolve o negócio já criado em vez de duplicar. Vazio em negócios
	// criados por clientes sem a chave.
	IdempotencyKey string `json:"idempotency_key,omitempty" bson:"idempotency_key,omitempty"`

	Documents    []Document      `json:"documents,omitempty" bson:"documents,omitempty"`
	History      []History       `json:"history,omitempty" bson:"history,omitempty"`
	Proposals    []Proposal      `json:"proposals,omitempty" bson:"proposals,omitempty"`
	DueDiligence *DueDiligence   `json:"due_diligence,omitempty" bson:"due_diligence,omitempty"`
	Parties      []Party         `json:"parties,omitempty" bson:"parties,omitempty"`
	Signers      []Signer        `json:"signers,omitempty" bson:"signers,omitempty"`
	Checklist    []ChecklistItem `json:"checklist,omitempty" bson:"checklist,omitempty"`
	Favorites    []Favorite      `json:"favorites,omitempty" bson:"favorites,omitempty"`

	// ChecklistStats is a derived view of `Checklist` populated by
	// callers (see `PopulateChecklistStats`) before serialization. Tagged
	// `bson:"-"` so the repository never persists it — the source of
	// truth is the items themselves. Surfaced on GetByID so the Esteira
	// Documentation tab can render a "X de Y obrigatórios aprovados"
	// badge without re-computing client-side.
	ChecklistStats *ChecklistStats `json:"checklist_stats,omitempty" bson:"-"`

	// CoBrokering is true when this deal was opened against a resource
	// (property or lead) SHARED by a partner company — a cross-company
	// co-brokering deal. Set once at creation; persisted in the deal JSONB.
	PartnershipSnapshot *access.Snapshot `json:"-"`
	CoBrokering         bool             `json:"co_brokering,omitempty" bson:"co_brokering,omitempty"`
	// SourceCompanyID is the tenant that OWNS the shared resource the deal
	// was started from. Only set when CoBrokering is true. Lets the Esteira
	// and the financial split identify the owner company.
	SourceCompanyID string `json:"source_company_id,omitempty" bson:"source_company_id,omitempty"`
}

type Favorite struct {
	PropertyID string    `json:"property_id" bson:"property_id"`
	AccountID  string    `json:"account_id" bson:"account_id"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}

type DocumentType string

const (
	DocumentTypeIdentity       DocumentType = "IDENTITY"        // RG, CPF
	DocumentTypeIncome         DocumentType = "INCOME"          // Comprovante de Renda
	DocumentTypeResidence      DocumentType = "RESIDENCE"       // Comprovante de Residência
	DocumentTypeProperty       DocumentType = "PROPERTY"        // Matrícula, IPTU
	DocumentTypeContract       DocumentType = "CONTRACT"        // Minuta do Contrato
	DocumentTypeSignedContract DocumentType = "SIGNED_CONTRACT" // Contrato Assinado
	DocumentTypeOther          DocumentType = "OTHER"
)

type DocumentStatus string

const (
	DocumentStatusPending  DocumentStatus = "PENDING"
	DocumentStatusUploaded DocumentStatus = "UPLOADED"
	DocumentStatusApproved DocumentStatus = "APPROVED"
	DocumentStatusRejected DocumentStatus = "REJECTED"
)

type ProposalStatus string

const (
	ProposalStatusDraft    ProposalStatus = "DRAFT"
	ProposalStatusSent     ProposalStatus = "SENT"
	ProposalStatusAccepted ProposalStatus = "ACCEPTED"
	ProposalStatusRejected ProposalStatus = "REJECTED"
	ProposalStatusCounter  ProposalStatus = "COUNTER_OFFER"
)

type ProposalAcceptance struct {
	PartyID    string    `json:"party_id" bson:"party_id"`
	Role       PartyRole `json:"role" bson:"role"`
	AcceptedAt time.Time `json:"accepted_at" bson:"accepted_at"`
}

type Proposal struct {
	ID            string               `json:"id" bson:"_id"`
	Value         float64              `json:"value" bson:"value"`
	CounterValue  *float64             `json:"counter_value,omitempty" bson:"counter_value,omitempty"`
	PaymentMethod string               `json:"payment_method" bson:"payment_method"`
	Installments  int                  `json:"installments,omitempty" bson:"installments,omitempty"`
	DownPayment   float64              `json:"down_payment,omitempty" bson:"down_payment,omitempty"`
	Status        ProposalStatus       `json:"status" bson:"status"`
	CreatedAt     time.Time            `json:"created_at" bson:"created_at"`
	CreatedBy     string               `json:"created_by" bson:"created_by"`
	Notes         string               `json:"notes,omitempty" bson:"notes,omitempty"`
	AcceptedBy    []ProposalAcceptance `json:"accepted_by,omitempty" bson:"accepted_by,omitempty"`
	// FollowUps conta os lembretes temporais já disparados para esta proposta
	// (plano beta §F6). É o "grupo" da régua: cada estágio dispara UMA vez e
	// só depois do intervalo da cadência, nunca a cada varredura.
	FollowUps int `json:"follow_ups,omitempty" bson:"follow_ups,omitempty"`
	// FollowUpLastAt é quando o último lembrete saiu — âncora da cadência
	// seguinte. Vazio significa que nenhum lembrete saiu ainda e a âncora é
	// o envio da proposta.
	FollowUpLastAt *time.Time `json:"follow_up_last_at,omitempty" bson:"follow_up_last_at,omitempty"`
}

type DueDiligence struct {
	ScoreSerasa   *int                                   `json:"score_serasa,omitempty" bson:"score_serasa,omitempty"`
	CNDStatus     string                                 `json:"cnd_status" bson:"cnd_status"` // "PENDING", "CLEAN", "RESTRICTED"
	LegalOpinion  string                                 `json:"legal_opinion,omitempty" bson:"legal_opinion,omitempty"`
	MonthlyIncome *int                                   `json:"monthly_income,omitempty" bson:"monthly_income,omitempty"`
	Criteria      map[string]DueDiligenceCriterionReview `json:"criteria,omitempty" bson:"criteria,omitempty"`
	Status        string                                 `json:"status" bson:"status"` // "PENDING", "APPROVED", "REJECTED"
	UpdatedAt     time.Time                              `json:"updated_at" bson:"updated_at"`
}

// DueDiligenceCriterionReview é a decisão por critério (Score, CND, Parecer,
// Renda). O status da parte é derivado dos quatro critérios: qualquer
// reprovação reprova; todos aprovados/dispensados aprovam; o resto fica
// pendente. O status manual continua existindo como fallback.
type DueDiligenceCriterionReview struct {
	Status     string     `json:"status" bson:"status"`
	ReviewedBy string     `json:"reviewed_by,omitempty" bson:"reviewed_by,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty" bson:"reviewed_at,omitempty"`
	Notes      string     `json:"notes,omitempty" bson:"notes,omitempty"`
}

// SignatoryRole defines the role of a signer in a deal/contract
type SignatoryRole string

const (
	SignatoryRoleInquilino    SignatoryRole = "inquilino"
	SignatoryRoleProprietario SignatoryRole = "proprietario"
	SignatoryRoleFiador       SignatoryRole = "fiador"
	SignatoryRoleLocador      SignatoryRole = "locador"
	SignatoryRoleLocatario    SignatoryRole = "locatario"
	SignatoryRoleComprador    SignatoryRole = "comprador"
	SignatoryRoleVendedor     SignatoryRole = "vendedor"
	SignatoryRoleAvalista     SignatoryRole = "avalista"
	SignatoryRoleTestemunha   SignatoryRole = "testemunha"
	SignatoryRoleConjuge      SignatoryRole = "conjuge"
	SignatoryRoleProcurador   SignatoryRole = "procurador"
	SignatoryRoleRepLegal     SignatoryRole = "representante_legal"
	SignatoryRoleTutor        SignatoryRole = "tutor"
	SignatoryRoleCurador      SignatoryRole = "curador"
)

type SignerStatus string

const (
	SignerStatusPending  SignerStatus = "PENDING"
	SignerStatusSigned   SignerStatus = "SIGNED"
	SignerStatusRejected SignerStatus = "REJECTED"
)

type Signer struct {
	ID           string        `json:"id" bson:"_id"`
	Name         string        `json:"name" bson:"name" pii:"name"`
	Email        string        `json:"email" bson:"email" pii:"email"`
	Role         SignatoryRole `json:"role" bson:"role"`
	Status       SignerStatus  `json:"status" bson:"status"`
	SignedAt     *time.Time    `json:"signed_at,omitempty" bson:"signed_at,omitempty"`
	SignatureURL string        `json:"signature_url,omitempty" bson:"signature_url,omitempty"`
	AccountID    string        `json:"account_id,omitempty" bson:"account_id,omitempty"`
	// IsRequired marks whether this signer must sign for the contract to be
	// considered complete. nil means "not configured" — defaults to true so
	// existing deals don't suddenly bypass signatures.
	IsRequired *bool `json:"is_required,omitempty" bson:"is_required,omitempty"`

	// ── Forensic signing metadata (Phase 3/S8) ──
	//
	// These fields capture what Clicksign records at signature time so a
	// legal dispute can be answered from toca's own state without
	// round-tripping to Clicksign's API. Populated by the toca-signature
	// envelope_finished/signer_signed subscribers when the webhook
	// payload carries them; empty for signers where Clicksign didn't
	// surface the data (e.g. simple email-token signers don't have a
	// certificate serial).
	//
	// Schema-first: defining the fields now lets the toca-signature
	// subscriber start populating them without a follow-up migration.
	// The Postgres repository stores these inside the Signers JSONB
	// column already — no schema change required.

	// SignedIP is the client IP recorded by Clicksign for the signature
	// event. Useful for fraud investigations ("the signer claims they
	// never signed but our log shows they did from X.Y.Z.W at HH:MM").
	SignedIP string `json:"signed_ip,omitempty" bson:"signed_ip,omitempty"`
	// AuthMethodUsed mirrors clicksign.AuthMethod (email | whatsapp |
	// sms | selfie | icp_brasil | selfie+document). Surfaces in the
	// audit trail so operators can answer "how did this signer
	// authenticate?" without joining against the envelope.
	AuthMethodUsed string `json:"auth_method_used,omitempty" bson:"auth_method_used,omitempty"`
	// CertificateSerial is the X.509 serial of the ICP-Brasil
	// certificate used to sign (when AuthMethodUsed=icp_brasil). Empty
	// for non-ICP signature methods. Stored as the canonical
	// uppercase-hex string Clicksign reports.
	CertificateSerial string `json:"certificate_serial,omitempty" bson:"certificate_serial,omitempty"`
	// WidgetEventLogURL is Clicksign's link to the per-signer event
	// log (page-views, downloads, signature acceptance). Operators
	// drop this into legal hand-offs as the canonical "audit trail of
	// the act of signing" for a single signer.
	WidgetEventLogURL string `json:"widget_event_log_url,omitempty" bson:"widget_event_log_url,omitempty"`
	// AuthEvidenceURL is Clicksign's link to the proof-of-authentication
	// document (e.g. selfie+document scan, ICP-Brasil certificate
	// snapshot). Stored separately from WidgetEventLogURL because legal
	// usually wants both attached side-by-side.
	AuthEvidenceURL string `json:"auth_evidence_url,omitempty" bson:"auth_evidence_url,omitempty"`
}

// IsSignerRequired returns whether this signer must sign before the deal can
// be marked as concluded. Defaults to true when unset (legacy signers).
func (s *Signer) IsSignerRequired() bool {
	if s == nil {
		return false
	}
	if s.IsRequired != nil {
		return *s.IsRequired
	}
	return true
}

type Document struct {
	ID          string         `json:"id" bson:"_id"`
	DealID      string         `json:"deal_id" bson:"deal_id"`
	Type        DocumentType   `json:"type" bson:"type"`
	Description string         `json:"description" bson:"description"`
	URL         string         `json:"url,omitempty" bson:"url,omitempty"`
	Status      DocumentStatus `json:"status" bson:"status"`
	UploadedBy  string         `json:"uploaded_by,omitempty" bson:"uploaded_by,omitempty"`
	UploadedAt  *time.Time     `json:"uploaded_at,omitempty" bson:"uploaded_at,omitempty"`
}

type History struct {
	ID        string    `json:"id" bson:"_id"`
	DealID    string    `json:"deal_id" bson:"deal_id"`
	Status    Status    `json:"status" bson:"status"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Notes     string    `json:"notes,omitempty" bson:"notes,omitempty"`
}

func New(dealType Type, propertyID, ownerID, customerID, brokerID string, value float64, tenantID string) (*Deal, error) {
	now := time.Now()
	d := &Deal{
		ID:         uuid.New().String(),
		Type:       dealType,
		Status:     StatusNegotiation,
		PropertyID: propertyID,
		OwnerID:    ownerID,
		CustomerID: customerID,
		LeadID:     customerID, // lead and customer start as the same entity
		BrokerID:   brokerID,
		Value:      value,
		CreatedAt:  now,
		UpdatedAt:  now,
		TenantID:   tenantID,
	}

	if err := validate.Struct(d); err != nil {
		return nil, err
	}

	// Add initial history
	d.AddHistory(StatusNegotiation, brokerID, "Deal created")

	return d, nil
}

func (d *Deal) RemoveSigner(signerID string) {
	var updated []Signer
	for _, s := range d.Signers {
		if s.ID != signerID {
			updated = append(updated, s)
		}
	}
	d.Signers = updated
	_ = d.removePartyByID(signerID, false)
	d.UpdatedAt = time.Now()
}

func (d *Deal) AddParty(p Party) error {
	if err := d.validatePartyRole(p.Role); err != nil {
		return err
	}
	now := time.Now()
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.SignStatus == "" {
		p.SignStatus = SignerStatusPending
	}
	if idx := d.findPartyIndexByID(p.ID); idx != -1 {
		p.CreatedAt = d.Parties[idx].CreatedAt
		d.Parties[idx] = p
		d.ensureSinglePrimaryForRole(p.Role, p.ID, p.IsPrimary)
		d.UpdatedAt = now
		return nil
	}
	d.Parties = append(d.Parties, p)
	d.ensureSinglePrimaryForRole(p.Role, p.ID, p.IsPrimary)
	d.UpdatedAt = now
	return nil
}

func (d *Deal) RemoveParty(partyID string) error {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return nil
	}
	role := d.Parties[idx].Role
	enforcePrimary := role == PartyRoleTenant || role == PartyRoleBuyer
	return d.removePartyByID(partyID, enforcePrimary)
}

func (d *Deal) SetPrimaryParty(partyID string) error {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return fmt.Errorf("party not found")
	}
	role := d.Parties[idx].Role
	now := time.Now()
	for i := range d.Parties {
		if d.Parties[i].Role != role {
			continue
		}
		d.Parties[i].IsPrimary = d.Parties[i].ID == partyID
		d.Parties[i].UpdatedAt = now
	}
	d.UpdatedAt = now
	return nil
}

func (d *Deal) UpdatePartyRole(partyID string, role PartyRole) error {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return fmt.Errorf("party not found")
	}
	if err := d.validatePartyRole(role); err != nil {
		return err
	}

	oldRole := d.Parties[idx].Role
	wasPrimary := d.Parties[idx].IsPrimary
	now := time.Now()

	d.Parties[idx].Role = role
	d.Parties[idx].UpdatedAt = now

	d.ensureSinglePrimaryForRole(role, partyID, wasPrimary)
	if wasPrimary && d.PrimaryByRole(oldRole) == nil {
		for i := range d.Parties {
			if d.Parties[i].Role == oldRole {
				d.Parties[i].IsPrimary = true
				d.Parties[i].UpdatedAt = now
				break
			}
		}
	}

	d.UpdatedAt = now
	return nil
}

func (d *Deal) PartiesByRole(role PartyRole) []Party {
	out := make([]Party, 0)
	for _, p := range d.Parties {
		if p.Role == role {
			out = append(out, p)
		}
	}
	return out
}

func (d *Deal) PrimaryByRole(role PartyRole) *Party {
	for i := range d.Parties {
		if d.Parties[i].Role == role && d.Parties[i].IsPrimary {
			return &d.Parties[i]
		}
	}
	return nil
}

func (d *Deal) EnsurePartiesFromLegacy() {
	if len(d.Parties) > 0 {
		d.SyncLegacyIDsFromParties()
		return
	}

	now := time.Now()

	if d.OwnerID != "" {
		_ = d.AddParty(Party{
			ID:        uuid.New().String(),
			LeadID:    d.OwnerID,
			Role:      PartyRoleOwner,
			IsPrimary: true,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	customerRole := PartyRoleTenant
	if d.Type == TypeSale {
		customerRole = PartyRoleBuyer
	}
	customerLeadID := d.LeadID
	if customerLeadID == "" {
		customerLeadID = d.CustomerID
	}
	if d.CustomerID != "" || customerLeadID != "" {
		_ = d.AddParty(Party{
			ID:           uuid.New().String(),
			LeadID:       customerLeadID,
			Role:         customerRole,
			IsPrimary:    true,
			DueDiligence: d.DueDiligence,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}

	for _, s := range d.Signers {
		pr, ok := partyRoleFromSignatoryRole(s.Role)
		if !ok {
			continue
		}
		p := Party{
			ID:           s.ID,
			AccountID:    s.AccountID,
			Name:         s.Name,
			Email:        s.Email,
			Role:         pr,
			IsPrimary:    false,
			SignStatus:   s.Status,
			SignedAt:     s.SignedAt,
			SignatureURL: s.SignatureURL,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = d.AddParty(p)
	}

	d.SyncLegacyIDsFromParties()
}

func (d *Deal) SyncLegacyIDsFromParties() {
	if len(d.Parties) == 0 {
		return
	}

	if p := d.PrimaryByRole(PartyRoleOwner); p != nil && p.LeadID != "" {
		d.OwnerID = p.LeadID
	}

	customerRole := PartyRoleTenant
	if d.Type == TypeSale {
		customerRole = PartyRoleBuyer
	}
	if p := d.PrimaryByRole(customerRole); p != nil && p.LeadID != "" {
		d.CustomerID = p.LeadID
		d.LeadID = p.LeadID
	}
}

// AddBrokerParticipation adds a broker with a specific role to the deal (idempotent by broker+role).
func (d *Deal) AddBrokerParticipation(brokerID string, role BrokerParticipationRole, commission, percentage float64) {
	for _, bp := range d.BrokerParticipations {
		if bp.BrokerID == brokerID && bp.Role == role {
			return // already registered
		}
	}
	d.BrokerParticipations = append(d.BrokerParticipations, BrokerParticipation{
		BrokerID:   brokerID,
		Role:       role,
		Commission: commission,
		Percentage: percentage,
	})
	d.UpdatedAt = time.Now()
}

// IsBrokerSplitLocked reports whether the broker_participations array can
// still be edited. Issue #420 — locked once the deal reaches CONVERTED
// (contract.signed already consumed the previous split via the financial
// pipeline; ver toca-financial/internal/service/event_handler.go #414).
// CANCELLED also locks for safety — no business value in editing a deal
// that is no longer in play.
//
// Kept on the domain (not the service) so handlers and the frontend's
// future "Locked" badge agree on the same predicate via the API response.
func (d *Deal) IsBrokerSplitLocked() bool {
	if d == nil {
		return true
	}
	return d.Status == StatusConverted || d.Status == StatusCancelled
}

func (d *Deal) AddDocument(docType DocumentType, description string) {
	doc := Document{
		ID:          uuid.New().String(),
		DealID:      d.ID,
		Type:        docType,
		Description: description,
		Status:      DocumentStatusPending,
	}
	d.Documents = append(d.Documents, doc)
}

func (d *Deal) UpdateDocument(docID, url, userID string) {
	for i, doc := range d.Documents {
		if doc.ID == docID {
			now := time.Now()
			d.Documents[i].URL = url
			d.Documents[i].UploadedBy = userID
			d.Documents[i].UploadedAt = &now
			d.Documents[i].Status = DocumentStatusUploaded
			return
		}
	}
}

func (d *Deal) AddHistory(status Status, userID, notes string) {
	h := History{
		ID:        uuid.New().String(),
		DealID:    d.ID,
		Status:    status,
		Timestamp: time.Now(),
		UserID:    userID,
		Notes:     notes,
	}
	d.History = append(d.History, h)
}

func (d *Deal) AddProposal(value float64, paymentMethod string, installments int, downPayment float64, createdBy, notes string) {
	p := Proposal{
		ID:            uuid.New().String(),
		Value:         value,
		PaymentMethod: paymentMethod,
		Installments:  installments,
		DownPayment:   downPayment,
		Status:        ProposalStatusSent,
		CreatedAt:     time.Now(),
		CreatedBy:     createdBy,
		Notes:         notes,
	}
	d.Proposals = append(d.Proposals, p)
}

func (d *Deal) UpdateDueDiligence(score *int, cndStatus, legalOpinion string, monthlyIncome *int, status string) {
	if d.DueDiligence == nil {
		d.DueDiligence = &DueDiligence{}
	}
	d.DueDiligence.UpdatedAt = time.Now()
	if score != nil {
		d.DueDiligence.ScoreSerasa = score
	}
	if cndStatus != "" {
		d.DueDiligence.CNDStatus = cndStatus
	}
	if legalOpinion != "" {
		d.DueDiligence.LegalOpinion = legalOpinion
	}
	if monthlyIncome != nil {
		d.DueDiligence.MonthlyIncome = monthlyIncome
	}
	if status != "" {
		d.DueDiligence.Status = status
	}
}

func (d *Deal) UpdatePartyDueDiligence(partyID string, score *int, cndStatus, legalOpinion string, monthlyIncome *int, status string) (string, string, error) {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return "", "", fmt.Errorf("party not found")
	}

	if d.Parties[idx].DueDiligence == nil {
		d.Parties[idx].DueDiligence = &DueDiligence{}
	}
	oldStatus := d.Parties[idx].DueDiligence.Status

	d.Parties[idx].DueDiligence.UpdatedAt = time.Now()
	if score != nil {
		d.Parties[idx].DueDiligence.ScoreSerasa = score
	}
	if cndStatus != "" {
		d.Parties[idx].DueDiligence.CNDStatus = cndStatus
	}
	if legalOpinion != "" {
		d.Parties[idx].DueDiligence.LegalOpinion = legalOpinion
	}
	if monthlyIncome != nil {
		d.Parties[idx].DueDiligence.MonthlyIncome = monthlyIncome
	}
	if status != "" {
		d.Parties[idx].DueDiligence.Status = status
	}
	d.Parties[idx].UpdatedAt = time.Now()
	return oldStatus, d.Parties[idx].DueDiligence.Status, nil
}

// UpdatePartyDueDiligenceCriterion registra a decisão de um critério e
// deriva o status da parte (REJECTED > PENDING > APPROVED). A decisão fica
// com revisor e data para auditoria.
func (d *Deal) UpdatePartyDueDiligenceCriterion(partyID, criterion, status, reviewer, notes string) (string, string, error) {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return "", "", fmt.Errorf("party not found")
	}
	if d.Parties[idx].DueDiligence == nil {
		d.Parties[idx].DueDiligence = &DueDiligence{}
	}
	dd := d.Parties[idx].DueDiligence
	if dd.Criteria == nil {
		dd.Criteria = map[string]DueDiligenceCriterionReview{}
	}
	oldStatus := dd.Status
	now := time.Now()
	dd.Criteria[criterion] = DueDiligenceCriterionReview{
		Status:     strings.ToLower(strings.TrimSpace(status)),
		ReviewedBy: reviewer,
		ReviewedAt: &now,
		Notes:      notes,
	}
	dd.Status = deriveDueDiligenceStatus(dd.Criteria)
	dd.UpdatedAt = now
	d.Parties[idx].UpdatedAt = now
	return oldStatus, dd.Status, nil
}

func deriveDueDiligenceStatus(criteria map[string]DueDiligenceCriterionReview) string {
	pending := false
	for _, key := range []string{"score", "cnd", "legal_opinion", "income"} {
		review, ok := criteria[key]
		if !ok {
			pending = true
			continue
		}
		switch strings.ToLower(strings.TrimSpace(review.Status)) {
		case "rejected":
			return "REJECTED"
		case "approved", "waived":
			// completo
		default:
			pending = true
		}
	}
	if pending {
		return "PENDING"
	}
	return "APPROVED"
}

func (d *Deal) AggregatedDueDiligence() *DueDiligence {
	if len(d.Parties) == 0 {
		return nil
	}
	any := false
	derivedStatus := "PENDING"
	hasPending := false

	for i := range d.Parties {
		dd := d.Parties[i].DueDiligence
		if dd == nil {
			continue
		}
		any = true
		st := strings.ToUpper(strings.TrimSpace(dd.Status))
		if st == "" || st == "PENDING" {
			hasPending = true
			continue
		}
		if st == "REJECTED" {
			derivedStatus = "REJECTED"
			break
		}
	}

	if !any {
		return nil
	}
	if derivedStatus != "REJECTED" {
		if hasPending {
			derivedStatus = "PENDING"
		} else {
			derivedStatus = "APPROVED"
		}
	}

	return &DueDiligence{
		Status:    derivedStatus,
		UpdatedAt: time.Now(),
	}
}

func (d *Deal) AddSigner(name, email string, role SignatoryRole) error {
	pr, ok := partyRoleFromSignatoryRole(role)
	if !ok {
		return fmt.Errorf("invalid signatory role '%s'", role)
	}
	if err := d.validatePartyRole(pr); err != nil {
		return err
	}

	// Normalize + validate the full name so downstream (Contract.Signatories
	// → Clicksign POST /signers) doesn't fail with the v3 API's opaque
	// "name não está em um formato válido" 400. Operator sees the precise
	// reason at the moment of input.
	name = validation.SanitizePersonName(name)
	if err := validation.ValidateFullPersonName(name, "Nome do signatário"); err != nil {
		return err
	}

	now := time.Now()
	partyID := ""
	if idx := d.findPartyIndexByRoleAndEmail(pr, email); idx != -1 {
		partyID = d.Parties[idx].ID
		if name != "" {
			d.Parties[idx].Name = name
		}
		if email != "" {
			d.Parties[idx].Email = email
		}
		if d.Parties[idx].SignStatus == "" {
			d.Parties[idx].SignStatus = SignerStatusPending
		}
		d.Parties[idx].UpdatedAt = now
	} else {
		partyID = uuid.New().String()
		_ = d.AddParty(Party{
			ID:         partyID,
			Name:       name,
			Email:      email,
			Role:       pr,
			SignStatus: SignerStatusPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}

	s := Signer{
		ID:     partyID,
		Name:   name,
		Email:  email,
		Role:   role,
		Status: SignerStatusPending,
	}
	d.Signers = append(d.Signers, s)
	d.UpdatedAt = now
	return nil
}

func (d *Deal) UpdateSignerStatus(signerID string, status SignerStatus, signatureURL string) {
	d.UpdateSignerStatusByIDOrEmail(signerID, "", status, signatureURL)
}

// UpdateSignerStatusByIDOrEmail mirrors a signer status onto Signers[] and
// Parties[]. Tries ID first, falls back to (role-aware) email match when the
// caller can provide it. The email fallback is what makes this resilient to
// the production drift we hit when a Contract.Signatory.ID and the matching
// Deal.Signer.ID get out of sync (e.g. a contract created before a deal
// signer was re-added, or a Reconcile that re-emitted events with the
// envelope's SignerID instead of the contract signatory's). Without this
// fallback the deal stays "Pendente" forever and only Reconciliar recovers.
func (d *Deal) UpdateSignerStatusByIDOrEmail(signerID, signerEmail string, status SignerStatus, signatureURL string) {
	var role SignatoryRole
	resolvedEmail := signerEmail
	signerMatched := false
	for i, s := range d.Signers {
		if s.ID == signerID || (signerEmail != "" && strings.EqualFold(s.Email, signerEmail)) {
			role = s.Role
			if resolvedEmail == "" {
				resolvedEmail = s.Email
			}
			d.Signers[i].Status = status
			if status == SignerStatusSigned {
				now := time.Now()
				d.Signers[i].SignedAt = &now
				d.Signers[i].SignatureURL = signatureURL
			}
			signerMatched = true
			break
		}
	}
	if idx := d.findPartyIndexByID(signerID); idx != -1 {
		d.Parties[idx].SignStatus = status
		d.Parties[idx].SignatureURL = signatureURL
		if status == SignerStatusSigned {
			now := time.Now()
			d.Parties[idx].SignedAt = &now
		}
		d.Parties[idx].UpdatedAt = time.Now()
		d.UpdatedAt = time.Now()
		return
	}
	pr, ok := partyRoleFromSignatoryRole(role)
	if ok && resolvedEmail != "" {
		if idx := d.findPartyIndexByRoleAndEmail(pr, resolvedEmail); idx != -1 {
			d.Parties[idx].SignStatus = status
			d.Parties[idx].SignatureURL = signatureURL
			if status == SignerStatusSigned {
				now := time.Now()
				d.Parties[idx].SignedAt = &now
			}
			d.Parties[idx].UpdatedAt = time.Now()
			d.UpdatedAt = time.Now()
			return
		}
	}
	// Last-chance party match by email alone — handles the case where Signer
	// wasn't on the deal at all (envelope was edited on Clicksign directly).
	if resolvedEmail != "" {
		for i := range d.Parties {
			if strings.EqualFold(d.Parties[i].Email, resolvedEmail) {
				d.Parties[i].SignStatus = status
				d.Parties[i].SignatureURL = signatureURL
				if status == SignerStatusSigned {
					now := time.Now()
					d.Parties[i].SignedAt = &now
				}
				d.Parties[i].UpdatedAt = time.Now()
				d.UpdatedAt = time.Now()
				return
			}
		}
	}
	_ = signerMatched
}

// ── Checklist (document checklist for esteira digital) ──

type ChecklistItemStatus string

const (
	ChecklistPending  ChecklistItemStatus = "PENDING"
	ChecklistUploaded ChecklistItemStatus = "UPLOADED"
	ChecklistApproved ChecklistItemStatus = "APPROVED"
	ChecklistRejected ChecklistItemStatus = "REJECTED"
)

// ChecklistItemRequestMode define como o cliente cumpre a solicitação:
//   - "upload" (default): o cliente anexa um arquivo;
//   - "form": o cliente apenas preenche o formulário (CPF, telefone, etc.)
//     sem anexo — a solicitação é cumprida com os campos preenchidos.
type ChecklistItemRequestMode string

const (
	RequestModeUpload ChecklistItemRequestMode = "upload"
	RequestModeForm   ChecklistItemRequestMode = "form"
)

// NormalizeRequestMode retorna o modo canônico: vazio → "upload".
func NormalizeRequestMode(mode string) ChecklistItemRequestMode {
	switch ChecklistItemRequestMode(mode) {
	case RequestModeForm:
		return RequestModeForm
	default:
		return RequestModeUpload
	}
}

type ChecklistItemCategory string

const (
	CategoryPessoal    ChecklistItemCategory = "pessoal"
	CategoryFinanceiro ChecklistItemCategory = "financeiro"
	CategoryImovel     ChecklistItemCategory = "imovel"
	CategoryJuridico   ChecklistItemCategory = "juridico"
	CategoryOutros     ChecklistItemCategory = "outros"
)

type ChecklistItem struct {
	ID          string                `json:"id" bson:"_id"`
	Name        string                `json:"name" bson:"name"`
	Category    ChecklistItemCategory `json:"category" bson:"category"`
	PartyID     string                `json:"party_id,omitempty" bson:"party_id,omitempty"`
	Status      ChecklistItemStatus   `json:"status" bson:"status"`
	Required    bool                  `json:"required" bson:"required"`
	Description string                `json:"description,omitempty" bson:"description,omitempty"`
	// RequestMode define se o cliente anexa arquivo ("upload") ou apenas
	// preenche o formulário ("form"). Vazio é normalizado para "upload".
	RequestMode     ChecklistItemRequestMode  `json:"request_mode,omitempty" bson:"request_mode,omitempty"`
	RequestedFields []ChecklistRequestedField `json:"requested_fields,omitempty" bson:"requested_fields,omitempty"`
	RejectionReason string                    `json:"rejection_reason,omitempty" bson:"rejection_reason,omitempty"`
	FileURL         string                    `json:"file_url,omitempty" bson:"file_url,omitempty"`
	StorageKey      string                    `json:"storage_key,omitempty" bson:"storage_key,omitempty"`
	FileName        string                    `json:"file_name,omitempty" bson:"file_name,omitempty"`
	ContentType     string                    `json:"content_type,omitempty" bson:"content_type,omitempty"`
	FileSize        int64                     `json:"file_size,omitempty" bson:"file_size,omitempty"`
	UploadedBy      string                    `json:"uploaded_by,omitempty" bson:"uploaded_by,omitempty"`
	UploadedAt      *time.Time                `json:"uploaded_at,omitempty" bson:"uploaded_at,omitempty"`
	ReviewedAt      *time.Time                `json:"reviewed_at,omitempty" bson:"reviewed_at,omitempty"`
	Solicited       bool                      `json:"solicited" bson:"solicited"`
	SolicitedAt     *time.Time                `json:"solicited_at,omitempty" bson:"solicited_at,omitempty"`
	// FormSubmission preserva o que o cliente enviou no formulário (snapshot
	// imutável da submissão), separado do valor atual da party.
	FormSubmission *ChecklistFormSubmission `json:"form_submission,omitempty" bson:"form_submission,omitempty"`
}

type ChecklistRequestedField struct {
	Key      string `json:"key" bson:"key"`
	Label    string `json:"label" bson:"label"`
	Type     string `json:"type" bson:"type"`
	Required bool   `json:"required" bson:"required"`
}

// ChecklistFormSubmission é o snapshot do formulário no momento em que o
// cliente o enviou. Permite revisar/auditar o que foi enviado mesmo depois de
// o operador corrigir a party.
type ChecklistFormSubmission struct {
	Values          map[string]string         `json:"values" bson:"values"`
	RequestedFields []ChecklistRequestedField `json:"requested_fields,omitempty" bson:"requested_fields,omitempty"`
	SubmittedBy     string                    `json:"submitted_by" bson:"submitted_by"`
	SubmittedAt     time.Time                 `json:"submitted_at" bson:"submitted_at"`
}

// InitChecklist replaces the entire checklist (used when applying a template).
func (d *Deal) InitChecklist(items []ChecklistItem) {
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = uuid.New().String()
		}
		if items[i].Status == "" {
			items[i].Status = ChecklistPending
		}
		items[i].RequestMode = NormalizeRequestMode(string(items[i].RequestMode))
	}
	d.Checklist = items
	d.UpdatedAt = time.Now()
}

// AddChecklistItem appends a single item to the checklist (upload mode).
func (d *Deal) AddChecklistItem(name string, category ChecklistItemCategory, required bool, description string) {
	d.AddChecklistItemConfigured(name, category, required, description, "", nil)
}

// AddChecklistItemConfigured appends a single item with solicitation config:
// requestMode ("upload" | "form") e os campos solicitados ao cliente.
func (d *Deal) AddChecklistItemConfigured(name string, category ChecklistItemCategory, required bool, description, requestMode string, requestedFields []ChecklistRequestedField) {
	item := ChecklistItem{
		ID:              uuid.New().String(),
		Name:            name,
		Category:        category,
		Status:          ChecklistPending,
		Required:        required,
		Description:     description,
		RequestMode:     NormalizeRequestMode(requestMode),
		RequestedFields: requestedFields,
	}
	d.Checklist = append(d.Checklist, item)
	d.UpdatedAt = time.Now()
}

// RemoveChecklistItem removes an item by ID.
func (d *Deal) RemoveChecklistItem(itemID string) {
	for i, item := range d.Checklist {
		if item.ID == itemID {
			d.Checklist = append(d.Checklist[:i], d.Checklist[i+1:]...)
			d.UpdatedAt = time.Now()
			return
		}
	}
}

// UpdateChecklistItemStatus updates the status of a checklist item.
// ChecklistItemByID devolve o item do checklist com o id informado, ou nil.
// Ponteiro para o item DENTRO do slice (callers leem os campos já atualizados,
// como o StorageKey que acabou de ser gravado no upload).
func (d *Deal) ChecklistItemByID(itemID string) *ChecklistItem {
	if d == nil {
		return nil
	}
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			return &d.Checklist[i]
		}
	}
	return nil
}

func (d *Deal) UpdateChecklistItemStatus(itemID string, status ChecklistItemStatus, rejectionReason string) {
	now := time.Now()
	for i, item := range d.Checklist {
		if item.ID == itemID {
			d.Checklist[i].Status = status
			switch status {
			case ChecklistRejected:
				d.Checklist[i].RejectionReason = rejectionReason
				d.Checklist[i].ReviewedAt = &now
			case ChecklistApproved:
				d.Checklist[i].RejectionReason = ""
				d.Checklist[i].ReviewedAt = &now
			case ChecklistUploaded:
				d.Checklist[i].UploadedAt = &now
			}
			d.UpdatedAt = now
			return
		}
	}
}

// ChecklistStats is a derived snapshot of the deal's checklist progress.
// Used by the Esteira's Documentation tab and by the (future) gate that
// blocks CONTRACT/CONVERTED transitions when required docs are missing
// (see roadmap §4 W3).
//
// Counts only reflect items present *on the deal* — items removed from
// the checklist (e.g. by the operator clearing a template) drop out of
// the totals. Empty checklist returns all zeros and
// `RequiredComplete = true` (vacuous truth: nothing to require, nothing
// to wait for).
type ChecklistStats struct {
	Total            int  `json:"total"`
	Required         int  `json:"required"`
	Approved         int  `json:"approved"`
	RequiredApproved int  `json:"required_approved"`
	Pending          int  `json:"pending"`
	Uploaded         int  `json:"uploaded"`
	Rejected         int  `json:"rejected"`
	RequiredComplete bool `json:"required_complete"`
}

// ComputeChecklistStats walks the checklist once and returns the
// aggregated counters. Pure — does not mutate the deal.
func (d *Deal) ComputeChecklistStats() ChecklistStats {
	stats := ChecklistStats{Total: len(d.Checklist)}
	for _, item := range d.Checklist {
		if item.Required {
			stats.Required++
		}
		switch item.Status {
		case ChecklistPending:
			stats.Pending++
		case ChecklistUploaded:
			stats.Uploaded++
		case ChecklistApproved:
			stats.Approved++
			if item.Required {
				stats.RequiredApproved++
			}
		case ChecklistRejected:
			stats.Rejected++
		}
	}
	stats.RequiredComplete = stats.Required == 0 || stats.RequiredApproved == stats.Required
	return stats
}

// RequiredChecklistComplete is a convenience predicate for callers that
// only need the gate boolean — same result as `ComputeChecklistStats().RequiredComplete`
// without allocating the struct.
func (d *Deal) RequiredChecklistComplete() bool {
	for _, item := range d.Checklist {
		if item.Required && item.Status != ChecklistApproved {
			return false
		}
	}
	return true
}

// PopulateChecklistStats fills the `ChecklistStats` field so the deal
// can be JSON-serialized with the computed snapshot included. Idempotent
// — safe to call multiple times.
func (d *Deal) PopulateChecklistStats() {
	s := d.ComputeChecklistStats()
	d.ChecklistStats = &s
}

// SolicitChecklistItem marks an item as solicited (requested from client).
func (d *Deal) SolicitChecklistItem(itemID string) {
	d.SolicitChecklistItemScoped(itemID, "", nil, "")
}

// SolicitChecklistItemScoped marca o item como solicitado e grava a
// configuração da solicitação: campos a preencher e modo (upload | form).
func (d *Deal) SolicitChecklistItemScoped(itemID string, partyID string, requestedFields []ChecklistRequestedField, requestMode string) {
	now := time.Now()
	for i, item := range d.Checklist {
		if item.ID == itemID {
			if partyID != "" {
				d.Checklist[i].PartyID = partyID
			}
			if requestedFields != nil {
				d.Checklist[i].RequestedFields = requestedFields
			}
			d.Checklist[i].RequestMode = NormalizeRequestMode(requestMode)
			d.Checklist[i].Solicited = true
			d.Checklist[i].SolicitedAt = &now
			d.UpdatedAt = now
			return
		}
	}
}

func (d *Deal) validatePartyRole(role PartyRole) error {
	switch d.Type {
	case TypeSale:
		if role != PartyRoleBuyer && role != PartyRoleOwner && role != PartyRoleGuarantor && role != PartyRoleWitness &&
			role != PartyRoleSpouse && role != PartyRoleAttorney && role != PartyRoleLegalRep && role != PartyRoleGuardian && role != PartyRoleCurator {
			return fmt.Errorf("invalid party role '%s' for sale deal", role)
		}
	case TypeRent:
		if role != PartyRoleTenant && role != PartyRoleOwner && role != PartyRoleGuarantor && role != PartyRoleWitness &&
			role != PartyRoleSpouse && role != PartyRoleAttorney && role != PartyRoleLegalRep && role != PartyRoleGuardian && role != PartyRoleCurator {
			return fmt.Errorf("invalid party role '%s' for rental deal", role)
		}
	}
	return nil
}

func partyRoleFromSignatoryRole(role SignatoryRole) (PartyRole, bool) {
	switch role {
	case SignatoryRoleInquilino, SignatoryRoleLocatario:
		return PartyRoleTenant, true
	case SignatoryRoleComprador:
		return PartyRoleBuyer, true
	case SignatoryRoleProprietario, SignatoryRoleLocador, SignatoryRoleVendedor:
		return PartyRoleOwner, true
	case SignatoryRoleFiador, SignatoryRoleAvalista:
		return PartyRoleGuarantor, true
	case SignatoryRoleTestemunha:
		return PartyRoleWitness, true
	case SignatoryRoleConjuge:
		return PartyRoleSpouse, true
	case SignatoryRoleProcurador:
		return PartyRoleAttorney, true
	case SignatoryRoleRepLegal:
		return PartyRoleLegalRep, true
	case SignatoryRoleTutor:
		return PartyRoleGuardian, true
	case SignatoryRoleCurador:
		return PartyRoleCurator, true
	default:
		return "", false
	}
}

func (d *Deal) findPartyIndexByID(partyID string) int {
	for i := range d.Parties {
		if d.Parties[i].ID == partyID {
			return i
		}
	}
	return -1
}

func (d *Deal) findPartyIndexByRoleAndEmail(role PartyRole, email string) int {
	for i := range d.Parties {
		if d.Parties[i].Role == role && d.Parties[i].Email != "" && d.Parties[i].Email == email {
			return i
		}
	}
	return -1
}

func (d *Deal) ensureSinglePrimaryForRole(role PartyRole, primaryID string, wantsPrimary bool) {
	if !wantsPrimary {
		if d.PrimaryByRole(role) != nil {
			return
		}
		for i := range d.Parties {
			if d.Parties[i].Role == role {
				d.Parties[i].IsPrimary = d.Parties[i].ID == primaryID
				break
			}
		}
		return
	}
	for i := range d.Parties {
		if d.Parties[i].Role != role {
			continue
		}
		d.Parties[i].IsPrimary = d.Parties[i].ID == primaryID
	}
}

func (d *Deal) removePartyByID(partyID string, enforcePrimary bool) error {
	idx := d.findPartyIndexByID(partyID)
	if idx == -1 {
		return nil
	}
	role := d.Parties[idx].Role
	isPrimary := d.Parties[idx].IsPrimary

	if enforcePrimary && isPrimary {
		others := 0
		for i := range d.Parties {
			if i == idx {
				continue
			}
			if d.Parties[i].Role == role {
				others++
			}
		}
		if others == 0 {
			return fmt.Errorf("cannot remove the only primary party for role '%s'", role)
		}
	}

	d.Parties = append(d.Parties[:idx], d.Parties[idx+1:]...)

	if enforcePrimary && isPrimary && d.PrimaryByRole(role) == nil {
		for i := range d.Parties {
			if d.Parties[i].Role == role {
				d.Parties[i].IsPrimary = true
				break
			}
		}
	}
	d.UpdatedAt = time.Now()
	return nil
}

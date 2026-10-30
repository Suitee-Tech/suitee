package lead

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type LeadStatus string

const (
	LeadStatusNew               LeadStatus = "NEW"
	LeadStatusPendingAcceptance LeadStatus = "PENDING_ACCEPTANCE"
	LeadStatusContacted         LeadStatus = "CONTACTED"
	LeadStatusQualified         LeadStatus = "QUALIFIED"
	LeadStatusVisitScheduled    LeadStatus = "VISIT_SCHEDULED"
	LeadStatusProposal          LeadStatus = "PROPOSAL"
	LeadStatusNegotiation       LeadStatus = "NEGOTIATION"
	LeadStatusConverted         LeadStatus = "CONVERTED"
	LeadStatusLost              LeadStatus = "LOST"
	// LeadStatusDisqualified — lead sem potencial, desqualificado
	// formalmente pelo corretor com um motivo registrado (#429).
	LeadStatusDisqualified LeadStatus = "DISQUALIFIED"
)

type LeadSource string

const (
	LeadSourceWebsite  LeadSource = "WEBSITE"
	LeadSourceReferral LeadSource = "REFERRAL"
	LeadSourceSocial   LeadSource = "SOCIAL"
	LeadSourceManual   LeadSource = "MANUAL"
	LeadSourceCampaign LeadSource = "CAMPAIGN"
	LeadSourceOther    LeadSource = "OTHER"
	// Portal sources — leads captured from real estate portals
	LeadSourceZAP      LeadSource = "ZAP_IMOVEIS"
	LeadSourceOLX      LeadSource = "OLX_GRUPO_ZAP"
	LeadSourceVivaReal LeadSource = "VIVAREAL"
)

type AuthMetadata struct {
	AuthProvider string `bson:"auth_provider,omitempty"`
	AuthID       string `bson:"auth_id,omitempty"`
	PasswordHash string `bson:"password_hash,omitempty"`
}

// ClientRole represents the role of a lead/client in the real estate process.
type ClientRole string

const (
	ClientRoleOwner     ClientRole = "owner"     // Proprietário (owner of a property)
	ClientRoleBuyer     ClientRole = "buyer"     // Comprador
	ClientRoleTenant    ClientRole = "tenant"    // Inquilino
	ClientRoleGuarantor ClientRole = "guarantor" // Fiador
)

type Lead struct {
	ID              string     `bson:"_id" validate:"required"`
	DisplayID       string     `bson:"display_id,omitempty"`
	Name            string     `bson:"name" validate:"required" pii:"name"`
	Email           string     `bson:"email" validate:"required,email" pii:"email"`
	Phone           string     `bson:"phone" validate:"required" pii:"phone"`
	Avatar          string     `bson:"avatar,omitempty"`
	Source          LeadSource `bson:"source" validate:"required"`
	Status          LeadStatus `bson:"status" validate:"required"`
	CreatedAt       time.Time  `bson:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"`
	UpdatedSeq      int64      `bson:"updated_seq,omitempty"`
	LastInteraction *time.Time `bson:"last_interaction,omitempty"`

	TenantID string `bson:"tenant_id" validate:"omitempty"`

	AuthMetadata `bson:",inline"`

	PropertyInterest *LeadProperty `bson:"property_interest,omitempty"`
	PropertyIDs      []string      `bson:"property_ids,omitempty"`
	AssignedBrokerID string        `bson:"assigned_broker_id"`
	History          []LeadHistory `bson:"history,omitempty"`

	// Client segmentation — set when lead is converted to a client
	// A lead keeps the same ID throughout its lifecycle (lead → client)
	IsClient bool       `bson:"is_client,omitempty"`
	Role     ClientRole `bson:"role,omitempty"` // owner, buyer, tenant, guarantor

	// Captação lead fields — set when this lead represents an owner from property acquisition
	CaptacaoPropertyID string `bson:"captacao_property_id,omitempty"` // property being registered
	OwnerLeadID        string `bson:"owner_lead_id,omitempty"`        // owner lead linked at captação

	// ContractID is set when the lead is converted to a client via a signed contract
	ContractID string `bson:"contract_id,omitempty"`

	// AccountID links to the unified auth account for client portal access
	AccountID string `bson:"account_id,omitempty"`

	// RecommendedPropertyID é o imóvel que o corretor recomenda ao cliente
	// (exibido no portal como "Sugestão do seu corretor").
	RecommendedPropertyID string `bson:"recommended_property_id,omitempty" json:"recommended_property_id,omitempty"`
	// RecommendedPropertySnapshot guarda um resumo do imóvel recomendado
	// (título/localização/valor) para o portal exibir sem depender do token
	// do cliente acessar o core.
	RecommendedPropertySnapshot *RecommendedPropertySnapshot `bson:"recommended_property_snapshot,omitempty" json:"recommended_property_snapshot,omitempty"`
	// VisitRequest guarda a solicitação de visita do cliente para o imóvel
	// recomendado (agendamento por lead, sem depender de negócio).
	VisitOperation *VisitOperation `json:"visit_operation,omitempty"`
	VisitRequest   *VisitRequest   `bson:"visit_request,omitempty" json:"visit_request,omitempty"`

	// PartnershipShared is the per-lead opt-in for the partner shared-content
	// feature: when true, this lead is exposed (privacy-projected) to partner
	// companies whose active partnership grants the `leads` view scope.
	//
	// NOTE: the Lead struct is otherwise bson-tagged only, so json.Marshal
	// stores every other field under its Go field name (PascalCase). This
	// field carries an explicit json tag so it is persisted under the
	// lowercase `partnership_shared` key — the key the shared-leads query
	// (metadata->>'partnership_shared') and the cross-tenant repo both read.
	PartnershipShared bool `bson:"partnership_shared,omitempty" json:"partnership_shared,omitempty"`

	// SLA retries por tier_id (controla redirecionamentos automáticos)
	SLARetries map[string]int `bson:"sla_retries,omitempty"`

	// SLA and Routing fields
	AssignedAt      time.Time `bson:"assigned_at,omitempty"`
	ExpiresAt       time.Time `bson:"expires_at,omitempty"`
	RouletteRetries int       `bson:"roulette_retries,omitempty"`
	EscalationLevel int       `bson:"escalation_level,omitempty"`

	// FirstContactAt — registrada quando o corretor faz o primeiro
	// contato com o lead (telefone, WhatsApp, etc.) ou quando o status
	// transita explicitamente para CONTACTED. Issue #427.
	//
	// SLA primeiro contato: leads atribuídos sem FirstContactAt em 1h
	// (configurável via FirstContactSLAMinutes) disparam evento
	// lead.first_contact_sla_violated. Worker em LeadSLAService varre
	// a cada minuto.
	//
	// Idempotência: uma vez setado, MarkFirstContact() não sobrescreve.
	// O timestamp original é o que importa — o primeiro contato cumpriu
	// (ou violou) o SLA e isso é o que ficou registrado.
	FirstContactAt *time.Time `bson:"first_contact_at,omitempty"`

	// FirstContactSLAViolatedAt — preenchido quando o worker detecta
	// que o lead atravessou o limite de 1h sem first contact. Marca o
	// evento como já notificado (idempotência: lead que já violou não
	// dispara evento duplicado).
	FirstContactSLAViolatedAt *time.Time `bson:"first_contact_sla_violated_at,omitempty"`

	Qualification map[string]interface{} `bson:"qualification,omitempty"`
}

// MarkFirstContact registra o primeiro contato com o lead. Idempotente
// — se já houver FirstContactAt, mantém o valor original. Issue #427.
//
// Também transita o status para CONTACTED quando ainda em NEW ou
// PENDING_ACCEPTANCE (estados pré-contato). Stages downstream
// (QUALIFIED, VISIT_SCHEDULED, etc.) não regressam pra CONTACTED.
func (l *Lead) MarkFirstContact(now time.Time) bool {
	if l.FirstContactAt != nil {
		return false // idempotente
	}
	l.FirstContactAt = &now
	if l.Status == LeadStatusNew || l.Status == LeadStatusPendingAcceptance {
		l.Status = LeadStatusContacted
	}
	l.UpdatedAt = now
	return true
}

// FirstContactSLABreached é true quando o SLA de primeiro contato
// foi atravessado: lead atribuído (AssignedAt setado), sem
// FirstContactAt, e mais antigo que slaCutoff. Lead em status terminal
// (CONVERTED, LOST) não conta — o SLA não se aplica retroativamente
// a leads fechados.
func (l *Lead) FirstContactSLABreached(slaCutoff time.Time) bool {
	if l.FirstContactAt != nil {
		return false
	}
	if l.AssignedAt.IsZero() {
		return false
	}
	if l.Status == LeadStatusConverted || l.Status == LeadStatusLost {
		return false
	}
	return l.AssignedAt.Before(slaCutoff)
}

func (l *Lead) Validate() error {
	return validate.Struct(l)
}

func NewLead(name, email, phone string, source LeadSource, tenantID string) (*Lead, error) {
	now := time.Now()
	name, err := normalizeLeadName(name)
	if err != nil {
		return nil, err
	}
	l := &Lead{
		ID:        uuid.New().String(),
		Name:      name,
		Email:     email,
		Phone:     phone,
		Source:    source,
		Status:    LeadStatusNew,
		CreatedAt: now,
		UpdatedAt: now,
		TenantID:  tenantID,
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return l, nil
}

// normalizeLeadName aplica a MESMA regra de nome de pessoa usada nos contratos
// e signatários (`pkg/validation`): o lead alimenta a parte do negócio, e um
// nome sem sobrenome só falharia depois, ao iniciar o negócio na esteira
// ("Nome do signatário precisa incluir sobrenome"). Falhar aqui aponta o campo
// enquanto o operador ainda está no cadastro.
func normalizeLeadName(name string) (string, error) {
	clean := validation.SanitizePersonName(name)
	if err := validation.ValidateFullPersonName(clean, "Nome"); err != nil {
		return "", err
	}
	return clean, nil
}

// AddHistory adds a new history entry to the lead
func (l *Lead) AddHistory(action LeadAction, userID string, details string, oldStatus, newStatus *LeadStatus) {
	entry := LeadHistory{
		ID:             uuid.New().String(),
		LeadID:         l.ID,
		Action:         action,
		Timestamp:      time.Now(),
		UserID:         userID,
		Details:        details,
		PreviousStatus: oldStatus,
		NewStatus:      newStatus,
	}
	l.History = append(l.History, entry)
	l.UpdatedAt = time.Now()
}

// ConvertToClient promotes a lead to a client, preserving its unique ID and history.
func (l *Lead) ConvertToClient(role ClientRole, contractID string) {
	l.IsClient = true
	l.Role = role
	if contractID != "" {
		l.ContractID = contractID
	}
	l.Status = LeadStatusConverted
	l.UpdatedAt = time.Now()
}

// NewLeadWithOptionalTenant creates a new lead with optional tenant ID and other fields
// VisitRequest é a solicitação de visita do cliente para o imóvel recomendado.
type VisitRequest struct {
	PropertyID    string `bson:"property_id,omitempty" json:"property_id,omitempty"`
	Date          string `bson:"date,omitempty" json:"date,omitempty"`
	Time          string `bson:"time,omitempty" json:"time,omitempty"`
	EndTime       string `bson:"end_time,omitempty" json:"end_time,omitempty"`
	Note          string `bson:"note,omitempty" json:"note,omitempty"`
	Status        string `bson:"status,omitempty" json:"status,omitempty"`
	AppointmentID string `bson:"appointment_id,omitempty" json:"appointment_id,omitempty"`
}

// RecommendedPropertySnapshot é um resumo leve do imóvel recomendado pelo corretor.
type RecommendedPropertySnapshot struct {
	ID           string   `bson:"id,omitempty" json:"id,omitempty"`
	Title        string   `bson:"title,omitempty" json:"title,omitempty"`
	Neighborhood string   `bson:"neighborhood,omitempty" json:"neighborhood,omitempty"`
	City         string   `bson:"city,omitempty" json:"city,omitempty"`
	State        string   `bson:"state,omitempty" json:"state,omitempty"`
	SalePrice    float64  `bson:"sale_price,omitempty" json:"sale_price,omitempty"`
	RentPrice    float64  `bson:"rent_price,omitempty" json:"rent_price,omitempty"`
	ListingType  string   `bson:"listing_type,omitempty" json:"listing_type,omitempty"`
	ThumbnailURL string   `bson:"thumbnail_url,omitempty" json:"thumbnail_url,omitempty"`
	MediaImages  []string `bson:"media_images,omitempty" json:"media_images,omitempty"`
}

func NewLeadWithOptionalTenant(name, email, phone string, avatar string, source LeadSource, status LeadStatus, propertyInterest *LeadProperty, assignedBrokerID string, tenantID string) (*Lead, error) {
	now := time.Now()

	name, err := normalizeLeadName(name)
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = LeadStatusNew
	}
	status = LeadStatus(strings.ToUpper(string(status)))

	l := &Lead{
		ID:               uuid.New().String(),
		Name:             name,
		Email:            email,
		Phone:            phone,
		Avatar:           avatar,
		Source:           source,
		Status:           status,
		CreatedAt:        now,
		UpdatedAt:        now,
		PropertyInterest: propertyInterest,
		AssignedBrokerID: assignedBrokerID,
	}

	// Only set tenant ID if provided
	if tenantID != "" {
		l.TenantID = tenantID
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}
	return l, nil
}

// VisitOperation is a durable intent. Portal overview only exposes VisitRequest.
type VisitOperation struct {
	ExpectedAgendaUpdatedAt *time.Time    `json:"expected_agenda_updated_at,omitempty"`
	CustomerAccountID       string        `json:"customer_account_id"`
	ID                      string        `json:"id"`
	Token                   string        `json:"token"`
	Desired                 *VisitRequest `json:"desired"`
	Before                  *VisitRequest `json:"before"`
	BrokerID                string        `json:"broker_id"`
	AccountID               string        `json:"account_id"`
	Create                  bool          `json:"create"`
	LeaseUntil              time.Time     `json:"lease_until"`
}

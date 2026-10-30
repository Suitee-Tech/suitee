package envelope

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status do envelope local. RUNNING/PARTIALLY_SIGNED/FINISHED/REFUSED espelham
// transições da Clicksign; PENDING e ERROR são estados puramente locais.
type Status string

const (
	StatusPending         Status = "PENDING"
	StatusDraft           Status = "DRAFT"
	StatusRunning         Status = "RUNNING"
	StatusPartiallySigned Status = "PARTIALLY_SIGNED"
	StatusFinished        Status = "FINISHED"
	StatusRefused         Status = "REFUSED"
	StatusCanceled        Status = "CANCELED"
	StatusExpired         Status = "EXPIRED"
	StatusError           Status = "ERROR"
)

// AuthMethod é o catálogo de métodos de autenticação suportados no MVP.
// Cada item é mapeado para o campo "auth" da Clicksign no client layer.
type AuthMethod string

const (
	AuthEmailToken    AuthMethod = "email"
	AuthWhatsApp      AuthMethod = "whatsapp"
	AuthSMS           AuthMethod = "sms"
	AuthSelfie        AuthMethod = "selfie"
	AuthICPBrasil     AuthMethod = "icp_brasil"
	AuthSelfiePlusDoc AuthMethod = "selfie+document"
)

type SignerRole string

const (
	SignerRoleSign    SignerRole = "sign"
	SignerRoleWitness SignerRole = "witness"
)

type SignerStatus string

const (
	SignerStatusPending SignerStatus = "PENDING"
	SignerStatusViewed  SignerStatus = "VIEWED"
	SignerStatusSigned  SignerStatus = "SIGNED"
	SignerStatusRefused SignerStatus = "REFUSED"
)

// EnvelopeKind distingue o envelope de assinatura do contrato em si do
// envelope do distrato (acordo de rescisão) — #436. Um mesmo contract_id
// pode ter um envelope ativo de cada kind; ver o índice único parcial
// uq_signature_envelopes_contract_kind_active.
type EnvelopeKind string

const (
	EnvelopeKindContract EnvelopeKind = "contract"
	EnvelopeKindDistrato EnvelopeKind = "distrato"
)

type Signer struct {
	SignerID            string       `json:"signer_id"`
	ClicksignSignerKey  string       `json:"clicksign_signer_key,omitempty"`
	Name                string       `json:"name"`
	Email               string       `json:"email"`
	PhoneNumber         string       `json:"phone_number,omitempty"`
	Documentation       string       `json:"documentation,omitempty"`
	Birthday            string       `json:"birthday,omitempty"`
	Role                SignerRole   `json:"role"`
	Auth                []AuthMethod `json:"auth"`
	Status              SignerStatus `json:"status"`
	SignedAt            *time.Time   `json:"signed_at,omitempty"`
	RequestSignatureKey string       `json:"request_signature_key,omitempty"`
	// Group is the 1-based signing group (Clicksign `group` attribute).
	// Signers in the same group sign in parallel; lower groups must finish
	// before higher groups are notified. 0/omitted = no ordering (legacy
	// behavior: every signer notified at activation time).
	Group int `json:"group,omitempty"`
}

type Document struct {
	ClicksignDocumentKey string `json:"clicksign_document_key,omitempty"`
	SHA256               string `json:"sha256,omitempty"`
	SizeBytes            int64  `json:"size_bytes,omitempty"`
	Filename             string `json:"filename,omitempty"`
}

type SignatureEnvelope struct {
	ID                 string       `json:"id"`
	TenantID           string       `json:"tenant_id"`
	ContractID         string       `json:"contract_id"`
	DealID             string       `json:"deal_id,omitempty"`
	Kind               EnvelopeKind `json:"kind"`
	ExternalEnvelopeID string       `json:"external_envelope_id,omitempty"`
	Environment        string       `json:"environment"`
	Status             Status       `json:"status"`
	Signers            []Signer     `json:"signers"`
	// Documents is the slice of all documents attached to the envelope. The
	// main contract PDF lives at index 0; subsequent entries are operator
	// attachments (procuracoes, RGs, etc.) added at envelope-create time.
	Documents []Document `json:"documents"`
	// Document is the legacy singular field kept for backward-compatibility
	// reads of envelopes persisted before multi-doc support. Writers must
	// populate Documents; the repository normalizes the legacy field into
	// Documents on read. Do NOT consume this field in new code.
	Document   *Document  `json:"document,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// TerminalEventPublishedAt marca quando o evento de desfecho
	// (refused/expired/envelope_finished) foi publicado no barramento.
	//
	// Existe porque o envelope pode terminar LOCALMENTE sem o toca-deal saber:
	// o operador cancela, o webhook chega com a escrita falhando, ou o evento
	// se perde no caminho. Sem esse carimbo um envelope terminal saía da
	// varredura de reconciliação (ListStaleActive só olhava estados ativos) e o
	// contrato ficava preso em PENDING_SIGNATURE para sempre — a tela do
	// envelope dizia "cancelado" e a do contrato "enviado para assinatura",
	// sem botão de reenviar. Com o carimbo, o envelope terminal ainda não
	// anunciado volta para a varredura e o evento sai uma única vez.
	TerminalEventPublishedAt *time.Time `json:"terminal_event_published_at,omitempty"`
}

// MainDocument returns the first document in the envelope (the contract PDF),
// or nil when the envelope has not yet uploaded its documents. Used by
// services that still need a single "primary" document reference.
func (e *SignatureEnvelope) MainDocument() *Document {
	if len(e.Documents) == 0 {
		return nil
	}
	return &e.Documents[0]
}

// New cria um envelope de contrato local com Status=PENDING. Atalho para
// NewWithKind com EnvelopeKindContract — o caso de longe mais comum.
func New(tenantID, contractID, dealID, environment string, signers []Signer) (*SignatureEnvelope, error) {
	return NewWithKind(tenantID, contractID, dealID, environment, EnvelopeKindContract, signers)
}

// NewWithKind cria um envelope local com Status=PENDING e o kind dado
// (contract ou distrato — #436). Os signers chegam apenas com
// SignerID/Name/Email/Role/Auth preenchidos — ClicksignSignerKey só vem
// após o POST /signers retornar.
func NewWithKind(tenantID, contractID, dealID, environment string, kind EnvelopeKind, signers []Signer) (*SignatureEnvelope, error) {
	if tenantID == "" || contractID == "" {
		return nil, errors.New("tenant_id and contract_id are required")
	}
	if len(signers) == 0 {
		return nil, errors.New("at least one signer is required")
	}
	if environment == "" {
		environment = "sandbox"
	}
	if kind == "" {
		kind = EnvelopeKindContract
	}
	now := time.Now()
	for i := range signers {
		if signers[i].Status == "" {
			signers[i].Status = SignerStatusPending
		}
	}
	return &SignatureEnvelope{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		ContractID:  contractID,
		DealID:      dealID,
		Kind:        kind,
		Environment: environment,
		Status:      StatusPending,
		Signers:     signers,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// MarkSignerViewed localiza o signer pela ClicksignSignerKey e marca como
// VIEWED — mas só se ainda estiver PENDING. Não regride status (um signer já
// SIGNED não volta para VIEWED se a Clicksign reentregar o evento de view
// fora de ordem). Retorna o signer atualizado ou nil se a chave não casa.
func (e *SignatureEnvelope) MarkSignerViewed(clicksignKey string, viewedAt time.Time) *Signer {
	for i := range e.Signers {
		if e.Signers[i].ClicksignSignerKey == clicksignKey {
			if e.Signers[i].Status == SignerStatusPending {
				e.Signers[i].Status = SignerStatusViewed
				e.UpdatedAt = time.Now()
			}
			return &e.Signers[i]
		}
	}
	return nil
}

// MarkSignerSigned localiza o signer pela ClicksignSignerKey e o marca como
// SIGNED. Retorna o signer atualizado e se TODOS já assinaram.
func (e *SignatureEnvelope) MarkSignerSigned(clicksignKey string, signedAt time.Time) (*Signer, bool) {
	var sig *Signer
	allSigned := true
	for i := range e.Signers {
		if e.Signers[i].ClicksignSignerKey == clicksignKey {
			e.Signers[i].Status = SignerStatusSigned
			e.Signers[i].SignedAt = &signedAt
			sig = &e.Signers[i]
		}
		if e.Signers[i].Status != SignerStatusSigned {
			allSigned = false
		}
	}
	if sig != nil {
		if allSigned {
			e.Status = StatusFinished
			e.FinishedAt = &signedAt
		} else {
			e.Status = StatusPartiallySigned
		}
		e.UpdatedAt = time.Now()
	}
	return sig, allSigned
}

// MarkRefused marca o envelope inteiro como REFUSED e o signer (se identificado)
// como REFUSED. Se clicksignKey for vazio, apenas o envelope é marcado.
func (e *SignatureEnvelope) MarkRefused(clicksignKey, _ string) *Signer {
	var sig *Signer
	now := time.Now()
	if clicksignKey != "" {
		for i := range e.Signers {
			if e.Signers[i].ClicksignSignerKey == clicksignKey {
				e.Signers[i].Status = SignerStatusRefused
				sig = &e.Signers[i]
			}
		}
	}
	e.Status = StatusRefused
	e.UpdatedAt = now
	return sig
}

func (e *SignatureEnvelope) MarkCanceled() {
	e.Status = StatusCanceled
	e.UpdatedAt = time.Now()
}

// MarkTerminalEventPublished carimba a publicação do evento de desfecho. É
// idempotente de propósito: a primeira publicação vale, e um segundo carimbo
// não pode reabrir a janela de reentrega (senão a varredura republicaria o
// mesmo desfecho a cada ciclo).
func (e *SignatureEnvelope) MarkTerminalEventPublished(at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	if e.TerminalEventPublishedAt == nil {
		e.TerminalEventPublishedAt = &at
	}
}

// TerminalEventAnnounced informa se o desfecho deste envelope já foi comunicado
// ao toca-deal. Falso num envelope CANCELED/REFUSED/EXPIRED cujo evento nunca
// saiu — o caso que prende o contrato.
func (e *SignatureEnvelope) TerminalEventAnnounced() bool {
	return e.TerminalEventPublishedAt != nil
}

// IsTerminal informa se o envelope já saiu do ciclo de assinatura (concluído,
// recusado, cancelado, expirado ou em erro).
func (e *SignatureEnvelope) IsTerminal() bool {
	switch e.Status {
	case StatusFinished, StatusRefused, StatusCanceled, StatusExpired, StatusError:
		return true
	default:
		return false
	}
}

// MarkExpired marca o envelope como EXPIRED. Disparado pelos webhooks
// `deadline` e `expired` da Clicksign. FinishedAt permanece null porque o
// envelope NÃO foi concluído — apenas atingiu o prazo.
func (e *SignatureEnvelope) MarkExpired(at time.Time) {
	e.Status = StatusExpired
	if at.IsZero() {
		at = time.Now()
	}
	e.UpdatedAt = at
}

func (e *SignatureEnvelope) MarkError(msg string) {
	e.Status = StatusError
	e.LastError = msg
	e.UpdatedAt = time.Now()
}

// SignerByID returns a pointer to the signer with the given internal SignerID,
// or nil if not found.
func (e *SignatureEnvelope) SignerByID(signerID string) *Signer {
	for i := range e.Signers {
		if e.Signers[i].SignerID == signerID {
			return &e.Signers[i]
		}
	}
	return nil
}

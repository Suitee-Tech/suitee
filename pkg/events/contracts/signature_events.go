package contracts

import "github.com/google/uuid"

// SignatureRequestedSigner descreve um signatário sendo enviado para assinatura externa.
type SignatureRequestedSigner struct {
	SignerID      string   `json:"signer_id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	PhoneNumber   string   `json:"phone_number,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Birthday      string   `json:"birthday,omitempty"`
	Role          string   `json:"role"`
	Auth          []string `json:"auth"`
}

// SignatureRequestedEventPayload é publicado por toca-deal quando um operador
// pede para enviar um contrato para assinatura externa. Consumido por
// toca-signature, que cria o envelope na Clicksign.
type SignatureRequestedEventPayload struct {
	TenantID   string `json:"tenant_id"`
	ContractID string `json:"contract_id"`
	DealID     string `json:"deal_id,omitempty"`
	DisplayID  string `json:"display_id,omitempty"`
	// Kind — "contract" (default, quando vazio) ou "distrato" (#436).
	// toca-signature roteia para CreateForContract ou CreateForDistrato.
	Kind        string                     `json:"kind,omitempty"`
	Signers     []SignatureRequestedSigner `json:"signers"`
	DocumentURL string                     `json:"document_url,omitempty"`
	Locale      string                     `json:"locale,omitempty"`
	Deadline    string                     `json:"deadline,omitempty"`
	Timestamp   string                     `json:"timestamp"`
}

type SignatureRequestedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       SignatureRequestedEventPayload `json:"payload"`
}

func (e SignatureRequestedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureRequestedEvent) Name() string         { return "signature.requested" }
func (e SignatureRequestedEvent) Payload() interface{} { return e.Data }

// SignatureSignerSignedEventPayload sinaliza que um único signatário assinou o
// envelope. Publicado por toca-signature quando o webhook `sign` chega.
type SignatureSignerSignedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	SignerID           string `json:"signer_id"`
	SignerEmail        string `json:"signer_email"`
	SignedAt           string `json:"signed_at"`
	EvidenceURL        string `json:"evidence_url,omitempty"`
	Timestamp          string `json:"timestamp"`
}

type SignatureSignerSignedEvent struct {
	EventIDVal string                            `json:"event_id"`
	Data       SignatureSignerSignedEventPayload `json:"payload"`
}

func (e SignatureSignerSignedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureSignerSignedEvent) Name() string         { return "signature.signer_signed" }
func (e SignatureSignerSignedEvent) Payload() interface{} { return e.Data }

// SignatureEnvelopeFinishedEventPayload sinaliza que todos os signatários
// assinaram. toca-deal converte isso para o ContractSignedEvent existente
// (mantendo o pipeline downstream intacto).
type SignatureEnvelopeFinishedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	// Kind — "contract" (default, quando vazio) ou "distrato" (#436).
	// toca-deal ramifica o handler de envelope_finished conforme o valor.
	Kind              string `json:"kind,omitempty"`
	FinishedAt        string `json:"finished_at"`
	SignedDocumentURL string `json:"signed_document_url,omitempty"`
	Timestamp         string `json:"timestamp"`
}

type SignatureEnvelopeFinishedEvent struct {
	EventIDVal string                                `json:"event_id"`
	Data       SignatureEnvelopeFinishedEventPayload `json:"payload"`
}

func (e SignatureEnvelopeFinishedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureEnvelopeFinishedEvent) Name() string         { return "signature.envelope_finished" }
func (e SignatureEnvelopeFinishedEvent) Payload() interface{} { return e.Data }

// SignatureRefusedEventPayload é publicado quando um signatário recusa assinar
// ou o envelope é cancelado (no caso de cancelamento, SignerID fica vazio).
type SignatureRefusedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	SignerID           string `json:"signer_id,omitempty"`
	Reason             string `json:"reason,omitempty"`
	RefusedAt          string `json:"refused_at"`
	Timestamp          string `json:"timestamp"`
}

type SignatureRefusedEvent struct {
	EventIDVal string                       `json:"event_id"`
	Data       SignatureRefusedEventPayload `json:"payload"`
}

func (e SignatureRefusedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureRefusedEvent) Name() string         { return "signature.refused" }
func (e SignatureRefusedEvent) Payload() interface{} { return e.Data }

// SignatureExpiredEventPayload é publicado quando o envelope atinge o prazo
// (webhook `deadline` ou `expired` da Clicksign) sem ter todos os signatários
// assinado. Tratado pelo toca-deal de forma análoga à recusa (contrato volta
// para DRAFT, lock liberado), mas distinto como evento para observabilidade
// (métricas por motivo de não-conclusão).
type SignatureExpiredEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	ExpiredAt          string `json:"expired_at"`
	Timestamp          string `json:"timestamp"`
}

type SignatureExpiredEvent struct {
	EventIDVal string                       `json:"event_id"`
	Data       SignatureExpiredEventPayload `json:"payload"`
}

func (e SignatureExpiredEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureExpiredEvent) Name() string         { return "signature.expired" }
func (e SignatureExpiredEvent) Payload() interface{} { return e.Data }

// SignatureEnvelopeStartedEventPayload sinaliza que o envelope na Clicksign
// passou de DRAFT para RUNNING (signers começaram a receber notificações).
// Publicado quando o webhook `signature_started` é recebido pela primeira
// vez. Antes desse evento, toca-deal só sabia quando alguém assinava — agora
// pode marcar o contrato como PENDING_SIGNATURE de fato (sem esperar o
// primeiro signer).
type SignatureEnvelopeStartedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	StartedAt          string `json:"started_at"`
	Timestamp          string `json:"timestamp"`
}

type SignatureEnvelopeStartedEvent struct {
	EventIDVal string                               `json:"event_id"`
	Data       SignatureEnvelopeStartedEventPayload `json:"payload"`
}

func (e SignatureEnvelopeStartedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureEnvelopeStartedEvent) Name() string         { return "signature.envelope_started" }
func (e SignatureEnvelopeStartedEvent) Payload() interface{} { return e.Data }

// SignatureSignerViewedEventPayload sinaliza que um signatário abriu o link
// de assinatura (Clicksign rastreia isso via webhook `view`). Útil para o
// admin saber que o signer está engajado mesmo que ainda não tenha assinado,
// e para o portal cliente mostrar status "Visualizado".
type SignatureSignerViewedEventPayload struct {
	TenantID           string `json:"tenant_id"`
	ContractID         string `json:"contract_id"`
	EnvelopeID         string `json:"envelope_id"`
	ExternalEnvelopeID string `json:"external_envelope_id"`
	SignerID           string `json:"signer_id"`
	SignerEmail        string `json:"signer_email,omitempty"`
	ViewedAt           string `json:"viewed_at"`
	Timestamp          string `json:"timestamp"`
}

type SignatureSignerViewedEvent struct {
	EventIDVal string                            `json:"event_id"`
	Data       SignatureSignerViewedEventPayload `json:"payload"`
}

func (e SignatureSignerViewedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignatureSignerViewedEvent) Name() string         { return "signature.signer_viewed" }
func (e SignatureSignerViewedEvent) Payload() interface{} { return e.Data }

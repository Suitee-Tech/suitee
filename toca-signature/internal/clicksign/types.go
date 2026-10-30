// Package clicksign wraps the Clicksign API v3 (JSON:API). Every request body
// follows the shape {"data":{"type":"<resource>","attributes":{...}}}; helpers
// in client.go take care of the wrapping.
package clicksign

import (
	"fmt"
	"strings"
	"time"
)

// EnvelopeAttributes is the subset of envelope fields we need for create/patch.
// The Clicksign API supports many more (default_subject, locale, deadline_at,
// etc.) — they can be added when callers need them.
type EnvelopeAttributes struct {
	Name       string  `json:"name,omitempty"`
	Status     string  `json:"status,omitempty"` // "running" to activate
	Locale     string  `json:"locale,omitempty"`
	AutoClose  *bool   `json:"auto_close,omitempty"`
	DeadlineAt *string `json:"deadline_at,omitempty"`
}

type EnvelopeResource struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Attributes EnvelopeAttributes `json:"attributes"`
}

// DocumentRequest is what we send when uploading a document. ContentBase64
// must already be a data URL (e.g. "data:application/pdf;base64,JVBER...").
type DocumentRequest struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

// DocumentStatusRequest é o payload do PATCH por documento — o ÚNICO caminho
// de cancelamento de um envelope em execução na API v3 (ver CancelEnvelope).
type DocumentStatusRequest struct {
	Status string `json:"status"`
}

type DocumentAttributes struct {
	Filename string `json:"filename,omitempty"`
	// Status é o estado do documento no provedor: "draft", "running",
	// "closed" ou "canceled". CancelEnvelope usa para não repetir o
	// cancelamento de um documento já encerrado.
	Status string `json:"status,omitempty"`
	// Legacy fallback only. The Clicksign v3 documents endpoint does NOT
	// put file URLs in `attributes` — it returns status/filename/timestamps
	// here and the actual URLs under `links.files` (see DocumentLinks).
	// Kept so SignedURL() can still read them if an older API shape (or a
	// future change) surfaces them here.
	SignedFileURL   string `json:"signed_file_url,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	OriginalFileURL string `json:"original_file_url,omitempty"`
}

type DocumentResource struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Attributes DocumentAttributes `json:"attributes"`
	// Links carries the JSON:API `links` object. The Clicksign v3 documents
	// endpoint exposes the file URLs HERE — under links.files.signed — not
	// in attributes. This is the field reconcile + webhook recovery need.
	Links DocumentLinks `json:"links"`
}

// DocumentLinks mirrors the JSON:API `links` object on a document returned
// by GET /api/v3/envelopes/:id/documents.
type DocumentLinks struct {
	Self  string            `json:"self,omitempty"`
	Files DocumentLinkFiles `json:"files"`
}

// DocumentLinkFiles holds the short-lived (X-Amz-Expires ≈ 5 min) S3 URLs
// Clicksign serves for a document. `Signed` is the final signed PDF once the
// envelope is closed; `Original` is the pre-signature upload.
type DocumentLinkFiles struct {
	Original string `json:"original,omitempty"`
	Signed   string `json:"signed,omitempty"`
	Ziped    string `json:"ziped,omitempty"`
}

// SignedURL returns the signed-PDF URL for this document, or "" when
// Clicksign has not produced one yet. It deliberately does NOT fall back to
// the original (unsigned) upload — persisting that as the signed contract
// would store an unsigned PDF.
func (d DocumentResource) SignedURL() string {
	if d.Links.Files.Signed != "" {
		return d.Links.Files.Signed
	}
	return d.Attributes.SignedFileURL // legacy fallback
}

// SignedContractURL picks the signed-PDF URL of the contract document out of
// the list returned by ListDocuments. A multi-document envelope also carries
// attachments and Clicksign does not guarantee ordering — toca uploads the
// contract as "contrato-<contractID>.pdf", so we match the contract id in the
// filename. Falls back to the first document that has a signed URL, which is
// correct for single-document envelopes.
func SignedContractURL(docs []DocumentResource, contractID string) string {
	if contractID != "" {
		for _, d := range docs {
			if strings.Contains(d.Attributes.Filename, contractID) {
				if u := d.SignedURL(); u != "" {
					return u
				}
			}
		}
	}
	for _, d := range docs {
		if u := d.SignedURL(); u != "" {
			return u
		}
	}
	return ""
}

// SignerRequest is the payload sent to POST /envelopes/:id/signers. The
// Clicksign field is `has_documentation` — when true, `documentation` (CPF)
// becomes mandatory for some auth methods (biometric, pix, ICP-Brasil).
type SignerRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	PhoneNumber      string `json:"phone_number,omitempty"`
	HasDocumentation bool   `json:"has_documentation"`
	Documentation    string `json:"documentation,omitempty"`
	Birthday         string `json:"birthday,omitempty"` // YYYY-MM-DD
	// Refusable controls whether the signer can refuse via the Clicksign UI.
	// Default true — operators almost always want this.
	Refusable          bool   `json:"refusable"`
	CommunicateEvents  string `json:"communicate_events,omitempty"`
	HandwrittenEnabled *bool  `json:"handwritten_enabled,omitempty"`
	// Group is the 1-based signing-order group. Signers in the same group
	// are notified together; signers in a higher group are only notified
	// after every signer in the previous group has signed. 0/omitted means
	// "no ordering" — same as omitting the field on Clicksign's side.
	Group int `json:"group,omitempty"`
}

type SignerAttributes struct {
	Name                string `json:"name"`
	Email               string `json:"email"`
	PhoneNumber         string `json:"phone_number,omitempty"`
	Documentation       string `json:"documentation,omitempty"`
	Birthday            string `json:"birthday,omitempty"`
	HasDocumentation    bool   `json:"has_documentation"`
	RequestSignatureKey string `json:"request_signature_key,omitempty"`
	Group               int    `json:"group,omitempty"`
	// Read-only fields returned by GET endpoints (list/show). Empty on
	// create/update payloads. `status` is one of "pending", "signed",
	// "refused" — depends on Clicksign API version semantics.
	Status        string `json:"status,omitempty"`
	SignedAt      string `json:"signed_at,omitempty"`
	ViewedAt      string `json:"viewed_at,omitempty"`
	RefusedAt     string `json:"refused_at,omitempty"`
	RefusedReason string `json:"refused_reason,omitempty"`
}

type SignerResource struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Attributes SignerAttributes `json:"attributes"`
}

// RequirementAction is one of "agree" | "provide_evidence" | "sign". For our
// MVP we only emit "agree" (qualification — defines the signer's role) and
// "provide_evidence" (authentication — defines the auth method).
type RequirementAction string

const (
	RequirementActionAgree           RequirementAction = "agree"
	RequirementActionProvideEvidence RequirementAction = "provide_evidence"
	RequirementActionSign            RequirementAction = "sign"
)

// RequirementRequest is the payload sent to POST /envelopes/:id/requirements.
// Both "qualification" (role) and "authentication" (auth method) requirements
// share this same shape — what differs is `action` and `auth`/`role`.
type RequirementRequest struct {
	Action RequirementAction `json:"action"`
	Auth   string            `json:"auth,omitempty"` // email|whatsapp|sms|selfie|...
	Role   string            `json:"role,omitempty"` // sign|witness|...
}

// RequirementRelationships links a requirement to the document and signer it
// applies to. Clicksign uses JSON:API relationships at the top level of the
// payload (not inside attributes).
type RequirementRelationships struct {
	Document *RequirementRel `json:"document,omitempty"`
	Signer   *RequirementRel `json:"signer,omitempty"`
}

type RequirementRel struct {
	Data RequirementRelData `json:"data"`
}

type RequirementRelData struct {
	Type string `json:"type"` // "documents" | "signers"
	ID   string `json:"id"`
}

type RequirementResource struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Attributes    RequirementRequest       `json:"attributes"`
	Relationships RequirementRelationships `json:"relationships,omitempty"`
}

// WebhookPayload mirrors the structure Clicksign POSTs to our callback URL.
// Field names follow the snake_case JSON delivered. We are conservative about
// types — anything the platform may include that we don't act on is left out.
type WebhookPayload struct {
	Event WebhookEvent `json:"event"`
	// Document and Signer are populated for envelope-level events. The
	// Clicksign payload nests these under top-level keys (not under event).
	Document *WebhookDocument `json:"document,omitempty"`
	Signer   *WebhookSigner   `json:"signer,omitempty"`
	Envelope *WebhookEnvelope `json:"envelope,omitempty"`
}

type WebhookEvent struct {
	Name       string    `json:"name"`        // "sign", "auto_close", "refusal", ...
	OccurredAt time.Time `json:"occurred_at"` // RFC3339
	DeliveryID string    `json:"delivery_id"` // optional dedup key
	Data       any       `json:"data,omitempty"`
}

type WebhookEnvelope struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type WebhookDocument struct {
	Key             string `json:"key"`
	Filename        string `json:"filename,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	SignedFileURL   string `json:"signed_file_url,omitempty"`
	OriginalFileURL string `json:"original_file_url,omitempty"`
}

type WebhookSigner struct {
	Key           string  `json:"key"` // matches our ClicksignSignerKey
	Email         string  `json:"email,omitempty"`
	Name          string  `json:"name,omitempty"`
	SignAt        *string `json:"sign_at,omitempty"`
	RefusedReason string  `json:"refused_reason,omitempty"`
	EvidenceURL   string  `json:"evidence_url,omitempty"`
}

// APIError é o erro tipado retornado pelo client quando a Clicksign responde
// com status >= 400. Implementa `error` e suporta `errors.As`. RawBody é
// mantido para o caso de o corpo não vir no shape JSON:API esperado.
type APIError struct {
	Method  string
	URL     string // host+path efetivamente requisitado (após redirects), usado em logs
	Path    string
	Status  int
	Errors  []ClicksignAPIError
	RawBody string
}

// ClicksignAPIError espelha o objeto dentro do array `errors` retornado pela
// Clicksign no padrão JSON:API.
//
//	{
//	  "errors": [
//	    {"code":"...", "title":"...", "detail":"...", "source":{"pointer":"..."}}
//	  ]
//	}
type ClicksignAPIError struct {
	Code   string            `json:"code,omitempty"`
	Title  string            `json:"title,omitempty"`
	Detail string            `json:"detail,omitempty"`
	Status string            `json:"status,omitempty"`
	Source map[string]string `json:"source,omitempty"`
}

// Error compõe uma linha de log diagnóstica. Inclui o URL completo (host+path)
// quando disponível — sem ele um openresty 405 vindo de um host errado fica
// impossível de diagnosticar só pelo path.
func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	target := e.URL
	if target == "" {
		target = e.Path
	}
	if len(e.Errors) > 0 {
		first := e.Errors[0]
		return fmt.Sprintf("clicksign %s %s status=%d code=%q title=%q detail=%q",
			e.Method, target, e.Status, first.Code, first.Title, first.Detail)
	}
	return fmt.Sprintf("clicksign %s %s status=%d body=%s", e.Method, target, e.Status, e.RawBody)
}

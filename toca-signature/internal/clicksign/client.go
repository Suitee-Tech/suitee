package clicksign

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
)

const acceptHeader = "application/vnd.api+json"

// API is the surface used by EnvelopeService. Defining it as an interface lets
// tests replace the network layer with a fake without spinning up an HTTP
// server. The concrete *Client below is the only production implementation.
type API interface {
	CreateEnvelope(ctx context.Context, tenantID, name string) (*EnvelopeResource, error)
	GetEnvelope(ctx context.Context, tenantID, envelopeID string) (*EnvelopeResource, error)
	// ListSigners returns every signer attached to the envelope with the
	// read-only status fields populated (status, signed_at, viewed_at).
	// Used by Reconcile to detect per-signer drift when individual sign
	// webhooks were never delivered — GetEnvelope alone returns only the
	// envelope-level status, never per-signer state.
	ListSigners(ctx context.Context, tenantID, envelopeID string) ([]SignerResource, error)
	// ListDocuments returns every document attached to the envelope with
	// the signed-file URL fields populated when the envelope has
	// finished. Used by Reconcile to recover the signed PDF URL when
	// the original `envelope_finished` webhook arrived without it
	// (Clicksign CDN race) and by the on-demand refresh-signed-url
	// admin endpoint backing the toca-deal backfill.
	ListDocuments(ctx context.Context, tenantID, envelopeID string) ([]DocumentResource, error)
	UploadDocument(ctx context.Context, tenantID, envelopeID string, req DocumentRequest) (*DocumentResource, error)
	AddSigner(ctx context.Context, tenantID, envelopeID string, req SignerRequest) (*SignerResource, error)
	// UpdateSigner sends a PATCH to /api/v3/envelopes/{id}/signers/{key}.
	// Clicksign allows updating name/email/phone while the signer has not
	// yet acted; once SIGNED/REFUSED the PATCH returns 422. Callers should
	// gate by signer.Status before calling.
	UpdateSigner(ctx context.Context, tenantID, envelopeID, signerKey string, req SignerRequest) (*SignerResource, error)
	AddRequirement(ctx context.Context, tenantID, envelopeID string, req RequirementRequest, rels RequirementRelationships) (*RequirementResource, error)
	ActivateEnvelope(ctx context.Context, tenantID, envelopeID string) (*EnvelopeResource, error)
	CancelEnvelope(ctx context.Context, tenantID, envelopeID string) error
	// ResendNotification triggers Clicksign to re-send the signing
	// notification. When signerKey is empty the notification fires for every
	// pending signer in the envelope; otherwise it targets the specific
	// signer. Endpoint: POST /api/v3/envelopes/{id}/notifications (envelope)
	// or POST /api/v3/envelopes/{id}/signers/{signer_key}/notifications.
	ResendNotification(ctx context.Context, tenantID, envelopeID, signerKey string) error
}

// Client uses pkg/clients.HTTPClient under the hood (circuit breaker + retries
// + OTel propagation already wired). One HTTPClient is cached per tenant —
// today there's only ever one (env-based provider) but the cache keeps the
// hot path allocation-free when we move to per-tenant credentials.
type Client struct {
	creds credential.CredentialProvider

	mu    sync.RWMutex
	cache map[string]*tenantClient
}

type tenantClient struct {
	http  *clients.HTTPClient
	creds *credential.Credentials
}

func NewClient(creds credential.CredentialProvider) *Client {
	return &Client{
		creds: creds,
		cache: make(map[string]*tenantClient),
	}
}

func (c *Client) httpFor(ctx context.Context, tenantID string) (*tenantClient, error) {
	c.mu.RLock()
	tc, ok := c.cache[tenantID]
	c.mu.RUnlock()
	if ok {
		return tc, nil
	}
	cred, err := c.creds.Get(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// Doc oficial (docs/signature/signature.md) usa o token cru — default raw.
	// Tenants que precisem do prefixo setam CLICKSIGN_AUTH_SCHEME=bearer.
	authValue := cred.AccessToken
	if cred.AuthScheme == credential.AuthSchemeBearer {
		authValue = "Bearer " + cred.AccessToken
	}
	httpC := clients.NewHTTPClient(cred.BaseURL,
		clients.WithHeaders(func(h http.Header) {
			h.Set("Authorization", authValue)
			h.Set("Accept", acceptHeader)
			h.Set("Content-Type", acceptHeader)
		}),
	)
	tc = &tenantClient{http: httpC, creds: cred}
	c.mu.Lock()
	c.cache[tenantID] = tc
	c.mu.Unlock()
	// Logamos uma vez por tenant para que `base_url` apareça nos logs do
	// serviço — sem isso, um BaseURL errado (apontando pra outro host com
	// openresty na frente) só dá pra detectar entrando no container.
	logger.Info("clicksign client initialized",
		"tenant_id", tenantID,
		"base_url", cred.BaseURL,
		"environment", cred.Environment,
		"auth_scheme", cred.AuthScheme,
	)
	return tc, nil
}

func (c *Client) CreateEnvelope(ctx context.Context, tenantID, name string) (*EnvelopeResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := wrap("envelopes", "", EnvelopeAttributes{Name: name})
	return doJSON[EnvelopeResource](ctx, tc.http, http.MethodPost, "/api/v3/envelopes", body)
}

func (c *Client) UploadDocument(ctx context.Context, tenantID, envelopeID string, req DocumentRequest) (*DocumentResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := wrap("documents", "", req)
	return doJSON[DocumentResource](ctx, tc.http, http.MethodPost,
		fmt.Sprintf("/api/v3/envelopes/%s/documents", envelopeID), body)
}

func (c *Client) AddSigner(ctx context.Context, tenantID, envelopeID string, req SignerRequest) (*SignerResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := wrap("signers", "", req)
	return doJSON[SignerResource](ctx, tc.http, http.MethodPost,
		fmt.Sprintf("/api/v3/envelopes/%s/signers", envelopeID), body)
}

func (c *Client) UpdateSigner(ctx context.Context, tenantID, envelopeID, signerKey string, req SignerRequest) (*SignerResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// PATCH requires the resource id in the JSON:API envelope.
	body := wrap("signers", signerKey, req)
	return doJSON[SignerResource](ctx, tc.http, http.MethodPatch,
		fmt.Sprintf("/api/v3/envelopes/%s/signers/%s", envelopeID, signerKey), body)
}

func (c *Client) AddRequirement(ctx context.Context, tenantID, envelopeID string, req RequirementRequest, rels RequirementRelationships) (*RequirementResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"data": map[string]any{
			"type":          "requirements",
			"attributes":    req,
			"relationships": rels,
		},
	}
	return doJSON[RequirementResource](ctx, tc.http, http.MethodPost,
		fmt.Sprintf("/api/v3/envelopes/%s/requirements", envelopeID), body)
}

func (c *Client) ActivateEnvelope(ctx context.Context, tenantID, envelopeID string) (*EnvelopeResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := wrap("envelopes", envelopeID, EnvelopeAttributes{Status: "running"})
	return doJSON[EnvelopeResource](ctx, tc.http, http.MethodPatch,
		fmt.Sprintf("/api/v3/envelopes/%s", envelopeID), body)
}

// CancelEnvelope cancela o envelope no provedor e invalida os links de
// assinatura.
//
// Na API v3 o cancelamento NÃO acontece no envelope:
//
//   - PATCH /envelopes/{id} aceita apenas `status` draft|running — mandar
//     "canceled" devolve 400 "status deve estar em: draft, running";
//   - DELETE /envelopes/{id} só vale para rascunho — num envelope em execução
//     devolve 403 "envelope não está com status draft".
//
// O caminho é por DOCUMENTO: PATCH /envelopes/{id}/documents/{doc} com
// {"status":"canceled"} — é essa chamada que leva o envelope a `canceled` e
// invalida o link. Confirmado contra o sandbox da Clicksign em 2026-10-05.
//
// Idempotente: envelope já encerrado (canceled/closed) ou documento já
// encerrado não geram chamada.
func (c *Client) CancelEnvelope(ctx context.Context, tenantID, envelopeID string) error {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return err
	}

	// Estado do envelope primeiro: um envelope já cancelado/fechado não tem o
	// que cancelar, e ler antes evita um PATCH condenado em cada tentativa do
	// operador (o botão pode ser clicado de novo depois de um cancelamento que
	// já valeu).
	env, err := c.GetEnvelope(ctx, tenantID, envelopeID)
	if err != nil {
		return err
	}
	if env != nil && isTerminalEnvelopeStatus(env.Attributes.Status) {
		return nil
	}

	docs, err := c.ListDocuments(ctx, tenantID, envelopeID)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("envelope %s has no documents to cancel", envelopeID)
	}

	path := fmt.Sprintf("/api/v3/envelopes/%s/documents", envelopeID)
	for _, doc := range docs {
		if doc.ID == "" || isTerminalEnvelopeStatus(doc.Attributes.Status) {
			continue
		}
		body := wrap("documents", doc.ID, DocumentStatusRequest{Status: "canceled"})
		if _, err := doJSON[DocumentResource](ctx, tc.http, http.MethodPatch, path+"/"+doc.ID, body); err != nil {
			return err
		}
	}
	return nil
}

// isTerminalEnvelopeStatus reconhece os estados em que o envelope/documento já
// não aceita mais interação. A Clicksign usa as mesmas palavras para os dois
// recursos ("canceled" no cancelamento manual, "closed" na conclusão).
func isTerminalEnvelopeStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "canceled", "cancelled", "closed", "finished":
		return true
	default:
		return false
	}
}

// GetEnvelope reads back the current Clicksign envelope state — useful for
// reconciliation when a webhook is missed (status drift between local DB and
// the source of truth).
func (c *Client) GetEnvelope(ctx context.Context, tenantID, envelopeID string) (*EnvelopeResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return doJSON[EnvelopeResource](ctx, tc.http, http.MethodGet,
		fmt.Sprintf("/api/v3/envelopes/%s", envelopeID), nil)
}

// ListSigners fetches every signer attached to an envelope, including the
// read-only fields (status, signed_at) Clicksign omits from GetEnvelope.
// Reconcile uses this to detect per-signer drift when individual sign
// webhooks were lost between Clicksign and toca-signature.
func (c *Client) ListSigners(ctx context.Context, tenantID, envelopeID string) ([]SignerResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out, err := doJSON[[]SignerResource](ctx, tc.http, http.MethodGet,
		fmt.Sprintf("/api/v3/envelopes/%s/signers", envelopeID), nil)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return *out, nil
}

// ListDocuments fetches every document attached to an envelope. Once the
// envelope is in `closed/finished`, Clicksign populates `signed_file_url`
// (and `download_url`) on each document — that's the canonical URL the
// persist subscriber needs to download the final PDF into Supabase.
//
// Returns an empty slice (not nil) when the envelope has no documents
// so callers can range without a nil-check.
func (c *Client) ListDocuments(ctx context.Context, tenantID, envelopeID string) ([]DocumentResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out, err := doJSON[[]DocumentResource](ctx, tc.http, http.MethodGet,
		fmt.Sprintf("/api/v3/envelopes/%s/documents", envelopeID), nil)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []DocumentResource{}, nil
	}
	return *out, nil
}

// ResendNotification asks Clicksign to deliver the signing token again.
// Empty signerKey ⇒ envelope-wide remind-all; non-empty ⇒ targets a single
// signer (operator UX: "resend to João").
func (c *Client) ResendNotification(ctx context.Context, tenantID, envelopeID, signerKey string) error {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/v3/envelopes/%s/notifications", envelopeID)
	if signerKey != "" {
		path = fmt.Sprintf("/api/v3/envelopes/%s/signers/%s/notifications", envelopeID, signerKey)
	}
	// Doc oficial (docs/signature/signature.md §8) inclui `attributes: {}` no
	// payload. Mantemos a forma exata para evitar validações JSON:API estritas.
	body := map[string]any{
		"data": map[string]any{
			"type":       "notifications",
			"attributes": map[string]any{},
		},
	}
	_, err = doJSON[notificationResource](ctx, tc.http, http.MethodPost, path, body)
	return err
}

// notificationResource is a placeholder for the notification resource returned
// by Clicksign. We don't surface any field today — the success of the POST is
// the only signal callers care about.
type notificationResource struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// wrap produces a JSON:API top-level envelope: {"data": {"type", "id"?, "attributes"}}.
// id is omitted (empty string) for POSTs and required for PATCHs.
func wrap(typeName, id string, attrs any) map[string]any {
	data := map[string]any{
		"type":       typeName,
		"attributes": attrs,
	}
	if id != "" {
		data["id"] = id
	}
	return map[string]any{"data": data}
}

// doJSON executes the request and unwraps the {data: T} envelope. Non-2xx
// responses are translated to *APIError so callers can `errors.As` and surface
// typed information (status, code, title, detail). Falls back to RawBody when
// the response isn't the standard JSON:API errors shape.
func doJSON[T any](ctx context.Context, hc *clients.HTTPClient, method, path string, body any) (*T, error) {
	resp, err := hc.Do(ctx, method, path, body)
	if err != nil {
		return nil, fmt.Errorf("clicksign %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fullURL := ""
		if resp.Request != nil && resp.Request.URL != nil {
			fullURL = resp.Request.URL.String()
		}
		return nil, parseError(method, path, fullURL, resp.StatusCode, raw)
	}
	if len(raw) == 0 {
		var empty T
		return &empty, nil
	}
	var wrap struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("clicksign decode %s %s: %w body=%s", method, path, err, string(raw))
	}
	return &wrap.Data, nil
}

// parseError tries to decode the Clicksign JSON:API errors envelope and
// returns a typed *APIError. When the body isn't valid JSON or doesn't
// contain an `errors` array, RawBody is populated so logs stay debuggable.
// fullURL is the actual URL hit (after redirects), useful when the BaseURL
// is misconfigured and the response is from an unexpected host/proxy.
func parseError(method, path, fullURL string, status int, raw []byte) error {
	apiErr := &APIError{
		Method:  method,
		URL:     fullURL,
		Path:    path,
		Status:  status,
		RawBody: string(raw),
	}
	if len(raw) == 0 {
		return apiErr
	}
	var envelope struct {
		Errors []ClicksignAPIError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Errors) > 0 {
		apiErr.Errors = envelope.Errors
	}
	return apiErr
}

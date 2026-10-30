package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SignatureEnvelopeSignerDTO mirrors the public shape of an envelope signer.
// Now includes the Clicksign per-signer widget key — toca-deal forwards it
// to the portal client so the embedded Clicksign widget can mount without
// the portal needing a separate admin call to the signature service.
type SignatureEnvelopeSignerDTO struct {
	SignerID            string `json:"signer_id"`
	Email               string `json:"email"`
	Name                string `json:"name,omitempty"`
	Status              string `json:"status"`
	SignedAt            string `json:"signed_at,omitempty"`
	RequestSignatureKey string `json:"request_signature_key,omitempty"`
}

type SignatureEnvelopeDTO struct {
	ID                 string                       `json:"id"`
	ContractID         string                       `json:"contract_id"`
	DealID             string                       `json:"deal_id,omitempty"`
	ExternalEnvelopeID string                       `json:"external_envelope_id,omitempty"`
	CreatedAt          time.Time                    `json:"created_at"`
	Environment        string                       `json:"environment"`
	Status             string                       `json:"status"`
	Signers            []SignatureEnvelopeSignerDTO `json:"signers,omitempty"`
	FinishedAt         string                       `json:"finished_at,omitempty"`
}

type CreateSignatureEnvelopeRequest struct {
	SignerAuth        map[string][]string               `json:"signer_auth,omitempty"`
	SignerGroups      map[string]int                    `json:"signer_groups,omitempty"`
	AttachmentDocIDs  []string                          `json:"attachment_document_ids,omitempty"`
	InlineAttachments []CreateSignatureInlineAttachment `json:"inline_attachments,omitempty"`
	Locale            string                            `json:"locale,omitempty"`
	Deadline          string                            `json:"deadline,omitempty"`
}

// CreateSignatureInlineAttachment carries a one-off PDF the operator
// attached at envelope creation time. ContentBase64 is the standard
// base64 payload (no `data:` prefix); toca-signature decodes it.
type CreateSignatureInlineAttachment struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

type SignatureClient struct {
	BaseClient *BaseClient
}

func NewSignatureClient() *SignatureClient {
	baseClient := NewBaseClient(os.Getenv("SIGNATURE_SERVICE_URL"))
	// Creating a Clicksign envelope renders the document and performs several
	// provider calls. The generic 5s internal timeout is too aggressive here.
	baseClient.HTTPClient.Timeout = 15 * time.Second
	return &SignatureClient{
		BaseClient: baseClient,
	}
}

// GetByContract returns the signature envelope for a given contract or nil if
// none exists. The signature service exposes this on an internal route guarded
// by X-Internal-Key.
func (c *SignatureClient) GetByContract(ctx context.Context, tenantID, contractID string) (*SignatureEnvelopeDTO, error) {
	path := fmt.Sprintf("/api/v1/signature/internal/envelopes/by-contract/%s?tenant_id=%s", contractID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, signatureServiceError(resp)
	}

	var response struct {
		Data SignatureEnvelopeDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// RefreshSignedDocumentURL hits Clicksign (via toca-signature) for the
// freshest signed-PDF URL on a contract's envelope. Returns the URL
// (which may be empty when the file genuinely isn't ready yet) or an
// error for transport-level failures. 404 from the upstream surfaces
// as nil URL + nil error — there's no envelope to query, treat as "no
// URL available" so the backfill skips and moves on.
//
// Used by DocumentService.BackfillUnstoredSignedPDFs as the recovery
// path when `contract.SignedDocumentURL` is empty (Phase 3 reconcile
// gap fix). Idempotent on the server side.
func (c *SignatureClient) RefreshSignedDocumentURL(ctx context.Context, tenantID, contractID string) (string, error) {
	path := fmt.Sprintf("/api/v1/signature/internal/envelopes/by-contract/%s/refresh-signed-url?tenant_id=%s", contractID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return "", err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", signatureServiceError(resp)
	}
	var response struct {
		Data struct {
			SignedDocumentURL string `json:"signed_document_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	return response.Data.SignedDocumentURL, nil
}

// CreateForContract synchronously creates (or returns the already-active)
// Clicksign envelope for a contract. The endpoint is idempotent in
// toca-signature, so callers may use it as an immediate path while still
// publishing signature.requested for asynchronous retry/observability.
func (c *SignatureClient) CreateForContract(ctx context.Context, tenantID, contractID string, req CreateSignatureEnvelopeRequest) (*SignatureEnvelopeDTO, error) {
	path := fmt.Sprintf("/api/v1/signature/internal/envelopes/by-contract/%s?tenant_id=%s", contractID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, signatureServiceError(resp)
	}

	var response struct {
		Data SignatureEnvelopeDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func signatureServiceError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(raw))
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &response); err == nil && strings.TrimSpace(response.Message) != "" {
		message = strings.TrimSpace(response.Message)
	}
	if message == "" {
		return fmt.Errorf("signature service returned status: %d", resp.StatusCode)
	}
	return fmt.Errorf("signature service returned status %d: %s", resp.StatusCode, message)
}

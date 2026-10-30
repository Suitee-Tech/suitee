package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"
)

// ContractSignatoryDTO mirrors toca-deal/internal/domain/contract.Signatory.
// Only fields actually consumed by other services are listed; extra fields in
// the JSON payload are ignored by the decoder.
type ContractSignatoryDTO struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Email              string   `json:"email"`
	Role               string   `json:"role"`
	PhoneNumber        string   `json:"phone_number,omitempty"`
	Documentation      string   `json:"documentation,omitempty"`
	Birthday           string   `json:"birthday,omitempty"`
	ClicksignSignerKey *string  `json:"clicksign_signer_key,omitempty"`
	AuthMethods        []string `json:"auth_methods,omitempty"`
	SignedAt           *string  `json:"signed_at,omitempty"`
	// Group is the default Clicksign signing-order group persisted on the
	// contract. Per-envelope overrides at create time take precedence.
	Group int `json:"group,omitempty"`
}

type ContractDTO struct {
	ID                 string                 `json:"id"`
	DisplayID          string                 `json:"display_id,omitempty"`
	DealID             string                 `json:"deal_id,omitempty"`
	Type               string                 `json:"type"`
	Status             string                 `json:"status,omitempty"`
	StartDate          time.Time              `json:"start_date"`
	EndDate            *time.Time             `json:"end_date,omitempty"`
	TenantID           string                 `json:"tenant_id"`
	Content            string                 `json:"content,omitempty"`
	Signatories        []ContractSignatoryDTO `json:"signatories,omitempty"`
	// Denormalized references that travel with the contract for
	// presentation/PDF rendering without forcing a separate property lookup.
	PropertyID    string `json:"property_id,omitempty"`
	PropertyTitle string `json:"property_title,omitempty"`
	Client        string `json:"client,omitempty"`
	// Optional per-contract financial rules. Zero values mean "use system
	// defaults" — both the delinquency engine and the transfer (repasse)
	// pipeline read these and fall back when unset.
	Value              float64 `json:"value,omitempty"`
	CommissionPct      float64 `json:"commission_pct,omitempty"`
	LateFeePct         float64 `json:"late_fee_pct,omitempty"`
	MonthlyInterestPct float64 `json:"monthly_interest_pct,omitempty"`
}

// DealBrokerParticipationDTO mirrors the subset of
// toca-deal/internal/domain/deal.BrokerParticipation that
// toca-financial consumes when projecting commissions in the preview.
type DealBrokerParticipationDTO struct {
	BrokerID   string  `json:"broker_id"`
	Role       string  `json:"role"`
	Percentage float64 `json:"percentage"`
}

// DealDTO is the read-only snapshot of a Deal consumed by
// toca-financial's FinancialPreviewService. Only fields actually
// required to project charges + commissions + transfers are listed;
// extra fields in the JSON payload are ignored. The shape mirrors the
// canonical deal.Deal struct exposed at
// GET /api/v1/deal/internal/deals/:id.
type DealDTO struct {
	ID                   string                       `json:"id"`
	TenantID             string                       `json:"tenant_id"`
	Type                 string                       `json:"type"`
	Status               string                       `json:"status,omitempty"`
	Value                float64                      `json:"value"`
	CommissionPct        float64                      `json:"commission,omitempty"`
	CustomerID           string                       `json:"customer_id"`
	OwnerID              string                       `json:"owner_id"`
	PropertyID           string                       `json:"property_id"`
	StartDate            *time.Time                   `json:"start_date,omitempty"`
	EndDate              *time.Time                   `json:"end_date,omitempty"`
	BrokerParticipations []DealBrokerParticipationDTO `json:"broker_participations,omitempty"`
}

type DealClient struct {
	BaseClient *BaseClient
}

func NewDealClient() *DealClient {
	return &DealClient{
		BaseClient: NewBaseClient(os.Getenv("DEAL_SERVICE_URL")),
	}
}

// GetDeal fetches a Deal via the internal endpoint
// GET /api/v1/deal/internal/deals/:id. Bypassa broker scope; usado
// pelo FinancialPreviewService (toca-financial) para projetar charges
// + comissões + repasses antes da assinatura.
func (c *DealClient) GetDeal(ctx context.Context, tenantID, dealID string) (*DealDTO, error) {
	path := fmt.Sprintf("/api/v1/deal/internal/deals/%s?tenant_id=%s", dealID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deal service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data DealDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func (c *DealClient) GetContract(ctx context.Context, tenantID, contractID string) (*ContractDTO, error) {
	path := fmt.Sprintf("/api/v1/deal/internal/contracts/%s?tenant_id=%s", contractID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deal service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data ContractDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// FetchDocumentContent pulls the raw bytes of a Document already persisted in
// toca-deal so toca-signature can upload it as a Clicksign envelope
// attachment. Returns the filename suggested by Content-Disposition (used as
// the Clicksign filename) and the body. Only application/pdf is accepted —
// the deal handler rejects non-PDF documents with 415.
func (c *DealClient) FetchDocumentContent(ctx context.Context, tenantID, documentID string) (string, []byte, error) {
	path := fmt.Sprintf("/api/v1/deal/internal/documents/%s/content?tenant_id=%s", documentID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("deal service returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read body: %w", err)
	}
	filename := documentID + ".pdf"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if name := params["filename"]; name != "" {
				filename = name
			}
		}
	}
	return filename, body, nil
}

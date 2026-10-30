package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// LeadDTO is the lean projection of a CRM lead used by other services.
// We deliberately keep this narrow — service-to-service callers don't need
// the full lead document, only enough to render names, send messages, or
// link back to the lead detail page.
type LeadDTO struct {
	ID            string `json:"id"`
	DisplayID     string `json:"display_id,omitempty"`
	Name          string `json:"name"`
	Email         string `json:"email,omitempty"`
	Phone         string `json:"phone,omitempty"`
	Documentation string `json:"documentation,omitempty"` // CPF/CNPJ
	Role          string `json:"role,omitempty"`
	// AccountID is the Supabase user id when the lead has been onboarded
	// to the client portal. Required by the comm service to push in-app
	// notifications.
	AccountID string `json:"account_id,omitempty"`
}

type CrmClient struct {
	BaseClient *BaseClient
}

func NewCrmClient() *CrmClient {
	return &CrmClient{
		BaseClient: NewBaseClient(os.Getenv("CRM_SERVICE_URL")),
	}
}

// GetLead fetches a single lead by id from the CRM service via the dedicated
// internal route (X-Internal-Key auth). Other consumers that already have a
// JWT should hit /api/v1/leads/:id directly instead.
func (c *CrmClient) GetLead(ctx context.Context, tenantID, leadID string) (*LeadDTO, error) {
	if leadID == "" {
		return nil, fmt.Errorf("lead_id is required")
	}
	path := fmt.Sprintf("/api/v1/crm/internal/leads/%s?tenant_id=%s", leadID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crm service returned status: %d", resp.StatusCode)
	}
	var response struct {
		Data LeadDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// PropertyAddressDTO é a projeção do endereço do imóvel que o toca-financial
// consome para montar a DIMOB (registro R02: endereço, CEP, UF, município).
type PropertyAddressDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	PropertyType string `json:"property_type"`
	Address      struct {
		Street       string `json:"street"`
		Number       string `json:"number"`
		Complement   string `json:"complement"`
		Neighborhood string `json:"neighborhood"`
		City         string `json:"city"`
		StateCode    string `json:"stateCode"`
		PostalCode   string `json:"postalCode"`
		Formatted    string `json:"formatted_address"`
	} `json:"address"`
}

// PropertyClient lê imóveis do toca-core pela rota interna
// GET /api/v1/core/internal/properties/:id (credencial interna).
type PropertyClient struct {
	BaseClient *BaseClient
}

func NewPropertyClient() *PropertyClient {
	return &PropertyClient{
		BaseClient: NewBaseClient(os.Getenv("CORE_SERVICE_URL")),
	}
}

// GetProperty busca o imóvel de um tenant pela rota interna do toca-core.
func (c *PropertyClient) GetProperty(ctx context.Context, tenantID, propertyID string) (*PropertyAddressDTO, error) {
	path := fmt.Sprintf("/api/v1/core/internal/properties/%s?tenant_id=%s", propertyID, tenantID)
	// DoWithTenant: o middleware global do core exige X-Tenant-Id quando a
	// chamada se identifica com X-Internal-Key (sem o header → 401).
	resp, err := c.BaseClient.DoWithTenant(ctx, http.MethodGet, path, nil, tenantID)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("core service returned status: %d", resp.StatusCode)
	}

	var envelope struct {
		Data PropertyAddressDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

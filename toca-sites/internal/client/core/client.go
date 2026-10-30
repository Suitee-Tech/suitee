// Package core lê a projeção do imóvel no toca-core pela rota interna.
//
// O evento `property.updated` carrega apenas ids: preço, mídia e status podem
// ter mudado depois do publish. Reler o agregado é o que mantém a vitrine
// consistente (o Payload nunca é source of truth). A listagem interna existe
// para o backfill quando o site é ligado com estoque já existente.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// ErrNotFound indica imóvel inexistente/removido. O sync traduz para
// "unpublish" em vez de erro: o RabbitMQ não deve reentregar para sempre um
// imóvel que deixou de existir.
var ErrNotFound = errors.New("property not found in core")

type Client struct {
	baseURL     string
	internalKey string
	http        *http.Client
}

func New(baseURL, internalKey string) *Client {
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		internalKey: internalKey,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

// property espelha o recorte do agregado `property.Property` que a vitrine
// publica. Campos não usados ficam de fora de propósito.
type property struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Status            string `json:"status"`
	PublicationStatus string `json:"publication_status"`
	PropertyType      string `json:"property_type"`
	ListingType       string `json:"listing_type"`
	Address           struct {
		Street       string `json:"street"`
		Number       string `json:"number"`
		Complement   string `json:"complement"`
		Neighborhood string `json:"neighborhood"`
		City         string `json:"city"`
		State        string `json:"state"`
		StateCode    string `json:"stateCode"`
		PostalCode   string `json:"postalCode"`
	} `json:"address"`
	Pricing struct {
		SalePrice      *money `json:"sale_price"`
		RentPrice      *money `json:"rent_price"`
		CondominiumFee *money `json:"condominium_fee"`
		PropertyTax    *money `json:"property_tax"`
	} `json:"pricing"`
	Areas struct {
		TotalAreaM2  float64  `json:"total_area_m2"`
		UsableAreaM2 *float64 `json:"usable_area_m2"`
		BuiltAreaM2  *float64 `json:"built_area_m2"`
	} `json:"areas"`
	Features struct {
		Bedrooms      *int `json:"bedrooms"`
		Bathrooms     *int `json:"bathrooms"`
		Suites        *int `json:"suites"`
		ParkingSpaces *int `json:"parking_spaces"`
	} `json:"features"`
	Amenities []string `json:"amenities"`
	Media     struct {
		Images []struct {
			URL         string `json:"url"`
			Description string `json:"description"`
			Order       int    `json:"order"`
		} `json:"images"`
	} `json:"media"`
}

type propertyResponse struct {
	Data property `json:"data"`
}

type propertyListResponse struct {
	Data []property `json:"data"`
	Meta struct {
		Total int64 `json:"total"`
	} `json:"meta"`
}

type money struct {
	CurrentAmount float64 `json:"current_amount"`
	Currency      string  `json:"currency"`
}

func (c *Client) GetProperty(ctx context.Context, tenantID, propertyID string) (*site.PropertySnapshot, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("CORE_SERVICE_URL is not configured")
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(propertyID) == "" {
		return nil, fmt.Errorf("tenant and property id are required")
	}

	endpoint := fmt.Sprintf("%s/api/v1/core/internal/properties/%s?tenant_id=%s",
		c.baseURL, url.PathEscape(propertyID), url.QueryEscape(tenantID))
	resp, err := c.get(ctx, endpoint, tenantID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.statusError(resp)
	}

	var out propertyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode core property: %w", err)
	}
	if strings.TrimSpace(out.Data.ID) == "" {
		return nil, fmt.Errorf("core returned an empty property")
	}
	return snapshotFrom(out.Data), nil
}

// ListProperties pagina o estoque do tenant para o backfill da vitrine. A rota
// interna devolve o mesmo recorte do GetProperty.
func (c *Client) ListProperties(ctx context.Context, tenantID string, limit, offset int) ([]*site.PropertySnapshot, int64, error) {
	if c.baseURL == "" {
		return nil, 0, fmt.Errorf("CORE_SERVICE_URL is not configured")
	}
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, fmt.Errorf("tenant is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	endpoint := fmt.Sprintf("%s/api/v1/core/internal/properties?tenant_id=%s&limit=%s&offset=%s",
		c.baseURL, url.QueryEscape(tenantID), strconv.Itoa(limit), strconv.Itoa(offset))
	resp, err := c.get(ctx, endpoint, tenantID)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, c.statusError(resp)
	}

	var out propertyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("decode core property list: %w", err)
	}
	snapshots := make([]*site.PropertySnapshot, 0, len(out.Data))
	for _, p := range out.Data {
		if strings.TrimSpace(p.ID) == "" {
			continue
		}
		snapshots = append(snapshots, snapshotFrom(p))
	}
	return snapshots, out.Meta.Total, nil
}

func (c *Client) get(ctx context.Context, endpoint, tenantID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)
	req.Header.Set("X-Tenant-Id", tenantID)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("core request failed: %w", err)
	}
	return resp, nil
}

func (c *Client) statusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return fmt.Errorf("core returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func snapshotFrom(p property) *site.PropertySnapshot {
	snapshot := &site.PropertySnapshot{
		ID:                p.ID,
		Title:             p.Title,
		Description:       p.Description,
		Status:            p.Status,
		PublicationStatus: p.PublicationStatus,
		PropertyType:      p.PropertyType,
		ListingType:       p.ListingType,
		Address: site.Address{
			Street:       p.Address.Street,
			Number:       p.Address.Number,
			Complement:   p.Address.Complement,
			Neighborhood: p.Address.Neighborhood,
			City:         p.Address.City,
			State:        stateOrCode(p.Address.State, p.Address.StateCode),
			PostalCode:   p.Address.PostalCode,
		},
		Features: site.Features{
			Bedrooms:      p.Features.Bedrooms,
			Bathrooms:     p.Features.Bathrooms,
			Suites:        p.Features.Suites,
			ParkingSpaces: p.Features.ParkingSpaces,
		},
		Amenities: p.Amenities,
		AreaTotal: p.Areas.TotalAreaM2,
		AreaBuilt: builtArea(p.Areas.BuiltAreaM2, p.Areas.UsableAreaM2),
		// O core não expõe updated_at no agregado; o instante do sync é a
		// melhor aproximação e é suficiente para o ISR.
		UpdatedAt: time.Now().UTC(),
	}
	snapshot.SalePrice = amount(p.Pricing.SalePrice)
	snapshot.RentPrice = amount(p.Pricing.RentPrice)
	snapshot.CondoFee = amount(p.Pricing.CondominiumFee)
	snapshot.PropertyTax = amount(p.Pricing.PropertyTax)
	for _, img := range p.Media.Images {
		if strings.TrimSpace(img.URL) == "" {
			continue
		}
		snapshot.Images = append(snapshot.Images, site.Image{
			URL:         img.URL,
			Description: img.Description,
			Order:       img.Order,
		})
	}
	return snapshot
}

// builtArea usa a área construída e, na falta dela, a área útil. O site exibe
// uma única metragem no card.
func builtArea(built, usable *float64) float64 {
	if built != nil && *built > 0 {
		return *built
	}
	if usable != nil && *usable > 0 {
		return *usable
	}
	return 0
}

func amount(m *money) float64 {
	if m == nil {
		return 0
	}
	return m.CurrentAmount
}

// stateOrCode prefere a UF curta (o Payload limita o campo a 2 chars e o
// card/JSON-LD usam a sigla). Nome completo é traduzido; desconhecido vira
// vazio em vez de estourar a validação do CMS.
func stateOrCode(state, code string) string {
	if c := strings.ToUpper(strings.TrimSpace(code)); len(c) == 2 {
		return c
	}
	return brazilianUF(state)
}

var brazilianUFs = map[string]string{
	"acre": "AC", "alagoas": "AL", "amapá": "AP", "amapa": "AP", "amazonas": "AM",
	"bahia": "BA", "ceará": "CE", "ceara": "CE", "distrito federal": "DF",
	"espírito santo": "ES", "espirito santo": "ES", "goiás": "GO", "goias": "GO",
	"maranhão": "MA", "maranhao": "MA", "mato grosso": "MT", "mato grosso do sul": "MS",
	"minas gerais": "MG", "pará": "PA", "para": "PA", "paraíba": "PB", "paraiba": "PB",
	"paraná": "PR", "parana": "PR", "pernambuco": "PE", "piauí": "PI", "piaui": "PI",
	"rio de janeiro": "RJ", "rio grande do norte": "RN", "rio grande do sul": "RS",
	"rondônia": "RO", "rondonia": "RO", "roraima": "RR", "santa catarina": "SC",
	"são paulo": "SP", "sao paulo": "SP", "sergipe": "SE", "tocantins": "TO",
}

func brazilianUF(state string) string {
	return brazilianUFs[strings.ToLower(strings.TrimSpace(state))]
}

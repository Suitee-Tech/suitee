// Package crm cria o lead do site no toca-crm por HTTP interno.
//
// O CRM é a fonte de verdade do lead. O toca-sites grava o espelho local antes
// da chamada para que uma indisponibilidade do CRM não perca o contato — o
// mesmo desenho do webhook do toca-portal.
package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// ErrUnavailable separa "CRM fora do ar" (lead fica failed, evento ainda é
// publicado) de erro de payload (não adianta reenviar).
var ErrUnavailable = errors.New("crm unavailable")

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

type createLeadRequest struct {
	Name             string                 `json:"name"`
	Email            string                 `json:"email"`
	Phone            string                 `json:"phone"`
	Source           string                 `json:"source"`
	PropertyInterest *propertyInterest      `json:"property_interest,omitempty"`
	Qualification    map[string]interface{} `json:"qualification,omitempty"`

	RecommendedPropertyID       string               `json:"recommended_property_id,omitempty"`
	RecommendedPropertySnapshot *recommendedSnapshot `json:"recommended_property_snapshot,omitempty"`
}

type propertyInterest struct {
	Type              string            `json:"type,omitempty"`
	PreferredLocation string            `json:"preferred_location,omitempty"`
	PriceRange        *priceRange       `json:"price_range,omitempty"`
	Features          *propertyFeatures `json:"features,omitempty"`
	Notes             string            `json:"notes,omitempty"`
}

type priceRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type recommendedSnapshot struct {
	ID           string   `json:"id,omitempty"`
	Title        string   `json:"title,omitempty"`
	Neighborhood string   `json:"neighborhood,omitempty"`
	City         string   `json:"city,omitempty"`
	State        string   `json:"state,omitempty"`
	SalePrice    float64  `json:"sale_price,omitempty"`
	RentPrice    float64  `json:"rent_price,omitempty"`
	ListingType  string   `json:"listing_type,omitempty"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	MediaImages  []string `json:"media_images,omitempty"`
}

type propertyFeatures struct {
	Bedrooms  *int     `json:"bedrooms,omitempty"`
	Bathrooms *int     `json:"bathrooms,omitempty"`
	Parking   *int     `json:"parking,omitempty"`
	AreaMin   *float64 `json:"area_min,omitempty"`
}

func crmPropertyType(propertyType string) string {
	switch strings.ToLower(strings.TrimSpace(propertyType)) {
	case "apartment":
		return "APARTMENT"
	case "house":
		return "HOUSE"
	case "land":
		return "LAND"
	case "commercial":
		return "COMMERCIAL"
	case "":
		return ""
	default:
		return "OTHER"
	}
}

func propertyInterestFromLead(l site.Lead) *propertyInterest {
	pc := l.PropertyContext
	if pc == nil || pc.PropertyID == "" {
		return nil
	}
	pi := &propertyInterest{
		Type:              crmPropertyType(pc.PropertyType),
		PreferredLocation: strings.Trim(strings.Join([]string{pc.Neighborhood, pc.City, pc.State}, ", "), ", "),
		Notes:             "Imóvel do site: " + strings.TrimSpace(pc.Title) + " (" + pc.PropertyID + ")",
	}
	price := pc.SalePrice
	if strings.EqualFold(pc.ListingType, "rent") && pc.RentPrice > 0 {
		price = pc.RentPrice
	}
	if price > 0 {
		pi.PriceRange = &priceRange{Min: 0, Max: price}
	}
	if pc.Bedrooms != nil || pc.Bathrooms != nil || pc.ParkingSpaces != nil || pc.AreaTotal > 0 {
		area := pc.AreaTotal
		pi.Features = &propertyFeatures{
			Bedrooms: pc.Bedrooms, Bathrooms: pc.Bathrooms, Parking: pc.ParkingSpaces,
		}
		if area > 0 {
			pi.Features.AreaMin = &area
		}
	}
	return pi
}

func (c *Client) CreateLead(ctx context.Context, tenantID string, l site.Lead) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("%w: CRM_SERVICE_URL is not configured", ErrUnavailable)
	}
	if strings.TrimSpace(tenantID) == "" {
		return "", fmt.Errorf("tenant is required")
	}

	reqPayload := createLeadRequest{
		Name:  l.Name,
		Email: l.Email,
		Phone: l.Phone,
		// WEBSITE é o source canônico para lead do site da imobiliária; não
		// existe enum específico de vitrine no CRM e criar um valor novo
		// quebraria os relatórios por origem.
		Source:           "WEBSITE",
		PropertyInterest: propertyInterestFromLead(l),
	}
	if pc := l.PropertyContext; pc != nil && pc.PropertyID != "" {
		thumbnail := ""
		if len(pc.MediaImages) > 0 {
			thumbnail = pc.MediaImages[0]
		}
		reqPayload.RecommendedPropertyID = pc.PropertyID
		reqPayload.RecommendedPropertySnapshot = &recommendedSnapshot{
			ID: pc.PropertyID, Title: pc.Title, Neighborhood: pc.Neighborhood,
			City: pc.City, State: pc.State, SalePrice: pc.SalePrice, RentPrice: pc.RentPrice,
			ListingType: pc.ListingType, ThumbnailURL: thumbnail, MediaImages: pc.MediaImages,
		}
	}
	if msg := strings.TrimSpace(l.Message); msg != "" {
		reqPayload.Qualification = map[string]interface{}{"site_message": msg}
	}
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/leads", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)
	req.Header.Set("X-Tenant-Id", tenantID)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		// 5xx é indisponibilidade; 4xx é payload/tenant inválido e não deve
		// ser reenviado.
		if resp.StatusCode >= 500 {
			return "", fmt.Errorf("%w: status %d: %s", ErrUnavailable, resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return "", fmt.Errorf("crm rejected lead (status %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode crm response: %w", err)
	}
	if strings.TrimSpace(out.Data.ID) == "" {
		return "", fmt.Errorf("%w: crm returned an empty lead id", ErrUnavailable)
	}
	return out.Data.ID, nil
}

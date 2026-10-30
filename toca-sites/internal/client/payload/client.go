// Package payload escreve a projeção pública no Payload CMS (toca-web).
//
// O toca-sites é o dono da orquestração; o Payload é uma projeção, nunca source
// of truth. Toda chamada usa a API interna (`X-Internal-Key` +
// `X-Tenant-Id`), porque o admin do Payload não é o painel da imobiliária.
package payload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// ErrNotFound é devolvido quando o Payload não tem o doc esperado (ex.: doc id
// local ficou obsoleto depois de um reset do CMS). O caller recria.
var ErrNotFound = errors.New("payload document not found")

type Client struct {
	baseURL     string
	internalKey string
	hostSuffix  string
	http        *http.Client
}

func New(baseURL, internalKey, hostSuffix string) *Client {
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		internalKey: internalKey,
		hostSuffix:  strings.Trim(strings.TrimSpace(hostSuffix), "."),
		http:        &http.Client{Timeout: 20 * time.Second},
	}
}

// Configured informa se há Payload configurado. Em dev sem Payload o serviço
// continua aceitando o cadastro do site e a projeção local (o painel não fica
// refém do CMS para ligar a vitrine).
func (c *Client) Configured() bool { return c.baseURL != "" }

// ---------------------------------------------------------------------------
// Tenant
// ---------------------------------------------------------------------------

func (c *Client) EnsureTenant(ctx context.Context, s *site.Site) (string, error) {
	existingID, err := c.findTenantID(ctx, s.TenantID)
	if err != nil {
		return "", err
	}

	body := map[string]interface{}{
		"suiteeTenantId": s.TenantID,
		"slug":           s.Slug,
		"name":           s.Name,
		"enabled":        s.Enabled,
		"domains": []map[string]string{
			{"domain": c.publicHost(s.Slug)},
		},
		"branding": map[string]string{
			"logoUrl":      s.Branding.LogoURL,
			"coverUrl":     s.Branding.CoverURL,
			"primaryColor": s.Branding.PrimaryColor,
			"accentColor":  s.Branding.AccentColor,
			"theme":        s.Branding.Theme,
			"tagline":      truncateRunes(s.Branding.Tagline, 160),
		},
		"contact": map[string]string{
			"email":       truncateRunes(s.Contact.Email, 160),
			"phone":       truncateRunes(s.Contact.Phone, 40),
			"whatsapp":    truncateRunes(s.Contact.WhatsApp, 40),
			"addressLine": truncateRunes(s.Contact.AddressLine, 180),
			"city":        truncateRunes(s.Contact.City, 100),
			"state":       truncateRunes(s.Contact.State, 60),
			"creci":       truncateRunes(s.Contact.CRECI, 40),
		},
	}

	if existingID != "" {
		var out docResponse
		if err := c.do(ctx, http.MethodPatch, "/api/tenants/"+existingID, s.TenantID, body, &out); err != nil {
			if errors.Is(err, ErrNotFound) {
				// Doc removido entre o GET e o PATCH: cai para criação.
				return c.createTenant(ctx, s, body)
			}
			return "", err
		}
		return idOf(out), nil
	}
	return c.createTenant(ctx, s, body)
}

func (c *Client) createTenant(ctx context.Context, s *site.Site, body map[string]interface{}) (string, error) {
	var out docResponse
	if err := c.do(ctx, http.MethodPost, "/api/tenants", s.TenantID, body, &out); err != nil {
		return "", err
	}
	id := idOf(out)
	if id == "" {
		return "", fmt.Errorf("payload returned an empty tenant id")
	}
	return id, nil
}

func (c *Client) findTenantID(ctx context.Context, tenantID string) (string, error) {
	query := url.Values{}
	query.Set("where[suiteeTenantId][equals]", tenantID)
	query.Set("limit", "1")
	query.Set("depth", "0")
	var out listResponse
	if err := c.do(ctx, http.MethodGet, "/api/tenants?"+query.Encode(), tenantID, nil, &out); err != nil {
		return "", err
	}
	if len(out.Docs) == 0 {
		return "", nil
	}
	return idOfRaw(out.Docs[0].ID), nil
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

type listingPayload struct {
	Tenant          string          `json:"tenant"`
	PropertyID      string          `json:"propertyId"`
	SeoSlug         string          `json:"seoSlug"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	ListingType     string          `json:"listingType"`
	PropertyType    string          `json:"propertyType"`
	SalePrice       float64         `json:"salePrice"`
	RentPrice       float64         `json:"rentPrice"`
	CondoFee        float64         `json:"condoFee"`
	PropertyTax     float64         `json:"propertyTax"`
	Address         listingAddress  `json:"address"`
	Features        listingFeatures `json:"features"`
	Amenities       []string        `json:"amenities"`
	Images          []listingImage  `json:"images"`
	Status          string          `json:"status"`
	PublishedAt     string          `json:"publishedAt,omitempty"`
	SourceUpdatedAt string          `json:"sourceUpdatedAt,omitempty"`
}

type listingAddress struct {
	Street       string `json:"street,omitempty"`
	Number       string `json:"number,omitempty"`
	Complement   string `json:"complement,omitempty"`
	Neighborhood string `json:"neighborhood,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	PostalCode   string `json:"postalCode,omitempty"`
}

type listingFeatures struct {
	Bedrooms      *int    `json:"bedrooms,omitempty"`
	Bathrooms     *int    `json:"bathrooms,omitempty"`
	Suites        *int    `json:"suites,omitempty"`
	ParkingSpaces *int    `json:"parkingSpaces,omitempty"`
	AreaTotal     float64 `json:"areaTotal,omitempty"`
	AreaBuilt     float64 `json:"areaBuilt,omitempty"`
}

type listingImage struct {
	URL   string `json:"url"`
	Alt   string `json:"alt,omitempty"`
	Order int    `json:"order"`
}

func (c *Client) UpsertListing(ctx context.Context, s *site.Site, l *site.Listing) (string, error) {
	tenantDocID := s.PayloadTenantID
	if tenantDocID == "" {
		id, err := c.EnsureTenant(ctx, s)
		if err != nil {
			return "", fmt.Errorf("ensure payload tenant: %w", err)
		}
		tenantDocID = id
	}

	body := listingPayload{
		Tenant:       tenantDocID,
		PropertyID:   l.PropertyID,
		SeoSlug:      l.SeoSlug,
		Title:        truncateRunes(l.Title, 180),
		Description:  l.Description,
		ListingType:  l.ListingType,
		PropertyType: l.PropertyType,
		SalePrice:    l.SalePrice,
		RentPrice:    l.RentPrice,
		CondoFee:     l.CondoFee,
		PropertyTax:  l.PropertyTax,
		Address: listingAddress{
			Street:       l.Address.Street,
			Number:       l.Address.Number,
			Complement:   l.Address.Complement,
			Neighborhood: l.Address.Neighborhood,
			City:         l.Address.City,
			State:        l.Address.State,
			PostalCode:   l.Address.PostalCode,
		},
		Features: listingFeatures{
			Bedrooms:      l.Features.Bedrooms,
			Bathrooms:     l.Features.Bathrooms,
			Suites:        l.Features.Suites,
			ParkingSpaces: l.Features.ParkingSpaces,
			AreaTotal:     l.Features.AreaTotal,
			AreaBuilt:     l.Features.AreaBuilt,
		},
		Amenities:       append([]string{}, l.Amenities...),
		Images:          toImages(l.Images),
		Status:          string(l.Status),
		SourceUpdatedAt: l.SourceUpdatedAt.UTC().Format(time.RFC3339),
	}
	if l.PublishedAt != nil {
		body.PublishedAt = l.PublishedAt.UTC().Format(time.RFC3339)
	}

	// Doc id local é o caminho feliz; o Payload pode ter sido resetado, então
	// um 404 cai para busca/criação.
	if l.PayloadDocID != "" {
		var out docResponse
		err := c.do(ctx, http.MethodPatch, "/api/listings/"+l.PayloadDocID, s.TenantID, body, &out)
		if err == nil {
			return idOf(out), nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}

	existingID, err := c.findListingID(ctx, s, l.PropertyID)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		var out docResponse
		if err := c.do(ctx, http.MethodPatch, "/api/listings/"+existingID, s.TenantID, body, &out); err != nil {
			return "", err
		}
		return idOf(out), nil
	}

	var out docResponse
	if err := c.do(ctx, http.MethodPost, "/api/listings", s.TenantID, body, &out); err != nil {
		return "", err
	}
	id := idOf(out)
	if id == "" {
		return "", fmt.Errorf("payload returned an empty listing id")
	}
	return id, nil
}

func (c *Client) UnpublishListing(ctx context.Context, s *site.Site, l *site.Listing) error {
	docID := l.PayloadDocID
	if docID == "" {
		// Sem doc id local (crash entre o create e o save) ainda é possível
		// existir uma listing publicada: procura antes de desistir, senão o
		// anúncio fica no ar no CMS mesmo despublicado no core.
		found, err := c.findListingID(ctx, s, l.PropertyID)
		if err != nil {
			return err
		}
		if found == "" {
			return nil
		}
		docID = found
	}
	body := map[string]string{"status": string(site.ListingUnpublished)}
	var out docResponse
	if err := c.do(ctx, http.MethodPatch, "/api/listings/"+docID, s.TenantID, body, &out); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) findListingID(ctx context.Context, s *site.Site, propertyID string) (string, error) {
	query := url.Values{}
	query.Set("where[propertyId][equals]", propertyID)
	query.Set("where[tenant.suiteeTenantId][equals]", s.TenantID)
	query.Set("limit", "1")
	query.Set("depth", "0")
	var out listResponse
	if err := c.do(ctx, http.MethodGet, "/api/listings?"+query.Encode(), s.TenantID, nil, &out); err != nil {
		return "", err
	}
	if len(out.Docs) == 0 {
		return "", nil
	}
	return idOfRaw(out.Docs[0].ID), nil
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

func (c *Client) UpsertPage(ctx context.Context, s *site.Site, p site.Page) error {
	tenantDocID := s.PayloadTenantID
	if tenantDocID == "" {
		id, err := c.EnsureTenant(ctx, s)
		if err != nil {
			return fmt.Errorf("ensure payload tenant: %w", err)
		}
		tenantDocID = id
	}

	body := map[string]interface{}{
		"tenant":   tenantDocID,
		"pageKey":  string(p.Key),
		"title":    truncateRunes(p.Title, 140),
		"subtitle": truncateRunes(p.Subtitle, 320),
		"body":     p.BodyText(),
		"status":   "published",
	}
	// A collection usa o grupo `seo`. Enviar o grupo mesmo vazio é obrigatório
	// para LIMPAR um SEO já publicado no Payload; antes, apagar os campos no
	// painel mantinha o metaTitle antigo no ar.
	body["seo"] = map[string]string{
		"metaTitle":       truncateRunes(p.SEOTitle, 70),
		"metaDescription": truncateRunes(p.SEODescription, 180),
	}

	existingID, err := c.findPageID(ctx, s, p.Key)
	if err != nil {
		return err
	}
	if existingID != "" {
		var out docResponse
		return c.do(ctx, http.MethodPatch, "/api/pages/"+existingID, s.TenantID, body, &out)
	}
	var out docResponse
	if err := c.do(ctx, http.MethodPost, "/api/pages", s.TenantID, body, &out); err != nil {
		return err
	}
	return nil
}

func (c *Client) findPageID(ctx context.Context, s *site.Site, key site.PageKey) (string, error) {
	query := url.Values{}
	query.Set("where[tenant.suiteeTenantId][equals]", s.TenantID)
	query.Set("where[pageKey][equals]", string(key))
	query.Set("limit", "1")
	query.Set("depth", "0")
	var out listResponse
	if err := c.do(ctx, http.MethodGet, "/api/pages?"+query.Encode(), s.TenantID, nil, &out); err != nil {
		return "", err
	}
	if len(out.Docs) == 0 {
		return "", nil
	}
	return idOfRaw(out.Docs[0].ID), nil
}

// ---------------------------------------------------------------------------
// ISR
// ---------------------------------------------------------------------------

func (c *Client) Revalidate(ctx context.Context, tenantSlug string, paths []string) error {
	if !c.Configured() || tenantSlug == "" || len(paths) == 0 {
		return nil
	}
	body := map[string]interface{}{"tenantSlug": tenantSlug, "paths": paths}
	var out map[string]interface{}
	return c.do(ctx, http.MethodPost, "/api/revalidate", "", body, &out)
}

// ---------------------------------------------------------------------------
// HTTP plumbing
// ---------------------------------------------------------------------------

type docResponse struct {
	Doc json.RawMessage `json:"doc"`
}

type listResponse struct {
	Docs []struct {
		ID json.RawMessage `json:"id"`
	} `json:"docs"`
}

func idOf(d docResponse) string {
	return idOfRaw(d.Doc)
}

func idOfRaw(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	var obj struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && len(obj.ID) > 0 {
		return idOfRaw(obj.ID)
	}
	return ""
}

func (c *Client) do(ctx context.Context, method, path, tenantID string, body interface{}, out interface{}) error {
	if !c.Configured() {
		return fmt.Errorf("payload is not configured")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)
	if tenantID != "" {
		req.Header.Set("X-Tenant-Id", tenantID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("payload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s %s", ErrNotFound, method, path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("payload %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// truncateRunes respeita os maxLength da collection. Sem isso um título longo
// do core faria o Payload recusar a escrita e o sync entraria em loop.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func (c *Client) publicHost(slug string) string {
	if c.hostSuffix == "" {
		return slug
	}
	return slug + "." + c.hostSuffix
}

// Noop é usado em dev/testes sem Payload: mantém a projeção local e o cadastro
// do site funcionando, apenas não publica no CMS.
type Noop struct{}

func (Noop) EnsureTenant(context.Context, *site.Site) (string, error)                 { return "", nil }
func (Noop) UpsertListing(context.Context, *site.Site, *site.Listing) (string, error) { return "", nil }
func (Noop) UnpublishListing(context.Context, *site.Site, *site.Listing) error        { return nil }
func (Noop) UpsertPage(context.Context, *site.Site, site.Page) error                  { return nil }
func (Noop) Revalidate(context.Context, string, []string) error                       { return nil }

func toImages(in []site.Image) []listingImage {
	out := make([]listingImage, 0, len(in))
	for i, img := range in {
		if strings.TrimSpace(img.URL) == "" {
			continue
		}
		order := img.Order
		if order == 0 {
			order = i
		}
		out = append(out, listingImage{URL: img.URL, Alt: img.Description, Order: order})
	}
	return out
}

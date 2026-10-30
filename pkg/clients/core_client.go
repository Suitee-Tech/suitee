package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type CoreClient struct {
	BaseURL    string
	HTTPClient *http.Client
	rdb        *redis.Client
}

type MyPermissions struct {
	FunctionalRole string `json:"functional_role"`
	// BaseRole é o papel do catálogo de onde um PERFIL DE ACESSO (`cr_…`) partiu.
	BaseRole    string   `json:"base_role,omitempty"`
	Profile     string   `json:"profile"`
	Permissions []string `json:"permissions"`
}

func NewCoreClient() *CoreClient {
	var rdb *redis.Client
	if uri := os.Getenv("REDIS_URI"); uri != "" {
		if opts, err := redis.ParseURL(uri); err == nil {
			rdb = redis.NewClient(opts)
		}
	}

	return &CoreClient{
		BaseURL: os.Getenv("CORE_SERVICE_URL"),
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		rdb: rdb,
	}
}

// permissionsCacheTTL e o TTL do cache local de permissões. Serve de rede de
// segurança: mesmo sem invalidação explícita, uma revogação propaga nesse prazo.
const permissionsCacheTTL = 30 * time.Second

// permissionsCacheVersionKey devolve a chave que carrega a VERSÃO da matriz do
// tenant. O toca-core (dono da matriz) a incrementa a cada PUT /permissions;
// os serviços que cacheiam comparam a versão antes de usar o cache.
//
// MOTIVAÇÃO (achado A5 da revisão de permissões de 2026-09-19): o cache tinha
// TTL de 30 s e NENHUMA invalidação na escrita, então uma revogação de
// permissão levava até 30 s para valer. Se ninguém incrementar a versão (ex.:
// toca-core sem Redis), a versão é vazia e o comportamento volta a ser
// "só TTL" — degradação segura, nunca cache eterno.
func permissionsCacheVersionKey(tenantID string) string {
	return fmt.Sprintf("permsver:%s", tenantID)
}

func permissionsCacheKey(tenantID, accountID string) string {
	return fmt.Sprintf("perms:%s:%s", tenantID, accountID)
}

// cachedPermissions é o envelope guardado no cache: o payload + a versão da
// matriz no momento em que ele foi gravado.
type cachedPermissions struct {
	Version string        `json:"v"`
	Payload MyPermissions `json:"p"`
}

func (c *CoreClient) GetMyPermissions(ctx context.Context) (*MyPermissions, error) {
	tenantID, _ := appCtx.GetTenantID(ctx)
	accountID, _ := appCtx.GetAccountID(ctx)
	useCache := tenantID != "" && accountID != "" && c.rdb != nil
	currentVersion := ""
	if useCache {
		// MGET para não pagar dois round-trips: versão corrente + entrada.
		values, err := c.rdb.MGet(ctx, permissionsCacheVersionKey(tenantID), permissionsCacheKey(tenantID, accountID)).Result()
		if err == nil && len(values) == 2 {
			if v, ok := values[0].(string); ok {
				currentVersion = v
			}
			if raw, ok := values[1].(string); ok && raw != "" {
				var envelope cachedPermissions
				if json.Unmarshal([]byte(raw), &envelope) == nil && envelope.Version == currentVersion {
					out := envelope.Payload
					return &out, nil
				}
			}
		}
	}

	token, err := appCtx.GetAuthToken(ctx)
	if err != nil {
		return nil, err
	}
	if c.BaseURL == "" {
		return nil, fmt.Errorf("CORE_SERVICE_URL is not set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/me/permissions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("core service returned status: %d", resp.StatusCode)
	}

	var out MyPermissions
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	if useCache {
		envelope := cachedPermissions{Version: currentVersion, Payload: out}
		if b, err := json.Marshal(envelope); err == nil {
			_ = c.rdb.Set(ctx, permissionsCacheKey(tenantID, accountID), string(b), permissionsCacheTTL).Err()
		}
	}

	return &out, nil
}

// InvalidatePermissionsCache remove a entrada de cache de um usuário. Usado nos
// testes e por qualquer caminho que precise forçar releitura imediata.
func (c *CoreClient) InvalidatePermissionsCache(ctx context.Context, tenantID, accountID string) error {
	if c.rdb == nil {
		return nil
	}
	return c.rdb.Del(ctx, permissionsCacheKey(tenantID, accountID)).Err()
}

// BumpPermissionsVersion incrementa a versão da matriz do tenant, invalidando
// TODAS as entradas de cache daquele tenant de uma vez. É o que o toca-core
// chama ao salvar a matriz (achado A5).
func (c *CoreClient) BumpPermissionsVersion(ctx context.Context, tenantID string) error {
	if c.rdb == nil {
		return nil
	}
	return c.rdb.Incr(ctx, permissionsCacheVersionKey(tenantID)).Err()
}

// PropertyDTO é uma projeção enxuta do core.Property usada por outros
// serviços via CoreClient — carrega só os campos que o match scoring
// (#415) e a renderização de "Sugestões para esse cliente" precisam.
// Mantemos os nomes JSON alinhados ao que o backend já serializa, mas o
// shape interno é flat (sem aninhar Pricing/Areas/Features) para o
// caller consumir sem ter que importar o pacote inteiro do toca-core.
type PropertyDTO struct {
	ID           string   `json:"id"`
	DisplayID    string   `json:"display_id,omitempty"`
	Title        string   `json:"title"`
	PropertyType string   `json:"property_type"`
	ListingType  string   `json:"listing_type"`
	AvailableFor []string `json:"available_for,omitempty"`
	Status       string   `json:"status,omitempty"`

	// Address — flat para facilitar matching/render.
	Street       string `json:"street,omitempty"`
	Neighborhood string `json:"neighborhood,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`

	// Pricing — só os "current_amount" que importam pro score.
	// Pode vir 0 quando o imóvel não tem o preço daquele tipo (ex.: só locação).
	SalePrice float64 `json:"sale_price,omitempty"`
	RentPrice float64 `json:"rent_price,omitempty"`
	CondoFee  float64 `json:"condo_fee,omitempty"`

	// Areas — total_area_m2.
	AreaTotal float64 `json:"area_total,omitempty"`

	// Features — só os campos do score (bedrooms) + extras para
	// render no card de sugestão (bathrooms, parking).
	Bedrooms      int `json:"bedrooms,omitempty"`
	Bathrooms     int `json:"bathrooms,omitempty"`
	ParkingSpaces int `json:"parking_spaces,omitempty"`

	// Media — só a thumbnail principal. O frontend pode pedir mais
	// fotos via GET /properties/:id quando o usuário clicar no card.
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	MediaImages  []string `json:"media_images,omitempty"`
}

// PropertyFilterDTO espelha os query params do GET /api/v1/properties
// que o CoreClient suporta. Subset do core.PropertyFilter — só os
// campos que o caller (LeadService.MatchProperties em toca-crm) precisa
// para filtrar candidatos. Limit/Offset paginam o pool.
type PropertyFilterDTO struct {
	TenantID     string
	ListingType  string // "sale" | "rent" | ""
	AvailableFor string // "sale" | "rent" | ""; accepts properties available in either primary or additional modality
	PropertyType string // "apartment" | "house" | "land" | "commercial" | ""
	MinPrice     *float64
	MaxPrice     *float64
	Limit        int64
	Offset       int64
}

// SearchProperties chama GET /api/v1/properties com filtros do caller.
// Usa o JWT do contexto (mesmo padrão de GetMyPermissions) — assim a
// chamada herda a permission do usuário que está pedindo o match
// (`properties:read`), sem precisar de internal-key.
//
// O backend devolve a Property "rica" (com address aninhado, pricing
// aninhado, etc.). Mapeamos para PropertyDTO enxuto na função abaixo —
// caller não precisa lidar com o JSON complexo do core.
func (c *CoreClient) SearchProperties(ctx context.Context, filter PropertyFilterDTO) ([]PropertyDTO, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("CORE_SERVICE_URL is not set")
	}
	token, err := appCtx.GetAuthToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth token required: %w", err)
	}

	q := url.Values{}
	if filter.ListingType != "" {
		q.Set("listing_type", filter.ListingType)
	}
	if filter.AvailableFor != "" {
		q.Set("available_for", filter.AvailableFor)
	}
	if filter.PropertyType != "" {
		q.Set("type", filter.PropertyType)
	}
	if filter.MinPrice != nil {
		q.Set("min_price", strconv.FormatFloat(*filter.MinPrice, 'f', 2, 64))
	}
	if filter.MaxPrice != nil {
		q.Set("max_price", strconv.FormatFloat(*filter.MaxPrice, 'f', 2, 64))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q.Set("limit", strconv.FormatInt(limit, 10))
	q.Set("offset", strconv.FormatInt(filter.Offset, 10))
	// "status" filter: o backend hoje aceita statuses de muitos tipos;
	// para match queremos só os "ainda disponíveis" — published e in_portfolio
	// são os mais relevantes. Sem filtro server-side ainda, o frontend
	// pode trazer todos e o score natural elimina os irrelevantes.

	endpoint := fmt.Sprintf("%s/api/v1/properties?%s", c.BaseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("core search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("core /properties returned status %d", resp.StatusCode)
	}

	// O endpoint é paginado: { "data": [...], "page", "limit", "total" }.
	// Cada Property tem campos aninhados — Address, Pricing, Areas,
	// Features. Decodificamos no formato bruto e mapeamos para o DTO
	// enxuto. O struct anônimo abaixo casa só os campos que importam.
	var raw struct {
		Data []rawProperty `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode core /properties: %w", err)
	}
	out := make([]PropertyDTO, 0, len(raw.Data))
	for _, p := range raw.Data {
		out = append(out, p.toDTO())
	}
	return out, nil
}

// GetPropertyByID busca um imóvel por id (GET /api/v1/properties/:id).
func (c *CoreClient) GetPropertyByID(ctx context.Context, id string) (*PropertyDTO, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("CORE_SERVICE_URL is not set")
	}
	token, err := appCtx.GetAuthToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth token required: %w", err)
	}
	endpoint := fmt.Sprintf("%s/api/v1/properties/%s", c.BaseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("core get property: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("core /properties/%s returned status %d", id, resp.StatusCode)
	}
	var raw struct {
		Data *rawProperty `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode core /properties/%s: %w", id, err)
	}
	if raw.Data == nil {
		return nil, fmt.Errorf("core /properties/%s returned no data", id)
	}
	dto := raw.Data.toDTO()
	return &dto, nil
}

// rawProperty espelha o JSON do core.Property só o suficiente pra
// extrair o que o DTO precisa. Internal type — não exportado.
type rawProperty struct {
	ID           string   `json:"id"`
	DisplayID    string   `json:"display_id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	PropertyType string   `json:"property_type"`
	ListingType  string   `json:"listing_type"`
	AvailableFor []string `json:"available_for"`
	Address      struct {
		Street       string `json:"street"`
		Neighborhood string `json:"neighborhood"`
		City         string `json:"city"`
		State        string `json:"state"`
	} `json:"address"`
	Pricing struct {
		SalePrice      *rawMoney `json:"sale_price,omitempty"`
		RentPrice      *rawMoney `json:"rent_price,omitempty"`
		CondominiumFee *rawMoney `json:"condominium_fee,omitempty"`
	} `json:"pricing"`
	Areas struct {
		TotalAreaM2 float64 `json:"total_area_m2"`
	} `json:"areas"`
	Features struct {
		Bedrooms      *int `json:"bedrooms"`
		Bathrooms     *int `json:"bathrooms"`
		ParkingSpaces *int `json:"parking_spaces"`
	} `json:"features"`
	Media struct {
		// thumbnail can come from a few different fields depending on the
		// upload pipeline — try the most common one and fall back to
		// the first image URL.
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		// core envia media.images como objetos {url,...}; aqui só a URL é usada
		Images []struct {
			URL string `json:"url"`
		} `json:"images,omitempty"`
	} `json:"media,omitempty"`
}

type rawMoney struct {
	CurrentAmount float64 `json:"current_amount"`
}

func (r rawProperty) toDTO() PropertyDTO {
	dto := PropertyDTO{
		ID:           r.ID,
		DisplayID:    r.DisplayID,
		Title:        r.Title,
		Status:       r.Status,
		PropertyType: r.PropertyType,
		ListingType:  r.ListingType,
		AvailableFor: r.AvailableFor,
		Street:       r.Address.Street,
		Neighborhood: r.Address.Neighborhood,
		City:         r.Address.City,
		State:        r.Address.State,
		AreaTotal:    r.Areas.TotalAreaM2,
	}
	if r.Pricing.SalePrice != nil {
		dto.SalePrice = r.Pricing.SalePrice.CurrentAmount
	}
	if r.Pricing.RentPrice != nil {
		dto.RentPrice = r.Pricing.RentPrice.CurrentAmount
	}
	if r.Pricing.CondominiumFee != nil {
		dto.CondoFee = r.Pricing.CondominiumFee.CurrentAmount
	}
	if r.Features.Bedrooms != nil {
		dto.Bedrooms = *r.Features.Bedrooms
	}
	if r.Features.Bathrooms != nil {
		dto.Bathrooms = *r.Features.Bathrooms
	}
	if r.Features.ParkingSpaces != nil {
		dto.ParkingSpaces = *r.Features.ParkingSpaces
	}
	if r.Media.ThumbnailURL != "" {
		dto.ThumbnailURL = r.Media.ThumbnailURL
	} else if len(r.Media.Images) > 0 {
		dto.ThumbnailURL = r.Media.Images[0].URL
	}
	for _, img := range r.Media.Images {
		if img.URL != "" {
			dto.MediaImages = append(dto.MediaImages, img.URL)
		}
	}
	if len(dto.MediaImages) == 0 && dto.ThumbnailURL != "" {
		dto.MediaImages = []string{dto.ThumbnailURL}
	}
	return dto
}

// ── Fuso horário do tenant ───────────────────────────────────────────────────

// tenantTimezoneCacheTTL é a validade do fuso em memória. É configuração (muda
// raramente), então evita uma chamada HTTP por query sem deixar a alteração do
// operador presa por muito tempo.
const tenantTimezoneCacheTTL = 5 * time.Minute

var (
	tenantTimezoneMu    sync.RWMutex
	tenantTimezoneCache = map[string]tenantTimezoneEntry{}
)

type tenantTimezoneEntry struct {
	timezone string
	expires  time.Time
}

// GetTenantTimezone lê o fuso configurado da imobiliária (Configurações →
// "Fuso horário") via rota INTERNA do toca-core.
//
// Sem JWT de usuário: usa a credencial interna (INTERNAL_CALLER_KEY, com a
// compartilhada como fallback) e o tenant explícito. Erro de rede não derruba o
// fluxo — o caller cai no fuso padrão do ambiente.
func (c *CoreClient) GetTenantTimezone(ctx context.Context, tenantID string) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", fmt.Errorf("tenant id is required")
	}

	tenantTimezoneMu.RLock()
	entry, ok := tenantTimezoneCache[tenantID]
	tenantTimezoneMu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.timezone, nil
	}

	if c.BaseURL == "" {
		return "", fmt.Errorf("CORE_SERVICE_URL is not set")
	}
	endpoint := c.BaseURL + "/api/v1/core/internal/tenant-settings?tenant_id=" + url.QueryEscape(tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Internal-Key", internalCallerKey())
	req.Header.Set("X-Tenant-Id", tenantID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("core service returned status: %d", resp.StatusCode)
	}

	var payload struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Timezone) == "" {
		return "", fmt.Errorf("core service returned empty timezone")
	}

	tenantTimezoneMu.Lock()
	tenantTimezoneCache[tenantID] = tenantTimezoneEntry{timezone: payload.Timezone, expires: time.Now().Add(tenantTimezoneCacheTTL)}
	tenantTimezoneMu.Unlock()
	return payload.Timezone, nil
}

// TenantLocationResolver é o adaptador usado por pkg/timeutil: devolve a
// localização configurada do tenant (e se ela existe). Erro de leitura devolve
// "não configurado" para o timeutil cair no fuso do ambiente.
func (c *CoreClient) TenantLocationResolver() func(ctx context.Context, tenantID string) (*time.Location, bool) {
	return func(ctx context.Context, tenantID string) (*time.Location, bool) {
		name, err := c.GetTenantTimezone(ctx, tenantID)
		if err != nil || strings.TrimSpace(name) == "" {
			return nil, false
		}
		loc, loadErr := time.LoadLocation(name)
		if loadErr != nil || loc == nil {
			return nil, false
		}
		return loc, true
	}
}

// InvalidateTenantTimezoneCache limpa o cache (testes e troca de configuração).
func InvalidateTenantTimezoneCache(tenantID ...string) {
	tenantTimezoneMu.Lock()
	defer tenantTimezoneMu.Unlock()
	if len(tenantID) == 0 {
		tenantTimezoneCache = map[string]tenantTimezoneEntry{}
		return
	}
	for _, id := range tenantID {
		delete(tenantTimezoneCache, id)
	}
}

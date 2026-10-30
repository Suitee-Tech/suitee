package site

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrNotFound é devolvido quando o site/listing não existe para o tenant
// consultado. Os handlers traduzem para 404 sem vazar se o recurso existe em
// outro tenant.
var ErrNotFound = errors.New("site not found")

// ErrSlugTaken indica que o slug escolhido já pertence a outra imobiliária.
var ErrSlugTaken = errors.New("essa marca já está em uso")

// ErrSlugInvalid indica que o slug não atende às regras da zona pública.
var ErrSlugInvalid = errors.New("marca do site inválida")

// ErrSiteDisabled é devolvido em resoluções públicas quando o site existe mas
// está desligado. Para o comprador o resultado é indistinguível de "não
// existe": 404, sem pista de que existe um site desligado.
var ErrSiteDisabled = errors.New("site is disabled")

// PageKey identifica as páginas editáveis do site.
type PageKey string

const (
	PageHome    PageKey = "home"
	PageAbout   PageKey = "about"
	PageContact PageKey = "contact"
)

// ValidPageKey reports whether key is one of the editable pages.
func ValidPageKey(key PageKey) bool {
	switch key {
	case PageHome, PageAbout, PageContact:
		return true
	default:
		return false
	}
}

// Branding é a projeção dos tokens visuais da imobiliária aplicados ao site
// público. A fonte continua sendo a aba Aparência do Toka; aqui é um snapshot
// para que o Payload consiga renderizar sem consultar o toca-core a cada
// request.
type Branding struct {
	LogoURL      string `json:"logo_url,omitempty"`
	CoverURL     string `json:"cover_url,omitempty"`
	PrimaryColor string `json:"primary_color,omitempty"`
	AccentColor  string `json:"accent_color,omitempty"`
	Theme        string `json:"theme,omitempty"`
	Tagline      string `json:"tagline,omitempty"`
}

// Contact é o recorte de contato da imobiliária publicado no site. A fonte é
// a aba Configurações do Toka (empresa + company settings); aqui é um snapshot
// para o rodapé, o card da imobiliária e os CTAs de WhatsApp.
type Contact struct {
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	WhatsApp    string `json:"whatsapp,omitempty"`
	AddressLine string `json:"address_line,omitempty"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	CRECI       string `json:"creci,omitempty"`
}

// Site é o registro da vitrine de uma imobiliária. `Slug` é a marca usada no
// host comercial `{slug}.imoveis.suitee.com.br` — nunca o UUID do tenant.
type Site struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Enabled         bool      `json:"enabled"`
	PublicBaseURL   string    `json:"public_base_url"`
	Branding        Branding  `json:"branding"`
	Contact         Contact   `json:"contact"`
	PayloadTenantID string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Page é uma das três páginas institucionais editáveis no painel.
type Page struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	SiteID         string          `json:"site_id"`
	Key            PageKey         `json:"key"`
	Title          string          `json:"title"`
	Subtitle       string          `json:"subtitle,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
	SEOTitle       string          `json:"seo_title,omitempty"`
	SEODescription string          `json:"seo_description,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// BodyText devolve o corpo editorial como texto simples. O painel edita texto
// (não rich text nesta fase); conteúdo legado em JSON é lido como string.
func (p Page) BodyText() string {
	if len(p.Body) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(p.Body, &text); err == nil {
		return text
	}
	return string(p.Body)
}

// Address é o recorte do endereço publicado no site. O número/rua só são
// expostos quando a imobiliária autoriza endereço exato.
type Address struct {
	Street       string `json:"street,omitempty"`
	Number       string `json:"number,omitempty"`
	Complement   string `json:"complement,omitempty"`
	Neighborhood string `json:"neighborhood,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	PostalCode   string `json:"postal_code,omitempty"`
}

// Features são os atributos usados nos filtros da vitrine.
type Features struct {
	Bedrooms      *int    `json:"bedrooms,omitempty"`
	Bathrooms     *int    `json:"bathrooms,omitempty"`
	Suites        *int    `json:"suites,omitempty"`
	ParkingSpaces *int    `json:"parking_spaces,omitempty"`
	AreaTotal     float64 `json:"area_total,omitempty"`
	AreaBuilt     float64 `json:"area_built,omitempty"`
}

// Image é uma foto do anúncio. A URL aponta para o Storage já existente do
// toca-core; o Payload não re-hospeda as fotos do imóvel.
type Image struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Order       int    `json:"order,omitempty"`
}

// ListingStatus é o estado da projeção no Payload.
type ListingStatus string

const (
	ListingPublished   ListingStatus = "published"
	ListingUnpublished ListingStatus = "unpublished"
)

// Listing é uma PROJEÇÃO do imóvel do toca-core. Nunca é source of truth:
// preço, status e mídia continuam vindo de core.properties.
type Listing struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenant_id"`
	SiteID          string               `json:"site_id"`
	PropertyID      string               `json:"property_id"`
	PropertyContext *LeadPropertyContext `json:"property_context,omitempty"`
	SeoSlug         string               `json:"seo_slug"`
	Status          ListingStatus        `json:"status"`
	Title           string               `json:"title"`
	Description     string               `json:"description,omitempty"`
	ListingType     string               `json:"listing_type"`
	PropertyType    string               `json:"property_type"`
	SalePrice       float64              `json:"sale_price,omitempty"`
	RentPrice       float64              `json:"rent_price,omitempty"`
	CondoFee        float64              `json:"condo_fee,omitempty"`
	PropertyTax     float64              `json:"property_tax,omitempty"`
	Address         Address              `json:"address"`
	Features        Features             `json:"features"`
	Amenities       []string             `json:"amenities,omitempty"`
	Images          []Image              `json:"images,omitempty"`
	SourceUpdatedAt time.Time            `json:"source_updated_at"`
	PublishedAt     *time.Time           `json:"published_at,omitempty"`
	PayloadDocID    string               `json:"-"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// PublicURL devolve a URL canônica do anúncio no host comercial.
func (l Listing) PublicURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" || l.SeoSlug == "" {
		return ""
	}
	return base + "/imoveis/" + l.SeoSlug
}

// SiteStats são os agregados exibidos no painel do site.
type SiteStats struct {
	Views7d             int64 `json:"views_7d"`
	Views30d            int64 `json:"views_30d"`
	Leads7d             int64 `json:"leads_7d"`
	Leads30d            int64 `json:"leads_30d"`
	PublishedListings   int64 `json:"published_listings"`
	TotalListings       int64 `json:"total_listings"`
	UnpublishedListings int64 `json:"unpublished_listings"`
}

// LeadPropertyContext é o recorte do imóvel que o comprador escolheu no site.
// Ele enriquece o lead no CRM (interesse/preferências) sem transformar o CRM em
// dono do estoque: os campos são um snapshot no momento do contato.
type LeadPropertyContext struct {
	PropertyID    string   `json:"property_id"`
	Title         string   `json:"title"`
	PropertyType  string   `json:"property_type"`
	ListingType   string   `json:"listing_type"`
	Neighborhood  string   `json:"neighborhood,omitempty"`
	City          string   `json:"city,omitempty"`
	State         string   `json:"state,omitempty"`
	SalePrice     float64  `json:"sale_price,omitempty"`
	RentPrice     float64  `json:"rent_price,omitempty"`
	Bedrooms      *int     `json:"bedrooms,omitempty"`
	Bathrooms     *int     `json:"bathrooms,omitempty"`
	Suites        *int     `json:"suites,omitempty"`
	ParkingSpaces *int     `json:"parking_spaces,omitempty"`
	AreaTotal     float64  `json:"area_total,omitempty"`
	ThumbnailURL  string   `json:"thumbnail_url,omitempty"`
	MediaImages   []string `json:"media_images,omitempty"`
}

// ListingStats é o payload do card "Performance Web" no detalhe do imóvel.
// Pageviews NÃO vão para o bus: são dado operacional lido sob demanda.
type ListingStats struct {
	PropertyID  string `json:"property_id"`
	ListingID   string `json:"listing_id,omitempty"`
	SeoSlug     string `json:"seo_slug,omitempty"`
	PublicURL   string `json:"public_url,omitempty"`
	Status      string `json:"status,omitempty"`
	Views7d     int64  `json:"views_7d"`
	Views30d    int64  `json:"views_30d"`
	Leads7d     int64  `json:"leads_7d"`
	Leads30d    int64  `json:"leads_30d"`
	LastViewAt  string `json:"last_view_at,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

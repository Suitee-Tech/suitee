package site

import "time"

// ViewInput é o beacon anônimo enviado pelo site público. Nenhum dado pessoal
// é necessário; o servidor guarda apenas um hash de sessão para deduplicação
// grosseira e atribuição de origem.
type ViewInput struct {
	PropertyID  string `json:"property_id"`
	Path        string `json:"path,omitempty"`
	Referrer    string `json:"referrer,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	UTMSource   string `json:"utm_source,omitempty"`
	UTMMedium   string `json:"utm_medium,omitempty"`
	UTMCampaign string `json:"utm_campaign,omitempty"`
}

// ListingView é a linha persistida em sites.listing_views. Pageviews são
// ruído para o bus: ficam só no Postgres e alimentam os agregados do painel.
type ListingView struct {
	ID          int64     `json:"id"`
	TenantID    string    `json:"tenant_id"`
	SiteID      string    `json:"site_id"`
	ListingID   string    `json:"listing_id"`
	PropertyID  string    `json:"property_id"`
	SessionKey  string    `json:"-"`
	Path        string    `json:"path,omitempty"`
	Referrer    string    `json:"referrer,omitempty"`
	UTMSource   string    `json:"utm_source,omitempty"`
	UTMMedium   string    `json:"utm_medium,omitempty"`
	UTMCampaign string    `json:"utm_campaign,omitempty"`
	Country     string    `json:"country,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// LeadStatus acompanha o espelhamento no CRM. O lead local é gravado ANTES da
// chamada HTTP para que uma indisponibilidade do CRM não perca o contato.
type LeadStatus string

const (
	LeadReceived  LeadStatus = "received"
	LeadProcessed LeadStatus = "processed"
	LeadFailed    LeadStatus = "failed"
	LeadDuplicate LeadStatus = "duplicate"
)

// Lead é o contato de interesse vindo do site público. Pertence SEMPRE ao
// tenant dono do host que recebeu o formulário — nunca a um tenant do corpo.
type Lead struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	SiteID     string `json:"site_id"`
	ListingID  string `json:"listing_id,omitempty"`
	PropertyID string `json:"property_id,omitempty"`
	// PropertyContext enriquece o lead do CRM com o imóvel que o comprador
	// abriu no site. Não é persistido em sites.leads (snapshot de contato).
	PropertyContext *LeadPropertyContext `json:"property_context,omitempty"`
	Name            string               `json:"name"`
	Email           string               `json:"email,omitempty"`
	Phone           string               `json:"phone,omitempty"`
	Message         string               `json:"message,omitempty"`
	Status          LeadStatus           `json:"status"`
	CRMLeadID       string               `json:"crm_lead_id,omitempty"`
	OriginHost      string               `json:"origin_host"`
	UTMSource       string               `json:"utm_source,omitempty"`
	UTMMedium       string               `json:"utm_medium,omitempty"`
	UTMCampaign     string               `json:"utm_campaign,omitempty"`
	Error           string               `json:"error,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// LeadInput é o corpo do formulário de interesse. `accepted_contact` é o
// consentimento explícito exigido para contato (LGPD); sem ele o lead é
// recusado em vez de ser gravado silenciosamente.
type LeadInput struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Message         string `json:"message,omitempty"`
	PropertyID      string `json:"property_id,omitempty"`
	AcceptedContact bool   `json:"accepted_contact"`
	UTMSource       string `json:"utm_source,omitempty"`
	UTMMedium       string `json:"utm_medium,omitempty"`
	UTMCampaign     string `json:"utm_campaign,omitempty"`
	// Honeypot é preenchido apenas por bots. Campo invisível no formulário.
	Honeypot string `json:"website,omitempty"`
}

// PublicListing é a menor projeção pública usada pelo beacon/lead para validar
// que o anúncio existe, está publicado e pertence ao site do host.
type PublicListing struct {
	ListingID  string
	PropertyID string
	SiteID     string
	TenantID   string
	SeoSlug    string
	Enabled    bool
}

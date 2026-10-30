package contracts

import "github.com/google/uuid"

// Propriedade da vitrine (ADR 0006). O toca-sites publica estes eventos no
// outbox (PublishReliable) — consumidores precisam ser idempotentes:
//
//   - site.listing.published  → toca-comm/analytics notificam publicada
//   - site.listing.unpublished → idem para saída da vitrine
//   - site.lead.received      → toca-comm abre atendimento; toca-crm é a fonte
//     de verdade do lead (o evento é o sinal de "novo lead do site").
//
// Pageviews NÃO são publicados: são ruído de barramento e ficam em
// sites.listing_views.

// SiteListingPublishedEventPayload fires quando um imóvel entra na vitrine
// pública (site enabled + status=published + publication_status=published).
type SiteListingPublishedEventPayload struct {
	SiteID     string `json:"site_id"`
	TenantID   string `json:"tenant_id"`
	ListingID  string `json:"listing_id"`
	PropertyID string `json:"property_id"`
	SeoSlug    string `json:"seo_slug"`
	PublicURL  string `json:"public_url,omitempty"`
	Timestamp  string `json:"timestamp"`
}

type SiteListingPublishedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       SiteListingPublishedEventPayload `json:"payload"`
}

func (e SiteListingPublishedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SiteListingPublishedEvent) Name() string         { return "site.listing.published" }
func (e SiteListingPublishedEvent) Payload() interface{} { return e.Data }

// SiteListingUnpublishedEventPayload fires quando o anúncio sai da vitrine
// (despublicação no core, site desligado ou imóvel removido).
type SiteListingUnpublishedEventPayload struct {
	SiteID     string `json:"site_id"`
	TenantID   string `json:"tenant_id"`
	ListingID  string `json:"listing_id"`
	PropertyID string `json:"property_id"`
	Reason     string `json:"reason,omitempty"`
	Timestamp  string `json:"timestamp"`
}

type SiteListingUnpublishedEvent struct {
	EventIDVal string                             `json:"event_id"`
	Data       SiteListingUnpublishedEventPayload `json:"payload"`
}

func (e SiteListingUnpublishedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SiteListingUnpublishedEvent) Name() string         { return "site.listing.unpublished" }
func (e SiteListingUnpublishedEvent) Payload() interface{} { return e.Data }

// SiteLeadReceivedEventPayload espelha o fluxo do webhook do toca-portal:
// o lead local já foi gravado (received) e o CRM foi chamado por HTTP interno.
// O evento carrega apenas o necessário para notificação/roteamento; dados
// pessoais completos continuam no CRM.
type SiteLeadReceivedEventPayload struct {
	LeadID     string `json:"lead_id"`
	TenantID   string `json:"tenant_id"`
	SiteID     string `json:"site_id"`
	ListingID  string `json:"listing_id,omitempty"`
	PropertyID string `json:"property_id,omitempty"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Message    string `json:"message,omitempty"`
	CRMLeadID  string `json:"crm_lead_id,omitempty"`
	OriginHost string `json:"origin_host,omitempty"`
	Timestamp  string `json:"timestamp"`
}

type SiteLeadReceivedEvent struct {
	EventIDVal string                       `json:"event_id"`
	Data       SiteLeadReceivedEventPayload `json:"payload"`
}

func (e SiteLeadReceivedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SiteLeadReceivedEvent) Name() string         { return "site.lead.received" }
func (e SiteLeadReceivedEvent) Payload() interface{} { return e.Data }

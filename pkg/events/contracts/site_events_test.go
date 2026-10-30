package contracts

import "testing"

func TestSiteListingPublishedEventPayloadShape(t *testing.T) {
	p := SiteListingPublishedEventPayload{
		SiteID: "s", TenantID: "t", ListingID: "l", PropertyID: "p",
		SeoSlug: "apartamento-centro", PublicURL: "https://a.imoveis.suitee.com.br/imoveis/x",
		Timestamp: "now",
	}
	want := []string{"listing_id", "property_id", "public_url", "seo_slug", "site_id", "tenant_id", "timestamp"}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestSiteListingUnpublishedEventPayloadShape(t *testing.T) {
	p := SiteListingUnpublishedEventPayload{
		SiteID: "s", TenantID: "t", ListingID: "l", PropertyID: "p",
		Reason: "unpublished", Timestamp: "now",
	}
	want := []string{"listing_id", "property_id", "reason", "site_id", "tenant_id", "timestamp"}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestSiteLeadReceivedEventPayloadShape(t *testing.T) {
	p := SiteLeadReceivedEventPayload{
		LeadID: "lead", TenantID: "t", SiteID: "s", ListingID: "l", PropertyID: "p",
		Name: "Ana", Email: "ana@example.com", Phone: "1199", Message: "oi",
		CRMLeadID: "crm", OriginHost: "a.imoveis.suitee.com.br", Timestamp: "now",
	}
	want := []string{
		"crm_lead_id", "email", "lead_id", "listing_id", "message", "name",
		"origin_host", "phone", "property_id", "site_id", "tenant_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

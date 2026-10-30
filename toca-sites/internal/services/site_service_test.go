package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

func TestSaveSiteDerivesSlugAndPublicURL(t *testing.T) {
	sites := newFakeSites()
	listings := newFakeListings()
	engagement := &fakeEngagement{}
	projection := &fakeProjection{}
	svc := NewSiteService(sites, listings, engagement, projection, "imoveis.suitee.com.br", "https")

	enabled := true
	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{
		Name: "Imobiliária Exemplo", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if out.Site.Slug != "imobiliaria-exemplo" {
		t.Fatalf("slug=%q", out.Site.Slug)
	}
	if out.Site.PublicBaseURL != "https://imobiliaria-exemplo.imoveis.suitee.com.br" {
		t.Fatalf("public_base_url=%q", out.Site.PublicBaseURL)
	}
	if projection.ensured != 1 {
		t.Fatalf("tenant Payload não provisionado: %d", projection.ensured)
	}
	if out.Site.PayloadTenantID == "" {
		t.Fatal("payload_tenant_id deve ser persistido")
	}
	if len(out.Pages) != 3 {
		t.Fatalf("esperava 3 páginas padrão, veio %d", len(out.Pages))
	}
}

func TestSaveSiteDerivesSemanticSlugFromNumericName(t *testing.T) {
	sites := newFakeSites()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "123"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if out.Site.Slug != "imobiliaria-111111" {
		t.Fatalf("slug semântico=%q", out.Site.Slug)
	}
}

func TestSaveSiteValidatesBrandingBeforePayload(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo",
		Enabled: true,
		Branding: site.Branding{
			PrimaryColor: "#1C905B", AccentColor: "#10b981", Theme: "light",
		},
	}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	_, err := svc.Save(context.Background(), testTenant, SaveSiteInput{
		Name: "exemplo", Branding: site.Branding{PrimaryColor: "#xyz"},
	})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "branding.primary_color" {
		t.Fatalf("cor inválida deveria ser 400 de branding, veio %v", err)
	}

	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{
		Name: "exemplo", Branding: site.Branding{Tagline: "Nova frase"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Branding.PrimaryColor != "#1c905b" || out.Site.Branding.AccentColor != "#10b981" {
		t.Fatalf("cores existentes devem ser preservadas quando omitidas: %+v", out.Site.Branding)
	}
	if out.Site.Branding.Tagline != "Nova frase" {
		t.Fatalf("tagline não aplicada: %+v", out.Site.Branding)
	}
}

func TestSaveSiteRejectsLongBrandingTagline(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo", Enabled: true}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	_, err := svc.Save(context.Background(), testTenant, SaveSiteInput{
		Name: "exemplo", Branding: site.Branding{Tagline: strings.Repeat("a", 161)},
	})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "branding.tagline" {
		t.Fatalf("tagline longa deveria ser 400, veio %v", err)
	}
}

func TestSaveSiteRejectsReservedSlug(t *testing.T) {
	sites := newFakeSites()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	_, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "Admin", Slug: "admin"})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "slug" {
		t.Fatalf("esperava ValidationError em slug, veio %v", err)
	}
}

func TestSaveSiteRejectsSlugFromAnotherTenant(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant["other-tenant"] = &site.Site{TenantID: "other-tenant", Slug: "exemplo", Enabled: true}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	_, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "Outra", Slug: "exemplo"})
	if !errors.Is(err, site.ErrSlugTaken) {
		t.Fatalf("esperava ErrSlugTaken, veio %v", err)
	}
}

func TestSaveSiteDerivesUniqueSlugWhenTaken(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant["other-tenant"] = &site.Site{TenantID: "other-tenant", Slug: "imobiliaria-exemplo", Enabled: true}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "Imobiliária Exemplo"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Slug != "imobiliaria-exemplo-2" {
		t.Fatalf("slug derivado=%q", out.Site.Slug)
	}
}

func TestDisablingSiteAlsoDisablesPayloadTenant(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo",
		Enabled: true, PublicBaseURL: "https://exemplo.imoveis.suitee.com.br",
		PayloadTenantID: "payload-Exemplo",
	}
	projection := &fakeProjection{}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, projection, "imoveis.suitee.com.br", "https")
	disabled := false
	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Enabled: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Enabled {
		t.Fatal("site local deveria estar desligado")
	}
	if projection.ensured != 1 || projection.lastEnabled {
		t.Fatalf("Payload deveria receber enabled=false: ensured=%d enabled=%v", projection.ensured, projection.lastEnabled)
	}
}

func TestSaveSiteWithPayloadDownDoesNotPersistEnabled(t *testing.T) {
	sites := newFakeSites()
	projection := &fakeProjection{err: errors.New("payload down")}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, projection, "imoveis.suitee.com.br", "https")
	enabled := true

	if _, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "exemplo", Enabled: &enabled}); err == nil {
		t.Fatal("esperava erro do Payload")
	}
	if _, err := sites.GetByTenant(context.Background(), testTenant); !errors.Is(err, site.ErrNotFound) {
		t.Fatal("site não pode ficar ligado quando o CMS falhou")
	}
}

func TestGetOverviewWithoutSiteReturnsDisabledDraft(t *testing.T) {
	svc := NewSiteService(newFakeSites(), newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")
	out, err := svc.GetOverview(context.Background(), testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Enabled || out.Site.Slug != "" || out.Site.TenantID != testTenant {
		t.Fatalf("overview inicial inesperado: %+v", out.Site)
	}
	if len(out.Pages) != 3 {
		t.Fatalf("painel precisa das 3 páginas mesmo sem site: %d", len(out.Pages))
	}
}

func TestSavePageRejectsUnknownKey(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	if _, err := svc.SavePage(context.Background(), testTenant, "blog", site.Page{Title: "x"}); err == nil {
		t.Fatal("página fora de home/about/contact deve ser recusada")
	}
}

func TestListingURLOnlyForPublishedListingOnEnabledSite(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	listings := newFakeListings()
	listings.byKey[listingKey(testTenant, testProperty)] = &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "apartamento-centro", Status: site.ListingPublished,
	}
	svc := NewSiteService(sites, listings, &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	out, err := svc.ListingURL(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Published || out.PublicURL != "https://exemplo.imoveis.suitee.com.br/imoveis/apartamento-centro" {
		t.Fatalf("URL pública inesperada: %+v", out)
	}

	sites.byTenant[testTenant].Enabled = false
	out, err = svc.ListingURL(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	if out.Published || out.PublicURL != "" {
		t.Fatalf("site desligado não pode expor URL: %+v", out)
	}
}

func TestSaveSiteNormalizesDirtyName(t *testing.T) {
	sites := newFakeSites()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")
	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Name: "  Ção & Cia Imóveis  "})
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Slug != "cao-cia-imoveis" {
		t.Fatalf("slug=%q", out.Site.Slug)
	}
	if !strings.HasSuffix(out.Site.PublicBaseURL, ".imoveis.suitee.com.br") {
		t.Fatalf("url=%q", out.Site.PublicBaseURL)
	}
}

func TestListingStatsFillsStatusAndURL(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	listings := newFakeListings()
	published := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	listings.byKey[listingKey(testTenant, testProperty)] = &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "apartamento-centro", Status: site.ListingPublished, PublishedAt: &published,
	}
	engagement := &fakeEngagement{}
	svc := NewSiteService(sites, listings, engagement, &fakeProjection{}, "imoveis.suitee.com.br", "https")
	svc.now = func() time.Time { return published }

	stats, err := svc.ListingStats(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ListingID != "listing-1" || stats.Status != "published" {
		t.Fatalf("stats: %+v", stats)
	}
	if stats.PublicURL != "https://exemplo.imoveis.suitee.com.br/imoveis/apartamento-centro" {
		t.Fatalf("url: %q", stats.PublicURL)
	}
	if stats.PublishedAt == "" {
		t.Fatal("published_at deveria ser preenchido")
	}
}

func TestSavePagePublishesToProjection(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	projection := &fakeProjection{}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, projection, "imoveis.suitee.com.br", "https")

	page, err := svc.SavePage(context.Background(), testTenant, "about", site.Page{Title: "Sobre nós", Body: []byte(`"texto"`)})
	if err != nil {
		t.Fatal(err)
	}
	if page.Key != site.PageAbout || page.Title != "Sobre nós" {
		t.Fatalf("página: %+v", page)
	}
	if projection.pages != 1 {
		t.Fatalf("página não replicada no Payload: %d", projection.pages)
	}
}

func TestListLeadsPassesPaginationToRepository(t *testing.T) {
	svc := NewSiteService(newFakeSites(), newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")
	leads, total, err := svc.ListLeads(context.Background(), testTenant, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(leads) != 0 {
		t.Fatalf("leads=%v total=%d", leads, total)
	}
}

func TestFirstEnableTriggersBackfill(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo", Enabled: false,
	}
	reconciler := &fakeReconciler{done: make(chan struct{})}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https").
		WithReconciler(reconciler)
	enabled := true
	if _, err := svc.Save(context.Background(), testTenant, SaveSiteInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reconciler.done:
	case <-time.After(2 * time.Second):
		t.Fatal("backfill não foi disparado no primeiro enable")
	}
	if reconciler.tenantID != testTenant {
		t.Fatalf("tenant do backfill=%q", reconciler.tenantID)
	}
}

func TestSaveWithoutEnablingDoesNotBackfill(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo", Enabled: false,
	}
	reconciler := &fakeReconciler{}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https").
		WithReconciler(reconciler)
	if _, err := svc.Save(context.Background(), testTenant, SaveSiteInput{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if reconciler.calls != 0 {
		t.Fatal("save de site já desligado não pode disparar backfill")
	}
}

func TestPublicPortAppearsInCanonicalURL(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: testSiteID, TenantID: testTenant, Slug: "exemplo", Enabled: true,
	}
	listings := newFakeListings()
	listings.byKey[listingKey(testTenant, testProperty)] = &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "apartamento-centro", Status: site.ListingPublished,
	}
	svc := NewSiteService(sites, listings, &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http").
		WithPublicPort("3002")
	out, err := svc.ListingURL(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	want := "http://exemplo.imoveis.localhost:3002/imoveis/apartamento-centro"
	if out.PublicURL != want {
		t.Fatalf("url=%q want %q", out.PublicURL, want)
	}
}

func TestSaveSitePersistsContact(t *testing.T) {
	sites := newFakeSites()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http").
		WithPublicPort("3002")
	enabled := true
	out, err := svc.Save(context.Background(), testTenant, SaveSiteInput{
		Name: "exemplo", Enabled: &enabled,
		Contact: site.Contact{
			Email: "contato@Exemplo.com.br", Phone: "(11) 3333-0000",
			WhatsApp: "(11) 99999-0000", CRECI: "CRECI-SP 12345-J",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Site.Contact.CRECI != "CRECI-SP 12345-J" || out.Site.Contact.Email != "contato@Exemplo.com.br" {
		t.Fatalf("contato não persistido: %+v", out.Site.Contact)
	}
}

func TestPortalSiteReturnsOnlyEnabledPublicData(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "exemplo",
		Enabled: true, Branding: site.Branding{PrimaryColor: "#1C905B"},
		Contact: site.Contact{WhatsApp: "(11) 99999-0000"},
	}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http").
		WithPublicPort("3002")
	out, err := svc.PortalSite(context.Background(), testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if out.PublicURL != "http://exemplo.imoveis.localhost:3002" || out.Name != "exemplo" {
		t.Fatalf("portal site: %+v", out)
	}
	if out.Branding.PrimaryColor != "#1C905B" || out.ContactPhone == "" {
		t.Fatalf("branding/contato: %+v", out)
	}
}

func TestPortalSiteHidesDisabledSite(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{ID: "site-1", TenantID: testTenant, Slug: "exemplo", Enabled: false}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http")
	if _, err := svc.PortalSite(context.Background(), testTenant); !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
}

func TestReconcileRunsBackfill(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{ID: "site-1", TenantID: testTenant, Slug: "exemplo", Enabled: true}
	reconciler := &fakeReconciler{done: make(chan struct{})}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http").
		WithReconciler(reconciler)
	if err := svc.Reconcile(context.Background(), testTenant); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reconciler.done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconcile não executou")
	}
	if reconciler.tenantID != testTenant {
		t.Fatalf("tenant=%q", reconciler.tenantID)
	}
}

func TestGetOverviewRefreshesStalePublicBaseURL(t *testing.T) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "Exemplo-imoveis", Name: "exemplo",
		Enabled: true, PublicBaseURL: "https://Exemplo-imoveis.imoveis.suitee.com.br",
	}
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "beta.imoveis.suitee.com.br", "https")
	out, err := svc.GetOverview(context.Background(), testTenant)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://Exemplo-imoveis.beta.imoveis.suitee.com.br"
	if out.Site.PublicBaseURL != want || sites.byTenant[testTenant].PublicBaseURL != want {
		t.Fatalf("URL canônica não foi atualizada: out=%q persisted=%q", out.Site.PublicBaseURL, sites.byTenant[testTenant].PublicBaseURL)
	}
}

func TestSiteReadinessStates(t *testing.T) {
	sites := newFakeSites()
	svc := NewSiteService(sites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.suitee.com.br", "https")

	out, err := svc.Readiness(context.Background(), testTenant)
	if err != nil || out.Status != "not_configured" || out.Ready {
		t.Fatalf("sem site: %+v err=%v", out, err)
	}

	sites.byTenant[testTenant] = &site.Site{ID: "site-1", TenantID: testTenant, Slug: "exemplo", Enabled: false}
	out, err = svc.Readiness(context.Background(), testTenant)
	if err != nil || out.Status != "disabled" || out.Ready {
		t.Fatalf("site desligado: %+v err=%v", out, err)
	}

	// No local a URL é HTTP e não há ACME; o link é considerado pronto.
	localSites := newFakeSites()
	localSites.byTenant[testTenant] = &site.Site{ID: "site-1", TenantID: testTenant, Slug: "exemplo", Enabled: true}
	localSvc := NewSiteService(localSites, newFakeListings(), &fakeEngagement{}, &fakeProjection{}, "imoveis.localhost", "http")
	out, err = localSvc.Readiness(context.Background(), testTenant)
	if err != nil || !out.Ready || out.Status != "ready" {
		t.Fatalf("site local: %+v err=%v", out, err)
	}
}

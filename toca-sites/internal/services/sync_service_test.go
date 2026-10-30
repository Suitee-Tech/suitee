package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/client/core"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

const (
	testTenant   = "11111111-1111-4111-8111-111111111111"
	testProperty = "22222222-2222-4222-8222-222222222222"
	testSiteID   = "33333333-3333-4333-8333-333333333333"
)

func eligibleProperty() *site.PropertySnapshot {
	return &site.PropertySnapshot{
		ID:                testProperty,
		Title:             "Apartamento 3 quartos no Centro",
		Description:       "Vista para a praça.",
		Status:            "published",
		PublicationStatus: "published",
		PropertyType:      "apartment",
		ListingType:       "sale",
		SalePrice:         350000,
		Address: site.Address{
			Street: "Rua das Flores", Number: "123", Neighborhood: "Centro", City: "Juazeiro do Norte",
			State: "CE", PostalCode: "63000-000",
		},
		Features:  site.Features{Bedrooms: intPtr(3), Bathrooms: intPtr(2)},
		AreaTotal: 120,
		AreaBuilt: 100,
		Images:    []site.Image{{URL: "https://cdn.example/1.jpg", Order: 1}},
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
	}
}

func enabledSite() *site.Site {
	return &site.Site{
		ID: testSiteID, TenantID: testTenant, Slug: "exemplo", Name: "Imobiliária Exemplo",
		Enabled: true, PublicBaseURL: "https://exemplo.imoveis.suitee.com.br",
	}
}

type syncFixture struct {
	sites      *fakeSites
	listings   *fakeListings
	core       *fakeCore
	projection *fakeProjection
	bus        *fakeBus
	service    *SyncService
}

func newSyncFixture() *syncFixture {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	listings := newFakeListings()
	coreSource := &fakeCore{property: eligibleProperty()}
	projection := &fakeProjection{}
	bus := &fakeBus{}
	return &syncFixture{
		sites: sites, listings: listings, core: coreSource, projection: projection, bus: bus,
		service: NewSyncService(sites, listings, coreSource, projection, bus, "imoveis.suitee.com.br", "https"),
	}
}

func TestSyncPublishesEligibleProperty(t *testing.T) {
	f := newSyncFixture()
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatalf("SyncProperty: %v", err)
	}
	if f.projection.upserts != 1 {
		t.Fatalf("esperava 1 upsert no Payload, veio %d", f.projection.upserts)
	}
	saved, err := f.listings.GetByProperty(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatalf("listing não persistido: %v", err)
	}
	if saved.Status != site.ListingPublished {
		t.Fatalf("status=%s, esperava published", saved.Status)
	}
	if saved.PayloadDocID != "doc-"+testProperty {
		t.Fatalf("payload doc id=%q", saved.PayloadDocID)
	}
	if saved.SeoSlug != "apartamento-3-quartos-no-centro" {
		t.Fatalf("seo slug=%q", saved.SeoSlug)
	}
	if f.bus.published != 1 || f.bus.names[0] != "site.listing.published" {
		t.Fatalf("eventos publicados=%v", f.bus.names)
	}
}

func TestSyncGeneratesFallbackTitleAndDescription(t *testing.T) {
	f := newSyncFixture()
	f.core.property.Title = "   "
	f.core.property.Description = ""
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	saved, err := f.listings.GetByProperty(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Title == "" || saved.Description == "" {
		t.Fatalf("fallback de conteúdo vazio: %+v", saved)
	}
	if saved.Features.AreaTotal != 120 || saved.Features.AreaBuilt != 100 {
		t.Fatalf("áreas não projetadas: %+v", saved.Features)
	}
	if saved.SeoSlug == "" {
		t.Fatal("slug SEO não pode ser vazio")
	}
}

func TestSyncRetriesSeoSlugAfterUniqueRace(t *testing.T) {
	f := newSyncFixture()
	f.listings.upsertErr = site.ErrSlugTaken

	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatalf("SyncProperty: %v", err)
	}
	saved, err := f.listings.GetByProperty(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SeoSlug != "apartamento-3-quartos-no-centro-2" {
		t.Fatalf("slug de retry=%q", saved.SeoSlug)
	}
	if f.projection.upserts != 2 {
		t.Fatalf("esperava 2 upserts no Payload, veio %d", f.projection.upserts)
	}
}

func TestSyncKeepsSeoSlugStableOnTitleChange(t *testing.T) {
	f := newSyncFixture()
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	f.core.property.Title = "Novo título comercial"
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	saved, _ := f.listings.GetByProperty(context.Background(), testTenant, testProperty)
	if saved.SeoSlug != "apartamento-3-quartos-no-centro" {
		t.Fatalf("slug mudou com o título: %q", saved.SeoSlug)
	}
}

func TestSyncDoesNotPublishDraft(t *testing.T) {
	f := newSyncFixture()
	// Já existe projeção publicada; o imóvel virou rascunho no core.
	existing := &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "apartamento-3-quartos-no-centro", Status: site.ListingPublished,
		PayloadDocID: "doc-1", Title: "Antigo",
	}
	f.listings.byKey[listingKey(testTenant, testProperty)] = existing
	f.core.property.PublicationStatus = "draft"

	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatalf("SyncProperty: %v", err)
	}
	if f.projection.unpublish != 1 {
		t.Fatalf("esperava unpublish no Payload, veio %d", f.projection.unpublish)
	}
	if f.listings.unpublished != 1 {
		t.Fatalf("esperava unpublish local, veio %d", f.listings.unpublished)
	}
	if len(f.bus.names) != 1 || f.bus.names[0] != "site.listing.unpublished" {
		t.Fatalf("eventos=%v", f.bus.names)
	}
	if f.projection.upserts != 0 {
		t.Fatal("não pode publicar imóvel em rascunho")
	}
}

func TestSyncSiteDisabledNeverReadsCore(t *testing.T) {
	f := newSyncFixture()
	f.sites.byTenant[testTenant].Enabled = false

	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if f.core.calls != 0 {
		t.Fatalf("core chamado %d vezes com site desligado", f.core.calls)
	}
	if f.bus.published != 0 {
		t.Fatalf("eventos=%v", f.bus.names)
	}
}

func TestSyncMissingPropertyUnpublishes(t *testing.T) {
	f := newSyncFixture()
	f.core.err = core.ErrNotFound
	existing := &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "x", Status: site.ListingPublished, PayloadDocID: "doc-1",
	}
	f.listings.byKey[listingKey(testTenant, testProperty)] = existing

	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if f.listings.unpublished != 1 {
		t.Fatal("imóvel removido do core deve sair da vitrine")
	}
}

func TestSyncAlreadyPublishedDoesNotRepublishEvent(t *testing.T) {
	f := newSyncFixture()
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if f.bus.published != 1 {
		t.Fatalf("re-sync duplicou evento: %v", f.bus.names)
	}
	if f.projection.upserts != 2 {
		t.Fatalf("upsert idempotente esperado em todo sync: %d", f.projection.upserts)
	}
}

func TestSyncNoSiteIsNoop(t *testing.T) {
	f := newSyncFixture()
	delete(f.sites.byTenant, testTenant)
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if f.core.calls != 0 || f.projection.upserts != 0 {
		t.Fatal("tenant sem site não deve tocar no CMS/core")
	}
}

func TestSyncPayloadFailureSurfacesForRetry(t *testing.T) {
	f := newSyncFixture()
	f.projection.err = errors.New("payload down")
	err := f.service.SyncProperty(context.Background(), testTenant, testProperty)
	if err == nil {
		t.Fatal("falha do Payload precisa subir para o RabbitMQ reentregar")
	}
	if _, err := f.listings.GetByProperty(context.Background(), testTenant, testProperty); err == nil {
		t.Fatal("não pode marcar published quando o CMS falhou")
	}
}

func TestHandlePropertyEventDropsInvalidIDs(t *testing.T) {
	f := newSyncFixture()
	if err := f.service.HandlePropertyEvent(context.Background(), "not-a-uuid", testProperty); err != nil {
		t.Fatalf("evento inválido deve ser descartado, não reenfileirado: %v", err)
	}
	if f.core.calls != 0 {
		t.Fatal("não deve consultar o core com id inválido")
	}
}

func TestUnpublishKeepsUiStableWhenNoProjection(t *testing.T) {
	f := newSyncFixture()
	f.sites.byTenant[testTenant].Enabled = false
	if err := f.service.SyncProperty(context.Background(), testTenant, testProperty); err != nil {
		t.Fatal(err)
	}
	if len(f.bus.names) != 0 {
		t.Fatalf("sem listing existente não há transição a publicar: %v", f.bus.names)
	}
}

func intPtr(n int) *int { return &n }

func TestReconcileTenantPublishesExistingStock(t *testing.T) {
	f := newSyncFixture()
	if err := f.service.ReconcileTenant(context.Background(), testTenant); err != nil {
		t.Fatal(err)
	}
	if f.projection.upserts != 1 {
		t.Fatalf("backfill deveria publicar o estoque: %d", f.projection.upserts)
	}
	if f.bus.published != 1 {
		t.Fatalf("primeira publicação deve emitir evento: %v", f.bus.names)
	}
}

func TestReconcileTenantSkipsDisabledSite(t *testing.T) {
	f := newSyncFixture()
	f.sites.byTenant[testTenant].Enabled = false
	if err := f.service.ReconcileTenant(context.Background(), testTenant); err != nil {
		t.Fatal(err)
	}
	if f.core.calls != 0 || f.projection.upserts != 0 {
		t.Fatal("site desligado não faz backfill")
	}
}

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

const testHost = "exemplo.imoveis.suitee.com.br"

func newEngagementFixture() (*EngagementService, *fakeSites, *fakeListings, *fakeEngagement, *fakeCRM, *fakeBus) {
	sites := newFakeSites()
	sites.byTenant[testTenant] = enabledSite()
	listings := newFakeListings()
	listings.byKey[listingKey(testTenant, testProperty)] = &site.Listing{
		ID: "listing-1", TenantID: testTenant, SiteID: testSiteID, PropertyID: testProperty,
		SeoSlug: "apartamento-centro", Status: site.ListingPublished,
	}
	repo := &fakeEngagement{}
	crm := &fakeCRM{}
	bus := &fakeBus{}
	svc := NewEngagementService(sites, listings, repo, crm, bus, "imoveis.suitee.com.br")
	svc.now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	return svc, sites, listings, repo, crm, bus
}

func TestRecordViewResolvesTenantFromHost(t *testing.T) {
	svc, _, _, repo, _, _ := newEngagementFixture()
	err := svc.RecordView(context.Background(), testHost, site.ViewInput{
		PropertyID: testProperty, Path: "/imoveis/apartamento-centro", SessionID: "abc",
	}, RequestMeta{IP: "203.0.113.9", UserAgent: "test"})
	if err != nil {
		t.Fatalf("RecordView: %v", err)
	}
	if len(repo.views) != 1 {
		t.Fatalf("views=%d", len(repo.views))
	}
	v := repo.views[0]
	if v.TenantID != testTenant || v.SiteID != testSiteID || v.ListingID != "listing-1" {
		t.Fatalf("view fora do tenant do host: %+v", v)
	}
	if v.SessionKey == "abc" || v.SessionKey == "" {
		t.Fatalf("sessão deve ser persistida como hash, veio %q", v.SessionKey)
	}
}

func TestRecordViewRejectsUnknownOrDisabledHost(t *testing.T) {
	svc, sites, _, repo, _, _ := newEngagementFixture()

	if err := svc.RecordView(context.Background(), "outra.imoveis.suitee.com.br", site.ViewInput{PropertyID: testProperty}, RequestMeta{}); !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("host sem site deve ser 404, veio %v", err)
	}
	if err := svc.RecordView(context.Background(), "Exemplo.sites.suitee.com.br", site.ViewInput{PropertyID: testProperty}, RequestMeta{}); !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("zona sites.* não é zona pública, veio %v", err)
	}
	sites.byTenant[testTenant].Enabled = false
	if err := svc.RecordView(context.Background(), testHost, site.ViewInput{PropertyID: testProperty}, RequestMeta{}); !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("site desligado deve ser 404, veio %v", err)
	}
	if len(repo.views) != 0 {
		t.Fatalf("nenhuma view pode ser gravada: %d", len(repo.views))
	}
}

func TestRecordViewRejectsUnpublishedListing(t *testing.T) {
	svc, _, listings, _, _, _ := newEngagementFixture()
	listings.byKey[listingKey(testTenant, testProperty)].Status = site.ListingUnpublished
	err := svc.RecordView(context.Background(), testHost, site.ViewInput{PropertyID: testProperty}, RequestMeta{})
	if !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("listing despublicada deve ser 404, veio %v", err)
	}
}

func TestCreateLeadPersistsThenCallsCRMAndPublishes(t *testing.T) {
	svc, _, _, repo, crm, bus := newEngagementFixture()
	lead, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana Souza", Email: "ana@example.com", Phone: "(88) 99999-0000",
		Message: "Tenho interesse", PropertyID: testProperty, AcceptedContact: true,
	}, RequestMeta{IP: "203.0.113.9"})
	if err != nil {
		t.Fatalf("CreateLead: %v", err)
	}
	if lead.TenantID != testTenant || lead.PropertyID != testProperty {
		t.Fatalf("lead fora do tenant do host: %+v", lead)
	}
	if crm.calls != 1 || crm.tenantID != testTenant {
		t.Fatalf("CRM chamado %d vezes (tenant %q)", crm.calls, crm.tenantID)
	}
	if lead.Status != site.LeadProcessed || lead.CRMLeadID == "" {
		t.Fatalf("lead deveria estar processed: %+v", lead)
	}
	if len(repo.leads) != 1 {
		t.Fatalf("lead local não gravado")
	}
	if bus.published != 1 || bus.names[0] != "site.lead.received" {
		t.Fatalf("evento=%v", bus.names)
	}
	if lead.Phone != "88999990000" {
		t.Fatalf("telefone deve ser normalizado, veio %q", lead.Phone)
	}
}

func TestCreateLeadSurvivesCRMOutage(t *testing.T) {
	svc, _, _, repo, crm, bus := newEngagementFixture()
	crm.err = errors.New("crm down")
	lead, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana Souza", Email: "ana@example.com", AcceptedContact: true,
	}, RequestMeta{})
	if err != nil {
		t.Fatalf("queda do CRM não pode derrubar o formulário: %v", err)
	}
	if lead.Status != site.LeadFailed {
		t.Fatalf("status=%s", lead.Status)
	}
	if len(repo.leads) != 1 || repo.leads[0].Status != site.LeadFailed {
		t.Fatalf("lead precisa ficar persistido como failed: %+v", repo.leads)
	}
	if bus.published != 0 {
		t.Fatal("sem lead no CRM não há site.lead.received")
	}
}

func TestCreateLeadDropsHoneypot(t *testing.T) {
	svc, _, _, repo, crm, _ := newEngagementFixture()
	_, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Bot", Email: "bot@example.com", AcceptedContact: true, Honeypot: "http://spam",
	}, RequestMeta{})
	if !errors.Is(err, ErrDiscarded) {
		t.Fatalf("esperava ErrDiscarded, veio %v", err)
	}
	if len(repo.leads) != 0 || crm.calls != 0 {
		t.Fatal("honeypot não pode gravar nem chamar o CRM")
	}
}

func TestCreateLeadRequiresConsentAndContact(t *testing.T) {
	svc, _, _, _, _, _ := newEngagementFixture()
	_, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{Name: "Ana Souza", Email: "ana@example.com"}, RequestMeta{})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "accepted_contact" {
		t.Fatalf("consentimento obrigatório: %v", err)
	}
	_, err = svc.CreateLead(context.Background(), testHost, site.LeadInput{Name: "Ana Souza", AcceptedContact: true}, RequestMeta{})
	if !errors.As(err, &ve) || ve.Field != "email" {
		t.Fatalf("e-mail/telefone obrigatório: %v", err)
	}
	_, err = svc.CreateLead(context.Background(), testHost, site.LeadInput{Name: "Ana Souza", Email: "nao-e-email", AcceptedContact: true}, RequestMeta{})
	if !errors.As(err, &ve) || ve.Field != "email" {
		t.Fatalf("e-mail inválido: %v", err)
	}
}

func TestCreateLeadRequiresSurname(t *testing.T) {
	svc, _, _, repo, crm, _ := newEngagementFixture()
	_, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana", Email: "ana@example.com", AcceptedContact: true,
	}, RequestMeta{})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("monônimo deve ser recusado no campo name, veio %v", err)
	}
	if len(repo.leads) != 0 || crm.calls != 0 {
		t.Fatal("lead sem sobrenome não pode ser gravado nem chegar ao CRM")
	}
}

func TestCreateLeadDeduplicatesWithinWindow(t *testing.T) {
	svc, _, _, repo, crm, _ := newEngagementFixture()
	repo.dup = &site.Lead{ID: "existing", TenantID: testTenant, Name: "Ana Souza", Status: site.LeadProcessed}
	lead, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana Souza", Email: "ana@example.com", AcceptedContact: true,
	}, RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if lead.ID != "existing" || lead.Status != site.LeadDuplicate {
		t.Fatalf("dedupe inesperado: %+v", lead)
	}
	if crm.calls != 0 {
		t.Fatal("duplicado não pode criar segundo lead no CRM")
	}
}

func TestCreateLeadRejectsPropertyFromAnotherSite(t *testing.T) {
	svc, _, _, repo, _, _ := newEngagementFixture()
	foreign := "44444444-4444-4444-8444-444444444444"
	_, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana Souza", Email: "ana@example.com", PropertyID: foreign, AcceptedContact: true,
	}, RequestMeta{})
	if !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("imóvel de outro tenant deve ser 404, veio %v", err)
	}
	if len(repo.leads) != 0 {
		t.Fatal("não pode gravar lead para imóvel de outra vitrine")
	}
}

func TestCreateLeadOnContactPageWithoutProperty(t *testing.T) {
	svc, _, _, _, crm, _ := newEngagementFixture()
	lead, err := svc.CreateLead(context.Background(), testHost, site.LeadInput{
		Name: "Ana Souza", Email: "ana@example.com", AcceptedContact: true,
	}, RequestMeta{})
	if err != nil {
		t.Fatalf("contato sem imóvel deve funcionar: %v", err)
	}
	if lead.PropertyID != "" || crm.calls != 1 {
		t.Fatalf("lead=%+v crm=%d", lead, crm.calls)
	}
}

func TestSessionKeyNeverStoresRawSession(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	key := sessionKey("session-raw", RequestMeta{IP: "203.0.113.9"}, now)
	if key == "session-raw" || len(key) != 32 {
		t.Fatalf("chave de sessão inesperada: %q", key)
	}
	if again := sessionKey("session-raw", RequestMeta{IP: "203.0.113.9"}, now); again != key {
		t.Fatal("hash deve ser determinístico para o mesmo par sessão/meta")
	}
	if anonymous := sessionKey("", RequestMeta{IP: "203.0.113.9", UserAgent: "ua"}, now); len(anonymous) != 32 {
		t.Fatalf("fallback anônimo: %q", anonymous)
	}
}

func TestNormalizeHostStripsSchemePortAndPath(t *testing.T) {
	cases := map[string]string{
		"https://exemplo.imoveis.Suitee.Com.Br:443/imoveis/x?utm=1": "exemplo.imoveis.suitee.com.br",
		"exemplo.imoveis.SUITEE.COM.BR.":                            "exemplo.imoveis.suitee.com.br",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q)=%q want %q", in, got, want)
		}
	}
}

package services

import (
	"context"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// Fakes compartilhados pelos testes de serviço. Mantêm o comportamento mínimo
// e registram chamadas para os asserts de contrato (evento, projeção, CRM).

type fakeSites struct {
	byTenant map[string]*site.Site
	pages    map[string][]site.Page
	slugErr  error
	nextID   int
}

func newFakeSites() *fakeSites {
	return &fakeSites{byTenant: map[string]*site.Site{}, pages: map[string][]site.Page{}, nextID: 1}
}

func (f *fakeSites) GetByTenant(_ context.Context, tenantID string) (*site.Site, error) {
	if s, ok := f.byTenant[tenantID]; ok {
		copy := *s
		return &copy, nil
	}
	return nil, site.ErrNotFound
}

func (f *fakeSites) FindBySlug(_ context.Context, slug string) (*site.Site, error) {
	for _, s := range f.byTenant {
		if s.Slug == slug {
			if !s.Enabled {
				return nil, site.ErrSiteDisabled
			}
			copy := *s
			return &copy, nil
		}
	}
	return nil, site.ErrNotFound
}

func (f *fakeSites) Create(_ context.Context, s *site.Site) error {
	if s.ID == "" {
		s.ID = "site-" + s.TenantID
	}
	copy := *s
	f.byTenant[s.TenantID] = &copy
	return nil
}

func (f *fakeSites) Update(_ context.Context, s *site.Site) error {
	if _, ok := f.byTenant[s.TenantID]; !ok {
		return site.ErrNotFound
	}
	copy := *s
	f.byTenant[s.TenantID] = &copy
	return nil
}

func (f *fakeSites) SlugTaken(_ context.Context, tenantID, slug string) (bool, error) {
	if f.slugErr != nil {
		return false, f.slugErr
	}
	for t, s := range f.byTenant {
		if s.Slug == slug && t != tenantID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeSites) SetPayloadTenantID(_ context.Context, tenantID, siteID, payloadTenantID string) error {
	if s, ok := f.byTenant[tenantID]; ok {
		s.PayloadTenantID = payloadTenantID
	}
	return nil
}

func (f *fakeSites) ListPages(_ context.Context, tenantID, siteID string) ([]site.Page, error) {
	return append([]site.Page{}, f.pages[tenantID]...), nil
}

func (f *fakeSites) GetPage(_ context.Context, tenantID, siteID string, key site.PageKey) (*site.Page, error) {
	for _, p := range f.pages[tenantID] {
		if p.Key == key {
			copy := p
			return &copy, nil
		}
	}
	return nil, site.ErrNotFound
}

func (f *fakeSites) UpsertPages(_ context.Context, tenantID, siteID string, pages []site.Page) error {
	for i := range pages {
		pages[i].TenantID = tenantID
		pages[i].SiteID = siteID
	}
	f.pages[tenantID] = append(f.pages[tenantID], pages...)
	return nil
}

type fakeListings struct {
	byKey        map[string]*site.Listing
	slugTaken    bool
	conflictSlug string
	upsertErr    error
	unpublished  int
}

func newFakeListings() *fakeListings {
	return &fakeListings{byKey: map[string]*site.Listing{}}
}

func listingKey(tenantID, propertyID string) string { return tenantID + "|" + propertyID }

func (f *fakeListings) Upsert(_ context.Context, l *site.Listing) (*site.Listing, error) {
	if f.upsertErr != nil {
		f.conflictSlug = l.SeoSlug
		err := f.upsertErr
		f.upsertErr = nil
		return nil, err
	}
	if l.ID == "" {
		l.ID = "listing-" + l.PropertyID
	}
	l.CreatedAt = time.Unix(1700000000, 0).UTC()
	l.UpdatedAt = l.CreatedAt
	copy := *l
	f.byKey[listingKey(l.TenantID, l.PropertyID)] = &copy
	return l, nil
}

func (f *fakeListings) GetByProperty(_ context.Context, tenantID, propertyID string) (*site.Listing, error) {
	if l, ok := f.byKey[listingKey(tenantID, propertyID)]; ok {
		copy := *l
		return &copy, nil
	}
	return nil, site.ErrNotFound
}

func (f *fakeListings) SeoSlugTaken(_ context.Context, _ string, seoSlug, _ string) (bool, error) {
	return f.slugTaken || seoSlug == f.conflictSlug, nil
}

func (f *fakeListings) MarkUnpublished(_ context.Context, tenantID, propertyID string) (*site.Listing, error) {
	l, ok := f.byKey[listingKey(tenantID, propertyID)]
	if !ok {
		return nil, site.ErrNotFound
	}
	l.Status = site.ListingUnpublished
	l.UpdatedAt = time.Unix(1700000000, 0).UTC()
	f.unpublished++
	copy := *l
	return &copy, nil
}

func (f *fakeListings) SetPayloadDocID(_ context.Context, tenantID, listingID, payloadDocID string) error {
	for _, l := range f.byKey {
		if l.ID == listingID {
			l.PayloadDocID = payloadDocID
		}
	}
	return nil
}

func (f *fakeListings) GetPublishedForPublic(_ context.Context, tenantID, siteID, propertyID string) (*site.PublicListing, error) {
	for _, l := range f.byKey {
		if l.TenantID == tenantID && l.SiteID == siteID && l.PropertyID == propertyID && l.Status == site.ListingPublished {
			return &site.PublicListing{
				ListingID: l.ID, PropertyID: l.PropertyID, SiteID: l.SiteID,
				TenantID: l.TenantID, SeoSlug: l.SeoSlug, Enabled: true,
			}, nil
		}
	}
	return nil, site.ErrNotFound
}

func (f *fakeListings) CountPublished(context.Context, string) (int64, error) { return 0, nil }

func (f *fakeListings) ListByTenant(context.Context, string, int64, int64) ([]site.Listing, error) {
	return nil, nil
}

type fakeEngagement struct {
	views   []site.ListingView
	leads   []site.Lead
	dup     *site.Lead
	crmFail bool
}

func (f *fakeEngagement) CreateView(_ context.Context, v *site.ListingView) error {
	f.views = append(f.views, *v)
	return nil
}

func (f *fakeEngagement) StatsForListing(_ context.Context, tenantID, propertyID string, _ time.Time) (*site.ListingStats, error) {
	return &site.ListingStats{PropertyID: propertyID}, nil
}

func (f *fakeEngagement) StatsForSite(context.Context, string, time.Time) (*site.SiteStats, error) {
	return &site.SiteStats{}, nil
}

func (f *fakeEngagement) CreateLead(_ context.Context, l *site.Lead) error {
	f.leads = append(f.leads, *l)
	return nil
}

func (f *fakeEngagement) UpdateLead(_ context.Context, l *site.Lead) error {
	for i := range f.leads {
		if f.leads[i].ID == l.ID {
			f.leads[i] = *l
		}
	}
	return nil
}

func (f *fakeEngagement) FindDuplicateLead(context.Context, string, string, string, string, time.Time) (*site.Lead, error) {
	if f.dup != nil {
		copy := *f.dup
		return &copy, nil
	}
	return nil, site.ErrNotFound
}

func (f *fakeEngagement) ListLeads(context.Context, string, int64, int64) ([]site.Lead, int64, error) {
	return f.leads, int64(len(f.leads)), nil
}

type fakeProjection struct {
	ensured     int
	upserts     int
	unpublish   int
	revalid     int
	pages       int
	tenantID    string
	lastEnabled bool
	lastPaths   []string
	err         error
}

func (f *fakeProjection) EnsureTenant(_ context.Context, s *site.Site) (string, error) {
	f.ensured++
	f.lastEnabled = s.Enabled
	if f.err != nil {
		return "", f.err
	}
	f.tenantID = "payload-" + s.TenantID
	return f.tenantID, nil
}

func (f *fakeProjection) UpsertListing(_ context.Context, s *site.Site, l *site.Listing) (string, error) {
	f.upserts++
	if f.err != nil {
		return "", f.err
	}
	return "doc-" + l.PropertyID, nil
}

func (f *fakeProjection) UnpublishListing(context.Context, *site.Site, *site.Listing) error {
	f.unpublish++
	return f.err
}

func (f *fakeProjection) UpsertPage(context.Context, *site.Site, site.Page) error {
	f.pages++
	return f.err
}

func (f *fakeProjection) Revalidate(_ context.Context, _ string, paths []string) error {
	f.revalid++
	f.lastPaths = paths
	return nil
}

type fakeCore struct {
	property *site.PropertySnapshot
	err      error
	calls    int
}

func (f *fakeCore) ListProperties(context.Context, string, int, int) ([]*site.PropertySnapshot, int64, error) {
	f.calls++
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.property == nil {
		return nil, 0, nil
	}
	return []*site.PropertySnapshot{f.property}, 1, nil
}

func (f *fakeCore) GetProperty(context.Context, string, string) (*site.PropertySnapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.property, nil
}

type fakeBus struct {
	names     []string
	published int
	err       error
}

func (f *fakeBus) PublishReliable(_ context.Context, event events.Event) error {
	if f.err != nil {
		return f.err
	}
	f.published++
	f.names = append(f.names, event.Name())
	return nil
}

type fakeReconciler struct {
	calls    int
	tenantID string
	done     chan struct{}
	err      error
}

func (f *fakeReconciler) ReconcileTenant(_ context.Context, tenantID string) error {
	f.calls++
	f.tenantID = tenantID
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

type fakeCRM struct {
	calls    int
	tenantID string
	err      error
}

func (f *fakeCRM) CreateLead(_ context.Context, tenantID string, _ site.Lead) (string, error) {
	f.calls++
	f.tenantID = tenantID
	if f.err != nil {
		return "", f.err
	}
	return "crm-" + tenantID, nil
}

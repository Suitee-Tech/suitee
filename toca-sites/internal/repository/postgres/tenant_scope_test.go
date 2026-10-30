package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// Contrato de isolamento sem Postgres (mesmo padrão de
// toca-ops/internal/repositories/postgres/tenant_scope_test.go). O serviço usa
// o pool de aplicação com BYPASSRLS, então o WHERE tenant_id é a fronteira
// real: se ele sumir de uma query, nada mais protege.
func TestTenantScopedQueriesKeepTenantFilter(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"sites.GetByTenant", siteByTenantQuery},
		{"sites.Update", siteUpdateQuery},
		{"pages.List", pagesBySiteQuery},
		{"pages.GetByKey", pageByKeyQuery},
		{"listings.GetByProperty", listingByPropertyQuery},
		{"listings.Upsert", listingUpsertQuery},
		{"listings.Unpublish", listingUnpublishQuery},
		{"listings.SetPayloadDocID", listingPayloadDocQuery},
		{"listings.CountPublished", listingCountPublishedQuery},
		{"views.StatsForListing", listingStatsQuery},
		{"leads.StatsForListing", leadStatsQuery},
		{"leads.Create", leadInsertQuery},
		{"leads.Update", leadUpdateQuery},
		{"leads.FindDuplicate", leadDuplicateQuery},
		{"leads.List", leadListQuery},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.query, "tenant_id = $1") && !strings.Contains(tc.query, "tenant_id, ") {
				t.Errorf("%s perdeu o filtro de tenant:\n%s", tc.name, tc.query)
			}
		})
	}
}

// A resolução pública por slug é a única escotilha sem tenant. Ela não pode
// vazar site desligado nem aceitar slug vazio.
func TestPublicResolutionIsExplicit(t *testing.T) {
	if !strings.Contains(siteBySlugQuery, "WHERE slug = $1") {
		t.Fatalf("resolução pública deve ser por slug: %s", siteBySlugQuery)
	}
	if strings.Contains(siteBySlugQuery, "tenant_id =") {
		t.Fatal("resolução por Host não recebe tenant")
	}
	if !strings.Contains(listingPublicQuery, "l.tenant_id = $1") || !strings.Contains(listingPublicQuery, "l.site_id = $2") {
		t.Fatal("validação pública da listing precisa ser tenant+site scoped")
	}
}

// Fail closed: sem tenant não há escopo de autorização. O db é nil de
// propósito — se a checagem não acontecer antes, o teste panicaria.
func TestRepositoryFailsClosedWithoutTenant(t *testing.T) {
	ctx := context.Background()
	repo := &Repository{}

	if _, err := repo.GetByTenant(ctx, "  "); err == nil {
		t.Error("GetByTenant sem tenant deveria falhar")
	}
	if _, err := repo.FindBySlug(ctx, ""); err == nil {
		t.Error("FindBySlug sem slug deveria falhar")
	}
	listing := siteListingFixture()
	if _, err := repo.Upsert(ctx, &listing); err == nil {
		t.Error("Upsert sem tenant deveria falhar")
	}
	if _, err := repo.GetByProperty(ctx, "", "prop"); err == nil {
		t.Error("GetByProperty sem tenant deveria falhar")
	}
	if _, err := repo.GetByProperty(ctx, "tenant", ""); err == nil {
		t.Error("GetByProperty sem property id deveria falhar")
	}
	if _, err := repo.MarkUnpublished(ctx, "", "prop"); err == nil {
		t.Error("MarkUnpublished sem tenant deveria falhar")
	}
	if _, err := repo.StatsForListing(ctx, "", "prop", nowFixture()); err == nil {
		t.Error("StatsForListing sem tenant deveria falhar")
	}
	if _, err := repo.StatsForSite(ctx, "", nowFixture()); err == nil {
		t.Error("StatsForSite sem tenant deveria falhar")
	}
	if _, _, err := repo.ListLeads(ctx, "", 10, 0); err == nil {
		t.Error("ListLeads sem tenant deveria falhar")
	}
	if _, err := repo.FindDuplicateLead(ctx, "", "prop", "a@b.c", "", nowFixture()); err == nil {
		t.Error("FindDuplicateLead sem tenant deveria falhar")
	}
	if _, err := repo.ListPages(ctx, "", "site"); err == nil {
		t.Error("ListPages sem tenant deveria falhar")
	}
	if _, err := repo.GetPage(ctx, "", "site", "home"); err == nil {
		t.Error("GetPage sem tenant deveria falhar")
	}
	if err := repo.UpsertPages(ctx, "", "site", nil); err == nil {
		t.Error("UpsertPages sem tenant deveria falhar")
	}
	lead := siteLeadFixture()
	if err := repo.CreateLead(ctx, &lead); err == nil {
		t.Error("CreateLead sem tenant deveria falhar")
	}
	view := siteViewFixture()
	if err := repo.CreateView(ctx, &view); err == nil {
		t.Error("CreateView sem tenant deveria falhar")
	}
}

func TestMapConstraintTranslatesSlugCollision(t *testing.T) {
	err := mapConstraint(errText("duplicate key value violates unique constraint \"uq_sites_sites_slug\""))
	if !errors.Is(err, site.ErrSlugTaken) {
		t.Fatalf("violação de slug deveria virar ErrSlugTaken, veio %v", err)
	}
	if mapConstraint(nil) != nil {
		t.Fatal("nil deve continuar nil")
	}
}

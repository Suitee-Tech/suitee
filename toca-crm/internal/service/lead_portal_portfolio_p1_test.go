package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// Revisão de acesso a imóveis (2026-09-19), itens A7 e "portal sem negócio":
//
//  A7 — o portal exibia o SNAPSHOT gravado no dia da recomendação. Imóvel
//       vendido continuava sugerido, e o valor exibido era o antigo.
//  Vitrine — quem não tinha negócio aberto só via recomendação/match do lead,
//       porque a busca do portal é `deal-scoped`.
type crossPortfolioStub struct {
	snapshots   map[string]*repository.PropertySnapshot
	snapshotErr error
	portfolio   []*repository.AvailableProperty
	portfolioErr error
	lastTenants []string
	lastListing string
}

func (s *crossPortfolioStub) ActivePartnershipsForCompany(context.Context, string) ([]*repository.PartnershipSharing, error) {
	return nil, nil
}

func (s *crossPortfolioStub) PropertySnapshot(_ context.Context, _, propertyID string) (*repository.PropertySnapshot, error) {
	if s.snapshotErr != nil {
		return nil, s.snapshotErr
	}
	return s.snapshots[propertyID], nil
}

func (s *crossPortfolioStub) AvailableProperties(_ context.Context, tenantIDs []string, listingType string, _ int) ([]*repository.AvailableProperty, error) {
	s.lastTenants, s.lastListing = tenantIDs, listingType
	if s.portfolioErr != nil {
		return nil, s.portfolioErr
	}
	return s.portfolio, nil
}

func newPortfolioService(repo lead.Repository, cross *crossPortfolioStub) *LeadService {
	svc := &LeadService{repo: repo, bootstrapping: map[string]bool{}}
	if cross != nil {
		svc.crossRepo = cross
	}
	return svc
}

func TestClientPortalOverviewDropsRecommendationOfUnavailableProperty(t *testing.T) {
	recommended := &lead.Lead{
		ID: "lead-1", TenantID: "tenant-A", Name: "Cliente", AccountID: "account-1",
		RecommendedPropertyID: "imovel-1",
		RecommendedPropertySnapshot: &lead.RecommendedPropertySnapshot{
			ID: "imovel-1", Title: "Casa (snapshot antigo)", ListingType: "sale", SalePrice: 1000000,
		},
	}
	cross := &crossPortfolioStub{snapshots: map[string]*repository.PropertySnapshot{
		// O imóvel existe no portfólio, mas está VENDIDO.
		"imovel-1": {ID: "imovel-1", TenantID: "tenant-A", Title: "Casa", Status: "sold", ListingType: "sale", SalePrice: 1180000},
	}}
	svc := newPortfolioService(newAcceptRepo(recommended), cross)

	out, err := svc.ClientPortalOverview(pkgCtxWithTenant("tenant-A"), "account-1", "cliente@example.test")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("leads = %d", len(out))
	}
	if out[0].RecommendedPropertyID != "" || out[0].RecommendedProperty != nil {
		t.Fatalf("imóvel VENDIDO continuou recomendado no portal: id=%q prop=%+v",
			out[0].RecommendedPropertyID, out[0].RecommendedProperty)
	}
}

func TestClientPortalOverviewRefreshesValueFromLiveProperty(t *testing.T) {
	recommended := &lead.Lead{
		ID: "lead-2", TenantID: "tenant-A", Name: "Cliente", AccountID: "account-2",
		RecommendedPropertyID: "imovel-2",
		RecommendedPropertySnapshot: &lead.RecommendedPropertySnapshot{
			ID: "imovel-2", Title: "Apartamento", ListingType: "sale", SalePrice: 400000,
		},
	}
	cross := &crossPortfolioStub{snapshots: map[string]*repository.PropertySnapshot{
		// Mesmo imóvel, valor ATUAL maior e ainda disponível.
		"imovel-2": {
			ID: "imovel-2", TenantID: "tenant-A", Title: "Apartamento reformado", Status: "published",
			ListingType: "sale", SalePrice: 465000, ThumbnailURL: "https://files.example/a.jpg",
		},
	}}
	svc := newPortfolioService(newAcceptRepo(recommended), cross)

	out, err := svc.ClientPortalOverview(pkgCtxWithTenant("tenant-A"), "account-2", "cliente@example.test")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	prop := out[0].RecommendedProperty
	if prop == nil {
		t.Fatal("recomendação de imóvel disponível desapareceu")
	}
	if prop.SalePrice != 465000 {
		t.Fatalf("valor exibido = %.2f, want o ATUAL (465000) e não o do snapshot (400000)", prop.SalePrice)
	}
	if prop.Title != "Apartamento reformado" {
		t.Fatalf("título exibido = %q, want o atual", prop.Title)
	}
}

func TestClientPortalOverviewKeepsSnapshotWhenLiveReadFails(t *testing.T) {
	recommended := &lead.Lead{
		ID: "lead-3", TenantID: "tenant-A", Name: "Cliente", AccountID: "account-3",
		RecommendedPropertyID: "imovel-3",
		RecommendedPropertySnapshot: &lead.RecommendedPropertySnapshot{
			ID: "imovel-3", Title: "Casa", ListingType: "sale", SalePrice: 900000,
		},
	}
	// Sem cross repo (deploy sem o lookup) o snapshot gravado continua sendo a
	// resposta — indisponibilidade de leitura não pode apagar a recomendação.
	svc := newPortfolioService(newAcceptRepo(recommended), nil)

	out, err := svc.ClientPortalOverview(pkgCtxWithTenant("tenant-A"), "account-3", "cliente@example.test")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if out[0].RecommendedProperty == nil || out[0].RecommendedProperty.SalePrice != 900000 {
		t.Fatalf("snapshot não foi usado como fallback: %+v", out[0].RecommendedProperty)
	}

	// Erro de leitura (banco fora, por exemplo) tem o mesmo tratamento.
	cross := &crossPortfolioStub{snapshotErr: errors.New("db down")}
	svc = newPortfolioService(newAcceptRepo(recommended), cross)
	out, err = svc.ClientPortalOverview(pkgCtxWithTenant("tenant-A"), "account-3", "cliente@example.test")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if out[0].RecommendedProperty == nil || out[0].RecommendedProperty.SalePrice != 900000 {
		t.Fatalf("erro de leitura apagou a recomendação: %+v", out[0].RecommendedProperty)
	}
}

func TestClientPortalPortfolioUsesTenantsOfTheClientsLeads(t *testing.T) {
	leads := []*lead.Lead{
		{ID: "lead-a", TenantID: "tenant-A", AccountID: "account-9"},
		{ID: "lead-b", TenantID: "tenant-B", AccountID: "account-9"},
		{ID: "lead-c", TenantID: "tenant-A", AccountID: "account-9"}, // duplicado de propósito
	}
	cross := &crossPortfolioStub{portfolio: []*repository.AvailableProperty{
		{ID: "imovel-1", Title: "Casa", ListingType: "rent", Price: 3200},
	}}
	svc := newPortfolioService(newAcceptRepo(leads...), cross)

	out, err := svc.ClientPortalPortfolio(pkgCtxWithTenant("tenant-A"), "account-9", "cliente@example.test", "rent", 30)
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if len(out) != 1 || out[0].ID != "imovel-1" {
		t.Fatalf("vitrine = %+v", out)
	}
	if len(cross.lastTenants) != 2 {
		t.Fatalf("tenants consultados = %v, want os dois distintos dos leads", cross.lastTenants)
	}
	if cross.lastListing != "rent" {
		t.Fatalf("finalidade consultada = %q", cross.lastListing)
	}
}

func TestClientPortalPortfolioWithoutLeadsDoesNotQueryProperties(t *testing.T) {
	cross := &crossPortfolioStub{}
	svc := newPortfolioService(newAcceptRepo(), cross)

	out, err := svc.ClientPortalPortfolio(pkgCtxWithTenant("tenant-A"), "account-sem-lead", "", "sale", 30)
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("vitrine = %+v, want vazia", out)
	}
	if cross.lastTenants != nil {
		t.Fatalf("consulta de imóveis sem lead do cliente: %v", cross.lastTenants)
	}
}

func TestClientPortalPortfolioFailsClosedWithoutCrossRepo(t *testing.T) {
	svc := newPortfolioService(newAcceptRepo(&lead.Lead{ID: "lead-x", TenantID: "tenant-A", AccountID: "account-x"}), nil)
	if _, err := svc.ClientPortalPortfolio(pkgCtxWithTenant("tenant-A"), "account-x", "", "sale", 30); err == nil {
		t.Fatal("vitrine sem lookup configurado deveria falhar (fail closed)")
	}
}

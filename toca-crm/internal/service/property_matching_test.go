package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }

// Helpers para deixar os testes de scoring legíveis. Pequenas builders
// para LeadProperty + PropertyDTO sem inicializar campos irrelevantes.
func leadPref(t *testing.T) lead.LeadProperty {
	t.Helper()
	return lead.LeadProperty{
		PriceRange:        lead.PriceRange{Min: 300_000, Max: 500_000},
		PreferredLocation: "Pinheiros",
		Features: lead.PropertyFeatures{
			Bedrooms: intPtr(3),
			AreaMin:  floatPtr(80),
			AreaMax:  floatPtr(150),
		},
	}
}

func propDTO(price, area float64, beds int, neighborhood string) clients.PropertyDTO {
	return clients.PropertyDTO{
		ID:           "p-1",
		Title:        "Apto",
		PropertyType: "apartment",
		ListingType:  "sale",
		Neighborhood: neighborhood,
		City:         "São Paulo",
		SalePrice:    price,
		AreaTotal:    area,
		Bedrooms:     beds,
	}
}

// ─────────────────────────── priceScore ──────────────────────────────

func TestPriceScore_WithinRange_ReturnsFullScore(t *testing.T) {
	r := lead.PriceRange{Min: 300_000, Max: 500_000}
	got := priceScore(r, 400_000)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestPriceScore_BelowMin_DecreasesLinearly(t *testing.T) {
	r := lead.PriceRange{Min: 300_000, Max: 500_000}
	// 25% abaixo do mínimo = 0.5 (perdeu metade da janela de 50%)
	got := priceScore(r, 225_000)
	if math.Abs(got-0.5) > 0.001 {
		t.Errorf("expected ~0.5 at 25%% undershoot, got %f", got)
	}
}

func TestPriceScore_AboveMax_DecreasesLinearly(t *testing.T) {
	r := lead.PriceRange{Min: 300_000, Max: 500_000}
	got := priceScore(r, 750_000) // 50% acima do max → 0
	if got != 0 {
		t.Errorf("expected 0 at 50%% overshoot, got %f", got)
	}
}

func TestPriceScore_OnlyMax_TreatedAsLessOrEqual(t *testing.T) {
	// Lead só preenche o teto: "quero gastar até 500k". Qualquer preço
	// <= max bate 1.0; acima decresce.
	r := lead.PriceRange{Max: 500_000}
	if got := priceScore(r, 400_000); got != 1.0 {
		t.Errorf("price=400k under max=500k should be 1.0, got %f", got)
	}
	if got := priceScore(r, 750_000); got != 0 {
		t.Errorf("price=750k over max=500k (50%%) should be 0, got %f", got)
	}
}

func TestPriceScore_OnlyMin_TreatedAsGreaterOrEqual(t *testing.T) {
	r := lead.PriceRange{Min: 300_000}
	if got := priceScore(r, 400_000); got != 1.0 {
		t.Errorf("price=400k over min=300k should be 1.0, got %f", got)
	}
	if got := priceScore(r, 150_000); got != 0 {
		t.Errorf("price=150k under min=300k (50%%) should be 0, got %f", got)
	}
}

func TestPriceScore_PropertyHasZeroPrice_Returns0(t *testing.T) {
	// Imóvel sem preço listado não dá pra comparar.
	r := lead.PriceRange{Min: 300_000, Max: 500_000}
	if got := priceScore(r, 0); got != 0 {
		t.Errorf("price=0 should score 0, got %f", got)
	}
}

// ─────────────────────────── locationScore ───────────────────────────

func TestLocationScore_ExactMatchInNeighborhood(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "Pinheiros", City: "São Paulo"}
	if got := locationScore("Pinheiros", prop); got != 1.0 {
		t.Errorf("exact neighborhood match should be 1.0, got %f", got)
	}
}

func TestLocationScore_CaseInsensitive(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "PINHEIROS", City: "São Paulo"}
	if got := locationScore("pinheiros", prop); got != 1.0 {
		t.Errorf("case-insensitive should match, got %f", got)
	}
}

func TestLocationScore_MultipleTermsAnyMatchHits1(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "Vila Madalena"}
	// Termos separados por vírgula: qualquer um que bate → 1.0.
	if got := locationScore("Pinheiros, Vila Madalena, Itaim", prop); got != 1.0 {
		t.Errorf("multi-term ANY match should be 1.0, got %f", got)
	}
}

func TestLocationScore_NoMatchReturnsZero(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "Liberdade", City: "São Paulo"}
	if got := locationScore("Pinheiros", prop); got != 0 {
		t.Errorf("no match should be 0, got %f", got)
	}
}

func TestLocationScore_PrefixPartialMatch(t *testing.T) {
	// "Centr" deve dar match parcial em "Centro" (5+ chars prefix).
	prop := clients.PropertyDTO{Neighborhood: "Centro"}
	got := locationScore("centrico", prop)
	if got != 0.7 {
		t.Errorf("prefix match should score 0.7, got %f", got)
	}
}

func TestLocationScore_EmptyPreferred(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "Pinheiros"}
	if got := locationScore("", prop); got != 0 {
		t.Errorf("empty preferred should be 0, got %f", got)
	}
}

func TestLocationScore_SeparatorsNormalized(t *testing.T) {
	prop := clients.PropertyDTO{Neighborhood: "Itaim"}
	// "/" e ";" também separam termos.
	if got := locationScore("Pinheiros / Itaim ; Bela Vista", prop); got != 1.0 {
		t.Errorf("expected match with mixed separators, got %f", got)
	}
}

// ─────────────────────────── areaScore ───────────────────────────────

func TestAreaScore_WithinRange(t *testing.T) {
	if got := areaScore(floatPtr(80), floatPtr(150), 100); got != 1.0 {
		t.Errorf("100m² in [80,150] should be 1.0, got %f", got)
	}
}

func TestAreaScore_BelowMin(t *testing.T) {
	// 60m² when min=80 → 25% undershoot → 0.5
	got := areaScore(floatPtr(80), floatPtr(150), 60)
	if math.Abs(got-0.5) > 0.001 {
		t.Errorf("undershoot 25%% should be 0.5, got %f", got)
	}
}

func TestAreaScore_OnlyMax(t *testing.T) {
	if got := areaScore(nil, floatPtr(100), 90); got != 1.0 {
		t.Errorf("only-max within: expected 1.0, got %f", got)
	}
}

func TestAreaScore_OnlyMin(t *testing.T) {
	if got := areaScore(floatPtr(50), nil, 100); got != 1.0 {
		t.Errorf("only-min above: expected 1.0, got %f", got)
	}
}

func TestAreaScore_NoArea(t *testing.T) {
	if got := areaScore(floatPtr(80), floatPtr(150), 0); got != 0 {
		t.Errorf("property without area should be 0, got %f", got)
	}
}

// ─────────────────────────── bedroomsScore ───────────────────────────

func TestBedroomsScore_ExactMatch(t *testing.T) {
	if got := bedroomsScore(3, 3); got != 1.0 {
		t.Errorf("3==3 should be 1.0, got %f", got)
	}
}

func TestBedroomsScore_OneMoreThanRequested(t *testing.T) {
	// +1 quarto extra ainda é considered match (1 a mais é tolerado).
	if got := bedroomsScore(3, 4); got != 1.0 {
		t.Errorf("3 requested, 4 found: still 1.0, got %f", got)
	}
}

func TestBedroomsScore_TooMany_PenaltyApplied(t *testing.T) {
	// +2 a mais: 0.85; +3: 0.7
	if got := bedroomsScore(2, 4); math.Abs(got-0.85) > 0.01 {
		t.Errorf("over-sized by 2 should be ~0.85, got %f", got)
	}
	if got := bedroomsScore(2, 5); math.Abs(got-0.7) > 0.01 {
		t.Errorf("over-sized by 3 should be ~0.7, got %f", got)
	}
}

func TestBedroomsScore_BelowRequested_DecreasesLinear(t *testing.T) {
	// 3 requested, 2 found: 0.5; 1 found: 0.
	if got := bedroomsScore(3, 2); got != 0.5 {
		t.Errorf("undersized by 1: expected 0.5, got %f", got)
	}
	if got := bedroomsScore(3, 1); got != 0 {
		t.Errorf("undersized by 2: expected 0, got %f", got)
	}
}

func TestBedroomsScore_PropertyZero(t *testing.T) {
	if got := bedroomsScore(3, 0); got != 0 {
		t.Errorf("property with 0 bedrooms should score 0, got %f", got)
	}
}

// ─────────────────────────── redistributeWeights ─────────────────────

func TestRedistributeWeights_AllActive_KeepsBase(t *testing.T) {
	got := redistributeWeights(DefaultMatchWeights, true, true, true, true)
	if got.Price != 50 || got.Location != 30 || got.Area != 10 || got.Bedrooms != 10 {
		t.Errorf("all-active should preserve base weights, got %+v", got)
	}
}

func TestRedistributeWeights_OnlyPriceAndLocation(t *testing.T) {
	got := redistributeWeights(DefaultMatchWeights, true, true, false, false)
	// 25 + 15 = 40 base → multiplica por 100/40 = 2.5.
	// Price = 25 * 2.5 = 62.5; Location = 15 * 2.5 = 37.5.
	if math.Abs(got.Price-62.5) > 0.1 {
		t.Errorf("expected Price≈62.5, got %f", got.Price)
	}
	if math.Abs(got.Location-37.5) > 0.1 {
		t.Errorf("expected Location≈37.5, got %f", got.Location)
	}
	if got.Area != 0 || got.Bedrooms != 0 {
		t.Errorf("inactive weights should be 0, got %+v", got)
	}
	// Sum should add to ~100.
	total := got.Price + got.Location + got.Area + got.Bedrooms
	if math.Abs(total-100) > 0.01 {
		t.Errorf("redistributed weights should sum to 100, got %f", total)
	}
}

func TestRedistributeWeights_NoneActive(t *testing.T) {
	got := redistributeWeights(DefaultMatchWeights, false, false, false, false)
	if got.Price != 0 || got.Location != 0 || got.Area != 0 || got.Bedrooms != 0 {
		t.Errorf("no-active should be zero weights, got %+v", got)
	}
}

// ─────────────────────────── ScoreProperty (integração) ──────────────

func TestScoreProperty_PerfectMatch(t *testing.T) {
	pref := leadPref(t)
	prop := propDTO(400_000, 120, 3, "Pinheiros")
	bk := ScoreProperty(pref, prop, DefaultMatchWeights)
	if total := bk.Total(); math.Abs(total-100) > 0.01 {
		t.Errorf("perfect match should be ~100, got %f (breakdown=%+v)", total, bk)
	}
}

func TestScoreProperty_PartialMatch_PriceOff(t *testing.T) {
	// Lead quer 300-500k, achamos imóvel de 750k (50% acima) — preço cai
	// pra 0; outros critérios bater perfeito → score ≈ 50 (Location 30 +
	// Area 10 + Bedrooms 10 após a redistribuição dos pesos atuais).
	pref := leadPref(t)
	prop := propDTO(750_000, 120, 3, "Pinheiros")
	bk := ScoreProperty(pref, prop, DefaultMatchWeights)
	total := bk.Total()
	if total > 55 || total < 45 {
		t.Errorf("partial match with price 0 should be ~50, got %f (breakdown=%+v)", total, bk)
	}
}

func TestScoreProperty_LeadWithoutLocation_PriceWeightUp(t *testing.T) {
	pref := leadPref(t)
	pref.PreferredLocation = "" // remove location preference
	prop := propDTO(400_000, 120, 3, "QualquerBairro")
	bk := ScoreProperty(pref, prop, DefaultMatchWeights)
	// Sem location, o peso é redistribuído entre preço, área e quartos.
	// Match perfeito nos 3 restantes → 100.
	if total := bk.Total(); math.Abs(total-100) > 0.01 {
		t.Errorf("perfect match minus location should still be ~100, got %f", total)
	}
	if bk.UsedWeights.Location != 0 {
		t.Errorf("expected used Location weight=0, got %f", bk.UsedWeights.Location)
	}
}

func TestScoreProperty_NoPreferences_ScoresZero(t *testing.T) {
	bk := ScoreProperty(lead.LeadProperty{}, propDTO(400_000, 120, 3, "Pinheiros"), DefaultMatchWeights)
	if total := bk.Total(); total != 0 {
		t.Errorf("lead with no preferences should score 0, got %f", total)
	}
}

// ─────────────────────────── MatchProperties (orchestration) ─────────

type stubPropertyLookup struct {
	returned []clients.PropertyDTO
	err      error
	lastReq  clients.PropertyFilterDTO
}

func (s *stubPropertyLookup) SearchProperties(_ context.Context, f clients.PropertyFilterDTO) ([]clients.PropertyDTO, error) {
	s.lastReq = f
	return s.returned, s.err
}

func TestMatchProperties_RanksAndCutsByLimit(t *testing.T) {
	// 3 imóveis: um perfeito, um ok, um ruim. Limit=2 → devolve os 2
	// melhores ordenados desc por score.
	pref := leadPref(t)
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-1",
		Role:             lead.ClientRoleBuyer,
		PropertyInterest: &pref,
	}
	repo := &fakeMatchRepo{lead: leadDoc}

	candidates := []clients.PropertyDTO{
		// Match perfeito.
		propDTO(400_000, 120, 3, "Pinheiros"),
		// Preço ruim (acima), resto bom.
		propDTO(800_000, 120, 3, "Pinheiros"),
		// Localização errada + área errada (continua acima do cutoff 30).
		propDTO(400_000, 300, 3, "Liberdade"),
	}
	candidates[0].ID = "best"
	candidates[1].ID = "ok"
	candidates[2].ID = "weak"

	lookup := &stubPropertyLookup{returned: candidates}
	svc := &LeadService{repo: repo, propertyClient: lookup}

	res, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 2)
	if err != nil {
		t.Fatalf("MatchProperties: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("limit=2 should return exactly 2, got %d", len(res))
	}
	if res[0].Property.ID != "best" {
		t.Errorf("expected 'best' to rank first, got %q", res[0].Property.ID)
	}
	if res[0].Score < res[1].Score {
		t.Errorf("results not ranked desc: %v vs %v", res[0].Score, res[1].Score)
	}
}

func TestMatchProperties_AppliesListingTypeFilterByRole(t *testing.T) {
	pref := leadPref(t)
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-1",
		Role:             lead.ClientRoleTenant,
		PropertyInterest: &pref,
	}
	repo := &fakeMatchRepo{lead: leadDoc}
	lookup := &stubPropertyLookup{returned: []clients.PropertyDTO{}}
	svc := &LeadService{repo: repo, propertyClient: lookup}

	_, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if err != nil {
		t.Fatalf("MatchProperties: %v", err)
	}
	if lookup.lastReq.AvailableFor != "rent" {
		t.Errorf("tenant lead should filter available_for=rent, got %q", lookup.lastReq.AvailableFor)
	}
}

func TestMatchProperties_UsesBusinessWeightForOperation(t *testing.T) {
	pref := leadPref(t)
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-1",
		Role:             lead.ClientRoleTenant,
		PropertyInterest: &pref,
	}
	rental := propDTO(400_000, 120, 3, "Pinheiros")
	rental.ListingType = "rent"
	rental.SalePrice = 0
	rental.RentPrice = 400_000
	lookup := &stubPropertyLookup{returned: []clients.PropertyDTO{rental}}
	svc := &LeadService{repo: &fakeMatchRepo{lead: leadDoc}, propertyClient: lookup}

	res, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if err != nil {
		t.Fatalf("MatchProperties: %v", err)
	}
	if len(res) != 1 || res[0].Reasons.Business != 50 || res[0].Reasons.UsedWeights.Business != 50 {
		t.Fatalf("rental operation should contribute the dominant fixed weight, got %+v", res)
	}
}

func TestMatchProperties_FiltersPropertyTypeAndUnavailableInventory(t *testing.T) {
	pref := leadPref(t)
	pref.Type = lead.PropertyTypeApartment
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-1",
		Role:             lead.ClientRoleBuyer,
		PropertyInterest: &pref,
	}
	lookup := &stubPropertyLookup{returned: []clients.PropertyDTO{
		{ID: "house", PropertyType: "house", ListingType: "sale", Status: "published", SalePrice: 400_000, Neighborhood: "Pinheiros", AreaTotal: 120, Bedrooms: 3},
		{ID: "rent-only", PropertyType: "apartment", ListingType: "sale", AvailableFor: []string{"rent"}, Status: "published", SalePrice: 400_000, Neighborhood: "Pinheiros", AreaTotal: 120, Bedrooms: 3},
		{ID: "sold", PropertyType: "apartment", ListingType: "sale", Status: "sold", SalePrice: 400_000, Neighborhood: "Pinheiros", AreaTotal: 120, Bedrooms: 3},
		{ID: "apartment", PropertyType: "apartment", ListingType: "sale", Status: "published", SalePrice: 400_000, Neighborhood: "Pinheiros", AreaTotal: 120, Bedrooms: 3},
	}}
	svc := &LeadService{repo: &fakeMatchRepo{lead: leadDoc}, propertyClient: lookup}

	res, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if err != nil {
		t.Fatalf("MatchProperties: %v", err)
	}
	if lookup.lastReq.PropertyType != "apartment" {
		t.Fatalf("expected property type filter, got %q", lookup.lastReq.PropertyType)
	}
	if len(res) != 1 || res[0].Property.ID != "apartment" {
		t.Fatalf("expected only available apartment, got %+v", res)
	}
}

func TestMatchProperties_LeadHasNoPreferences(t *testing.T) {
	leadDoc := &lead.Lead{
		ID:       "lead-1",
		TenantID: "tenant-1",
		Role:     lead.ClientRoleBuyer,
		// PropertyInterest deliberately nil
	}
	repo := &fakeMatchRepo{lead: leadDoc}
	svc := &LeadService{repo: repo, propertyClient: &stubPropertyLookup{}}

	_, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if !errors.Is(err, ErrLeadHasNoPreferences) {
		t.Errorf("expected ErrLeadHasNoPreferences, got %v", err)
	}
}

func TestMatchProperties_CrossTenant_NotFound(t *testing.T) {
	pref := leadPref(t)
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-A",
		Role:             lead.ClientRoleBuyer,
		PropertyInterest: &pref,
	}
	repo := &fakeMatchRepo{lead: leadDoc}
	svc := &LeadService{repo: repo, propertyClient: &stubPropertyLookup{}}

	_, err := svc.MatchProperties(context.Background(), "tenant-B", "lead-1", 10)
	if err == nil || err.Error() != "lead not found" {
		t.Errorf("cross-tenant should be hidden as 'lead not found', got %v", err)
	}
}

func TestMatchProperties_CutsOffLowScores(t *testing.T) {
	pref := leadPref(t)
	leadDoc := &lead.Lead{
		ID:               "lead-1",
		TenantID:         "tenant-1",
		Role:             lead.ClientRoleBuyer,
		PropertyInterest: &pref,
	}
	repo := &fakeMatchRepo{lead: leadDoc}
	// Imóvel com tudo ruim: preço dobrado, localização errada, área 5x,
	// quartos absurdamente over. Só a modalidade bate, então score=50 e o
	// novo mínimo de qualidade para matches com modalidade (<60) corta.
	bad := clients.PropertyDTO{
		ID: "bad", Title: "Bad", SalePrice: 5_000_000, AreaTotal: 1000,
		Bedrooms: 10, Neighborhood: "Outro", ListingType: "sale",
	}
	lookup := &stubPropertyLookup{returned: []clients.PropertyDTO{bad}}
	svc := &LeadService{repo: repo, propertyClient: lookup}

	res, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if err != nil {
		t.Fatalf("MatchProperties: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("very bad match should be cut off, got %d results", len(res))
	}
}

func TestMatchProperties_NoPropertyClient_ReturnsConfigError(t *testing.T) {
	svc := &LeadService{repo: &fakeMatchRepo{}}
	_, err := svc.MatchProperties(context.Background(), "tenant-1", "lead-1", 10)
	if err == nil || err.Error() != "property lookup client not configured" {
		t.Errorf("expected config error, got %v", err)
	}
}

// ─────────────────────────── fake repo ───────────────────────────────

// fakeMatchRepo é o mínimo da lead.Repository que o MatchProperties usa
// (só GetByID). Outros métodos retornam zero-values.
type fakeMatchRepo struct {
	lead.Repository
	lead *lead.Lead
}

// GetByID espelha o contrato tenant-scoped do repositório real (achado C9).
func (f *fakeMatchRepo) GetByID(_ context.Context, tenantID, _ string) (*lead.Lead, error) {
	if f.lead == nil {
		return nil, nil
	}
	if tenantID != "" && f.lead.TenantID != "" && f.lead.TenantID != tenantID {
		return nil, nil
	}
	return f.lead, nil
}

func (s *stubPropertyLookup) GetPropertyByID(_ context.Context, id string) (*clients.PropertyDTO, error) {
	if s.err != nil {
		return nil, s.err
	}
	for _, property := range s.returned {
		if property.ID == id {
			copy := property
			return &copy, nil
		}
	}
	return nil, nil
}

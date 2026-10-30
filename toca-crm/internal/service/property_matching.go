package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/propertymatch"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// property_matching.go — Match Properties / Cruzamento Inteligente (#415).
//
// Resolve a pergunta "quais imóveis disponíveis batem com as preferências
// deste lead?" via score ponderado. Algoritmo isolado em função pura
// (`ScoreProperty`) pra ser testado sem mexer em rede; o método de
// serviço lá embaixo só orquestra (lookup do lead, fetch de properties,
// rankeamento).
//
// Pesos por critério (audit J02 #415 AC):
//   - Tipo de negócio: 50 (compra/venda vs locação)
//   - Faixa de preço:  25
//   - Localização:     15
//   - Área (m²):        5
//   - Quartos:          5
//
// Quando o lead NÃO declarou um critério (ex.: sem preferred_location),
// o peso desse critério é redistribuído entre os outros critérios opcionais;
// os 50 pontos da modalidade permanecem fixos — assim leads com
// preferências parciais não tiram nota baixa por omissão nem supervalorizam
// um match apenas de compra/locação.

// MatchedProperty agrega um imóvel candidato + o score final + breakdown
// detalhado para o frontend explicar ao usuário por que o match foi bom
// ou ruim. Mantém PropertyClientResult inteiro (não enxuga) — o
// resumo/foto/preço vem direto pro card "Sugestões" sem segundo round-trip.
type MatchedProperty struct {
	Property clients.PropertyDTO `json:"property"`
	Score    float64             `json:"score"`   // 0..100, com 2 casas decimais
	Reasons  MatchBreakdown      `json:"reasons"` // explicabilidade por critério
}

// MatchBreakdown — contribuição de cada dimensão para o score final.
// Soma esperada == Score quando todos os critérios tinham peso original.
// Quando o lead omite um critério, sua contribuição vira 0 e os outros
// pesos sobem proporcionalmente — o frontend pode renderizar isso em
// formato de barra ("Negócio 50/50, Preço 25/25, …").
type MatchBreakdown struct {
	Business    float64 `json:"business"`     // 0..BusinessWeight
	Price       float64 `json:"price"`        // 0..PriceWeight
	Location    float64 `json:"location"`     // 0..LocationWeight
	Area        float64 `json:"area"`         // 0..AreaWeight
	Bedrooms    float64 `json:"bedrooms"`     // 0..BedroomsWeight
	UsedWeights Weights `json:"used_weights"` // pesos efetivos após redistribuição
}

// Weights — pesos por critério, configuráveis caso queiramos per-tenant no
// futuro. Os valores padrão seguem a regra de modalidade dominante.
type Weights struct {
	Business float64 `json:"business"`
	Price    float64 `json:"price"`
	Location float64 `json:"location"`
	Area     float64 `json:"area"`
	Bedrooms float64 `json:"bedrooms"`
}

func toMatchPreferences(p lead.LeadProperty) propertymatch.Preferences {
	return toMatchPreferencesForOperation(p, "")
}

func toMatchPreferencesForOperation(p lead.LeadProperty, businessType string) propertymatch.Preferences {
	return propertymatch.Preferences{
		Type:              string(p.Type),
		BusinessType:      businessType,
		PreferredLocation: p.PreferredLocation,
		PriceRange:        propertymatch.PriceRange{Min: p.PriceRange.Min, Max: p.PriceRange.Max},
		Bedrooms:          p.Features.Bedrooms,
		AreaMin:           p.Features.AreaMin,
		AreaMax:           p.Features.AreaMax,
	}
}

func toMatchProperty(p clients.PropertyDTO) propertymatch.Property {
	return propertymatch.Property{
		ID:           p.ID,
		PropertyType: p.PropertyType,
		ListingType:  p.ListingType,
		AvailableFor: p.AvailableFor,
		Street:       p.Street,
		Neighborhood: p.Neighborhood,
		City:         p.City,
		State:        p.State,
		SalePrice:    p.SalePrice,
		RentPrice:    p.RentPrice,
		AreaTotal:    p.AreaTotal,
		Bedrooms:     p.Bedrooms,
	}
}

func toMatchWeights(w Weights) propertymatch.Weights {
	return propertymatch.Weights{Business: w.Business, Price: w.Price, Location: w.Location, Area: w.Area, Bedrooms: w.Bedrooms}
}

// DefaultMatchWeights — pesos do issue #415. Mantido como var (não
// const) para tests poderem comparar valores; soma == 100.
var DefaultMatchWeights = Weights{
	Business: 50,
	Price:    25,
	Location: 15,
	Area:     5,
	Bedrooms: 5,
}

// ScoreProperty é a função pura de matching. Recebe a preferência do
// lead e o imóvel candidato; devolve breakdown + score consolidado.
// Sem dependência em I/O — ideal para table-driven tests.
//
// Critério ausente no lead → o peso desse critério é redistribuído
// proporcionalmente entre os outros. Se TUDO está ausente, retorna
// score=0 (lead sem preferências, sem como rankear).
func ScoreProperty(li lead.LeadProperty, prop clients.PropertyDTO, weights Weights) MatchBreakdown {
	b := propertymatch.Score(toMatchPreferences(li), toMatchProperty(prop), toMatchWeights(weights))
	return MatchBreakdown{Business: b.Business, Price: b.Price, Location: b.Location, Area: b.Area, Bedrooms: b.Bedrooms, UsedWeights: Weights{
		Business: b.UsedWeights.Business, Price: b.UsedWeights.Price, Location: b.UsedWeights.Location, Area: b.UsedWeights.Area, Bedrooms: b.UsedWeights.Bedrooms,
	}}
}

func scorePropertyForOperation(li lead.LeadProperty, prop clients.PropertyDTO, weights Weights, businessType string) MatchBreakdown {
	b := propertymatch.Score(toMatchPreferencesForOperation(li, businessType), toMatchProperty(prop), toMatchWeights(weights))
	return MatchBreakdown{Business: b.Business, Price: b.Price, Location: b.Location, Area: b.Area, Bedrooms: b.Bedrooms, UsedWeights: Weights{
		Business: b.UsedWeights.Business, Price: b.UsedWeights.Price, Location: b.UsedWeights.Location, Area: b.UsedWeights.Area, Bedrooms: b.UsedWeights.Bedrooms,
	}}
}

// Total devolve a soma agregada do breakdown, arredondada para 2 casas
// decimais — útil para a UI mostrar "87,53 / 100".
func (b MatchBreakdown) Total() float64 {
	sum := b.Business + b.Price + b.Location + b.Area + b.Bedrooms
	return math.Round(sum*100) / 100
}

// ---------------------------------------------------------------------------
// Helpers de scoring por critério — todos retornam 0..1, sem peso aplicado
// ---------------------------------------------------------------------------

// priceScore vale 1.0 quando o preço cai dentro da faixa do lead.
// Fora da faixa, decresce linearmente até 0 conforme a distância
// relativa (cap em 50% acima do max ou abaixo do min). Sem preço
// declarado pelo imóvel → 0 (não dá pra comparar).
func priceScore(r lead.PriceRange, propertyPrice float64) float64 {
	return propertymatch.PriceScore(propertymatch.PriceRange{Min: r.Min, Max: r.Max}, propertyPrice)
}

// locationScore — match por substring (case-insensitive) no
// neighborhood, city ou street do imóvel. Heurística simples para v1;
// versões futuras podem fazer geocoding + raio em km.
//
// Lead preferred_location pode ter múltiplos termos separados por
// vírgula ou "/" ("Centro / Bela Vista"). Qualquer match conta como 1.0;
// match parcial (substring de uma palavra) conta como 0.7.
func locationScore(preferred string, prop clients.PropertyDTO) float64 {
	return propertymatch.LocationScore(preferred, toMatchProperty(prop))
}

// areaScore — 1.0 quando a área cai dentro do range; decresce com
// distância relativa. Tolera campos parcialmente preenchidos.
func areaScore(minPtr, maxPtr *float64, propArea float64) float64 {
	return propertymatch.AreaScore(minPtr, maxPtr, propArea)
}

// bedroomsScore — convenção "lead.Bedrooms" é o MÍNIMO desejado. 1.0
// se o imóvel tem >= esse número; decresce em 0.5 por quarto faltante.
func bedroomsScore(leadMinBedrooms, propBedrooms int) float64 {
	return propertymatch.BedroomsScore(leadMinBedrooms, propBedrooms)
}

// redistributeWeights — se o lead omite critérios, os pesos restantes
// sobem proporcionalmente para manter a soma == 100. Evita penalizar
// leads com preferências parciais.
func redistributeWeights(base Weights, hasPrice, hasLocation, hasArea, hasBedrooms bool) Weights {
	w := propertymatch.RedistributeWeights(toMatchWeights(base), hasPrice, hasLocation, hasArea, hasBedrooms)
	return Weights{Business: w.Business, Price: w.Price, Location: w.Location, Area: w.Area, Bedrooms: w.Bedrooms}
}

// propertyPrice — extrai o valor "que importa pro match" do PropertyDTO.
// Se for venda usa sale_price.current_amount; locação usa rent_price.
func propertyPrice(p clients.PropertyDTO) float64 {
	return propertyPriceForOperation(p, "")
}

func propertyPriceForOperation(p clients.PropertyDTO, businessType string) float64 {
	return propertymatch.EffectivePriceForOperation(propertymatch.Property{
		ListingType:  p.ListingType,
		AvailableFor: p.AvailableFor,
		SalePrice:    p.SalePrice,
		RentPrice:    p.RentPrice,
	}, businessType)
}

// splitLocationTerms separa "Centro / Bela Vista, Pinheiros" em
// ["Centro", "Bela Vista", "Pinheiros"]. Tolera múltiplos separadores.
func splitLocationTerms(raw string) []string {
	return propertymatch.SplitLocationTerms(raw)
}

// ---------------------------------------------------------------------------
// Service-level orchestration
// ---------------------------------------------------------------------------

// propertyLookup é a interface mínima que MatchProperties precisa do
// CoreClient — definida local para destacar nas dependências e facilitar
// mock em testes.
type propertyLookup interface {
	SearchProperties(ctx context.Context, filter clients.PropertyFilterDTO) ([]clients.PropertyDTO, error)
	GetPropertyByID(ctx context.Context, id string) (*clients.PropertyDTO, error)
}

// ErrLeadHasNoPreferences sinaliza que o lead não tem property_interest
// suficiente pra rodar matching. Handler responde 422 — o admin precisa
// completar o cadastro antes de pedir sugestões.
var ErrLeadHasNoPreferences = errors.New("lead has no property_interest preferences to match against")

// MatchProperties busca imóveis e rankeia por compatibilidade com as
// preferências do lead. Limite (default 10) controla quantos top
// matches retornar. Aplica filtro server-side por modalidade disponível
// (sale vs rent) inferida do role do lead, evitando trazer locações pra um
// comprador (ou vice-versa), inclusive quando o imóvel aceita mais de uma.
func (s *LeadService) MatchProperties(ctx context.Context, tenantID, leadID string, limit int) ([]MatchedProperty, error) {
	if s.propertyClient == nil {
		return nil, errors.New("property lookup client not configured")
	}

	// SECURITY (achado C9): o repositório agora filtra por tenant — antes a
	// validação abaixo era a ÚNICA barreira contra cross-tenant lookup (o
	// comentário original registrava isso). Mantida como defesa em
	// profundidade, agora redundante.
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return nil, fmt.Errorf("lead lookup: %w", err)
	}
	if l == nil {
		return nil, errors.New("lead not found")
	}
	if l.TenantID != "" && tenantID != "" && l.TenantID != tenantID {
		return nil, errors.New("lead not found")
	}

	return s.MatchPropertiesForLead(ctx, l, limit)
}

// MatchPropertiesForLead roda o matching sobre um lead JÁ carregado pelo
// chamador. Existe para o portal do cliente, que lê o lead pelo pool de serviço
// (o tenant do token do cliente é a conta local dele, não a imobiliária dona do
// lead) e não pode reusar MatchProperties, cuja leitura passa pelo pool de
// aplicação — sob RLS ela não acharia o lead e os matches viriam vazios.
func (s *LeadService) MatchPropertiesForLead(ctx context.Context, l *lead.Lead, limit int) ([]MatchedProperty, error) {
	if s.propertyClient == nil {
		return nil, errors.New("property lookup client not configured")
	}
	if l == nil {
		return nil, errors.New("lead not found")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	tenantID := l.TenantID

	pref := l.PropertyInterest
	if pref == nil || !hasAnyPreference(*pref) {
		return nil, ErrLeadHasNoPreferences
	}

	// Filtro pro CoreClient: pega só imóveis disponíveis (não vendidos/
	// alugados/inativos) + modalidade compatível com o role do lead.
	filter := clients.PropertyFilterDTO{
		TenantID:     tenantID,
		AvailableFor: listingTypeForLead(l.Role),
		PropertyType: propertyTypeForLead(pref.Type),
		Limit:        200, // teto razoável para o pool de candidatos
	}
	// Hint de filtro por preço — pode reduzir o pool antes de scoring.
	// Tolera caso só um lado está preenchido.
	if pref.PriceRange.Min > 0 {
		min := pref.PriceRange.Min * 0.7 // 30% abaixo do mínimo declarado entram (downgrade tolerável)
		filter.MinPrice = &min
	}
	if pref.PriceRange.Max > 0 {
		max := pref.PriceRange.Max * 1.3 // 30% acima do máximo declarado entram (upgrade tolerável)
		filter.MaxPrice = &max
	}

	candidates, err := s.propertyClient.SearchProperties(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("property lookup: %w", err)
	}

	scored := make([]MatchedProperty, 0, len(candidates))
	for _, p := range candidates {
		if filter.AvailableFor != "" && !propertymatch.BusinessTypeMatches(filter.AvailableFor, toMatchProperty(p)) {
			continue
		}
		if filter.PropertyType != "" && !strings.EqualFold(strings.TrimSpace(p.PropertyType), filter.PropertyType) {
			continue
		}
		if !matchEligibleStatus(p.Status) {
			continue
		}
		bk := scorePropertyForOperation(*pref, p, DefaultMatchWeights, filter.AvailableFor)
		total := bk.Total()
		// Com modalidade conhecida, a compatibilidade de negócio sozinha
		// vale 50 pontos e ainda não é uma recomendação de qualidade. Exigimos
		// pelo menos mais uma evidência relevante para evitar cards genéricos.
		minimumScore := 30.0
		if filter.AvailableFor != "" {
			minimumScore = 60
		}
		if total < minimumScore {
			continue
		}
		matchedProperty := p
		if filter.AvailableFor != "" {
			// The match payload is operation-specific so the card displays the
			// correct price (rent vs sale) for a multi-modality property.
			matchedProperty.ListingType = filter.AvailableFor
		}
		scored = append(scored, MatchedProperty{
			Property: matchedProperty,
			Score:    total,
			Reasons:  bk,
		})
	}

	// Ordenação desc por score; quebra de empate por preço ASC quando
	// limit corta — o mais "barato dentro do match" ganha empate.
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return propertyPriceForOperation(scored[i].Property, filter.AvailableFor) < propertyPriceForOperation(scored[j].Property, filter.AvailableFor)
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// hasAnyPreference responde "vale a pena tentar matching?". Critério:
// pelo menos um dos 4 critérios do score está preenchido.
func hasAnyPreference(p lead.LeadProperty) bool {
	if p.PriceRange.Min > 0 || p.PriceRange.Max > 0 {
		return true
	}
	if strings.TrimSpace(p.PreferredLocation) != "" {
		return true
	}
	if p.Features.AreaMin != nil && *p.Features.AreaMin > 0 {
		return true
	}
	if p.Features.AreaMax != nil && *p.Features.AreaMax > 0 {
		return true
	}
	if p.Features.Bedrooms != nil && *p.Features.Bedrooms > 0 {
		return true
	}
	return false
}

// listingTypeForLead infere "sale" vs "rent" a partir do role do
// lead. Quando o role não dá pista (lead genérico), volta "" — o filtro
// fica neutro e o backend devolve ambos os tipos.
func listingTypeForLead(role lead.ClientRole) string {
	switch role {
	case lead.ClientRoleTenant:
		return "rent"
	case lead.ClientRoleBuyer:
		return "sale"
	default:
		return ""
	}
}

func propertyTypeForLead(value lead.PropertyType) string {
	switch value {
	case lead.PropertyTypeApartment:
		return "apartment"
	case lead.PropertyTypeHouse:
		return "house"
	case lead.PropertyTypeLand:
		return "land"
	case lead.PropertyTypeCommercial:
		return "commercial"
	default:
		return ""
	}
}

func matchEligibleStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "published", "in_portfolio", "available":
		// Empty is accepted for legacy fixtures/records. New core properties
		// carry an explicit status and are restricted to active inventory.
		return true
	default:
		return false
	}
}

package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"strings"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/propertymatch"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
)

// leadPostgresRepository acts as a dummy/stub repository reading directly from crm.leads schema for the pilot.
type leadPostgresRepository struct {
	db *database.PostgresDB
}

func NewLeadPostgresRepository(db *database.PostgresDB) *leadPostgresRepository {
	return &leadPostgresRepository{db: db}
}

func (r *leadPostgresRepository) GetInterest(ctx context.Context, tenantID, id string) (interface{}, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata->'PropertyInterest'
		FROM crm.leads
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// Exists checks whether a non-deleted lead with the given id exists in the
// tenant. Used by DealService.Create to gate cross-tenant ID forgery.
func (r *leadPostgresRepository) Exists(ctx context.Context, tenantID, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM crm.leads
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		)
	`, id, tenantID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ClientLeadTenants devolve as imobiliárias (tenants) em que a conta do portal
// tem ATENDIMENTO vinculado.
//
// O vínculo do cliente com a imobiliária nasce no LEAD (o corretor cadastra o
// proprietário/comprador, abre a conversa e só depois existe negócio). A lista
// de canais do portal derivava os tenants apenas de `deal.deals` e, sem negócio,
// não consultava o toca-comm — a conversa aberta pelo painel não aparecia para o
// cliente (incidente 2026-09-21).
func (r *leadPostgresRepository) ClientLeadTenants(ctx context.Context, accountID string) ([]string, error) {
	out := []string{}
	if strings.TrimSpace(accountID) == "" {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT tenant_id::text
		FROM crm.leads
		WHERE deleted_at IS NULL AND metadata->>'AccountID' = $1
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, err
		}
		out = append(out, tenantID)
	}
	return out, rows.Err()
}

// propertyPostgresRepository acts as a dummy/stub repository reading directly from core.properties schema for the pilot.
type propertyPostgresRepository struct {
	db *database.PostgresDB
}

func NewPropertyPostgresRepository(db *database.PostgresDB) *propertyPostgresRepository {
	return &propertyPostgresRepository{db: db}
}

type SearchParams struct {
	Query       string
	ListingType string
	MinBedrooms *int
	MinPrice    *float64
	MaxPrice    *float64
	Limit       int64
	Offset      int64
}

// propertySummaryQuery alimenta as duas leituras do imóvel: a do operador
// (pool de aplicação, tenant-scoped) e a do portal do cliente (pool de serviço,
// cross-tenant). Manter uma única consulta evita as duas divergirem.
// propertySummaryColumns é a projeção única do imóvel para o portal e para o
// painel: título, localização, mídia, preço, características e descrição. Fica
// numa função porque o placeholder do tipo de negócio muda de posição entre a
// leitura por id ($3) e as listas ($2) — antes cada consulta repetia a lista de
// colunas e elas podiam divergir (era o caso: a lista não trazia as imagens e a
// tela do portal abria sem galeria).
func propertySummaryColumns(dealTypeArg string) string {
	return fmt.Sprintf(`
			id::text,
			metadata->>'title',
			TRIM(CONCAT(COALESCE(metadata#>>'{address,street}',''), ', ', COALESCE(metadata#>>'{address,number}',''))),
			COALESCE(metadata#>>'{address,neighborhood}',''),
			COALESCE(metadata#>>'{address,city}',''),
			COALESCE(metadata#>>'{address,state}',''),
			COALESCE(metadata#>>'{media,images,0,url}',''),
			CASE
				WHEN %[1]s = 'SALE' THEN COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, 0)
				ELSE COALESCE((metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0)
			END,
			COALESCE((metadata#>>'{features,bedrooms}')::int, 0),
			COALESCE((metadata#>>'{features,bathrooms}')::int, 0),
			COALESCE((metadata#>>'{features,parking_spaces}')::int, 0),
			COALESCE((metadata#>>'{areas,total_area_m2}')::numeric, 0),
			CASE WHEN %[1]s = 'RENT' THEN 'rent' ELSE 'sale' END,
			COALESCE((metadata#>>'{features,suites}')::int, 0),
			COALESCE((metadata#>>'{areas,usable_area_m2}')::numeric, 0),
			COALESCE(metadata->'amenities', '[]'::jsonb)::text,
			COALESCE(metadata->>'description',''),
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('url', img->>'url', 'description', COALESCE(img->>'description',''))
				                 ORDER BY COALESCE((img->>'order')::int, 0))
				FROM jsonb_array_elements(COALESCE(metadata#>'{media,images}','[]'::jsonb)) AS img
				WHERE COALESCE(img->>'url','') <> ''
			), '[]'::jsonb)::text
	`, dealTypeArg)
}

// propertySummaryScan destinos na ordem de propertySummaryColumns.
type propertySummaryRow struct {
	summary       portal.PropertySummary
	amenitiesJSON string
	imagesJSON    string
}

var propertySummaryQuery = `
		SELECT
	` + propertySummaryColumns("$3") + `
		FROM core.properties
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

// scanPropertySummary lê a linha na mesma ordem da projeção e desserializa os
// campos JSON (comodidades e galeria).
func scanPropertySummary(row interface {
	Scan(dest ...any) error
}) (*portal.PropertySummary, error) {
	var scanned propertySummaryRow
	if err := row.Scan(
		&scanned.summary.ID,
		&scanned.summary.Title,
		&scanned.summary.Address,
		&scanned.summary.Neighborhood,
		&scanned.summary.City,
		&scanned.summary.State,
		&scanned.summary.ImageURL,
		&scanned.summary.Price,
		&scanned.summary.Bedrooms,
		&scanned.summary.Bathrooms,
		&scanned.summary.ParkingSpaces,
		&scanned.summary.TotalAreaM2,
		&scanned.summary.ListingType,
		&scanned.summary.Suites,
		&scanned.summary.UsableAreaM2,
		&scanned.amenitiesJSON,
		&scanned.summary.Description,
		&scanned.imagesJSON,
	); err != nil {
		return nil, err
	}
	decodePropertySummaryJSON(&scanned)
	return &scanned.summary, nil
}

// decodePropertySummaryJSON desserializa os campos JSON da projeção.
func decodePropertySummaryJSON(scanned *propertySummaryRow) {
	if scanned == nil {
		return
	}
	if scanned.amenitiesJSON != "" {
		_ = json.Unmarshal([]byte(scanned.amenitiesJSON), &scanned.summary.Amenities)
	}
	if scanned.imagesJSON != "" {
		_ = json.Unmarshal([]byte(scanned.imagesJSON), &scanned.summary.Images)
	}
}

func (r *propertyPostgresRepository) GetSummaryByID(ctx context.Context, tenantID, id string, dealType deal.Type) (*portal.PropertySummary, error) {
	dealTypeStr := string(dealType)
	out, err := scanPropertySummary(r.db.QueryRowContext(ctx, propertySummaryQuery, id, tenantID, dealTypeStr))
	if err != nil {
		return nil, fmt.Errorf("property not found: %w", err)
	}
	return out, nil
}

// GetSummaryByIDService é a leitura do imóvel para o PORTAL DO CLIENTE: o
// cliente é cross-tenant por definição (o tenant do token é a conta local dele;
// o imóvel pertence à imobiliária) e sob a RLS do pool de aplicação a consulta
// voltava vazia — a tela do negócio ficava sem o imóvel. O tenant usado é o do
// PRÓPRIO negócio, resolvido no vínculo do signatário.
func (r *propertyPostgresRepository) GetSummaryByIDService(ctx context.Context, tenantID, id string, dealType deal.Type) (*portal.PropertySummary, error) {
	out, err := scanPropertySummary(r.db.QueryServiceRow(ctx, propertySummaryQuery, id, tenantID, string(dealType)))
	if err != nil {
		return nil, fmt.Errorf("property not found: %w", err)
	}
	return out, nil
}

func (r *propertyPostgresRepository) ListRecommendations(ctx context.Context, tenantID string, dealType deal.Type, excludePropertyID string, interest interface{}, limit int64) ([]*portal.PropertySummary, error) {
	if limit <= 0 {
		limit = 10
	}

	listingType := "sale"
	if dealType == deal.TypeRent {
		listingType = "rent"
	}

	args := []any{tenantID, listingType}
	where := []string{
		"tenant_id = $1",
		"deleted_at IS NULL",
		"((metadata->>'listing_type') = $2 OR COALESCE(metadata->'available_for', '[]'::jsonb) ? $2)",
		"COALESCE(metadata->>'status','') IN ('', 'published', 'in_portfolio', 'available')",
	}
	if excludePropertyID != "" {
		args = append(args, excludePropertyID)
		where = append(where, fmt.Sprintf("id::text <> $%d", len(args)))
	}

	pref, hasPreferences := recommendationPreferences(interest)
	if hasPreferences {
		pref.BusinessType = listingType
		if propertyType := recommendationPropertyType(pref.Type); propertyType != "" {
			args = append(args, propertyType)
			where = append(where, fmt.Sprintf("(metadata->>'property_type') = $%d", len(args)))
		}
	}

	candidateLimit := limit * 10
	if candidateLimit < 50 {
		candidateLimit = 50
	}
	if candidateLimit > 200 {
		candidateLimit = 200
	}
	args = append(args, candidateLimit)
	limitArg := len(args)

	query := fmt.Sprintf(`
		SELECT
	`+propertySummaryColumns("UPPER($2)")+`,
			COALESCE(metadata->'available_for', '[]'::jsonb)
		FROM core.properties
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), limitArg)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*portal.PropertySummary, 0)
	for rows.Next() {
		var scanned propertySummaryRow
		var availableForRaw []byte
		if err := rows.Scan(
			&scanned.summary.ID,
			&scanned.summary.Title,
			&scanned.summary.Address,
			&scanned.summary.Neighborhood,
			&scanned.summary.City,
			&scanned.summary.State,
			&scanned.summary.ImageURL,
			&scanned.summary.Price,
			&scanned.summary.Bedrooms,
			&scanned.summary.Bathrooms,
			&scanned.summary.ParkingSpaces,
			&scanned.summary.TotalAreaM2,
			&scanned.summary.ListingType,
			&scanned.summary.Suites,
			&scanned.summary.UsableAreaM2,
			&scanned.amenitiesJSON,
			&scanned.summary.Description,
			&scanned.imagesJSON,
			&availableForRaw,
		); err != nil {
			return nil, err
		}
		decodePropertySummaryJSON(&scanned)
		if len(availableForRaw) > 0 {
			if err := json.Unmarshal(availableForRaw, &scanned.summary.AvailableFor); err != nil {
				return nil, fmt.Errorf("decode property available_for: %w", err)
			}
		}
		// Vitrine (recomendações): sem identificação exata do imóvel.
		out = append(out, portal.MaskPropertyPrivacy(&scanned.summary))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !hasPreferences {
		if len(out) > int(limit) {
			out = out[:limit]
		}
		return out, nil
	}

	scores := make([]float64, len(out))
	prices := make([]float64, len(out))
	for i, candidate := range out {
		matched := propertymatch.Score(pref, propertymatch.Property{
			ListingType:  candidate.ListingType,
			AvailableFor: candidate.AvailableFor,
			Neighborhood: candidate.Neighborhood,
			City:         candidate.City,
			State:        candidate.State,
			Street:       candidate.Address,
			SalePrice:    candidate.Price,
			RentPrice:    candidate.Price,
			AreaTotal:    candidate.TotalAreaM2,
			Bedrooms:     candidate.Bedrooms,
		}, propertymatch.DefaultWeights)
		scores[i] = matched.Total()
		prices[i] = candidate.Price
		candidate.MatchScore = &scores[i]
		candidate.MatchReasons = &matched
	}
	order := propertymatch.SortScores(scores, prices)
	ranked := make([]*portal.PropertySummary, 0, len(order))
	for _, index := range order {
		// Modality contributes 50 points, but by itself it is not enough to
		// show a generic recommendation in the portal.
		if scores[index] < 60 {
			continue
		}
		ranked = append(ranked, out[index])
		if len(ranked) == int(limit) {
			break
		}
	}
	return ranked, nil
}

func recommendationPreferences(interest interface{}) (propertymatch.Preferences, bool) {
	if interest == nil {
		return propertymatch.Preferences{}, false
	}
	var raw []byte
	switch value := interest.(type) {
	case json.RawMessage:
		raw = value
	case []byte:
		raw = value
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return propertymatch.Preferences{}, false
		}
		raw = encoded
	}
	pref, err := propertymatch.DecodePreferences(raw)
	return pref, err == nil && propertymatch.HasAnyPreference(pref)
}

func recommendationPropertyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "apartment":
		return "apartment"
	case "house":
		return "house"
	case "land":
		return "land"
	case "commercial":
		return "commercial"
	default:
		return ""
	}
}

func (r *propertyPostgresRepository) Search(ctx context.Context, tenantID string, dealType deal.Type, params SearchParams) ([]*portal.PropertySummary, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if s := strings.TrimSpace(params.Query); s != "" {
		where = append(where, fmt.Sprintf("(metadata->>'title') ILIKE $%d", argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	// Finalidade: o cliente escolhe "Para comprar"/"Para alugar"; sem escolha, o
	// padrão vem do tipo do NEGÓCIO (um negócio de venda não abre a busca em
	// aluguel). A condição aceita `available_for` — o mesmo critério da
	// recomendação de lead (`ListRecommendations`). Antes os dois caminhos
	// discordavam: um imóvel marcado como disponível para venda E aluguel
	// aparecia na recomendação e sumia da busca.
	listingType := strings.TrimSpace(params.ListingType)
	if listingType == "" {
		listingType = "sale"
		if dealType == deal.TypeRent {
			listingType = "rent"
		}
	}
	listingArgN := argN
	where = append(where, fmt.Sprintf(
		"((metadata->>'listing_type') = $%d OR COALESCE(metadata->'available_for','[]'::jsonb) ? $%d)",
		argN, argN))
	args = append(args, listingType)
	argN++

	// Disponibilidade: mesma régua da recomendação de lead. Sem isto a busca do
	// portal devolvia imóvel vendido, pausado ou ainda em rascunho — o cliente
	// via na vitrine o que a imobiliária não colocou à venda.
	where = append(where, "COALESCE(metadata->>'status','') IN ('', 'published', 'in_portfolio', 'available')")

	if params.MinBedrooms != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata#>>'{features,bedrooms}')::int, 0) >= $%d", argN))
		args = append(args, *params.MinBedrooms)
		argN++
	}

	// Valor: SOMENTE o preço da finalidade pedida. O fallback cruzado antigo
	// (`sale_price` caindo para `rent_price`) apresentava o ALUGUEL como preço de
	// um imóvel à venda — número errado com rótulo de venda. Sem preço para a
	// finalidade o valor é 0 e a interface diz "Preço sob consulta".
	priceExpr := "COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, 0)"
	if listingType == "rent" {
		priceExpr = "COALESCE((metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0)"
	}

	if params.MinPrice != nil {
		where = append(where, fmt.Sprintf("%s >= $%d", priceExpr, argN))
		args = append(args, *params.MinPrice)
		argN++
	}
	if params.MaxPrice != nil {
		where = append(where, fmt.Sprintf("%s <= $%d", priceExpr, argN))
		args = append(args, *params.MaxPrice)
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	args = append(args, limit, offset)

	// priceExpr entra como expressão de preço; a projeção base vem da função
	// compartilhada (mesmas colunas do portal e das recomendações).
	query := fmt.Sprintf(`
		SELECT
			id::text,
			metadata->>'title',
			TRIM(CONCAT(COALESCE(metadata#>>'{address,street}',''), ', ', COALESCE(metadata#>>'{address,number}',''))),
			COALESCE(metadata#>>'{address,neighborhood}',''),
			COALESCE(metadata#>>'{address,city}',''),
			COALESCE(metadata#>>'{address,state}',''),
			COALESCE(metadata#>>'{media,images,0,url}',''),
			%s,
			COALESCE((metadata#>>'{features,bedrooms}')::int, 0),
			COALESCE((metadata#>>'{features,bathrooms}')::int, 0),
			COALESCE((metadata#>>'{features,parking_spaces}')::int, 0),
			COALESCE((metadata#>>'{areas,total_area_m2}')::numeric, 0),
			$%d,
			COALESCE((metadata#>>'{features,suites}')::int, 0),
			COALESCE((metadata#>>'{areas,usable_area_m2}')::numeric, 0),
			COALESCE(metadata->'amenities', '[]'::jsonb)::text,
			COALESCE(metadata->>'description',''),
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('url', img->>'url', 'description', COALESCE(img->>'description',''))
				                 ORDER BY COALESCE((img->>'order')::int, 0))
				FROM jsonb_array_elements(COALESCE(metadata#>'{media,images}','[]'::jsonb)) AS img
				WHERE COALESCE(img->>'url','') <> ''
			), '[]'::jsonb)::text
		FROM core.properties
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, priceExpr, listingArgN, whereSQL, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*portal.PropertySummary, 0)
	for rows.Next() {
		var scanned propertySummaryRow
		if err := rows.Scan(
			&scanned.summary.ID,
			&scanned.summary.Title,
			&scanned.summary.Address,
			&scanned.summary.Neighborhood,
			&scanned.summary.City,
			&scanned.summary.State,
			&scanned.summary.ImageURL,
			&scanned.summary.Price,
			&scanned.summary.Bedrooms,
			&scanned.summary.Bathrooms,
			&scanned.summary.ParkingSpaces,
			&scanned.summary.TotalAreaM2,
			&scanned.summary.ListingType,
			&scanned.summary.Suites,
			&scanned.summary.UsableAreaM2,
			&scanned.amenitiesJSON,
			&scanned.summary.Description,
			&scanned.imagesJSON,
		); err != nil {
			return nil, err
		}
		decodePropertySummaryJSON(&scanned)
		// Vitrine (busca do portal): sem identificação exata do imóvel.
		out = append(out, portal.MaskPropertyPrivacy(&scanned.summary))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────────────
// Cross-company co-brokering — operate-shared-content feature.
//
// CrossPartnership is a LOCAL projection of a core.partnerships row, decoded
// from its metadata JSONB. toca-deal does not import the toca-core partnership
// domain — it reads the cross-schema row directly (same pilot pattern as
// toca-financial/internal/repository/cross_postgres.go) and only needs the
// slice the co-brokering check consumes: the granted permission scopes and the
// sharing config (AllowCoBrokering).
//
// The JSONB shape is whatever toca-core's PartnershipRepository persists via
// json.Marshal(*partnership.Partnership) — the json tags on that struct.
// ─────────────────────────────────────────────────────────────────────────

type CrossPartnership struct {
	Grants           map[string]access.Grant      `json:"grants"`
	FinancialVersion int                          `json:"financial_version"`
	FinancialConfig  json.RawMessage              `json:"financial_config"`
	ID               string                       `json:"id"`
	RequesterID      string                       `json:"requester_id"`
	TargetID         string                       `json:"target_id"`
	Type             string                       `json:"type"`
	Status           string                       `json:"status"`
	Permissions      []CrossPartnershipPermission `json:"permissions"`
	Sharing          CrossPartnershipSharing      `json:"sharing_config"`
}

// CrossPartnershipPermission mirrors partnership.Permission.
type CrossPartnershipPermission = access.Permission

// CrossPartnershipSharing mirrors partnership.SharingConfig.
type CrossPartnershipSharing = access.Sharing

// CrossTenantRepository exposes minimal cross-schema lookups against schemas
// owned by other services. Same pilot pattern as toca-financial's cross repo:
// every schema lives in the same Postgres instance, so we read core.properties
// / crm.leads / core.partnerships directly. Swap for HTTP clients if the
// modules ever split into distinct databases — the interface stays the same.
type CrossTenantRepository interface {
	// PropertyOwnerTenant returns the tenant_id that owns a property. Used
	// to detect a cross-company deal (the property's owner tenant differs
	// from the closing deal's tenant). Returns sql.ErrNoRows-wrapped error
	// when the property does not exist or is soft-deleted.
	PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error)

	// LeadOwnerTenant returns the tenant_id that owns a lead. Same purpose
	// as PropertyOwnerTenant for the lead-shared path.
	LeadOwnerTenant(ctx context.Context, leadID string) (string, error)

	// ActivePartnershipBetween returns the ACTIVE partnership between two
	// companies (in either direction), or nil when none exists.
	ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*CrossPartnership, error)

	// BrokersInTenant devolve, dos `brokerIDs` pedidos, quais existem no tenant.
	//
	// N14 (reauditoria de 2026-09-20): a divisão de comissão
	// (`broker_participations`) é gravada no negócio e direciona repasse
	// financeiro, e os ids vinham do corpo da requisição **sem nenhuma
	// conferência** — a divisão podia apontar para corretor de outro tenant ou
	// para um id inexistente. Os corretores são do toca-crm; entramos por
	// `crm.brokers` pelo mesmo caminho cross-schema já usado para `crm.leads`.
	//
	// Compara `id::text = ANY(...)` de propósito: um id malformado não vira erro
	// de cast (500), apenas "não encontrado" — que é erro do chamador.
	BrokersInTenant(ctx context.Context, tenantID string, brokerIDs []string) (map[string]struct{}, error)
}

type crossPostgresRepository struct {
	db *database.PostgresDB
}

func NewCrossPostgresRepository(db *database.PostgresDB) CrossTenantRepository {
	return &crossPostgresRepository{db: db}
}

func (r *crossPostgresRepository) PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error) {
	if propertyID == "" {
		return "", errors.New("property_id required")
	}
	var tenantID string
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id::text
		FROM core.properties
		WHERE id = $1 AND deleted_at IS NULL
	`, propertyID).Scan(&tenantID)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (r *crossPostgresRepository) LeadOwnerTenant(ctx context.Context, leadID string) (string, error) {
	if leadID == "" {
		return "", errors.New("lead_id required")
	}
	var tenantID string
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id::text
		FROM crm.leads
		WHERE id = $1 AND deleted_at IS NULL
	`, leadID).Scan(&tenantID)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (r *crossPostgresRepository) ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*CrossPartnership, error) {
	if companyA == "" || companyB == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM core.partnerships
		WHERE status = 'active'
		  AND deleted_at IS NULL
		  AND (
		        (requester_id::text = $1 AND target_id::text = $2)
		     OR (requester_id::text = $2 AND target_id::text = $1)
		      )
		ORDER BY created_at DESC
		LIMIT 1
	`, companyA, companyB).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var p CrossPartnership
	if err := json.Unmarshal(metadata, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// BrokersInTenant responde quais dos ids pedidos existem no tenant. Ver a
// motivação na interface (N14).
//
// Tenant vazio devolve conjunto vazio sem consultar o banco: a validação que
// consome isto trata "nenhum corretor encontrado" como erro do chamador, e uma
// consulta sem tenant não teria como estar certa.
func (r *crossPostgresRepository) BrokersInTenant(ctx context.Context, tenantID string, brokerIDs []string) (map[string]struct{}, error) {
	found := make(map[string]struct{}, len(brokerIDs))
	if strings.TrimSpace(tenantID) == "" || len(brokerIDs) == 0 {
		return found, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text
		FROM crm.brokers
		WHERE tenant_id = $1
		  AND id::text = ANY($2)
	`, tenantID, brokerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found[id] = struct{}{}
	}
	return found, rows.Err()
}

func (p *CrossPartnership) GrantFrom(owner string) access.Grant {
	return access.Resolve(p.Grants, owner, p.Sharing, p.Permissions)
}
func (p *CrossPartnership) Snapshot() *access.Snapshot {
	return &access.Snapshot{ID: p.ID, Version: p.FinancialVersion, Type: p.Type, RequesterID: p.RequesterID, TargetID: p.TargetID, FinancialConfig: p.FinancialConfig}
}

// Reuse the same item policy as the shared portfolio, never authorize by tenant alone.
func (r *crossPostgresRepository) EligibleSharedResource(ctx context.Context, kind, id, owner string, g access.Grant) (bool, error) {
	var metadata []byte
	query := `SELECT metadata FROM core.properties WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL`
	if kind == "lead" {
		query = `SELECT metadata FROM crm.leads WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL`
	}
	if err := r.db.QueryRowContext(ctx, query, id, owner).Scan(&metadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if kind == "lead" {
		var l struct {
			Shared bool `json:"partnership_shared"`
		}
		if err := json.Unmarshal(metadata, &l); err != nil {
			return false, err
		}
		return l.Shared && g.Allows("leads"), nil
	}
	var p access.Property
	if err := json.Unmarshal(metadata, &p); err != nil {
		return false, err
	}
	return g.IncludesProperty(p), nil
}

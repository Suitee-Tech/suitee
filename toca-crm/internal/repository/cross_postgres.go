package repository

import (
	"context"
	"encoding/json"
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"strings"

	"github.com/oginoino/toca/pkg/database"
)

// PartnershipSharing is a LOCAL projection of a core.partnerships row, decoded
// from its metadata JSONB. toca-crm does not import the toca-core partnership
// domain — it reads the cross-schema row directly (same pilot pattern as
// toca-financial/internal/repository/cross_postgres.go and the toca-deal
// cross repo) and only needs the slice the shared-leads feature consumes.
//
// The JSONB shape is whatever toca-core's PartnershipRepository persists via
// json.Marshal(*partnership.Partnership) — i.e. the json tags on the
// partnership domain struct: requester_id / target_id / type / status /
// permissions[] / sharing_config{}.
type PartnershipSharing struct {
	CompanyNames map[string]string       `json:"company_names"`
	Grants       map[string]access.Grant `json:"grants"`
	ID           string                  `json:"id"`
	RequesterID  string                  `json:"requester_id"`
	TargetID     string                  `json:"target_id"`
	Type         string                  `json:"type"`
	Status       string                  `json:"status"`
	Permissions  []PartnershipPermission `json:"permissions"`
	Sharing      PartnershipSharingConf  `json:"sharing_config"`
}

// PartnershipPermission mirrors partnership.Permission — a (scope, level)
// pair. Scope is "shared_portfolio" | "leads"; AccessLevel is "view" etc.
type PartnershipPermission = access.Permission

// PartnershipSharingConf mirrors partnership.SharingConfig.
type PartnershipSharingConf = access.Sharing

// Counterpart returns the OTHER participant id given one participant, or "".
func (p *PartnershipSharing) Counterpart(companyID string) string {
	if p == nil {
		return ""
	}
	switch companyID {
	case p.RequesterID:
		return p.TargetID
	case p.TargetID:
		return p.RequesterID
	default:
		return ""
	}
}

// CrossTenantRepository exposes minimal cross-schema lookups against schemas
// owned by other services. Same pilot pattern as the financial/deal cross
// repos: every schema lives in the same Postgres instance, so we read
// core.partnerships directly. Swap for an HTTP client if the modules ever
// split into distinct databases — the interface stays the same.
type CrossTenantRepository interface {
	// ActivePartnershipsForCompany returns every ACTIVE partnership the
	// company participates in (as requester OR target).
	ActivePartnershipsForCompany(ctx context.Context, companyID string) ([]*PartnershipSharing, error)

	// PropertySnapshot lê o estado ATUAL de um imóvel do portfólio (preço,
	// finalidade, disponibilidade). Existe para o portal do cliente não exibir
	// recomendação com valor congelado no dia em que o operador recomendou.
	PropertySnapshot(ctx context.Context, tenantID, propertyID string) (*PropertySnapshot, error)

	// AvailableProperties lista os imóveis DISPONÍVEIS dos tenants informados,
	// com o mesmo recorte de disponibilidade da busca do portal (toca-deal).
	AvailableProperties(ctx context.Context, tenantIDs []string, listingType string, limit int) ([]*AvailableProperty, error)
}

// PropertySnapshot é a leitura viva de um imóvel de core.properties.
type PropertySnapshot struct {
	ID           string
	TenantID     string
	Title        string
	Neighborhood string
	City         string
	State        string
	ListingType  string
	Status       string
	SalePrice    float64
	RentPrice    float64
	ThumbnailURL string
	MediaImages  []string
	Deleted      bool
}

// availabilityStatuses é a MESMA régua da busca do portal e da recomendação de
// lead (toca-deal/internal/repositories/cross_postgres.go). Imóvel vendido,
// alugado, pausado ou em rascunho não é vitrine.
func (p *PropertySnapshot) Available() bool {
	if p == nil || p.Deleted {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.Status)) {
	case "", "published", "in_portfolio", "available":
		return true
	}
	return false
}

// PriceFor devolve o valor da finalidade pedida (0 quando não há).
func (p *PropertySnapshot) PriceFor(listingType string) float64 {
	if strings.EqualFold(listingType, "rent") {
		return p.RentPrice
	}
	return p.SalePrice
}

// AvailableProperty é a projeção de vitrine usada pelo portal.
type AvailableProperty struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Neighborhood string   `json:"neighborhood"`
	City         string   `json:"city"`
	State        string   `json:"state"`
	ImageURL     string   `json:"image_url,omitempty"`
	Price        float64  `json:"price"`
	ListingType  string   `json:"listing_type"`
	Bedrooms     int      `json:"bedrooms,omitempty"`
	Bathrooms    int      `json:"bathrooms,omitempty"`
	Parking      int      `json:"parking_spaces,omitempty"`
	TotalAreaM2  float64  `json:"total_area_m2,omitempty"`
	PropertyType string   `json:"property_type,omitempty"`
	MediaImages  []string `json:"media_images,omitempty"`
}

type crossPostgresRepository struct {
	db *database.PostgresDB
}

func NewCrossPostgresRepository(db *database.PostgresDB) CrossTenantRepository {
	return &crossPostgresRepository{db: db}
}

func (r *crossPostgresRepository) ActivePartnershipsForCompany(ctx context.Context, companyID string) ([]*PartnershipSharing, error) {
	if companyID == "" {
		return nil, nil
	}
	// POOL DE SERVIÇO (BYPASSRLS): leitura cross-tenant por definição — a
	// parceria liga DUAS empresas e o chamador pode ser qualquer uma delas.
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata || jsonb_build_object('company_names',jsonb_build_object(requester_id::text,(SELECT a.metadata#>>'{Company,Name}' FROM toca_auth.accounts a WHERE a.id=requester_id),target_id::text,(SELECT a.metadata#>>'{Company,Name}' FROM toca_auth.accounts a WHERE a.id=target_id)))
		FROM core.partnerships
		WHERE status = 'active'
		  AND deleted_at IS NULL
		  AND (requester_id::text = $1 OR target_id::text = $1)
		ORDER BY created_at DESC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*PartnershipSharing, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var p PartnershipSharing
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// propertyAvailabilitySQL é a régua de disponibilidade — a MESMA da busca do
// portal (toca-deal) e da recomendação de lead. Duplicada aqui porque os dois
// módulos leem core.properties por caminhos próprios; qualquer mudança precisa
// valer nos três (há teste de banco no toca-deal travando o recorte).
const propertyAvailabilitySQL = "COALESCE(metadata->>'status','') IN ('', 'published', 'in_portfolio', 'available')"

func (r *crossPostgresRepository) PropertySnapshot(ctx context.Context, tenantID, propertyID string) (*PropertySnapshot, error) {
	if tenantID == "" || propertyID == "" {
		return nil, fmt.Errorf("tenant_id and property_id are required")
	}
	var out PropertySnapshot
	var mediaRaw []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT id::text, tenant_id::text,
			COALESCE(metadata->>'title',''),
			COALESCE(metadata#>>'{address,neighborhood}',''),
			COALESCE(metadata#>>'{address,city}',''),
			COALESCE(metadata#>>'{address,state}',''),
			COALESCE(metadata->>'listing_type',''),
			COALESCE(metadata->>'status',''),
			COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, 0),
			COALESCE((metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0),
			COALESCE(metadata#>>'{media,images,0,url}',''),
			COALESCE(metadata->'media'->'images','[]'::jsonb),
			(deleted_at IS NOT NULL)
		FROM core.properties
		WHERE id = $1::uuid AND tenant_id = $2::uuid
	`, propertyID, tenantID).Scan(
		&out.ID, &out.TenantID, &out.Title, &out.Neighborhood, &out.City, &out.State,
		&out.ListingType, &out.Status, &out.SalePrice, &out.RentPrice, &out.ThumbnailURL,
		&mediaRaw, &out.Deleted,
	)
	if err != nil {
		// Imóvel inexistente para este tenant: o chamador decide (a recomendação
		// deixa de ser exibida, o snapshot antigo NÃO é apagado por engano).
		return nil, nil
	}
	out.MediaImages = mediaURLs(mediaRaw)
	return &out, nil
}

func (r *crossPostgresRepository) AvailableProperties(ctx context.Context, tenantIDs []string, listingType string, limit int) ([]*AvailableProperty, error) {
	if len(tenantIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 60 {
		limit = 30
	}
	if strings.TrimSpace(listingType) == "" {
		listingType = "sale"
	}
	listingType = strings.ToLower(strings.TrimSpace(listingType))

	// `sale_price`/`rent_price` SEM fallback cruzado: valor só da finalidade
	// pedida (o portal diz "Preço sob consulta" quando é 0).
	priceExpr := "COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, 0)"
	if listingType == "rent" {
		priceExpr = "COALESCE((metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0)"
	}

	args := []any{tenantIDs}
	where := []string{
		"tenant_id = ANY($1::uuid[])",
		"deleted_at IS NULL",
		"((metadata->>'listing_type') = $2 OR COALESCE(metadata->'available_for','[]'::jsonb) ? $2)",
		propertyAvailabilitySQL,
	}
	args = append(args, listingType, limit)

	query := fmt.Sprintf(`
		SELECT id::text, tenant_id::text,
			COALESCE(metadata->>'title',''),
			COALESCE(metadata#>>'{address,neighborhood}',''),
			COALESCE(metadata#>>'{address,city}',''),
			COALESCE(metadata#>>'{address,state}',''),
			COALESCE(metadata#>>'{media,images,0,url}',''),
			%s,
			$2,
			COALESCE((metadata#>>'{features,bedrooms}')::int, 0),
			COALESCE((metadata#>>'{features,bathrooms}')::int, 0),
			COALESCE((metadata#>>'{features,parking_spaces}')::int, 0),
			COALESCE((metadata#>>'{areas,total_area_m2}')::numeric, 0),
			COALESCE(metadata->>'property_type',''),
			COALESCE(metadata->'media'->'images','[]'::jsonb)
		FROM core.properties
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $3
	`, priceExpr, strings.Join(where, " AND "))

	// POOL DE SERVIÇO (BYPASSRLS): a vitrine do portal do cliente lê imóveis
	// dos tenants das IMOBILIÁRIAS com quem ele se relaciona, enquanto o tenant
	// do token dele é a própria conta local. Sob RLS o pool de aplicação
	// devolveria lista vazia.
	rows, err := r.db.QueryServiceRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*AvailableProperty, 0, limit)
	for rows.Next() {
		var item AvailableProperty
		var mediaRaw []byte
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Neighborhood, &item.City, &item.State,
			&item.ImageURL, &item.Price, &item.ListingType,
			&item.Bedrooms, &item.Bathrooms, &item.Parking, &item.TotalAreaM2,
			&item.PropertyType, &mediaRaw,
		); err != nil {
			return nil, err
		}
		item.MediaImages = mediaURLs(mediaRaw)
		out = append(out, &item)
	}
	return out, rows.Err()
}

// mediaURLs extrai as URLs de `media.images` (array de objetos com `url`).
func mediaURLs(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var images []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &images); err != nil {
		return nil
	}
	out := make([]string, 0, len(images))
	for _, img := range images {
		if strings.TrimSpace(img.URL) != "" {
			out = append(out, img.URL)
		}
	}
	return out
}

func (p *PartnershipSharing) GrantFrom(owner string) access.Grant {
	return access.Resolve(p.Grants, owner, p.Sharing, p.Permissions)
}

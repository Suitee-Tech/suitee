// Package postgres implementa os repositórios do toca-sites no schema `sites`.
//
// Isolamento: TODA query de painel carrega `tenant_id` no WHERE e falha fechado
// quando o tenant chega vazio. O serviço usa o pool de aplicação (que bypassa
// RLS por ser service role), então o filtro explícito é a fronteira real; as
// políticas em supabase/migrations são o backstop para leituras via Supabase
// REST. O único caminho sem tenant é a resolução pública por slug/Host, e ele
// devolve apenas sites habilitados.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

type Repository struct {
	db *database.PostgresDB
}

func New(db *database.PostgresDB) *Repository {
	return &Repository{db: db}
}

// withTenant garante que o pool de aplicação receba o claim de tenant antes da
// query (o hook de conexão do pkg/database semeia `request.jwt.claims`). Sem
// isso, num ambiente cujo role NÃO bypassa RLS, as políticas filtrariam todas
// as linhas e o serviço leria "nada" — o pior modo de falha (silencioso).
// O filtro explícito no WHERE continua sendo a fronteira; o claim é o backstop.
func withTenant(ctx context.Context, tenantID string) context.Context {
	if strings.TrimSpace(tenantID) == "" {
		return ctx
	}
	if existing, err := appCtx.GetTenantID(ctx); err == nil && strings.TrimSpace(existing) != "" {
		return ctx
	}
	return appCtx.WithTenantID(ctx, tenantID)
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows)
}

// rowScanner é satisfeito por *sql.Row e por pgx.Row. A resolução pública usa
// o pool de serviço (cross-tenant por natureza); as demais usam o pool de
// aplicação com o claim do tenant.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// SQL exposto para os testes de contrato de isolamento (mesmo padrão de
// toca-ops/tenant_scope_test.go). Qualquer query que perca o filtro de tenant
// quebra o teste sem precisar de Postgres.
const (
	siteByTenantQuery = `SELECT id, tenant_id, slug, name, enabled, public_base_url, branding, contact, COALESCE(payload_tenant_id, ''), created_at, updated_at
		FROM sites.sites WHERE tenant_id = $1`
	siteBySlugQuery = `SELECT id, tenant_id, slug, name, enabled, public_base_url, branding, contact, COALESCE(payload_tenant_id, ''), created_at, updated_at
		FROM sites.sites WHERE slug = $1`
	siteUpdateQuery = `UPDATE sites.sites
		SET slug = $3, name = $4, enabled = $5, public_base_url = $6, branding = $7, contact = $8, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING updated_at`
	siteSlugTakenQuery = `SELECT EXISTS(SELECT 1 FROM sites.sites WHERE slug = $1 AND tenant_id <> $2)`

	pagesBySiteQuery = `SELECT id, tenant_id, site_id, page_key, title, subtitle, body, seo_title, seo_description, updated_at
		FROM sites.pages WHERE tenant_id = $1 AND site_id = $2 ORDER BY page_key`
	pageByKeyQuery = `SELECT id, tenant_id, site_id, page_key, title, subtitle, body, seo_title, seo_description, updated_at
		FROM sites.pages WHERE tenant_id = $1 AND site_id = $2 AND page_key = $3`
	pageUpsertQuery = `INSERT INTO sites.pages (tenant_id, site_id, page_key, title, subtitle, body, seo_title, seo_description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (site_id, page_key) DO UPDATE SET
			title = EXCLUDED.title, subtitle = EXCLUDED.subtitle, body = EXCLUDED.body,
			seo_title = EXCLUDED.seo_title, seo_description = EXCLUDED.seo_description, updated_at = NOW()
		WHERE sites.pages.tenant_id = EXCLUDED.tenant_id
		RETURNING id, updated_at`

	listingByPropertyQuery = `SELECT ` + listingColumns + `
		FROM sites.listings WHERE tenant_id = $1 AND property_id = $2`
	listingSeoSlugTakenQuery = `SELECT EXISTS(SELECT 1 FROM sites.listings
		WHERE tenant_id = $1 AND seo_slug = $2 AND property_id <> $3)`
	listingUpsertQuery = `INSERT INTO sites.listings
			(tenant_id, site_id, property_id, seo_slug, status, title, description, listing_type, property_type,
			 sale_price, rent_price, condo_fee, property_tax, address, features, amenities, images,
			 source_updated_at, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (tenant_id, property_id) DO UPDATE SET
			seo_slug = EXCLUDED.seo_slug, status = EXCLUDED.status, title = EXCLUDED.title,
			description = EXCLUDED.description, listing_type = EXCLUDED.listing_type,
			property_type = EXCLUDED.property_type, sale_price = EXCLUDED.sale_price,
			rent_price = EXCLUDED.rent_price, condo_fee = EXCLUDED.condo_fee,
			property_tax = EXCLUDED.property_tax, address = EXCLUDED.address,
			features = EXCLUDED.features, amenities = EXCLUDED.amenities, images = EXCLUDED.images,
			source_updated_at = EXCLUDED.source_updated_at, published_at = EXCLUDED.published_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at, COALESCE(payload_doc_id, '')`
	listingUnpublishQuery = `UPDATE sites.listings
		SET status = 'unpublished', updated_at = NOW()
		WHERE tenant_id = $1 AND property_id = $2
		RETURNING id, tenant_id, site_id, property_id, seo_slug`
	listingPayloadDocQuery = `UPDATE sites.listings SET payload_doc_id = $3, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2`
	listingPublicQuery = `SELECT l.id, l.property_id, l.site_id, l.tenant_id, l.seo_slug, s.enabled
		FROM sites.listings l
		JOIN sites.sites s ON s.id = l.site_id AND s.tenant_id = l.tenant_id
		WHERE l.tenant_id = $1 AND l.site_id = $2 AND l.property_id = $3 AND l.status = 'published' AND s.enabled = TRUE`
	listingCountPublishedQuery = `SELECT COUNT(*) FROM sites.listings WHERE tenant_id = $1 AND status = 'published'`
	listingListQuery           = `SELECT ` + listingColumns + ` FROM sites.listings
		WHERE tenant_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`

	listingStatsQuery = `SELECT
			COALESCE(COUNT(*) FILTER (WHERE created_at >= $3::timestamptz), 0),
			COALESCE(COUNT(*) FILTER (WHERE created_at >= $4::timestamptz), 0),
			COALESCE(MAX(created_at), NULL::timestamptz)
		FROM sites.listing_views WHERE tenant_id = $1 AND property_id = $2`
	leadStatsQuery = `SELECT
			COALESCE(COUNT(*) FILTER (WHERE created_at >= $3::timestamptz), 0),
			COALESCE(COUNT(*) FILTER (WHERE created_at >= $4::timestamptz), 0)
		FROM sites.leads WHERE tenant_id = $1 AND property_id = $2`
	siteStatsQuery = `SELECT
			(SELECT COUNT(*) FROM sites.listing_views v WHERE v.tenant_id = $1 AND v.created_at >= $2),
			(SELECT COUNT(*) FROM sites.listing_views v WHERE v.tenant_id = $1 AND v.created_at >= $3),
			(SELECT COUNT(*) FROM sites.leads l WHERE l.tenant_id = $1 AND l.created_at >= $2),
			(SELECT COUNT(*) FROM sites.leads l WHERE l.tenant_id = $1 AND l.created_at >= $3),
			(SELECT COUNT(*) FROM sites.listings l WHERE l.tenant_id = $1 AND l.status = 'published'),
			(SELECT COUNT(*) FROM sites.listings l WHERE l.tenant_id = $1)`

	leadInsertQuery = `INSERT INTO sites.leads
			(id, tenant_id, site_id, listing_id, property_id, name, email, phone, message, status, origin_host,
			 utm_source, utm_medium, utm_campaign, error)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13,$14,NULLIF($15,''))
		RETURNING created_at, updated_at`
	leadUpdateQuery = `UPDATE sites.leads
		SET status = $3, crm_lead_id = NULLIF($4,''), error = NULLIF($5,''), updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING updated_at`
	leadDuplicateQuery = `SELECT ` + leadColumns + ` FROM sites.leads
		WHERE tenant_id = $1 AND created_at >= $5
		  AND ((NULLIF($2, '')::uuid IS NULL AND property_id IS NULL)
		    OR property_id = NULLIF($2, '')::uuid)
		  AND ((NULLIF($3, '') IS NOT NULL AND lower(email) = lower($3))
		    OR (NULLIF($4, '') IS NOT NULL AND phone = $4))
		ORDER BY created_at DESC LIMIT 1`
	leadListQuery = `SELECT ` + leadColumns + ` FROM sites.leads
		WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	leadCountQuery = `SELECT COUNT(*) FROM sites.leads WHERE tenant_id = $1`
)

const listingColumns = `id, tenant_id, site_id, property_id, seo_slug, status, title, description,
	listing_type, property_type, sale_price, rent_price, condo_fee, property_tax,
	address, features, amenities, images, source_updated_at, published_at, COALESCE(payload_doc_id, ''), created_at, updated_at`

const leadColumns = `id, tenant_id, site_id, COALESCE(listing_id::text, ''), COALESCE(property_id::text, ''),
	name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(message, ''), status, COALESCE(crm_lead_id, ''),
	origin_host, COALESCE(utm_source, ''), COALESCE(utm_medium, ''), COALESCE(utm_campaign, ''),
	COALESCE(error, ''), created_at, updated_at`

// ---------------------------------------------------------------------------
// Sites
// ---------------------------------------------------------------------------

func (r *Repository) GetByTenant(ctx context.Context, tenantID string) (*site.Site, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	return r.scanSite(r.db.QueryRowContext(ctx, siteByTenantQuery, tenantID))
}

// ListEnabledSlugs é a leitura cross-tenant usada pelo sincronizador de TLS.
// Devolve apenas hosts que podem receber tráfego público e usa o pool de
// serviço; a lista não contém dados de contato nem branding.
func (r *Repository) ListEnabledSlugs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryServiceRows(ctx, `SELECT slug FROM sites.sites WHERE enabled ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slugs := make([]string, 0)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}

// FindBySlug é a resolução pública por Host. Devolve ErrSiteDisabled para site
// desligado para que o handler responda 404 — o comprador não deve enxergar
// que existe uma vitrine desligada naquele endereço.
func (r *Repository) FindBySlug(ctx context.Context, slug string) (*site.Site, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, site.ErrNotFound
	}
	// Única escotilha cross-tenant do repositório: o Host ainda não revelou o
	// tenant, então a checagem de RLS seria circular. O retorno é restrito a
	// site habilitado e o tenant só é usado depois, escopado.
	s, err := r.scanSite(r.db.QueryServiceRow(ctx, siteBySlugQuery, slug))
	if err != nil {
		return nil, err
	}
	if !s.Enabled {
		return nil, site.ErrSiteDisabled
	}
	return s, nil
}

func (r *Repository) scanSite(row rowScanner) (*site.Site, error) {
	var (
		s         site.Site
		branding  []byte
		contact   []byte
		createdAt time.Time
		updatedAt time.Time
	)
	err := row.Scan(&s.ID, &s.TenantID, &s.Slug, &s.Name, &s.Enabled, &s.PublicBaseURL, &branding,
		&contact, &s.PayloadTenantID, &createdAt, &updatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, site.ErrNotFound
		}
		return nil, fmt.Errorf("load site: %w", err)
	}
	if len(branding) > 0 {
		if err := json.Unmarshal(branding, &s.Branding); err != nil {
			return nil, fmt.Errorf("decode site branding: %w", err)
		}
	}
	if len(contact) > 0 {
		if err := json.Unmarshal(contact, &s.Contact); err != nil {
			return nil, fmt.Errorf("decode site contact: %w", err)
		}
	}
	s.CreatedAt = createdAt
	s.UpdatedAt = updatedAt
	return &s, nil
}

func (r *Repository) Create(ctx context.Context, s *site.Site) error {
	if strings.TrimSpace(s.TenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, s.TenantID)
	branding, err := json.Marshal(s.Branding)
	if err != nil {
		return err
	}
	contact, err := json.Marshal(s.Contact)
	if err != nil {
		return err
	}
	query := `INSERT INTO sites.sites (tenant_id, slug, name, enabled, public_base_url, branding, contact, payload_tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''))
		RETURNING id, created_at, updated_at`
	err = r.db.QueryRowContext(ctx, query, s.TenantID, s.Slug, s.Name, s.Enabled, s.PublicBaseURL, string(branding), string(contact), s.PayloadTenantID).
		Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return mapConstraint(err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, s *site.Site) error {
	if strings.TrimSpace(s.TenantID) == "" || strings.TrimSpace(s.ID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, s.TenantID)
	branding, err := json.Marshal(s.Branding)
	if err != nil {
		return err
	}
	contact, err := json.Marshal(s.Contact)
	if err != nil {
		return err
	}
	res := r.db.QueryRowContext(ctx, siteUpdateQuery, s.TenantID, s.ID, s.Slug, s.Name, s.Enabled, s.PublicBaseURL, string(branding), string(contact))
	if err := res.Scan(&s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return site.ErrNotFound
		}
		return mapConstraint(err)
	}
	return nil
}

func (r *Repository) SlugTaken(ctx context.Context, tenantID, slug string) (bool, error) {
	if strings.TrimSpace(tenantID) == "" {
		return true, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	var taken bool
	if err := r.db.QueryRowContext(ctx, siteSlugTakenQuery, slug, tenantID).Scan(&taken); err != nil {
		return false, err
	}
	return taken, nil
}

func (r *Repository) SetPayloadTenantID(ctx context.Context, tenantID, siteID, payloadTenantID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	_, err := r.db.ExecContext(ctx, `UPDATE sites.sites SET payload_tenant_id = NULLIF($3,''), updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2`, tenantID, siteID, payloadTenantID)
	return err
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

func (r *Repository) ListPages(ctx context.Context, tenantID, siteID string) ([]site.Page, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	rows, err := r.db.QueryContext(ctx, pagesBySiteQuery, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]site.Page, 0, 3)
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *Repository) GetPage(ctx context.Context, tenantID, siteID string, key site.PageKey) (*site.Page, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	row := r.db.QueryRowContext(ctx, pageByKeyQuery, tenantID, siteID, string(key))
	p, err := scanPage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *Repository) UpsertPages(ctx context.Context, tenantID, siteID string, pages []site.Page) error {
	if strings.TrimSpace(tenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	for _, p := range pages {
		// pgx não converte []byte em jsonb (envia bytea); a string preserva o
		// JSON e é aceita pelo parser do Postgres.
		var body interface{}
		if len(p.Body) > 0 {
			body = string(p.Body)
		}
		row := r.db.QueryRowContext(ctx, pageUpsertQuery, tenantID, siteID, string(p.Key), p.Title, p.Subtitle,
			body, p.SEOTitle, p.SEODescription)
		var id string
		var updatedAt time.Time
		if err := row.Scan(&id, &updatedAt); err != nil {
			return fmt.Errorf("upsert page %s: %w", p.Key, err)
		}
	}
	return nil
}

type pageScanner interface {
	Scan(dest ...interface{}) error
}

func scanPage(row pageScanner) (*site.Page, error) {
	var (
		p        site.Page
		key      string
		body     []byte
		subtitle sql.NullString
		seoTitle sql.NullString
		seoDesc  sql.NullString
	)
	if err := row.Scan(&p.ID, &p.TenantID, &p.SiteID, &key, &p.Title, &subtitle, &body, &seoTitle, &seoDesc, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Key = site.PageKey(key)
	p.Subtitle = subtitle.String
	p.SEOTitle = seoTitle.String
	p.SEODescription = seoDesc.String
	if len(body) > 0 {
		p.Body = json.RawMessage(body)
	}
	return &p, nil
}

// ---------------------------------------------------------------------------
// Listings
// ---------------------------------------------------------------------------

func (r *Repository) Upsert(ctx context.Context, l *site.Listing) (*site.Listing, error) {
	if strings.TrimSpace(l.TenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, l.TenantID)
	address, err := json.Marshal(l.Address)
	if err != nil {
		return nil, err
	}
	features, err := json.Marshal(l.Features)
	if err != nil {
		return nil, err
	}
	amenities, err := json.Marshal(l.Amenities)
	if err != nil {
		return nil, err
	}
	images, err := json.Marshal(l.Images)
	if err != nil {
		return nil, err
	}
	if l.Amenities == nil {
		amenities = []byte("[]")
	}
	if l.Images == nil {
		images = []byte("[]")
	}
	row := r.db.QueryRowContext(ctx, listingUpsertQuery,
		l.TenantID, l.SiteID, l.PropertyID, l.SeoSlug, string(l.Status), l.Title, l.Description,
		l.ListingType, l.PropertyType, l.SalePrice, l.RentPrice, l.CondoFee, l.PropertyTax,
		string(address), string(features), string(amenities), string(images), l.SourceUpdatedAt, l.PublishedAt)
	err = row.Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt, &l.PayloadDocID)
	if err != nil {
		return nil, mapConstraint(err)
	}
	return l, nil
}

func (r *Repository) GetByProperty(ctx context.Context, tenantID, propertyID string) (*site.Listing, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(propertyID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	l, err := scanListing(r.db.QueryRowContext(ctx, listingByPropertyQuery, tenantID, propertyID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	return l, nil
}

func (r *Repository) SeoSlugTaken(ctx context.Context, tenantID, seoSlug, propertyID string) (bool, error) {
	if strings.TrimSpace(tenantID) == "" {
		return true, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	var taken bool
	if err := r.db.QueryRowContext(ctx, listingSeoSlugTakenQuery, tenantID, seoSlug, propertyID).Scan(&taken); err != nil {
		return false, err
	}
	return taken, nil
}

func (r *Repository) MarkUnpublished(ctx context.Context, tenantID, propertyID string) (*site.Listing, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	var l site.Listing
	row := r.db.QueryRowContext(ctx, listingUnpublishQuery, tenantID, propertyID)
	if err := row.Scan(&l.ID, &l.TenantID, &l.SiteID, &l.PropertyID, &l.SeoSlug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	l.Status = site.ListingUnpublished
	return &l, nil
}

func (r *Repository) SetPayloadDocID(ctx context.Context, tenantID, listingID, payloadDocID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	_, err := r.db.ExecContext(ctx, listingPayloadDocQuery, tenantID, listingID, payloadDocID)
	return err
}

func (r *Repository) GetPublishedForPublic(ctx context.Context, tenantID, siteID, propertyID string) (*site.PublicListing, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	var p site.PublicListing
	err := r.db.QueryRowContext(ctx, listingPublicQuery, tenantID, siteID, propertyID).
		Scan(&p.ListingID, &p.PropertyID, &p.SiteID, &p.TenantID, &p.SeoSlug, &p.Enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repository) CountPublished(ctx context.Context, tenantID string) (int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return 0, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	var n int64
	if err := r.db.QueryRowContext(ctx, listingCountPublishedQuery, tenantID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *Repository) ListByTenant(ctx context.Context, tenantID string, limit, offset int64) ([]site.Listing, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	rows, err := r.db.QueryContext(ctx, listingListQuery, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]site.Listing, 0, limit)
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

type listingScanner interface {
	Scan(dest ...interface{}) error
}

func scanListing(row listingScanner) (*site.Listing, error) {
	var (
		l         site.Listing
		status    string
		address   []byte
		features  []byte
		amenities []byte
		images    []byte
		published sql.NullTime
		sourceUpd sql.NullTime
		createdAt time.Time
		updatedAt time.Time
	)
	err := row.Scan(&l.ID, &l.TenantID, &l.SiteID, &l.PropertyID, &l.SeoSlug, &status, &l.Title, &l.Description,
		&l.ListingType, &l.PropertyType, &l.SalePrice, &l.RentPrice, &l.CondoFee, &l.PropertyTax,
		&address, &features, &amenities, &images, &sourceUpd, &published, &l.PayloadDocID, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	l.Status = site.ListingStatus(status)
	if len(address) > 0 {
		_ = json.Unmarshal(address, &l.Address)
	}
	if len(features) > 0 {
		_ = json.Unmarshal(features, &l.Features)
	}
	if len(amenities) > 0 {
		_ = json.Unmarshal(amenities, &l.Amenities)
	}
	if len(images) > 0 {
		_ = json.Unmarshal(images, &l.Images)
	}
	if sourceUpd.Valid {
		l.SourceUpdatedAt = sourceUpd.Time
	}
	if published.Valid {
		t := published.Time
		l.PublishedAt = &t
	}
	l.CreatedAt = createdAt
	l.UpdatedAt = updatedAt
	return &l, nil
}

// ---------------------------------------------------------------------------
// Engagement (views + leads)
// ---------------------------------------------------------------------------

func (r *Repository) CreateView(ctx context.Context, v *site.ListingView) error {
	if strings.TrimSpace(v.TenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, v.TenantID)
	query := `INSERT INTO sites.listing_views
		(tenant_id, site_id, listing_id, property_id, session_key, path, referrer, utm_source, utm_medium, utm_campaign, country)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''))
		RETURNING id, created_at`
	return r.db.QueryRowContext(ctx, query, v.TenantID, v.SiteID, v.ListingID, v.PropertyID, v.SessionKey,
		v.Path, v.Referrer, v.UTMSource, v.UTMMedium, v.UTMCampaign, v.Country).Scan(&v.ID, &v.CreatedAt)
}

func (r *Repository) StatsForListing(ctx context.Context, tenantID, propertyID string, now time.Time) (*site.ListingStats, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	stats := &site.ListingStats{PropertyID: propertyID}
	week := now.Add(-7 * 24 * time.Hour)
	month := now.Add(-30 * 24 * time.Hour)
	var lastView sql.NullTime
	if err := r.db.QueryRowContext(ctx, listingStatsQuery, tenantID, propertyID, week, month).
		Scan(&stats.Views7d, &stats.Views30d, &lastView); err != nil {
		return nil, err
	}
	if lastView.Valid {
		stats.LastViewAt = lastView.Time.UTC().Format(time.RFC3339)
	}
	if err := r.db.QueryRowContext(ctx, leadStatsQuery, tenantID, propertyID, week, month).
		Scan(&stats.Leads7d, &stats.Leads30d); err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *Repository) StatsForSite(ctx context.Context, tenantID string, now time.Time) (*site.SiteStats, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	week := now.Add(-7 * 24 * time.Hour)
	month := now.Add(-30 * 24 * time.Hour)
	stats := &site.SiteStats{}
	err := r.db.QueryRowContext(ctx, siteStatsQuery, tenantID, week, month).Scan(
		&stats.Views7d, &stats.Views30d, &stats.Leads7d, &stats.Leads30d,
		&stats.PublishedListings, &stats.TotalListings,
	)
	if err != nil {
		return nil, err
	}
	stats.UnpublishedListings = stats.TotalListings - stats.PublishedListings
	return stats, nil
}

func (r *Repository) CreateLead(ctx context.Context, l *site.Lead) error {
	if strings.TrimSpace(l.TenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, l.TenantID)
	err := r.db.QueryRowContext(ctx, leadInsertQuery,
		l.ID, l.TenantID, l.SiteID, l.ListingID, l.PropertyID, l.Name, l.Email, l.Phone, l.Message,
		string(l.Status), l.OriginHost, l.UTMSource, l.UTMMedium, l.UTMCampaign, l.Error,
	).Scan(&l.CreatedAt, &l.UpdatedAt)
	return mapConstraint(err)
}

func (r *Repository) UpdateLead(ctx context.Context, l *site.Lead) error {
	if strings.TrimSpace(l.TenantID) == "" {
		return site.ErrNotFound
	}
	ctx = withTenant(ctx, l.TenantID)
	err := r.db.QueryRowContext(ctx, leadUpdateQuery, l.TenantID, l.ID, string(l.Status), l.CRMLeadID, l.Error).
		Scan(&l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return site.ErrNotFound
	}
	return err
}

func (r *Repository) FindDuplicateLead(ctx context.Context, tenantID, propertyID, email, phone string, since time.Time) (*site.Lead, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	l, err := scanLead(r.db.QueryRowContext(ctx, leadDuplicateQuery, tenantID, propertyID, email, phone, since))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	return l, nil
}

func (r *Repository) ListLeads(ctx context.Context, tenantID string, limit, offset int64) ([]site.Lead, int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, site.ErrNotFound
	}
	ctx = withTenant(ctx, tenantID)
	rows, err := r.db.QueryContext(ctx, leadListQuery, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]site.Lead, 0, limit)
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, leadCountQuery, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type leadScanner interface {
	Scan(dest ...interface{}) error
}

func scanLead(row leadScanner) (*site.Lead, error) {
	var (
		l      site.Lead
		status string
	)
	err := row.Scan(&l.ID, &l.TenantID, &l.SiteID, &l.ListingID, &l.PropertyID, &l.Name, &l.Email, &l.Phone,
		&l.Message, &status, &l.CRMLeadID, &l.OriginHost, &l.UTMSource, &l.UTMMedium, &l.UTMCampaign,
		&l.Error, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	l.Status = site.LeadStatus(status)
	return &l, nil
}

// mapConstraint traduz violação de unicidade em erro de domínio. Evita que o
// handler devolva 500 para um slug duplicado (que é 409/400).
func mapConstraint(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "SQLSTATE 23505") {
		if strings.Contains(msg, "slug") {
			return site.ErrSlugTaken
		}
	}
	// Constraints semânticas (novas e antigas) não podem virar 500 quando
	// alguém grava direto no banco/uma migração antiga escapa da validação.
	if strings.Contains(msg, "sites_slug_semantic") ||
		strings.Contains(msg, "sites_slug_not_reserved") ||
		strings.Contains(msg, "sites_listings_seo_slug_format") {
		return fmt.Errorf("%w: formato inválido", site.ErrSlugInvalid)
	}
	return err
}

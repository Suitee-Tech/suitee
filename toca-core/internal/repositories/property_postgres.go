package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
)

type propertyPostgresRepository struct {
	db *database.PostgresDB
}

// NewPropertyPostgresRepository creates a new property repository backed by PostgreSQL
func NewPropertyPostgresRepository(db *database.PostgresDB) PropertyRepository {
	return &propertyPostgresRepository{
		db: db,
	}
}

func (r *propertyPostgresRepository) Create(ctx context.Context, p *property.Property) error {
	tenantID, _ := appCtx.GetTenantID(ctx)
	if tenantID == "" && p.TenantID != "" {
		tenantID = p.TenantID
	}
	if tenantID == "" {
		return fmt.Errorf("tenant_id required")
	}

	p.TenantID = tenantID

	if p.ID == "" {
		p.ID = uuid.NewString()
	}

	metadataBytes, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to serialize property metadata: %w", err)
	}

	query := `
		INSERT INTO core.properties (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	err = r.db.QueryRowContext(ctx, query,
		p.ID,
		tenantID,
		string(metadataBytes),
	).Scan(&p.ID)

	if err != nil {
		return fmt.Errorf("erro ao inserir propriedade no postgres: %w", err)
	}

	return nil
}

func (r *propertyPostgresRepository) GetByID(ctx context.Context, id, tenantID string) (*property.Property, error) {
	ctxTenantID, _ := appCtx.GetTenantID(ctx)
	if ctxTenantID == "" {
		ctxTenantID = tenantID
	}

	query := `SELECT id, metadata FROM core.properties WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`

	var dbID string
	var metadataBytes []byte
	err := r.db.QueryRowContext(ctx, query, id, ctxTenantID).Scan(&dbID, &metadataBytes)
	if err != nil {
		return nil, fmt.Errorf("property not found or access denied: %w", err)
	}

	var p property.Property
	if err := json.Unmarshal(metadataBytes, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal property metadata: %w", err)
	}
	if p.ID == "" {
		p.ID = dbID
	}

	return &p, nil
}

func (r *propertyPostgresRepository) GetByIDs(ctx context.Context, ids []string, tenantID string) ([]*property.Property, error) {
	ctxTenantID, _ := appCtx.GetTenantID(ctx)
	if ctxTenantID == "" {
		ctxTenantID = tenantID
	}
	if ctxTenantID == "" {
		return nil, fmt.Errorf("tenant_id required")
	}

	q, err := database.NewJSONBQuery(r.db, "core", "properties", true)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetByIDs(ctx, ctxTenantID, ids)
	if err != nil {
		return nil, err
	}

	out := make([]*property.Property, 0, len(rows))
	for _, row := range rows {
		var p property.Property
		if err := json.Unmarshal(row.Metadata, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			p.ID = row.ID
		}
		if p.TenantID == "" {
			p.TenantID = ctxTenantID
		}
		out = append(out, &p)
	}
	return out, nil
}

func (r *propertyPostgresRepository) GetAll(ctx context.Context, filter property.PropertyFilter) ([]*property.Property, int64, error) {
	tenantID := filter.TenantID
	if tenantID == "" {
		tenantID, _ = appCtx.GetTenantID(ctx)
	}
	if tenantID == "" {
		return nil, 0, fmt.Errorf("tenant_id required")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []interface{}{tenantID}
	argPos := 2

	if filter.Query != "" {
		where = append(where, "(metadata->>'title' ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,street}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,neighborhood}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,city}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,formatted_address}','') ILIKE $"+fmt.Sprint(argPos)+")")
		args = append(args, "%"+filter.Query+"%")
		argPos++
	}
	if filter.Address != "" {
		where = append(where, "(COALESCE(metadata#>>'{address,street}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,formatted_address}','') ILIKE $"+fmt.Sprint(argPos)+")")
		args = append(args, "%"+filter.Address+"%")
		argPos++
	}
	if filter.Neighborhood != "" {
		where = append(where, "COALESCE(metadata#>>'{address,neighborhood}','') ILIKE $"+fmt.Sprint(argPos))
		args = append(args, "%"+filter.Neighborhood+"%")
		argPos++
	}
	if filter.City != "" {
		where = append(where, "COALESCE(metadata#>>'{address,city}','') ILIKE $"+fmt.Sprint(argPos))
		args = append(args, "%"+filter.City+"%")
		argPos++
	}
	if filter.State != "" {
		where = append(where, "(COALESCE(metadata#>>'{address,state}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,stateCode}','') ILIKE $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,state_code}','') ILIKE $"+fmt.Sprint(argPos)+")")
		args = append(args, "%"+filter.State+"%")
		argPos++
	}
	if filter.ZipCode != "" {
		where = append(where, "(COALESCE(metadata#>>'{address,postalCode}','') = $"+fmt.Sprint(argPos)+" OR COALESCE(metadata#>>'{address,postal_code}','') = $"+fmt.Sprint(argPos)+")")
		args = append(args, filter.ZipCode)
		argPos++
	}
	if filter.Status != "" {
		where = append(where, "metadata->>'status' = $"+fmt.Sprint(argPos))
		args = append(args, string(filter.Status))
		argPos++
	}
	if filter.PropertyType != "" {
		where = append(where, "metadata->>'property_type' = $"+fmt.Sprint(argPos))
		args = append(args, string(filter.PropertyType))
		argPos++
	}
	if filter.ListingType != "" {
		where = append(where, "metadata->>'listing_type' = $"+fmt.Sprint(argPos))
		args = append(args, string(filter.ListingType))
		argPos++
	}
	if filter.AvailableFor != "" {
		// New properties can advertise more than one modality in
		// available_for. Legacy records only have listing_type, so keep it as
		// the fallback instead of dropping those records from a match.
		where = append(where, "(COALESCE(metadata->'available_for', '[]'::jsonb) ? $"+fmt.Sprint(argPos)+" OR metadata->>'listing_type' = $"+fmt.Sprint(argPos)+")")
		args = append(args, string(filter.AvailableFor))
		argPos++
	}
	if filter.OwnerID != "" {
		where = append(where, "metadata->'owners' @> $"+fmt.Sprint(argPos)+"::jsonb")
		args = append(args, fmt.Sprintf(`[{"lead_id":"%s"}]`, filter.OwnerID))
		argPos++
	}
	if filter.CapturingBrokerID != "" {
		where = append(where, "metadata->'capturing_brokers' @> $"+fmt.Sprint(argPos)+"::jsonb")
		args = append(args, fmt.Sprintf(`[{"broker_id":"%s"}]`, filter.CapturingBrokerID))
		argPos++
	}
	if filter.InterestedLeadID != "" {
		where = append(where, "metadata->'interested_leads' @> $"+fmt.Sprint(argPos)+"::jsonb")
		args = append(args, fmt.Sprintf(`[{"lead_id":"%s"}]`, filter.InterestedLeadID))
		argPos++
	}
	priceExpr := "COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, (metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0)"
	priceListingType := filter.ListingType
	if priceListingType == "" {
		priceListingType = filter.AvailableFor
	}
	if priceListingType == property.ListingTypeRent {
		priceExpr = "COALESCE((metadata#>>'{pricing,rent_price,current_amount}')::numeric, 0)"
	} else if priceListingType == property.ListingTypeSale {
		priceExpr = "COALESCE((metadata#>>'{pricing,sale_price,current_amount}')::numeric, 0)"
	}
	if filter.MinPrice != nil {
		where = append(where, priceExpr+" >= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MinPrice)
		argPos++
	}
	if filter.MaxPrice != nil {
		where = append(where, priceExpr+" <= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MaxPrice)
		argPos++
	}
	if filter.MinBedrooms != nil {
		where = append(where, "COALESCE((metadata#>>'{features,bedrooms}')::int, 0) >= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MinBedrooms)
		argPos++
	}
	if filter.MinBathrooms != nil {
		where = append(where, "COALESCE((metadata#>>'{features,bathrooms}')::int, 0) >= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MinBathrooms)
		argPos++
	}

	areaExpr := "COALESCE(NULLIF((metadata#>>'{areas,total_area_m2}')::numeric,0), NULLIF((metadata#>>'{areas,usable_area_m2}')::numeric,0), NULLIF((metadata#>>'{areas,built_area_m2}')::numeric,0), 0)"
	if filter.MinArea != nil {
		where = append(where, areaExpr+" >= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MinArea)
		argPos++
	}
	if filter.MaxArea != nil {
		where = append(where, areaExpr+" <= $"+fmt.Sprint(argPos))
		args = append(args, *filter.MaxArea)
		argPos++
	}

	whereSQL := strings.Join(where, " AND ")

	var total int64
	countQuery := "SELECT COUNT(1) FROM core.properties WHERE " + whereSQL
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := "SELECT id, metadata FROM core.properties WHERE " + whereSQL + " ORDER BY created_at DESC LIMIT $" + fmt.Sprint(argPos) + " OFFSET $" + fmt.Sprint(argPos+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]*property.Property, 0)
	for rows.Next() {
		var dbID string
		var metadata []byte
		if err := rows.Scan(&dbID, &metadata); err != nil {
			return nil, 0, err
		}
		var p property.Property
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, 0, err
		}
		if p.ID == "" {
			p.ID = dbID
		}
		result = append(result, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return result, total, nil
}

func (r *propertyPostgresRepository) Update(ctx context.Context, p *property.Property) error {
	tenantID, _ := appCtx.GetTenantID(ctx)
	if tenantID == "" && p.TenantID != "" {
		tenantID = p.TenantID
	}
	if tenantID == "" {
		return fmt.Errorf("tenant_id required")
	}

	metadataBytes, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to serialize property metadata: %w", err)
	}

	query := `
		UPDATE core.properties
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(ctx, query, string(metadataBytes), p.ID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update property in postgres: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("property not found or no changes made")
	}

	return nil
}

// AppendImage usa um único UPDATE com JSONB para que uploads concorrentes não
// sobrescrevam uns aos outros. O order é calculado na própria linha, já sob o
// row lock do PostgreSQL.
func (r *propertyPostgresRepository) AppendImage(ctx context.Context, id, tenantID string, image property.Image) (*property.Property, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("property id and tenant_id required")
	}
	query := `
		UPDATE core.properties
		SET metadata = jsonb_set(
			metadata,
			'{media,images}',
			COALESCE(metadata->'media'->'images', '[]'::jsonb) ||
			jsonb_build_array(jsonb_build_object(
				'url', $1::text,
				'description', $2::text,
				'order', jsonb_array_length(COALESCE(metadata->'media'->'images', '[]'::jsonb))
			)),
			true
		), updated_at = NOW()
		WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL
		RETURNING metadata
	`
	var metadata []byte
	if err := r.db.QueryRowContext(ctx, query, image.URL, image.Description, id, tenantID).Scan(&metadata); err != nil {
		return nil, fmt.Errorf("append property image: %w", err)
	}
	var p property.Property
	if err := json.Unmarshal(metadata, &p); err != nil {
		return nil, fmt.Errorf("decode property after append: %w", err)
	}
	if p.ID == "" {
		p.ID = id
	}
	return &p, nil
}

// ListSharedForPartner returns every property owned by ownerTenantID that is
// flagged for the partnership portfolio and whose capture is not exclusive.
//
// SECURITY: this query intentionally bypasses the usual tenant scoping (it
// filters by ownerTenantID, NOT by the JWT's tenant). It must only be invoked
// from PartnershipService.SharedPortfolioForPartnership, which first verifies
// the caller belongs to an active partnership granting shared_portfolio view.
func (r *propertyPostgresRepository) ListSharedForPartner(ctx context.Context, ownerTenantID string, propertyTypes, regions []string) ([]*property.Property, error) {
	if ownerTenantID == "" {
		return nil, fmt.Errorf("owner tenant_id required")
	}

	where := []string{
		"tenant_id = $1",
		"deleted_at IS NULL",
		"(metadata->>'partnership_portfolio') = 'true'",
		"COALESCE(metadata->>'capture_exclusivity','open') <> 'exclusive'",
	}
	args := []interface{}{ownerTenantID}
	argPos := 2

	// Autonomy rule: the owner company may restrict sharing to specific
	// property types. Empty list = all types.
	if len(propertyTypes) > 0 {
		placeholders := make([]string, 0, len(propertyTypes))
		for _, t := range propertyTypes {
			placeholders = append(placeholders, "$"+fmt.Sprint(argPos))
			args = append(args, t)
			argPos++
		}
		where = append(where, "metadata->>'property_type' IN ("+strings.Join(placeholders, ",")+")")
	}

	// Autonomy rule: the owner company may restrict sharing to specific
	// regions — matched case-insensitively against city/neighborhood/state.
	// Empty list = all regions.
	if len(regions) > 0 {
		ors := make([]string, 0, len(regions))
		for _, region := range regions {
			p := fmt.Sprint(argPos)
			ors = append(ors, "COALESCE(metadata#>>'{address,city}','') ILIKE $"+p+
				" OR COALESCE(metadata#>>'{address,neighborhood}','') ILIKE $"+p+
				" OR COALESCE(metadata#>>'{address,state}','') ILIKE $"+p)
			args = append(args, "%"+region+"%")
			argPos++
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}

	query := "SELECT id, metadata FROM core.properties WHERE " +
		strings.Join(where, " AND ") + " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*property.Property, 0)
	for rows.Next() {
		var dbID string
		var metadata []byte
		if err := rows.Scan(&dbID, &metadata); err != nil {
			return nil, err
		}
		var p property.Property
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			p.ID = dbID
		}
		if p.TenantID == "" {
			p.TenantID = ownerTenantID
		}
		result = append(result, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *propertyPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	ctxTenantID, _ := appCtx.GetTenantID(ctx)
	if ctxTenantID == "" {
		ctxTenantID = tenantID
	}

	query := `
		UPDATE core.properties
		SET deleted_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

	res, err := r.db.ExecContext(ctx, query, id, ctxTenantID)
	if err != nil {
		return fmt.Errorf("failed to delete property in postgres: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("property not found or already deleted")
	}

	return nil
}

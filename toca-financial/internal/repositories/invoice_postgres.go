package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
)

type invoicePostgresRepository struct {
	db *database.PostgresDB
}

func NewInvoicePostgresRepository(db *database.PostgresDB) invoice.Repository {
	return &invoicePostgresRepository{db: db}
}

func (r *invoicePostgresRepository) Create(ctx context.Context, i *invoice.Invoice) error {
	i.CreatedAt = time.Now()
	metadata, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("encode invoice: %w", err)
	}

	if i.SourceEventID != "" {
		query := `
			INSERT INTO financial.invoices (id, tenant_id, metadata)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
			WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
			DO NOTHING
			RETURNING created_at`
		err := r.db.QueryRow(ctx, query, i.ID, i.TenantID, string(metadata)).Scan(&i.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("invoice already exists: %w", invoice.ErrInvoiceAlreadyExists)
		}
		if err != nil {
			return fmt.Errorf("create invoice: %w", err)
		}
		return nil
	}

	query := `INSERT INTO financial.invoices (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, i.ID, i.TenantID, string(metadata)).Scan(&i.CreatedAt)
}

func (r *invoicePostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	return r.getByID(ctx, tenantID, id, false)
}

// GetByIDForUpdate serializes competing invoice payment commands. It must be
// called inside the transaction owned by InvoiceService; after the first
// command commits PAID, the waiter observes that terminal projection and does
// not allocate the same invoice a second time under a different request key.
func (r *invoicePostgresRepository) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	return r.getByID(ctx, tenantID, id, true)
}

func (r *invoicePostgresRepository) getByID(ctx context.Context, tenantID, id string, forUpdate bool) (*invoice.Invoice, error) {
	var metadata []byte
	query := "SELECT metadata FROM financial.invoices WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL"
	if forUpdate {
		query += " FOR UPDATE"
	}
	err := r.db.QueryRow(ctx, query, tenantID, id).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id=%s", invoice.ErrInvoiceNotFound, id)
		}
		return nil, fmt.Errorf("invoice not found: %w", err)
	}
	var i invoice.Invoice
	if err := json.Unmarshal(metadata, &i); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}
	return &i, nil
}

func (r *invoicePostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*invoice.Invoice, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.invoices WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL",
		tenantID, sourceEventID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("invoice lookup by source_event_id: %w", err)
	}
	var i invoice.Invoice
	if err := json.Unmarshal(metadata, &i); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}
	return &i, nil
}

// SumOpenAmountCentsByChargeID is evaluated while InvoiceService holds the
// corresponding Charge row FOR UPDATE. That row lock is the serialization
// point for invoice issuance and charge payments, so a concurrent command
// observes the invoice inserted by the command that acquired the lock first.
func (r *invoicePostgresRepository) SumOpenAmountCentsByChargeID(ctx context.Context, tenantID, chargeID, excludedInvoiceID string) (int64, error) {
	var reservedCents int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(ROUND((metadata->>'amount')::numeric * 100)), 0)::bigint
		FROM financial.invoices
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'charge_id') = $2
		  AND COALESCE(metadata->>'status', '') NOT IN ('PAID', 'CANCELLED')
		  AND ($3 = '' OR id::text <> $3)
	`, tenantID, chargeID, excludedInvoiceID).Scan(&reservedCents)
	if err != nil {
		return 0, fmt.Errorf("sum open invoice reservations for charge %s: %w", chargeID, err)
	}
	return reservedCents, nil
}

func (r *invoicePostgresRepository) GetByReferenceID(ctx context.Context, tenantID, referenceID string) ([]*invoice.Invoice, error) {
	rows, err := r.db.Query(ctx, `
			SELECT metadata
			FROM financial.invoices
			WHERE tenant_id = $1
			  AND deleted_at IS NULL
			  AND (metadata->>'reference_id') = $2
			ORDER BY created_at DESC
		`, tenantID, referenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*invoice.Invoice, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var inv invoice.Invoice
		if err := json.Unmarshal(metadata, &inv); err != nil {
			return nil, err
		}
		out = append(out, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *invoicePostgresRepository) Update(ctx context.Context, i *invoice.Invoice) error {
	i.UpdatedAt = time.Now()
	metadata, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("encode invoice: %w", err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.invoices SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), i.ID, i.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return invoice.ErrInvoiceNotFound
	}
	return nil
}

func (r *invoicePostgresRepository) List(ctx context.Context, tenantID string, status *invoice.Status, referenceID *string) ([]*invoice.Invoice, error) {
	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if status != nil && *status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, string(*status))
		argN++
	}
	if referenceID != nil && *referenceID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_id') = $%d", argN))
		args = append(args, *referenceID)
	}

	query := "SELECT metadata FROM financial.invoices WHERE " + strings.Join(where, " AND ") + " ORDER BY created_at DESC"
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*invoice.Invoice, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var inv invoice.Invoice
		if err := json.Unmarshal(metadata, &inv); err != nil {
			return nil, err
		}
		out = append(out, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *invoicePostgresRepository) Search(ctx context.Context, tenantID string, filter invoice.SearchFilter) ([]*invoice.Invoice, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if v := strings.TrimSpace(filter.ReferenceID); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_id') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.CustomerID); v != "" {
		where = append(where, fmt.Sprintf("((metadata->>'payer_id') = $%d OR (metadata->>'customer_id') = $%d)", argN, argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.CustomerName); v != "" {
		where = append(where, fmt.Sprintf("COALESCE(metadata->>'customer_name','') ILIKE $%d", argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if filter.MinAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'amount')::numeric, 0) >= $%d", argN))
		args = append(args, *filter.MinAmount)
		argN++
	}
	if filter.MaxAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'amount')::numeric, 0) <= $%d", argN))
		args = append(args, *filter.MaxAmount)
		argN++
	}
	if filter.IssuedFrom != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, *filter.IssuedFrom)
		argN++
	}
	if filter.IssuedTo != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, *filter.IssuedTo)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM financial.invoices "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.invoices %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*invoice.Invoice, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var inv invoice.Invoice
		if err := json.Unmarshal(metadata, &inv); err != nil {
			return nil, 0, err
		}
		out = append(out, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

type transactionPostgresRepository struct {
	db *database.PostgresDB
}

func NewTransactionPostgresRepository(db *database.PostgresDB) transaction.RepositoryInterface {
	return &transactionPostgresRepository{db: db}
}

func (r *transactionPostgresRepository) Create(ctx context.Context, t *transaction.Transaction) error {
	t.CreatedAt = time.Now()
	metadata, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal transaction: %w", err)
	}
	if t.SourceEventID != "" {
		query := `INSERT INTO financial.transactions (id, tenant_id, metadata) VALUES ($1, $2, $3)
		          ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
		          WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
		          DO NOTHING
		          RETURNING created_at`
		row := r.db.QueryRow(ctx, query, t.ID, t.TenantID, string(metadata))
		if err := row.Scan(&t.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return transaction.ErrTransactionAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `INSERT INTO financial.transactions (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, t.ID, t.TenantID, string(metadata)).Scan(&t.CreatedAt)
}

func (r *transactionPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*transaction.Transaction, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, "SELECT metadata FROM financial.transactions WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, transaction.ErrTransactionNotFound
		}
		return nil, fmt.Errorf("transaction not found: %w", err)
	}
	var t transaction.Transaction
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, fmt.Errorf("decode transaction metadata: %w", err)
	}
	return &t, nil
}

func (r *transactionPostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	if sourceEventID == "" || tenantID == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(
		ctx,
		"SELECT metadata FROM financial.transactions WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL LIMIT 1",
		tenantID,
		sourceEventID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("transaction lookup by source_event_id failed: %w", err)
	}
	var t transaction.Transaction
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, fmt.Errorf("decode transaction metadata: %w", err)
	}
	return &t, nil
}

// ListByOperationID returns every immutable posting line created by one manual
// command. It is used to hydrate pre-waterfall receipt rows during settlement.
func (r *transactionPostgresRepository) ListByOperationID(ctx context.Context, tenantID, operationID string) ([]*transaction.Transaction, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(operationID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata
		FROM financial.transactions
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'operation_id') = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*transaction.Transaction, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var item transaction.Transaction
		if err := json.Unmarshal(metadata, &item); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *transactionPostgresRepository) Update(ctx context.Context, t *transaction.Transaction) error {
	t.UpdatedAt = time.Now()
	metadata, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal transaction: %w", err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.transactions SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), t.ID, t.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return transaction.ErrTransactionNotFound
	}
	return nil
}

// UpdateIfVersion protects lifecycle transitions and editor writes against a
// lost update on the JSON aggregate. Legacy rows without updated_at may be
// claimed once with an expected zero revision.
func (r *transactionPostgresRepository) UpdateIfVersion(ctx context.Context, t *transaction.Transaction, expected time.Time) (bool, error) {
	t.UpdatedAt = time.Now()
	metadata, err := json.Marshal(t)
	if err != nil {
		return false, fmt.Errorf("marshal transaction: %w", err)
	}
	// Passa a versão esperada como string UTC RFC3339 (em vez de time.Time)
	// para eliminar a ambiguidade de encoding do driver na comparação
	// `timestamptz = $param`: o mesmo instante pode ser truncado pelo driver
	// (micros) e arredondado pelo parse do texto do metadata, gerando um falso
	// "modified concurrently". Mesmo padrão já usado em charge/transfer.
	expectedStr := ""
	if !expected.IsZero() {
		expectedStr = expected.UTC().Format(time.RFC3339Nano)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.transactions
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND (
			NULLIF(metadata->>'updated_at', '')::timestamptz = NULLIF($4, '')::timestamptz
			OR ($5 AND NOT (metadata ? 'updated_at'))
		  )
	`, string(metadata), t.ID, t.TenantID, expectedStr, expected.IsZero())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (r *transactionPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	result, err := r.db.Exec(ctx, "UPDATE financial.transactions SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return transaction.ErrTransactionNotFound
	}
	return nil
}

func (r *transactionPostgresRepository) List(ctx context.Context, tenantID string, contractID string, limit, offset int64) ([]*transaction.Transaction, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if contractID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, contractID)
		argN++
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.transactions "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.transactions %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*transaction.Transaction, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t transaction.Transaction
		if err := json.Unmarshal(metadata, &t); err != nil {
			return nil, 0, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CreateBatch insere múltiplas transações em uma única tx de banco.
// Atomicidade total: se qualquer INSERT falhar, todas são rollback.
func (r *transactionPostgresRepository) CreateBatch(ctx context.Context, txs []*transaction.Transaction) error {
	type batchRow struct {
		tx       *transaction.Transaction
		metadata []byte
	}

	rows := make([]batchRow, 0, len(txs))
	for i, t := range txs {
		if t == nil {
			return fmt.Errorf("batch transaction %d is nil", i)
		}
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("batch transaction %d has no posting id", i)
		}
		if strings.TrimSpace(t.TenantID) == "" {
			return fmt.Errorf("batch transaction %d has no tenant_id", i)
		}
		if strings.TrimSpace(t.SourceEventID) == "" {
			return fmt.Errorf("batch transaction %d has no source_event_id", i)
		}
		metadata, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("marshal tx %s: %w", t.ID, err)
		}
		rows = append(rows, batchRow{tx: t, metadata: metadata})
	}

	persist := func(execCtx context.Context) error {
		for _, row := range rows {
			t := row.tx
			result, err := r.db.Exec(execCtx,
				`INSERT INTO financial.transactions (id, tenant_id, metadata, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
			 WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
			 DO NOTHING`,
				t.ID, t.TenantID, string(row.metadata), t.CreatedAt, t.UpdatedAt)
			if err != nil {
				return fmt.Errorf("insert tx %s: %w", t.ID, err)
			}
			// A duplicate is valid only when it is the canonical retry of the
			// same stable posting. A source collision against another primary key
			// would otherwise make a partial batch look successful.
			affected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("batch result for tx %s: %w", t.ID, err)
			}
			if affected == 0 {
				var canonicalID string
				lookupErr := r.db.QueryRow(execCtx,
					`SELECT id::text
					 FROM financial.transactions
					 WHERE tenant_id = $1
					   AND (metadata->>'source_event_id') = $2
					   AND deleted_at IS NULL`,
					t.TenantID, t.SourceEventID,
				).Scan(&canonicalID)
				if lookupErr != nil {
					return fmt.Errorf("lookup batch conflict for tx %s: %w", t.ID, lookupErr)
				}
				if canonicalID != t.ID {
					return fmt.Errorf("batch source_event_id %s belongs to posting %s, expected %s", t.SourceEventID, canonicalID, t.ID)
				}
			}
		}
		return nil
	}

	// Participate in a surrounding service transaction when present. Otherwise
	// own one SQL transaction for the whole manual posting.
	if database.TxFromCtx(ctx) != nil {
		return persist(ctx)
	}
	return r.db.RunInTx(ctx, func(dbTx *sqlx.Tx) error {
		return persist(database.WithTx(ctx, dbTx))
	})
}

func (r *transactionPostgresRepository) Search(ctx context.Context, tenantID string, filter transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
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

	if v := strings.TrimSpace(filter.ContractID); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Category); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'category') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Type); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'type') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Description); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'description') ILIKE $%d", argN))
		args = append(args, "%"+v+"%")
		argN++
	}

	if filter.MinAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'value')::numeric, 0) >= $%d", argN))
		args = append(args, *filter.MinAmount)
		argN++
	}
	if filter.MaxAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'value')::numeric, 0) <= $%d", argN))
		args = append(args, *filter.MaxAmount)
		argN++
	}

	if filter.DateFrom != nil {
		where = append(where, fmt.Sprintf("(metadata->>'date')::timestamptz >= $%d", argN))
		args = append(args, *filter.DateFrom)
		argN++
	}
	if filter.DateTo != nil {
		where = append(where, fmt.Sprintf("(metadata->>'date')::timestamptz <= $%d", argN))
		args = append(args, *filter.DateTo)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.transactions "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.transactions %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*transaction.Transaction, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t transaction.Transaction
		if err := json.Unmarshal(metadata, &t); err != nil {
			return nil, 0, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// PartnershipShare reuses historical commission mirrors, including rows created
// before deal_id was stored, so a signed amendment cannot accrue the same debt.
func (r *transactionPostgresRepository) PartnershipShare(ctx context.Context, tenantID, dealID, contractID string) (*transaction.Transaction, error) {
	rows, err := r.db.Query(ctx, `SELECT t.metadata FROM financial.transactions t
 WHERE t.tenant_id=$1 AND t.deleted_at IS NULL AND t.metadata->>'category'='Comissão Parceria'
 AND (t.metadata->>'deal_id'=$2 OR t.metadata->>'contract_id'=$3 OR t.metadata->>'contract_id' IN
 (SELECT c.id::text FROM deal.contracts c WHERE c.metadata->>'deal_id'=$2)) LIMIT 2`, tenantID, dealID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found *transaction.Transaction
	for rows.Next() {
		if found != nil {
			return nil, fmt.Errorf("comissão de parceria duplicada: reconcilie o histórico antes de provisionar")
		}
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		found = &transaction.Transaction{}
		if err := json.Unmarshal(raw, found); err != nil {
			return nil, err
		}
	}
	return found, rows.Err()
}

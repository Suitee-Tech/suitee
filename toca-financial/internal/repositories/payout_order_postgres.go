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
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
)

type payoutOrderPostgresRepository struct {
	db *database.PostgresDB
}

const getActivePayoutOrderBySourceEventQuery = `SELECT metadata, updated_at
		 FROM financial.payout_orders
		 WHERE tenant_id = $1
		   AND (metadata->>'source_event_id') = $2
		   AND deleted_at IS NULL
		   AND COALESCE(metadata->>'status', '') <> 'CANCELLED'
		 ORDER BY created_at DESC
		 LIMIT 1`

var _ payout_order.TransitionRepository = (*payoutOrderPostgresRepository)(nil)
var _ payout_order.TransitionTransactionRepository = (*payoutOrderPostgresRepository)(nil)
var _ payout_order.RoyaltyTransitionTransactionRepository = (*payoutOrderPostgresRepository)(nil)

func NewPayoutOrderPostgresRepository(db *database.PostgresDB) payout_order.Repository {
	return &payoutOrderPostgresRepository{db: db}
}

func decodeVersionedPayoutOrder(metadata []byte, persistedUpdatedAt time.Time) (*payout_order.PayoutOrder, error) {
	var order payout_order.PayoutOrder
	if err := json.Unmarshal(metadata, &order); err != nil {
		return nil, err
	}
	order.UpdatedAt = persistedUpdatedAt
	return &order, nil
}

func (r *payoutOrderPostgresRepository) Create(ctx context.Context, order *payout_order.PayoutOrder) error {
	order.CreatedAt = time.Now()
	metadata, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("encode payout order metadata: %w", err)
	}
	if order.SourceEventID != "" {
		query := `INSERT INTO financial.payout_orders (id, tenant_id, metadata) VALUES ($1, $2, $3)
		          ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
		          WHERE deleted_at IS NULL
		            AND (metadata->>'source_event_id') IS NOT NULL
		            AND COALESCE((metadata->>'status'), '') <> 'CANCELLED'
		          DO NOTHING
		          RETURNING created_at`
		row := r.db.QueryRow(ctx, query, order.ID, order.TenantID, string(metadata))
		if err := row.Scan(&order.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return payout_order.ErrPayoutOrderAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `INSERT INTO financial.payout_orders (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, order.ID, order.TenantID, string(metadata)).Scan(&order.CreatedAt)
}

func (r *payoutOrderPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx,
		"SELECT metadata, updated_at FROM financial.payout_orders WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("payout_order not found: %w", err)
	}
	return decodeVersionedPayoutOrder(metadata, persistedUpdatedAt)
}

func (r *payoutOrderPostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*payout_order.PayoutOrder, error) {
	if tenantID == "" || sourceEventID == "" {
		return nil, nil
	}
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(
		ctx,
		getActivePayoutOrderBySourceEventQuery,
		tenantID,
		sourceEventID,
	).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("payout_order lookup by source_event_id failed: %w", err)
	}
	return decodeVersionedPayoutOrder(metadata, persistedUpdatedAt)
}

func (r *payoutOrderPostgresRepository) List(ctx context.Context, tenantID string, filter payout_order.ListFilter) ([]*payout_order.PayoutOrder, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	whereSQL, args := buildPayoutOrderWhere(tenantID, filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.payout_orders "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.payout_orders %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*payout_order.PayoutOrder, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var p payout_order.PayoutOrder
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, 0, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// buildPayoutOrderWhere constrói a cláusula WHERE e os argumentos
// parametrizados com base no ListFilter. Reusada por List e Totals
// para garantir que os totais cubram exatamente os mesmos registros
// que a listagem paginada (FIN-COM-13).
func buildPayoutOrderWhere(tenantID string, filter payout_order.ListFilter) (string, []any) {
	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	addEq := func(jsonField, value string) {
		if value == "" {
			return
		}
		where = append(where, fmt.Sprintf("(metadata->>'%s') = $%d", jsonField, argN))
		args = append(args, value)
		argN++
	}
	addEq("status", filter.Status)
	addEq("source_type", filter.SourceType)
	addEq("source_id", filter.SourceID)
	addEq("contract_id", filter.ContractID)
	addEq("deal_id", filter.DealID)
	addEq("recipient_type", filter.RecipientType)
	addEq("recipient_id", filter.RecipientID)
	addEq("group_id", filter.GroupID)

	return "WHERE " + strings.Join(where, " AND "), args
}

// Totals retorna totais agregados (count, soma de amount_cents) para
// todos os payout_orders que batem com o filtro, sem LIMIT/OFFSET.
// Os totais não dependem da paginação — cobrem o conjunto completo
// de registros (FIN-COM-13).
func (r *payoutOrderPostgresRepository) Totals(ctx context.Context, tenantID string, filter payout_order.ListFilter) (*payout_order.PayoutOrderTotals, error) {
	whereSQL, args := buildPayoutOrderWhere(tenantID, filter)

	query := fmt.Sprintf(`
		SELECT
			COALESCE((metadata->>'status'), 'UNKNOWN'),
			COUNT(1),
			COALESCE(SUM((metadata->>'amount_cents')::bigint), 0)
		FROM financial.payout_orders %s
		GROUP BY (metadata->>'status')`, whereSQL)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	totals := &payout_order.PayoutOrderTotals{
		ByStatus: make(map[payout_order.Status]payout_order.StatusBreakdown),
	}
	for rows.Next() {
		var status string
		var count, amountCents int64
		if err := rows.Scan(&status, &count, &amountCents); err != nil {
			return nil, err
		}
		totals.TotalCount += count
		totals.TotalAmountCents += amountCents
		totals.ByStatus[payout_order.Status(status)] = payout_order.StatusBreakdown{
			Count:       count,
			AmountCents: amountCents,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return totals, nil
}

func (r *payoutOrderPostgresRepository) ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*payout_order.PayoutOrder, error) {
	if len(ids) == 0 {
		return []*payout_order.PayoutOrder{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.payout_orders
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND id::text = ANY($2)
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*payout_order.PayoutOrder, 0, len(ids))
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, err
		}
		p, err := decodeVersionedPayoutOrder(metadata, persistedUpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *payoutOrderPostgresRepository) Update(ctx context.Context, order *payout_order.PayoutOrder) error {
	order.UpdatedAt = time.Now()
	metadata, err := json.Marshal(order)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(ctx,
		"UPDATE financial.payout_orders SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL",
		string(metadata), order.ID, order.TenantID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return payout_order.ErrPayoutOrderNotFound
	}
	return nil
}

// UpdateIfStatus atomically persists a domain transition only when both the
// status and row revision observed by the service are still current. The
// revision guard also prevents approval from overwriting a same-status amount
// change made while synchronizing a Commission deduction.
func (r *payoutOrderPostgresRepository) UpdateIfStatus(ctx context.Context, order *payout_order.PayoutOrder, expected payout_order.Status, expectedUpdatedAt time.Time) error {
	order.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(order)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.payout_orders
		SET metadata = $1, updated_at = $5
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND (metadata->>'status') = $4
		  AND updated_at = $6
	`, string(metadata), order.ID, order.TenantID, string(expected), order.UpdatedAt, expectedUpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return payout_order.ErrConcurrentTransition
	}
	return nil
}

// RunTransitionTransaction makes the payout CAS and linked Commission
// lifecycle write commit or roll back together. Repository calls downstream
// use database.WithTx-aware Query/Exec methods.
func (r *payoutOrderPostgresRepository) RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	if database.TxFromCtx(ctx) != nil {
		return fn(ctx)
	}
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		return fn(database.WithTx(ctx, tx))
	})
}

// RunRoyaltyTransitionTransaction crosses the payer/payee RLS boundary on the
// verified service-role pool. PayoutOrderService has already resolved the
// payout through the caller's tenant-scoped GetByID; both rows are then
// protected again by their tenant/id/revision CAS predicates here.
func (r *payoutOrderPostgresRepository) RunRoyaltyTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	return r.db.RunInServiceContext(ctx, fn)
}

func (r *payoutOrderPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.Exec(ctx,
		"UPDATE financial.payout_orders SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	)
	return err
}

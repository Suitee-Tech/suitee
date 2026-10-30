package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
)

// O arquivo repository.go para commission não foi lido e não parece ter interface explícita clara.
// Vou criar a struct baseada nas conventions do mongodb repository do projeto

type commissionPostgresRepository struct {
	db *database.PostgresDB
}

func NewCommissionPostgresRepository(db *database.PostgresDB) *commissionPostgresRepository {
	return &commissionPostgresRepository{db: db}
}

func decodeVersionedCommission(metadata []byte, persistedUpdatedAt time.Time) (*commission.Commission, error) {
	var c commission.Commission
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	c.UpdatedAt = persistedUpdatedAt
	return &c, nil
}

func (r *commissionPostgresRepository) Create(ctx context.Context, c *commission.Commission) error {
	if c == nil {
		return errors.New("commission is required")
	}
	c.CreatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal commission for create: %w", err)
	}
	if c.SourceEventID != "" {
		query := `INSERT INTO financial.commissions (id, tenant_id, metadata) VALUES ($1, $2, $3)
		          ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
		          WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
		          DO NOTHING
		          RETURNING created_at`
		row := r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata))
		if err := row.Scan(&c.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return commission.ErrCommissionAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `INSERT INTO financial.commissions (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata)).Scan(&c.CreatedAt)
}

func (r *commissionPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*commission.Commission, error) {
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx, "SELECT metadata, updated_at FROM financial.commissions WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("commission not found: %w", err)
	}
	c, err := decodeVersionedCommission(metadata, persistedUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("unmarshal commission %s: %w", id, err)
	}
	return c, nil
}

func (r *commissionPostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*commission.Commission, error) {
	if tenantID == "" || sourceEventID == "" {
		return nil, nil
	}
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(
		ctx,
		"SELECT metadata, updated_at FROM financial.commissions WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL LIMIT 1",
		tenantID,
		sourceEventID,
	).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("commission lookup by source_event_id failed: %w", err)
	}
	c, err := decodeVersionedCommission(metadata, persistedUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("unmarshal commission for source_event_id %s: %w", sourceEventID, err)
	}
	return c, nil
}

// GetByLedgerTransactionID resolves the explicit one-to-one link created by a
// manual posting. More than one live Commission for the same ledger row is a
// consistency violation, never an arbitrary LIMIT 1 choice.
func (r *commissionPostgresRepository) GetByLedgerTransactionID(ctx context.Context, tenantID, ledgerTransactionID string) (*commission.Commission, error) {
	if tenantID == "" || ledgerTransactionID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.commissions
		WHERE tenant_id = $1
		  AND (metadata->>'ledger_transaction_id') = $2
		  AND deleted_at IS NULL
		ORDER BY id
		LIMIT 2
	`, tenantID, ledgerTransactionID)
	if err != nil {
		return nil, fmt.Errorf("commission lookup by ledger_transaction_id failed: %w", err)
	}
	defer rows.Close()

	var found *commission.Commission
	for rows.Next() {
		if found != nil {
			return nil, fmt.Errorf("multiple commissions reference ledger transaction %s", ledgerTransactionID)
		}
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, fmt.Errorf("scan commission for ledger_transaction_id %s: %w", ledgerTransactionID, err)
		}
		found, err = decodeVersionedCommission(metadata, persistedUpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("unmarshal commission for ledger_transaction_id %s: %w", ledgerTransactionID, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commissions for ledger_transaction_id %s: %w", ledgerTransactionID, err)
	}
	return found, nil
}

func (r *commissionPostgresRepository) Update(ctx context.Context, c *commission.Commission) error {
	if c == nil {
		return errors.New("commission is required")
	}
	c.UpdatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal commission %s for update: %w", c.ID, err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.commissions SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), c.ID, c.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("commission %s not found for update", c.ID)
	}
	return nil
}

// UpdateIfStatus protects standalone and payout-driven lifecycle transitions
// from overwriting a newer status or monetary revision of the JSON aggregate.
func (r *commissionPostgresRepository) UpdateIfStatus(ctx context.Context, c *commission.Commission, expected commission.Status, expectedUpdatedAt time.Time) error {
	if c == nil {
		return errors.New("commission is required")
	}
	c.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal commission %s for transition: %w", c.ID, err)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.commissions
		SET metadata = $1, updated_at = $5
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND (metadata->>'status') = $4
		  AND updated_at = $6
	`, string(metadata), c.ID, c.TenantID, string(expected), c.UpdatedAt, expectedUpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return commission.ErrConcurrentTransition
	}
	return nil
}

func sameDeductionRecord(left, right commission.Deduction) bool {
	return left.Kind == right.Kind && left.Description == right.Description && left.Amount == right.Amount
}

// mutateDeductionAtomic serializes the complete deduction read/mutate/write
// cycle with SELECT ... FOR UPDATE. Concurrent additions therefore observe the
// latest committed deduction list instead of replacing one another's JSON.
func (r *commissionPostgresRepository) mutateDeductionAtomic(
	ctx context.Context,
	tenantID, commissionID string,
	mutate func(*commission.Commission) (bool, error),
) (*commission.Commission, float64, bool, error) {
	var updated *commission.Commission
	var previousNet float64
	var changed bool

	persist := func(txCtx context.Context) error {
		var metadata []byte
		err := r.db.QueryRow(txCtx, `
			SELECT metadata
			FROM financial.commissions
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
			FOR UPDATE
		`, commissionID, tenantID).Scan(&metadata)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("commission not found: %w", err)
			}
			return err
		}

		var current commission.Commission
		if err := json.Unmarshal(metadata, &current); err != nil {
			return err
		}
		current.ID = commissionID
		current.TenantID = tenantID
		previousNet = current.NetAmount
		changed, err = mutate(&current)
		if err != nil {
			return err
		}
		if !changed {
			updated = &current
			return nil
		}
		if err := r.persistLockedCommissionAndPayoutNet(txCtx, &current); err != nil {
			return err
		}
		updated = &current
		return nil
	}

	var err error
	if database.TxFromCtx(ctx) != nil {
		err = persist(ctx)
	} else {
		err = r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return persist(database.WithTx(ctx, tx))
		})
	}
	if err != nil {
		return nil, 0, false, err
	}
	return updated, previousNet, changed, nil
}

// persistLockedCommissionAndPayoutNet expects the Commission row to already
// be locked. When linked, it also locks and rewrites the payout aggregate so
// amount_cents and updated_at have the same typed JSON representation as the
// Go domain object (avoiding PostgreSQL to_jsonb(timestamp) ambiguity).
func (r *commissionPostgresRepository) persistLockedCommissionAndPayoutNet(ctx context.Context, c *commission.Commission) error {
	if c == nil {
		return errors.New("commission is required")
	}

	var linkedPayout *payout_order.PayoutOrder
	if c.PayoutOrderID != "" {
		netCents := int64(math.Round(c.NetAmount * 100))
		if netCents <= 0 {
			return commission.ErrLinkedPayoutAmountNotPayable
		}
		var payoutMetadata []byte
		err := r.db.QueryRow(ctx, `
			SELECT metadata
			FROM financial.payout_orders
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
			FOR UPDATE
		`, c.PayoutOrderID, c.TenantID).Scan(&payoutMetadata)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return commission.ErrLinkedPayoutNotAdjustable
			}
			return err
		}
		var payout payout_order.PayoutOrder
		if err := json.Unmarshal(payoutMetadata, &payout); err != nil {
			return err
		}
		payout.ID = c.PayoutOrderID
		payout.TenantID = c.TenantID
		if payout.Status != payout_order.StatusDraft && payout.Status != payout_order.StatusPendingReview {
			return commission.ErrLinkedPayoutNotAdjustable
		}
		payout.AmountCents = netCents
		linkedPayout = &payout
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	c.UpdatedAt = now
	commissionMetadata, err := json.Marshal(c)
	if err != nil {
		return err
	}
	commissionResult, err := r.db.Exec(ctx, `
		UPDATE financial.commissions
		SET metadata = $1, updated_at = $2
		WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL
	`, string(commissionMetadata), now, c.ID, c.TenantID)
	if err != nil {
		return err
	}
	commissionRows, err := commissionResult.RowsAffected()
	if err != nil {
		return err
	}
	if commissionRows != 1 {
		return fmt.Errorf("commission %s not found for deduction persistence", c.ID)
	}

	if linkedPayout == nil {
		return nil
	}
	linkedPayout.UpdatedAt = now
	payoutMetadata, err := json.Marshal(linkedPayout)
	if err != nil {
		return err
	}
	payoutResult, err := r.db.Exec(ctx, `
		UPDATE financial.payout_orders
		SET metadata = $1, updated_at = $2
		WHERE id = $3
		  AND tenant_id = $4
		  AND deleted_at IS NULL
		  AND (metadata->>'status') IN ($5, $6)
	`, string(payoutMetadata), now, linkedPayout.ID, linkedPayout.TenantID,
		string(payout_order.StatusDraft), string(payout_order.StatusPendingReview))
	if err != nil {
		return err
	}
	payoutRows, err := payoutResult.RowsAffected()
	if err != nil {
		return err
	}
	if payoutRows != 1 {
		return commission.ErrLinkedPayoutNotAdjustable
	}
	return nil
}

func (r *commissionPostgresRepository) AddDeductionAtomic(ctx context.Context, tenantID, commissionID string, d commission.Deduction) (*commission.Commission, float64, bool, error) {
	return r.mutateDeductionAtomic(ctx, tenantID, commissionID, func(c *commission.Commission) (bool, error) {
		if existing, ok := c.DeductionByID(d.ID); ok {
			if !sameDeductionRecord(existing, d) {
				return false, commission.ErrDeductionConflict
			}
			return false, nil
		}
		for i, existing := range c.Deductions {
			if existing.ID == "" && sameDeductionRecord(existing, d) {
				c.Deductions[i].ID = d.ID
				return true, nil
			}
		}
		if err := c.AddDeduction(d); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (r *commissionPostgresRepository) RemoveDeductionAtomic(ctx context.Context, tenantID, commissionID, deductionID string) (*commission.Commission, float64, bool, error) {
	return r.mutateDeductionAtomic(ctx, tenantID, commissionID, func(c *commission.Commission) (bool, error) {
		return c.RemoveDeductionByID(deductionID)
	})
}

func (r *commissionPostgresRepository) List(ctx context.Context, tenantID string, limit, offset int64) ([]*commission.Commission, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM financial.commissions
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.commissions
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*commission.Commission, 0)
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, 0, err
		}
		c, err := decodeVersionedCommission(metadata, persistedUpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *commissionPostgresRepository) UpdateStatus(ctx context.Context, tenantID, id string, status commission.Status) error {
	_, err := r.db.Exec(ctx, "UPDATE financial.commissions SET metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text)) WHERE id = $2 AND tenant_id = $3", status, id, tenantID)
	return err
}

func (r *commissionPostgresRepository) UpdateStatusByDealID(ctx context.Context, tenantID, dealID string, status commission.Status) error {
	_, err := r.db.Exec(ctx, "UPDATE financial.commissions SET metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text)) WHERE metadata->>'deal_id' = $2 AND tenant_id = $3", status, dealID, tenantID)
	return err
}

// ListByDealID devolve todas as commissions (independente do status, exceto
// soft-deletadas) atadas ao deal. Usado por (a) GET /commissions/by-deal/:id
// e (b) o royalty net-base lookup em event_handler.royaltyBaseAmount.
func (r *commissionPostgresRepository) ListByDealID(ctx context.Context, tenantID, dealID string) ([]*commission.Commission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.commissions
		WHERE tenant_id = $1
		  AND metadata->>'deal_id' = $2
		  AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*commission.Commission, 0)
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, err
		}
		c, err := decodeVersionedCommission(metadata, persistedUpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListByBrokerID alimenta a aba "Minhas comissões" do portal do corretor +
// a tela admin /comissoes filtrada por broker. Paginação fica a cargo do
// service quando necessária — broker individual raramente passa de 100.
func (r *commissionPostgresRepository) ListByBrokerID(ctx context.Context, tenantID, brokerID string, limit, offset int64) ([]*commission.Commission, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM financial.commissions
		WHERE tenant_id = $1 AND metadata->>'broker_id' = $2 AND deleted_at IS NULL
	`, tenantID, brokerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.commissions
		WHERE tenant_id = $1 AND metadata->>'broker_id' = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, tenantID, brokerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*commission.Commission, 0)
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, 0, err
		}
		c, err := decodeVersionedCommission(metadata, persistedUpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *commissionPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	_, err := r.db.Exec(ctx,
		"UPDATE financial.commissions SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID)
	return err
}

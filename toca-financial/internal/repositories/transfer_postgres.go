package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type transferPostgresRepository struct {
	db *database.PostgresDB
}

func NewTransferPostgresRepository(db *database.PostgresDB) transfer.RepositoryInterface {
	return &transferPostgresRepository{db: db}
}

func (r *transferPostgresRepository) Create(ctx context.Context, t *transfer.OwnerTransfer) error {
	t.CreatedAt = time.Now()
	metadata, _ := json.Marshal(t)
	query := `INSERT INTO financial.transfers (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
		RETURNING created_at`
	if err := r.db.QueryRow(ctx, query, t.ID, t.TenantID, string(metadata)).Scan(&t.CreatedAt); err != nil {
		// DO NOTHING avoids aborting the surrounding payment transaction. The
		// service can safely re-read whichever financial identity won the race.
		if errors.Is(err, sql.ErrNoRows) {
			return transfer.ErrTransferAlreadyExists
		}
		return err
	}
	return nil
}

// GetBySourceChargeID resolves the one transfer released by a canonical paid
// charge. It is tenant-scoped and ignores soft-deleted rows.
func (r *transferPostgresRepository) GetBySourceChargeID(ctx context.Context, tenantID, sourceChargeID string) (*transfer.OwnerTransfer, error) {
	if strings.TrimSpace(sourceChargeID) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'source_charge_id') = $2
		LIMIT 1
	`, tenantID, sourceChargeID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *transferPostgresRepository) GetBySourceTransactionID(ctx context.Context, tenantID, sourceTransactionID string) (*transfer.OwnerTransfer, error) {
	if strings.TrimSpace(sourceTransactionID) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'source_transaction_id') = $2
		LIMIT 1
	`, tenantID, sourceTransactionID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// GetLegacyByContractMonth finds only rows created before either canonical
// charge or manual-receipt identity was persisted. It is used once to
// claim/backfill an existing monthly transfer during rollout.
func (r *transferPostgresRepository) GetLegacyByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'contract_id') = $2
		  AND (metadata->>'reference_month') = $3
		  AND COALESCE(metadata->>'source_charge_id', '') = ''
		  AND COALESCE(metadata->>'source_transaction_id', '') = ''
		LIMIT 1
		FOR UPDATE
	`, tenantID, contractID, referenceMonth).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ClaimLegacyForCharge atomically assigns an unclaimed legacy monthly row to
// a charge while persisting any safe PENDING reconciliation prepared by the
// service. A false result means another payment claimed the row first.
func (r *transferPostgresRepository) ClaimLegacyForCharge(ctx context.Context, item *transfer.OwnerTransfer) (bool, error) {
	if item == nil || strings.TrimSpace(item.SourceChargeID) == "" {
		return false, nil
	}
	expectedVersion := item.UpdatedAt
	item.UpdatedAt = time.Now()
	metadata, err := json.Marshal(item)
	if err != nil {
		return false, err
	}
	// Versão esperada como string UTC RFC3339 — evita ambiguidade de encoding
	// time.Time -> driver em `timestamptz = $param` (mesma correção da charge).
	expectedStr := ""
	if !expectedVersion.IsZero() {
		expectedStr = expectedVersion.UTC().Format(time.RFC3339Nano)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.transfers
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND COALESCE(metadata->>'source_charge_id', '') = ''
		  AND COALESCE(metadata->>'source_transaction_id', '') = ''
		  AND (
			NULLIF(metadata->>'updated_at', '')::timestamptz = NULLIF($4, '')::timestamptz
			OR ($5 AND NOT (metadata ? 'updated_at'))
		  )
	`, string(metadata), item.ID, item.TenantID, expectedStr, expectedVersion.IsZero())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// ApplyDeductionIdempotent assigns a reference exactly once within a tenant.
// New references are serialized onto the oldest PENDING transfer for the
// contract/month. The immutable claim and transfer mutation share one SQL
// transaction, so retries keep returning the same canonical transfer even if
// another installment exists in the same month or the chosen transfer later
// advances in its lifecycle.
func (r *transferPostgresRepository) ApplyDeductionIdempotent(
	ctx context.Context,
	tenantID, contractID, referenceMonth string,
	detail transfer.Deduction,
) (*transfer.OwnerTransfer, bool, error) {
	var (
		canonical *transfer.OwnerTransfer
		applied   bool
	)
	persist := func(txCtx context.Context) error {
		existing, err := r.getTransferByDeductionClaim(txCtx, tenantID, detail)
		if err != nil {
			return err
		}
		if existing != nil {
			if err := validateDeductionTarget(existing, contractID, referenceMonth, detail); err != nil {
				return err
			}
			canonical = existing
			return nil
		}

		// Backfill retries of deductions written before the claim table existed.
		existing, err = r.getTransferContainingDeductionReference(txCtx, tenantID, detail.ReferenceID)
		if err != nil {
			return err
		}
		if existing != nil {
			if err := validateDeductionTarget(existing, contractID, referenceMonth, detail); err != nil {
				return err
			}
			claimed, err := r.claimDeductionReference(txCtx, existing, detail)
			if err != nil {
				return err
			}
			if !claimed {
				existing, err = r.getTransferByDeductionClaim(txCtx, tenantID, detail)
				if err != nil {
					return err
				}
				if existing == nil {
					return fmt.Errorf("deduction reference %s was claimed but its transfer was not found", detail.ReferenceID)
				}
				if err := validateDeductionTarget(existing, contractID, referenceMonth, detail); err != nil {
					return err
				}
			}
			canonical = existing
			return nil
		}

		candidate, err := r.getPendingTransferForDeduction(txCtx, tenantID, contractID, referenceMonth)
		if err != nil {
			return err
		}
		if candidate == nil {
			return fmt.Errorf("pending owner transfer not found for contract %s month %s", contractID, referenceMonth)
		}
		// A waiter may receive the row version committed by the transaction that
		// won this same reference immediately before it acquired the lock.
		if hasDeductionReference(candidate, detail.ReferenceID) {
			if err := validateDeductionTarget(candidate, contractID, referenceMonth, detail); err != nil {
				return err
			}
			claimed, err := r.claimDeductionReference(txCtx, candidate, detail)
			if err != nil {
				return err
			}
			if !claimed {
				candidate, err = r.getTransferByDeductionClaim(txCtx, tenantID, detail)
				if err != nil {
					return err
				}
				if err := validateDeductionTarget(candidate, contractID, referenceMonth, detail); err != nil {
					return err
				}
			}
			canonical = candidate
			return nil
		}

		claimed, err := r.claimDeductionReference(txCtx, candidate, detail)
		if err != nil {
			return err
		}
		if !claimed {
			canonical, err = r.getTransferByDeductionClaim(txCtx, tenantID, detail)
			if err != nil {
				return err
			}
			if canonical == nil {
				return fmt.Errorf("deduction reference %s was claimed but its transfer was not found", detail.ReferenceID)
			}
			return validateDeductionTarget(canonical, contractID, referenceMonth, detail)
		}

		applyTransferDeduction(candidate, detail)
		if err := r.Update(txCtx, candidate); err != nil {
			return err
		}
		canonical = candidate
		applied = true
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
		return nil, false, err
	}
	return canonical, applied, nil
}

func (r *transferPostgresRepository) getTransferByDeductionClaim(ctx context.Context, tenantID string, detail transfer.Deduction) (*transfer.OwnerTransfer, error) {
	var (
		metadata           []byte
		claimedType        string
		claimedAmountCents int64
	)
	err := r.db.QueryRow(ctx, `
		SELECT t.metadata, c.deduction_type, c.amount_cents
		FROM financial.transfer_deduction_claims c
		JOIN financial.transfers t
		  ON t.id = c.transfer_id
		 AND t.tenant_id = c.tenant_id
		WHERE c.tenant_id = $1
		  AND c.reference_id = $2
		LIMIT 1
	`, tenantID, detail.ReferenceID).Scan(&metadata, &claimedType, &claimedAmountCents)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if claimedType != detail.Type || claimedAmountCents != deductionAmountCents(detail.Amount) {
		return nil, fmt.Errorf("deduction reference %s was reused with different type or amount", detail.ReferenceID)
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *transferPostgresRepository) getTransferContainingDeductionReference(ctx context.Context, tenantID, referenceID string) (*transfer.OwnerTransfer, error) {
	contains, err := json.Marshal([]map[string]string{{"reference_id": referenceID}})
	if err != nil {
		return nil, err
	}
	var metadata []byte
	err = r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND COALESCE(metadata->'deduction_details', '[]'::jsonb) @> $2::jsonb
		ORDER BY created_at ASC, id ASC
		LIMIT 1
		FOR UPDATE
	`, tenantID, string(contains)).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *transferPostgresRepository) getPendingTransferForDeduction(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'contract_id') = $2
		  AND (metadata->>'reference_month') = $3
		  AND (metadata->>'status') = $4
		ORDER BY created_at ASC, id ASC
		LIMIT 1
		FOR UPDATE
	`, tenantID, contractID, referenceMonth, transfer.StatusPending).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var item transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *transferPostgresRepository) claimDeductionReference(ctx context.Context, item *transfer.OwnerTransfer, detail transfer.Deduction) (bool, error) {
	var transferID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO financial.transfer_deduction_claims
			(tenant_id, reference_id, contract_id, reference_month, transfer_id, deduction_type, amount_cents)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, reference_id) DO NOTHING
		RETURNING transfer_id
	`, item.TenantID, detail.ReferenceID, item.ContractID, item.ReferenceMonth, item.ID, detail.Type, deductionAmountCents(detail.Amount)).Scan(&transferID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return transferID == item.ID, nil
}

func validateDeductionTarget(item *transfer.OwnerTransfer, contractID, referenceMonth string, detail transfer.Deduction) error {
	if item == nil {
		return fmt.Errorf("deduction reference %s has no canonical transfer", detail.ReferenceID)
	}
	if item.ContractID != contractID || item.ReferenceMonth != referenceMonth {
		return fmt.Errorf("deduction reference %s already belongs to contract %s month %s", detail.ReferenceID, item.ContractID, item.ReferenceMonth)
	}
	for _, existing := range item.DeductionDetails {
		if existing.ReferenceID != detail.ReferenceID {
			continue
		}
		if existing.Type != detail.Type || deductionAmountCents(existing.Amount) != deductionAmountCents(detail.Amount) {
			return fmt.Errorf("deduction reference %s was reused with different type or amount", detail.ReferenceID)
		}
		return nil
	}
	return fmt.Errorf("deduction reference %s claim is missing from canonical transfer %s", detail.ReferenceID, item.ID)
}

func deductionAmountCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func hasDeductionReference(item *transfer.OwnerTransfer, referenceID string) bool {
	for _, detail := range item.DeductionDetails {
		if detail.ReferenceID == referenceID {
			return true
		}
	}
	return false
}

func applyTransferDeduction(item *transfer.OwnerTransfer, detail transfer.Deduction) {
	if detail.Type == "MAINTENANCE" {
		item.MaintenanceDeductions += detail.Amount
	} else {
		item.OtherDeductions += detail.Amount
	}
	item.DeductionDetails = append(item.DeductionDetails, detail)
	item.Calculate()
	// Uma dedução pode consumir todo o valor a repassar (imposto retido,
	// recuperação de manutenção, conta paga em nome do proprietário). Sem esta
	// transição o repasse fica zerado e pendente para sempre, sem baixa possível.
	item.SyncNoPayableBalanceState(time.Now())
}

// GetByContractMonth devolve a (única) row para (tenant, contract,
// reference_month), ou nil quando ausente.
func (r *transferPostgresRepository) GetByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.transfers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'contract_id') = $2
		  AND (metadata->>'reference_month') = $3
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, tenantID, contractID, referenceMonth).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var t transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *transferPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, "SELECT metadata FROM financial.transfers WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("transfer not found: %w", err)
	}
	var t transfer.OwnerTransfer
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *transferPostgresRepository) Update(ctx context.Context, t *transfer.OwnerTransfer) error {
	t.UpdatedAt = time.Now()
	metadata, err := json.Marshal(t)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.transfers SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), t.ID, t.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("transfer not found")
	}
	return nil
}

// UpdateIfVersion persists an optimistic lifecycle mutation. The revision is
// the metadata.updated_at snapshot read with the aggregate, which also changes
// whenever ApplyDeduction commits. A stale Approve/Process therefore fails
// instead of replacing the entire JSON document and losing the deduction.
func (r *transferPostgresRepository) UpdateIfVersion(ctx context.Context, item *transfer.OwnerTransfer, expected time.Time) (bool, error) {
	item.UpdatedAt = time.Now()
	metadata, err := json.Marshal(item)
	if err != nil {
		return false, err
	}
	expectedStr := ""
	if !expected.IsZero() {
		expectedStr = expected.UTC().Format(time.RFC3339Nano)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.transfers
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND (
			NULLIF(metadata->>'updated_at', '')::timestamptz = NULLIF($4, '')::timestamptz
			OR ($5 AND NOT (metadata ? 'updated_at'))
		  )
	`, string(metadata), item.ID, item.TenantID, expectedStr, expected.IsZero())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (r *transferPostgresRepository) UpdateStatus(ctx context.Context, tenantID, id string, status transfer.Status) error {
	_, err := r.db.Exec(ctx, "UPDATE financial.transfers SET metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text)) WHERE id = $2 AND tenant_id = $3", status, id, tenantID)
	return err
}

func (r *transferPostgresRepository) List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error) {
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
	if ownerID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'owner_id') = $%d", argN))
		args = append(args, ownerID)
		argN++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}
	if referenceMonth != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, referenceMonth)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.transfers "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.transfers %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*transfer.OwnerTransfer, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t transfer.OwnerTransfer
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

// ListByOwnerLeadIDs lista repasses cujo owner_id está na lista — entry
// point para o portal cliente listar "Meus Repasses". A resolução
// account_id → leadIDs é responsabilidade do caller (CrossTenantRepository
// no PortalTransferHandler.ListMine).
//
// Lista vazia é tratada como "sem leads, sem resultados" — devolve
// imediatamente para evitar consulta com IN () que dependendo do driver
// vira erro ou um SELECT WHERE FALSE silenciosamente.
func (r *transferPostgresRepository) ListByOwnerLeadIDs(ctx context.Context, tenantID string, ownerLeadIDs []string, filter transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	if len(ownerLeadIDs) == 0 {
		return []*transfer.OwnerTransfer{}, 0, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Constrói IN (...) com placeholders dinâmicos. owner_id é texto no
	// JSONB metadata; cast explícito mantém o planner usando o índice
	// de expressão se existir.
	args := []any{tenantID}
	argN := 2
	placeholders := make([]string, 0, len(ownerLeadIDs))
	for _, id := range ownerLeadIDs {
		placeholders = append(placeholders, fmt.Sprintf("$%d", argN))
		args = append(args, id)
		argN++
	}
	where := []string{
		"tenant_id = $1",
		"deleted_at IS NULL",
		fmt.Sprintf("(metadata->>'owner_id') IN (%s)", strings.Join(placeholders, ", ")),
	}

	if filter.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, filter.Status)
		argN++
	}
	if filter.Year > 0 {
		// reference_month é "YYYY-MM"; filtramos pelo prefixo do ano.
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') LIKE $%d", argN))
		args = append(args, fmt.Sprintf("%04d-%%", filter.Year))
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.transfers "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		"SELECT metadata FROM financial.transfers %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d",
		whereSQL, argN, argN+1,
	)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*transfer.OwnerTransfer, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t transfer.OwnerTransfer
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

func (r *transferPostgresRepository) Search(ctx context.Context, tenantID string, filter transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
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
	owner := strings.TrimSpace(filter.OwnerID)
	if owner == "" {
		owner = strings.TrimSpace(filter.BrokerID)
	}
	if owner != "" {
		where = append(where, fmt.Sprintf("(metadata->>'owner_id') = $%d", argN))
		args = append(args, owner)
		argN++
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.ReferenceMonth); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, v)
		argN++
	}

	if filter.MinAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'net_amount')::numeric, 0) >= $%d", argN))
		args = append(args, *filter.MinAmount)
		argN++
	}
	if filter.MaxAmount != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'net_amount')::numeric, 0) <= $%d", argN))
		args = append(args, *filter.MaxAmount)
		argN++
	}

	if filter.DateFrom != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, *filter.DateFrom)
		argN++
	}
	if filter.DateTo != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, *filter.DateTo)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.transfers "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.transfers %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*transfer.OwnerTransfer, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t transfer.OwnerTransfer
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

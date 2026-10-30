package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

type journalPostgresRepository struct {
	db *database.PostgresDB
}

// NewJournalPostgresRepository constrói o repositório append-only do journal.
func NewJournalPostgresRepository(db *database.PostgresDB) journal.Repository {
	return &journalPostgresRepository{db: db}
}

func (r *journalPostgresRepository) Create(ctx context.Context, entry *journal.Entry) error {
	entry.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	entry.UpdatedAt = entry.CreatedAt
	metadata, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal journal entry: %w", err)
	}
	query := `INSERT INTO financial.journal_entries (id, tenant_id, metadata, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)
	          ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
	          WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
	          DO NOTHING
	          RETURNING created_at, updated_at`
	var scanErr error
	if tx := serviceTx(ctx); tx != nil {
		scanErr = tx.QueryRowContext(ctx, query, entry.ID, entry.TenantID, string(metadata), entry.CreatedAt, entry.UpdatedAt).
			Scan(&entry.CreatedAt, &entry.UpdatedAt)
	} else {
		scanErr = r.db.QueryServiceRow(ctx, query, entry.ID, entry.TenantID, string(metadata), entry.CreatedAt, entry.UpdatedAt).
			Scan(&entry.CreatedAt, &entry.UpdatedAt)
	}
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) || errors.Is(scanErr, pgx.ErrNoRows) {
			return journal.ErrEntryAlreadyExists
		}
		return scanErr
	}
	return nil
}

func (r *journalPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*journal.Entry, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.journal_entries WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("journal entry lookup: %w", err)
	}
	return decodeEntry(metadata)
}

func (r *journalPostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*journal.Entry, error) {
	if sourceEventID == "" || tenantID == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.journal_entries WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL LIMIT 1",
		tenantID, sourceEventID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("journal entry lookup by source_event_id failed: %w", err)
	}
	return decodeEntry(metadata)
}

func (r *journalPostgresRepository) UpdateStatus(ctx context.Context, entry *journal.Entry, expected time.Time) (bool, error) {
	entry.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(entry)
	if err != nil {
		return false, fmt.Errorf("marshal journal entry: %w", err)
	}
	// Mesmo padrão de charge/transfer/transaction: versão esperada como texto
	// UTC RFC3339 para que o parse do metadata e o parâmetro arredondem igual.
	expectedStr := ""
	if !expected.IsZero() {
		expectedStr = expected.UTC().Format(time.RFC3339Nano)
	}
	query := `
		UPDATE financial.journal_entries
		SET metadata = $1, updated_at = $2
		WHERE id = $3
		  AND tenant_id = $4
		  AND deleted_at IS NULL
		  AND (
			NULLIF(metadata->>'updated_at', '')::timestamptz = NULLIF($5, '')::timestamptz
			OR ($6 AND NOT (metadata ? 'updated_at'))
		  )
	`
	args := []any{string(metadata), entry.UpdatedAt, entry.ID, entry.TenantID, expectedStr, expected.IsZero()}
	var rows int64
	if tx := serviceTx(ctx); tx != nil {
		result, execErr := tx.ExecContext(ctx, query, args...)
		if execErr != nil {
			return false, execErr
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return false, affectedErr
		}
		rows = affected
	} else {
		tag, execErr := r.db.ExecServiceRows(ctx, query, args...)
		if execErr != nil {
			return false, execErr
		}
		rows = tag.RowsAffected()
	}
	return rows == 1, nil
}

// serviceTx devolve a transação do contexto quando ela é do pool de SERVIÇO.
//
// O ledger é append-only por decisão de schema: a migration 20260822123000
// dropou as policies de escrita e revogou INSERT/UPDATE/DELETE de
// authenticated/anon, então o pool de aplicação não consegue gravar nesta
// tabela (SQLSTATE 42501). Quem grava é o escritor privilegiado:
//
//   - transação de serviço no contexto (consumidores de evento rodam em
//     RunInServiceContext) → participa dela, mantendo ledger + comissão atômicos;
//   - fora disso (requisição de usuário, backfill) → o caller usa o pool de
//     serviço direto nos métodos abaixo.
//
// Uma transação do pool de APLICAÇÃO não serve: ela não tem permissão de
// escrita aqui, então é ignorada de propósito (o write sai pelo pool de
// serviço, fora daquela transação).
func serviceTx(ctx context.Context) *sqlx.Tx {
	if !database.ServiceTxFromCtx(ctx) {
		return nil
	}
	return database.TxFromCtx(ctx)
}

func (r *journalPostgresRepository) AccountBalance(ctx context.Context, tenantID string, account journal.Account) (int64, error) {
	var balance int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			CASE
				WHEN leg->>'direction' = 'debit' THEN COALESCE((leg->>'amount_cents')::bigint, 0)
				ELSE -COALESCE((leg->>'amount_cents')::bigint, 0)
			END
		), 0)
		FROM financial.journal_entries,
		     LATERAL jsonb_array_elements(metadata->'legs') AS leg
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (leg->>'account') = $2`,
		tenantID, string(account),
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("journal account balance: %w", err)
	}
	return balance, nil
}

// AccountBalanceByFlow agrega o saldo de uma conta por modalidade econômica
// (leg.flow = sale|rent|vazio). É a base do desdobramento do plano de contas
// em receita de compra/venda versus locação.
func (r *journalPostgresRepository) AccountBalanceByFlow(ctx context.Context, tenantID string, account journal.Account) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT COALESCE(leg->>'flow', ''),
		       COALESCE(SUM(
		           CASE
		               WHEN leg->>'direction' = 'debit' THEN COALESCE((leg->>'amount_cents')::bigint, 0)
		               ELSE -COALESCE((leg->>'amount_cents')::bigint, 0)
		           END
		       ), 0)
		FROM financial.journal_entries,
		     LATERAL jsonb_array_elements(metadata->'legs') AS leg
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (leg->>'account') = $2
		GROUP BY COALESCE(leg->>'flow', '')
	`, tenantID, string(account))
	if err != nil {
		return nil, fmt.Errorf("journal account balance by flow: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64)
	for rows.Next() {
		var flow string
		var balance int64
		if err := rows.Scan(&flow, &balance); err != nil {
			return nil, err
		}
		out[flow] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *journalPostgresRepository) List(ctx context.Context, tenantID, contractID string, limit, offset int64) ([]*journal.Entry, int64, error) {
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
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.journal_entries "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.journal_entries %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*journal.Entry, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		entry, err := decodeEntry(metadata)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func decodeEntry(metadata []byte) (*journal.Entry, error) {
	var entry journal.Entry
	if err := json.Unmarshal(metadata, &entry); err != nil {
		return nil, fmt.Errorf("decode journal entry metadata: %w", err)
	}
	return &entry, nil
}

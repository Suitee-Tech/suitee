package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
)

type inspectionRepository struct {
	db *database.PostgresDB
}

func NewInspectionRepository(db *database.PostgresDB) inspection.Repository {
	return &inspectionRepository{db: db}
}

func (r *inspectionRepository) Create(ctx context.Context, i *inspection.Inspection) error {
	if i.ID == "" {
		i.ID = uuid.NewString()
	}
	i.CreatedAt = time.Now()
	i.UpdatedAt = i.CreatedAt
	metadata, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if err := tx.QueryRowContext(ctx, `INSERT INTO ops.inspections (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`, i.ID, i.TenantID, string(metadata)).Scan(&i.CreatedAt); err != nil {
			return err
		}
		// EnqueueOutboxInTxService roteia a linha do outbox para o pool de
		// serviço (events.outbox_events é deny-by-default no pool de aplicação).
		_, err := events.EnqueueOutboxInTxService(pkgctx.WithTenantID(ctx, i.TenantID), tx, r.db, i.CalendarEvent())
		return err
	})
}

// inspectionGetByIDQuery é tenant-scoped de propósito (auditoria C8): a tabela
// é chaveada por id, mas a autorização é por tenant. A query anterior filtrava
// apenas por id, de modo que qualquer principal autenticado lia — e, via
// POST /inspections/:id/generate-laudo, sobrescrevia — a vistoria de outro
// tenant. O tenant vem sempre do chamador, nunca do registro carregado.
const inspectionGetByIDQuery = `SELECT metadata FROM ops.inspections WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`

// GetByID devolve a vistoria apenas quando ela pertence a tenantID. Sem tenant
// (ou sem id) falha fechado: retorna erro antes de tocar no banco, em vez de
// cair em uma busca sem escopo.
func (r *inspectionRepository) GetByID(ctx context.Context, tenantID, id string) (*inspection.Inspection, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("inspection not found: tenant context required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("inspection not found: id required")
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, inspectionGetByIDQuery, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("inspection not found: %w", err)
	}
	var i inspection.Inspection
	_ = json.Unmarshal(metadata, &i)
	return &i, nil
}

func (r *inspectionRepository) Update(ctx context.Context, i *inspection.Inspection) error {
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		var lockedID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM ops.inspections WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL FOR UPDATE`, i.ID, i.TenantID).Scan(&lockedID); err != nil {
			return err
		}
		i.UpdatedAt = time.Now()
		metadata, err := json.Marshal(i)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ops.inspections SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`, string(metadata), i.ID, i.TenantID); err != nil {
			return err
		}
		_, err = events.EnqueueOutboxInTxService(pkgctx.WithTenantID(ctx, i.TenantID), tx, r.db, i.CalendarEvent())
		return err
	})
}

func (r *inspectionRepository) List(ctx context.Context, f inspection.ListFilter) ([]*inspection.Inspection, int64, error) {
	limit := f.Limit
	offset := f.Offset
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{f.TenantID}
	argN := 2
	if f.PropertyID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'property_id') = $%d", argN))
		args = append(args, f.PropertyID)
		argN++
	}
	if f.ContractID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, f.ContractID)
		argN++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, f.Status)
		argN++
	}
	if f.Type != "" {
		where = append(where, fmt.Sprintf("(metadata->>'type') = $%d", argN))
		args = append(args, inspection.NormalizeType(f.Type))
		argN++
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ops.inspections "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM ops.inspections %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*inspection.Inspection, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var i inspection.Inspection
		if err := json.Unmarshal(metadata, &i); err != nil {
			return nil, 0, err
		}
		out = append(out, &i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *inspectionRepository) Delete(ctx context.Context, tenantID, id string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		// Fail-closed: sem tenant não há escrita a fazer (mesma decisão do
		// GetByID desta tabela).
		return fmt.Errorf("inspection not found: tenant context required")
	}
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		var metadata []byte
		if err := tx.QueryRowContext(ctx, `UPDATE ops.inspections SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL RETURNING metadata`, id, tenantID).Scan(&metadata); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		var i inspection.Inspection
		if err := json.Unmarshal(metadata, &i); err != nil {
			return err
		}
		i.Status = inspection.StatusCancelada
		i.UpdatedAt = time.Now()
		_, err := events.EnqueueOutboxInTxService(pkgctx.WithTenantID(ctx, i.TenantID), tx, r.db, i.CalendarEvent())
		return err
	})
}

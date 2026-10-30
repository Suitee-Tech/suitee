package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type rouletteAuditPostgresRepository struct {
	db *database.PostgresDB
}

func NewRouletteAuditPostgresRepository(db *database.PostgresDB) *rouletteAuditPostgresRepository {
	return &rouletteAuditPostgresRepository{db: db}
}

func (r *rouletteAuditPostgresRepository) Create(ctx context.Context, entry *lead.RouletteAuditEntry) error {
	if entry == nil {
		return nil
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	diffBytes, _ := json.Marshal(entry.Diff)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO crm.roulette_audit (id, tenant_id, actor_id, actor_email, action, target_id, diff, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, entry.ID, entry.TenantID, entry.ActorID, entry.ActorEmail, entry.Action, entry.TargetID, string(diffBytes), entry.CreatedAt)
	return err
}

func (r *rouletteAuditPostgresRepository) List(ctx context.Context, tenantID string, filter lead.RouletteAuditFilter) ([]*lead.RouletteAuditEntry, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := "tenant_id = $1"
	args := []interface{}{tenantID}
	addArg := func(val interface{}, predicate string) {
		args = append(args, val)
		where += " AND " + predicate + " $" + strconv.Itoa(len(args))
	}
	if filter.Action != "" {
		addArg(filter.Action, "action =")
	}
	if filter.TargetID != "" {
		addArg(filter.TargetID, "target_id =")
	}
	if filter.From != nil {
		addArg(filter.From.UTC(), "created_at >=")
	}
	if filter.To != nil {
		addArg(filter.To.UTC(), "created_at <=")
	}

	countQuery := "SELECT COUNT(1) FROM crm.roulette_audit WHERE " + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, limit, offset)

	query := `
		SELECT id::text, tenant_id::text, actor_id, COALESCE(actor_email,''), action, COALESCE(target_id,''), diff, created_at
		FROM crm.roulette_audit
		WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*lead.RouletteAuditEntry, 0)
	for rows.Next() {
		var e lead.RouletteAuditEntry
		var diffBytes []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorID, &e.ActorEmail, &e.Action, &e.TargetID, &diffBytes, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		var diff map[string]interface{}
		_ = json.Unmarshal(diffBytes, &diff)
		e.Diff = diff
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

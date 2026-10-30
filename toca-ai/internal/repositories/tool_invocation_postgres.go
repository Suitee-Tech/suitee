package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ai/internal/domain"
)

type ToolInvocationRepository interface {
	Create(ctx context.Context, inv *domain.ToolInvocation) error
}

type toolInvocationPostgresRepository struct {
	db *database.PostgresDB
}

func NewToolInvocationPostgresRepository(db *database.PostgresDB) ToolInvocationRepository {
	return &toolInvocationPostgresRepository{db: db}
}

func (r *toolInvocationPostgresRepository) Create(ctx context.Context, inv *domain.ToolInvocation) error {
	now := time.Now()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	args := inv.Args
	if args == nil {
		args = map[string]any{}
	}
	argsJSON, _ := json.Marshal(args)
	argsJSONString := string(argsJSON)

	if claims, ok := jwtClaimsFromContext(ctx); ok {
		_, err := r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			INSERT INTO ai.tool_invocations (
				id, conversation_id, tenant_id, account_id, tool_name, args, result_preview, success, has_error, duration_ms, created_at
			) SELECT $2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12 FROM __jwt
		`, claims, inv.ID, inv.ConversationID, inv.TenantID, inv.AccountID, inv.ToolName, argsJSONString, nullIfEmpty(inv.ResultPreview), inv.Success, inv.HasError, nullIfZero(inv.DurationMS), inv.CreatedAt)
		return err
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ai.tool_invocations (
			id, conversation_id, tenant_id, account_id, tool_name, args, result_preview, success, has_error, duration_ms, created_at
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11)
	`, inv.ID, inv.ConversationID, inv.TenantID, inv.AccountID, inv.ToolName, argsJSONString, nullIfEmpty(inv.ResultPreview), inv.Success, inv.HasError, nullIfZero(inv.DurationMS), inv.CreatedAt)
	return err
}

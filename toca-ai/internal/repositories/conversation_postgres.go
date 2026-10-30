package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ai/internal/domain"
)

type ConversationRepository interface {
	CreateConversation(ctx context.Context, conv *domain.Conversation) error
	ListConversations(ctx context.Context, tenantID, accountID string, limit int) ([]domain.Conversation, error)
	SoftDeleteConversation(ctx context.Context, id, tenantID, accountID string) (bool, error)
	SoftDeleteAllConversations(ctx context.Context, tenantID, accountID string) (int, error)
	CreateMessage(ctx context.Context, msg *domain.Message) error
	ListMessages(ctx context.Context, conversationID, tenantID, accountID string, limit int) ([]domain.Message, error)
}

type conversationPostgresRepository struct {
	db *database.PostgresDB
}

func NewConversationPostgresRepository(db *database.PostgresDB) ConversationRepository {
	return &conversationPostgresRepository{db: db}
}

func (r *conversationPostgresRepository) CreateConversation(ctx context.Context, conv *domain.Conversation) error {
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now

	metadata := conv.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, _ := json.Marshal(metadata)
	metadataJSONString := string(metadataJSON)

	if claims, ok := jwtClaimsFromContext(ctx); ok {
		_, err := r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			INSERT INTO ai.conversations (id, tenant_id, account_id, title, metadata, created_at, updated_at)
			SELECT $2, $3, $4, $5, $6::jsonb, $7, $8 FROM __jwt
		`, claims, conv.ID, conv.TenantID, conv.AccountID, conv.Title, metadataJSONString, conv.CreatedAt, conv.UpdatedAt)
		return err
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ai.conversations (id, tenant_id, account_id, title, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
	`, conv.ID, conv.TenantID, conv.AccountID, conv.Title, metadataJSONString, conv.CreatedAt, conv.UpdatedAt)
	return err
}

func (r *conversationPostgresRepository) ListConversations(ctx context.Context, tenantID, accountID string, limit int) ([]domain.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}

	var (
		rows *sql.Rows
		err  error
	)
	if claims, ok := jwtClaimsFromContext(ctx); ok {
		rows, err = r.db.QueryContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			SELECT id::text, tenant_id::text, account_id::text, COALESCE(title, ''), metadata, created_at, updated_at, deleted_at
			FROM ai.conversations
			WHERE tenant_id = $2 AND account_id = $3 AND deleted_at IS NULL
				AND EXISTS (SELECT 1 FROM __jwt)
			ORDER BY updated_at DESC
			LIMIT $4
		`, claims, tenantID, accountID, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id::text, tenant_id::text, account_id::text, COALESCE(title, ''), metadata, created_at, updated_at, deleted_at
			FROM ai.conversations
			WHERE tenant_id = $1 AND account_id = $2 AND deleted_at IS NULL
			ORDER BY updated_at DESC
			LIMIT $3
		`, tenantID, accountID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Conversation, 0)
	for rows.Next() {
		var c domain.Conversation
		var metadata []byte
		if err := rows.Scan(&c.ID, &c.TenantID, &c.AccountID, &c.Title, &metadata, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &c.Metadata)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *conversationPostgresRepository) SoftDeleteConversation(ctx context.Context, id, tenantID, accountID string) (bool, error) {
	var (
		res sql.Result
		err error
	)
	if claims, ok := jwtClaimsFromContext(ctx); ok {
		res, err = r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			UPDATE ai.conversations
			SET deleted_at = NOW(), updated_at = NOW()
			FROM __jwt
			WHERE id = $2 AND tenant_id = $3 AND account_id = $4 AND deleted_at IS NULL
		`, claims, id, tenantID, accountID)
	} else {
		res, err = r.db.ExecContext(ctx, `
			UPDATE ai.conversations
			SET deleted_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND account_id = $3 AND deleted_at IS NULL
		`, id, tenantID, accountID)
	}
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *conversationPostgresRepository) SoftDeleteAllConversations(ctx context.Context, tenantID, accountID string) (int, error) {
	var (
		res sql.Result
		err error
	)
	if claims, ok := jwtClaimsFromContext(ctx); ok {
		res, err = r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			UPDATE ai.conversations
			SET deleted_at = NOW(), updated_at = NOW()
			FROM __jwt
			WHERE tenant_id = $2 AND account_id = $3 AND deleted_at IS NULL
		`, claims, tenantID, accountID)
	} else {
		res, err = r.db.ExecContext(ctx, `
			UPDATE ai.conversations
			SET deleted_at = NOW(), updated_at = NOW()
			WHERE tenant_id = $1 AND account_id = $2 AND deleted_at IS NULL
		`, tenantID, accountID)
	}
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *conversationPostgresRepository) CreateMessage(ctx context.Context, msg *domain.Message) error {
	now := time.Now()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}

	var toolCallsJSON []byte
	if msg.ToolCalls != nil {
		toolCallsJSON, _ = json.Marshal(msg.ToolCalls)
	}
	var toolCallsJSONString any = nil
	if len(toolCallsJSON) > 0 {
		toolCallsJSONString = string(toolCallsJSON)
	}

	if claims, ok := jwtClaimsFromContext(ctx); ok {
		_, err := r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			INSERT INTO ai.messages (
				id, tenant_id, conversation_id, role, content, tool_calls, tool_call_id, tool_name, tokens_in, tokens_out, created_at
			) SELECT $2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12 FROM __jwt
		`, claims, msg.ID, msg.TenantID, msg.ConversationID, string(msg.Role), msg.Content, toolCallsJSONString, nullIfEmpty(msg.ToolCallID), nullIfEmpty(msg.ToolName), nullIfZero(msg.TokensIn), nullIfZero(msg.TokensOut), msg.CreatedAt)
		if err != nil {
			return err
		}

		_, _ = r.db.ExecContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			UPDATE ai.conversations SET updated_at = NOW()
			FROM __jwt
			WHERE id = $2 AND tenant_id = $3
		`, claims, msg.ConversationID, msg.TenantID)
		return nil
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ai.messages (
			id, tenant_id, conversation_id, role, content, tool_calls, tool_call_id, tool_name, tokens_in, tokens_out, created_at
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11)
	`, msg.ID, msg.TenantID, msg.ConversationID, string(msg.Role), msg.Content, toolCallsJSONString, nullIfEmpty(msg.ToolCallID), nullIfEmpty(msg.ToolName), nullIfZero(msg.TokensIn), nullIfZero(msg.TokensOut), msg.CreatedAt)
	if err != nil {
		return err
	}

	_, _ = r.db.ExecContext(ctx, `UPDATE ai.conversations SET updated_at = NOW() WHERE id = $1 AND tenant_id = $2`, msg.ConversationID, msg.TenantID)
	return nil
}

func (r *conversationPostgresRepository) ListMessages(ctx context.Context, conversationID, tenantID, accountID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 200
	}

	var (
		rows *sql.Rows
		err  error
	)
	if claims, ok := jwtClaimsFromContext(ctx); ok {
		rows, err = r.db.QueryContext(ctx, `
			WITH __jwt AS (SELECT set_config('request.jwt.claims', $1, true) AS _)
			SELECT m.id::text, m.tenant_id::text, m.conversation_id::text, m.role, m.content, m.tool_calls, COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tokens_in, 0), COALESCE(m.tokens_out, 0), m.created_at
			FROM ai.messages m
			JOIN ai.conversations c ON c.id = m.conversation_id
			WHERE m.conversation_id = $2 AND m.tenant_id = $3 AND c.account_id = $4 AND c.deleted_at IS NULL
				AND EXISTS (SELECT 1 FROM __jwt)
			ORDER BY m.created_at ASC
			LIMIT $5
		`, claims, conversationID, tenantID, accountID, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT m.id::text, m.tenant_id::text, m.conversation_id::text, m.role, m.content, m.tool_calls, COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tokens_in, 0), COALESCE(m.tokens_out, 0), m.created_at
			FROM ai.messages m
			JOIN ai.conversations c ON c.id = m.conversation_id
			WHERE m.conversation_id = $1 AND m.tenant_id = $2 AND c.account_id = $3 AND c.deleted_at IS NULL
			ORDER BY m.created_at ASC
			LIMIT $4
		`, conversationID, tenantID, accountID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Message, 0)
	for rows.Next() {
		var m domain.Message
		var role string
		var toolCalls []byte
		if err := rows.Scan(&m.ID, &m.TenantID, &m.ConversationID, &role, &m.Content, &toolCalls, &m.ToolCallID, &m.ToolName, &m.TokensIn, &m.TokensOut, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Role = domain.Role(role)
		if len(toolCalls) > 0 {
			_ = json.Unmarshal(toolCalls, &m.ToolCalls)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

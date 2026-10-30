package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
)

// ChatReminderCandidate é um par (canal, participante) com mensagem não lida
// elegível a lembrete por e-mail (beta F3 — notificacoes-email-beta.md §8).
type ChatReminderCandidate struct {
	TenantID      string
	ChannelID     string
	ChannelName   string
	MessageID     string
	SenderID      string
	SenderName    string
	Snippet       string
	SentAt        time.Time
	ParticipantID string
}

// ChatReminderRepository alimenta o ChatReminderEngine com as conversas que
// têm mensagem parada há mais de minAge (e menos de maxAge) para algum
// participante que não leu, não é o remetente e não mutou o canal.
type ChatReminderRepository interface {
	ListUnreadCandidates(ctx context.Context, minAge, maxAge time.Duration, limit int) ([]ChatReminderCandidate, error)
}

type chatReminderPostgresRepository struct {
	db *database.PostgresDB
}

func NewChatReminderPostgresRepository(db *database.PostgresDB) ChatReminderRepository {
	return &chatReminderPostgresRepository{db: db}
}

// ListUnreadCandidates consulta via service pool (o motor roda sem JWT).
//
// O join LATERAL explode participants do canal; a elegibilidade é filtrada no
// SQL: participante ≠ remetente, ∉ read_by, ∉ muted_by, mensagem dentro da
// janela [now-maxAge, now-minAge]. ORDER BY sent_at ASC + LIMIT garante lote
// limitado (motor pode ser chamado de novo no próximo tick).
func (r *chatReminderPostgresRepository) ListUnreadCandidates(ctx context.Context, minAge, maxAge time.Duration, limit int) ([]ChatReminderCandidate, error) {
	if minAge <= 0 || maxAge <= 0 || minAge >= maxAge {
		return nil, fmt.Errorf("invalid candidate window (min=%v max=%v)", minAge, maxAge)
	}
	if limit <= 0 {
		limit = 200
	}
	if r.db == nil || r.db.ServicePool == nil {
		return nil, nil
	}
	rows, err := r.db.ServicePool.Query(ctx, `
		SELECT c.tenant_id::text,
		       c.id::text,
		       COALESCE(c.metadata->>'name', ''),
		       m.id::text,
		       COALESCE(m.metadata->>'sender_id', ''),
		       COALESCE(m.metadata->>'sender_name', ''),
		       LEFT(COALESCE(m.metadata->>'content', ''), 140),
		       m.created_at,
		       p.participant
		FROM comm.channels c
		JOIN comm.messages m
		  ON m.tenant_id = c.tenant_id
		 AND COALESCE(m.metadata->>'channel_id', '') = c.id::text
		 AND m.deleted_at IS NULL
		JOIN LATERAL jsonb_array_elements_text(COALESCE(c.metadata->'participants', '[]'::jsonb)) AS p(participant) ON TRUE
		WHERE c.deleted_at IS NULL
		  AND m.created_at <= NOW() - make_interval(secs => $1)
		  AND m.created_at >  NOW() - make_interval(secs => $2)
		  AND p.participant <> COALESCE(m.metadata->>'sender_id', '')
		  AND NOT (COALESCE(m.metadata->'read_by', '[]'::jsonb) @> to_jsonb(p.participant))
		  AND NOT (COALESCE(c.metadata->'muted_by', '[]'::jsonb) @> to_jsonb(p.participant))
		ORDER BY m.created_at ASC
		LIMIT $3
	`, minAge.Seconds(), maxAge.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("list chat reminder candidates: %w", err)
	}
	defer rows.Close()

	out := []ChatReminderCandidate{}
	for rows.Next() {
		var cand ChatReminderCandidate
		if err := rows.Scan(&cand.TenantID, &cand.ChannelID, &cand.ChannelName, &cand.MessageID,
			&cand.SenderID, &cand.SenderName, &cand.Snippet, &cand.SentAt, &cand.ParticipantID); err != nil {
			return nil, fmt.Errorf("scan chat reminder candidate: %w", err)
		}
		out = append(out, cand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat reminder candidates: %w", err)
	}
	return out, nil
}

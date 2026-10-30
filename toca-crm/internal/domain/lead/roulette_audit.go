package lead

import (
	"context"
	"time"
)

type RouletteAuditEntry struct {
	ID         string                 `json:"id"`
	TenantID   string                 `json:"tenant_id"`
	ActorID    string                 `json:"actor_id"`
	ActorEmail string                 `json:"actor_email,omitempty"`
	Action     string                 `json:"action"`
	TargetID   string                 `json:"target_id,omitempty"`
	Diff       map[string]interface{} `json:"diff"`
	CreatedAt  time.Time              `json:"created_at"`
}

// RouletteAuditFilter captures every dimension the audit UI can query by.
// Zero values mean "no filter" for that dimension. Wave P2.14 estendeu
// para suportar filtro por janela de tempo + alvo (broker/lead) sem
// quebrar callers — campos novos são opcionais e ignorados se vazios.
type RouletteAuditFilter struct {
	Limit    int64
	Offset   int64
	Action   string
	TargetID string
	From     *time.Time
	To       *time.Time
}

type RouletteAuditRepository interface {
	Create(ctx context.Context, entry *RouletteAuditEntry) error
	List(ctx context.Context, tenantID string, filter RouletteAuditFilter) ([]*RouletteAuditEntry, int64, error)
}

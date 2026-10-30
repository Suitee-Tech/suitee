package tax_obligation

import (
	"context"
	"time"
)

// ListFilter alimenta listagem e totais. Zero value = tudo do tenant.
type ListFilter struct {
	Status     []Status
	Kind       []Kind
	Competence string
	// DueBefore limita às obrigações com vencimento até a data (usado no filtro
	// "vence este mês" / "atrasadas").
	DueBefore   *time.Time
	OverdueOnly bool
	Limit       int
	Offset      int
}

// KindTotals é o retrato por tipo de tributo.
type KindTotals struct {
	OpenCount       int64 `json:"open_count"`
	OpenAmountCents int64 `json:"open_amount_cents"`
	PaidAmountCents int64 `json:"paid_amount_cents"`
}

// Totals é o resumo do filtro corrente, calculado no servidor (independente da
// paginação) — a tela precisa do total a recolher, não da soma da página.
type Totals struct {
	OpenCount          int64               `json:"open_count"`
	OpenAmountCents    int64               `json:"open_amount_cents"`
	OverdueCount       int64               `json:"overdue_count"`
	OverdueAmountCents int64               `json:"overdue_amount_cents"`
	PaidCount          int64               `json:"paid_count"`
	PaidAmountCents    int64               `json:"paid_amount_cents"`
	ByKind             map[Kind]KindTotals `json:"by_kind"`
}

// Repository persiste as obrigações. Update usa a revisão (updated_at) para não
// sobrescrever uma baixa concorrente.
type Repository interface {
	GetByID(ctx context.Context, tenantID, id string) (*Obligation, error)
	GetByKindCompetence(ctx context.Context, tenantID string, kind Kind, competence string) (*Obligation, error)
	List(ctx context.Context, tenantID string, filter ListFilter) ([]*Obligation, error)
	Totals(ctx context.Context, tenantID string, filter ListFilter) (*Totals, error)
	Create(ctx context.Context, o *Obligation) error
	Update(ctx context.Context, o *Obligation, expectedUpdatedAt time.Time) error
}

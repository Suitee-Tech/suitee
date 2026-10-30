package sale_billing_plan_draft

import "context"

// Repository é a porta de persistência do draft. O índice partial
// (tenant_id, deal_id) WHERE consumed_at IS NULL garante 1 draft
// ativo por deal — upsert substitui em vez de duplicar.
type Repository interface {
	// Upsert grava o draft. Se já existir um draft ATIVO para o
	// (tenant_id, deal_id), substitui (sobrescreve metadata + bump
	// updated_at). Mantém o mesmo row id quando substitui.
	Upsert(ctx context.Context, d *Draft) error

	// GetActiveByDeal devolve o draft ATIVO (consumed_at IS NULL)
	// para o deal. Retorna (nil, nil) quando ausente.
	GetActiveByDeal(ctx context.Context, tenantID, dealID string) (*Draft, error)
	// GetActiveByDealForService é a mesma leitura para rotas internas, onde não
	// há claims de JWT e o pool do app não enxerga a linha.
	GetActiveByDealForService(ctx context.Context, tenantID, dealID string) (*Draft, error)

	// MarkConsumed marca o draft ativo do deal como consumido pelo
	// contract.signed. Idempotente: re-chamadas em draft já consumido
	// retornam nil sem erro.
	MarkConsumed(ctx context.Context, tenantID, dealID string) error
}

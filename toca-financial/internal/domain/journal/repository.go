package journal

import (
	"context"
	"time"
)

// Repository persiste lançamentos contábeis de forma append-only. Não há
// método de edição nem de remoção: a única mutação permitida é a transição
// de status para REVERSED (UpdateStatus), feita de forma otimista.
type Repository interface {
	Create(ctx context.Context, entry *Entry) error
	GetByID(ctx context.Context, tenantID, id string) (*Entry, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*Entry, error)
	UpdateStatus(ctx context.Context, entry *Entry, expected time.Time) (bool, error)
	List(ctx context.Context, tenantID, contractID string, limit, offset int64) ([]*Entry, int64, error)
	// AccountBalance soma as pernas de uma única conta para um tenant,
	// devolvendo o saldo em débito (positivo) e crédito (negativo) em centavos.
	AccountBalance(ctx context.Context, tenantID string, account Account) (int64, error)
}

// FlowBalanceRepository é implementado pelo repositório PostgreSQL e reporta
// o saldo de uma única conta agrupado pela tag de modalidade da perna
// (sale/rent/vazio), para o desdobramento do plano de contas por modalidade.
type FlowBalanceRepository interface {
	AccountBalanceByFlow(ctx context.Context, tenantID string, account Account) (map[string]int64, error)
}

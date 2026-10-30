package broker_advance

import "context"

type ListFilter struct {
	BrokerID string
	Status   []Status
	// OpenOnly limita aos que ainda têm saldo (o que o operador precisa abater).
	OpenOnly bool
	Limit    int
	Offset   int
}

// BrokerBalance é o retrato por corretor: quanto foi adiantado e quanto falta
// recuperar. É o número que o operador olha antes de lançar o desconto.
type BrokerBalance struct {
	BrokerID         string `json:"broker_id"`
	OpenCount        int64  `json:"open_count"`
	OutstandingCents int64  `json:"outstanding_cents"`
	SettledCents     int64  `json:"settled_cents"`
}

type Repository interface {
	GetByID(ctx context.Context, tenantID, id string) (*Advance, error)
	// ListByBroker devolve os adiantamentos de um corretor, dos mais antigos para
	// os mais novos: a recuperação é FIFO (o adiantamento mais antigo primeiro).
	ListByBroker(ctx context.Context, tenantID, brokerID string, filter ListFilter) ([]*Advance, error)
	List(ctx context.Context, tenantID string, filter ListFilter) ([]*Advance, error)
	Balances(ctx context.Context, tenantID string) ([]BrokerBalance, error)
	Create(ctx context.Context, a *Advance) error
	Update(ctx context.Context, a *Advance, expectedUpdatedAt any) error
}

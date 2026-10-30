package deal

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type DealFilter struct {
	Limit  int64
	Offset int64

	Status Status
	Type   Type

	MinValue *float64
	MaxValue *float64

	PropertyID      string
	PropertyAddress string

	PartyName  string
	PartyEmail string
	LeadID     string

	BrokerID      string
	BrokerScopeID string

	CreatedFrom *time.Time
	CreatedTo   *time.Time

	Search string

	CustomerIDs []string
}

type Repository interface {
	Create(ctx context.Context, deal *Deal) error
	GetByID(ctx context.Context, tenantID, id string) (*Deal, error)
	GetByIDScoped(ctx context.Context, tenantID, id, accountID string) (*Deal, error)
	// GetByIDInTx mirrors GetByID but reads via the caller's transaction so
	// callers already inside RunInTx don't acquire a second pool connection
	// (which under load can self-deadlock the pool when N+ concurrent
	// transactions each need a 2nd conn). Use when the read is part of a
	// tx that's about to call UpdateInTx.
	GetByIDInTx(ctx context.Context, tx *sqlx.Tx, tenantID, id string) (*Deal, error)
	Update(ctx context.Context, deal *Deal) error
	// UpdateInTx mirrors Update but joins the caller's transaction so the
	// deal Update can be batched with another write in the same tx
	// (typically an EnqueueOutboxInTx call). Audit P0.1 follow-up.
	UpdateInTx(ctx context.Context, tx *sqlx.Tx, deal *Deal) error
	Delete(ctx context.Context, tenantID, id string) error
	DeleteScoped(ctx context.Context, tenantID, id, accountID string) error
	List(ctx context.Context, tenantID string, limit, offset int64) ([]*Deal, int64, error)
	ListScoped(ctx context.Context, tenantID, accountID string, limit, offset int64) ([]*Deal, int64, error)
	FindByPartyAccountID(ctx context.Context, accountID string) ([]*Deal, error)
	FindByPartyEmail(ctx context.Context, email string) ([]*Deal, error)
	FindByPartyLeadID(ctx context.Context, leadID string) ([]*Deal, error)
	ListByCustomerIDs(ctx context.Context, tenantID string, customerIDs []string, limit, offset int64) ([]*Deal, int64, error)
	ListByCustomerIDsScoped(ctx context.Context, tenantID, accountID string, customerIDs []string, limit, offset int64) ([]*Deal, int64, error)
	FindBySignerEmail(ctx context.Context, email string) ([]*Deal, error)
	FindBySignerIdentity(ctx context.Context, document, email string) ([]*Deal, error)
	GetStats(ctx context.Context, tenantID string) (int64, float64, error)
	GetStatsScoped(ctx context.Context, tenantID, accountID string) (int64, float64, error)
	Search(ctx context.Context, tenantID string, filter DealFilter) ([]*Deal, int64, error)
}

// CreationGuardRepository é a parte OPCIONAL do repositório que protege a
// criação contra duplicação: repetição idempotente (mesma chave) e corrida de
// janela (mesmo imóvel + cliente + tipo + valor em um intervalo curto).
// Implementada pelo repositório Postgres; repositórios de teste que não a
// implementam apenas desligam a proteção (a criação continua funcionando).
type CreationGuardRepository interface {
	// GetByIdempotencyKey devolve o negócio ATIVO criado com a chave — linhas
	// canceladas/removidas liberam a chave para uma nova intenção.
	GetByIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (*Deal, error)
	// FindRecentDuplicate devolve o negócio ativo mais recente com o mesmo
	// imóvel, cliente, tipo e valor criado a partir de `since`.
	FindRecentDuplicate(ctx context.Context, tenantID, propertyID, customerID string, dealType Type, value float64, since time.Time) (*Deal, error)
}

// PortalAccountRepairRepository é a fatia OPCIONAL do repositório usada pelo
// reparo de contas de portal (cmd/repair-portal-accounts): leitura cross-tenant
// pelo pool de serviço, já que o comando roda sem claims de tenant. Repositórios
// de teste que não a implementam apenas deixam o reparo indisponível.
type PortalAccountRepairRepository interface {
	// ListForPortalAccountRepair devolve os negócios ativos que ainda têm
	// alguma parte/signatário com e-mail e sem conta de portal. tenantID vazio
	// = todos os tenants.
	ListForPortalAccountRepair(ctx context.Context, tenantID string, limit int) ([]*Deal, error)
}

// ErrIdempotencyConflict sinaliza que a inserção colidiu com a chave de
// idempotência (ou com a PK) e o vencedor deve ser relido pela chave.
var ErrIdempotencyConflict = errors.New("deal idempotency conflict: an active deal with this key already exists")

// ErrChecklistItemNotFound evita que o DELETE de um item inexistente devolva
// 200 com o item ainda no payload — o frontend acabava reinserindo o item e o
// operador via a remoção "não funcionar".
var ErrChecklistItemNotFound = errors.New("checklist item not found")

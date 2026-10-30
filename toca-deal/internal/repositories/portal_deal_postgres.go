package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// portalDealRepository atende as operações de negócio usadas pelo PORTAL DO
// CLIENTE no POOL DE SERVIÇO (BYPASSRLS).
//
// Por que existe: o portal é cross-tenant por definição — o token de um cliente
// (OTP) tem `app_metadata.tenant_id` = a conta local DELE, enquanto o negócio,
// o checklist, as propostas e os favoritos pertencem ao tenant da IMOBILIÁRIA.
// A RLS de `deal.deals` filtra por `tenant_id = jwt.app_metadata.tenant_id`, então
// sob o pool de aplicação TUDO o que o cliente lê ou escreve voltava vazio: ele
// perdia o acompanhamento do negócio na esteira e os fluxos de aceitar proposta
// e enviar documento falhavam.
//
// A AUTORIZAÇÃO não muda: as rotas do portal resolvem o negócio por
// `findMyDeal`, que só devolve o negócio em que o cliente autenticado é
// signatário (account_id ou e-mail do token), e o `tenant_id` usado adiante é o
// do PRÓPRIO negócio. O pool de serviço apenas remove o filtro de tenant, que
// não se aplica a este ator.
//
// Por que aqui e não em `RunInServiceContext`: medido no beta, a transação de
// serviço do `pkg/database` NÃO bypassa a RLS — `QueryServiceRow(s)` devolvem a
// linha e `RunInServiceContext` + `QueryRowContext` devolvem zero SEM erro
// (relatório separado). Estes dois métodos usam os helpers explícitos, que são
// os verificados em produção, e o resto da superfície segue no repositório
// comum embutido.
type portalDealRepository struct {
	deal.Repository
	db *database.PostgresDB
}

// NewPortalDealRepository monta o repositório do portal sobre o de aplicação
// (que segue atendendo as rotas de operador, tenant-scoped).
func NewPortalDealRepository(db *database.PostgresDB) deal.Repository {
	return &portalDealRepository{Repository: NewDealPostgresRepository(db), db: db}
}

// GetByID lê o negócio do cliente pelo tenant do PRÓPRIO negócio (resolvido no
// vínculo do signatário) no pool de serviço.
func (r *portalDealRepository) GetByID(ctx context.Context, tenantID, id string) (*deal.Deal, error) {
	if tenantID == "" || id == "" {
		return nil, fmt.Errorf("deal not found: tenant ou id vazio")
	}
	var metadata []byte
	if err := r.db.QueryServiceRow(ctx,
		"SELECT metadata FROM deal.deals WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata); err != nil {
		return nil, fmt.Errorf("deal not found: %w", err)
	}
	var d deal.Deal
	if err := unmarshalStoredDeal(metadata, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Update grava o negócio do cliente no pool de serviço, mantendo `tenant_id` no
// predicado como defesa em profundidade (mesma garantia do repositório comum).
func (r *portalDealRepository) Update(ctx context.Context, d *deal.Deal) error {
	if d == nil {
		return fmt.Errorf("deal is nil")
	}
	d.UpdatedAt = time.Now()
	metadata, err := marshalStoredDeal(d)
	if err != nil {
		return err
	}
	return r.db.ExecService(ctx,
		"UPDATE deal.deals SET metadata = $1 WHERE id = $2 AND tenant_id = $3",
		string(metadata), d.ID, d.TenantID)
}

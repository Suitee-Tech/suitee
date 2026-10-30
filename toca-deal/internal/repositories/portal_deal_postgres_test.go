package repositories

import (
	"context"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// O portal do cliente é cross-tenant: as operações do negócio dele precisam
// sair pelo pool de serviço. Sem wiring explícito (testes/dev), o repositório
// do portal simplesmente delega para o comum — nunca fica sem implementation.
type recordingRepository struct {
	deal.Repository
	getByIDCalls int
	updateCalls  int
	lastCtx      context.Context
}

func (r *recordingRepository) GetByID(ctx context.Context, tenantID, id string) (*deal.Deal, error) {
	r.getByIDCalls++
	r.lastCtx = ctx
	return &deal.Deal{ID: id, TenantID: tenantID}, nil
}

func (r *recordingRepository) Update(_ context.Context, _ *deal.Deal) error {
	r.updateCalls++
	return nil
}

func TestPortalDealRepositoryFailsLoudlyWithoutIdentifiers(t *testing.T) {
	repo := &portalDealRepository{Repository: &recordingRepository{}}
	if _, err := repo.GetByID(context.Background(), "", "deal"); err == nil {
		t.Fatal("leitura sem tenant deveria falhar (não pode cair em varredura cross-tenant)")
	}
	if _, err := repo.GetByID(context.Background(), "tenant", ""); err == nil {
		t.Fatal("leitura sem id deveria falhar")
	}
}

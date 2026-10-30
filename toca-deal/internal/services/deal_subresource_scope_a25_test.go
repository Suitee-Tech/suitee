package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// a25EventBus é o dublê mínimo do barramento (o teste não exercita publicação).
type a25EventBus struct{}

func (a25EventBus) Publish(context.Context, events.Event) error         { return nil }
func (a25EventBus) PublishReliable(context.Context, events.Event) error { return nil }
func (a25EventBus) Subscribe(string, events.EventHandler) error         { return nil }
func (a25EventBus) SubscribeWithQueue(string, string, events.EventHandler) error {
	return nil
}

// P1/A25 — escopo por corretor nos sub-recursos do negócio.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A25) mostrou que `reconcile-signers`, `GET /deals/:id/contracts`,
// `GET /contracts/:id` e `GET /contracts/:id/documents` só filtravam por tenant:
// com as permissões que o `corretor` tem (`deals:write`, `contracts:read`,
// `documents:read`), ele alcançava negócio de OUTRO corretor dentro do tenant.
//
// O dublê abaixo é deliberadamente FAIL-CLOSED no escopo — ao contrário dos
// dublês lenientes usados em outros testes, que devolvem o negócio para qualquer
// account e por isso esconderiam justamente o furo do A25.

type a25DealRepo struct {
	deal.Repository
	d *deal.Deal
}

func (r *a25DealRepo) GetByID(_ context.Context, tenantID, id string) (*deal.Deal, error) {
	if r.d == nil || r.d.ID != id || (tenantID != "" && r.d.TenantID != tenantID) {
		return nil, errors.New("deal not found")
	}
	return r.d, nil
}

// GetByIDScoped só devolve o negócio quando o account é o corretor dono ou um
// participante — é o contrato do repositório real.
func (r *a25DealRepo) GetByIDScoped(_ context.Context, tenantID, id, accountID string) (*deal.Deal, error) {
	d, err := r.GetByID(context.Background(), tenantID, id)
	if err != nil {
		return nil, err
	}
	if d.BrokerID == accountID {
		return d, nil
	}
	for _, p := range d.BrokerParticipations {
		if p.BrokerID == accountID {
			return d, nil
		}
	}
	return nil, errors.New("deal not found")
}

type a25ContractRepo struct {
	contract.Repository
	items []*contract.Contract
}

func (r *a25ContractRepo) GetByID(_ context.Context, tenantID, id string) (*contract.Contract, error) {
	for _, c := range r.items {
		if c.ID == id && (tenantID == "" || c.TenantID == tenantID) {
			return c, nil
		}
	}
	return nil, nil
}

func (r *a25ContractRepo) GetByDealID(_ context.Context, tenantID, dealID string) ([]*contract.Contract, error) {
	out := make([]*contract.Contract, 0)
	for _, c := range r.items {
		if c.DealID == dealID && (tenantID == "" || c.TenantID == tenantID) {
			out = append(out, c)
		}
	}
	return out, nil
}

func a25Deal() *deal.Deal {
	return &deal.Deal{ID: "deal-1", TenantID: "tenant-1", BrokerID: "broker-dono"}
}

func a25Contract() *contract.Contract {
	return &contract.Contract{ID: "contract-1", TenantID: "tenant-1", DealID: "deal-1"}
}

func newA25ContractService() *ContractService {
	return NewContractService(
		&a25ContractRepo{items: []*contract.Contract{a25Contract()}},
		&a25DealRepo{d: a25Deal()},
		a25EventBus{},
		nil,
	)
}

// O corretor de OUTRO negócio não lista os contratos do negócio alheio.
func TestContractService_ListContractsByDeal_RespectsBrokerScope(t *testing.T) {
	svc := newA25ContractService()
	ctx := context.Background()

	// Sem escopo (admin/owner/manager ou chamador interno): lista.
	if got, err := svc.ListContractsByDeal(ctx, "tenant-1", "deal-1", ""); err != nil || len(got) != 1 {
		t.Fatalf("chamador sem recorte deveria listar: err=%v len=%d", err, len(got))
	}

	// Corretor dono: lista.
	if got, err := svc.ListContractsByDeal(ctx, "tenant-1", "deal-1", "broker-dono"); err != nil || len(got) != 1 {
		t.Fatalf("corretor dono deveria listar: err=%v len=%d", err, len(got))
	}

	// Corretor estranho: NÃO lista (A25).
	got, err := svc.ListContractsByDeal(ctx, "tenant-1", "deal-1", "broker-estranho")
	if !errors.Is(err, ErrDealNotInScope) {
		t.Fatalf("corretor estranho deveria receber ErrDealNotInScope, veio err=%v (contratos=%d)", err, len(got))
	}
	if len(got) != 0 {
		t.Fatalf("contratos vazaram para corretor estranho: %d", len(got))
	}
}

// O corretor estranho não lê o contrato por id — e a resposta é a MESMA do
// contrato inexistente (não confirma existência).
func TestContractService_GetContract_RespectsBrokerScope(t *testing.T) {
	svc := newA25ContractService()
	ctx := context.Background()

	if _, err := svc.GetContract(ctx, "tenant-1", "contract-1", ""); err != nil {
		t.Fatalf("chamador sem recorte deveria ler o contrato: %v", err)
	}
	if _, err := svc.GetContract(ctx, "tenant-1", "contract-1", "broker-dono"); err != nil {
		t.Fatalf("corretor dono deveria ler o contrato: %v", err)
	}

	_, err := svc.GetContract(ctx, "tenant-1", "contract-1", "broker-estranho")
	if err == nil || err.Error() != "contract not found" {
		t.Fatalf("corretor estranho deveria receber 'contract not found', veio %v", err)
	}
}

// O validador usado pelos documentos do contrato segue a mesma regra.
func TestContractService_ContractInBrokerScope(t *testing.T) {
	svc := newA25ContractService()
	ctx := context.Background()

	if err := svc.ContractInBrokerScope(ctx, "tenant-1", "contract-1", ""); err != nil {
		t.Fatalf("sem recorte deveria passar: %v", err)
	}
	if err := svc.ContractInBrokerScope(ctx, "tenant-1", "contract-1", "broker-dono"); err != nil {
		t.Fatalf("corretor dono deveria passar: %v", err)
	}
	if err := svc.ContractInBrokerScope(ctx, "tenant-1", "contract-1", "broker-estranho"); !errors.Is(err, ErrDealNotInScope) {
		t.Fatalf("corretor estranho deveria ser barrado, veio %v", err)
	}
	if err := svc.ContractInBrokerScope(ctx, "tenant-1", "contract-inexistente", "broker-estranho"); !errors.Is(err, ErrDealNotInScope) {
		t.Fatalf("contrato inexistente deveria ser barrado, veio %v", err)
	}
}

// Contrato sem negócio vinculado (dado legado) não é recortado — não há negócio
// para comparar e negar quebraria a leitura de registros antigos.
func TestContractService_ContractWithoutDealIsNotScoped(t *testing.T) {
	svc := NewContractService(
		&a25ContractRepo{items: []*contract.Contract{{ID: "contract-legado", TenantID: "tenant-1"}}},
		&a25DealRepo{d: a25Deal()},
		a25EventBus{},
		nil,
	)
	if err := svc.ContractInBrokerScope(context.Background(), "tenant-1", "contract-legado", "broker-estranho"); err != nil {
		t.Fatalf("contrato sem deal não deveria ser recortado: %v", err)
	}
}

// fakeSignerSyncer registra se a ressincronização foi chamada.
type fakeSignerSyncer struct {
	calls int
}

func (f *fakeSignerSyncer) SyncDealSignersFromActiveContract(context.Context, string, string) error {
	f.calls++
	return nil
}

func newA25DealService(syncer *fakeSignerSyncer) *DealService {
	return &DealService{
		repo:         &a25DealRepo{d: a25Deal()},
		signerSyncer: syncer,
	}
}

// reconcile-signers com corretor estranho: negado E sem efeito (o sincronizador
// não é chamado).
func TestDealService_ReconcileSigners_RespectsBrokerScope(t *testing.T) {
	syncer := &fakeSignerSyncer{}
	svc := newA25DealService(syncer)
	ctx := context.Background()

	if err := svc.ReconcileSigners(ctx, "tenant-1", "deal-1", "broker-estranho"); !errors.Is(err, ErrDealNotInScope) {
		t.Fatalf("corretor estranho deveria receber ErrDealNotInScope, veio %v", err)
	}
	if syncer.calls != 0 {
		t.Fatalf("a ressincronização rodou apesar do escopo: %d chamada(s)", syncer.calls)
	}

	// Corretor dono: ressincroniza.
	if err := svc.ReconcileSigners(ctx, "tenant-1", "deal-1", "broker-dono"); err != nil {
		t.Fatalf("corretor dono deveria ressincronizar: %v", err)
	}
	if syncer.calls != 1 {
		t.Fatalf("esperava 1 chamada do sincronizador, veio %d", syncer.calls)
	}

	// Sem recorte (admin/owner/manager ou serviço): ressincroniza.
	if err := svc.ReconcileSigners(ctx, "tenant-1", "deal-1", ""); err != nil {
		t.Fatalf("chamador sem recorte deveria ressincronizar: %v", err)
	}
	if syncer.calls != 2 {
		t.Fatalf("esperava 2 chamadas, veio %d", syncer.calls)
	}
}

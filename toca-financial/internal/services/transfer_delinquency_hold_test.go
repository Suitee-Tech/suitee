package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// transferRepoMemoria reaproveita o fake completo do pacote (billingTransferRepo)
// e sobrescreve o que a política precisa: leitura por id, listagem por contrato e
// gravação. Assim o teste não depende de reimplementar a interface inteira.
type transferRepoMemoria struct {
	itens   []*transfer.OwnerTransfer
	updated []*transfer.OwnerTransfer
}

// Stubs exigidos pela interface (o teste exercita leitura, listagem e gravação).
func (r *transferRepoMemoria) Create(context.Context, *transfer.OwnerTransfer) error { return nil }
func (r *transferRepoMemoria) GetByContractMonth(context.Context, string, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}
func (r *transferRepoMemoria) ListByOwnerLeadIDs(context.Context, string, []string, transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *transferRepoMemoria) UpdateStatus(context.Context, string, string, transfer.Status) error {
	return nil
}
func (r *transferRepoMemoria) Search(context.Context, string, transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *transferRepoMemoria) GetByID(_ context.Context, _, id string) (*transfer.OwnerTransfer, error) {
	for _, t := range r.itens {
		if t.ID == id {
			copia := *t
			return &copia, nil
		}
	}
	return nil, nil
}

func (r *transferRepoMemoria) List(_ context.Context, _, contractID, _, _, _ string, _, _ int64) ([]*transfer.OwnerTransfer, int64, error) {
	out := make([]*transfer.OwnerTransfer, 0, len(r.itens))
	for _, t := range r.itens {
		if contractID == "" || t.ContractID == contractID {
			copia := *t
			out = append(out, &copia)
		}
	}
	return out, int64(len(out)), nil
}

func (r *transferRepoMemoria) Update(_ context.Context, t *transfer.OwnerTransfer) error {
	for i, atual := range r.itens {
		if atual.ID == t.ID {
			copia := *t
			r.itens[i] = &copia
			r.updated = append(r.updated, &copia)
			return nil
		}
	}
	return nil
}

func repasseDeTeste(id, contractID string, valor float64) *transfer.OwnerTransfer {
	return &transfer.OwnerTransfer{
		ID: id, TenantID: "t-1", ContractID: contractID, OwnerID: "dono-1",
		ReferenceMonth: "2026-10", NetAmount: valor, Status: transfer.StatusPending,
		OwnerBalance: &transfer.OwnerBalance{TotalCents: int64(valor * 100)},
		UpdatedAt:    time.Now(),
	}
}

// A materialidade é a resposta para "reter R$ 3.000 por R$ 200": com a régua
// padrão (100%), dívida pequena NÃO trava o repasse do proprietário.
func TestRetencaoNaoAplicaQuandoDividaEhPequena(t *testing.T) {
	repo := &transferRepoMemoria{}
	svc := NewTransferService(repo).
		WithDelinquencyPolicy(func(context.Context, string) DelinquencyPolicy {
			return DelinquencyPolicy{Enabled: true} // materialidade padrão: 100%
		}).
		WithOpenCaseDebt(func(context.Context, string, string) (float64, string) {
			return 200, "caso-1" // R$ 200 de dívida
		})

	repasse := repasseDeTeste("tr-1", "c-1", 3000)
	svc.applyDelinquencyHold(context.Background(), repasse)

	if repasse.IsHeldForDelinquency() {
		t.Fatalf("repasse de R$ 3.000 não pode ser retido por dívida de R$ 200")
	}
}

// Dívida que cobre o repasse retém — e o motivo fica registrado no próprio repasse.
func TestRetencaoAplicaQuandoDividaCobreRepasse(t *testing.T) {
	repo := &transferRepoMemoria{}
	svc := NewTransferService(repo).
		WithDelinquencyPolicy(func(context.Context, string) DelinquencyPolicy {
			return DelinquencyPolicy{Enabled: true}
		}).
		WithOpenCaseDebt(func(context.Context, string, string) (float64, string) {
			return 3000, "caso-9"
		})

	repasse := repasseDeTeste("tr-1", "c-1", 3000)
	svc.applyDelinquencyHold(context.Background(), repasse)

	if !repasse.IsHeldForDelinquency() {
		t.Fatalf("dívida que cobre o repasse deveria retê-lo")
	}
	if repasse.DelinquencyHeldCents != 300000 || repasse.DelinquencyCaseID != "caso-9" {
		t.Fatalf("retenção mal registrada: cents=%d caso=%q", repasse.DelinquencyHeldCents, repasse.DelinquencyCaseID)
	}
	if repasse.DelinquencyHoldReason == "" || repasse.DelinquencyHeldAt == nil {
		t.Fatalf("motivo/data da retenção não registrados")
	}
}

// A imobiliária que quiser ser mais conservadora baixa a materialidade.
func TestRetencaoComMaterialidadeConfigurada(t *testing.T) {
	svc := NewTransferService(&transferRepoMemoria{}).
		WithDelinquencyPolicy(func(context.Context, string) DelinquencyPolicy {
			return DelinquencyPolicy{Enabled: true, MinSharePct: 30}
		}).
		WithOpenCaseDebt(func(context.Context, string, string) (float64, string) {
			return 1000, "caso-2"
		})

	repasse := repasseDeTeste("tr-1", "c-1", 3000) // dívida = 33% do repasse
	svc.applyDelinquencyHold(context.Background(), repasse)

	if !repasse.IsHeldForDelinquency() {
		t.Fatalf("com materialidade de 30%%, dívida de 33%% deveria reter")
	}
}

// Política desligada (padrão da plataforma) não retém nada: comportamento legado.
func TestRetencaoDesligadaNaoTravaNada(t *testing.T) {
	svc := NewTransferService(&transferRepoMemoria{}).
		WithDelinquencyPolicy(func(context.Context, string) DelinquencyPolicy {
			return DelinquencyPolicy{Enabled: false}
		}).
		WithOpenCaseDebt(func(context.Context, string, string) (float64, string) {
			return 99999, "caso-3"
		})

	repasse := repasseDeTeste("tr-1", "c-1", 3000)
	svc.applyDelinquencyHold(context.Background(), repasse)

	if repasse.IsHeldForDelinquency() {
		t.Fatalf("política desligada não pode reter")
	}
}

// Repasse retido não é aprovado nem pago; liberar devolve ao fluxo.
func TestRepasseRetidoNaoAprovaNemPagaERelebara(t *testing.T) {
	retido := repasseDeTeste("tr-1", "c-1", 3000)
	if err := retido.HoldForDelinquency(300000, "caso-1", "dívida em aberto", time.Now()); err != nil {
		t.Fatalf("HoldForDelinquency: %v", err)
	}
	repo := &transferRepoMemoria{itens: []*transfer.OwnerTransfer{retido}}
	svc := NewTransferService(repo)

	if err := svc.Approve(context.Background(), "t-1", "tr-1", "u1", "Operador"); !errors.Is(err, transfer.ErrTransferHeldByDelinquency) {
		t.Fatalf("aprovação de repasse retido = %v, esperado ErrTransferHeldByDelinquency", err)
	}

	liberado, err := svc.ReleaseDelinquencyHold(context.Background(), "t-1", "tr-1", "risco aceito", "gerente")
	if err != nil {
		t.Fatalf("ReleaseDelinquencyHold: %v", err)
	}
	if liberado.IsHeldForDelinquency() {
		t.Fatalf("retenção deveria ter sido liberada")
	}
	if err := svc.Approve(context.Background(), "t-1", "tr-1", "u1", "Operador"); err != nil {
		t.Fatalf("após liberar, a aprovação deveria funcionar: %v", err)
	}
}

// Caso resolvido libera a retenção do contrato — sem pagar nada (o pagamento
// continua exigindo aprovação).
func TestCasoResolvidoLiberaRetencaoDoContrato(t *testing.T) {
	um := repasseDeTeste("tr-1", "c-1", 3000)
	dois := repasseDeTeste("tr-2", "c-2", 5000)
	for _, r := range []*transfer.OwnerTransfer{um, dois} {
		_ = r.HoldForDelinquency(int64(r.NetAmount*100), "caso-1", "dívida em aberto", time.Now())
	}
	repo := &transferRepoMemoria{itens: []*transfer.OwnerTransfer{um, dois}}
	svc := NewTransferService(repo)

	liberados, err := svc.ReleaseHoldsForContract(context.Background(), "t-1", "c-1", "caso resolvido")
	if err != nil {
		t.Fatalf("ReleaseHoldsForContract: %v", err)
	}
	if liberados != 1 {
		t.Fatalf("liberados = %d, esperado 1 (só o contrato c-1)", liberados)
	}
	if repo.itens[0].IsHeldForDelinquency() {
		t.Fatalf("repasse do contrato resolvido continua retido")
	}
	if !repo.itens[1].IsHeldForDelinquency() {
		t.Fatalf("retenção de OUTRO contrato não pode ser liberada")
	}
	if repo.itens[0].Status == transfer.StatusProcessed {
		t.Fatalf("liberar retenção não pode pagar o repasse")
	}
}

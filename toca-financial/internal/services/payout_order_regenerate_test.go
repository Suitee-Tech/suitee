package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fake mínimo da regeração: lê a ordem cancelada, sabe procurar uma ordem viva
// pela mesma origem e guarda o que foi criado.
type regenerateRepo struct {
	payout_order.Repository
	order       *payout_order.PayoutOrder
	live        *payout_order.PayoutOrder
	chain       []*payout_order.PayoutOrder
	cancellable bool
	created     *payout_order.PayoutOrder
	createCalls int
	createErr   error
	transitions int
}

func (r *regenerateRepo) GetByID(_ context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	if r.order == nil || r.order.TenantID != tenantID || r.order.ID != id {
		return nil, nil
	}
	return clonePayoutOrderForTest(r.order), nil
}

func (r *regenerateRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*payout_order.PayoutOrder, error) {
	if r.live == nil || r.live.TenantID != tenantID || r.live.SourceEventID != sourceEventID {
		return nil, nil
	}
	return clonePayoutOrderForTest(r.live), nil
}

func (r *regenerateRepo) ListByIDs(_ context.Context, tenantID string, ids []string) ([]*payout_order.PayoutOrder, error) {
	var out []*payout_order.PayoutOrder
	for _, candidate := range r.chain {
		if candidate == nil || candidate.TenantID != tenantID {
			continue
		}
		for _, id := range ids {
			if candidate.ID == id {
				out = append(out, clonePayoutOrderForTest(candidate))
			}
		}
	}
	return out, nil
}

func (r *regenerateRepo) UpdateIfStatus(_ context.Context, order *payout_order.PayoutOrder, _ payout_order.Status, _ time.Time) error {
	if !r.cancellable {
		return payout_order.ErrTransitionCompareAndSetMissing
	}
	r.order = clonePayoutOrderForTest(order)
	return nil
}

func (r *regenerateRepo) Create(_ context.Context, order *payout_order.PayoutOrder) error {
	r.createCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.created = clonePayoutOrderForTest(order)
	return nil
}

func (r *regenerateRepo) RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.transitions++
	return fn(ctx)
}

func (r *regenerateRepo) runTransitionUnused(ctx context.Context, fn func(context.Context) error) error {
	r.transitions++
	return fn(ctx)
}

func cancelledOrderForRegenerate() *payout_order.PayoutOrder {
	return &payout_order.PayoutOrder{
		ID:             "pay-cancelled",
		TenantID:       "tenant-1",
		SourceType:     payout_order.SourceManual,
		SourceID:       "contract-1",
		ContractID:     "contract-1",
		RecipientType:  payout_order.RecipientExternal,
		RecipientID:    "payee-1",
		RecipientLabel: "Repasse avulso",
		AmountCents:    150000,
		SourceEventID:  "source-event-1",
		Status:         payout_order.StatusCancelled,
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
	}
}

// O caminho do relato: cancelou, quer lançar de novo. Regerar cria uma ordem
// nova em PENDING_REVIEW (pronta para aprovar) mantendo a origem e apontando
// para a cancelada, dentro de uma transação.
func TestRegenerate_CreatesFreshOrderReadyToLaunch(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	repo := &regenerateRepo{order: cancelled}
	svc := NewPayoutOrderService(repo)

	fresh, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	require.NoError(t, err)
	require.NotNil(t, fresh)

	assert.Equal(t, payout_order.StatusPendingReview, fresh.Status)
	assert.NotEqual(t, cancelled.ID, fresh.ID)
	assert.Equal(t, cancelled.SourceEventID, fresh.SourceEventID)
	assert.Equal(t, cancelled.AmountCents, fresh.AmountCents)
	assert.Equal(t, cancelled.ID, fresh.RegeneratedFromID)
	assert.Equal(t, 1, repo.createCalls, "a ordem nova precisa ser persistida")
	assert.Equal(t, 1, repo.transitions, "criação + reabertura da obrigação na mesma transação")
	require.NotNil(t, repo.created)
	assert.Equal(t, fresh.ID, repo.created.ID)
	// A cancelada segue como histórico.
	assert.Equal(t, payout_order.StatusCancelled, cancelled.Status)
}

// Dois cliques no botão não podem gerar duas obrigações: se já existe ordem viva
// para a mesma origem, ela é devolvida.
func TestRegenerate_ReturnsLiveOrderInsteadOfDuplicating(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	live := clonePayoutOrderForTest(cancelled)
	live.ID = "pay-live"
	live.Status = payout_order.StatusPendingReview

	repo := &regenerateRepo{order: cancelled, live: live}
	svc := NewPayoutOrderService(repo)

	got, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pay-live", got.ID)
	assert.Zero(t, repo.createCalls, "não pode criar uma segunda ordem para a mesma obrigação")
}

func TestRegenerate_RejectsOrderThatIsNotCancelled(t *testing.T) {
	order := cancelledOrderForRegenerate()
	order.Status = payout_order.StatusApproved
	repo := &regenerateRepo{order: order}
	svc := NewPayoutOrderService(repo)

	_, err := svc.Regenerate(context.Background(), "tenant-1", order.ID, "operator-1")
	assert.ErrorIs(t, err, payout_order.ErrNotCancelled)
	assert.Zero(t, repo.createCalls)
}

// Sem vínculo (ordem externa/manual sem comissão) a regeração não pode falhar:
// a ordem nova é o suficiente.
func TestRegenerate_WithoutLinkedObligationStillCreates(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	repo := &regenerateRepo{order: cancelled}
	svc := NewPayoutOrderService(repo)

	_, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	require.NoError(t, err)
	assert.Equal(t, 1, repo.createCalls)
}

// Falha ao criar não pode deixar rastro: a transação é quem decide.
func TestRegenerate_PropagatesCreateFailure(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	repo := &regenerateRepo{order: cancelled, createErr: errors.New("insert failed")}
	svc := NewPayoutOrderService(repo)

	_, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	require.Error(t, err)
	assert.Equal(t, 1, repo.transitions)
}

// O caminho com obrigação vinculada: a comissão está presa à ordem CANCELADA e
// precisa terminar reaberta e ligada à ordem NOVA. Este teste cobre exatamente o
// guard que o beta pegou — validar o vínculo contra a ordem nova é recusado,
// porque a comissão ainda aponta para a cancelada.
func TestRegenerate_ReopensLinkedCommissionAndRelinksIt(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	cancelled.RecipientType = payout_order.RecipientBroker
	cancelled.SourceRef = map[string]any{"deal_id": "deal-1"}

	comm := &commission.Commission{
		ID:            "comm-1",
		TenantID:      "tenant-1",
		DealID:        "deal-1",
		Status:        commission.StatusCancelled,
		PayoutOrderID: cancelled.ID,
		Amount:        1500,
		UpdatedAt:     time.Now().Add(-time.Minute),
	}
	commRepo := &payoutAtomicCommissionRepo{commission: comm}
	repo := &regenerateRepo{order: cancelled}

	svc := NewPayoutOrderService(repo)
	svc.SetCommissionService(NewCommissionService(commRepo))

	fresh, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	require.NoError(t, err)
	require.NotNil(t, fresh)

	require.NotNil(t, commRepo.commission)
	assert.Equal(t, commission.StatusPending, commRepo.commission.Status, "a comissão cancelada precisa voltar a PENDING")
	assert.Equal(t, fresh.ID, commRepo.commission.PayoutOrderID, "a comissão precisa passar a apontar para a ordem regerada")
	assert.Nil(t, commRepo.commission.CancelledAt)
}

// A comissão pode estar presa a OUTRA ordem da mesma origem — o operador
// cancela uma ordem regerada e regera a partir da cancelada anterior. Comparar
// só por id deixaria a comissão morta com uma ordem viva na mão (a ordem
// regerada não poderia ser aprovada).
func TestRegenerate_FindsCommissionLinkedElsewhereInTheSameObligation(t *testing.T) {
	ancestor := cancelledOrderForRegenerate()
	ancestor.ID = "pay-ancestor"
	ancestor.RecipientType = payout_order.RecipientBroker
	ancestor.SourceRef = map[string]any{"deal_id": "deal-1"}

	leaf := clonePayoutOrderForTest(ancestor)
	leaf.ID = "pay-leaf"
	leaf.Status = payout_order.StatusCancelled

	comm := &commission.Commission{
		ID:            "comm-1",
		TenantID:      "tenant-1",
		DealID:        "deal-1",
		Status:        commission.StatusCancelled,
		PayoutOrderID: "pay-leaf", // presa à ordem NOVA, não à ancestral
		Amount:        1500,
		UpdatedAt:     time.Now().Add(-time.Minute),
	}
	commRepo := &payoutAtomicCommissionRepo{commission: comm}
	repo := &regenerateRepo{order: ancestor, chain: []*payout_order.PayoutOrder{leaf}}

	svc := NewPayoutOrderService(repo)
	svc.SetCommissionService(NewCommissionService(commRepo))

	fresh, err := svc.Regenerate(context.Background(), "tenant-1", ancestor.ID, "operator-1")
	require.NoError(t, err)
	require.NotNil(t, fresh)
	require.NotNil(t, commRepo.commission)
	assert.Equal(t, commission.StatusPending, commRepo.commission.Status)
	assert.Equal(t, fresh.ID, commRepo.commission.PayoutOrderID)
}

// Repasse a corretor sem comissão nenhuma: recusar é melhor do que devolver uma
// ordem que não pode ser aprovada (o cascade da aprovação exige a comissão).
func TestRegenerate_RefusesBrokerPayoutWithoutLinkedCommission(t *testing.T) {
	cancelled := cancelledOrderForRegenerate()
	cancelled.RecipientType = payout_order.RecipientBroker
	cancelled.SourceRef = map[string]any{"deal_id": "deal-1"}
	commRepo := &payoutAtomicCommissionRepo{} // nenhuma comissão
	repo := &regenerateRepo{order: cancelled}

	svc := NewPayoutOrderService(repo)
	svc.SetCommissionService(NewCommissionService(commRepo))

	_, err := svc.Regenerate(context.Background(), "tenant-1", cancelled.ID, "operator-1")
	assert.ErrorIs(t, err, ErrLinkedCommissionNotFound)
}

// O caminho completo do operador: cancelar a ordem regerada (a comissão aponta
// para a anterior da cadeia) e regerar de novo. Antes deste ajuste o CANCELAMENTO
// falhava com "linked commission not found for broker payout" — a ordem regerada
// ficava impossível de cancelar e de aprovar.
func TestCancel_ResolvesCommissionThroughTheObligationChain(t *testing.T) {
	leaf := cancelledOrderForRegenerate()
	leaf.ID = "pay-leaf"
	leaf.Status = payout_order.StatusPendingReview
	leaf.RecipientType = payout_order.RecipientBroker
	leaf.SourceRef = map[string]any{"deal_id": "deal-1"}

	previous := clonePayoutOrderForTest(leaf)
	previous.ID = "pay-previous"
	previous.Status = payout_order.StatusCancelled

	comm := &commission.Commission{
		ID:            "comm-1",
		TenantID:      "tenant-1",
		DealID:        "deal-1",
		Status:        commission.StatusPending,
		PayoutOrderID: "pay-previous", // presa à ordem ANTERIOR da mesma obrigação
		Amount:        1500,
		UpdatedAt:     time.Now().Add(-time.Minute),
	}
	commRepo := &payoutAtomicCommissionRepo{commission: comm}
	repo := &regenerateRepo{order: leaf, chain: []*payout_order.PayoutOrder{previous}, cancellable: true}

	svc := NewPayoutOrderService(repo)
	svc.SetCommissionService(NewCommissionService(commRepo))

	cancelled, err := svc.Cancel(context.Background(), "tenant-1", leaf.ID, "operator-1", "corrigindo")
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, payout_order.StatusCancelled, cancelled.Status)
	assert.Equal(t, commission.StatusCancelled, commRepo.commission.Status, "a comissão da obrigação precisa cair junto")
}

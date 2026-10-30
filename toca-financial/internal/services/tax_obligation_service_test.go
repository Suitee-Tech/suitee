package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/tax_obligation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taxObligationFakeRepo struct {
	mu      sync.Mutex
	byKey   map[string]*tax_obligation.Obligation
	updates int
}

func newTaxObligationFakeRepo() *taxObligationFakeRepo {
	return &taxObligationFakeRepo{byKey: map[string]*tax_obligation.Obligation{}}
}

func key(kind tax_obligation.Kind, competence string) string { return string(kind) + "|" + competence }

func (r *taxObligationFakeRepo) GetByID(_ context.Context, _, id string) (*tax_obligation.Obligation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, obligation := range r.byKey {
		if obligation.ID == id {
			clone := *obligation
			return &clone, nil
		}
	}
	return nil, nil
}

func (r *taxObligationFakeRepo) GetByKindCompetence(_ context.Context, _ string, kind tax_obligation.Kind, competence string) (*tax_obligation.Obligation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	obligation := r.byKey[key(kind, competence)]
	if obligation == nil || obligation.Status == tax_obligation.StatusCancelled {
		return nil, nil
	}
	clone := *obligation
	return &clone, nil
}

func (r *taxObligationFakeRepo) List(_ context.Context, _ string, filter tax_obligation.ListFilter) ([]*tax_obligation.Obligation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*tax_obligation.Obligation, 0, len(r.byKey))
	for _, obligation := range r.byKey {
		if len(filter.Status) > 0 {
			matched := false
			for _, status := range filter.Status {
				if obligation.Status == status {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		clone := *obligation
		out = append(out, &clone)
	}
	return out, nil
}

func (r *taxObligationFakeRepo) Totals(_ context.Context, _ string, _ tax_obligation.ListFilter) (*tax_obligation.Totals, error) {
	return &tax_obligation.Totals{ByKind: map[tax_obligation.Kind]tax_obligation.KindTotals{}}, nil
}

func (r *taxObligationFakeRepo) Create(_ context.Context, obligation *tax_obligation.Obligation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *obligation
	r.byKey[key(obligation.Kind, obligation.Competence)] = &clone
	return nil
}

func (r *taxObligationFakeRepo) Update(_ context.Context, obligation *tax_obligation.Obligation, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *obligation
	r.byKey[key(obligation.Kind, obligation.Competence)] = &clone
	r.updates++
	return nil
}

type taxCommissionLookupFake struct {
	commission *commission.Commission
}

func (f *taxCommissionLookupFake) GetByID(_ context.Context, _, _ string) (*commission.Commission, error) {
	return f.commission, nil
}

func (f *taxCommissionLookupFake) List(_ context.Context, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	if f.commission == nil {
		return nil, 0, nil
	}
	return []*commission.Commission{f.commission}, 1, nil
}

func TestTaxObligation_AccumulatesPerKindAndCompetence(t *testing.T) {
	repo := newTaxObligationFakeRepo()
	paidAt := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	c := &commission.Commission{
		ID: "comm-1", TenantID: "tenant-1", ContractID: "contract-1", BrokerID: "broker-1",
		Status: commission.StatusPending, GrossAmount: 100000, NetAmount: 93200,
		Deductions: []commission.Deduction{
			{ID: "d1", Kind: commission.DeductionIRRF, Amount: 432, CreatedAt: paidAt},
			{ID: "d2", Kind: commission.DeductionIRRF, Amount: 100, CreatedAt: paidAt},
			{ID: "d3", Kind: commission.DeductionISS, Amount: 500, CreatedAt: paidAt},
		},
	}
	svc := NewTaxObligationService(repo, &taxCommissionLookupFake{commission: c})
	require.NoError(t, svc.SyncCommission(context.Background(), "tenant-1", c.ID))

	irrf := repo.byKey[key(tax_obligation.KindIRRF, "2026-09")]
	require.NotNil(t, irrf, "IRRF de setembro precisa virar obrigação")
	assert.Equal(t, int64(53200), irrf.AmountCents, "duas retenções de IRRF do mês somam na mesma obrigação")
	assert.Equal(t, "2026-10-20", irrf.DueDate.Format("2006-01-02"))
	assert.Equal(t, tax_obligation.StatusOpen, irrf.Status)

	iss := repo.byKey[key(tax_obligation.KindISS, "2026-09")]
	require.NotNil(t, iss, "ISS tem obrigação própria")
	assert.Equal(t, int64(50000), iss.AmountCents)

	// Recomputar é idempotente: o mesmo estado não duplica valor.
	require.NoError(t, svc.SyncCommission(context.Background(), "tenant-1", c.ID))
	assert.Equal(t, int64(53200), repo.byKey[key(tax_obligation.KindIRRF, "2026-09")].AmountCents)
}

func TestTaxObligation_RemovingDeductionDropsTheObligation(t *testing.T) {
	repo := newTaxObligationFakeRepo()
	at := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	c := &commission.Commission{
		ID: "comm-1", TenantID: "tenant-1", Status: commission.StatusPending,
		Deductions: []commission.Deduction{{ID: "d1", Kind: commission.DeductionIRRF, Amount: 432, CreatedAt: at}},
	}
	svc := NewTaxObligationService(repo, &taxCommissionLookupFake{commission: c})
	require.NoError(t, svc.SyncCommission(context.Background(), "tenant-1", c.ID))
	require.NotNil(t, repo.byKey[key(tax_obligation.KindIRRF, "2026-09")])

	// O operador removeu o desconto: a retenção deixa de existir.
	c.Deductions = nil
	require.NoError(t, svc.SyncCommission(context.Background(), "tenant-1", c.ID))
	obligation := repo.byKey[key(tax_obligation.KindIRRF, "2026-09")]
	assert.Equal(t, tax_obligation.StatusCancelled, obligation.Status)
	assert.Zero(t, obligation.AmountCents)
}

// Recolhido é recolhido: a baixa fecha o passivo e o desconto que o originou não
// pode mais ser removido por edição (estorno é o caminho).
func TestTaxObligation_SettlementClosesAndFreezes(t *testing.T) {
	repo := newTaxObligationFakeRepo()
	at := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	c := &commission.Commission{
		ID: "comm-1", TenantID: "tenant-1", Status: commission.StatusPending,
		Deductions: []commission.Deduction{{ID: "d1", Kind: commission.DeductionIRRF, Amount: 432, CreatedAt: at}},
	}
	svc := NewTaxObligationService(repo, &taxCommissionLookupFake{commission: c})
	require.NoError(t, svc.SyncCommission(context.Background(), "tenant-1", c.ID))
	obligation := repo.byKey[key(tax_obligation.KindIRRF, "2026-09")]
	require.NotNil(t, obligation)

	settled, err := svc.Pay(context.Background(), "tenant-1", obligation.ID, "operator-1", "https://storage/darf.pdf", "darf.pdf", "DARF 0561")
	require.NoError(t, err)
	assert.Equal(t, tax_obligation.StatusPaid, settled.Status)
	assert.NotNil(t, settled.PaidAt)

	// Remover o desconto depois do recolhimento é recusado.
	c.Deductions = nil
	err = svc.RemoveCommission(context.Background(), &commission.Commission{ID: c.ID, TenantID: "tenant-1", Deductions: []commission.Deduction{{ID: "d1", Kind: commission.DeductionIRRF, Amount: 432, CreatedAt: at}}}, "engano")
	assert.ErrorIs(t, err, tax_obligation.ErrAlreadySettled)
}

func TestTaxObligation_RecomputeRebuildsFromCommission(t *testing.T) {
	repo := newTaxObligationFakeRepo()
	at := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	c := &commission.Commission{
		ID: "comm-1", TenantID: "tenant-1", Status: commission.StatusPending,
		Deductions: []commission.Deduction{{ID: "d1", Kind: commission.DeductionINSS, Amount: 200, CreatedAt: at}},
	}
	svc := NewTaxObligationService(repo, &taxCommissionLookupFake{commission: c})
	processed, err := svc.Recompute(context.Background(), "tenant-1", "")
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NotNil(t, repo.byKey[key(tax_obligation.KindINSS, "2026-09")], "recomputação precisa reconstruir a obrigação")
}

// O beta pegou este buraco: a remoção do desconto passou depois do recolhimento
// (200 "Deduction removed successfully"), deixando a guia paga sem o desconto que
// a originou — razão contando imposto que não corresponde ao retido. A guarda
// vive no serviço da comissão, ANTES de gravar.
func TestTaxObligation_BlocksDeductionChangeAfterSettlement(t *testing.T) {
	repo := newTaxObligationFakeRepo()
	at := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	svc := NewTaxObligationService(repo, &taxCommissionLookupFake{})
	ctx := context.Background()

	// Nada recolhido ainda: a mudança passa.
	require.NoError(t, svc.EnsureDeductionChangeAllowed(ctx, "tenant-1", commission.DeductionIRRF, "2026-09"))

	obligation, err := tax_obligation.New("tenant-1", tax_obligation.KindIRRF, "2026-09", at)
	require.NoError(t, err)
	_, err = obligation.ApplyItem(tax_obligation.Item{CommissionID: "comm-1", DeductionID: "d1", AmountCents: 43200, Competence: "2026-09"}, at)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, obligation))

	// Aberta: ainda pode mexer no desconto.
	require.NoError(t, svc.EnsureDeductionChangeAllowed(ctx, "tenant-1", commission.DeductionIRRF, "2026-09"))

	// Recolhida: remover/relançar o desconto daquela competência é recusado.
	require.NoError(t, obligation.Pay("operator-1", "https://storage/darf.pdf", "darf.pdf", "", at))
	require.NoError(t, repo.Update(ctx, obligation, obligation.UpdatedAt))
	err = svc.EnsureDeductionChangeAllowed(ctx, "tenant-1", commission.DeductionIRRF, "2026-09")
	require.Error(t, err)
	assert.ErrorIs(t, err, tax_obligation.ErrAlreadySettled)
	assert.Contains(t, err.Error(), "estorne o recolhimento")

	// Outro tributo/competência segue livre.
	require.NoError(t, svc.EnsureDeductionChangeAllowed(ctx, "tenant-1", commission.DeductionISS, "2026-09"))
	require.NoError(t, svc.EnsureDeductionChangeAllowed(ctx, "tenant-1", commission.DeductionIRRF, "2026-10"))
}

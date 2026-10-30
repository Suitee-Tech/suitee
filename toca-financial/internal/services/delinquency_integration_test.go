package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// collectionRepoEspiao registra o que o motor pediu ao repositório de casos,
// para os testes de G4/G5 verificarem o efeito sem depender de banco.
type collectionRepoEspiao struct {
	*stubCollectionRepo
	criados    []*collection.Collection
	atualizado *collection.Collection
}

func (r *collectionRepoEspiao) Create(_ context.Context, c *collection.Collection) error {
	r.criados = append(r.criados, c)
	return nil
}

func (r *collectionRepoEspiao) CreateIdempotent(_ context.Context, c *collection.Collection) error {
	r.criados = append(r.criados, c)
	return nil
}

func (r *collectionRepoEspiao) Update(_ context.Context, c *collection.Collection) error {
	r.atualizado = c
	return nil
}

func (r *collectionRepoEspiao) UpdateIfVersion(_ context.Context, c *collection.Collection, _ time.Time) (bool, error) {
	r.atualizado = c
	return true, nil
}

// G5: dívida NOVA depois de um caso resolvido não pode ficar invisível.
//
// Antes, `ensureCollectionCase` retornava cedo para caso RESOLVED e a
// inadimplência recorrente (inquilino quita, atrasa de novo) nunca virava caso.
func TestEnsureCollectionCaseAbreCasoNovoParaDividaRecorrente(t *testing.T) {
	cobranca := makeChargeDue(t, "ch-recorrente", time.Now().AddDate(0, 0, -10), charge.StatusOverdue)
	resolvido := &collection.Collection{
		ID:         "coll-antigo",
		TenantID:   "t-1",
		ContractID: "c-1",
		Status:     collection.StatusResolved,
		ChargeIDs:  []string{"ch-antiga"},
	}
	repo := &collectionRepoEspiao{stubCollectionRepo: &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{"ch-recorrente": resolvido},
	}}
	svc := NewDelinquencyService(
		&staleDelinquencyChargeRepo{mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{cobranca}}},
		repo, nil, DefaultDelinquencyConfig())

	result := &ProcessResult{}
	if err := svc.ensureCollectionCase(context.Background(), cobranca, result); err != nil {
		t.Fatalf("ensureCollectionCase() erro = %v", err)
	}

	if len(repo.criados) != 1 {
		t.Fatalf("casos criados = %d, esperado 1 (dívida recorrente abre caso novo)", len(repo.criados))
	}
	novo := repo.criados[0]
	if novo.Status != collection.StatusActive {
		t.Fatalf("caso novo status = %s, esperado ACTIVE", novo.Status)
	}
	if len(novo.ChargeIDs) != 1 || novo.ChargeIDs[0] != cobranca.ID {
		t.Fatalf("charge_ids do caso novo = %v, esperado [%s]", novo.ChargeIDs, cobranca.ID)
	}
	if novo.SourceEventID == "" {
		t.Fatalf("caso criado pelo motor precisa de chave de idempotência (SourceEventID)")
	}
	if result.CollectionsCreated != 1 {
		t.Fatalf("result.CollectionsCreated = %d, esperado 1", result.CollectionsCreated)
	}
}

// Quando a cobrança JÁ pertence ao caso resolvido, é estado residual: o motor
// não cria caso novo nem revive o encerrado.
func TestEnsureCollectionCaseIgnoraResidualDeCasoResolvido(t *testing.T) {
	cobranca := makeChargeDue(t, "ch-residual", time.Now().AddDate(0, 0, -10), charge.StatusOverdue)
	resolvido := &collection.Collection{
		ID:         "coll-antigo",
		TenantID:   "t-1",
		ContractID: "c-1",
		Status:     collection.StatusResolved,
		ChargeIDs:  []string{"ch-residual"},
	}
	repo := &collectionRepoEspiao{stubCollectionRepo: &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{"ch-residual": resolvido},
	}}
	svc := NewDelinquencyService(
		&staleDelinquencyChargeRepo{mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{cobranca}}},
		repo, nil, DefaultDelinquencyConfig())

	if err := svc.ensureCollectionCase(context.Background(), cobranca, &ProcessResult{}); err != nil {
		t.Fatalf("ensureCollectionCase() erro = %v", err)
	}

	if len(repo.criados) != 0 {
		t.Fatalf("casos criados = %d, esperado 0 (cobrança já estava no caso encerrado)", len(repo.criados))
	}
}

// F0.6 (lado do motor): a descoberta é por ESTADO — o tick usa ListOverdue, que
// inclui cobranças já marcadas OVERDUE. Com a descoberta por transição, uma
// cobrança marcada OVERDUE com menos de 24h de atraso saía da lista e nunca mais
// recebia encargo.
func TestDelinquencyEngineDescobreCobrancaJaVencidaPorEstado(t *testing.T) {
	jaVencida := makeChargeDue(t, "ch-ja-overdue", time.Now().AddDate(0, 0, -3), charge.StatusOverdue)
	// staleDelinquencyChargeRepo existe só para dar tenant ao motor
	// (ListDistinctTenantIDs); a listagem em si é a do mock, que devolve as
	// cobranças e conta a chamada em overdueCalls.
	mock := &mockChargeRepo{charges: []*charge.Charge{jaVencida}}
	repo := &staleDelinquencyChargeRepo{mockChargeRepo: mock, tenantIDs: []string{"t-1"}}
	svc := NewDelinquencyService(repo, &stubCollectionRepo{}, nil, DefaultDelinquencyConfig())

	result, err := svc.ProcessOverdueCharges(context.Background())
	if err != nil {
		t.Fatalf("ProcessOverdueCharges() erro = %v", err)
	}

	if mock.overdueCalls == 0 {
		t.Fatalf("o tick precisa descobrir cobranças por estado (ListOverdue), overdueCalls = 0")
	}
	if result.LateFessCalculated == 0 {
		t.Fatalf("cobrança já OVERDUE precisa ter os encargos recalculados no tick")
	}
	if jaVencida.LateFee.Cents() <= 0 {
		t.Fatalf("multa não foi aplicada na cobrança já vencida (multa=%d)", jaVencida.LateFee.Cents())
	}
}

// G4: cancelar a cobrança precisa tirar a dívida do caso. O serviço de cobrança
// chama a reconciliação, que recalcula o caso a partir das cobranças restantes.
func TestResolveChargePaymentRemoveCobrancaCanceladaDoCaso(t *testing.T) {
	cancelada := makeChargeDue(t, "ch-cancelada", time.Now().AddDate(0, 0, -30), charge.StatusCancelled)
	emAberto := makeChargeDue(t, "ch-aberta", time.Now().AddDate(0, 0, -20), charge.StatusOverdue)
	caso := &collection.Collection{
		ID:          "coll-1",
		TenantID:    "t-1",
		ContractID:  "c-1",
		Status:      collection.StatusActive,
		TotalDebt:   3000,
		DaysOverdue: 30,
		ChargeIDs:   []string{cancelada.ID, emAberto.ID},
	}
	repo := &collectionRepoEspiao{stubCollectionRepo: &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{cancelada.ID: caso},
	}}
	svc := NewDelinquencyService(
		&mockChargeRepo{charges: []*charge.Charge{cancelada, emAberto}},
		repo, nil, DefaultDelinquencyConfig())

	if err := svc.ResolveChargePayment(context.Background(), "t-1", cancelada.ID); err != nil {
		t.Fatalf("ResolveChargePayment() erro = %v", err)
	}

	if repo.atualizado == nil {
		t.Fatalf("o caso deveria ter sido recalculado após o cancelamento")
	}
	if repo.atualizado.Status == collection.StatusResolved {
		t.Fatalf("caso não pode ser resolvido: ainda há cobrança em aberto")
	}
	esperado := emAberto.TotalAmount.Float()
	if diff := repo.atualizado.TotalDebt - esperado; diff > 0.01 || diff < -0.01 {
		t.Fatalf("dívida do caso = %.2f, esperado %.2f (só a cobrança em aberto)", repo.atualizado.TotalDebt, esperado)
	}
}

// O limiar de abertura de caso deixa de ser constante de código: a política da
// imobiliária manda quando existe, e o padrão da plataforma vale no resto.
func TestAutoCaseDaysForTenantUsaPoliticaDoTenant(t *testing.T) {
	svc := NewDelinquencyService(
		&staleDelinquencyChargeRepo{mockChargeRepo: &mockChargeRepo{}, tenantIDs: []string{"t-1"}},
		&stubCollectionRepo{}, nil, DefaultDelinquencyConfig())

	config := DefaultDelinquencyConfig()

	// Sem repositório de configurações injetado: padrão da plataforma.
	dias, origem := svc.AutoCaseDaysForTenant(context.Background(), "t-1")
	if dias != config.AutoCreateCollectionAfterDays || origem != "global" {
		t.Fatalf("sem política: dias=%d origem=%s, esperado %d/global", dias, origem, config.AutoCreateCollectionAfterDays)
	}

	// Com a política do tenant (3 dias), ela vence — e o cache não pode servir o
	// valor antigo para sempre.
	svc.SetTenantSettings(NewTenantFinancialSettingsService(&tenantSettingsStub{tenantID: "t-1", dias: 3}))
	svc.limiarMu.Lock()
	svc.limiarCache = nil
	svc.limiarMu.Unlock()

	dias, origem = svc.AutoCaseDaysForTenant(context.Background(), "t-1")
	if dias != 3 || origem != "tenant" {
		t.Fatalf("com política do tenant: dias=%d origem=%s, esperado 3/tenant", dias, origem)
	}

	// Outro tenant sem configuração continua no padrão da plataforma.
	dias, origem = svc.AutoCaseDaysForTenant(context.Background(), "t-2")
	if dias != config.AutoCreateCollectionAfterDays || origem != "global" {
		t.Fatalf("tenant sem política: dias=%d origem=%s", dias, origem)
	}
}

// tenantSettingsStub devolve o limiar só para o tenant configurado — os demais
// ficam sem política (0), como no banco quando não há linha de configuração.
type tenantSettingsStub struct {
	tenantID string
	dias     int
}

func (s *tenantSettingsStub) Get(_ context.Context, tenantID string) (*tenant_financial_settings.Settings, error) {
	if tenantID != s.tenantID {
		return &tenant_financial_settings.Settings{TenantID: tenantID}, nil
	}
	return &tenant_financial_settings.Settings{TenantID: tenantID, DelinquencyAutoCaseAfterDays: s.dias}, nil
}
func (s *tenantSettingsStub) Upsert(context.Context, *tenant_financial_settings.Settings) error {
	return nil
}

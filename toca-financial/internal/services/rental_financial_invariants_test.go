package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// Revisão das invariantes financeiras nos cenários de REAJUSTE e ADITIVO.
//
//	I1  valor já PAGO nunca é alterado
//	I2  valor já PAGO nunca é duplicado
//	I3  o que não está pago é replanejado (e o liquidado não é afetado)
//
// Estes testes existem porque as três falharam em produção de formas distintas:
// um aditivo gerou comissão e ordem de pagamento em dobro, e a guarda de
// idempotência não pegava o caso.

// ── Fake que enxerga o que foi tocado ────────────────────────────────

type revisaoChargeRepo struct {
	mu       sync.Mutex
	charges  []*charge.Charge
	atualiza []*charge.Charge
	criadas  []*charge.Charge
	existing map[string]bool
}

func (r *revisaoChargeRepo) Create(_ context.Context, c *charge.Charge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.existing == nil {
		r.existing = map[string]bool{}
	}
	if r.existing[c.SourceEventID] {
		return nil
	}
	r.existing[c.SourceEventID] = true
	r.criadas = append(r.criadas, c)
	return nil
}

func (r *revisaoChargeRepo) Update(_ context.Context, c *charge.Charge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.atualiza = append(r.atualiza, c)
	return nil
}

func (r *revisaoChargeRepo) List(_ context.Context, _, _ string, _, _ string, _, _ int64) ([]*charge.Charge, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.charges, int64(len(r.charges)), nil
}

func (r *revisaoChargeRepo) tocadas() []*charge.Charge {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.atualiza
}

func (r *revisaoChargeRepo) novas() []*charge.Charge {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.criadas
}

// chargeDe monta uma cobrança com o status pedido. `pagoEm` é o marco que não
// pode ser cruzado por nenhuma reescrita.
func chargeDe(id string, status charge.Status, mes string, dueDate time.Time, cents int64) *charge.Charge {
	c, err := charge.NewWithLineItems(
		"contrato-1", "locatario-1", "proprietario-1", "imovel-1", "tenant-1",
		charge.TypeRent, "Aluguel", mes, float64(cents)/100, dueDate, nil,
	)
	if err != nil {
		panic(err)
	}
	c.ID = id
	c.Status = status
	c.SourceEventID = "rental." + id
	return c
}

func configBase() *rental_charge_config.RentalChargeConfig {
	cfg, err := rental_charge_config.New(
		"tenant-1", "imovel-1", "contrato-1", "locatario-1", "proprietario-1",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
		rental_charge_config.SameMonthProRata, 10, 343000,
	)
	if err != nil {
		panic(err)
	}
	return cfg
}

// I1 + I3: o reajuste replaneja o que está em aberto e NÃO toca o que já foi
// pago. Um pagamento é um fato consumado: reescrever o valor depois de recebido
// quebraria a conciliação e o repasse ao proprietário.
func TestReajuste_NaoTocaCobrancaPaga_E_ReplanejaAberta(t *testing.T) {
	paga := chargeDe("paga", charge.StatusPaid, "2026-02", time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), 343000)
	aberta := chargeDe("aberta", charge.StatusPending, "2026-08", time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), 343000)
	parcial := chargeDe("parcial", charge.StatusPartiallyPaid, "2026-07", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), 343000)
	vencida := chargeDe("vencida", charge.StatusOverdue, "2026-06", time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), 343000)

	repo := &revisaoChargeRepo{
		charges: []*charge.Charge{vencida, parcial, paga, aberta},
	}
	cfgRepo := &readjustmentConfigRepo{cfg: configBase()}
	svc := NewRentalChargeConfigService(cfgRepo, repo, nil)

	vigencia := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	novoMensal := int64(358435)
	if _, _, err := svc.ApplyReadjustment(context.Background(), "tenant-1", "contrato-1", "evt-1", novoMensal, vigencia, time.Now().UTC()); err != nil {
		t.Fatalf("reajuste: %v", err)
	}

	// I1: nenhuma cobrança paga pode aparecer entre as atualizadas.
	for _, c := range repo.tocadas() {
		if c.Status == charge.StatusPaid {
			t.Errorf("cobrança PAGA %s foi reescrita pelo reajuste — valor liquidado não pode ser alterado", c.ID)
		}
	}

	// I3: a aberta foi replanejada com o novo valor.
	replanejadas := map[string]bool{}
	for _, c := range repo.tocadas() {
		replanejadas[c.ID] = true
	}
	if !replanejadas["aberta"] {
		t.Error("a cobrança em aberto deveria ter sido replanejada com o novo valor")
	}

	// Parcial e vencida ficam intocadas: são posições jurídicas em curso, e
	// reescrever o valor de uma cobrança parcialmente paga mudaria a dívida sem
	// decisão da operação.
	if replanejadas["parcial"] {
		t.Error("cobrança PARCIALMENTE paga foi reescrita; deve exigir decisão da operação")
	}
	if replanejadas["vencida"] {
		t.Error("cobrança VENCIDA foi reescrita; deve exigir decisão da operação")
	}
}

// I2: reentregar o evento de reajuste não pode criar segunda cobrança para a
// mesma competência — o mesmo plano, aplicado duas vezes, não pode virar duas
// dívidas.
func TestReajuste_ReentregaNaoDuplicaCobranca(t *testing.T) {
	repo := &revisaoChargeRepo{}
	cfgRepo := &readjustmentConfigRepo{cfg: configBase()}
	svc := NewRentalChargeConfigService(cfgRepo, repo, nil)

	vigencia := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := svc.ApplyReadjustment(context.Background(), "tenant-1", "contrato-1", "evt-1", 358435, vigencia, time.Now().UTC()); err != nil {
		t.Fatalf("primeiro reajuste: %v", err)
	}
	primeira := len(repo.novas())

	// Mesmo evento, reentrega.
	if _, _, err := svc.ApplyReadjustment(context.Background(), "tenant-1", "contrato-1", "evt-1", 358435, vigencia, time.Now().UTC()); err != nil {
		t.Fatalf("reentrega: %v", err)
	}
	if got := len(repo.novas()); got != primeira {
		t.Errorf("reentrega criou %d cobranças a mais (antes %d, depois %d)", got-primeira, primeira, got)
	}
}

// I3: o valor a pagar segue o cronograma depois da vigência e o valor original
// antes dela. O reajuste não retroage.
func TestReajuste_NaoRetroage(t *testing.T) {
	cfg := configBase()
	svc := NewRentalChargeConfigService(&readjustmentConfigRepo{cfg: cfg}, &revisaoChargeRepo{}, nil)

	vigencia := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := svc.ApplyReadjustment(context.Background(), "tenant-1", "contrato-1", "evt-1", 358435, vigencia, time.Now().UTC()); err != nil {
		t.Fatalf("reajuste: %v", err)
	}

	if got := cfg.MonthlyAmountFor(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)); got != 343000 {
		t.Errorf("mês anterior à vigência = %d, want 343000 (o reajuste não pode retroagir)", got)
	}
	if got := cfg.MonthlyAmountFor(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)); got != 358435 {
		t.Errorf("mês posterior à vigência = %d, want 358435", got)
	}
}

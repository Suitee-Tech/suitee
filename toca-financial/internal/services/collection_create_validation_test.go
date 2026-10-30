package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
)

// chargeRepoPorIDs implementa o mínimo usado pela validação de criação:
// `ListByIDs` devolve apenas as cobranças conhecidas do tenant informado, o que
// é exatamente como o repositório real se comporta (filtro por tenant na
// query). As demais leituras ficam indisponíveis de propósito — se a validação
// passar a depender de outra, o teste falha em vez de passar por acidente.
type chargeRepoPorIDs struct {
	*mockChargeRepo
	porTenant map[string][]*charge.Charge
	erro      error
}

func (r *chargeRepoPorIDs) ListByIDs(_ context.Context, tenantID string, ids []string) ([]*charge.Charge, error) {
	if r.erro != nil {
		return nil, r.erro
	}
	queridas := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		queridas[id] = struct{}{}
	}
	out := make([]*charge.Charge, 0, len(ids))
	for _, c := range r.porTenant[tenantID] {
		if _, ok := queridas[c.ID]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func cobrancaVencida(t *testing.T, id string, diasAtras int, status charge.Status) *charge.Charge {
	t.Helper()
	c := makeChargeDue(t, id, time.Now().AddDate(0, 0, -diasAtras), status)
	return c
}

func TestCollectionCreateRejeitaCobrancaInexistente(t *testing.T) {
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 1500, 10, []string{"ch-fantasma"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
}

func TestCollectionCreateRejeitaCobrancaDeOutroTenant(t *testing.T) {
	// A cobrança existe, mas no tenant "t-2": para "t-1" ela simplesmente não
	// volta da query, então o caso não pode ser aberto.
	alheia := cobrancaVencida(t, "ch-outro", 10, charge.StatusOverdue)
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-2": {alheia}}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 1500, 10, []string{"ch-outro"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
}

func TestCollectionCreateRejeitaCobrancaPaga(t *testing.T) {
	paga := cobrancaVencida(t, "ch-paga", 10, charge.StatusPaid)
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {paga}}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 1500, 10, []string{"ch-paga"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
	if !strings.Contains(err.Error(), "PAID") {
		t.Fatalf("mensagem deveria citar o status atual: %v", err)
	}
}

func TestCollectionCreateRejeitaCobrancaNaoVencida(t *testing.T) {
	futura := makeChargeDue(t, "ch-futura", time.Now().AddDate(0, 0, 5), charge.StatusPending)
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {futura}}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 1500, 10, []string{"ch-futura"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
}

func TestCollectionCreateRejeitaDividaDivergenteDaSoma(t *testing.T) {
	c := cobrancaVencida(t, "ch-1", 10, charge.StatusOverdue)
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {c}}})

	// A cobrança vale 1500 (makeChargeDue) e o caso declara 9000.
	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 9000, 10, []string{"ch-1"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
	if !strings.Contains(err.Error(), "difere da soma") {
		t.Fatalf("mensagem deveria explicar a divergência: %v", err)
	}
}

func TestCollectionCreateRejeitaCobrancaRepetida(t *testing.T) {
	c := cobrancaVencida(t, "ch-1", 10, charge.StatusOverdue)
	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {c}}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", c.TotalAmount.Float()*2, 10, []string{"ch-1", "ch-1"})

	if !errors.Is(err, ErrInvalidCollectionInput) {
		t.Fatalf("erro = %v, esperado ErrInvalidCollectionInput", err)
	}
}

func TestCollectionCreateAceitaCobrancasValidasEParcial(t *testing.T) {
	cheia := cobrancaVencida(t, "ch-cheia", 10, charge.StatusOverdue)
	parcial := cobrancaVencida(t, "ch-parcial", 20, charge.StatusPartiallyPaid)
	if _, err := parcial.ApplyPayment("pay-1", 500, "PIX", time.Now()); err != nil {
		t.Fatalf("ApplyPayment: %v", err)
	}
	soma := cheia.TotalAmount.Float() + (parcial.TotalAmount.Float() - parcial.PaidAmount.Float())

	svc := NewCollectionService(&stubCollectionRepo{}).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {cheia, parcial}}})

	caso, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", soma, 20, []string{"ch-cheia", "ch-parcial"})

	if err != nil {
		t.Fatalf("Create() erro = %v, esperado sucesso", err)
	}
	if caso == nil || caso.Status != collection.StatusActive {
		t.Fatalf("caso criado = %+v, esperado ACTIVE", caso)
	}
	if len(caso.ChargeIDs) != 2 {
		t.Fatalf("charge_ids = %v, esperado 2", caso.ChargeIDs)
	}
}

// Sem repositório de cobranças injetado a validação é pulada — o router de
// produção sempre injeta, e este teste fixa essa decisão para o caso de alguém
// remover o wiring sem perceber.
func TestCollectionCreateSemChargeRepoNaoValida(t *testing.T) {
	svc := NewCollectionService(&stubCollectionRepo{})

	if _, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", 1234, 10, []string{"qualquer"}); err != nil {
		t.Fatalf("sem chargeRepo a criação não deveria validar: %v", err)
	}
}

// Abrir caso manual para cobrança que já está num caso aberto duplicaria a
// dívida: o motor passaria a atualizar dois casos para a mesma cobrança.
func TestCollectionCreateRejeitaCobrancaJaEmCasoAberto(t *testing.T) {
	c := cobrancaVencida(t, "ch-1", 10, charge.StatusOverdue)
	existente := &collection.Collection{
		ID: "coll-existente", TenantID: "t-1", Status: collection.StatusActive,
		ChargeIDs: []string{c.ID},
	}
	repo := &casoRepoMemoria{stubCollectionRepo: &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{c.ID: existente},
	}, caso: existente}
	svc := NewCollectionService(repo).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {c}}})

	_, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", c.TotalAmount.Float(), 10, []string{c.ID})

	if !errors.Is(err, ErrChargeAlreadyInCase) {
		t.Fatalf("erro = %v, esperado ErrChargeAlreadyInCase", err)
	}
	if !strings.Contains(err.Error(), "coll-existente") {
		t.Fatalf("a mensagem deve dizer em qual caso a cobrança está: %v", err)
	}
}

// Caso RESOLVIDO não bloqueia: a dívida recorrente precisa poder virar caso novo.
func TestCollectionCreateAceitaCobrancaDeCasoResolvido(t *testing.T) {
	c := cobrancaVencida(t, "ch-1", 10, charge.StatusOverdue)
	resolvido := &collection.Collection{
		ID: "coll-antigo", TenantID: "t-1", Status: collection.StatusResolved,
		ChargeIDs: []string{c.ID},
	}
	repo := &casoRepoMemoria{stubCollectionRepo: &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{c.ID: resolvido},
	}, caso: resolvido}
	svc := NewCollectionService(repo).
		WithChargeRepository(&chargeRepoPorIDs{porTenant: map[string][]*charge.Charge{"t-1": {c}}})

	if _, err := svc.Create(context.Background(), "c-1", "d-1", "Devedor", "t-1", c.TotalAmount.Float(), 10, []string{c.ID}); err != nil {
		t.Fatalf("caso resolvido não pode bloquear novo caso: %v", err)
	}
}

// Caso resolvido pelo PUT da tela precisa liberar a retenção de repasse do
// contrato: o `PUT /collections/:id` usa UpdateDetails (não UpdateStatus), e a
// validação no beta mostrou o dinheiro do proprietário ficando preso porque o
// gancho existia só no outro caminho.
func TestUpdateDetailsResolvidoLiberaRetencaoDeRepasse(t *testing.T) {
	caso := &collection.Collection{
		ID: "coll-1", TenantID: "t-1", ContractID: "ctr-1",
		Status: collection.StatusActive, UpdatedAt: time.Now(),
	}
	repo := &casoRepoMemoria{stubCollectionRepo: &stubCollectionRepo{}, caso: caso}

	type liberacao struct{ tenantID, contractID, motivo string }
	var liberacoes []liberacao
	svc := NewCollectionService(repo).WithTransferHoldReleaser(
		func(_ context.Context, tenantID, contractID, reason string) (int, error) {
			liberacoes = append(liberacoes, liberacao{tenantID, contractID, reason})
			return 1, nil
		})

	if _, err := svc.UpdateDetails(context.Background(), caso, collection.StatusResolved, "operador"); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if len(liberacoes) != 1 {
		t.Fatalf("liberações = %d, esperado 1 (caso resolvido libera a retenção)", len(liberacoes))
	}
	if liberacoes[0].tenantID != "t-1" || liberacoes[0].contractID != "ctr-1" {
		t.Fatalf("liberação com alvo errado: %+v", liberacoes[0])
	}

	// Mudança de status que NÃO encerra o caso não libera nada.
	liberacoes = nil
	caso.Status = collection.StatusActive
	caso.UpdatedAt = time.Now()
	if _, err := svc.UpdateDetails(context.Background(), caso, collection.StatusNegotiated, "operador"); err != nil {
		t.Fatalf("UpdateDetails (negociado): %v", err)
	}
	if len(liberacoes) != 0 {
		t.Fatalf("acordo em andamento não pode liberar retenção: %+v", liberacoes)
	}
}

package services_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// P0 / C12 — assinatura de contrato: identidade do signatário.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado C12) mostrou que `POST /contracts/:id/sign` assinava o signatário cujo
// `signatory_id` viesse no CORPO, sem confrontar o chamador, aceitando qualquer
// string como `signature`. Qualquer usuário com `contracts:write` assinava em
// nome do proprietário, do comprador ou do banco — e o evento
// `contract.signed` dispara efeitos financeiros.
//
// O fluxo legítimo (toka/e2e/contract-financial.spec.ts) cria o signatário com
// `id = accountId` e `email` do próprio usuário; por isso o vínculo aceito é
// account id OU e-mail, exigindo um dos dois.

func c12Contract() *contract.Contract {
	return &contract.Contract{
		ID:        "contract-c12",
		DealID:    "deal-c12",
		TenantID:  "tenant-1",
		Status:    contract.StatusPending,
		Type:      contract.TypeVenda,
		StartDate: time.Now(),
		Signatories: []contract.Signatory{
			{ID: "signer-da-vitima", Name: "Maria Compradora", Email: "maria@example.com", Role: contract.RoleComprador},
		},
	}
}

func c12Service(c *contract.Contract, bus *recordingBus) *service.ContractService {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-c12"
	return service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, bus, nil)
}

// TestSignContract_RejectsForeignSignatory é o teste central do C12: um
// chamador que NÃO é o signatário-alvo não pode assiná-lo.
func TestSignContract_RejectsForeignSignatory(t *testing.T) {
	c := c12Contract()
	bus := &recordingBus{}
	svc := c12Service(c, bus)

	attacker := service.SignerIdentity{AccountID: "conta-do-atacante", Email: "atacante@example.com"}
	_, err := svc.SignContract(context.Background(), "tenant-1", c.ID, "signer-da-vitima", "assinatura-forjada", attacker)

	if err == nil {
		t.Fatal("ASSINATURA FORJADA ACEITA: chamador que não é o signatário assinou o contrato")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("esperava erro de autorização, veio %v", err)
	}
	if c.Status == contract.StatusSigned {
		t.Fatal("contrato marcado como SIGNED por chamador não autorizado")
	}
	if c.Signatories[0].Signature != "" || c.Signatories[0].SignedAt != nil {
		t.Fatalf("signatário da vítima foi marcado como assinado: %+v", c.Signatories[0])
	}
	if len(bus.published) != 0 {
		t.Fatalf("evento publicado por assinatura não autorizada: %d", len(bus.published))
	}
}

// TestSignContract_RejectsEmptyIdentity cobre o chamador sem identidade
// utilizável (token sem e-mail/account id): falha fechado.
func TestSignContract_RejectsEmptyIdentity(t *testing.T) {
	c := c12Contract()
	svc := c12Service(c, &recordingBus{})

	if _, err := svc.SignContract(context.Background(), "tenant-1", c.ID, "signer-da-vitima", "x", service.SignerIdentity{}); err == nil {
		t.Fatal("identidade vazia deveria ser recusada (fail-closed)")
	}
}

// TestSignContract_AcceptsSignatoryByAccountID preserva o fluxo legítimo do
// e2e (signatário criado com id = accountId do usuário).
func TestSignContract_AcceptsSignatoryByAccountID(t *testing.T) {
	c := c12Contract()
	bus := &recordingBus{}
	svc := c12Service(c, bus)

	signer := service.SignerIdentity{AccountID: "signer-da-vitima"}
	if _, err := svc.SignContract(context.Background(), "tenant-1", c.ID, "signer-da-vitima", "assinatura-real", signer); err != nil {
		t.Fatalf("assinatura legítima por account id falhou: %v", err)
	}
	if c.Signatories[0].Signature != "assinatura-real" || c.Signatories[0].SignedAt == nil {
		t.Fatal("assinatura legítima não foi registrada")
	}
}

// TestSignContract_AcceptsSignatoryByEmail cobre o vínculo por e-mail
// (case-insensitive), que é o identificador natural do signatário.
func TestSignContract_AcceptsSignatoryByEmail(t *testing.T) {
	c := c12Contract()
	svc := c12Service(c, &recordingBus{})

	signer := service.SignerIdentity{AccountID: "outra-conta-qualquer", Email: "MARIA@example.com"}
	if _, err := svc.SignContract(context.Background(), "tenant-1", c.ID, "signer-da-vitima", "ok", signer); err != nil {
		t.Fatalf("assinatura legítima por e-mail falhou: %v", err)
	}
	if c.Signatories[0].SignedAt == nil {
		t.Fatal("assinatura por e-mail não registrada")
	}
}

// TestSignContract_RetryStillRequiresIdentity garante que o caminho
// idempotente (contrato já SIGNED + retry) não vire um bypass da checagem.
func TestSignContract_RetryStillRequiresIdentity(t *testing.T) {
	c := c12Contract()
	signed := time.Now()
	c.Status = contract.StatusSigned
	c.Signatories[0].SignedAt = &signed
	svc := c12Service(c, &recordingBus{})

	attacker := service.SignerIdentity{AccountID: "conta-do-atacante", Email: "atacante@example.com"}
	if _, err := svc.SignContract(context.Background(), "tenant-1", c.ID, "signer-da-vitima", "x", attacker); err == nil {
		t.Fatal("retry idempotente aceitou chamador que não é o signatário")
	}
}

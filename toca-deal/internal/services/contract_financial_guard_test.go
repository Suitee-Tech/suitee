package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// T-01: o guard de plano financeiro SALE é server-side — um contrato de
// venda só é gerado/enviado com draft de recebimento ativo e total igual ao
// valor do negócio. Sem draft (ou com divergência), nem o Create nem o
// SendForSignature passam; o contract.signed nunca fabricaria o fallback à
// vista.

func saleDeal(t *testing.T, value float64) *deal.Deal {
	t.Helper()
	d, err := deal.New(deal.TypeSale, "prop-1", "owner-1", "cust-1", "broker-1", value, "tenant-1")
	if err != nil {
		t.Fatalf("setup: deal.New: %v", err)
	}
	return d
}

func saleCreateRequest(dealID string) *contract.CreateContractRequest {
	return &contract.CreateContractRequest{
		DealID: dealID,
		Type:   contract.TypeVenda,
		Signatories: []contract.Signatory{
			{Role: contract.RoleComprador},
			{Role: contract.RoleVendedor},
		},
		StartDate: time.Now(),
	}
}

func TestCreateContract_SALEWithoutDraftIsRefused(t *testing.T) {
	d := saleDeal(t, 1000)
	svc := service.NewContractService(&mockContractRepo{}, &mockDealRepo{d: d}, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: false})

	_, err := svc.CreateContract(context.Background(), "tenant-1", saleCreateRequest(d.ID))
	if !errors.Is(err, service.ErrSalePlanNotReady) {
		t.Fatalf("esperava ErrSalePlanNotReady, veio %v", err)
	}
}

func TestCreateContract_SALEMismatchedDraftIsRefused(t *testing.T) {
	d := saleDeal(t, 1000) // 100000 centavos
	svc := service.NewContractService(&mockContractRepo{}, &mockDealRepo{d: d}, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 99999})

	_, err := svc.CreateContract(context.Background(), "tenant-1", saleCreateRequest(d.ID))
	if !errors.Is(err, service.ErrSalePlanNotReady) {
		t.Fatalf("esperava ErrSalePlanNotReady, veio %v", err)
	}
}

func TestCreateContract_SALEReadyPlanProceeds(t *testing.T) {
	d := saleDeal(t, 1000)
	svc := service.NewContractService(&mockContractRepo{}, &mockDealRepo{d: d}, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 100000})

	created, err := svc.CreateContract(context.Background(), "tenant-1", saleCreateRequest(d.ID))
	if err != nil {
		t.Fatalf("plano pronto não deveria recusar: %v", err)
	}
	if created == nil {
		t.Fatal("contrato não criado")
	}
}

func TestCreateContract_SALEWhenFinancialIsUnavailableFailsClosed(t *testing.T) {
	d := saleDeal(t, 1000)
	svc := service.NewContractService(&mockContractRepo{}, &mockDealRepo{d: d}, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{err: errors.New("connection refused")})

	_, err := svc.CreateContract(context.Background(), "tenant-1", saleCreateRequest(d.ID))
	if err == nil {
		t.Fatal("indisponibilidade do financeiro deveria falhar fechado")
	}
}

func TestCreateContract_RENTDoesNotRequirePlan(t *testing.T) {
	d, err := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	if err != nil {
		t.Fatalf("setup: deal.New: %v", err)
	}
	svc := service.NewContractService(&mockContractRepo{}, &mockDealRepo{d: d}, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{err: errors.New("should not be called")})

	req := &contract.CreateContractRequest{
		DealID: d.ID,
		Type:   contract.TypeLocacao,
		Signatories: []contract.Signatory{
			{Role: contract.RoleInquilino},
			{Role: contract.RoleProprietario},
		},
		StartDate: time.Now(),
	}
	if _, err := svc.CreateContract(context.Background(), "tenant-1", req); err != nil {
		t.Fatalf("locação não deveria exigir plano SALE: %v", err)
	}
}

func saleContract() *contract.Contract {
	return &contract.Contract{
		ID:        "contract-1",
		DealID:    "deal-1",
		TenantID:  "tenant-1",
		Status:    contract.StatusDraft,
		Type:      contract.TypeVenda,
		StartDate: time.Now(),
		Signatories: []contract.Signatory{
			{ID: "sig-1", Name: "Comprador", Email: "c@x.com", Role: contract.RoleComprador},
			{ID: "sig-2", Name: "Vendedor", Email: "v@x.com", Role: contract.RoleVendedor},
		},
	}
}

type recordingEventBus struct {
	published events.Event
}

func (r *recordingEventBus) Publish(ctx context.Context, event events.Event) error {
	r.published = event
	return nil
}
func (r *recordingEventBus) PublishReliable(ctx context.Context, event events.Event) error {
	r.published = event
	return nil
}
func (r *recordingEventBus) Subscribe(topic string, handler events.EventHandler) error { return nil }
func (r *recordingEventBus) SubscribeWithQueue(queueName, eventName string, handler events.EventHandler) error {
	return nil
}

func TestSendForSignature_SALEWithoutDraftIsRefused(t *testing.T) {
	d := saleDeal(t, 1000)
	d.ID = "deal-1"
	bus := &recordingEventBus{}
	svc := service.NewContractService(&mockContractRepo{c: saleContract()}, &mockDealRepo{d: d}, bus, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: false})

	_, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if !errors.Is(err, service.ErrSalePlanNotReady) {
		t.Fatalf("esperava ErrSalePlanNotReady, veio %v", err)
	}
	if bus.published != nil {
		t.Fatal("nenhum evento deveria ser publicado com o envio recusado")
	}
}

func TestSendForSignature_SALEWithMismatchedDraftIsRefused(t *testing.T) {
	d := saleDeal(t, 1000)
	d.ID = "deal-1"
	bus := &recordingEventBus{}
	svc := service.NewContractService(&mockContractRepo{c: saleContract()}, &mockDealRepo{d: d}, bus, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 200000})

	_, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if !errors.Is(err, service.ErrSalePlanNotReady) {
		t.Fatalf("esperava ErrSalePlanNotReady, veio %v", err)
	}
	if bus.published != nil {
		t.Fatal("nenhum evento deveria ser publicado com o envio recusado")
	}
}

func TestSendForSignature_SALEReadyPlanProceeds(t *testing.T) {
	d := saleDeal(t, 1000)
	d.ID = "deal-1"
	bus := &recordingEventBus{}
	repo := &mockContractRepo{c: saleContract()}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 100000})

	out, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if err != nil {
		t.Fatalf("plano pronto não deveria recusar: %v", err)
	}
	if out.Status != contract.StatusPending {
		t.Fatalf("status = %s, esperava PENDING_SIGNATURE", out.Status)
	}
	if bus.published == nil {
		t.Fatal("signature.requested deveria ter sido publicado")
	}
}

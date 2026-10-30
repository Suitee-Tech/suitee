package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// fakePlanChecker devolve a readiness do plano SALE de forma controlada
// (T-01): sem draft (active=false), com total divergente ou indisponível.
type fakePlanChecker struct {
	active bool
	total  int64
	err    error
}

func (f *fakePlanChecker) GetSalePlanReadiness(ctx context.Context, tenantID, dealID string) (*clients.SalePlanReadiness, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &clients.SalePlanReadiness{Active: f.active, TotalAmountCents: f.total, Mode: "INSTALLMENTS"}, nil
}

// Mock repos
type mockDealRepo struct {
	d *deal.Deal
}

func (m *mockDealRepo) Create(ctx context.Context, d *deal.Deal) error { return nil }
func (m *mockDealRepo) GetByID(ctx context.Context, tenantID, id string) (*deal.Deal, error) {
	return m.d, nil
}
func (m *mockDealRepo) GetByIDInTx(ctx context.Context, _ *sqlx.Tx, tenantID, id string) (*deal.Deal, error) {
	return m.d, nil
}
func (m *mockDealRepo) Update(ctx context.Context, d *deal.Deal) error { return nil }
func (m *mockDealRepo) UpdateInTx(ctx context.Context, _ *sqlx.Tx, d *deal.Deal) error {
	return m.Update(ctx, d)
}
func (m *mockDealRepo) DeleteScoped(ctx context.Context, tenantID, id, accountID string) error {
	return nil
}
func (m *mockDealRepo) Delete(ctx context.Context, tenantID, id string) error { return nil }
func (m *mockDealRepo) List(ctx context.Context, tenantID string, skip, limit int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (m *mockDealRepo) FindByPartyAccountID(ctx context.Context, accountID string) ([]*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) FindByPartyEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) FindByPartyLeadID(ctx context.Context, leadID string) ([]*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) FindBySignerEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) FindBySignerIdentity(ctx context.Context, document, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) GetByIDScoped(ctx context.Context, tenantID, id, accountID string) (*deal.Deal, error) {
	return nil, nil
}
func (m *mockDealRepo) ListByCustomerIDs(ctx context.Context, tenantID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (m *mockDealRepo) ListByCustomerIDsScoped(ctx context.Context, tenantID, accountID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (m *mockDealRepo) ListScoped(ctx context.Context, tenantID, accountID string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (m *mockDealRepo) GetStats(ctx context.Context, tenantID string) (int64, float64, error) {
	return 0, 0, nil
}
func (m *mockDealRepo) GetStatsScoped(ctx context.Context, tenantID, accountID string) (int64, float64, error) {
	return 0, 0, nil
}
func (m *mockDealRepo) Search(ctx context.Context, tenantID string, filter deal.DealFilter) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}

type mockContractRepo struct {
	c *contract.Contract
	// Reproduz a falha de gravação quando o contexto está cancelado.
	failUpdateWhenCanceled bool
	// Chamado depois de um GetByID bem-sucedido: permite cancelar o contexto no
	// meio da operação, como acontece num timeout de provedor.
	afterGet func()
	// Último estado efetivamente gravado com sucesso.
	ultimoPersistido contract.Status
	ultimoLock       *time.Time
}

func (m *mockContractRepo) Create(ctx context.Context, c *contract.Contract) error {
	m.c = c
	return nil
}
func (m *mockContractRepo) GetByID(ctx context.Context, tenantID, id string) (*contract.Contract, error) {
	if m.afterGet != nil {
		defer m.afterGet()
	}
	return m.c, nil
}
func (m *mockContractRepo) GetByDealID(ctx context.Context, tenantID, dealID string) ([]*contract.Contract, error) {
	return nil, nil
}
func (m *mockContractRepo) Update(ctx context.Context, c *contract.Contract) error {
	// O repositório real falha com contexto cancelado. Sem isso o mock gravava
	// sempre, e o teste de recuperação passava mesmo com o defeito presente.
	if m.failUpdateWhenCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	m.c = c
	// Guarda o que foi de fato PERSISTIDO. `m.c` é o mesmo ponteiro que o
	// serviço muta antes de gravar, então ele reflete a intenção mesmo quando a
	// gravação é recusada — usar `m.c` numa asserção dava falso positivo.
	m.ultimoPersistido = c.Status
	m.ultimoLock = c.SignatureLockedAt
	return nil
}
func (m *mockContractRepo) UpdateInTx(ctx context.Context, _ *sqlx.Tx, c *contract.Contract) error {
	return m.Update(ctx, c)
}
func (m *mockContractRepo) Delete(ctx context.Context, id, tenantID string) error { return nil }
func (m *mockContractRepo) List(ctx context.Context, tenantID string, status *contract.Status) ([]*contract.Contract, error) {
	return nil, nil
}
func (m *mockContractRepo) ListByPropertyID(ctx context.Context, propertyID string) ([]*contract.Contract, error) {
	if m.c != nil && m.c.PropertyID == propertyID {
		return []*contract.Contract{m.c}, nil
	}
	return nil, nil
}
func (m *mockContractRepo) ListAutoAdjustDue(ctx context.Context, today time.Time) ([]*contract.Contract, error) {
	return nil, nil
}
func (m *mockContractRepo) ListMissingCRMSnapshot(ctx context.Context, tenantID string) ([]*contract.Contract, error) {
	return nil, nil
}

type mockEventBus struct{}

func (m *mockEventBus) Publish(ctx context.Context, event events.Event) error         { return nil }
func (m *mockEventBus) PublishReliable(ctx context.Context, event events.Event) error { return nil }
func (m *mockEventBus) Subscribe(topic string, handler events.EventHandler) error     { return nil }
func (m *mockEventBus) SubscribeWithQueue(queueName, eventName string, handler events.EventHandler) error {
	return nil
}

func TestCreateContract_Validation(t *testing.T) {
	d, _ := deal.New(deal.TypeSale, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")

	dealRepo := &mockDealRepo{d: d}
	contractRepo := &mockContractRepo{}
	svc := service.NewContractService(contractRepo, dealRepo, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 100000})

	// Try to create contract with invalid signatory
	req := &contract.CreateContractRequest{
		DealID: d.ID,
		Type:   contract.TypeVenda,
		Signatories: []contract.Signatory{
			{Role: contract.RoleInquilino},
		},
		StartDate: time.Now(),
	}

	_, err := svc.CreateContract(context.Background(), "tenant-1", req)
	if err == nil {
		t.Errorf("Expected error due to invalid signatory role, got nil")
	}

	// Try with valid signatory
	req.Signatories = []contract.Signatory{
		{Role: contract.RoleComprador},
	}
	_, err = svc.CreateContract(context.Background(), "tenant-1", req)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCreateContract_AcceptsSignersGeneratedFromSaleParties(t *testing.T) {
	d, err := deal.New(deal.TypeSale, "prop-1", "owner-1", "cust-1", "broker-1", 3950000, "tenant-1")
	if err != nil {
		t.Fatalf("setup: deal.New: %v", err)
	}

	dealRepo := &mockDealRepo{d: d}
	contractRepo := &mockContractRepo{}
	svc := service.NewContractService(contractRepo, dealRepo, &mockEventBus{}, nil).
		WithFinancialPlanChecker(&fakePlanChecker{active: true, total: 395000000})

	req := &contract.CreateContractRequest{
		DealID: d.ID,
		Type:   contract.TypeVenda,
		Signatories: []contract.Signatory{
			{Role: contract.RoleComprador},
			{Role: contract.RoleVendedor},
			{Role: contract.RoleConjuge},
		},
		StartDate: time.Now(),
	}

	created, err := svc.CreateContract(context.Background(), "tenant-1", req)
	if err != nil {
		t.Fatalf("CreateContract() error = %v", err)
	}
	if created == nil || len(created.Signatories) != 3 {
		t.Fatalf("expected contract with 3 signatories, got %+v", created)
	}
	if created.Signatories[2].Role != contract.RoleConjuge {
		t.Fatalf("spouse role was not preserved: %+v", created.Signatories[2])
	}
}

func TestCreateContract_PopulatesCRMFieldsFromDeal(t *testing.T) {
	d, err := deal.New(deal.TypeRent, "prop-42", "owner-1", "lead-1", "broker-1", 2500, "tenant-1")
	if err != nil {
		t.Fatalf("setup: deal.New: %v", err)
	}
	d.Commission = 8.0
	if err := d.AddParty(deal.Party{
		ID:        "party-1",
		Role:      deal.PartyRoleTenant,
		Name:      "Maria Inquilina",
		LeadID:    "lead-1",
		Email:     "maria@example.com",
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("setup: AddParty: %v", err)
	}

	dealRepo := &mockDealRepo{d: d}
	contractRepo := &mockContractRepo{}
	svc := service.NewContractService(contractRepo, dealRepo, &mockEventBus{}, nil)

	req := &contract.CreateContractRequest{
		DealID:        d.ID,
		Type:          contract.TypeLocacao,
		Signatories:   []contract.Signatory{{Role: contract.RoleInquilino, Name: "Maria Inquilina", Email: "maria@example.com"}},
		StartDate:     time.Now(),
		PropertyTitle: "Apto Vila Mariana",
		// Frontend used to send client=customer_id (UUID). Backend must
		// ignore that and derive from the Deal — verified by leaving
		// Client unset in the request below.
		AdjustIndex: "IPCA",
		AutoAdjust:  true,
	}

	created, err := svc.CreateContract(context.Background(), "tenant-1", req)
	if err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	if created.Client != "Maria Inquilina" {
		t.Errorf("Client: got %q, want %q (derived from Deal primary tenant party)", created.Client, "Maria Inquilina")
	}
	if created.CustomerID != "lead-1" {
		t.Errorf("CustomerID: got %q, want %q (from Deal.CustomerID)", created.CustomerID, "lead-1")
	}
	if created.PropertyID != "prop-42" {
		t.Errorf("PropertyID: got %q, want %q (from Deal.PropertyID)", created.PropertyID, "prop-42")
	}
	if created.Value != 2500 {
		t.Errorf("Value: got %v, want 2500 (from Deal.Value)", created.Value)
	}
	if created.CommissionPct != 8.0 {
		t.Errorf("CommissionPct: got %v, want 8.0 (from Deal.Commission)", created.CommissionPct)
	}
	if created.PropertyTitle != "Apto Vila Mariana" {
		t.Errorf("PropertyTitle: got %q, want %q (snapshot from request)", created.PropertyTitle, "Apto Vila Mariana")
	}
	if created.AdjustIndex != "IPCA" {
		t.Errorf("AdjustIndex: got %q, want %q (snapshot from request)", created.AdjustIndex, "IPCA")
	}
	if !created.AutoAdjust {
		t.Errorf("AutoAdjust: got false, want true (snapshot from request)")
	}
}

func TestBackfillContract_PopulatesEmptyFields(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "lead-1", "broker-1", 1800, "tenant-1")
	d.Commission = 6.0
	_ = d.AddParty(deal.Party{ID: "party-1", Role: deal.PartyRoleTenant, Name: "João", LeadID: "lead-1", IsPrimary: true})

	c := &contract.Contract{ID: "c1", DealID: d.ID, Type: contract.TypeLocacao, TenantID: "tenant-1"}
	changed := service.BackfillContract(c, d)
	if !changed {
		t.Fatal("BackfillContract returned false, expected true (every field was empty)")
	}
	if c.Client != "João" || c.CustomerID != "lead-1" || c.PropertyID != "prop-1" || c.Value != 1800 || c.CommissionPct != 6.0 {
		t.Errorf("BackfillContract did not fill all empty fields: %+v", c)
	}
}

func TestBackfillContract_PreservesExistingValues(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "lead-1", "broker-1", 1800, "tenant-1")
	d.Commission = 6.0
	_ = d.AddParty(deal.Party{ID: "party-1", Role: deal.PartyRoleTenant, Name: "João Novo", LeadID: "lead-1", IsPrimary: true})

	c := &contract.Contract{
		ID:            "c1",
		DealID:        d.ID,
		Type:          contract.TypeLocacao,
		TenantID:      "tenant-1",
		Client:        "João Antigo",
		CustomerID:    "lead-original",
		PropertyID:    "prop-original",
		Value:         9999,
		CommissionPct: 12,
	}
	changed := service.BackfillContract(c, d)
	if changed {
		t.Errorf("BackfillContract returned true with all fields populated; expected idempotent no-op")
	}
	if c.Client != "João Antigo" || c.CustomerID != "lead-original" || c.PropertyID != "prop-original" || c.Value != 9999 || c.CommissionPct != 12 {
		t.Errorf("BackfillContract overwrote existing fields: %+v", c)
	}
}

func TestBackfillContract_NilSafe(t *testing.T) {
	if changed := service.BackfillContract(nil, nil); changed {
		t.Errorf("nil inputs should return false")
	}
	c := &contract.Contract{}
	if changed := service.BackfillContract(c, nil); changed {
		t.Errorf("nil deal should return false")
	}
}

func (m *mockContractRepo) UpdateIfUnchanged(ctx context.Context, c *contract.Contract, _ time.Time) error {
	// Reproduz o comportamento real do repositório: uma gravação com contexto
	// cancelado falha, que foi o que impediu a recuperação do contrato.
	if m.failUpdateWhenCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	return m.Update(ctx, c)
}

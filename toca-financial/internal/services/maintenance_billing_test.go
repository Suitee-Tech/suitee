package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// billingChargeRepo é um repositório de cobranças em memória suficiente para
// exercitar a decisão de destino do custo de manutenção.
type billingChargeRepo struct {
	items   []*charge.Charge
	created []*charge.Charge
	updated []*charge.Charge
}

func (r *billingChargeRepo) Create(_ context.Context, c *charge.Charge) error {
	r.created = append(r.created, c)
	r.items = append(r.items, c)
	return nil
}

func (r *billingChargeRepo) GetByID(_ context.Context, _, id string) (*charge.Charge, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (r *billingChargeRepo) GetBySourceEventID(_ context.Context, _, sourceEventID string) (*charge.Charge, error) {
	for _, item := range r.items {
		if item.SourceEventID != "" && item.SourceEventID == sourceEventID {
			return item, nil
		}
	}
	return nil, nil
}

func (r *billingChargeRepo) Update(_ context.Context, c *charge.Charge) error {
	r.updated = append(r.updated, c)
	for index, item := range r.items {
		if item.ID == c.ID {
			r.items[index] = c
		}
	}
	return nil
}

func (r *billingChargeRepo) Delete(context.Context, string) error { return nil }

func (r *billingChargeRepo) List(_ context.Context, _, contractID, _, _ string, _, _ int64) ([]*charge.Charge, int64, error) {
	var out []*charge.Charge
	for _, item := range r.items {
		if contractID == "" || item.ContractID == contractID {
			out = append(out, item)
		}
	}
	return out, int64(len(out)), nil
}

func (r *billingChargeRepo) ListByPartyIDs(context.Context, string, []string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *billingChargeRepo) ListByLeadIDs(context.Context, string, []string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *billingChargeRepo) Search(context.Context, string, charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *billingChargeRepo) ListOverdue(context.Context, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *billingChargeRepo) AgingReport(context.Context, string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (r *billingChargeRepo) GetDelinquencyStats(context.Context, string) (*charge.DelinquencyStats, error) {
	return nil, nil
}
func (r *billingChargeRepo) BulkUpdateStatus(context.Context, string, []string, charge.Status) error {
	return nil
}

func (r *billingChargeRepo) lastCreated() *charge.Charge {
	if len(r.created) == 0 {
		return nil
	}
	return r.created[len(r.created)-1]
}

// billingTransferRepo devolve os repasses pendentes semeados no teste.
type billingTransferRepo struct {
	pending []*transfer.OwnerTransfer
	updated []*transfer.OwnerTransfer
}

func (r *billingTransferRepo) Create(context.Context, *transfer.OwnerTransfer) error { return nil }

func (r *billingChargeRepo) ListByDealAndCustomer(context.Context, string, string, string, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *billingChargeRepo) ListByIDs(context.Context, string, []string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *billingChargeRepo) ListPendingPastDue(context.Context, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *billingChargeRepo) ListDistinctTenantIDs(context.Context) ([]string, error) { return nil, nil }
func (r *billingChargeRepo) UpdateStatus(context.Context, string, charge.Status, *time.Time, *float64, string) error {
	return nil
}
func (r *billingTransferRepo) GetByID(context.Context, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}
func (r *billingTransferRepo) GetByContractMonth(_ context.Context, _, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.pending {
		if item.ContractID == contractID && item.ReferenceMonth == referenceMonth {
			return item, nil
		}
	}
	return nil, nil
}
func (r *billingTransferRepo) Update(_ context.Context, t *transfer.OwnerTransfer) error {
	r.updated = append(r.updated, t)
	return nil
}
func (r *billingTransferRepo) List(_ context.Context, _, contractID, _, status, _ string, _, _ int64) ([]*transfer.OwnerTransfer, int64, error) {
	var out []*transfer.OwnerTransfer
	for _, item := range r.pending {
		if contractID != "" && item.ContractID != contractID {
			continue
		}
		if status != "" && string(item.Status) != status {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}
func (r *billingTransferRepo) ListByOwnerLeadIDs(context.Context, string, []string, transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *billingTransferRepo) UpdateStatus(context.Context, string, string, transfer.Status) error {
	return nil
}
func (r *billingTransferRepo) Search(context.Context, string, transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

// dateInDays devolve a data (YYYY-MM-DD) N dias à frente de hoje; monthInDays
// devolve o mês (YYYY-MM) correspondente.
//
// Este arquivo usava datas fixas (dateInDays(3) como "cobrança mais próxima").
// Em 10/10/2026 essa data virou passado, `findNextOpenCharge` passou a
// descartá-la (ela exige vencimento não vencido) e o teste reprovou sem
// nenhum defeito no código. Datas relativas mantêm a intenção do cenário.
func dateInDays(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

func monthInDays(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01")
}

func makeOpenCharge(t *testing.T, id string, dueDate string, amount float64) *charge.Charge {
	t.Helper()
	due, err := time.Parse("2006-01-02", dueDate)
	require.NoError(t, err)
	c, err := charge.New("contract-1", "lead-tenant", "lead-owner", "property-1", "tenant-1", charge.TypeRent, "Aluguel", dueDate[:7], amount, due)
	require.NoError(t, err)
	c.ID = id
	// Todo aluguel emitido pelo plano-base nasce com composição; é nela que a
	// manutenção se soma.
	require.NoError(t, c.SetLineItems([]charge.LineItem{{
		Description:     "Aluguel",
		AmountCents:     c.Amount.Cents(),
		SourceType:      "rental_base",
		SourceID:        "cfg-1",
		EconomicOwnerID: "lead-owner",
		SettlementMode:  charge.SettlementModeCollection,
		AllocationType:  charge.AllocationTypeOwnerPayable,
	}}))
	return c
}

// Cobrança sem composição não recebe a manutenção: sem saber o dono econômico
// do valor original, o destino seguro é a cobrança avulsa.
func makeUncomposedCharge(t *testing.T, id string, dueDate string, amount float64) *charge.Charge {
	t.Helper()
	due, err := time.Parse("2006-01-02", dueDate)
	require.NoError(t, err)
	c, err := charge.New("contract-1", "lead-tenant", "lead-owner", "property-1", "tenant-1", charge.TypeRent, "Aluguel legado", dueDate[:7], amount, due)
	require.NoError(t, err)
	c.ID = id
	return c
}

func makePendingTransfer(referenceMonth string) *transfer.OwnerTransfer {
	return &transfer.OwnerTransfer{
		ID:             "transfer-" + referenceMonth,
		TenantID:       "tenant-1",
		ContractID:     "contract-1",
		OwnerID:        "lead-owner",
		ReferenceMonth: referenceMonth,
		Status:         transfer.StatusPending,
		NetAmount:      2000,
	}
}

// Repasse com subledger: o teto do desconto é total − reservado.
func makePendingTransferWithBalance(referenceMonth string, totalCents, reservedCents int64) *transfer.OwnerTransfer {
	item := makePendingTransfer(referenceMonth)
	item.OwnerBalance = &transfer.OwnerBalance{TotalCents: totalCents, ReservedCents: reservedCents}
	return item
}

func billingInput(role string) service.MaintenanceBillingInput {
	return service.MaintenanceBillingInput{
		TicketID:        "ticket-1",
		TenantID:        "tenant-1",
		ContractID:      "contract-1",
		PropertyID:      "property-1",
		RequesterLeadID: "lead-tenant",
		Category:        "Elétrica",
		PayerRole:       role,
	}
}

// A cobrança do inquilino deve receber o valor da manutenção como item da
// composição do próximo documento — não como uma segunda cobrança paralela.
func TestMaintenanceBilling_Tenant_SumsIntoNextOpenCharge(t *testing.T) {
	repo := &billingChargeRepo{}
	near := makeOpenCharge(t, "charge-near", dateInDays(3), 1000)
	far := makeOpenCharge(t, "charge-far", dateInDays(33), 1000)
	repo.items = []*charge.Charge{far, near}

	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)
	target, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.Equal(t, "charge", target.Kind)
	assert.Equal(t, "charge-near", target.ChargeID, "o valor entra na cobrança mais próxima")
	require.Len(t, repo.updated, 1)

	updated := repo.updated[0]
	assert.InDelta(t, 1130.0, updated.Amount.Float(), 0.001)
	// A composição do plano é preservada e a manutenção entra como item novo.
	require.Len(t, updated.LineItems, 2)
	assert.Equal(t, "rental_base", updated.LineItems[0].SourceType)
	assert.InDelta(t, float64(updated.Amount.Cents()), float64(totalItemCents(updated)), 0.001)

	item := maintenanceItemOf(updated, "ticket-1")
	require.NotNil(t, item)
	assert.Equal(t, int64(13000), item.AmountCents)
	assert.Contains(t, item.Description, "Manutenção: Elétrica")
	// Custo de terceiro: o recebimento baixa o adiantamento, não vira receita.
	assert.Equal(t, charge.AllocationTypeReceivable, item.AllocationType)
	assert.Equal(t, charge.SettlementModeDirectLiquidation, item.SettlementMode)
	assert.Equal(t, "tenant-1", item.EconomicOwnerID)
}

// Cobrança que já tem composição recebe apenas o item novo.
func TestMaintenanceBilling_Tenant_AppendsToExistingComposition(t *testing.T) {
	repo := &billingChargeRepo{}
	target := makeOpenCharge(t, "charge-composed", dateInDays(3), 1000)
	require.NoError(t, target.SetLineItems([]charge.LineItem{{
		Description:     "Aluguel",
		AmountCents:     100000,
		SourceType:      "rental_base",
		SourceID:        "rental_base",
		EconomicOwnerID: "lead-owner",
		AllocationType:  charge.AllocationTypeOwnerPayable,
	}}))
	repo.items = []*charge.Charge{target}

	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)
	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "ticket-1")
	require.NoError(t, err)

	require.Len(t, repo.updated, 1)
	assert.Len(t, repo.updated[0].LineItems, 2)
	assert.InDelta(t, 1130.0, repo.updated[0].Amount.Float(), 0.001)
}

func maintenanceItemOf(c *charge.Charge, ticketID string) *charge.LineItem {
	for index := range c.LineItems {
		if c.LineItems[index].SourceID == ticketID {
			return &c.LineItems[index]
		}
	}
	return nil
}

// Repetir o evento não pode somar o mesmo custo duas vezes.
func TestMaintenanceBilling_Tenant_IsIdempotent(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)
	_, err = billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	assert.Len(t, repo.created, 0, "não pode criar cobrança nova")
	require.Len(t, repo.updated, 1, "o segundo evento não altera de novo")
	assert.InDelta(t, 1130.0, repo.updated[0].Amount.Float(), 0.001)
}

// Sem cobrança em aberto, o custo vira uma cobrança avulsa — mas com
// composição, para o pagamento ter dono econômico.
func TestMaintenanceBilling_Tenant_WithoutOpenCharge_CreatesComposedCharge(t *testing.T) {
	repo := &billingChargeRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	target, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.True(t, target.Created)
	created := repo.lastCreated()
	require.NotNil(t, created)
	assert.Equal(t, charge.TypeMaintenance, created.Type)
	require.Len(t, created.LineItems, 1)
	assert.Equal(t, int64(13000), created.LineItems[0].AmountCents)
	assert.InDelta(t, 130.0, created.Amount.Float(), 0.001)
}

// Cobrado do proprietário: desconto no próximo repasse PENDENTE, mesmo que o
// repasse do mês corrente já tenha sido processado.
func TestMaintenanceBilling_Owner_DeductsFromNextPendingTransfer(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransfer(monthInDays(33))}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	target, err := billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.Equal(t, "transfer", target.Kind)
	assert.Equal(t, monthInDays(33), target.ReferenceMonth)
	require.Len(t, transferRepo.updated, 1)
	assert.InDelta(t, 130.0, transferRepo.updated[0].MaintenanceDeductions, 0.001)
	require.Len(t, transferRepo.updated[0].DeductionDetails, 1)
	assert.Equal(t, "ticket-1", transferRepo.updated[0].DeductionDetails[0].ReferenceID)
	assert.Contains(t, transferRepo.updated[0].DeductionDetails[0].Description, "Manutenção: Elétrica")
}

// Sem repasse pendente o operador precisa saber — falha explícita em vez de
// perder o valor silenciosamente.
func TestMaintenanceBilling_Owner_WithoutPendingTransferFails(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repasse pendente")
}

// Responsabilidade da imobiliária não gera cobrança nem desconto.
func TestMaintenanceBilling_Agency_DoesNothing(t *testing.T) {
	repo := &billingChargeRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	target, err := billing.Apply(context.Background(), billingInput("agency"), 130, "evt-1")
	require.NoError(t, err)
	assert.Nil(t, target)
	assert.Empty(t, repo.created)
	assert.Empty(t, repo.updated)
}

// O handler de requisição usa o mesmo destino: o valor entra na próxima
// cobrança em aberto em vez de virar uma cobrança paralela.
func TestMaintenanceRequestedHandler_AddsToNextOpenCharge(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)
	handler := service.NewMaintenanceRequestedEventHandler(billing)

	evt := maintenanceRequestedEvent{
		payload: map[string]interface{}{
			"ticket_id":         "ticket-1",
			"tenant_id":         "tenant-1",
			"contract_id":       "contract-1",
			"property_id":       "property-1",
			"requester_lead_id": "lead-tenant",
			"category":          "Elétrica",
			"payer_role":        "tenant",
			"estimated_cost":    130.0,
		},
	}

	require.NoError(t, handler(context.Background(), evt))
	require.Len(t, repo.updated, 1)
	assert.Equal(t, "charge-near", repo.updated[0].ID)
	assert.InDelta(t, 1130.0, repo.updated[0].Amount.Float(), 0.001)
	assert.Empty(t, repo.created, "não deve criar uma cobrança paralela")
}

type maintenanceRequestedEvent struct {
	payload map[string]interface{}
}

func (m maintenanceRequestedEvent) Name() string    { return "maintenance.requested" }
func (m maintenanceRequestedEvent) EventID() string { return "evt-1" }
func (m maintenanceRequestedEvent) Payload() interface{} {
	return m.payload
}

// Escolha explícita do operador: avulsa mesmo com cobrança e repasse
// disponíveis (contrato encerrado, acordo fora do ciclo mensal).
func TestMaintenanceBilling_StandaloneTargetWins(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransfer(monthInDays(3))}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(repo, nil),
		repo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	input := billingInput("tenant")
	input.Target = "standalone"

	target, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.Equal(t, "charge", target.Kind)
	assert.True(t, target.Created, "a avulsa é uma cobrança nova, não compõe a existente")
	assert.NotEqual(t, "charge-near", target.ChargeID)
	assert.Empty(t, repo.updated, "a cobrança em aberto não é alterada")
	require.Len(t, repo.created, 1)
	assert.Equal(t, charge.TypeMaintenance, repo.created[0].Type)
}

// Cobrança avulsa contra o proprietário quando não há mais repasse
// (locação encerrada ou venda já repassada).
func TestMaintenanceBilling_StandaloneForOwnerWithoutPendingTransfer(t *testing.T) {
	repo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(repo, nil),
		repo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	input := billingInput("owner")
	input.Target = "standalone"
	input.RequesterLeadID = "lead-owner"
	input.PayerLeadID = "lead-owner"

	target, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	created := repo.lastCreated()
	require.NotNil(t, created)
	assert.Equal(t, "lead-owner", created.TenantPartyID, "o proprietário é o pagador da avulsa")
	require.Len(t, created.LineItems, 1)
	assert.Equal(t, int64(13000), created.LineItems[0].AmountCents)
}

// Sem escolha explícita, o destino continua derivado do papel (compatível com
// chamados anteriores).
func TestMaintenanceBilling_TargetFallsBackToPayerRole(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	chargeRepo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(chargeRepo, nil), chargeRepo, nil, nil)

	target, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "charge-near", target.ChargeID)
	assert.Len(t, chargeRepo.updated, 1)
}

// `next_transfer` sem repasse pendente falha com mensagem que aponta a saída.
func TestMaintenanceBilling_NextTransferWithoutPendingTransferExplainsStandalone(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	input := billingInput("owner")
	input.Target = "next_transfer"

	_, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cobrança avulsa")
	assert.Empty(t, chargeRepo.created, "não emite avulsa sem o operador pedir")
}

// addMaintenanceItem compõe a cobrança com o item do chamado, ajustando o
// total antes (a soma dos itens precisa fechar com o valor da cobrança).
func addMaintenanceItem(t *testing.T, c *charge.Charge, ticketID string, amountCents int64) {
	t.Helper()
	c.Amount = valueobjects.NewMoney(c.Amount.Cents() + amountCents)
	c.TotalAmount = c.Amount
	require.NoError(t, c.SetLineItems(append(c.LineItems, charge.LineItem{
		Description:     "Manutenção: Elétrica (#" + ticketID + ")",
		AmountCents:     amountCents,
		SourceType:      "maintenance",
		SourceID:        ticketID,
		EconomicOwnerID: c.TenantID,
		AllocationType:  charge.AllocationTypeRevenue,
	})))
}

func totalItemCents(c *charge.Charge) int64 {
	var sum int64
	for _, item := range c.LineItems {
		sum += item.AmountCents
	}
	return sum
}

// Cobrança em aberto sem composição não é usada: a manutenção vira avulsa em
// vez de forçar o sistema a inventar o dono econômico do valor original.
func TestMaintenanceBilling_Tenant_SkipsUncomposedCharge(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeUncomposedCharge(t, "charge-legacy", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	target, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.True(t, target.Created, "emite cobrança avulsa")
	assert.Empty(t, repo.updated, "a cobrança legada fica intocada")
	created := repo.lastCreated()
	require.NotNil(t, created)
	require.Len(t, created.LineItems, 1, "a avulsa nasce com composição")
	assert.Equal(t, "maintenance", created.LineItems[0].SourceType)
}

// Desconto maior do que o repasse absorve seria truncado em silêncio: o
// líquido não fica negativo e o excedente se perde. Melhor recusar e apontar
// a cobrança avulsa.
func TestMaintenanceBilling_Owner_RejectsDeductionAboveTransferCapacity(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{
		makePendingTransferWithBalance(monthInDays(3), 20000, 0), // R$ 200,00
	}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Apply(context.Background(), billingInput("owner"), 500, "evt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cobrança avulsa")
	assert.Empty(t, transferRepo.updated, "nada é descontado quando não cabe")
}

// Desconto que cabe é aplicado integralmente.
func TestMaintenanceBilling_Owner_DeductionWithinCapacity(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{
		makePendingTransferWithBalance(monthInDays(3), 20000, 5000), // absorve R$ 150,00
	}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.NoError(t, err)
	require.Len(t, transferRepo.updated, 1)
	assert.InDelta(t, 130.0, transferRepo.updated[0].MaintenanceDeductions, 0.001)
}

// Custo final diferente do estimado precisa ajustar o item da cobrança — não
// deixar o valor antigo nem somar de novo.
func TestMaintenanceBilling_ReconcileAdjustsChargeItem(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	target, err := billing.Reconcile(context.Background(), billingInput("tenant"), 150, "evt-1")
	require.NoError(t, err)

	require.NotNil(t, target)
	assert.Equal(t, "charge-near", target.ChargeID)
	updated := repo.items[0]
	assert.InDelta(t, 1150.0, updated.Amount.Float(), 0.001)
	item := maintenanceItemOf(updated, "ticket-1")
	require.NotNil(t, item)
	assert.Equal(t, int64(15000), item.AmountCents)
	assert.Equal(t, int64(updated.Amount.Cents()), totalItemCents(updated))
}

// Reconciliar duas vezes com o mesmo custo final não muda nada (reentrega).
func TestMaintenanceBilling_ReconcileIsIdempotent(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)
	_, err = billing.Reconcile(context.Background(), billingInput("tenant"), 150, "evt-1")
	require.NoError(t, err)
	updatesAfterFirst := len(repo.updated)
	_, err = billing.Reconcile(context.Background(), billingInput("tenant"), 150, "evt-1")
	require.NoError(t, err)

	assert.Equal(t, updatesAfterFirst, len(repo.updated), "nada é regravado quando o valor já confere")
	assert.InDelta(t, 1150.0, repo.items[0].Amount.Float(), 0.001)
}

// Custo final menor reduz o item e o total; zerar remove a manutenção.
func TestMaintenanceBilling_ReconcileRemovesItemWhenCreditedToZero(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)
	_, err = billing.Reconcile(context.Background(), billingInput("tenant"), 0, "evt-1")
	require.NoError(t, err)

	updated := repo.items[0]
	assert.Nil(t, maintenanceItemOf(updated, "ticket-1"))
	assert.InDelta(t, 1000.0, updated.Amount.Float(), 0.001)
	assert.Equal(t, int64(updated.Amount.Cents()), totalItemCents(updated))
}

// Repasse: o desconto é ajustado ao custo final, mantendo o líquido coerente.
func TestMaintenanceBilling_ReconcileAdjustsTransferDeduction(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(monthInDays(3), 500000, 0)}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.NoError(t, err)
	_, err = billing.Reconcile(context.Background(), billingInput("owner"), 150, "evt-1")
	require.NoError(t, err)

	updated := transferRepo.pending[0]
	assert.InDelta(t, 150.0, updated.MaintenanceDeductions, 0.001)
	require.Len(t, updated.DeductionDetails, 1, "um único desconto, com o valor final")
	assert.InDelta(t, 150.0, updated.DeductionDetails[0].Amount, 0.001)
	assert.Equal(t, "ticket-1", updated.DeductionDetails[0].ReferenceID)
}

// Reentrega do evento de requisição depois da reconciliação não relança.
func TestMaintenanceBilling_ApplyAfterReconcileIsNoOp(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(monthInDays(3), 500000, 0)}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.NoError(t, err)
	_, err = billing.Reconcile(context.Background(), billingInput("owner"), 150, "evt-1")
	require.NoError(t, err)
	updates := len(transferRepo.updated)

	_, err = billing.Apply(context.Background(), billingInput("owner"), 130, "evt-1")
	require.NoError(t, err)

	assert.Equal(t, updates, len(transferRepo.updated))
	assert.InDelta(t, 150.0, transferRepo.pending[0].MaintenanceDeductions, 0.001,
		"a reentrega não volta o desconto para o valor estimado")
}

/** Cobrança paga + crédito: o restante vai para a próxima cobrança em aberto. */
func TestMaintenanceBilling_CreditGoesToNextOpenCharge(t *testing.T) {
	repo := &billingChargeRepo{}
	paid := makeOpenCharge(t, "charge-paid", "2026-09-10", 1000)
	paid.Status = charge.StatusPaid
	next := makeOpenCharge(t, "charge-next", dateInDays(33), 1000)
	repo.items = []*charge.Charge{paid, next}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	// Lançado na cobrança já paga (item de 130), custo final 100 → crédito de 30.
	addMaintenanceItem(t, paid, "ticket-1", 13000)

	_, err := billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)

	assert.Equal(t, int64(3000), next.Discount.Cents(), "crédito abatido como desconto na cobrança seguinte")
	assert.InDelta(t, 1130.0, paid.Amount.Float(), 0.001, "a cobrança paga não é editada")
}

/** Sem cobrança seguinte, o crédito ao inquilino vira lançamento avulso. */
func TestMaintenanceBilling_CreditToTenantBecomesStandalone(t *testing.T) {
	repo := &billingChargeRepo{}
	paid := makeOpenCharge(t, "charge-paid", "2026-09-10", 1000)
	paid.Status = charge.StatusPaid
	addMaintenanceItem(t, paid, "ticket-1", 13000)
	repo.items = []*charge.Charge{paid}

	txRepo := &billingTxRepo{}
	txService := service.NewTransactionService(txRepo)
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithTransactions(txService)

	_, err := billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)

	require.Len(t, txRepo.created, 1, "crédito avulso registrado")
	credit := txRepo.created[0]
	assert.Equal(t, transaction.TypeEstorno, credit.Type, "nota de crédito, não despesa")
	assert.InDelta(t, -30.0, credit.Value, 0.001, "estorno é o único posting negativo")
}

/** Repasse processado + crédito: devolve no próximo repasse pendente. */
func TestMaintenanceBilling_CreditReducesNextTransferWithholding(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	processed := makePendingTransferWithBalance("2026-09", 500000, 0)
	processed.Status = transfer.StatusProcessed
	processed.MaintenanceDeductions = 130
	processed.DeductionDetails = []transfer.Deduction{
		{Description: "Manutenção: Elétrica (#ticket-1)", Amount: 130, Type: "MAINTENANCE", ReferenceID: "ticket-1"},
	}
	next := makePendingTransferWithBalance("2026-11", 500000, 0)
	next.MaintenanceDeductions = 50
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{processed, next}}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	)

	_, err := billing.Reconcile(context.Background(), billingInput("owner"), 100, "evt-1")
	require.NoError(t, err)

	// Crédito de 30 abate a retenção de 50 do repasse seguinte.
	assert.InDelta(t, 20.0, next.MaintenanceDeductions, 0.001)
	assert.Equal(t, -30.0, next.DeductionDetails[len(next.DeductionDetails)-1].Amount)
}

/** Sem repasse seguinte, o crédito ao proprietário é pago em avulso. */
func TestMaintenanceBilling_CreditToOwnerBecomesStandalone(t *testing.T) {
	chargeRepo := &billingChargeRepo{}
	processed := makePendingTransferWithBalance("2026-09", 500000, 0)
	processed.Status = transfer.StatusProcessed
	processed.MaintenanceDeductions = 130
	processed.DeductionDetails = []transfer.Deduction{
		{Description: "Manutenção: Elétrica (#ticket-1)", Amount: 130, Type: "MAINTENANCE", ReferenceID: "ticket-1"},
	}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{processed}}
	txRepo := &billingTxRepo{}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(chargeRepo, nil),
		chargeRepo,
		transferRepo,
		service.NewTransferService(transferRepo),
	).WithTransactions(service.NewTransactionService(txRepo))

	_, err := billing.Reconcile(context.Background(), billingInput("owner"), 100, "evt-1")
	require.NoError(t, err)

	require.Len(t, txRepo.created, 1)
	assert.Equal(t, transaction.TypeRepasse, txRepo.created[0].Type, "devolução ao proprietário é um pagamento")
	assert.InDelta(t, 30.0, txRepo.created[0].Value, 0.001)
}

/** Reentrega do evento não credita duas vezes. */
func TestMaintenanceBilling_CreditIsIdempotent(t *testing.T) {
	repo := &billingChargeRepo{}
	paid := makeOpenCharge(t, "charge-paid", "2026-09-10", 1000)
	paid.Status = charge.StatusPaid
	addMaintenanceItem(t, paid, "ticket-1", 13000)
	next := makeOpenCharge(t, "charge-next", dateInDays(33), 1000)
	repo.items = []*charge.Charge{paid, next}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	_, err := billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)
	_, err = billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)

	assert.Equal(t, int64(3000), next.Discount.Cents(), "o desconto não acumula na reentrega")
}

type billingTxRepo struct {
	created []*transaction.Transaction
}

func (r *billingTxRepo) Create(_ context.Context, t *transaction.Transaction) error {
	r.created = append(r.created, t)
	return nil
}
func (r *billingTxRepo) CreateBatch(_ context.Context, items []*transaction.Transaction) error {
	r.created = append(r.created, items...)
	return nil
}
func (r *billingTxRepo) GetByID(context.Context, string, string) (*transaction.Transaction, error) {
	return nil, nil
}
func (r *billingTxRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	for _, item := range r.created {
		if item.TenantID == tenantID && item.SourceEventID == sourceEventID {
			return item, nil
		}
	}
	return nil, nil
}
func (r *billingTxRepo) Update(context.Context, *transaction.Transaction) error { return nil }
func (r *billingTxRepo) Delete(context.Context, string, string) error           { return nil }
func (r *billingTxRepo) List(context.Context, string, string, int64, int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (r *billingTxRepo) Search(context.Context, string, transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}

/** A baixa contábil acompanha a dedução: owner_payable × direito a receber. */
func TestTransferDeductionPostsRecoveryJournal(t *testing.T) {
	referenceMonth := monthInDays(3)
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(referenceMonth, 500000, 0)}}
	journalRepo := &billingJournalRepo{}
	transferService := service.NewTransferService(transferRepo).
		WithJournalService(service.NewJournalService(journalRepo))

	require.NoError(t, transferService.ApplyDeduction(context.Background(), "tenant-1", "contract-1", referenceMonth,
		"MAINTENANCE", "Manutenção: Elétrica (#ticket-1)", "ticket-1", 130))

	require.Len(t, journalRepo.entries, 1, "a retenção precisa baixar o passivo no razão")
	legs := map[journal.Account]journal.Leg{}
	for _, leg := range journalRepo.entries[0].Legs {
		legs[leg.Account] = leg
	}
	assert.Equal(t, journal.Debit, legs[journal.AccountOwnerPayable].Direction)
	assert.Equal(t, int64(13000), legs[journal.AccountOwnerPayable].AmountCents)
	assert.Equal(t, journal.Credit, legs[journal.AccountAccountsReceivable].Direction,
		"retenção reembolsa o direito a receber, não gera receita própria")
	assert.Equal(t, int64(13000), legs[journal.AccountAccountsReceivable].AmountCents)
	_, hasRevenue := legs[journal.AccountRevenue]
	assert.False(t, hasRevenue)
}

/** Conta de consumo paga em nome do proprietário é estorno de despesa. */
func TestTransferDeductionPostsExpenseReversalForUtility(t *testing.T) {
	referenceMonth := monthInDays(3)
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(referenceMonth, 500000, 0)}}
	journalRepo := &billingJournalRepo{}
	transferService := service.NewTransferService(transferRepo).
		WithJournalService(service.NewJournalService(journalRepo))

	require.NoError(t, transferService.ApplyDeduction(context.Background(), "tenant-1", "contract-1", referenceMonth,
		"UTILITY", "Energia — 2026-10", "utility.ua-1.2026-10", 80))

	require.Len(t, journalRepo.entries, 1)
	legs := map[journal.Account]journal.Leg{}
	for _, leg := range journalRepo.entries[0].Legs {
		legs[leg.Account] = leg
	}
	assert.Equal(t, journal.Credit, legs[journal.AccountExpense].Direction,
		"tipo sem natureza confirmada não infla receita")
	assert.Equal(t, int64(8000), legs[journal.AccountExpense].AmountCents)
}

/** Reentrega da dedução não duplica o lançamento. */
func TestTransferDeductionLedgerIsIdempotent(t *testing.T) {
	referenceMonth := monthInDays(3)
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(referenceMonth, 500000, 0)}}
	journalRepo := &billingJournalRepo{}
	transferService := service.NewTransferService(transferRepo).
		WithJournalService(service.NewJournalService(journalRepo))

	for i := 0; i < 2; i++ {
		require.NoError(t, transferService.ApplyDeduction(context.Background(), "tenant-1", "contract-1", referenceMonth,
			"MAINTENANCE", "Manutenção: Elétrica (#ticket-1)", "ticket-1", 130))
	}
	assert.Len(t, journalRepo.entries, 1)
}

/** O adiantamento da imobiliária é direito a receber, nunca receita. */
func TestMaintenanceBilling_AgencyAdvanceIsReceivableNotRevenue(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	journalRepo := &billingJournalRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithJournal(service.NewJournalService(journalRepo))

	_, err := billing.Apply(context.Background(), billingInput("tenant"), 130, "evt-1")
	require.NoError(t, err)

	require.Len(t, journalRepo.entries, 1, "o desembolso do fornecedor é registrado")
	legs := map[journal.Account]journal.Leg{}
	for _, leg := range journalRepo.entries[0].Legs {
		legs[leg.Account] = leg
	}
	assert.Equal(t, journal.Debit, legs[journal.AccountAccountsReceivable].Direction)
	assert.Equal(t, int64(13000), legs[journal.AccountAccountsReceivable].AmountCents)
	assert.Equal(t, journal.Credit, legs[journal.AccountCash].Direction)
	_, hasRevenue := legs[journal.AccountRevenue]
	assert.False(t, hasRevenue, "intermediar um custo não gera receita própria")
}

/** Quem adiantou é quem assume: nada passa pela imobiliária. */
func TestMaintenanceBilling_SamePartyAdvancedAndBears_NoMovement(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	transferRepo := &billingTransferRepo{pending: []*transfer.OwnerTransfer{makePendingTransferWithBalance(monthInDays(3), 500000, 0)}}
	journalRepo := &billingJournalRepo{}
	billing := service.NewMaintenanceBillingService(
		service.NewChargeService(repo, nil),
		repo,
		transferRepo,
		service.NewTransferService(transferRepo),
	).WithJournal(service.NewJournalService(journalRepo))

	// O proprietário pagou o fornecedor e é ele quem assume o custo.
	input := billingInput("owner")
	input.AdvancedBy = "proprietario"

	target, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	assert.Nil(t, target, "não há valor a cobrar nem a reembolsar")
	assert.Empty(t, repo.updated, "nenhuma cobrança é alterada")
	assert.Empty(t, transferRepo.updated, "nenhum repasse é retido")
	assert.Empty(t, journalRepo.entries, "nada entra no razão da imobiliária")
}

/** Inquilino adiantou e é ele quem assume: nada passa pela imobiliária.
 * O papel do pagador (tenant) e quem adiantou (inquilino) são a mesma parte em
 * vocabulários diferentes — comparar as strings cruas erraria o caso. */
func TestMaintenanceBilling_TenantAdvancedAndBears_NoMovement(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	journalRepo := &billingJournalRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithJournal(service.NewJournalService(journalRepo))

	input := billingInput("tenant")
	input.AdvancedBy = "inquilino"

	target, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	assert.Nil(t, target)
	assert.Empty(t, repo.updated, "não há reembolso a cobrar do próprio inquilino")
	assert.Empty(t, journalRepo.entries, "nada entra no razão: o custo nunca foi da imobiliária")
}

/** Proprietário adiantou e o inquilino reembolsa: o valor é do proprietário. */
func TestMaintenanceBilling_OwnerAdvanced_TenantReimbursesOwner(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	input := billingInput("tenant")
	input.AdvancedBy = "proprietario"
	input.OwnerLeadID = "lead-owner"

	_, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	require.Len(t, repo.updated, 1)
	item := maintenanceItemOf(repo.updated[0], "ticket-1")
	require.NotNil(t, item)
	assert.Equal(t, charge.AllocationTypeOwnerPayable, item.AllocationType,
		"o reembolso pertence a quem adiantou, não à imobiliária")
	assert.Equal(t, "lead-owner", item.EconomicOwnerID)
	assert.Equal(t, charge.SettlementModeReimbursement, item.SettlementMode)
}

/** Custo assumido pela imobiliária é despesa própria, não cobrança. */
func TestMaintenanceBilling_AgencyBearsCost_BecomesExpense(t *testing.T) {
	repo := &billingChargeRepo{}
	txRepo := &billingTxRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithTransactions(service.NewTransactionService(txRepo))

	target, err := billing.Apply(context.Background(), billingInput("agency"), 130, "evt-1")
	require.NoError(t, err)

	assert.Nil(t, target)
	assert.Empty(t, repo.created, "não há cobrança de terceiro")
	require.Len(t, txRepo.created, 1)
	assert.Equal(t, transaction.TypeDespesa, txRepo.created[0].Type)
	assert.InDelta(t, 130.0, txRepo.created[0].Value, 0.001)
}

type billingJournalRepo struct {
	entries []*journal.Entry
}

func (r *billingJournalRepo) Create(_ context.Context, entry *journal.Entry) error {
	for _, existing := range r.entries {
		if existing.SourceEventID != "" && existing.SourceEventID == entry.SourceEventID {
			return journal.ErrEntryAlreadyExists
		}
	}
	r.entries = append(r.entries, entry)
	return nil
}
func (r *billingJournalRepo) GetByID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (r *billingJournalRepo) GetBySourceEventID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (r *billingJournalRepo) List(context.Context, string, string, int64, int64) ([]*journal.Entry, int64, error) {
	return r.entries, int64(len(r.entries)), nil
}
func (r *billingJournalRepo) UpdateStatus(context.Context, *journal.Entry, time.Time) (bool, error) {
	return true, nil
}
func (r *billingJournalRepo) AccountBalance(_ context.Context, _ string, account journal.Account) (int64, error) {
	var balance int64
	for _, entry := range r.entries {
		for _, leg := range entry.Legs {
			if leg.Account != account {
				continue
			}
			if leg.Direction == journal.Debit {
				balance += leg.AmountCents
			} else {
				balance -= leg.AmountCents
			}
		}
	}
	return balance, nil
}

/** Crédito avulso é nota de crédito de receita, não despesa. */
func TestMaintenanceBilling_TenantCreditIsRevenueCreditNote(t *testing.T) {
	repo := &billingChargeRepo{}
	paid := makeOpenCharge(t, "charge-paid", "2026-09-10", 1000)
	paid.Status = charge.StatusPaid
	addMaintenanceItem(t, paid, "ticket-1", 13000)
	repo.items = []*charge.Charge{paid}

	txRepo := &billingTxRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithTransactions(service.NewTransactionService(txRepo))

	_, err := billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)

	require.Len(t, txRepo.created, 1)
	credit := txRepo.created[0]
	assert.Equal(t, transaction.TypeEstorno, credit.Type, "devolução de valor de terceiro não é despesa")
	assert.InDelta(t, -30.0, credit.Value, 0.001)
	assert.Equal(t, "2026-09", credit.Month, "o crédito entra na competência do lançamento que o gerou")
}

/** Sem identificar o proprietário que adiantou, o valor não é creditado à
 * imobiliária: falha explícita em vez de dono econômico inventado. */
func TestMaintenanceBilling_OwnerAdvancedWithoutOwner_Fails(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil)

	input := billingInput("tenant")
	input.AdvancedBy = "proprietario"

	_, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "proprietário")
	assert.Empty(t, repo.updated, "nada é lançado com dono econômico incerto")
}

/** O proprietário do contrato é resolvido quando o chamado não o traz. */
func TestMaintenanceBilling_OwnerAdvancedResolvesOwnerFromContract(t *testing.T) {
	repo := &billingChargeRepo{}
	repo.items = []*charge.Charge{makeOpenCharge(t, "charge-near", dateInDays(3), 1000)}
	configRepo := &billingRentalConfigRepo{ownerID: "owner-from-contract"}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, configRepo), repo, nil, nil)

	input := billingInput("tenant")
	input.AdvancedBy = "proprietario"

	_, err := billing.Apply(context.Background(), input, 130, "evt-1")
	require.NoError(t, err)

	item := maintenanceItemOf(repo.updated[0], "ticket-1")
	require.NotNil(t, item)
	assert.Equal(t, "owner-from-contract", item.EconomicOwnerID)
}

type billingRentalConfigRepo struct {
	ownerID string
}

func (r *billingRentalConfigRepo) Create(context.Context, *rental_charge_config.RentalChargeConfig) error {
	return nil
}
func (r *billingRentalConfigRepo) GetByID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error) {
	return nil, nil
}
func (r *billingRentalConfigRepo) GetByContractID(_ context.Context, _, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	if contractID != "contract-1" {
		return nil, nil
	}
	return &rental_charge_config.RentalChargeConfig{ID: "cfg-1", TenantID: "tenant-1", ContractID: contractID, OwnerID: r.ownerID}, nil
}
func (r *billingRentalConfigRepo) Update(context.Context, *rental_charge_config.RentalChargeConfig) error {
	return nil
}
func (r *billingRentalConfigRepo) Delete(context.Context, string, string) error { return nil }
func (r *billingRentalConfigRepo) List(context.Context, string, int64, int64) ([]*rental_charge_config.RentalChargeConfig, int64, error) {
	return nil, 0, nil
}

/** A nota de crédito ao inquilino reduz o direito a receber e cria a obrigação. */
func TestMaintenanceBilling_TenantCreditPostsLiabilityNotRevenue(t *testing.T) {
	repo := &billingChargeRepo{}
	paid := makeOpenCharge(t, "charge-paid", "2026-09-10", 1000)
	paid.Status = charge.StatusPaid
	addMaintenanceItem(t, paid, "ticket-1", 13000)
	repo.items = []*charge.Charge{paid}

	txRepo := &billingTxRepo{}
	journalRepo := &billingJournalRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(repo, nil), repo, nil, nil).
		WithTransactions(service.NewTransactionService(txRepo)).
		WithJournal(service.NewJournalService(journalRepo))

	_, err := billing.Reconcile(context.Background(), billingInput("tenant"), 100, "evt-1")
	require.NoError(t, err)

	require.Len(t, journalRepo.entries, 1)
	legs := map[journal.Account]journal.Leg{}
	for _, leg := range journalRepo.entries[0].Legs {
		legs[leg.Account] = leg
	}
	assert.Equal(t, journal.Debit, legs[journal.AccountAccountsReceivable].Direction,
		"o valor retido a maior deixa de ser direito a receber")
	assert.Equal(t, journal.Credit, legs[journal.AccountOtherPayable].Direction,
		"e passa a ser obrigação com o inquilino")
	_, hasRevenue := legs[journal.AccountRevenue]
	assert.False(t, hasRevenue, "devolução não passa por receita nem por despesa")
	_, hasExpense := legs[journal.AccountExpense]
	assert.False(t, hasExpense, "devolução não é despesa da imobiliária")
}

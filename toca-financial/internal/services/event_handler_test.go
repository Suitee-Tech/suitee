package services_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCommissionRepo captures Commission entities created by the SALE path
// so tests can assert per-broker persistence. Matches the minimal slice of
// CommissionRepository the service interacts with — Create is the only one
// the contract.signed pipeline calls today (#414).
type mockCommissionRepo struct {
	created    []*commission.Commission
	err        error
	paidDealID string
}

func (m *mockCommissionRepo) Create(_ context.Context, c *commission.Commission) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, c)
	return nil
}

func (m *mockCommissionRepo) GetByID(_ context.Context, _ string, id string) (*commission.Commission, error) {
	for _, c := range m.created {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockCommissionRepo) GetBySourceEventID(_ context.Context, _, _ string) (*commission.Commission, error) {
	return nil, nil
}
func (m *mockCommissionRepo) List(_ context.Context, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}
func (m *mockCommissionRepo) UpdateStatus(_ context.Context, _, _ string, _ commission.Status) error {
	return nil
}
func (m *mockCommissionRepo) UpdateStatusByDealID(_ context.Context, _, dealID string, _ commission.Status) error {
	m.paidDealID = dealID
	return nil
}

// Update / ListByDealID / Delete foram adicionados ao
// service.CommissionRepository quando o ciclo de vida de Commission ganhou
// approve/pay/cancel e 1:1 payout linking (FIN-COM-08, P1-12).
func (m *mockCommissionRepo) Update(_ context.Context, c *commission.Commission) error {
	for i, existing := range m.created {
		if existing.ID == c.ID {
			m.created[i] = c
			return nil
		}
	}
	return nil
}
func (m *mockCommissionRepo) ListByDealID(_ context.Context, _, _ string) ([]*commission.Commission, error) {
	return m.created, nil
}
func (m *mockCommissionRepo) Delete(_ context.Context, _, _ string) error { return nil }

func (m *mockCommissionRepo) ListByBrokerID(_ context.Context, _, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}

// Mock Repositories

type mockChargeRepo struct {
	created *charge.Charge
}

func (m *mockChargeRepo) Create(ctx context.Context, c *charge.Charge) error {
	m.created = c
	return nil
}
func (m *mockChargeRepo) GetByID(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*charge.Charge, error) {
	if m.created != nil && m.created.SourceEventID == sourceEventID {
		return m.created, nil
	}
	return nil, nil
}
func (m *mockChargeRepo) Update(ctx context.Context, c *charge.Charge) error { return nil }
func (m *mockChargeRepo) Delete(ctx context.Context, id string) error        { return nil }
func (m *mockChargeRepo) List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) Search(ctx context.Context, tenantID string, filter charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (m *mockChargeRepo) ListOverdue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) AgingReport(ctx context.Context, tenantID string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (m *mockChargeRepo) GetDelinquencyStats(ctx context.Context, tenantID string) (*charge.DelinquencyStats, error) {
	return nil, nil
}
func (m *mockChargeRepo) BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status charge.Status) error {
	return nil
}
func (m *mockChargeRepo) ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListPendingPastDue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListByDealAndCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) UpdateStatus(ctx context.Context, id string, status charge.Status, paidAt *time.Time, paidAmount *float64, paymentMethod string) error {
	return nil
}

type mockTransferRepo struct {
	created *transfer.OwnerTransfer
}

func (m *mockTransferRepo) Create(ctx context.Context, t *transfer.OwnerTransfer) error {
	m.created = t
	return nil
}
func (m *mockTransferRepo) GetByID(ctx context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}
func (m *mockTransferRepo) Update(ctx context.Context, t *transfer.OwnerTransfer) error { return nil }
func (m *mockTransferRepo) List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (m *mockTransferRepo) UpdateStatus(ctx context.Context, tenantID, id string, status transfer.Status) error {
	return nil
}
func (m *mockTransferRepo) Search(ctx context.Context, tenantID string, filter transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (m *mockTransferRepo) ListByOwnerLeadIDs(ctx context.Context, tenantID string, ownerLeadIDs []string, filter transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (m *mockTransferRepo) GetByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	if m.created != nil && m.created.TenantID == tenantID && m.created.ContractID == contractID && m.created.ReferenceMonth == referenceMonth {
		return m.created, nil
	}
	return nil, nil
}

type mockTxRepo struct {
	created []*transaction.Transaction
}

func (m *mockTxRepo) Create(ctx context.Context, t *transaction.Transaction) error {
	m.created = append(m.created, t)
	return nil
}
func (m *mockTxRepo) CreateBatch(ctx context.Context, txs []*transaction.Transaction) error {
	m.created = append(m.created, txs...)
	return nil
}
func (m *mockTxRepo) GetByID(ctx context.Context, tenantID, id string) (*transaction.Transaction, error) {
	return nil, nil
}
func (m *mockTxRepo) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	for _, tx := range m.created {
		if tx.TenantID == tenantID && tx.SourceEventID == sourceEventID {
			return tx, nil
		}
	}
	return nil, nil
}
func (m *mockTxRepo) Update(ctx context.Context, t *transaction.Transaction) error { return nil }
func (m *mockTxRepo) Delete(ctx context.Context, id string, tenantID string) error { return nil }
func (m *mockTxRepo) List(ctx context.Context, tenantID string, contractID string, limit, offset int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (m *mockTxRepo) Search(ctx context.Context, tenantID string, filter transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (m *mockTxRepo) GetStats(ctx context.Context, tenantID string) (map[string]interface{}, error) {
	return nil, nil
}

type mockEvent struct {
	name    string
	payload interface{}
}

func (m mockEvent) Name() string         { return m.name }
func (m mockEvent) Payload() interface{} { return m.payload }

type recordingChargeRepo struct {
	lastTenantID string
	lastID       string
	err          error
}

func (r *recordingChargeRepo) Create(_ context.Context, _ *charge.Charge) error { return nil }
func (r *recordingChargeRepo) GetByID(_ context.Context, tenantID, id string) (*charge.Charge, error) {
	r.lastTenantID = tenantID
	r.lastID = id
	return nil, r.err
}
func (r *recordingChargeRepo) GetBySourceEventID(_ context.Context, _ string, _ string) (*charge.Charge, error) {
	return nil, nil
}
func (r *recordingChargeRepo) Update(_ context.Context, _ *charge.Charge) error { return nil }
func (r *recordingChargeRepo) Delete(_ context.Context, _ string) error         { return nil }
func (r *recordingChargeRepo) List(_ context.Context, _ string, _ string, _ string, _ string, _ int64, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *recordingChargeRepo) ListByPartyIDs(_ context.Context, _ string, _ []string, _ string, _ string, _ int64, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *recordingChargeRepo) ListByLeadIDs(_ context.Context, _ string, _ []string, _ string, _ string, _ int64, _ int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *recordingChargeRepo) Search(_ context.Context, _ string, _ charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (r *recordingChargeRepo) ListOverdue(_ context.Context, _ string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *recordingChargeRepo) AgingReport(_ context.Context, _ string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (r *recordingChargeRepo) GetDelinquencyStats(_ context.Context, _ string) (*charge.DelinquencyStats, error) {
	return nil, nil
}
func (r *recordingChargeRepo) BulkUpdateStatus(_ context.Context, _ string, _ []string, _ charge.Status) error {
	return nil
}
func (r *recordingChargeRepo) ListByIDs(_ context.Context, _ string, _ []string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *recordingChargeRepo) ListPendingPastDue(_ context.Context, _ string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *recordingChargeRepo) ListDistinctTenantIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
func (r *recordingChargeRepo) ListByDealAndCustomer(_ context.Context, _ string, _ string, _ string, _ string) ([]*charge.Charge, error) {
	return nil, nil
}
func (r *recordingChargeRepo) UpdateStatus(_ context.Context, _ string, _ charge.Status, _ *time.Time, _ *float64, _ string) error {
	return nil
}

type noopPayoutOrderRepo struct {
	created   []*payout_order.PayoutOrder
	createErr error
}

func (r *noopPayoutOrderRepo) Create(_ context.Context, o *payout_order.PayoutOrder) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, o)
	return nil
}
func (r *noopPayoutOrderRepo) GetByID(_ context.Context, _ string, _ string) (*payout_order.PayoutOrder, error) {
	return nil, nil
}
func (r *noopPayoutOrderRepo) GetBySourceEventID(_ context.Context, _ string, _ string) (*payout_order.PayoutOrder, error) {
	return nil, nil
}
func (r *noopPayoutOrderRepo) List(_ context.Context, _ string, _ payout_order.ListFilter) ([]*payout_order.PayoutOrder, int64, error) {
	return nil, 0, nil
}
func (r *noopPayoutOrderRepo) Totals(_ context.Context, _ string, _ payout_order.ListFilter) (*payout_order.PayoutOrderTotals, error) {
	return nil, nil
}
func (r *noopPayoutOrderRepo) ListByIDs(_ context.Context, _ string, _ []string) ([]*payout_order.PayoutOrder, error) {
	return nil, nil
}
func (r *noopPayoutOrderRepo) Update(_ context.Context, _ *payout_order.PayoutOrder) error {
	return nil
}
func (r *noopPayoutOrderRepo) Delete(_ context.Context, _ string, _ string) error { return nil }

type staticCommissionRulesRepo struct {
	rules []commission.Rule
}

func (r *staticCommissionRulesRepo) List(_ context.Context, _ string) ([]commission.Rule, error) {
	return append([]commission.Rule(nil), r.rules...), nil
}

func (r *staticCommissionRulesRepo) Upsert(_ context.Context, _ string, rules []commission.Rule) error {
	r.rules = append([]commission.Rule(nil), rules...)
	return nil
}

type staticFinancialSettingsRepo struct {
	settings *tenant_financial_settings.Settings
}

func (r *staticFinancialSettingsRepo) Get(_ context.Context, _ string) (*tenant_financial_settings.Settings, error) {
	return r.settings, nil
}

func (r *staticFinancialSettingsRepo) Upsert(_ context.Context, settings *tenant_financial_settings.Settings) error {
	r.settings = settings
	return nil
}

// fakeRoyaltyLookup feeds canned partner_royalty rows to the orchestration so
// the royalty-at-signing provisioning can be asserted without a DB.
type fakeRoyaltyLookup struct {
	byDeal         map[string][]map[string]any
	err            error
	payerTenant    string
	lookupDealID   string
	lookupContract string
}

func (f *fakeRoyaltyLookup) ListPendingByPayerAndContract(_ context.Context, payerTenant, dealID, contractID string) ([]map[string]any, error) {
	f.payerTenant = payerTenant
	f.lookupDealID = dealID
	f.lookupContract = contractID
	return f.byDeal[dealID], f.err
}

func TestContractSignedProvisionsPerBrokerCommissionPayoutAndLinks1to1(t *testing.T) {
	// P1-12: reconcile orphan commit 358f5d99 — per-broker payout linked
	// 1:1 to Commission, idempotent by SourceEventID.
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	payoutRepo := &noopPayoutOrderRepo{}

	rules := []commission.Rule{
		{TenantID: "tenant-1", Role: commission.RoleCaptador, DefaultPercentage: 50},
		{TenantID: "tenant-1", Role: commission.RoleFechamento, DefaultPercentage: 50},
	}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)
	rulesService := service.NewCommissionRulesService(&staticCommissionRulesRepo{rules: rules})

	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commService)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService,
		nil, nil, nil, rulesService, nil, payoutService,
	)

	payload := map[string]interface{}{
		"contract_id":    "cont-broker-payouts",
		"deal_id":        "deal-payouts",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          2000.0,
		"commission_pct": 10.0,
		"customer_id":    "cust-1",
		"owner_id":       "owner-1",
		"property_id":    "prop-1",
		"broker_participations": []interface{}{
			map[string]interface{}{"broker_id": "broker-1", "role": "captador", "percentage": 100.0},
		},
	}
	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)

	// Should create exactly 1 Commission entity (captador).
	assert.Len(t, commRepo.created, 1, "deve criar 1 Commission (captador)")

	// Verify payout was created for the broker with correct type + amount.
	require.NotEmpty(t, payoutRepo.created, "deve provisionar payout por corretor")
	brokerPayouts := 0
	consolidatedPayouts := 0
	for _, o := range payoutRepo.created {
		if o.RecipientType == payout_order.RecipientBroker {
			brokerPayouts++
			assert.Equal(t, int64(20000), o.AmountCents, "10%% de R$2000,00 = R$200,00 (20000 cents); captador 100%%")
			assert.Equal(t, "broker-1", o.RecipientID)
		}
		if o.RecipientType == payout_order.RecipientExternal {
			consolidatedPayouts++
		}
	}
	assert.Equal(t, 1, brokerPayouts, "deve ter 1 payout RecipientBroker")
	assert.Equal(t, 0, consolidatedPayouts, "não deve criar payout consolidado quando há Commission por corretor")

	// P1-12: commission↔payout link is 1:1.
	assert.NotEmpty(t, commRepo.created[0].PayoutOrderID,
		"Commission deve estar vinculada ao PayoutOrder")
	payoutID := commRepo.created[0].PayoutOrderID
	assert.Equal(t, payoutRepo.created[0].ID, payoutID,
		"PayoutOrderID da Commission deve bater com o ID do payout criado")
}

func TestContractSignedProvisionsRoyaltyPayout(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	payoutRepo := &noopPayoutOrderRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commService)
	royaltyLookup := &fakeRoyaltyLookup{byDeal: map[string][]map[string]any{
		"deal-1": {
			{"id": "roy-1", "payee_tenant": "franchisor-1", "percentage": 5.0, "amount": 30.0},
			{"id": "roy-2", "payee_tenant": "franchisor-1", "percentage": 2.5, "amount": 15.0},
		},
	}}
	payoutService.SetRoyaltyLookup(royaltyLookup)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, nil, nil, payoutService)

	payload := map[string]interface{}{
		"contract_id":    "cont-1",
		"deal_id":        "deal-1",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          2000.0,
		"commission_pct": 0.0,
		"customer_id":    "cust-1",
		"owner_id":       "owner-1",
		"property_id":    "prop-1",
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)

	// Royalty remains independently payable even with no broker commission.
	// Each payout uses the one-time accrued amount, not a per-charge slice.
	royaltyPayouts := make([]*payout_order.PayoutOrder, 0, 2)
	for _, o := range payoutRepo.created {
		if o.SourceType == payout_order.SourceRoyaltySettlement {
			royaltyPayouts = append(royaltyPayouts, o)
		}
	}
	require.Len(t, royaltyPayouts, 2, "cada royalty deve gerar uma obrigação distinta")
	assert.Equal(t, payout_order.RecipientRoyaltyPayee, royaltyPayouts[0].RecipientType)
	assert.Equal(t, "franchisor-1", royaltyPayouts[0].RecipientID)
	assert.Equal(t, int64(3000), royaltyPayouts[0].AmountCents)
	assert.Equal(t, "cont-1", royaltyPayouts[0].ContractID)
	assert.Equal(t, "roy-1", royaltyPayouts[0].SourceID)
	assert.Equal(t, "roy-2", royaltyPayouts[1].SourceID)
	assert.NotEqual(t, royaltyPayouts[0].SourceEventID, royaltyPayouts[1].SourceEventID,
		"royalty ID must participate in payout identity even for the same payee")
	assert.Equal(t, "tenant-1", royaltyLookup.payerTenant)
	assert.Equal(t, "deal-1", royaltyLookup.lookupDealID)
	assert.Equal(t, "cont-1", royaltyLookup.lookupContract)
}

func TestContractSignedRoyaltyLookupFailureIsFailFast(t *testing.T) {
	lookupErr := errors.New("royalty lookup unavailable")
	repo := &noopPayoutOrderRepo{}
	serviceUnderTest := service.NewPayoutOrchestrationService(repo, nil, nil)
	serviceUnderTest.SetRoyaltyLookup(&fakeRoyaltyLookup{err: lookupErr})

	err := serviceUnderTest.OnContractSigned(context.Background(), service.ContractFinancialsInput{
		EventID: "event-royalty-lookup-failure",
		Payload: &contracts.ContractSignedEventPayload{
			TenantID: "tenant-1", DealID: "deal-1", ContractID: "contract-1", Type: "SALE", Value: 2_000,
		},
	})

	require.ErrorIs(t, err, lookupErr)
	assert.Empty(t, repo.created)
}

func TestContractSignedRoyaltyPayoutPersistenceFailureIsFailFast(t *testing.T) {
	persistErr := errors.New("payout repository unavailable")
	repo := &noopPayoutOrderRepo{createErr: persistErr}
	serviceUnderTest := service.NewPayoutOrchestrationService(repo, nil, nil)
	serviceUnderTest.SetRoyaltyLookup(&fakeRoyaltyLookup{byDeal: map[string][]map[string]any{
		"deal-1": {{"id": "royalty-1", "payee_tenant": "franchisor-1", "percentage": 5.0, "amount": 30.0}},
	}})

	err := serviceUnderTest.OnContractSigned(context.Background(), service.ContractFinancialsInput{
		EventID: "event-royalty-persist-failure",
		Payload: &contracts.ContractSignedEventPayload{
			TenantID: "tenant-1", DealID: "deal-1", ContractID: "contract-1", Type: "SALE", Value: 2_000,
		},
	})

	require.ErrorIs(t, err, persistErr)
	assert.Empty(t, repo.created)
}

func TestContractSignedEventHandler(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	payoutRepo := &noopPayoutOrderRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commService)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, nil, nil, payoutService)

	payload := map[string]interface{}{
		"contract_id":    "cont-1",
		"deal_id":        "deal-1",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          2000.0,
		"commission_pct": 10.0,
		"customer_id":    "cust-1",
		"owner_id":       "owner-1",
		"property_id":    "prop-1",
		"broker_participations": []interface{}{
			map[string]interface{}{
				"broker_id":  "broker-1",
				"role":       "captador",
				"percentage": 50.0,
			},
			map[string]interface{}{
				"broker_id":  "broker-2",
				"role":       "fechamento",
				"percentage": 50.0,
			},
		},
	}

	event := mockEvent{
		name:    "contract.signed",
		payload: payload,
	}

	err := handler(context.Background(), event)
	assert.NoError(t, err)

	// Modelo manual: NENHUMA charge é emitida na assinatura.
	assert.Nil(t, cRepo.created, "manual: nenhuma cobrança deve ser emitida no contract.signed")

	// SALE não provisiona repasse ao proprietário (só RENT gera Transfer).
	assert.Nil(t, tRepo.created, "SALE não provisiona OwnerTransfer")

	// Um payout por beneficiário, conservando 10% de 2000 = 200,00.
	assert.Len(t, payoutRepo.created, 2, "deve provisionar um payout por corretor")
	assert.Equal(t, int64(10000), payoutRepo.created[0].AmountCents)
	assert.Equal(t, int64(10000), payoutRepo.created[1].AmountCents)

	// Verify Commission TXs (ledger view)
	assert.Len(t, txRepo.created, 2)
	assert.Equal(t, 100.0, txRepo.created[0].Value) // 50% of 200 (commission amount)
	assert.Equal(t, "Comissão - captador (50.0%)", txRepo.created[0].Description)

	// Verify Commission domain entities (lifecycle view) — issue #414.
	// One per broker with the share Amount + PENDING status + correct deal
	// linkage so commission UI / payment workflow can drive each.
	assert.Len(t, commRepo.created, 2, "SALE should persist one Commission per broker")
	assert.Equal(t, "deal-1", commRepo.created[0].DealID)
	assert.Equal(t, "broker-1", commRepo.created[0].BrokerID)
	assert.Equal(t, 100.0, commRepo.created[0].Amount)
	assert.Equal(t, "tenant-1", commRepo.created[0].TenantID)
	assert.Equal(t, commission.StatusPending, commRepo.created[0].Status)
	assert.Equal(t, "broker-2", commRepo.created[1].BrokerID)
	assert.Equal(t, 100.0, commRepo.created[1].Amount)
}

// SALE com participações sem percentual explícito → o engine cai no
// commission.DefaultPercentages para a role. Testa que a Distribuição
// "configurável por tenant" (critério de #414) tem fallback determinístico.
func TestContractSignedEventHandler_SALE_appliesDefaultPercentagesWhenBPPercentageIsZero(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, nil, nil, nil)

	payload := map[string]interface{}{
		"contract_id":    "cont-2",
		"deal_id":        "deal-2",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          10000.0,
		"commission_pct": 6.0, // commissionAmount = 600
		"customer_id":    "cust-2",
		"owner_id":       "owner-2",
		"property_id":    "prop-2",
		"broker_participations": []interface{}{
			// Percentage omitted — engine should fall back to
			// DefaultPercentages[captador]=30, [visitador]=20, [fechamento]=50.
			map[string]interface{}{"broker_id": "broker-cap", "role": "captador"},
			map[string]interface{}{"broker_id": "broker-vis", "role": "visitador"},
			map[string]interface{}{"broker_id": "broker-fec", "role": "fechamento"},
		},
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)

	// captador: 30% of 600 = 180
	// visitador: 20% of 600 = 120
	// fechamento: 50% of 600 = 300
	assert.Len(t, commRepo.created, 3)
	assert.Equal(t, "broker-cap", commRepo.created[0].BrokerID)
	assert.Equal(t, 180.0, commRepo.created[0].Amount)
	assert.Equal(t, "broker-vis", commRepo.created[1].BrokerID)
	assert.Equal(t, 120.0, commRepo.created[1].Amount)
	assert.Equal(t, "broker-fec", commRepo.created[2].BrokerID)
	assert.Equal(t, 300.0, commRepo.created[2].Amount)
}

func TestContractSignedEventHandler_TenantDefaultAndResolvedRuleKeepAllOutputsEqual(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	payoutRepo := &noopPayoutOrderRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)
	rulesService := service.NewCommissionRulesService(&staticCommissionRulesRepo{rules: []commission.Rule{
		{TenantID: "tenant-1", Role: commission.RoleCaptador, DefaultPercentage: 40},
		{TenantID: "tenant-1", Role: commission.RoleImobiliaria, DefaultPercentage: 60},
	}})
	settings := tenant_financial_settings.New("tenant-1")
	settings.DefaultCommissionPctSale = 7.5
	settingsService := service.NewTenantFinancialSettingsService(&staticFinancialSettingsRepo{settings: settings})
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commService)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, rulesService, settingsService, payoutService)
	payload := map[string]interface{}{
		"contract_id":    "cont-resolved",
		"deal_id":        "deal-resolved",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          2000.0,
		"commission_pct": 0.0,
		"customer_id":    "customer-1",
		"owner_id":       "owner-1",
		"property_id":    "property-1",
		"broker_participations": []interface{}{
			map[string]interface{}{
				"broker_id": "broker-1",
				"role":      "captador",
				// Omitted percentage resolves to the tenant rule (40%).
			},
			map[string]interface{}{
				"role": "imobiliaria",
				// Retained remainder closes the plan without creating a payable.
			},
		},
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})

	require.NoError(t, err)
	require.Len(t, txRepo.created, 1)
	require.Len(t, commRepo.created, 1)
	require.Len(t, payoutRepo.created, 1)
	// 7.5% (tenant SALE default) of 2,000 = 150; captador gets 40% = 60.
	assert.Equal(t, 60.0, txRepo.created[0].Value)
	assert.Equal(t, 60.0, commRepo.created[0].Amount)
	assert.Equal(t, 40.0, commRepo.created[0].Percentage)
	assert.Equal(t, int64(6000), payoutRepo.created[0].AmountCents)
	assert.Equal(t, 40.0, payoutRepo.created[0].SourceRef["split_pct"])
}

// RENT contracts now ALSO provision Commission entities (PENDING) per broker
// in the manual model — settled manually via /commissions/:id/pay. Regression
// guard that the entity persistence covers RENT, not just SALE.
func TestContractSignedEventHandler_RENT_persistsCommissionEntities(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	// RENT path also exercises rentalConfigService / rentalService; passing
	// nil here drops both — the engine logs and proceeds (same behaviour
	// as pre-#414).
	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, nil, nil, nil)

	payload := map[string]interface{}{
		"contract_id":    "cont-rent",
		"deal_id":        "deal-rent",
		"tenant_id":      "tenant-1",
		"type":           "RENT",
		"value":          3000.0,
		"commission_pct": 10.0,
		"customer_id":    "cust-r",
		"owner_id":       "owner-r",
		"property_id":    "prop-r",
		"broker_participations": []interface{}{
			map[string]interface{}{"broker_id": "broker-rent", "role": "fechamento", "percentage": 100.0},
		},
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)
	// Modelo manual: RENT agora também provisiona Commission (PENDING) por
	// broker — 100% de (3000 * 10%) = 300.
	assert.Len(t, commRepo.created, 1, "RENT deve provisionar Commission por broker")
	assert.Equal(t, "broker-rent", commRepo.created[0].BrokerID)
	assert.Equal(t, 300.0, commRepo.created[0].Amount)
	// The Transaction ledger row still goes out for both RENT and SALE.
	assert.Len(t, txRepo.created, 1)
}

// SALE sem broker_participations → a obrigação fica consolidada no ledger.
// Nenhuma Commission com broker_id sintético pode ser criada: preview e
// materialização permanecem iguais até existirem beneficiários reais.
func TestContractSignedEventHandler_SALE_noParticipationsDoesNotFail(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, commService, nil, nil, nil, nil, nil, nil)

	payload := map[string]interface{}{
		"contract_id":    "cont-3",
		"deal_id":        "deal-3",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          5000.0,
		"commission_pct": 6.0,
		"customer_id":    "cust-3",
		"owner_id":       "owner-3",
		"property_id":    "prop-3",
		// no broker_participations
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)
	assert.Empty(t, commRepo.created, "missing beneficiaries must not create synthetic Commission entities")
	// FIN-COM-09: sem broker_participations não gera transação sintética no ledger
	assert.Empty(t, txRepo.created, "FIN-COM-09: no synthetic ledger when no participants")
}

// Pre-#414 callers (and tests that still pass nil for commissionService)
// must keep working unchanged — feature toggle by wiring.
func TestContractSignedEventHandler_SALE_nilCommissionServiceFallsBackToLedger(t *testing.T) {
	cRepo := &mockChargeRepo{}
	tRepo := &mockTransferRepo{}
	txRepo := &mockTxRepo{}

	cService := service.NewChargeService(cRepo, nil)
	tService := service.NewTransferService(tRepo)
	txService := service.NewTransactionService(txRepo)

	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	payload := map[string]interface{}{
		"contract_id":    "cont-4",
		"deal_id":        "deal-4",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          1000.0,
		"commission_pct": 10.0,
		"customer_id":    "cust-4",
		"owner_id":       "owner-4",
		"property_id":    "prop-4",
		"broker_participations": []interface{}{
			map[string]interface{}{"broker_id": "broker-x", "role": "fechamento", "percentage": 100.0},
		},
	}

	err := handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	assert.NoError(t, err)
	assert.Len(t, txRepo.created, 1) // Transaction ledger still emitted
	// No commRepo to inspect — nil service shorts out the Commission branch.
}

func TestContractSignedEventHandler_Validation(t *testing.T) {
	cService := service.NewChargeService(&mockChargeRepo{}, nil)
	tService := service.NewTransferService(&mockTransferRepo{})
	txService := service.NewTransactionService(&mockTxRepo{})

	// commService nil is fine here — these cases short-circuit before
	// reaching the commission branch (validation rejects payload).
	handler := service.NewContractSignedEventHandler(nil, cService, tService, txService, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	t.Run("Zero Value", func(t *testing.T) {
		event := mockEvent{
			name: "contract.signed",
			payload: map[string]interface{}{
				"contract_id": "cont-1",
				"deal_id":     "deal-1",
				"value":       0.0,
			},
		}
		err := handler(context.Background(), event)
		assert.ErrorContains(t, err, "invalid contract value")
	})

	t.Run("Missing Parties", func(t *testing.T) {
		event := mockEvent{
			name: "contract.signed",
			payload: map[string]interface{}{
				"contract_id": "cont-1",
				"deal_id":     "deal-1",
				"value":       1000.0,
			},
		}
		err := handler(context.Background(), event)
		assert.ErrorContains(t, err, "invalid contract parties")
	})
}

func TestLegacyInvoicePaidEventHandlerUsesSyntheticInvoiceIDFallback(t *testing.T) {
	commRepo := &mockCommissionRepo{}
	commService := service.NewCommissionService(commRepo)

	chargeRepo := &recordingChargeRepo{err: sql.ErrNoRows}
	payoutRepo := &noopPayoutOrderRepo{}
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, chargeRepo, nil)

	h := service.NewInvoicePaidEventHandler(commService, payoutService)
	err := h(context.Background(), mockEvent{
		name: "invoice.paid",
		payload: map[string]interface{}{
			"tenant_id":    "tenant-1",
			"reference_id": "deal-1",
			"invoice_id":   "charge-123",
			"type":         "RENT",
			"amount":       100.0,
		},
	})
	assert.NoError(t, err)
	assert.Empty(t, commRepo.paidDealID, "pagamento da cobrança não pode liquidar comissão automaticamente")
	assert.Equal(t, "tenant-1", chargeRepo.lastTenantID)
	assert.Equal(t, "charge-123", chargeRepo.lastID)
}

func TestLegacyInvoicePaidEventHandlerIgnoresModernInvoiceContract(t *testing.T) {
	chargeRepo := &recordingChargeRepo{err: errors.New("must not be called")}
	payoutService := service.NewPayoutOrchestrationService(&noopPayoutOrderRepo{}, chargeRepo, nil)
	h := service.NewLegacyInvoicePaidEventHandler(payoutService)

	err := h(context.Background(), mockEvent{
		name: "invoice.paid",
		payload: contracts.InvoicePaidEventPayload{
			SchemaVersion: 2,
			InvoiceID:     "invoice-123",
			ChargeID:      "charge-123",
			TenantID:      "tenant-1",
			Amount:        40,
		},
	})
	assert.NoError(t, err)
	assert.Empty(t, chargeRepo.lastID, "modern partial invoice event must not release charge payout")
}

func TestChargePaidEventHandlerPropagatesPayoutFailure(t *testing.T) {
	wantErr := errors.New("charge repository unavailable")
	chargeRepo := &recordingChargeRepo{err: wantErr}
	payoutService := service.NewPayoutOrchestrationService(&noopPayoutOrderRepo{}, chargeRepo, nil)
	h := service.NewChargePaidEventHandler(payoutService)

	err := h(context.Background(), mockEvent{
		name: "charge.paid",
		payload: contracts.ChargePaidEventPayload{
			TenantID: "tenant-1",
			ChargeID: "charge-123",
		},
	})
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, "tenant-1", chargeRepo.lastTenantID)
	assert.Equal(t, "charge-123", chargeRepo.lastID)
}
func (m mockEvent) EventID() string {
	return ""
}

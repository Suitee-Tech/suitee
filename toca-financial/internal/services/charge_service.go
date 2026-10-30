package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/providers"
)

// Service-level sentinels for infrastructure / configuration errors. Handlers
// match against these via errors.Is so the HTTP status mapping survives
// rewording at the service layer. Domain-level sentinels (charge not found,
// invalid state) live next to the entity in domain/charge/errors.go.
var (
	ErrCommClientNotConfigured                = errors.New("comm client not configured")
	ErrCrmClientNotConfigured                 = errors.New("crm client not configured")
	ErrChargeInvoiceReservationLookupRequired = errors.New("invoice reservation lookup is required for an atomic payment")
	ErrChargePaymentExceedsUnreservedBalance  = errors.New("payment exceeds charge balance not reserved by open invoices")
)

// chargeTransferHook é a interface mínima consumida pelo ChargeService
// para disparar a criação do OwnerTransfer mensal quando uma charge
// transita para PAID. Implementação concreta: TransferChargeHook em
// transfer_charge_hook.go. Mantida como interface local para preservar
// a testabilidade do ChargeService — testes injetam um stub sem
// precisar do TransferService real.
type chargeTransferHook interface {
	EnsureMonthlyForChargePaid(ctx context.Context, c *charge.Charge) error
}

// chargeVersionedRepository is implemented by PostgreSQL without widening
// the domain repository contract used by lightweight test fakes. It protects
// JSONB payment accumulation with an optimistic compare-and-swap.
type chargeVersionedRepository interface {
	UpdateIfVersion(ctx context.Context, c *charge.Charge, expected time.Time) (bool, error)
}

type chargeLockingRepository interface {
	GetByIDForUpdate(ctx context.Context, tenantID, id string) (*charge.Charge, error)
}

type chargeInvoiceReservationLookup interface {
	SumOpenAmountCentsByChargeID(ctx context.Context, tenantID, chargeID, excludedInvoiceID string) (int64, error)
}

type chargeCollectionResolver interface {
	ResolveChargePayment(ctx context.Context, tenantID, chargeID string) error
}

type chargePaymentAllocationResolver interface {
	BuildPaymentAllocations(
		ctx context.Context,
		c *charge.Charge,
		paymentID string,
		paymentCents int64,
	) (charge.AllocationMethod, []charge.Allocation, error)
}

// chargePaymentAllocationRebuilder is the repair variant of the resolver:
// it derives allocations for a payment whose charge originally had no
// economic composition (historical unclassified cash) without the safety
// guard that blocks mixing old unallocated cash with a new policy.
type chargePaymentAllocationRebuilder interface {
	RebuildPaymentAllocations(
		ctx context.Context,
		c *charge.Charge,
		paymentID string,
		paymentCents int64,
	) (charge.AllocationMethod, []charge.Allocation, error)
}

type chargeTransactionalOutbox interface {
	EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event events.Event) (string, error)
}

type chargePaymentTxRunner func(ctx context.Context, work func(context.Context) error) error

type chargePaymentMutation struct {
	changed      bool
	beforeStatus charge.Status
}

type ChargeService struct {
	repo             charge.RepositoryInterface
	rentalConfigRepo rental_charge_config.Repository
	commClient       chargeNotifier
	crmClient        chargePartyLookup
	// eventBus (opcional) é usado pelo MarkPaid para publicar charge.paid
	// quando a cobrança canônica é integralmente quitada. A liquidação de uma
	// invoice é um evento separado, produzido pelo InvoiceService.
	eventBus events.EventBus
	// transferHook (opcional) é chamado em MarkPaid para criar o
	// OwnerTransfer mensal idempotente. Nil-safe: sem ele, repasse não
	// é gerado automaticamente.
	transferHook chargeTransferHook
	// txService (opcional) é chamado em MarkPaid para atualizar somente a
	// Transaction de receita ligada à cobrança pelo mesmo source_event_id.
	txService           *TransactionService
	collectionResolver  chargeCollectionResolver
	allocationResolver  chargePaymentAllocationResolver
	paymentJournal      *JournalService
	saleFunding         *SaleCommissionFundingService
	invoiceReservations chargeInvoiceReservationLookup
	db                  *database.PostgresDB
	paymentTx           chargePaymentTxRunner
	// paymentAudit is injectable only to make commit/rollback semantics
	// observable in unit tests. Production falls back to audit.Record.
	paymentAudit func(context.Context, audit.Entry)
	// salePlanResolver (opcional) resolves o plano de comissão canônico de um
	// deal para aplicar a composição econômica de venda na baixa de uma charge
	// criada sem line items (ex.: plano criado antes da assinatura).
	salePlanResolver saleCommissionPlanResolver
	// salePlanMaterializer (opcional) materializa o snapshot congelado de
	// comissões (Commissions + payout orders + ledger + journal) de um deal
	// de venda criado pelo wizard ANTES da assinatura, para que a baixa
	// (FundAllocations) encontre as obrigações congeladas com o mesmo
	// plan hash. Nil-safe: sem ele, só a composição econômica é aplicada.
	salePlanMaterializer saleCommissionPlanMaterializer
	// paymentProvider (P1-05) gera links de cobrança (Pix/boleto) em
	// provedor externo. Nil = endpoint de link responde "não configurado";
	// o router injeta o sandbox conforme PAYMENT_PROVIDER.
	paymentProvider providers.PaymentProvider
}

// WithPaymentProvider injeta o provedor de pagamentos (P1-05). Encadeável.
func (s *ChargeService) WithPaymentProvider(p providers.PaymentProvider) *ChargeService {
	s.paymentProvider = p
	return s
}

// ErrPaymentProviderNotConfigured marca o ambiente sem provedor de
// pagamentos — o endpoint de link responde 409 com orientação de wiring.
var ErrPaymentProviderNotConfigured = errors.New("payment provider not configured")

// GeneratePaymentLink cria (via provedor) o link de cobrança de uma charge
// pendente. O link não é persistido: provedores sandbox são determinísticos
// por cobrança e provedores reais devem gravar o link na cobrança (webhook
// de confirmação fecha o ciclo) — este método é o ponto de costura do
// contrato P1-05.
func (s *ChargeService) GeneratePaymentLink(ctx context.Context, tenantID, chargeID string) (*providers.PaymentLink, error) {
	if s.paymentProvider == nil {
		return nil, ErrPaymentProviderNotConfigured
	}
	c, err := s.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("charge not found")
	}
	if c.Status == charge.StatusPaid {
		return nil, errors.New("charge is already paid")
	}
	// Link já emitido e ainda válido é REUSADO: gerar um novo a cada abertura de
	// tela invalidaria o link que o cliente já recebeu e poluiria o provedor.
	if c.PaymentURL != "" && (c.PaymentExpiresAt == nil || c.PaymentExpiresAt.After(time.Now())) {
		return &providers.PaymentLink{
			URL:        c.PaymentURL,
			ExternalID: c.PaymentExternalID,
			Provider:   c.PaymentProvider,
			ExpiresAt:  derefTime(c.PaymentExpiresAt),
		}, nil
	}

	link, err := s.paymentProvider.CreatePaymentLink(ctx, providers.PaymentLinkInput{
		TenantID:    tenantID,
		ChargeID:    chargeID,
		AmountCents: c.TotalAmount.Cents(),
		DueDate:     c.DueDate,
	})
	if err != nil {
		return nil, err
	}

	// Persistir é o que faz o portal ter destino: antes o link morria na resposta
	// da emissão e a tela do cliente mostrava "pague conforme as instruções".
	expectedVersion := c.UpdatedAt
	c.PaymentURL = link.URL
	c.PaymentExternalID = link.ExternalID
	c.PaymentProvider = link.Provider
	if !link.ExpiresAt.IsZero() {
		expira := link.ExpiresAt
		c.PaymentExpiresAt = &expira
	}
	if _, err := updateChargeIfCurrent(ctx, s.repo, c, expectedVersion); err != nil {
		// O link existe no provedor; falhar aqui não pode esconder isso do
		// chamador — ele recebe o link e a próxima emissão tenta gravar de novo.
		log.Printf("[ChargeService] failed to persist payment link of charge %s: %v", c.ID, err)
	}
	audit.Record(ctx, audit.Entry{
		Action:     "charge.payment_link_created",
		EntityType: "Charge",
		EntityID:   c.ID,
		After:      map[string]any{"provider": link.Provider, "external_id": link.ExternalID, "expires_at": link.ExpiresAt},
		Metadata:   map[string]any{"path": "GeneratePaymentLink", "tenant_id": tenantID},
	})
	return link, nil
}

// derefTime devolve o valor do ponteiro ou o zero value.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// saleCommissionPlanResolver resolves the canonical commission plan for a deal
// (same resolution as the preview and contract.signed).
type saleCommissionPlanResolver interface {
	ResolvePlanForDeal(ctx context.Context, tenantID, dealID string) (*ResolvedCommissionPlan, error)
}

// saleCommissionPlanMaterializer converges the frozen commission snapshot of a
// SALE deal so charge payment can fund it.
type saleCommissionPlanMaterializer interface {
	EnsureMaterialized(ctx context.Context, tenantID, contractID, dealID string) error
}

// chargeNotifier and chargePartyLookup are narrow surfaces of the comm and
// CRM clients respectively. Defining them as local interfaces keeps the
// service unit-testable with stubs without pulling the full client SDKs.
type chargeNotifier interface {
	SendNotification(ctx context.Context, req clients.SendNotificationRequest) error
	PushInAppNotification(ctx context.Context, accountID string, req clients.PushNotificationRequest) error
}

type chargePartyLookup interface {
	GetLead(ctx context.Context, tenantID, leadID string) (*clients.LeadDTO, error)
}

func NewChargeService(repo charge.RepositoryInterface, rentalConfigRepo rental_charge_config.Repository) *ChargeService {
	return &ChargeService{repo: repo, rentalConfigRepo: rentalConfigRepo}
}

// WithSaleCommissionFunding connects signed-sale obligations to collected cash.
func (s *ChargeService) WithSaleCommissionFunding(funding *SaleCommissionFundingService) *ChargeService {
	s.saleFunding = funding
	return s
}

// WithSalePlanResolver wires the commission plan resolver used to apply the
// sale economic composition on the fly when a charge is paid without line items.
func (s *ChargeService) WithSalePlanResolver(resolver saleCommissionPlanResolver) *ChargeService {
	s.salePlanResolver = resolver
	return s
}

// WithSalePlanMaterializer wires the sale commission snapshot materializer so
// a charge created by the wizard before signing can be paid without a prior
// deal.contract.signed delivery.
func (s *ChargeService) WithSalePlanMaterializer(materializer saleCommissionPlanMaterializer) *ChargeService {
	s.salePlanMaterializer = materializer
	return s
}

// SetCommClient wires the messaging service used by Notify. Nil-safe: when
// unset, Notify falls back to a no-op so admin UI flows that don't need
// outbound messaging (tests, dev environments) still work.
func (s *ChargeService) SetCommClient(c chargeNotifier) { s.commClient = c }

// SetCrmClient wires the lead lookup used to resolve the payer's contact
// info (name/email/phone/account_id) when sending reminders. Nil-safe.
func (s *ChargeService) SetCrmClient(c chargePartyLookup) { s.crmClient = c }

// WithEventBus wires the event bus used by MarkPaid to publish
// charge.paid downstream. Fluent setter para permitir wire-up no
// main.go depois do construtor (assim como o transferService já faz).
// Sem chamada, MarkPaid não publica — preserva compat para testes.
func (s *ChargeService) WithEventBus(bus events.EventBus) *ChargeService {
	s.eventBus = bus
	return s
}

// WithInvoiceReservations prevents direct cash commands from consuming value
// already promised by open invoices. InvoiceService passes its own invoice ID
// so that settling that invoice consumes, rather than competes with, its
// reservation. PostgreSQL paths fail closed when this dependency is absent.
func (s *ChargeService) WithInvoiceReservations(lookup chargeInvoiceReservationLookup) *ChargeService {
	s.invoiceReservations = lookup
	return s
}

// WithDB makes payment persistence, transfer creation, collection closure,
// ledger settlement and the charge.paid outbox row one SQL transaction.
func (s *ChargeService) WithDB(db *database.PostgresDB) *ChargeService {
	s.db = db
	if db == nil {
		s.paymentTx = nil
		return s
	}
	s.paymentTx = func(ctx context.Context, work func(context.Context) error) error {
		return db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return work(database.WithTx(ctx, tx))
		})
	}
	return s
}

// SetTransferHook wires o hook que cria o OwnerTransfer mensal quando
// uma charge é paga. Setter (não fluent) para permitir injeção do hook
// que pode ser construído depois do ChargeService (dependência circular
// implícita: TransferChargeHook depende do TransferService que vive
// no mesmo wire-up).
func (s *ChargeService) SetTransferHook(h chargeTransferHook) { s.transferHook = h }

// SetTxService wires the TransactionService used by MarkPaid to sync the
// source-linked receivable transaction when a Charge transitions to PAID.
func (s *ChargeService) SetTxService(ts *TransactionService) { s.txService = ts }

// SetCollectionResolver wires automatic collection reconciliation after a
// charge is fully paid. Retries re-run this idempotent repair path.
func (s *ChargeService) SetCollectionResolver(r chargeCollectionResolver) {
	s.collectionResolver = r
}

// SetPaymentAllocationResolver wires the explicit payment appropriation rule.
// Production uses EconomicAllocationService; nil keeps legacy charges without
// typed line items backwards compatible in lightweight tests and tools.
func (s *ChargeService) SetPaymentAllocationResolver(r chargePaymentAllocationResolver) {
	s.allocationResolver = r
}

// SetPaymentJournal wires the append-only double-entry ledger used for typed
// payments. PostgreSQL payment transactions fail closed when allocations exist
// but this dependency is absent.
func (s *ChargeService) SetPaymentJournal(j *JournalService) {
	s.paymentJournal = j
}

func (s *ChargeService) Create(ctx context.Context, contractID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount float64, dueDate time.Time) (*charge.Charge, error) {
	return s.CreateWithRefs(ctx, contractID, "", "", tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
}

// CreateWithSourceEventID is the entry point for handlers that materialize
// a charge from an upstream broker event. It writes the deterministic
// sourceEventID into the charge metadata so the repository's ON CONFLICT
// branch (against uq_finance_charges_source_event_id) collapses redeliveries
// into a single row. When the index reports a collision, the existing
// charge is fetched and returned — handlers can treat it as a benign retry.
//
// Callers that don't have an upstream event should keep using CreateWithRefs.
func (s *ChargeService) CreateWithSourceEventID(ctx context.Context, sourceEventID, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount float64, dueDate time.Time) (*charge.Charge, error) {
	c, _, err := s.EnsureWithSourceEventID(ctx, sourceEventID, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	return c, err
}

// EnsureWithSourceEventID is CreateWithSourceEventID plus a created flag,
// allowing repair loops to count only newly materialized rows.
func (s *ChargeService) EnsureWithSourceEventID(ctx context.Context, sourceEventID, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount float64, dueDate time.Time) (*charge.Charge, bool, error) {
	c, err := charge.New(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create charge entity: %w", err)
	}
	c.DealID = dealID
	c.LeadID = leadID
	c.SourceEventID = sourceEventID

	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, charge.ErrChargeAlreadyExists) {
			// Idempotent path — fetch the row that won the race and return it.
			existing, gerr := s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
			if gerr != nil {
				return nil, false, gerr
			}
			return existing, false, nil
		}
		return nil, false, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "charge.created",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":          string(c.Status),
			"charge_type":     string(c.Type),
			"amount":          c.TotalAmount.Float(),
			"contract_id":     c.ContractID,
			"reference_month": c.ReferenceMonth,
			"due_date":        c.DueDate,
		},
		Metadata: map[string]any{"path": "CreateWithSourceEventID", "source_event_id": sourceEventID},
	})
	return c, true, nil
}

// CreateWithRefs persists a charge while denormalizing upstream deal_id /
// lead_id into the charge metadata. Use this when the caller knows which
// deal originated the charge — the original Create signature is preserved
// for legacy callers and just delegates here with empty refs.
//
// Why denormalize instead of joining: charges live in JSONB and the only
// existing FK is contract_id. To answer "all charges of deal X" or "lead's
// total balance" without N+1 we'd otherwise have to traverse contract→deal
// at query time. Storing the refs on the charge is the cheapest fix; the
// new index on `(tenant_id, metadata->>'deal_id')` makes lookups O(log n).
func (s *ChargeService) CreateWithRefs(ctx context.Context, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount float64, dueDate time.Time) (*charge.Charge, error) {
	c, err := charge.New(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	if err != nil {
		return nil, fmt.Errorf("failed to create charge entity: %w", err)
	}
	c.DealID = dealID
	c.LeadID = leadID

	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "charge.created",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":          string(c.Status),
			"charge_type":     string(c.Type),
			"amount":          c.TotalAmount.Float(),
			"contract_id":     c.ContractID,
			"reference_month": c.ReferenceMonth,
			"due_date":        c.DueDate,
		},
		Metadata: map[string]any{"path": "CreateWithRefs"},
	})
	return c, nil
}

// CreateWithComposition is the manual-posting entry point used by the UI to
// launch a charge with a line-item breakdown ("valores inclusos na cobrança")
// and/or a discount. When items are present, the authoritative amount is the
// sum of their cents so charge.ValidateLineItems (Σ items == amount, ±1¢)
// always holds — the caller's amount is overridden. The discount is applied
// via CalculateTotal (TotalAmount = amount + lateFee + interest - discount).
func (s *ChargeService) CreateWithComposition(ctx context.Context, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount, discount float64, dueDate time.Time, items []charge.LineItem) (*charge.Charge, error) {
	if len(items) > 0 {
		var sumCents int64
		for _, it := range items {
			sumCents += it.AmountCents
		}
		amount = float64(sumCents) / 100.0
	}

	var c *charge.Charge
	var err error
	if len(items) > 0 {
		c, err = charge.NewWithLineItems(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate, items)
	} else {
		c, err = charge.New(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create charge entity: %w", err)
	}
	c.DealID = dealID
	c.LeadID = leadID

	if discount > 0 {
		if discount > amount {
			return nil, fmt.Errorf("discount (%.2f) cannot exceed amount (%.2f)", discount, amount)
		}
		c.Discount = valueobjects.NewMoneyFromFloat(discount)
		c.CalculateTotal()
	}

	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "charge.created",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":          string(c.Status),
			"charge_type":     string(c.Type),
			"amount":          c.Amount.Float(),
			"discount":        c.Discount.Float(),
			"total_amount":    c.TotalAmount.Float(),
			"contract_id":     c.ContractID,
			"reference_month": c.ReferenceMonth,
			"line_items":      len(items),
		},
		Metadata: map[string]any{"path": "CreateWithComposition"},
	})
	return c, nil
}

func (s *ChargeService) CreateWithCompositionFromRequest(ctx context.Context, sourceEventID, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType charge.ChargeType, description, referenceMonth string, amount, discount float64, dueDate time.Time, items []charge.LineItem) (*charge.Charge, error) {
	sourceEventID = strings.TrimSpace(sourceEventID)
	if sourceEventID == "" {
		return s.CreateWithComposition(ctx, contractID, dealID, leadID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, discount, dueDate, items)
	}
	if len(items) > 0 {
		var sumCents int64
		for _, it := range items {
			sumCents += it.AmountCents
		}
		amount = float64(sumCents) / 100.0
	}
	var c *charge.Charge
	var err error
	if len(items) > 0 {
		c, err = charge.NewWithLineItems(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate, items)
	} else {
		c, err = charge.New(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create charge entity: %w", err)
	}
	c.DealID = dealID
	c.LeadID = leadID
	c.SourceEventID = sourceEventID
	if discount > 0 {
		if discount > amount {
			return nil, fmt.Errorf("discount (%.2f) cannot exceed amount (%.2f)", discount, amount)
		}
		c.Discount = valueobjects.NewMoneyFromFloat(discount)
		c.CalculateTotal()
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, charge.ErrChargeAlreadyExists) {
			existing, gerr := s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
			if gerr != nil {
				return nil, gerr
			}
			return existing, nil
		}
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "charge.created",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":          string(c.Status),
			"charge_type":     string(c.Type),
			"amount":          c.Amount.Float(),
			"discount":        c.Discount.Float(),
			"total_amount":    c.TotalAmount.Float(),
			"contract_id":     c.ContractID,
			"reference_month": c.ReferenceMonth,
			"line_items":      len(items),
		},
		Metadata: map[string]any{"path": "CreateWithCompositionFromRequest", "source_event_id": sourceEventID},
	})
	return c, nil
}

func (s *ChargeService) CreateBatch(ctx context.Context, charges []*charge.Charge) error {
	for _, c := range charges {
		if c == nil {
			continue
		}

		if err := s.repo.Create(ctx, c); err != nil {
			return err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "charge.created",
			EntityType: "Charge",
			EntityID:   c.ID,
			After: map[string]any{
				"status":          string(c.Status),
				"charge_type":     string(c.Type),
				"amount":          c.TotalAmount.Float(),
				"contract_id":     c.ContractID,
				"reference_month": c.ReferenceMonth,
			},
			Metadata: map[string]any{"path": "CreateBatch"},
		})
	}
	return nil
}

func (s *ChargeService) GetByID(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, charge.ErrNotFound
	}
	return c, nil
}

// GetByIDForUpdate locks one canonical receivable inside the caller-owned SQL
// transaction. InvoiceService uses this as the shared serialization point for
// issuance reservations and cash application against the same Charge.
func (s *ChargeService) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	if database.TxFromCtx(ctx) == nil {
		return nil, errors.New("charge lock requires a transaction")
	}
	lockingRepo, ok := s.repo.(chargeLockingRepository)
	if !ok {
		return nil, errors.New("charge repository does not support row locking")
	}
	c, err := lockingRepo.GetByIDForUpdate(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, charge.ErrNotFound
	}
	return c, nil
}

func (s *ChargeService) Update(ctx context.Context, c *charge.Charge) error {
	expectedVersion := c.UpdatedAt
	// Snapshot do status anterior para o audit distinguir um cancelamento
	// de uma edição comum. Best-effort: se a leitura falhar, segue sem o
	// before — o repo.Update abaixo é a fonte de verdade.
	var beforeStatus string
	if existing, err := s.repo.GetByID(ctx, c.TenantID, c.ID); err == nil && existing != nil {
		beforeStatus = string(existing.Status)
	}
	updated, err := updateChargeIfCurrent(ctx, s.repo, c, expectedVersion)
	if err != nil {
		return err
	}
	if !updated {
		return charge.ErrConcurrentModification
	}
	action := "charge.updated"
	if c.Status == charge.StatusCancelled && beforeStatus != string(charge.StatusCancelled) {
		action = "charge.cancelled"
	}
	audit.Record(ctx, audit.Entry{
		Action:     action,
		EntityType: "Charge",
		EntityID:   c.ID,
		Before:     map[string]any{"status": beforeStatus},
		After:      map[string]any{"status": string(c.Status), "amount": c.TotalAmount.Float()},
		Metadata:   map[string]any{"path": "Update"},
	})
	return nil
}

// ApplySaleEconomicComposition freezes the economic waterfall on the SALE
// receivables after the commission plan has been resolved. The face value is
// split into owner proceeds, broker/partner obligations and the agency's
// retained share. The charge amount itself never changes; only its
// line-item ownership metadata is enriched before any payment is applied.
//
// partnerPayableCents is the owner-company share in a cross-company deal. It
// is kept separate from broker commissions so the later transfer breakdown
// can explain both obligations without pretending the partner is an agency
// expense.
func (s *ChargeService) ApplySaleEconomicComposition(
	ctx context.Context,
	tenantID, contractID string,
	plan ResolvedCommissionPlan,
	partnerPayableCents int64,
	partnerID string,
	royaltyPayableCents int64,
	royaltyPayeeID string,
) error {
	if s == nil || s.repo == nil {
		return errors.New("charge service is not configured")
	}
	if plan.TotalAmountCents < 0 || partnerPayableCents < 0 || royaltyPayableCents < 0 {
		return errors.New("sale economic composition cannot contain negative amounts")
	}
	charges, _, err := s.repo.List(ctx, tenantID, contractID, "", "", 1000, 0)
	if err != nil {
		return fmt.Errorf("list sale charges for composition: %w", err)
	}
	saleCharges := make([]*charge.Charge, 0, len(charges))
	var totalSaleCents int64
	for _, c := range charges {
		if c == nil || c.Type != charge.TypeSale {
			continue
		}
		saleCharges = append(saleCharges, c)
		totalSaleCents += c.Amount.Cents()
	}
	if len(saleCharges) == 0 {
		return nil
	}
	if totalSaleCents <= 0 {
		return errors.New("sale charges have no positive economic base")
	}

	var brokerCommissionCents int64
	for _, share := range plan.Shares {
		if share.AmountCents > 0 {
			brokerCommissionCents += share.AmountCents
		}
	}
	// The residual of the resolved closing-company pool belongs to the agency.
	// When no broker is configured, the full commission pool remains an agency
	// amount pending later distribution rather than disappearing from the
	// owner's settlement.
	// Franchise royalty is paid out of the closing agency's commission share;
	// it is not an extra seller deduction. Keep it as a third-party obligation
	// and expose only the residual as effective agency revenue.
	agencyRetainedCents := plan.TotalAmountCents - brokerCommissionCents - royaltyPayableCents
	if agencyRetainedCents < 0 {
		return fmt.Errorf("broker and royalty obligations exceed resolved closing commission pool: brokers=%d royalty=%d pool=%d", brokerCommissionCents, royaltyPayableCents, plan.TotalAmountCents)
	}
	commissionPoolCents := plan.TotalAmountCents + partnerPayableCents
	if partnerPayableCents > 0 && strings.TrimSpace(partnerID) == "" {
		return errors.New("partner identity is required for a cross-company sale obligation")
	}
	if royaltyPayableCents > 0 && strings.TrimSpace(royaltyPayeeID) == "" {
		return errors.New("royalty payee identity is required for a franchise sale obligation")
	}
	if commissionPoolCents > totalSaleCents {
		return fmt.Errorf("sale obligations exceed sale value: obligations=%d sale=%d", commissionPoolCents, totalSaleCents)
	}

	ownerByCharge := allocateCentsProportionally(totalSaleCents-commissionPoolCents, saleCharges)
	brokerByCharge := allocateCentsProportionally(brokerCommissionCents, saleCharges)
	partnerByCharge := allocateCentsProportionally(partnerPayableCents, saleCharges)
	royaltyByCharge := allocateCentsProportionally(royaltyPayableCents, saleCharges)
	retainedByCharge := allocateCentsProportionally(agencyRetainedCents, saleCharges)
	for i, c := range saleCharges {
		items := make([]charge.LineItem, 0, 4)
		appendItem := func(description, sourceType, sourceID string, amount int64, economicOwnerID string, allocationType charge.AllocationType) {
			if amount <= 0 {
				return
			}
			items = append(items, charge.LineItem{
				Description: description, SourceType: sourceType, SourceID: sourceID,
				AmountCents: amount, EconomicOwnerID: economicOwnerID,
				SettlementMode: charge.SettlementModeCollection, AllocationType: allocationType,
				RuleVersionID: plan.PlanHash,
			})
		}
		appendItem("Saldo do proprietário", "sale_owner_proceeds", "owner_proceeds", ownerByCharge[i], c.OwnerID, charge.AllocationTypeOwnerPayable)
		appendItem("Comissões a corretores", "sale_commission_payable", "broker_commissions", brokerByCharge[i], c.TenantID, charge.AllocationTypeRevenue)
		appendItem("Comissão da empresa captadora", "sale_partner_payable", "owner_company_commission", partnerByCharge[i], partnerID, charge.AllocationTypePartnerPayable)
		appendItem("Royalty de franquia", "sale_royalty_payable", "franchise_royalty", royaltyByCharge[i], royaltyPayeeID, charge.AllocationTypeRoyaltyPayable)
		appendItem("Retenção da imobiliária", "sale_agency_retained", "agency_retained", retainedByCharge[i], c.TenantID, charge.AllocationTypeRevenue)
		if len(items) == 0 {
			return fmt.Errorf("sale charge %s produced an empty economic composition", c.ID)
		}
		if len(c.Payments) > 0 || c.Status == charge.StatusPaid || c.Status == charge.StatusPartiallyPaid {
			if len(c.LineItems) == 0 {
				// Reparo: cobrança paga antes da composição econômica existir
				// (plano criado antes da assinatura, sem deal_id no passado).
				// Aplica a composição agora; a baixa retry re-aloca o pagamento.
				if err := c.SetLineItems(items); err != nil {
					return fmt.Errorf("build economic composition for sale charge %s: %w", c.ID, err)
				}
				if err := s.Update(ctx, c); err != nil {
					return fmt.Errorf("persist economic composition for sale charge %s: %w", c.ID, err)
				}
				continue
			}
			if err := ensureSaleCompositionMatches(c, items); err != nil {
				return fmt.Errorf("sale charge %s was already paid with another composition: %w", c.ID, err)
			}
			continue
		}
		if err := c.SetLineItems(items); err != nil {
			return fmt.Errorf("build economic composition for sale charge %s: %w", c.ID, err)
		}
		if err := s.Update(ctx, c); err != nil {
			return fmt.Errorf("persist economic composition for sale charge %s: %w", c.ID, err)
		}
	}
	return nil
}

// ensureSaleCompositionBeforePayment aplica a composição econômica de venda a
// uma charge SALE sem line items (ex.: plano/cobrança criados antes da
// assinatura ou via wizard commit sem deal_id no passado). Com o resolver
// configurado, o plano canônico do deal é re-resolvido e a charge passa a ter
// owner/proprietário e comissões na alocação.
func (s *ChargeService) ensureSaleCompositionBeforePayment(ctx context.Context, tenantID, id string) error {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if c == nil || c.Type != charge.TypeSale || len(c.LineItems) > 0 {
		return nil
	}
	if strings.TrimSpace(c.DealID) == "" || s.salePlanResolver == nil {
		return nil
	}
	// Snapshot congelado de comissões primeiro: a baixa precisa encontrar as
	// obrigações (Commission + payout_order) com o MESMO plan hash para
	// financiar o pool. Deal/plano criados pelo wizard antes da assinatura
	// nunca passaram pelo deal.contract.signed — sem isso o FundAllocations
	// rejeitaria o recebimento.
	if s.salePlanMaterializer != nil {
		if err := s.salePlanMaterializer.EnsureMaterialized(ctx, tenantID, c.ContractID, c.DealID); err != nil {
			return fmt.Errorf("ensure sale commission snapshot: %w", err)
		}
	}
	plan, err := s.salePlanResolver.ResolvePlanForDeal(ctx, tenantID, c.DealID)
	if err != nil {
		return fmt.Errorf("resolve sale commission plan: %w", err)
	}
	if plan == nil || plan.TotalAmountCents <= 0 {
		return nil
	}
	if err := s.ApplySaleEconomicComposition(ctx, tenantID, c.ContractID, *plan, 0, "", 0, ""); err != nil {
		return fmt.Errorf("apply sale economic composition: %w", err)
	}
	return nil
}

// rebuildAllocationsForPayment re-derives the economic allocations of an
// existing zero-allocation payment now that the charge carries line items.
// It uses the force-rebuild resolver when available (the regular builder
// refuses to mix allocations when the charge history has unclassified cash)
// and applies the sale commission funding so the frozen pool is financed.
func (s *ChargeService) rebuildAllocationsForPayment(ctx context.Context, current *charge.Charge, payment *charge.PaymentEntry) (charge.AllocationMethod, []charge.Allocation, error) {
	if s == nil || s.allocationResolver == nil || payment == nil {
		return "", nil, nil
	}
	allocationMethod, allocations, err := s.buildPaymentAllocationsForRepair(ctx, current, payment.ID, payment.AmountCents)
	if err != nil {
		return "", nil, err
	}
	if current.Type == charge.TypeSale && hasSaleCommissionPool(current) && len(allocations) > 0 {
		if s.saleFunding == nil {
			if s.db != nil {
				return "", nil, errors.New("sale commission funding service is required for a signed sale receipt")
			}
			return allocationMethod, allocations, nil
		}
		allocations, err = s.saleFunding.FundAllocations(ctx, current, payment.ID, payment.PaidAt, allocations)
		if err != nil {
			return "", nil, fmt.Errorf("fund sale commissions: %w", err)
		}
	}
	return allocationMethod, allocations, nil
}

// buildPaymentAllocationsForRepair prefers the force-rebuild resolver
// (chargePaymentAllocationRebuilder) over the regular builder so historical
// unallocated payments do not block the repair with an empty result.
func (s *ChargeService) buildPaymentAllocationsForRepair(ctx context.Context, current *charge.Charge, paymentID string, paymentCents int64) (charge.AllocationMethod, []charge.Allocation, error) {
	if rebuilder, ok := s.allocationResolver.(chargePaymentAllocationRebuilder); ok {
		return rebuilder.RebuildPaymentAllocations(ctx, current, paymentID, paymentCents)
	}
	return s.allocationResolver.BuildPaymentAllocations(ctx, current, paymentID, paymentCents)
}

// repairUnallocatedPayments re-derives allocations for every zero-allocation
// payment of an already-paid charge and persists the repaired snapshot once.
func (s *ChargeService) repairUnallocatedPayments(ctx context.Context, current *charge.Charge, expectedVersion time.Time, versioned chargeVersionedRepository, supportsCAS bool) (bool, error) {
	rebuilt := false
	for i := range current.Payments {
		payment := &current.Payments[i]
		if payment.AmountCents <= 0 || len(payment.Allocations) > 0 {
			continue
		}
		allocMethod, allocations, err := s.rebuildAllocationsForPayment(ctx, current, payment)
		if err != nil {
			return false, err
		}
		if len(allocations) == 0 {
			continue
		}
		payment.Allocations = allocations
		payment.AllocationMethod = allocMethod
		rebuilt = true
	}
	if !rebuilt {
		return false, nil
	}
	if supportsCAS {
		updated, err := versioned.UpdateIfVersion(ctx, current, expectedVersion)
		if err != nil {
			return false, err
		}
		if !updated {
			return false, charge.ErrConcurrentModification
		}
	} else if err := s.repo.Update(ctx, current); err != nil {
		return false, err
	}
	// O pagamento original nunca teve journal (não tinha alocação quando foi
	// lançado). O reparo grava agora o lançamento alocado — idempotente por
	// source_event_id, então qualquer retry colapsa no mesmo entry.
	repairedPayments := make([]*charge.PaymentEntry, 0, len(current.Payments))
	for i := range current.Payments {
		if current.Payments[i].AmountCents > 0 && len(current.Payments[i].Allocations) > 0 {
			repairedPayments = append(repairedPayments, &current.Payments[i])
		}
	}
	if s.paymentJournal != nil {
		for _, payment := range repairedPayments {
			if err := s.postAllocatedPaymentJournal(ctx, current, payment); err != nil {
				return false, fmt.Errorf("post repaired payment journal: %w", err)
			}
		}
	}
	return true, nil
}

type proportionalChargeRemainder struct {
	index     int
	amount    int64
	remainder int64
}

func allocateCentsProportionally(totalCents int64, charges []*charge.Charge) []int64 {
	allocated := make([]int64, len(charges))
	if totalCents <= 0 || len(charges) == 0 {
		return allocated
	}
	var baseCents int64
	shares := make([]proportionalChargeRemainder, len(charges))
	for _, c := range charges {
		baseCents += c.Amount.Cents()
	}
	if baseCents <= 0 {
		return allocated
	}
	allocatedBase := int64(0)
	for i, c := range charges {
		numerator := totalCents * c.Amount.Cents()
		allocated[i] = numerator / baseCents
		shares[i] = proportionalChargeRemainder{index: i, amount: allocated[i], remainder: numerator % baseCents}
		allocatedBase += allocated[i]
	}
	sort.SliceStable(shares, func(i, j int) bool { return shares[i].remainder > shares[j].remainder })
	for residual := totalCents - allocatedBase; residual > 0; residual-- {
		allocated[shares[(totalCents-allocatedBase-residual)%int64(len(shares))].index]++
	}
	return allocated
}

func ensureSaleCompositionMatches(c *charge.Charge, expected []charge.LineItem) error {
	if len(c.LineItems) != len(expected) {
		return fmt.Errorf("line-item count is %d, expected %d", len(c.LineItems), len(expected))
	}
	for i, item := range expected {
		actual := c.LineItems[i]
		if actual.Description != item.Description || actual.AmountCents != item.AmountCents ||
			actual.SourceType != item.SourceType || actual.SourceID != item.SourceID ||
			actual.EconomicOwnerID != item.EconomicOwnerID || actual.AllocationType != item.AllocationType {
			return fmt.Errorf("line item %d does not match the resolved sale plan", i)
		}
	}
	return nil
}

// Cancel marca a charge como CANCELLED preservando o motivo no audit.
// Diferente de Update, NÃO chama o caminho genérico de audit — gravamos um
// único evento "charge.cancelled" com o reason em metadata para evitar
// duplicação (Update também emite "charge.cancelled" quando detecta a
// transição de status, mas sem reason; ver Update acima). actor_id e
// tenant_id são resolvidos automaticamente pelo audit.Record via ctx.
//
// Refund/estorno de já PAID é fora de escopo aqui — a primitiva ainda não
// existe. Mantemos PAID terminal.
func (s *ChargeService) Cancel(ctx context.Context, tenantID, id, reason string) (*charge.Charge, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, charge.ErrChargeNotFound
	}
	if c.Status == charge.StatusPaid {
		return nil, charge.ErrChargeAlreadyPaid
	}
	if c.Status == charge.StatusCancelled {
		return nil, charge.ErrChargeAlreadyCancelled
	}
	beforeStatus := c.Status
	expectedVersion := c.UpdatedAt
	if err := c.Cancel(); err != nil {
		return nil, err
	}
	updated, err := updateChargeIfCurrent(ctx, s.repo, c, expectedVersion)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, charge.ErrConcurrentModification
	}
	audit.Record(ctx, audit.Entry{
		Action:     "charge.cancelled",
		EntityType: "Charge",
		EntityID:   c.ID,
		Before:     map[string]any{"status": string(beforeStatus)},
		After:      map[string]any{"status": string(c.Status)},
		Metadata:   map[string]any{"path": "Cancel", "reason": reason},
	})
	// G4 do plano de inadimplência: cobrança cancelada precisa SAIR do caso.
	// Antes o cancelamento só mudava a cobrança e o caso continuava exibindo a
	// dívida antiga (charge_ids e total_debt obsoletos) até alguém pagar outra
	// cobrança do mesmo caso. A reconciliação recalcula o caso sem esta
	// cobrança e resolve se não sobrou nada.
	s.reconcileCollection(ctx, c)
	return c, nil
}

// reconcileCollection avisa o motor de inadimplência que uma cobrança mudou de
// estado e o caso precisa ser recalculado.
//
// Best-effort de propósito: a cobrança já foi persistida e auditada, e falhar
// aqui não pode desfazer o cancelamento. O próximo tick do motor recalcula o
// caso de qualquer forma (a descoberta é por estado), então uma falha transitória
// se corrige sozinha em até uma hora.
func (s *ChargeService) reconcileCollection(ctx context.Context, c *charge.Charge) {
	if s.collectionResolver == nil || c == nil {
		return
	}
	if err := s.collectionResolver.ResolveChargePayment(ctx, c.TenantID, c.ID); err != nil {
		log.Printf("[ChargeService] collection reconcile failed for charge %s: %v", c.ID, err)
	}
}

// Reverse transita PAID → REVERSED para estorno contábil de charge paga.
func (s *ChargeService) Reverse(ctx context.Context, tenantID, id, reason string) (*charge.Charge, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, charge.ErrChargeNotFound
	}
	beforeStatus := c.Status
	if beforeStatus == charge.StatusReversed {
		// Idempotent retry: terminal state was already persisted and audited.
		return c, nil
	}
	expectedVersion := c.UpdatedAt
	if err := c.Reverse(reason); err != nil {
		return nil, err
	}
	updated, err := updateChargeIfCurrent(ctx, s.repo, c, expectedVersion)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, charge.ErrConcurrentModification
	}
	audit.Record(ctx, audit.Entry{
		Action:     "charge.reversed",
		EntityType: "Charge",
		EntityID:   c.ID,
		Before:     map[string]any{"status": string(beforeStatus)},
		After:      map[string]any{"status": string(c.Status)},
		Metadata:   map[string]any{"path": "Reverse", "reason": reason},
	})
	// G4: estorno também tira a cobrança do caso (ela deixa de ser dívida).
	s.reconcileCollection(ctx, c)
	return c, nil
}

func (s *ChargeService) MarkPaid(ctx context.Context, tenantID, id, paymentMethod string, paidAmount float64) (*charge.Charge, error) {
	return s.MarkPaidIdempotent(ctx, tenantID, id, paymentMethod, paidAmount, uuid.NewString(), time.Now())
}

// MarkPaidIdempotent records a payment command exactly once. The paymentID
// must be stable across caller retries. PostgreSQL writes use optimistic
// concurrency and are re-applied to the latest charge snapshot on conflict.
func (s *ChargeService) MarkPaidIdempotent(ctx context.Context, tenantID, id, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time) (*charge.Charge, error) {
	return s.markPaidCommand(ctx, tenantID, id, "", paymentMethod, paidAmount, paymentID, paidAt)
}

// MarkInvoicePaidIdempotent applies cash allocated by one specific Invoice.
// The invoice identity releases only that invoice's open reservation while all
// other issued/overdue invoices remain protected from overpayment.
func (s *ChargeService) MarkInvoicePaidIdempotent(ctx context.Context, tenantID, id, invoiceID, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time) (*charge.Charge, error) {
	if strings.TrimSpace(invoiceID) == "" {
		return nil, errors.New("invoice id is required")
	}
	return s.markPaidCommand(ctx, tenantID, id, strings.TrimSpace(invoiceID), paymentMethod, paidAmount, paymentID, paidAt)
}

func (s *ChargeService) markPaidCommand(ctx context.Context, tenantID, id, excludedInvoiceID, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time) (*charge.Charge, error) {
	mutation := &chargePaymentMutation{}
	if s.paymentTx != nil && database.TxFromCtx(ctx) == nil {
		var paid *charge.Charge
		err := s.paymentTx(ctx, func(txCtx context.Context) error {
			var err error
			paid, err = s.markPaidIdempotent(txCtx, tenantID, id, excludedInvoiceID, paymentMethod, paidAmount, paymentID, paidAt, mutation)
			return err
		})
		if err != nil {
			return nil, err
		}
		s.recordPaymentAudit(ctx, paid, paymentMethod, paidAmount, paymentID, mutation)
		return paid, nil
	}
	paid, err := s.markPaidIdempotent(ctx, tenantID, id, excludedInvoiceID, paymentMethod, paidAmount, paymentID, paidAt, mutation)
	if err != nil {
		return nil, err
	}
	// Lightweight tests/dev fakes record immediately. When a caller already
	// owns a managed transaction, audit.Record queues the entry until the outer
	// RunInTx commits; the normal HTTP path records after its transaction above.
	if s.db == nil || database.TxFromCtx(ctx) != nil {
		s.recordPaymentAudit(ctx, paid, paymentMethod, paidAmount, paymentID, mutation)
	}
	return paid, nil
}

func (s *ChargeService) markPaidIdempotent(ctx context.Context, tenantID, id, excludedInvoiceID, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time, mutation *chargePaymentMutation) (*charge.Charge, error) {
	if strings.TrimSpace(paymentID) == "" {
		return nil, errors.New("payment id is required")
	}
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	// Cobranças de venda criadas sem composição econômica (ex.: plano criado
	// antes da assinatura) recebem a composição na baixa, para que a alocação,
	// o repasse e a comissão nasçam automaticamente.
	if err := s.ensureSaleCompositionBeforePayment(ctx, tenantID, id); err != nil {
		return nil, err
	}

	var (
		c            *charge.Charge
		beforeStatus charge.Status
		changed      bool
		completed    bool
	)
	versioned, supportsCAS := s.repo.(chargeVersionedRepository)
	for attempt := 0; attempt < 8; attempt++ {
		var current *charge.Charge
		var err error
		if attempt == 0 && database.TxFromCtx(ctx) != nil {
			lockingRepo, ok := s.repo.(chargeLockingRepository)
			if !ok && s.db != nil {
				return nil, errors.New("charge repository does not support payment locking")
			}
			if ok {
				current, err = lockingRepo.GetByIDForUpdate(ctx, tenantID, id)
			} else {
				current, err = s.repo.GetByID(ctx, tenantID, id)
			}
		} else {
			current, err = s.repo.GetByID(ctx, tenantID, id)
		}
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, charge.ErrNotFound
		}
		expectedVersion := current.UpdatedAt
		beforeStatus = current.Status
		existingPayment := chargePaymentByID(current, paymentID)
		if existingPayment != nil {
			paymentCents := valueobjects.NewMoneyFromFloat(paidAmount).Cents()
			if existingPayment.AmountCents != paymentCents || existingPayment.Method != strings.TrimSpace(paymentMethod) {
				return nil, fmt.Errorf("cannot apply payment: payment id was already used with different data: %w", charge.ErrInvalidState)
			}
			// Reparo: pagamento existente sem alocação porque a charge não tinha
			// composição econômica na época (plano criado antes da assinatura).
			// Re-executa a alocação e roda o downstream (repasse/comissão/journal).
			if len(existingPayment.Allocations) == 0 && len(current.LineItems) > 0 && s.allocationResolver != nil {
				allocationMethod, allocations, allocErr := s.rebuildAllocationsForPayment(ctx, current, existingPayment)
				if allocErr != nil {
					return nil, fmt.Errorf("rebuild payment allocations: %w", allocErr)
				}
				if len(allocations) > 0 {
					existingPayment.Allocations = allocations
					existingPayment.AllocationMethod = allocationMethod
					if supportsCAS {
						updated, casErr := versioned.UpdateIfVersion(ctx, current, expectedVersion)
						if casErr != nil {
							return nil, casErr
						}
						if !updated {
							return nil, charge.ErrConcurrentModification
						}
					} else if updateErr := s.repo.Update(ctx, current); updateErr != nil {
						return nil, updateErr
					}
					changed = true
				}
			}
			c = current
			completed = true
			break
		}
		// Reparo: cobrança já quitada cujo pagamento foi registrado sem alocação
		// porque a composição econômica não existia na época (plano/wizard sem
		// deal_id no passado). O retry do reconcile chega com outro paymentID
		// (a UI gera uma chave de idempotência nova por clique); detectamos a
		// charge paga com pagamentos desalocados e re-executamos a alocação
		// do(s) pagamento(s) existente(s) antes de seguir para o downstream.
		if len(current.LineItems) > 0 && current.Status == charge.StatusPaid && current.TotalAmount.Cents() > 0 {
			paidCents := int64(0)
			if current.PaidAmount != nil {
				paidCents = current.PaidAmount.Cents()
			}
			if paidCents >= current.TotalAmount.Cents() {
				repaired, repairErr := s.repairUnallocatedPayments(ctx, current, expectedVersion, versioned, supportsCAS)
				if repairErr != nil {
					return nil, repairErr
				}
				if repaired {
					c = current
					changed = true
					completed = true
					break
				}
			}
		}
		if s.db != nil {
			if s.invoiceReservations == nil {
				return nil, ErrChargeInvoiceReservationLookupRequired
			}
			reservedCents, reservationErr := s.invoiceReservations.SumOpenAmountCentsByChargeID(ctx, tenantID, id, excludedInvoiceID)
			if reservationErr != nil {
				return nil, fmt.Errorf("load open invoice reservations for charge %s: %w", id, reservationErr)
			}
			paidCents := int64(0)
			if current.PaidAmount != nil {
				paidCents = current.PaidAmount.Cents()
			}
			paymentCents := valueobjects.NewMoneyFromFloat(paidAmount).Cents()
			if reservedCents < 0 || paymentCents <= 0 || paidCents+paymentCents+reservedCents > current.TotalAmount.Cents() {
				return nil, fmt.Errorf(
					"%w: paid=%d cents payment=%d cents open_invoices=%d cents total=%d cents: %w",
					ErrChargePaymentExceedsUnreservedBalance, paidCents, paymentCents, reservedCents, current.TotalAmount.Cents(), charge.ErrInvalidState,
				)
			}
		}
		allocationMethod := charge.AllocationMethod("")
		var allocations []charge.Allocation
		if s.allocationResolver != nil {
			allocationMethod, allocations, err = s.allocationResolver.BuildPaymentAllocations(
				ctx, current, paymentID, valueobjects.NewMoneyFromFloat(paidAmount).Cents(),
			)
			if err != nil {
				return nil, fmt.Errorf("build payment allocations: %w", err)
			}
		}
		if current.Type == charge.TypeSale && hasSaleCommissionPool(current) {
			if s.saleFunding == nil {
				if s.db != nil {
					return nil, errors.New("sale commission funding service is required for a signed sale receipt")
				}
			} else {
				allocations, err = s.saleFunding.FundAllocations(ctx, current, paymentID, paidAt, allocations)
				if err != nil {
					return nil, fmt.Errorf("fund sale commissions: %w", err)
				}
			}
		}
		applied, err := current.ApplyPaymentWithAllocations(paymentID, paidAmount, paymentMethod, paidAt, allocationMethod, allocations)
		if err != nil {
			return nil, fmt.Errorf("cannot apply payment: %w: %w", err, charge.ErrInvalidState)
		}
		c = current
		if !applied {
			// Benign retry. Downstream idempotent hooks still run below so a
			// previous publish failure can heal on the next delivery.
			changed = false
			completed = true
			break
		}

		if supportsCAS {
			updated, err := versioned.UpdateIfVersion(ctx, current, expectedVersion)
			if err != nil {
				return nil, err
			}
			if !updated {
				// Conflito de CAS: a charge foi atualizada concorrentemente
				// (ex.: DelinquencyEngine marcando OVERDUE). Aguarda um backoff
				// curto e re-lê para acumular em vez de estourar 409 pro cliente.
				select {
				case <-time.After(time.Duration(attempt+1) * 40 * time.Millisecond):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				continue
			}
		} else if err := s.repo.Update(ctx, current); err != nil {
			return nil, err
		}
		changed = true
		completed = true
		break
	}
	if c == nil || !completed {
		return nil, charge.ErrConcurrentModification
	}
	// In the transactional production path a persisted payment implies every
	// downstream write and the outbox row committed with it. A duplicate command
	// therefore returns immediately and must not enqueue a second event.
	if !changed && s.db != nil {
		return c, nil
	}
	if changed {
		payment := chargePaymentByID(c, paymentID)
		if payment != nil && len(payment.Allocations) > 0 {
			if s.paymentJournal == nil {
				if s.db != nil {
					return nil, errors.New("payment journal is required for an allocated payment")
				}
			} else if err := s.postAllocatedPaymentJournal(ctx, c, payment); err != nil {
				return nil, fmt.Errorf("post allocated payment journal: %w", err)
			}
		}
	}

	if s.rentalConfigRepo != nil && c.Status == charge.StatusPaid {
		cfg, err := s.rentalConfigRepo.GetByContractID(ctx, tenantID, c.ContractID)
		if err != nil {
			return nil, fmt.Errorf("load rental charge config after payment: %w", err)
		}
		if cfg != nil && !cfg.FirstChargePaid {
			ay, am, ad := c.DueDate.Date()
			by, bm, bd := cfg.FirstChargeDate.Date()
			if ay == by && am == bm && ad == bd {
				cfg.FirstChargePaid = true
				if err := s.rentalConfigRepo.Update(ctx, cfg); err != nil {
					return nil, fmt.Errorf("mark first rental charge paid: %w", err)
				}
			}
		}
	}

	// FASE D — fechar o ciclo downstream do pagamento.
	//
	// 1. Publica charge.paid pelo eventBus (apenas pagamento total).
	//    EventID determinístico via DeriveNotificationID garante
	//    idempotência.
	//
	// 2. Chama o transferHook para criar 1 OwnerTransfer mensal somente
	//    depois da quitação integral. Pagamento parcial não libera repasse.
	//
	if c.Status == charge.StatusPaid {
		if s.db != nil && (c.Type == charge.TypeRent || c.Type == charge.TypeSale) && s.transferHook == nil {
			return nil, errors.New("transfer hook is required for a paid rent or sale charge")
		}
		if s.transferHook != nil {
			if err := s.transferHook.EnsureMonthlyForChargePaid(ctx, c); err != nil {
				return nil, fmt.Errorf("ensure owner transfer after payment: %w", err)
			}
		}
		if s.db != nil && s.collectionResolver == nil {
			return nil, errors.New("collection resolver is required for an atomic payment")
		}
		if s.collectionResolver != nil {
			if err := s.collectionResolver.ResolveChargePayment(ctx, c.TenantID, c.ID); err != nil {
				return nil, fmt.Errorf("reconcile collection after payment: %w", err)
			}
		}
		// Sync only the receivable created from the same upstream event.
		// contract_id is deliberately not used as a fallback: one contract can
		// have several installments plus commission/expense ledger entries.
		// Manual charges have no reliable Transaction link yet and therefore
		// leave the ledger untouched instead of settling unrelated entries.
		if s.db != nil && c.SourceEventID != "" && s.txService == nil {
			return nil, errors.New("transaction service is required for a source-linked payment")
		}
		if s.txService != nil {
			if _, err := s.txService.SettleReceivableBySourceEventID(ctx, c.TenantID, c.ContractID, c.SourceEventID, paidAt, paymentMethod); err != nil {
				return nil, fmt.Errorf("settle source-linked receivable after payment: %w", err)
			}
		}

		paidAtISO := paidAt.UTC().Format(time.RFC3339)
		if c.PaidAt != nil {
			paidAtISO = c.PaidAt.UTC().Format(time.RFC3339)
		}
		paidAmount := c.TotalAmount.Float()
		if c.PaidAmount != nil {
			paidAmount = c.PaidAmount.Float()
		}
		evt := contracts.ChargePaidEvent{
			EventIDVal: events.DeriveNotificationID("charge.paid", c.TenantID, c.ID),
			Data: contracts.ChargePaidEventPayload{
				ChargeID: c.ID, TenantID: c.TenantID, ContractID: c.ContractID,
				DealID: c.DealID, Type: string(c.Type), Amount: paidAmount,
				Currency: "BRL", PaidAt: paidAtISO,
			},
		}
		if s.eventBus == nil {
			if s.db != nil {
				return nil, errors.New("event bus is required for an atomic full payment")
			}
		} else if tx := database.TxFromCtx(ctx); tx != nil {
			outbox, ok := s.eventBus.(chargeTransactionalOutbox)
			if !ok {
				return nil, errors.New("event bus does not support transactional outbox")
			}
			if _, err := outbox.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
				return nil, fmt.Errorf("enqueue charge.paid atomically: %w", err)
			}
		} else if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			return nil, fmt.Errorf("publish charge.paid: %w", err)
		}
	}

	if mutation != nil {
		mutation.changed = changed
		mutation.beforeStatus = beforeStatus
	}
	return c, nil
}

func (s *ChargeService) postAllocatedPaymentJournal(ctx context.Context, c *charge.Charge, payment *charge.PaymentEntry) error {
	entry, err := newAllocatedPaymentJournalEntry(c, payment)
	if err != nil {
		return err
	}
	sourceEventID := events.DeriveNotificationID("charge.payment.journal", c.TenantID, c.ID, payment.ID)
	return s.paymentJournal.CreateWithSourceEventID(ctx, sourceEventID, entry)
}

func newAllocatedPaymentJournalEntry(c *charge.Charge, payment *charge.PaymentEntry) (*journal.Entry, error) {
	if c == nil || payment == nil {
		return nil, errors.New("charge and payment are required")
	}
	if err := charge.ValidateAllocations(payment.Allocations, payment.AmountCents); err != nil {
		return nil, err
	}
	flow := chargeFlow(c.Type)
	legs := make([]journal.Leg, 0, len(payment.Allocations)+1)
	legs = append(legs, journal.Leg{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: payment.AmountCents, Flow: flow})
	for _, allocation := range payment.Allocations {
		account, err := journalAccountForAllocation(allocation.AllocationType)
		if err != nil {
			return nil, err
		}
		legs = append(legs, journal.Leg{
			Account:            account,
			Direction:          journal.Credit,
			AmountCents:        allocation.AmountCents,
			AllocationID:       allocation.ID,
			ChargeItemID:       allocation.ChargeItemID,
			BeneficiaryPartyID: allocation.BeneficiaryPartyID,
			RuleVersionID:      allocation.RuleVersionID,
			AllocationPurpose:  string(allocation.Purpose),
			RelatedEntityID:    allocation.RelatedEntityID,
			Flow:               flow,
		})
	}
	entry, err := journal.New(c.TenantID, c.ContractID, "Recebimento alocado da cobrança "+c.ID, payment.PaidAt, legs)
	if err != nil {
		return nil, err
	}
	entry.OperationID = payment.ID
	return entry, nil
}

func journalAccountForAllocation(allocationType charge.AllocationType) (journal.Account, error) {
	switch allocationType {
	case charge.AllocationTypeRevenue:
		return journal.AccountRevenue, nil
	case charge.AllocationTypeOwnerPayable:
		return journal.AccountOwnerPayable, nil
	case charge.AllocationTypeCommissionPayable:
		return journal.AccountCommissionPayable, nil
	case charge.AllocationTypePartnerPayable:
		return journal.AccountPartnerPayable, nil
	case charge.AllocationTypeRoyaltyPayable:
		return journal.AccountRoyaltyPayable, nil
	case charge.AllocationTypeTaxWithheld:
		return journal.AccountTaxWithheld, nil
	case charge.AllocationTypeReceivable:
		// Custo de terceiro adiantado pela imobiliária: o recebimento baixa o
		// direito a receber, sem passar por receita.
		return journal.AccountAccountsReceivable, nil
	default:
		return "", fmt.Errorf("unsupported allocation type %q", allocationType)
	}
}

// chargeFlow maps a charge type to the economic modality tag written on the
// journal legs so the plano de contas can break revenue down by sale vs rent.
func chargeFlow(t charge.ChargeType) string {
	switch t {
	case charge.TypeSale:
		return "sale"
	case charge.TypeRent:
		return "rent"
	default:
		return ""
	}
}

func hasChargePayment(c *charge.Charge, paymentID string) bool {
	return chargePaymentByID(c, paymentID) != nil
}

func chargePaymentByID(c *charge.Charge, paymentID string) *charge.PaymentEntry {
	if c == nil {
		return nil
	}
	paymentID = strings.TrimSpace(paymentID)
	for i := range c.Payments {
		entry := &c.Payments[i]
		if entry.ID == paymentID {
			return entry
		}
	}
	return nil
}

func (s *ChargeService) recordPaymentAudit(ctx context.Context, c *charge.Charge, paymentMethod string, paidAmount float64, paymentID string, mutation *chargePaymentMutation) {
	if c == nil || mutation == nil || !mutation.changed {
		return
	}
	paidTotal := float64(0)
	if c.PaidAmount != nil {
		paidTotal = c.PaidAmount.Float()
	}
	record := audit.Record
	if s.paymentAudit != nil {
		record = s.paymentAudit
	}
	record(ctx, audit.Entry{
		Action:     "charge.paid",
		EntityType: "Charge",
		EntityID:   c.ID,
		Before:     map[string]any{"status": string(mutation.beforeStatus)},
		After: map[string]any{
			"status":         string(c.Status),
			"payment_amount": paidAmount,
			"paid_amount":    paidTotal,
			"payment_method": paymentMethod,
		},
		Metadata: map[string]any{"path": "MarkPaidIdempotent", "payment_id": paymentID},
	})
}

func (s *ChargeService) List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return s.repo.List(ctx, tenantID, contractID, status, referenceMonth, limit, offset)
}

func (s *ChargeService) ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return s.repo.ListByPartyIDs(ctx, tenantID, partyIDs, status, referenceMonth, limit, offset)
}

// ListByLeadIDs serve o portal cliente (#460). Filtra charges em que algum
// leadID corresponde tanto à posição de customer (lead_id) quanto à de
// owner (owner_id) — assim inquilino e proprietário compartilham o mesmo
// endpoint sem precisar diferenciar role aqui (frontend ja apresenta
// "Conta a pagar" vs "Repassse a receber" pela perspectiva da role do lead).
func (s *ChargeService) ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return s.repo.ListByLeadIDs(ctx, tenantID, leadIDs, status, referenceMonth, limit, offset)
}

func (s *ChargeService) Search(ctx context.Context, tenantID string, filter charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return s.repo.Search(ctx, tenantID, filter)
}

func (s *ChargeService) ListOverdue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	return s.repo.ListOverdue(ctx, tenantID)
}

// ListInternalCharges atende a rota interna de cobranças por deal/parte.
//
// SECURITY (achado A3): o tenant é explícito e vem da credencial resolvida pelo
// middleware — antes a consulta não tinha predicado de tenant algum.
func (s *ChargeService) ListInternalCharges(ctx context.Context, tenantID, dealID, accountID, role string) ([]*charge.Charge, error) {
	return s.repo.ListByDealAndCustomer(ctx, tenantID, dealID, accountID, role)
}

// NotifyResult summarises the dispatch attempt for a single charge so the
// HTTP layer can surface which channels actually went through and which
// were skipped (e.g. lead has no email on file).
type NotifyResult struct {
	ChargeID     string   `json:"charge_id"`
	Recipient    string   `json:"recipient,omitempty"`
	Channels     []string `json:"channels_sent"`
	SkippedNotes []string `json:"skipped,omitempty"`
}

// NotifyOptions tweaks how Notify renders and credits the message. Both
// fields are optional; zero values mean "use defaults" — the message body
// falls back to the built-in template and the audit entry is credited to
// the empty SentBy.
type NotifyOptions struct {
	// Template, when set, overrides the built-in body. Supports the
	// placeholders documented on renderChargeTemplate (e.g. {tenant_name},
	// {amount}, {due_date}, {total_amount}, {days_overdue},
	// {description}). Unknown placeholders are left as-is so operators see
	// their typo verbatim.
	Template string
	// SentBy attributes the dispatch in RemindersSent. Convention:
	//   "user:<account_id>"   manual click in the admin UI
	//   "rule:<rule_id>"      auto-régua tick
	//   ""                    legacy / unknown
	SentBy string
	// SentByLabel is the human-readable snapshot of the actor (operator
	// name, rule name, etc.) at the moment of dispatch. Persisted on the
	// audit so the UI stays meaningful even when the underlying broker /
	// rule is later removed or renamed.
	SentByLabel string
	// Channels narrows the dispatch to a specific subset (e.g. ["email"]).
	// Nil/empty means "use every channel available on the lead" which is
	// the legacy behaviour the manual Notify button gives.
	Channels []string
}

// Notify is the legacy entry point — kept for the manual "Notificar" button
// and any existing caller that doesn't care about template customisation.
// It delegates to NotifyWithOptions with zero options.
func (s *ChargeService) Notify(ctx context.Context, tenantID, chargeID string) (*NotifyResult, error) {
	return s.NotifyWithOptions(ctx, tenantID, chargeID, NotifyOptions{})
}

// NotifyWithOptions resolves the charge's payer lead via the CRM client and
// pushes a reminder through the available channels: email + in-app when an
// account_id exists. Best-effort: each channel failure is recorded in
// SkippedNotes so the operator sees what happened without the whole call
// failing.
//
// When opts.Template is set, the message body is rendered from it with
// placeholders substituted (see renderChargeTemplate). When opts.SentBy is
// set, it's persisted on the audit entry so we can tell apart manual
// clicks from auto-régua ticks.
func (s *ChargeService) NotifyWithOptions(ctx context.Context, tenantID, chargeID string, opts NotifyOptions) (*NotifyResult, error) {
	c, err := s.repo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, charge.ErrNotFound
	}
	if !charge.IsOutstandingStatus(c.Status) {
		return nil, fmt.Errorf("cannot notify a %s charge: %w", strings.ToLower(string(c.Status)), charge.ErrInvalidState)
	}

	result := &NotifyResult{ChargeID: c.ID}

	if s.commClient == nil {
		return result, ErrCommClientNotConfigured
	}

	// Resolve the payer (lead). Without a CRM client we can still try to send
	// to a fallback contact via a notes field, but that's brittle — for now
	// we bail out with a helpful error so the operator knows why.
	if s.crmClient == nil {
		return result, ErrCrmClientNotConfigured
	}
	if c.TenantPartyID == "" {
		return result, errors.New("charge has no tenant_party_id; cannot resolve payer")
	}
	lead, err := s.crmClient.GetLead(ctx, tenantID, c.TenantPartyID)
	if err != nil || lead == nil {
		return result, fmt.Errorf("failed to load payer lead: %w", err)
	}

	subject := buildChargeNotifySubject(c)
	var body string
	if strings.TrimSpace(opts.Template) != "" {
		body = renderChargeTemplate(opts.Template, c, lead)
	} else {
		body = buildChargeNotifyBody(c, lead.Name)
	}

	allowChannel := func(name string) bool {
		if len(opts.Channels) == 0 {
			return true
		}
		for _, c := range opts.Channels {
			if c == name {
				return true
			}
		}
		return false
	}

	// Email — only attempted when the lead has one. We accept that some
	// leads are phone-only (WhatsApp-first) and skip silently with a note.
	if allowChannel("email") {
		if strings.TrimSpace(lead.Email) != "" {
			err := s.commClient.SendNotification(ctx, clients.SendNotificationRequest{
				Type:      "email",
				Recipient: lead.Email,
				Subject:   subject,
				Content:   body,
				// P1/A3: atribuição da mensagem (o toca-comm resolve o tenant
				// pela credencial; worker sem JWT precisa informá-lo).
				TenantID: tenantID,
			})
			if err != nil {
				result.SkippedNotes = append(result.SkippedNotes, fmt.Sprintf("email: %s", err.Error()))
			} else {
				result.Channels = append(result.Channels, "email")
				result.Recipient = lead.Email
			}
		} else {
			result.SkippedNotes = append(result.SkippedNotes, "email: lead has no email on file")
		}
	}

	// In-app push — only when the lead has been onboarded to the client
	// portal (account_id present).
	if allowChannel("in_app") {
		if strings.TrimSpace(lead.AccountID) != "" {
			err := s.commClient.PushInAppNotification(ctx, lead.AccountID, clients.PushNotificationRequest{
				AccountID: lead.AccountID,
				Title:     subject,
				Message:   body,
				Type:      "charge_reminder",
				Data: map[string]string{
					"charge_id":   c.ID,
					"contract_id": c.ContractID,
					// P1/A3: a rota interna do toca-comm resolve o tenant pela
					// credencial e recusa sem ele. Chamadas originadas por usuário
					// já resolvem pelo JWT; as de worker (lembrete de cobrança)
					// precisam do tenant explícito — sem isto viravam
					// SkippedNote e a notificação não chegava.
					"tenant_id": tenantID,
				},
			})
			if err != nil {
				result.SkippedNotes = append(result.SkippedNotes, fmt.Sprintf("in_app: %s", err.Error()))
			} else {
				result.Channels = append(result.Channels, "in_app")
			}
		} else {
			result.SkippedNotes = append(result.SkippedNotes, "in_app: lead has no account_id")
		}
	}

	// WhatsApp — integrado via Meta Cloud API direta (sem Twilio).
	// Issue #443. O comm service expõe `POST /notifications/send` com
	// `type=WHATSAPP` que internamente roteia para o WhatsAppService.
	// Quando o ambiente não habilita WhatsApp (env vars não setadas),
	// o comm devolve erro descritivo e nós tratamos como SkippedNote em
	// vez de quebrar o pipeline — outros canais (email/in-app) ainda saem.
	if allowChannel("whatsapp") {
		if strings.TrimSpace(lead.Phone) == "" {
			result.SkippedNotes = append(result.SkippedNotes, "whatsapp: lead has no phone on file")
		} else {
			err := s.commClient.SendNotification(ctx, clients.SendNotificationRequest{
				Type:      "whatsapp",
				Recipient: lead.Phone,
				Subject:   subject,
				Content:   body,
				// P1/A3: atribuição da mensagem (o toca-comm resolve o tenant
				// pela credencial; worker sem JWT precisa informá-lo).
				TenantID: tenantID,
			})
			if err != nil {
				result.SkippedNotes = append(result.SkippedNotes, fmt.Sprintf("whatsapp: %s", err.Error()))
			} else {
				result.Channels = append(result.Channels, "whatsapp")
				// Quando o e-mail não rolou (ou foi pulado), guardar o
				// phone como Recipient principal — o badge de "Cobrança
				// enviada para X" no UI pega esse campo. Se já tinha
				// e-mail aí, não sobrescreve.
				if result.Recipient == "" {
					result.Recipient = lead.Phone
				}
			}
		}
	}

	// SMS entrou como canal da régua (F1.1). O toca-comm só tem provedor de SMS
	// quando as credenciais do ambiente existem; sem elas o envio vira
	// SkippedNote e os outros canais seguem — mesma política do WhatsApp.
	if allowChannel("sms") {
		if strings.TrimSpace(lead.Phone) == "" {
			result.SkippedNotes = append(result.SkippedNotes, "sms: lead has no phone on file")
		} else {
			err := s.commClient.SendNotification(ctx, clients.SendNotificationRequest{
				Type:      "sms",
				Recipient: lead.Phone,
				Subject:   subject,
				Content:   body,
				TenantID:  tenantID,
			})
			if err != nil {
				result.SkippedNotes = append(result.SkippedNotes, fmt.Sprintf("sms: %s", err.Error()))
			} else {
				result.Channels = append(result.Channels, "sms")
				if result.Recipient == "" {
					result.Recipient = lead.Phone
				}
			}
		}
	}

	if len(result.Channels) == 0 {
		return result, errors.New("no channels delivered the reminder")
	}

	// Audit: append the dispatch to the charge so the UI can show "3
	// lembretes enviados, último em 12/05" and the operator gets a soft
	// warning before triggering a duplicate within 24h.
	c.RemindersSent = append(c.RemindersSent, charge.ReminderEntry{
		SentAt:      time.Now(),
		Channels:    append([]string(nil), result.Channels...),
		Recipient:   result.Recipient,
		SentBy:      opts.SentBy,
		SentByLabel: opts.SentByLabel,
	})
	c.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, c); err != nil {
		// We've already sent the reminder; surface the persistence failure
		// to the caller but don't pretend the send didn't happen.
		return result, fmt.Errorf("reminder sent but audit persist failed: %w", err)
	}
	return result, nil
}

func buildChargeNotifySubject(c *charge.Charge) string {
	switch c.Status {
	case charge.StatusOverdue:
		return fmt.Sprintf("Cobrança em atraso — vencimento %s", c.DueDate.Format("02/01/2006"))
	default:
		return fmt.Sprintf("Lembrete de cobrança — vence em %s", c.DueDate.Format("02/01/2006"))
	}
}

// buildChargeNotifyBody is the fallback body used when NotifyOptions.Template
// is empty. Kept around so the manual "Notificar" click still has a sane
// default message even before any Régua rules exist.
func buildChargeNotifyBody(c *charge.Charge, payerName string) string {
	greeting := "Olá"
	if strings.TrimSpace(payerName) != "" {
		greeting = "Olá " + payerName
	}
	amount := money.BRL(c.TotalAmount.Float())
	statusText := "vence em " + c.DueDate.Format("02/01/2006")
	if c.Status == charge.StatusOverdue {
		statusText = "venceu em " + c.DueDate.Format("02/01/2006")
	}
	body := fmt.Sprintf("%s, sua cobrança de %s %s.", greeting, amount, statusText)
	if strings.TrimSpace(c.Description) != "" {
		body += " Referência: " + c.Description + "."
	}
	return body
}

// renderChargeTemplate substitutes the placeholders supported by the Régua
// UI into the operator-defined template. We intentionally use a tiny
// {placeholder} grammar instead of text/template so the UI can be a plain
// textarea — operators don't write Go template syntax.
//
// Supported placeholders (all optional; unknown tokens are left as-is so
// a typo is surfaced verbatim instead of being silently dropped):
//   - {tenant_name}    payer's name from CRM
//   - {amount}         base amount in R$
//   - {total_amount}   amount + late_fee + interest - discount in R$
//   - {due_date}       due date formatted DD/MM/YYYY
//   - {description}    description set on the charge
//   - {days_overdue}   integer days past due (negative or 0 if not yet due)
func renderChargeTemplate(template string, c *charge.Charge, lead *clients.LeadDTO) string {
	now := time.Now()
	days := int(now.Sub(c.DueDate).Hours() / 24)
	values := map[string]string{
		"{tenant_name}":  firstNonEmptyStr(lead.Name, lead.DisplayID, ""),
		"{amount}":       money.BRL(c.Amount.Float()),
		"{total_amount}": money.BRL(c.TotalAmount.Float()),
		"{due_date}":     c.DueDate.Format("02/01/2006"),
		"{description}":  c.Description,
		"{days_overdue}": fmt.Sprintf("%d", days),
	}
	out := template
	for k, v := range values {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

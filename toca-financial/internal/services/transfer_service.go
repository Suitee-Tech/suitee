package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type TransferService struct {
	// delinquencyPolicy e openCaseDebt alimentam a retenção por inadimplência
	// (política opcional da imobiliária; sem elas a retenção não existe).
	delinquencyPolicy func(ctx context.Context, tenantID string) DelinquencyPolicy
	openCaseDebt      func(ctx context.Context, tenantID, contractID string) (float64, string)
	repo              transfer.RepositoryInterface
	eventBus          events.EventBus
	journalService    *JournalService
	db                *database.PostgresDB
}

var (
	ErrTransferSettlementTransactionRequired = errors.New("transfer settlement journal requires a database transaction")
	ErrTransferSettlementJournalMissing      = errors.New("processed transfer has no canonical settlement journal")
	ErrTransferSettlementCoverageRequired    = errors.New("owner transfer settlement requires allocation-aware payable coverage")
	ErrTransferSettlementAmountMismatch      = errors.New("owner transfer settlement amount differs from available balance")
	ErrTransferSettlementAmountInvalid       = errors.New("owner transfer settlement amount must be positive")
	// ErrTransferWithoutPayableBalanceNotOpen marca a tentativa de baixar um
	// repasse sem saldo a repassar que não está mais aberto (cancelado, por
	// exemplo): a conclusão sem saldo só existe para PENDING/APPROVED.
	ErrTransferWithoutPayableBalanceNotOpen = errors.New("transfer without payable balance is not open for settlement")
)

const transferProcessedPublishTimeout = 5 * time.Second

// transferChargeIdentityRepository is implemented by PostgreSQL and by
// focused fakes. Keeping it optional preserves compatibility with lightweight
// repositories used outside the automatic charge-payment flow.
type transferChargeIdentityRepository interface {
	GetBySourceChargeID(ctx context.Context, tenantID, sourceChargeID string) (*transfer.OwnerTransfer, error)
	GetLegacyByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error)
	ClaimLegacyForCharge(ctx context.Context, item *transfer.OwnerTransfer) (bool, error)
}

type transferTransactionIdentityRepository interface {
	GetBySourceTransactionID(ctx context.Context, tenantID, sourceTransactionID string) (*transfer.OwnerTransfer, error)
}

type transferDeductionRepository interface {
	ApplyDeductionIdempotent(
		ctx context.Context,
		tenantID, contractID, referenceMonth string,
		detail transfer.Deduction,
	) (*transfer.OwnerTransfer, bool, error)
}

type transferVersionedRepository interface {
	UpdateIfVersion(ctx context.Context, item *transfer.OwnerTransfer, expected time.Time) (bool, error)
}

type transferTransactionalEventBus interface {
	EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event events.Event) (string, error)
}

var _ transferTransactionalEventBus = (*events.RabbitMQEventBus)(nil)

// NewTransferService cria o serviço. Sem event bus, eventos de domínio
// (transfer.processed, transfer.approved) não são publicados — útil
// para testes que não precisam observar handlers.
func NewTransferService(repo transfer.RepositoryInterface) *TransferService {
	return &TransferService{repo: repo}
}

// WithEventBus injeta o event bus depois da construção. Padrão fluent
// alinhado com rental_service — main.go pode chamar
// `service.NewTransferService(repo).WithEventBus(bus)`.
func (s *TransferService) WithEventBus(bus events.EventBus) *TransferService {
	s.eventBus = bus
	return s
}

// WithDB habilita a fronteira transacional que mantém a mudança de estado do
// repasse e sua baixa no journal no mesmo commit PostgreSQL.
func (s *TransferService) WithDB(db *database.PostgresDB) *TransferService {
	s.db = db
	return s
}

// WithJournalService injeta o ledger de dupla entrada. Instâncias legadas sem
// journal mantêm o comportamento anterior; a composição produtiva deve sempre
// injetar journal e banco em conjunto.
func (s *TransferService) WithJournalService(service *JournalService) *TransferService {
	s.journalService = service
	return s
}

func (s *TransferService) runTransferTx(ctx context.Context, work func(context.Context) error) error {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		return s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return work(database.WithTx(ctx, tx))
		})
	}
	return work(ctx)
}

// TransferProcessedEvent é emitido quando o repasse é marcado como
// processed (banco confirmou crédito). Trigger para toca-comm notificar
// o owner ("seu repasse de Junho/2026 foi creditado: R$ X") e para o
// frontend portal owner atualizar a lista de Meus Repasses (#457) sem
// polling.
type TransferProcessedEvent struct {
	TransferID     string    `json:"transfer_id"`
	TenantID       string    `json:"tenant_id"`
	OwnerLeadID    string    `json:"owner_lead_id"`
	ContractID     string    `json:"contract_id"`
	PropertyID     string    `json:"property_id"`
	ReferenceMonth string    `json:"reference_month"`
	GrossRent      float64   `json:"gross_rent"`
	AdminFeeAmount float64   `json:"admin_fee_amount"`
	NetAmount      float64   `json:"net_amount"`
	ProcessedAt    time.Time `json:"processed_at"`
	ProcessedBy    string    `json:"processed_by"`
}

func (e TransferProcessedEvent) EventID() string {
	return fmt.Sprintf("transfer.processed-%s-%d", e.TransferID, e.ProcessedAt.UnixNano())
}
func (e TransferProcessedEvent) Name() string         { return "transfer.processed" }
func (e TransferProcessedEvent) Payload() interface{} { return e }

// Compile-time check de aderência ao contrato events.Event.
var _ events.Event = TransferProcessedEvent{}

type paidChargeTransferComposition struct {
	grossRentCents             int64
	adminFeePercent            float64
	adminFeeCents              int64
	ownerPayableCents          int64
	ownerReservedCents         int64
	flow                       string
	agencyRevenueCents         int64
	agencyRetainedCents        int64
	intermediationRevenueCents int64
	commissionFundedCents      int64
	commissionPayableCents     int64
	partnerPayableCents        int64
	discountCents              int64
	allocationAware            bool
}

func (c paidChargeTransferComposition) applyTo(item *transfer.OwnerTransfer) error {
	if !c.allocationAware {
		if len(item.OwnerBalanceMovements) > 0 {
			return transfer.ErrOwnerBalanceBaseChanged
		}
		item.GrossRent = float64(c.grossRentCents) / 100
		item.AdminFeePercent = c.adminFeePercent
		item.OwnerBalance = nil
		item.OwnerBalanceBaseTotalCents = 0
		item.OwnerBalanceBaseReservedCents = 0
		item.SettlementType = c.flow
		item.EconomicBreakdown = nil
		item.Calculate()
		return nil
	}

	balance, err := transfer.NewOwnerBalance(c.ownerPayableCents, c.ownerReservedCents, 0)
	if err != nil {
		return fmt.Errorf("build owner balance: %w", err)
	}
	if err := item.SetAllocatedOwnerBalance(balance, c.adminFeeCents); err != nil {
		return fmt.Errorf("apply owner balance: %w", err)
	}
	item.GrossRent = float64(c.grossRentCents) / 100
	item.AdminFeePercent = c.adminFeePercent
	item.SettlementType = c.flow
	item.EconomicBreakdown = &transfer.EconomicBreakdown{
		Flow:                       c.flow,
		GrossCents:                 c.grossRentCents,
		OwnerPayableCents:          c.ownerPayableCents,
		ReservedCents:              c.ownerReservedCents,
		AvailableCents:             c.ownerPayableCents - c.ownerReservedCents,
		AgencyRevenueCents:         c.agencyRevenueCents,
		AdminFeeCents:              c.adminFeeCents,
		IntermediationRevenueCents: c.intermediationRevenueCents,
		CommissionFundedCents:      c.commissionFundedCents,
		CommissionPayableCents:     c.commissionPayableCents,
		PartnerPayableCents:        c.partnerPayableCents,
		AgencyRetainedCents:        c.agencyRetainedCents,
		DiscountCents:              c.discountCents,
	}
	item.Calculate()
	return nil
}

func (c paidChargeTransferComposition) matches(item *transfer.OwnerTransfer) (bool, error) {
	if item == nil {
		return false, nil
	}
	headerMatches := transferAmountCents(item.GrossRent) == c.grossRentCents &&
		math.Abs(item.AdminFeePercent-c.adminFeePercent) <= 0.0000001 &&
		transferAmountCents(item.AdminFeeAmount) == c.adminFeeCents
	// Keep the whole allocation-derived transfer composition immutable after a
	// managed-bill movement. Owner balance replay protects total/reserved, while
	// this guard also catches a changed rent or administration-fee basis.
	if len(item.OwnerBalanceMovements) > 0 && (!c.allocationAware || item.OwnerBalance == nil || !headerMatches) {
		return false, transfer.ErrOwnerBalanceBaseChanged
	}
	if !headerMatches {
		return false, nil
	}
	if !c.allocationAware {
		return item.OwnerBalance == nil, nil
	}
	if item.OwnerBalance == nil {
		return false, nil
	}
	expected, err := item.ExpectedOwnerBalanceForBase(c.ownerPayableCents, c.ownerReservedCents)
	if err != nil {
		return false, err
	}
	if item.OwnerBalance.TotalCents != expected.TotalCents || item.OwnerBalance.ReservedCents != expected.ReservedCents {
		return false, nil
	}
	if item.OwnerBalance.BlockedCents != expected.BlockedCents || transferAmountCents(item.NetAmount) != expected.AvailableCents() {
		return false, nil
	}
	breakdown := item.EconomicBreakdown
	if item.SettlementType != c.flow || breakdown == nil {
		return false, nil
	}
	return breakdown.Flow == c.flow &&
		breakdown.GrossCents == c.grossRentCents &&
		breakdown.OwnerPayableCents == c.ownerPayableCents &&
		breakdown.ReservedCents == c.ownerReservedCents &&
		breakdown.AgencyRevenueCents == c.agencyRevenueCents &&
		breakdown.AdminFeeCents == c.adminFeeCents &&
		breakdown.IntermediationRevenueCents == c.intermediationRevenueCents &&
		breakdown.CommissionFundedCents == c.commissionFundedCents &&
		breakdown.CommissionPayableCents == c.commissionPayableCents &&
		breakdown.PartnerPayableCents == c.partnerPayableCents &&
		breakdown.AgencyRetainedCents == c.agencyRetainedCents &&
		breakdown.DiscountCents == c.discountCents, nil
}

func transferAmountCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func derivePaidChargeTransferComposition(c *charge.Charge, legacyAdminFeePct float64) (paidChargeTransferComposition, error) {
	paidCents := c.PaidAmount.Cents()
	legacy := paidChargeTransferComposition{
		grossRentCents:  paidCents,
		adminFeePercent: legacyAdminFeePct,
		adminFeeCents:   int64(math.Round(float64(paidCents) * legacyAdminFeePct / 100)),
		flow:            "rent",
	}

	// A legacy SALE without line items has no frozen commission waterfall. Keep
	// the old owner transfer shape, but do not invent a seller balance split.
	if c.Type == charge.TypeSale && len(c.LineItems) == 0 {
		legacy.adminFeePercent = 0
		legacy.adminFeeCents = 0
		legacy.flow = "sale"
		return legacy, nil
	}

	var allocationCount int
	var allocatedCents int64
	for _, payment := range c.Payments {
		var paymentAllocatedCents int64
		for _, allocation := range payment.Allocations {
			if allocation.AmountCents <= 0 {
				return paidChargeTransferComposition{}, fmt.Errorf("payment %s has a non-positive allocation", payment.ID)
			}
			if allocation.PaymentID != payment.ID {
				return paidChargeTransferComposition{}, fmt.Errorf("allocation %s does not belong to payment %s", allocation.ID, payment.ID)
			}
			allocationCount++
			paymentAllocatedCents += allocation.AmountCents
		}
		if len(payment.Allocations) > 0 && paymentAllocatedCents != payment.AmountCents {
			return paidChargeTransferComposition{}, fmt.Errorf("payment %s allocations total %d cents, expected %d", payment.ID, paymentAllocatedCents, payment.AmountCents)
		}
		allocatedCents += paymentAllocatedCents
	}
	if allocationCount == 0 {
		if c.Type == charge.TypeSale {
			legacy.adminFeePercent = 0
			legacy.adminFeeCents = 0
			legacy.flow = "sale"
		}
		return legacy, nil
	}
	if allocatedCents != paidCents {
		return paidChargeTransferComposition{}, fmt.Errorf("accumulated allocations total %d cents, expected paid amount %d", allocatedCents, paidCents)
	}

	lineItems := make(map[string]charge.LineItem, len(c.LineItems))
	hasExplicitTenantAdminFee := false
	baseRuleVersions := make(map[string]struct{})
	for _, item := range c.LineItems {
		if item.SourceType == "rental_base" && item.EconomicOwnerID == c.OwnerID && strings.TrimSpace(item.RuleVersionID) != "" {
			baseRuleVersions[item.RuleVersionID] = struct{}{}
		}
	}
	includedAdminItemIDs := make(map[string]struct{})
	for _, item := range c.LineItems {
		itemID := strings.TrimSpace(item.ID)
		if itemID == "" {
			continue
		}
		if _, duplicated := lineItems[itemID]; duplicated {
			return paidChargeTransferComposition{}, fmt.Errorf("charge has duplicated line item id %s", itemID)
		}
		lineItems[itemID] = item
		if strings.EqualFold(strings.TrimSpace(item.SourceID), "admin_fee") &&
			strings.TrimSpace(item.EconomicOwnerID) == strings.TrimSpace(c.TenantID) {
			hasExplicitTenantAdminFee = true
			// The recurring generator marks the administration slice already
			// included in the contractual rent as revenue. An extra admin_fee
			// bundled on top of rent has no such override. Keep that distinction
			// so explaining the gross rent cannot deduct an extra fee twice.
			if strings.HasPrefix(c.SourceEventID, "rental.") && item.SourceType == "rental_bundled_item" && item.AllocationType == charge.AllocationTypeRevenue {
				if _, sameRule := baseRuleVersions[item.RuleVersionID]; sameRule {
					includedAdminItemIDs[itemID] = struct{}{}
				}
			}
		}
	}

	var baseRentCents int64
	var baseRevenueCents int64
	var includedAdminFeeCents int64
	var intermediationRevenueCents int64
	var intermediationAgencyCents int64
	var commissionFundedCents int64
	var availableOwnerPayableCents int64
	var reservedOwnerPayableCents int64
	var agencyRevenueCents int64
	var commissionPayableCents int64
	var partnerPayableCents int64
	for _, payment := range c.Payments {
		for _, allocation := range payment.Allocations {
			item, ok := lineItems[strings.TrimSpace(allocation.ChargeItemID)]
			if !ok {
				return paidChargeTransferComposition{}, fmt.Errorf("allocation %s references unknown charge item %s", allocation.ID, allocation.ChargeItemID)
			}
			if _, includedAdmin := includedAdminItemIDs[item.ID]; includedAdmin {
				if allocation.AllocationType != charge.AllocationTypeRevenue || allocation.BeneficiaryPartyID != c.TenantID {
					return paidChargeTransferComposition{}, fmt.Errorf("included administration item %s has a divergent economic allocation", item.ID)
				}
				includedAdminFeeCents += allocation.AmountCents
			}

			isRentalBase := strings.EqualFold(strings.TrimSpace(item.SourceType), "rental_base")
			if isRentalBase {
				baseRentCents += allocation.AmountCents
				if allocation.AllocationType == charge.AllocationTypeRevenue {
					switch allocation.Purpose {
					case charge.AllocationPurposeRentalIntermediationBroker:
						intermediationRevenueCents += allocation.AmountCents
						commissionFundedCents += allocation.AmountCents
					case charge.AllocationPurposeRentalIntermediationAgency:
						intermediationRevenueCents += allocation.AmountCents
						intermediationAgencyCents += allocation.AmountCents
					case "", charge.AllocationPurposeAdminFee:
						baseRevenueCents += allocation.AmountCents
					default:
						return paidChargeTransferComposition{}, fmt.Errorf("allocation %s has unsupported rental revenue purpose %q", allocation.ID, allocation.Purpose)
					}
				}
			}
			switch allocation.AllocationType {
			case charge.AllocationTypeRevenue:
				agencyRevenueCents += allocation.AmountCents
				if c.Type == charge.TypeSale && allocation.Purpose == charge.AllocationPurposeSaleCommissionBroker {
					commissionFundedCents += allocation.AmountCents
				}
			case charge.AllocationTypeCommissionPayable:
				commissionPayableCents += allocation.AmountCents
			case charge.AllocationTypePartnerPayable, charge.AllocationTypeRoyaltyPayable, charge.AllocationTypeTaxWithheld:
				partnerPayableCents += allocation.AmountCents
			}

			if allocation.AllocationType != charge.AllocationTypeOwnerPayable ||
				strings.TrimSpace(allocation.BeneficiaryPartyID) != strings.TrimSpace(c.OwnerID) {
				continue
			}
			switch item.SettlementMode {
			case "", charge.SettlementModeCollection, charge.SettlementModeReimbursement:
				availableOwnerPayableCents += allocation.AmountCents
			case charge.SettlementModeDirectLiquidation, charge.SettlementModeOffset:
				reservedOwnerPayableCents += allocation.AmountCents
			default:
				return paidChargeTransferComposition{}, fmt.Errorf("charge item %s has unsupported settlement mode %q", item.ID, item.SettlementMode)
			}
		}
	}

	if c.Type != charge.TypeSale && baseRentCents <= 0 {
		return paidChargeTransferComposition{}, fmt.Errorf("allocation-aware RENT charge has no rental_base allocation")
	}
	if hasExplicitTenantAdminFee && baseRevenueCents > 0 {
		return paidChargeTransferComposition{}, fmt.Errorf("explicit tenant admin-fee item conflicts with a second revenue retention on rental_base")
	}
	ownerPayableCents := availableOwnerPayableCents + reservedOwnerPayableCents
	if ownerPayableCents < 0 || (ownerPayableCents == 0 && c.Type == charge.TypeSale) {
		return paidChargeTransferComposition{}, fmt.Errorf("allocation-aware RENT charge has no owner payable for owner %s", c.OwnerID)
	}

	adminFeePercent := float64(0)
	flow := "sale"
	// grossCents é atribuído nos DOIS ramos abaixo (venda ou locação); o valor
	// inicial era ineffectual e foi removido pelo lint.
	var grossCents int64
	discountCents := int64(0)
	adminFeeCents := int64(0)
	if c.Type != charge.TypeSale {
		flow = "rent"
		grossCents = baseRentCents + includedAdminFeeCents
		adminFeeCents = baseRevenueCents + includedAdminFeeCents
	} else {
		grossCents = c.Amount.Cents()
		discountCents = c.Discount.Cents()
	}
	if c.Type != charge.TypeSale && adminFeeCents > 0 {
		adminFeePercent = float64(adminFeeCents) * 100 / float64(grossCents)
	}
	agencyRetainedCents := adminFeeCents + intermediationAgencyCents
	if c.Type == charge.TypeSale {
		agencyRetainedCents = agencyRevenueCents - commissionFundedCents
	}
	return paidChargeTransferComposition{
		grossRentCents:             grossCents,
		adminFeePercent:            adminFeePercent,
		adminFeeCents:              adminFeeCents,
		ownerPayableCents:          ownerPayableCents,
		ownerReservedCents:         reservedOwnerPayableCents,
		flow:                       flow,
		agencyRevenueCents:         agencyRevenueCents,
		agencyRetainedCents:        agencyRetainedCents,
		intermediationRevenueCents: intermediationRevenueCents,
		commissionFundedCents:      commissionFundedCents,
		commissionPayableCents:     commissionPayableCents + commissionFundedCents,
		partnerPayableCents:        partnerPayableCents,
		discountCents:              discountCents,
		allocationAware:            true,
	}, nil
}

// EnsureMonthlyForChargePaid cria — ou recalcula o existente — um
// OwnerTransfer para a charge paga. Transferências automáticas são
// idempotentes por (tenant, source_charge_id); linhas legadas/manuais sem
// source_charge_id continuam compatíveis com a identidade mensal anterior.
//
// Aplicado para RENT e SALE: cada charge integralmente paga gera 1 Transfer
// mensal. RENT com allocations usa a composição econômica acumulada para
// separar aluguel, saldo disponível e obrigações reservadas; charges legadas
// sem allocations mantêm gross = PaidAmount. SALE nunca recebe a taxa de
// administração locatícia.
//
// Se um transfer já existir em status PENDING, recalcula o gross
// com base no PaidAmount da charge (corrige pro-rata de meses
// parciais provisionados com o valor cheio do contrato).
//
// Race-safe: em colisão (ErrTransferAlreadyExists), re-busca e
// devolve a row existente — o caller pode tratar como caso
// idempotente sem propagar erro. Retorna o transfer + booleano
// indicando se foi criado (true) ou já existia (false).
func (s *TransferService) EnsureMonthlyForChargePaid(ctx context.Context, c *charge.Charge, adminFeePct float64) (*transfer.OwnerTransfer, bool, error) {
	if c == nil {
		return nil, false, fmt.Errorf("charge is nil")
	}
	if c.Status != charge.StatusPaid || c.PaidAmount == nil {
		return nil, false, fmt.Errorf("charge must be fully paid before transfer")
	}
	if c.PaidAmount.Cents() != c.TotalAmount.Cents() {
		return nil, false, fmt.Errorf("paid amount does not match charge total")
	}
	if c.OwnerID == "" {
		return nil, false, fmt.Errorf("charge has empty owner_id; cannot create transfer")
	}
	if c.ReferenceMonth == "" {
		return nil, false, fmt.Errorf("charge has empty reference_month")
	}
	if strings.TrimSpace(c.ID) == "" {
		return nil, false, fmt.Errorf("charge has empty id")
	}
	composition, err := derivePaidChargeTransferComposition(c, adminFeePct)
	if err != nil {
		return nil, false, fmt.Errorf("derive paid charge transfer: %w", err)
	}
	if composition.grossRentCents <= 0 {
		return nil, false, fmt.Errorf("charge has non-positive paid amount")
	}

	identityRepo, supportsChargeIdentity := s.repo.(transferChargeIdentityRepository)
	if supportsChargeIdentity {
		existing, err := identityRepo.GetBySourceChargeID(ctx, c.TenantID, c.ID)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			expectedVersion := existing.UpdatedAt
			changed, err := reconcileTransferForPaidCharge(existing, c, composition, false)
			if err != nil {
				return nil, false, err
			}
			if changed {
				if err := updateTransferIfCurrent(ctx, s.repo, existing, expectedVersion); err != nil {
					return nil, false, err
				}
			}
			return existing, false, nil
		}

		// Backward compatibility: claim at most one pre-migration monthly row.
		// The claim is conditional, so concurrent payments for distinct SALE
		// installments cannot steal the same legacy transfer from each other.
		legacy, err := identityRepo.GetLegacyByContractMonth(ctx, c.TenantID, c.ContractID, c.ReferenceMonth)
		if err != nil {
			return nil, false, err
		}
		if legacy != nil {
			if _, err := reconcileTransferForPaidCharge(legacy, c, composition, true); err != nil {
				return nil, false, err
			}
			claimed, err := identityRepo.ClaimLegacyForCharge(ctx, legacy)
			if err != nil {
				return nil, false, err
			}
			if claimed {
				return legacy, false, nil
			}
			// A concurrent retry may have claimed it for this same charge.
			existing, err = identityRepo.GetBySourceChargeID(ctx, c.TenantID, c.ID)
			if err != nil {
				return nil, false, err
			}
			if existing != nil {
				return existing, false, nil
			}
			// A lifecycle CAS may have changed the legacy row while an
			// out-of-transaction repair was trying to claim it. Do not create a
			// second transfer until the caller retries from the fresh revision.
			stillLegacy, err := identityRepo.GetLegacyByContractMonth(ctx, c.TenantID, c.ContractID, c.ReferenceMonth)
			if err != nil {
				return nil, false, err
			}
			if stillLegacy != nil {
				return nil, false, transfer.ErrConcurrentModification
			}
		}
	} else {
		// Compatibility for simple fakes/adapters that predate charge identity.
		existing, err := s.repo.GetByContractMonth(ctx, c.TenantID, c.ContractID, c.ReferenceMonth)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			expectedVersion := existing.UpdatedAt
			changed, err := reconcileTransferForPaidCharge(existing, c, composition, false)
			if err != nil {
				return nil, false, err
			}
			if changed {
				if err := updateTransferIfCurrent(ctx, s.repo, existing, expectedVersion); err != nil {
					return nil, false, err
				}
			}
			return existing, false, nil
		}
	}

	notes := fmt.Sprintf("Repasse automático originado pela charge %s (%s)", c.ID, c.Type)
	t := transfer.New(
		c.ContractID,
		c.OwnerID,
		c.PropertyID,
		c.TenantID,
		c.ReferenceMonth,
		float64(composition.grossRentCents)/100,
		composition.adminFeePercent,
		0,
		0,
		nil,
		notes,
	)
	if err := composition.applyTo(t); err != nil {
		return nil, false, err
	}
	t.SourceChargeID = c.ID
	t.SettleIfNoPayableBalance(paidChargeSettlementTime(c))
	err = s.repo.Create(ctx, t)
	if err != nil {
		if errors.Is(err, transfer.ErrTransferAlreadyExists) {
			var again *transfer.OwnerTransfer
			var gerr error
			if supportsChargeIdentity {
				again, gerr = identityRepo.GetBySourceChargeID(ctx, c.TenantID, c.ID)
			} else {
				again, gerr = s.repo.GetByContractMonth(ctx, c.TenantID, c.ContractID, c.ReferenceMonth)
			}
			if gerr != nil {
				return nil, false, gerr
			}
			if again == nil {
				return nil, false, fmt.Errorf("transfer identity conflicted but canonical row was not found")
			}
			expectedVersion := again.UpdatedAt
			changed, reconcileErr := reconcileTransferForPaidCharge(again, c, composition, false)
			if reconcileErr != nil {
				return nil, false, reconcileErr
			}
			if changed {
				if uerr := updateTransferIfCurrent(ctx, s.repo, again, expectedVersion); uerr != nil {
					return nil, false, uerr
				}
			}
			return again, false, nil
		}
		return nil, false, err
	}
	// Política de inadimplência (opcional): se o contrato tem dívida em aberto
	// material em relação a este repasse, ele nasce retido.
	s.applyDelinquencyHold(ctx, t)
	return t, true, nil
}

// EnsureForManualSaleReceipt materializes the seller obligation released by
// an operator-posted receipt. Identity is the receipt transaction, so retries
// cannot create a second repasse even when the HTTP response is lost.
func (s *TransferService) EnsureForManualSaleReceipt(ctx context.Context, receipt *transaction.Transaction, ownerID, propertyID string) (*transfer.OwnerTransfer, bool, error) {
	if receipt == nil || receipt.EconomicBreakdown == nil || receipt.EconomicBreakdown.Flow != "sale" {
		return nil, false, errors.New("manual sale receipt breakdown is required")
	}
	if transaction.CanonicalStatus(receipt.Status) != transaction.StatusPaid {
		return nil, false, errors.New("manual sale receipt must be paid before owner transfer")
	}
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(propertyID) == "" || strings.TrimSpace(receipt.ContractID) == "" {
		return nil, false, errors.New("manual sale receipt is missing owner, property or contract")
	}
	b := receipt.EconomicBreakdown
	if b.OwnerPayableCents < 0 || b.GrossCents <= 0 || b.CommissionPoolCents != b.CommissionPayableCents+b.PartnerPayableCents+b.AgencyRetainedCents ||
		b.GrossCents-b.DiscountCents != b.OwnerPayableCents+b.CommissionPoolCents {
		return nil, false, errors.New("manual sale receipt economic breakdown does not close")
	}
	identityRepo, ok := s.repo.(transferTransactionIdentityRepository)
	if !ok {
		return nil, false, errors.New("transfer repository cannot identify manual receipt transfers")
	}
	existing, err := identityRepo.GetBySourceTransactionID(ctx, receipt.TenantID, receipt.ID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if !sameManualSaleTransfer(existing, receipt, ownerID, propertyID) {
			return nil, false, errors.New("manual receipt transfer exists with another economic composition")
		}
		return existing, false, nil
	}
	referenceDate := receipt.Date
	if receipt.PaymentDate != nil {
		referenceDate = *receipt.PaymentDate
	}
	item := transfer.New(receipt.ContractID, ownerID, propertyID, receipt.TenantID, referenceDate.Format("2006-01"), float64(b.GrossCents)/100, 0, 0, 0, nil,
		fmt.Sprintf("Repasse da venda liberado pelo recebimento manual %s", receipt.ID))
	item.ID = events.DeriveNotificationID("manual-sale-receipt-transfer", receipt.TenantID, receipt.ID)
	item.SourceTransactionID = receipt.ID
	item.SettlementType = "sale"
	balance, err := transfer.NewOwnerBalance(b.OwnerPayableCents, 0, 0)
	if err != nil {
		return nil, false, err
	}
	if err := item.SetAllocatedOwnerBalance(balance, 0); err != nil {
		return nil, false, err
	}
	item.GrossRent = float64(b.GrossCents) / 100
	item.EconomicBreakdown = &transfer.EconomicBreakdown{
		Flow:                   "sale",
		GrossCents:             b.GrossCents,
		OwnerPayableCents:      b.OwnerPayableCents,
		AvailableCents:         b.OwnerPayableCents,
		AgencyRevenueCents:     b.AgencyRetainedCents + b.CommissionPayableCents,
		CommissionPayableCents: b.CommissionPayableCents,
		PartnerPayableCents:    b.PartnerPayableCents,
		AgencyRetainedCents:    b.AgencyRetainedCents,
		DiscountCents:          b.DiscountCents,
	}
	item.Calculate()
	item.SettleIfNoPayableBalance(manualReceiptSettlementTime(receipt))
	if err := s.repo.Create(ctx, item); err != nil {
		if errors.Is(err, transfer.ErrTransferAlreadyExists) {
			existing, lookupErr := identityRepo.GetBySourceTransactionID(ctx, receipt.TenantID, receipt.ID)
			if lookupErr != nil {
				return nil, false, lookupErr
			}
			if existing != nil && sameManualSaleTransfer(existing, receipt, ownerID, propertyID) {
				return existing, false, nil
			}
		}
		return nil, false, err
	}
	return item, true, nil
}

// EnsureForManualReceipt dispatches a settled manual receipt to the correct
// owner-obligation materializer according to its economic flow.
func (s *TransferService) EnsureForManualReceipt(ctx context.Context, receipt *transaction.Transaction, ownerID, propertyID string) (*transfer.OwnerTransfer, bool, error) {
	if receipt == nil || receipt.EconomicBreakdown == nil {
		return nil, false, errors.New("manual receipt breakdown is required")
	}
	switch receipt.EconomicBreakdown.Flow {
	case "sale":
		return s.EnsureForManualSaleReceipt(ctx, receipt, ownerID, propertyID)
	case "rent":
		return s.EnsureForManualRentReceipt(ctx, receipt, ownerID, propertyID)
	default:
		return nil, false, errors.New("manual receipt has an unsupported economic flow")
	}
}

// EnsureForManualRentReceipt materializes the owner obligation released by an
// operator-posted rental receipt. It mirrors the sale path but carries the
// recurring administration fee and a "rent" settlement type.
func (s *TransferService) EnsureForManualRentReceipt(ctx context.Context, receipt *transaction.Transaction, ownerID, propertyID string) (*transfer.OwnerTransfer, bool, error) {
	if receipt == nil || receipt.EconomicBreakdown == nil || receipt.EconomicBreakdown.Flow != "rent" {
		return nil, false, errors.New("manual rent receipt breakdown is required")
	}
	if transaction.CanonicalStatus(receipt.Status) != transaction.StatusPaid {
		return nil, false, errors.New("manual rent receipt must be paid before owner transfer")
	}
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(propertyID) == "" || strings.TrimSpace(receipt.ContractID) == "" {
		return nil, false, errors.New("manual rent receipt is missing owner, property or contract")
	}
	b := receipt.EconomicBreakdown
	if b.OwnerPayableCents < 0 || b.GrossCents <= 0 || b.AdminFeeCents < 0 ||
		b.CommissionPoolCents != b.CommissionPayableCents+b.PartnerPayableCents+b.AgencyRetainedCents ||
		b.GrossCents-b.DiscountCents != b.OwnerPayableCents+b.CommissionPoolCents+b.AdminFeeCents {
		return nil, false, errors.New("manual rent receipt economic breakdown does not close")
	}
	identityRepo, ok := s.repo.(transferTransactionIdentityRepository)
	if !ok {
		return nil, false, errors.New("transfer repository cannot identify manual receipt transfers")
	}
	existing, err := identityRepo.GetBySourceTransactionID(ctx, receipt.TenantID, receipt.ID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if !sameManualRentTransfer(existing, receipt, ownerID, propertyID) {
			return nil, false, errors.New("manual receipt transfer exists with another economic composition")
		}
		return existing, false, nil
	}
	referenceDate := receipt.Date
	if receipt.PaymentDate != nil {
		referenceDate = *receipt.PaymentDate
	}
	item := transfer.New(receipt.ContractID, ownerID, propertyID, receipt.TenantID, referenceDate.Format("2006-01"), float64(b.GrossCents)/100, 0, 0, 0, nil,
		fmt.Sprintf("Repasse da locação liberado pelo recebimento manual %s", receipt.ID))
	item.ID = events.DeriveNotificationID("manual-rent-receipt-transfer", receipt.TenantID, receipt.ID)
	item.SourceTransactionID = receipt.ID
	item.SettlementType = "rent"
	balance, err := transfer.NewOwnerBalance(b.OwnerPayableCents, 0, 0)
	if err != nil {
		return nil, false, err
	}
	if err := item.SetAllocatedOwnerBalance(balance, b.AdminFeeCents); err != nil {
		return nil, false, err
	}
	item.GrossRent = float64(b.GrossCents) / 100
	item.EconomicBreakdown = &transfer.EconomicBreakdown{
		Flow:                       "rent",
		GrossCents:                 b.GrossCents,
		OwnerPayableCents:          b.OwnerPayableCents,
		AvailableCents:             b.OwnerPayableCents,
		AgencyRevenueCents:         b.AgencyRetainedCents + b.CommissionPayableCents + b.AdminFeeCents,
		IntermediationRevenueCents: b.CommissionPoolCents,
		CommissionFundedCents:      b.CommissionFundedCents,
		AdminFeeCents:              b.AdminFeeCents,
		CommissionPayableCents:     b.CommissionPayableCents,
		PartnerPayableCents:        b.PartnerPayableCents,
		AgencyRetainedCents:        b.AgencyRetainedCents,
		DiscountCents:              b.DiscountCents,
	}
	item.Calculate()
	item.SettleIfNoPayableBalance(manualReceiptSettlementTime(receipt))
	if err := s.repo.Create(ctx, item); err != nil {
		if errors.Is(err, transfer.ErrTransferAlreadyExists) {
			existing, lookupErr := identityRepo.GetBySourceTransactionID(ctx, receipt.TenantID, receipt.ID)
			if lookupErr != nil {
				return nil, false, lookupErr
			}
			if existing != nil && sameManualRentTransfer(existing, receipt, ownerID, propertyID) {
				return existing, false, nil
			}
		}
		return nil, false, err
	}
	return item, true, nil
}

func sameManualRentTransfer(item *transfer.OwnerTransfer, receipt *transaction.Transaction, ownerID, propertyID string) bool {
	if item == nil || receipt == nil || receipt.EconomicBreakdown == nil || item.EconomicBreakdown == nil || item.OwnerBalance == nil {
		return false
	}
	b := receipt.EconomicBreakdown
	// Older manual rent transfers stored the entire intermediation as retained.
	// Accept that exact historical representation on retry without rewriting a
	// processed transfer or creating a second owner obligation.
	stored := item.EconomicBreakdown
	sameSplit := stored.CommissionPayableCents == b.CommissionPayableCents && stored.AgencyRetainedCents == b.AgencyRetainedCents && stored.CommissionFundedCents == b.CommissionFundedCents
	legacySplit := stored.CommissionPayableCents == 0 && stored.CommissionFundedCents == 0 && stored.IntermediationRevenueCents == 0 && stored.AgencyRetainedCents == b.CommissionPoolCents && b.PartnerPayableCents == 0
	return item.SourceTransactionID == receipt.ID && item.SourceChargeID == "" && item.TenantID == receipt.TenantID &&
		item.ContractID == receipt.ContractID && item.OwnerID == ownerID && item.PropertyID == propertyID &&
		item.SettlementType == "rent" && item.OwnerBalance.TotalCents == b.OwnerPayableCents &&
		item.EconomicBreakdown.GrossCents == b.GrossCents && item.EconomicBreakdown.OwnerPayableCents == b.OwnerPayableCents &&
		item.EconomicBreakdown.AdminFeeCents == b.AdminFeeCents &&
		(sameSplit || legacySplit) &&
		item.EconomicBreakdown.PartnerPayableCents == b.PartnerPayableCents &&
		item.EconomicBreakdown.DiscountCents == b.DiscountCents
}

func sameManualSaleTransfer(item *transfer.OwnerTransfer, receipt *transaction.Transaction, ownerID, propertyID string) bool {
	if item == nil || receipt == nil || receipt.EconomicBreakdown == nil || item.EconomicBreakdown == nil || item.OwnerBalance == nil {
		return false
	}
	b := receipt.EconomicBreakdown
	return item.SourceTransactionID == receipt.ID && item.SourceChargeID == "" && item.TenantID == receipt.TenantID &&
		item.ContractID == receipt.ContractID && item.OwnerID == ownerID && item.PropertyID == propertyID &&
		item.SettlementType == "sale" && item.OwnerBalance.TotalCents == b.OwnerPayableCents &&
		item.EconomicBreakdown.GrossCents == b.GrossCents && item.EconomicBreakdown.OwnerPayableCents == b.OwnerPayableCents &&
		item.EconomicBreakdown.CommissionPayableCents == b.CommissionPayableCents &&
		item.EconomicBreakdown.PartnerPayableCents == b.PartnerPayableCents &&
		item.EconomicBreakdown.AgencyRetainedCents == b.AgencyRetainedCents && item.EconomicBreakdown.DiscountCents == b.DiscountCents
}

func reconcileTransferForPaidCharge(existing *transfer.OwnerTransfer, c *charge.Charge, composition paidChargeTransferComposition, claimLegacy bool) (bool, error) {
	if existing == nil || c == nil {
		return false, fmt.Errorf("transfer and charge are required")
	}
	if existing.TenantID != c.TenantID || existing.ContractID != c.ContractID || existing.ReferenceMonth != c.ReferenceMonth {
		return false, fmt.Errorf("transfer %s does not belong to paid charge %s", existing.ID, c.ID)
	}
	if existing.OwnerID != c.OwnerID || existing.PropertyID != c.PropertyID {
		return false, fmt.Errorf("transfer %s owner/property does not match paid charge %s", existing.ID, c.ID)
	}
	if existing.SourceChargeID != "" && existing.SourceChargeID != c.ID {
		return false, fmt.Errorf("transfer %s belongs to another source charge", existing.ID)
	}
	changed := false
	if claimLegacy && existing.SourceChargeID == "" {
		existing.SourceChargeID = c.ID
		changed = true
	}
	matches, err := composition.matches(existing)
	if err != nil {
		return false, fmt.Errorf("rebuild allocation-aware owner balance: %w", err)
	}
	if !matches {
		if existing.Status != transfer.StatusPending {
			// Um repasse encerrado sem saldo a repassar é uma conclusão sem
			// caixa: a composição econômica real da cobrança paga pode adotá-lo
			// em vez de derrubar o recebimento. Qualquer outro estado terminal
			// (pago de fato ou cancelado) continua sendo conflito explícito.
			if !existing.ReopenWithoutPayableBalance(paidChargeSettlementTime(c), "Cobrança paga assumiu o repasse encerrado sem saldo.") {
				return false, fmt.Errorf(
					"existing %s transfer %s has gross %.2f/fee %.2f, expected %.2f/%.2f",
					existing.Status,
					existing.ID,
					existing.GrossRent,
					existing.AdminFeeAmount,
					float64(composition.grossRentCents)/100,
					float64(composition.adminFeeCents)/100,
				)
			}
			changed = true
		}
		if err := composition.applyTo(existing); err != nil {
			return false, err
		}
		changed = true
	}
	if existing.SettleIfNoPayableBalance(paidChargeSettlementTime(c)) {
		changed = true
	}
	return changed, nil
}

func paidChargeSettlementTime(c *charge.Charge) time.Time {
	if c != nil && c.PaidAt != nil {
		return *c.PaidAt
	}
	return time.Now()
}

func manualReceiptSettlementTime(receipt *transaction.Transaction) time.Time {
	if receipt != nil && receipt.PaymentDate != nil {
		return *receipt.PaymentDate
	}
	return time.Now()
}

func updateTransferIfCurrent(ctx context.Context, repo transfer.RepositoryInterface, item *transfer.OwnerTransfer, expected time.Time) error {
	if versioned, ok := repo.(transferVersionedRepository); ok {
		updated, err := versioned.UpdateIfVersion(ctx, item, expected)
		if err != nil {
			return err
		}
		if !updated {
			return transfer.ErrConcurrentModification
		}
		return nil
	}
	return repo.Update(ctx, item)
}

func (s *TransferService) Create(ctx context.Context, contractID, ownerID, propertyID, tenantID, referenceMonth string, grossRent, adminFeePercent, maintenanceDeductions, otherDeductions float64, deductionDetails []transfer.Deduction, notes string) (*transfer.OwnerTransfer, error) {
	t := transfer.New(contractID, ownerID, propertyID, tenantID, referenceMonth, grossRent, adminFeePercent, maintenanceDeductions, otherDeductions, deductionDetails, notes)
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransferService) CreateIdempotent(ctx context.Context, contractID, ownerID, propertyID, tenantID, referenceMonth string, grossRent, adminFeePercent, maintenanceDeductions, otherDeductions float64, deductionDetails []transfer.Deduction, notes string) (*transfer.OwnerTransfer, error) {
	existing, err := s.repo.GetByContractMonth(ctx, tenantID, contractID, referenceMonth)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	t, err := s.Create(ctx, contractID, ownerID, propertyID, tenantID, referenceMonth, grossRent, adminFeePercent, maintenanceDeductions, otherDeductions, deductionDetails, notes)
	if err != nil {
		if errors.Is(err, transfer.ErrTransferAlreadyExists) {
			again, gerr := s.repo.GetByContractMonth(ctx, tenantID, contractID, referenceMonth)
			if gerr != nil {
				return nil, gerr
			}
			return again, nil
		}
		return nil, err
	}
	// Mesma política do repasse nascido de cobrança paga: vale para qualquer
	// caminho de criação (manual, recibo avulso, venda).
	s.applyDelinquencyHold(ctx, t)
	return t, nil
}

func (s *TransferService) ApplyDeduction(
	ctx context.Context,
	tenantID, contractID, referenceMonth, deductionType, description, referenceID string,
	amount float64,
) error {
	return s.ApplyDeductionDetailed(ctx, tenantID, contractID, referenceMonth, transfer.Deduction{
		Description: description,
		Amount:      amount,
		Type:        deductionType,
		ReferenceID: referenceID,
	})
}

// ApplyDeductionDetailed é a variante que recebe a dedução completa — inclusive
// a contrapartida no razão, quando quem aplica conhece a natureza do valor.
func (s *TransferService) ApplyDeductionDetailed(
	ctx context.Context,
	tenantID, contractID, referenceMonth string,
	detail transfer.Deduction,
) error {
	amount := detail.Amount
	if amount <= 0 {
		return nil
	}
	amountCents := int64(math.Round(amount * 100))
	if amountCents <= 0 {
		return fmt.Errorf("deduction amount must be at least one cent")
	}
	amount = float64(amountCents) / 100
	detail.Amount = amount
	detail.ReferenceID = strings.TrimSpace(detail.ReferenceID)
	if detail.ReferenceID == "" {
		return fmt.Errorf("deduction reference_id is required")
	}
	referenceID := detail.ReferenceID
	if atomicRepo, ok := s.repo.(transferDeductionRepository); ok {
		canonical, applied, err := atomicRepo.ApplyDeductionIdempotent(ctx, tenantID, contractID, referenceMonth, detail)
		if err != nil {
			return err
		}
		if canonical == nil {
			return fmt.Errorf("pending owner transfer not found for contract %s month %s", contractID, referenceMonth)
		}
		if !applied {
			// Dedução já registrada: a baixa contábil foi postada na primeira
			// aplicação (o source_event é derivado da referência).
			return nil
		}
		return s.postDeductionLedgerEntry(ctx, tenantID, contractID, canonical, detail)
	}

	// Compatibility for lightweight fakes/adapters. PostgreSQL implements the
	// atomic path above, which resolves reference_id globally and serializes
	// deterministic selection when multiple SALE installments share a month.
	t, err := s.repo.GetByContractMonth(ctx, tenantID, contractID, referenceMonth)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("owner transfer not found for contract %s month %s", contractID, referenceMonth)
	}
	for _, detail := range t.DeductionDetails {
		if detail.ReferenceID == referenceID {
			return nil
		}
	}
	if t.Status != transfer.StatusPending {
		return fmt.Errorf("owner transfer %s is not pending", t.ID)
	}
	expectedVersion := t.UpdatedAt
	applyTransferDeduction(t, detail)
	if err := updateTransferIfCurrent(ctx, s.repo, t, expectedVersion); err != nil {
		return err
	}
	return s.postDeductionLedgerEntry(ctx, tenantID, contractID, t, detail)
}

// postDeductionLedgerEntry baixa owner_payable no valor retido do proprietário
// contra a conta que representa a natureza do valor. Sem isso o passivo fica
// aberto no razão enquanto o proprietário já recebeu menos.
//
// A conta do crédito depende da origem: manutenção adiantada pela imobiliária
// é recuperação de receita (mesmo tratamento da fatia cobrada do inquilino);
// conta de consumo paga em nome do proprietário é estorno de despesa.
//
// Idempotente pelo source_event derivado da referência da dedução, então a
// reentrega do evento não duplica o lançamento.
func (s *TransferService) postDeductionLedgerEntry(
	ctx context.Context,
	tenantID, contractID string,
	item *transfer.OwnerTransfer,
	detail transfer.Deduction,
) error {
	if s.journalService == nil || item == nil || detail.Amount <= 0 {
		return nil
	}
	cents := int64(math.Round(detail.Amount * 100))
	if cents <= 0 {
		return nil
	}
	entry, err := journal.New(
		tenantID,
		contractID,
		"Dedução no repasse: "+detail.Description,
		time.Now().UTC(),
		[]journal.Leg{
			{
				Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: cents,
				TransferID: item.ID, BeneficiaryPartyID: item.OwnerID,
			},
			{
				Account: deductionLedgerAccount(detail), Direction: journal.Credit, AmountCents: cents,
				TransferID: item.ID, BeneficiaryPartyID: item.OwnerID,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("build deduction ledger entry for %s: %w", detail.ReferenceID, err)
	}
	entry.OperationID = item.ID
	sourceEventID := "deduction." + strings.TrimSpace(detail.ReferenceID)
	if err := s.journalService.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
		return fmt.Errorf("post deduction ledger entry for %s: %w", detail.ReferenceID, err)
	}
	return nil
}

// deductionLedgerAccount escolhe a contrapartida da retenção. Quem aplica a
// dedução pode declarar a conta (a natureza depende de quem adiantou o
// pagamento); sem declaração, tipo desconhecido cai em despesa — leitura
// conservadora, que não infla receita sem confirmação da origem.
func deductionLedgerAccount(detail transfer.Deduction) journal.Account {
	if account := journal.Account(strings.TrimSpace(detail.LedgerAccount)); account != "" && journal.ValidAccounts[account] {
		return account
	}
	switch strings.ToUpper(strings.TrimSpace(detail.Type)) {
	case "MAINTENANCE":
		// A imobiliária adiantou o serviço: a retenção reembolsa um direito a
		// receber, não gera receita própria.
		return journal.AccountAccountsReceivable
	default:
		return journal.AccountExpense
	}
}

func applyTransferDeduction(item *transfer.OwnerTransfer, detail transfer.Deduction) {
	if detail.Type == "MAINTENANCE" {
		item.MaintenanceDeductions += detail.Amount
	} else {
		item.OtherDeductions += detail.Amount
	}
	item.DeductionDetails = append(item.DeductionDetails, detail)
	item.Calculate()
	// Uma dedução pode consumir todo o valor a repassar: o repasse zera e não
	// tem mais contrapartida em caixa, então encerra na mesma transição em vez
	// de ficar pendente para sempre.
	item.SyncNoPayableBalanceState(time.Now())
}

func (s *TransferService) GetByID(ctx context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("transfer not found")
	}
	return t, nil
}

// Approve transitions the transfer state and persists the actor + history
// entry alongside the status change. actorLabel is the human-readable
// snapshot of the operator name at the moment of the click — captured by
// the handler from the JWT user_name claim so the audit doesn't depend on
// broker records still existing later.
func (s *TransferService) Approve(ctx context.Context, tenantID, id, actor, actorLabel string) error {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("transfer not found")
	}
	// A repeated approval after a committed response is an idempotent success.
	// This is especially important for operators behind unstable connections:
	// the retry must not surface a conflict after the obligation was already
	// approved, and must not append a second audit/history entry.
	if t.Status == transfer.StatusApproved || t.Status == transfer.StatusProcessed {
		return nil
	}
	// Retenção por inadimplência: o repasse não anda enquanto o contrato tiver
	// caso de cobrança aberto que justifique a retenção (ou até alguém com alçada
	// liberar). A checagem fica ANTES de qualquer escrita.
	if t.IsHeldForDelinquency() {
		return fmt.Errorf("%w: %s", transfer.ErrTransferHeldByDelinquency, t.DelinquencyHoldReason)
	}
	expectedVersion := t.UpdatedAt
	if err := t.Approve(actor, actorLabel); err != nil {
		return err
	}
	return updateTransferIfCurrent(ctx, s.repo, t, expectedVersion)
}

// Cancel cancela um repasse ainda não processado (PENDING/APPROVED). Um
// repasse PROCESSED já movimentou caixa e não é cancelável por este caminho —
// exige estorno explícito da baixa do proprietário. Idempotente.
func (s *TransferService) Cancel(ctx context.Context, tenantID, id, actor, actorLabel, reason string) error {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("transfer not found")
	}
	if t.Status == transfer.StatusCancelled {
		return nil
	}
	expectedVersion := t.UpdatedAt
	if err := t.Cancel(actor, actorLabel, reason); err != nil {
		return err
	}
	return updateTransferIfCurrent(ctx, s.repo, t, expectedVersion)
}

// Process marca o repasse como processed (banco confirmou crédito ao
// owner). Publica transfer.processed para que toca-comm notifique o
// owner via portal/email e o frontend atualize Meus Repasses (#461).
// Process marks the transfer as PROCESSED (backward-compatible, sem evidência).
func (s *TransferService) Process(ctx context.Context, tenantID, id, actor, actorLabel string) error {
	return s.ProcessWithEvidence(ctx, tenantID, id, actor, actorLabel, "", "")
}

// ProcessWithEvidence marks a transfer as PROCESSED. Quando o repasse depende
// de fluxo externo (banco), o operador pode efetivá-lo manualmente anexando um
// comprovante (proofURL) e/ou observação (note), registrados no repasse e no audit.
func (s *TransferService) ProcessWithEvidence(ctx context.Context, tenantID, id, actor, actorLabel, proofURL, note string) error {
	if s.journalService != nil && s.db == nil && database.TxFromCtx(ctx) == nil {
		return ErrTransferSettlementTransactionRequired
	}

	return s.runTransferTx(ctx, func(txCtx context.Context) error {
		t, err := s.repo.GetByID(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("transfer not found")
		}
		// Retenção por inadimplência bloqueia o pagamento: enquanto o caso de
		// cobrança do contrato estiver aberto (e a retenção ativa), o dinheiro do
		// proprietário fica retido. Liberar é explícito — automático quando o caso
		// é resolvido, ou por quem tem alçada de repasse.
		if t.IsHeldForDelinquency() {
			return fmt.Errorf("%w: %s", transfer.ErrTransferHeldByDelinquency, t.DelinquencyHoldReason)
		}
		// A repetição após commit valida o lançamento canônico e retorna sucesso
		// sem novo histórico, audit ou evento. Registros PROCESSED anteriores ao
		// recorte não são completados silenciosamente: exigem backfill explícito.
		if t.Status == transfer.StatusProcessed && s.journalService != nil {
			return s.validateProcessedTransferSettlement(txCtx, t)
		}
		// Repasse sem saldo a repassar: a baixa é o encerramento de uma obrigação
		// que não tem contrapartida em caixa. Não exige aprovação prévia (nada é
		// pago), não exige comprovante e não gera lançamento de liquidação —
		// inventar um pagamento de R$ 0,00 era exatamente o que travava a baixa.
		if t.HasNoPayableBalance() {
			// Reentrega depois do commit: a validação canônica reconhece o
			// encerramento sem perna de caixa e responde sucesso idempotente.
			if t.Status == transfer.StatusProcessed {
				return s.validateProcessedTransferSettlement(txCtx, t)
			}
			return s.settleTransferWithoutPayableBalance(txCtx, t, actor, actorLabel, note)
		}
		// P1/A13 — segregação de funções: quem aprovou o repasse não efetiva o
		// pagamento. Mesmo padrão de `managed_bill_service` (pagador ≠ aprovador)
		// e mesma dispensa para quem responde pela operação financeira
		// (`WithSeparationOfDutiesWaiver`, aplicada na borda).
		if !SeparationOfDutiesWaived(txCtx) && actor != "" && strings.TrimSpace(t.ApprovedBy) != "" && actor == strings.TrimSpace(t.ApprovedBy) {
			return ErrSegregationOfDutiesApproverCannotPay
		}
		beforeStatus := t.Status
		expectedVersion := t.UpdatedAt
		if err := t.ProcessWithProof(actor, actorLabel, proofURL, note); err != nil {
			return err
		}

		var settlementEntry *journal.Entry
		if s.journalService != nil {
			settlementEntry, err = newOwnerTransferSettlementJournalEntry(t)
			if err != nil {
				return err
			}
			if settlementEntry != nil {
				t.SettlementJournalSourceID = ownerTransferSettlementSourceEventID(t.TenantID, t.ID)
			}
		}

		// O CAS ocorre antes da baixa, mas ambos ainda pertencem à mesma
		// transação. Um concorrente perde o CAS antes de tentar publicar uma
		// entrada com outro processed_at; qualquer falha posterior no journal
		// reverte também o status PROCESSED.
		if err := updateTransferIfCurrent(txCtx, s.repo, t, expectedVersion); err != nil {
			if errors.Is(err, transfer.ErrConcurrentModification) && s.journalService != nil {
				fresh, getErr := s.repo.GetByID(txCtx, tenantID, id)
				if getErr != nil {
					return getErr
				}
				if fresh != nil && fresh.Status == transfer.StatusProcessed {
					return s.validateProcessedTransferSettlement(txCtx, fresh)
				}
			}
			return err
		}
		if settlementEntry != nil {
			if _, err := s.journalService.CreateCanonicalWithSourceEventID(txCtx, t.SettlementJournalSourceID, settlementEntry); err != nil {
				return fmt.Errorf("post owner transfer settlement journal: %w", err)
			}
		}

		processedAt := time.Now()
		if t.ProcessedAt != nil {
			processedAt = *t.ProcessedAt
		}
		// O audit é adiado até o commit; no barramento produtivo, o evento entra
		// no outbox da própria transação. Nenhum observador anuncia um repasse
		// revertido.
		audit.Record(txCtx, audit.Entry{
			Action:     "transfer.processed",
			EntityType: "Transfer",
			EntityID:   t.ID,
			Before:     map[string]any{"status": string(beforeStatus)},
			After: map[string]any{
				"status":                       string(t.Status),
				"processed_at":                 processedAt,
				"processed_by":                 actor,
				"net_amount":                   t.NetAmount,
				"settlement_journal_source_id": t.SettlementJournalSourceID,
				"proof_url":                    t.ProofURL,
				"notes":                        t.Notes,
			},
			Metadata: map[string]any{
				"actor_label":     actorLabel,
				"contract_id":     t.ContractID,
				"owner_lead_id":   t.OwnerID,
				"reference_month": t.ReferenceMonth,
			},
		})

		if s.eventBus != nil {
			evt := TransferProcessedEvent{
				TransferID:     t.ID,
				TenantID:       t.TenantID,
				OwnerLeadID:    t.OwnerID,
				ContractID:     t.ContractID,
				PropertyID:     t.PropertyID,
				ReferenceMonth: t.ReferenceMonth,
				GrossRent:      t.GrossRent,
				AdminFeeAmount: t.AdminFeeAmount,
				NetAmount:      t.NetAmount,
				ProcessedAt:    processedAt,
				ProcessedBy:    actor,
			}
			if tx := database.TxFromCtx(txCtx); tx != nil {
				if outbox, ok := s.eventBus.(transferTransactionalEventBus); ok {
					if _, err := outbox.EnqueueOutboxInTx(txCtx, tx, evt); err != nil {
						return fmt.Errorf("enqueue transfer.processed: %w", err)
					}
					return nil
				}
			}
			publish := func() {
				publishCtx, cancel := context.WithTimeout(context.WithoutCancel(txCtx), transferProcessedPublishTimeout)
				defer cancel()
				if pubErr := s.eventBus.PublishReliable(publishCtx, evt); pubErr != nil {
					log.Printf("WARN: failed to publish transfer.processed for transfer=%s: %v", t.ID, pubErr)
				}
			}
			if !database.DeferUntilCommit(txCtx, publish) {
				publish()
			}
		}

		return nil
	})
}

func ownerTransferSettlementSourceEventID(tenantID, transferID string) string {
	return events.DeriveNotificationID("owner_transfer.settlement", tenantID, transferID)
}

// settleTransferWithoutPayableBalance encerra um repasse que não tem nada a
// repassar, a pedido do operador. A conclusão não cria lançamento de caixa nem
// publica transfer.processed: nenhum valor sai (ou entra) na conta do
// proprietário, então não há pagamento a comunicar no portal. O que fica
// registrado é o estado PROCESSED com o marcador settled_without_balance, o
// operador no histórico e o audit da transição — suficiente para a conciliação
// e para reabrir o registro caso o saldo volte (ReopenWithoutPayableBalance).
func (s *TransferService) settleTransferWithoutPayableBalance(ctx context.Context, t *transfer.OwnerTransfer, actor, actorLabel, note string) error {
	if t == nil || !t.HasNoPayableBalance() {
		return ErrTransferSettlementAmountInvalid
	}
	beforeStatus := t.Status
	expectedVersion := t.UpdatedAt
	at := time.Now()
	if !t.SettleWithoutPayableBalance(at, actor, actorLabel) {
		return fmt.Errorf("%w (current: %s)", ErrTransferWithoutPayableBalanceNotOpen, t.Status)
	}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		t.Notes = trimmed
	}
	if err := updateTransferIfCurrent(ctx, s.repo, t, expectedVersion); err != nil {
		if errors.Is(err, transfer.ErrConcurrentModification) && s.journalService != nil {
			fresh, getErr := s.repo.GetByID(ctx, t.TenantID, t.ID)
			if getErr != nil {
				return getErr
			}
			if fresh != nil && fresh.Status == transfer.StatusProcessed {
				return s.validateProcessedTransferSettlement(ctx, fresh)
			}
		}
		return err
	}
	processedAt := at
	if t.ProcessedAt != nil {
		processedAt = *t.ProcessedAt
	}
	audit.Record(ctx, audit.Entry{
		Action:     "transfer.processed",
		EntityType: "Transfer",
		EntityID:   t.ID,
		Before:     map[string]any{"status": string(beforeStatus)},
		After: map[string]any{
			"status":                  string(t.Status),
			"processed_at":            processedAt,
			"processed_by":            t.ProcessedBy,
			"settled_without_balance": true,
			"net_amount":              t.NetAmount,
			"notes":                   t.Notes,
		},
		Metadata: map[string]any{
			"actor_label":     actorLabel,
			"contract_id":     t.ContractID,
			"owner_lead_id":   t.OwnerID,
			"reference_month": t.ReferenceMonth,
			"reason":          "no_payable_balance",
		},
	})
	return nil
}

// SettleDanglingWithoutPayableBalance conclui um repasse que JÁ está sem nada a
// repassar mas segue pendente porque a transição que zerou o saldo aconteceu
// antes desta regra existir (reserva de conta administrada consumida, dedução
// aplicada ou composição adotada). É idempotente e usado pelo reparo
// operacional; repasses com valor a repassar não são tocados.
func (s *TransferService) SettleDanglingWithoutPayableBalance(ctx context.Context, tenantID, id, actor, actorLabel string) (bool, error) {
	settled := false
	err := s.runTransferTx(ctx, func(txCtx context.Context) error {
		t, err := s.repo.GetByID(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("transfer not found")
		}
		if !t.HasNoPayableBalance() {
			return nil
		}
		if t.Status == transfer.StatusProcessed && t.SettledWithoutBalance {
			return nil
		}
		if err := s.settleTransferWithoutPayableBalance(txCtx, t, actor, actorLabel, ""); err != nil {
			return err
		}
		settled = true
		return nil
	})
	return settled, err
}

func (s *TransferService) validateProcessedTransferSettlement(ctx context.Context, item *transfer.OwnerTransfer) error {
	// Repasse encerrado sem saldo a repassar não tem perna de caixa: exigir um
	// lançamento de liquidação aqui recriaria o pagamento falso de R$ 0,00 que a
	// conclusão sem saldo existe justamente para evitar. A decisão vem do
	// marcador persistido, não do saldo atual: um estorno posterior de conta
	// administrada pode devolver saldo sem que este encerramento tenha
	// movimentado caixa.
	if item.SettledWithoutBalance {
		if strings.TrimSpace(item.SettlementJournalSourceID) != "" {
			return journal.ErrSourceEventConflict
		}
		return nil
	}
	if item.HasNoPayableBalance() && item.ProcessedBy == "system" {
		if strings.TrimSpace(item.SettlementJournalSourceID) != "" {
			return journal.ErrSourceEventConflict
		}
		return nil
	}
	sourceEventID := ownerTransferSettlementSourceEventID(item.TenantID, item.ID)
	if strings.TrimSpace(item.SettlementJournalSourceID) == "" {
		return ErrTransferSettlementJournalMissing
	}
	if item.SettlementJournalSourceID != sourceEventID {
		return journal.ErrSourceEventConflict
	}
	expected, err := newOwnerTransferSettlementJournalEntry(item)
	if err != nil {
		return err
	}
	if expected == nil {
		return nil
	}
	existing, err := s.journalService.GetBySourceEventID(ctx, item.TenantID, sourceEventID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTransferSettlementJournalMissing
	}
	_, err = s.journalService.CreateCanonicalWithSourceEventID(ctx, sourceEventID, expected)
	return err
}

func newOwnerTransferSettlementJournalEntry(item *transfer.OwnerTransfer) (*journal.Entry, error) {
	if item == nil {
		return nil, errors.New("owner transfer is required")
	}
	// O débito em owner_payable só é seguro quando a composição alocada prova
	// quanto daquele passivo continua disponível. Repasses manuais ou legados
	// sem essa cobertura exigem backfill explícito e auditável.
	if item.OwnerBalance == nil {
		return nil, ErrTransferSettlementCoverageRequired
	}
	amountCents := int64(math.Round(item.NetAmount * 100))
	if amountCents <= 0 {
		return nil, ErrTransferSettlementAmountInvalid
	}
	availableCents := item.OwnerBalance.AvailableCents()
	if amountCents != availableCents {
		return nil, fmt.Errorf("%w: net=%d available=%d", ErrTransferSettlementAmountMismatch, amountCents, availableCents)
	}
	processedAt := time.Now().UTC()
	if item.ProcessedAt != nil {
		processedAt = item.ProcessedAt.UTC()
	}
	entry, err := journal.New(item.TenantID, item.ContractID, "Repasse ao proprietário "+item.ID, processedAt, []journal.Leg{
		{
			Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: amountCents,
			TransferID: item.ID, BeneficiaryPartyID: item.OwnerID,
		},
		{
			Account: journal.AccountCash, Direction: journal.Credit, AmountCents: amountCents,
			TransferID: item.ID, BeneficiaryPartyID: item.OwnerID,
		},
	})
	if err != nil {
		return nil, err
	}
	entry.OperationID = item.ID
	return entry, nil
}

func (s *TransferService) List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error) {
	return s.repo.List(ctx, tenantID, contractID, ownerID, status, referenceMonth, limit, offset)
}

// ListByOwnerLeadIDs é a entry point do portal cliente para Meus Repasses
// (#456). Caller (PortalTransferHandler.ListMine) resolve account_id →
// leadIDs via CrossTenantRepository.LeadIDsForAccount e passa aqui.
func (s *TransferService) ListByOwnerLeadIDs(ctx context.Context, tenantID string, ownerLeadIDs []string, filter transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return s.repo.ListByOwnerLeadIDs(ctx, tenantID, ownerLeadIDs, filter)
}

func (s *TransferService) Search(ctx context.Context, tenantID string, filter transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return s.repo.Search(ctx, tenantID, filter)
}

// ── Retenção de repasse por inadimplência ──────────────────────────────────

// DelinquencyPolicy é a política da imobiliária para reter repasse quando o
// contrato tem dívida em aberto na inadimplência.
type DelinquencyPolicy struct {
	Enabled bool
	// MinSharePct é a materialidade mínima da dívida em relação ao repasse para
	// a retenção acontecer, em %. 100 (padrão) = só retém quando a dívida em
	// aberto cobre o repasse inteiro.
	MinSharePct int
}

// WithDelinquencyPolicy injeta a leitura da política por tenant. Sem ela a
// retenção não existe (comportamento legado: repasse segue o fluxo normal).
func (s *TransferService) WithDelinquencyPolicy(fn func(ctx context.Context, tenantID string) DelinquencyPolicy) *TransferService {
	s.delinquencyPolicy = fn
	return s
}

// WithOpenCaseDebt injeta a consulta de dívida aberta do contrato na
// inadimplência: devolve o valor em reais e o id do caso que a sustenta.
func (s *TransferService) WithOpenCaseDebt(fn func(ctx context.Context, tenantID, contractID string) (float64, string)) *TransferService {
	s.openCaseDebt = fn
	return s
}

// applyDelinquencyHold retém o repasse quando a política do tenant está ligada e a
// dívida em aberto do contrato é material em relação ao valor do repasse.
//
// Regra de materialidade (a resposta para "reter R$ 3.000 por R$ 200"):
// a retenção só acontece quando `dívida >= MinSharePct% do repasse`. Com o padrão
// de 100%, reter exige que a dívida cubra o repasse inteiro — dívida pequena não
// trava o dinheiro do proprietário. A imobiliária que quiser ser mais conservadora
// baixa a materialidade na configuração, ciente de que aí um repasse grande pode
// ficar retido por uma dívida menor.
//
// Best-effort: falha ao ler a política ou a dívida NÃO retém (o repasse segue o
// fluxo normal). Retenção é proteção, não punição — e um erro de leitura não pode
// travar dinheiro de terceiro sem decisão.
func (s *TransferService) applyDelinquencyHold(ctx context.Context, t *transfer.OwnerTransfer) {
	if t == nil || s.delinquencyPolicy == nil || s.openCaseDebt == nil {
		return
	}
	if t.Status == transfer.StatusProcessed || t.Status == transfer.StatusCancelled {
		return
	}
	policy, err := func() (p DelinquencyPolicy, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("delinquency policy panic: %v", r)
			}
		}()
		return s.delinquencyPolicy(ctx, t.TenantID), nil
	}()
	if err != nil || !policy.Enabled {
		return
	}

	debt, caseID := s.openCaseDebt(ctx, t.TenantID, t.ContractID)
	if debt <= 0 {
		return
	}
	share := policy.MinSharePct
	if share <= 0 {
		share = 100
	}
	payable := t.NetAmount
	if t.OwnerBalance != nil && t.OwnerBalance.TotalCents > 0 {
		payable = float64(t.OwnerBalance.TotalCents) / 100
	}
	if payable <= 0 || debt < payable*float64(share)/100 {
		return
	}

	reason := fmt.Sprintf("retenção por inadimplência: dívida de R$ %.2f em aberto no contrato (caso %s)", debt, shortID(caseID))
	if err := t.HoldForDelinquency(int64(math.Round(payable*100)), caseID, reason, time.Now()); err != nil {
		return
	}
	if err := s.repo.Update(ctx, t); err != nil {
		log.Printf("[TransferService] failed to persist delinquency hold on transfer %s: %v", t.ID, err)
		return
	}
	audit.Record(ctx, audit.Entry{
		Action:     "transfer.held_delinquency",
		EntityType: "OwnerTransfer",
		EntityID:   t.ID,
		After: map[string]any{
			"held_cents": t.DelinquencyHeldCents,
			"debt":       debt,
			"case_id":    caseID,
			"reason":     reason,
		},
		Metadata: map[string]any{"path": "applyDelinquencyHold", "tenant_id": t.TenantID, "contract_id": t.ContractID},
	})
}

// ReleaseDelinquencyHold libera a retenção de um repasse (caso resolvido ou
// decisão de quem tem alçada de repasse).
func (s *TransferService) ReleaseDelinquencyHold(ctx context.Context, tenantID, id, reason, actor string) (*transfer.OwnerTransfer, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("transfer not found")
	}
	if !t.ReleaseDelinquencyHold(time.Now()) {
		return t, nil // sem retenção ativa: idempotente
	}
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "transfer.delinquency_hold_released",
		EntityType: "OwnerTransfer",
		EntityID:   t.ID,
		After:      map[string]any{"released_cents": t.NetAmount, "reason": reason},
		Metadata:   map[string]any{"path": "ReleaseDelinquencyHold", "tenant_id": tenantID, "actor": actor},
	})
	return t, nil
}

// ReleaseHoldsForContract libera as retenções dos repasses de um contrato.
//
// É o caminho automático: quando o caso de cobrança do contrato deixa de estar
// aberto, a razão da retenção desapareceu. Liberar aqui NÃO paga nada — o
// pagamento continua exigindo aprovação (`finance:approve_transfer`), então não há
// dinheiro andando sozinho: o que se desfaz é o bloqueio, para o repasse não ficar
// preso por esquecimento.
func (s *TransferService) ReleaseHoldsForContract(ctx context.Context, tenantID, contractID, reason string) (int, error) {
	if s.repo == nil || contractID == "" {
		return 0, nil
	}
	lista, _, err := s.repo.List(ctx, tenantID, contractID, "", "", "", 200, 0)
	if err != nil {
		return 0, err
	}
	liberados := 0
	for _, t := range lista {
		if t == nil || !t.IsHeldForDelinquency() {
			continue
		}
		if !t.ReleaseDelinquencyHold(time.Now()) {
			continue
		}
		if err := s.repo.Update(ctx, t); err != nil {
			log.Printf("[TransferService] failed to release delinquency hold on transfer %s: %v", t.ID, err)
			continue
		}
		liberados++
		audit.Record(ctx, audit.Entry{
			Action:     "transfer.delinquency_hold_released",
			EntityType: "OwnerTransfer",
			EntityID:   t.ID,
			After:      map[string]any{"reason": reason, "trigger": "case_closed"},
			Metadata:   map[string]any{"path": "ReleaseHoldsForContract", "tenant_id": tenantID, "contract_id": contractID},
		})
	}
	return liberados, nil
}

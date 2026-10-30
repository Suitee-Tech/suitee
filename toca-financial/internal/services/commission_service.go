package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
)

type CommissionRepository interface {
	Create(ctx context.Context, c *commission.Commission) error
	GetByID(ctx context.Context, tenantID, id string) (*commission.Commission, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*commission.Commission, error)
	List(ctx context.Context, tenantID string, limit, offset int64) ([]*commission.Commission, int64, error)
	UpdateStatus(ctx context.Context, tenantID, id string, status commission.Status) error
	UpdateStatusByDealID(ctx context.Context, tenantID, dealID string, status commission.Status) error
	Update(ctx context.Context, c *commission.Commission) error
	ListByDealID(ctx context.Context, tenantID, dealID string) ([]*commission.Commission, error)
	ListByBrokerID(ctx context.Context, tenantID, brokerID string, limit, offset int64) ([]*commission.Commission, int64, error)
	Delete(ctx context.Context, id, tenantID string) error
}

type CommissionService struct {
	repo                     CommissionRepository
	db                       *database.PostgresDB
	manualTransactionService *TransactionService
	contractLookup           commissionContractLookup
	// taxObligations materializa o tributo retido como obrigação a recolher.
	taxObligations *TaxObligationService
	// brokerAdvances abate o adiantamento da imobiliária ao corretor quando o
	// desconto é do tipo ADVANCE (ver broker_advance_service.go).
	brokerAdvances *BrokerAdvanceService
}

type commissionContractLookup interface {
	GetContract(ctx context.Context, tenantID, contractID string) (*clients.ContractDTO, error)
}

// commissionPayoutNetUpdater keeps compatibility with lightweight/legacy
// repositories used outside production. The Postgres repository implements
// commissionDeductionAtomicRepository below so its read/mutate/write cycle is
// serialized as well as transactionally synchronized with the payout.
type commissionPayoutNetUpdater interface {
	UpdateWithLinkedPayoutNet(ctx context.Context, c *commission.Commission) error
}

// commissionDeductionAtomicRepository serializes the full read/mutate/write
// cycle under a database row lock. It prevents concurrent distinct deductions
// (or add/remove pairs) from overwriting one another and keeps a linked payout
// net amount in the same transaction.
type commissionDeductionAtomicRepository interface {
	AddDeductionAtomic(ctx context.Context, tenantID, commissionID string, d commission.Deduction) (updated *commission.Commission, previousNet float64, changed bool, err error)
	RemoveDeductionAtomic(ctx context.Context, tenantID, commissionID, deductionID string) (updated *commission.Commission, previousNet float64, changed bool, err error)
}

type commissionTransitionRepository interface {
	UpdateIfStatus(ctx context.Context, c *commission.Commission, expected commission.Status, expectedUpdatedAt time.Time) error
}

type commissionLedgerRepository interface {
	GetByLedgerTransactionID(ctx context.Context, tenantID, ledgerTransactionID string) (*commission.Commission, error)
}

func NewCommissionService(repo CommissionRepository) *CommissionService {
	return &CommissionService{repo: repo}
}

// WithDB makes a Commission transition and its linked ledger transition one
// commit boundary. Nested callers reuse the transaction already in context.
func (s *CommissionService) WithDB(db *database.PostgresDB) *CommissionService {
	s.db = db
	return s
}

// WithManualTransactionService enables the inverse half of the manual
// Commission/Transaction lifecycle. Both services deliberately use private
// one-way transition helpers to avoid recursive synchronization.
// WithTaxObligationService liga a materialização do tributo retido: cada
// desconto de IRRF/INSS/ISS passa a alimentar a obrigação a recolher da
// competência (ver tax_obligation_service.go). Opcional — sem ele o desconto
// continua reduzindo o líquido e a ordem de pagamento, mas o passivo fiscal não
// ganha vida própria.
func (s *CommissionService) WithTaxObligationService(obligations *TaxObligationService) *CommissionService {
	s.taxObligations = obligations
	return s
}

// syncTaxObligation alinha a obrigação fiscal desta comissão ao estado atual dos
// descontos. Uma falha aqui NÃO derruba o lançamento do desconto (que já está
// gravado e reduz o líquido): a obrigação é derivada e recomputável, então
// registramos o erro de forma observável e seguimos — o operador tem
// `POST /tax-obligations/recompute` para reconstruir.
func (s *CommissionService) syncTaxObligation(ctx context.Context, tenantID, commissionID string) {
	if s.taxObligations == nil {
		return
	}
	if err := s.taxObligations.SyncCommission(ctx, tenantID, commissionID); err != nil {
		logger.FromContext(ctx).Error("tax_obligation.sync_failed",
			"event", "tax_obligation.sync_failed",
			"tenant_id", tenantID,
			"commission_id", commissionID,
			"error", err.Error(),
			"hint", "recomputar com POST /tax-obligations/recompute",
		)
	}
}

// WithBrokerAdvanceService liga o abatimento de adiantamentos: um desconto do
// tipo ADVANCE consome o saldo do corretor (FIFO, do mais antigo ao mais novo) e
// removê-lo devolve o saldo. Opcional.
func (s *CommissionService) WithBrokerAdvanceService(advances *BrokerAdvanceService) *CommissionService {
	s.brokerAdvances = advances
	return s
}

// settleBrokerAdvance abate o adiantamento do desconto recém-lançado. Falha aqui
// SOBE: um desconto de adiantamento sem saldo correspondente reduziria o líquido
// do corretor sem dar baixa em nada — melhor recusar o lançamento.
func (s *CommissionService) settleBrokerAdvance(ctx context.Context, c *commission.Commission, d commission.Deduction) error {
	if s.brokerAdvances == nil || commission.DeductionKind(d.Kind) != commission.DeductionAdvance {
		return nil
	}
	if err := s.brokerAdvances.SettleFromDeduction(ctx, c, d); err != nil {
		return err
	}
	return nil
}

func (s *CommissionService) WithManualTransactionService(transactions *TransactionService) *CommissionService {
	s.manualTransactionService = transactions
	return s
}

// WithContractLookup makes legacy rental commissions fail closed. Current
// rental rows carry FundingRequiredCents; older rows need the contract type to
// distinguish them from sale/manual commissions before lifecycle transitions.
func (s *CommissionService) WithContractLookup(lookup commissionContractLookup) *CommissionService {
	s.contractLookup = lookup
	return s
}

func (s *CommissionService) requireRentalFundingConfiguration(ctx context.Context, c *commission.Commission) error {
	if c == nil || c.FundingRequiredCents > 0 || strings.TrimSpace(c.ContractID) == "" || s.contractLookup == nil {
		return nil
	}
	contract, err := s.contractLookup.GetContract(ctx, c.TenantID, c.ContractID)
	if err != nil {
		return fmt.Errorf("verify commission contract type before settlement: %w", err)
	}
	if contract == nil {
		return errors.New("verify commission contract type before settlement: contract not found")
	}
	if isRentalDealType(contract.Type) {
		return fmt.Errorf("%w: legacy rental commission must be reconciled with collected rent before approval or payment", commission.ErrFundingGateConflict)
	}
	return nil
}

func (s *CommissionService) runLifecycleTx(ctx context.Context, work func(context.Context) (*commission.Commission, error)) (*commission.Commission, error) {
	if s.db == nil || database.TxFromCtx(ctx) != nil {
		return work(ctx)
	}
	var result *commission.Commission
	err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		var err error
		result, err = work(database.WithTx(ctx, tx))
		return err
	})
	return result, err
}

func (s *CommissionService) commissionByLedgerTransactionID(ctx context.Context, tenantID, ledgerTransactionID string) (*commission.Commission, error) {
	ledgerTransactionID = strings.TrimSpace(ledgerTransactionID)
	if ledgerTransactionID == "" {
		return nil, nil
	}
	repo, ok := s.repo.(commissionLedgerRepository)
	if !ok {
		return nil, errors.New("commission repository cannot resolve ledger_transaction_id")
	}
	return repo.GetByLedgerTransactionID(ctx, tenantID, ledgerTransactionID)
}

// Create cria uma commission "crua" — usado pela rota manual
// POST /commissions. O fluxo automático (contract.signed) usa
// CreateWithRole abaixo, que carrega role/percentage/contractID.
func (s *CommissionService) Create(ctx context.Context, dealID, brokerID string, amount float64, tenantID, requestedBy string) (*commission.Commission, error) {
	c := commission.New(dealID, brokerID, amount, tenantID)
	// P1/A13 (fechamento): o autor entra no ESTADO, não só na trilha.
	c.RequestedBy = strings.TrimSpace(requestedBy)
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Commission",
		EntityID:   c.ID,
		After: map[string]any{
			"deal_id":   dealID,
			"broker_id": brokerID,
			"amount":    amount,
		},
	})
	return c, nil
}

func (s *CommissionService) CreateFromRequest(ctx context.Context, sourceEventID, dealID, brokerID string, amount float64, tenantID, requestedBy string) (*commission.Commission, error) {
	c := commission.New(dealID, brokerID, amount, tenantID)
	c.RequestedBy = strings.TrimSpace(requestedBy)
	c.SourceEventID = strings.TrimSpace(sourceEventID)
	if c.SourceEventID == "" {
		return s.Create(ctx, dealID, brokerID, amount, tenantID, requestedBy)
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, commission.ErrCommissionAlreadyExists) {
			existing, gerr := s.repo.GetBySourceEventID(ctx, tenantID, c.SourceEventID)
			if gerr != nil {
				return nil, gerr
			}
			if existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Commission",
		EntityID:   c.ID,
		After: map[string]any{
			"deal_id":         dealID,
			"broker_id":       brokerID,
			"amount":          amount,
			"source_event_id": c.SourceEventID,
		},
	})
	return c, nil
}

// CreateWithRole é o construtor preferido pelo event_handler — já carrega
// role, percentage e contract_id, e calcula NetAmount via RecomputeNet
// (sem deductions iniciais NetAmount == GrossAmount == amount).
func (s *CommissionService) CreateWithRole(ctx context.Context, dealID, brokerID, contractID, role string, percentage, amount float64, tenantID, requestedBy string) (*commission.Commission, error) {
	c := commission.NewWithRole(dealID, brokerID, contractID, role, percentage, amount, tenantID)
	c.RequestedBy = strings.TrimSpace(requestedBy)
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Commission",
		EntityID:   c.ID,
		After: map[string]any{
			"deal_id":     dealID,
			"broker_id":   brokerID,
			"contract_id": contractID,
			"role":        role,
			"percentage":  percentage,
			"amount":      amount,
		},
	})
	return c, nil
}

func (s *CommissionService) CreateWithRoleFromEvent(ctx context.Context, sourceEventID, dealID, brokerID, contractID, role string, percentage, amount float64, tenantID, ownerID, customerID, propertyID string, planHash, requestedBy string) (*commission.Commission, error) {
	return s.createWithRoleFromEventAndLedger(ctx, sourceEventID, dealID, brokerID, contractID, role, percentage, amount, tenantID, ownerID, customerID, propertyID, planHash, "", 0, requestedBy)
}

// CreateRentalWithRoleFromEvent creates the contractual commission while
// gating settlement on future collected-rent funding.
func (s *CommissionService) CreateRentalWithRoleFromEvent(ctx context.Context, sourceEventID, dealID, brokerID, contractID, role string, percentage, amount float64, tenantID, ownerID, customerID, propertyID string, planHash string, fundingRequiredCents int64) (*commission.Commission, error) {
	if fundingRequiredCents <= 0 {
		return nil, errors.New("rental commission funding requirement must be positive")
	}
	return s.createWithRoleFromEventAndLedger(ctx, sourceEventID, dealID, brokerID, contractID, role, percentage, amount, tenantID, ownerID, customerID, propertyID, planHash, "", fundingRequiredCents, "")
}

func (s *CommissionService) createWithRoleFromEventAndLedger(ctx context.Context, sourceEventID, dealID, brokerID, contractID, role string, percentage, amount float64, tenantID, ownerID, customerID, propertyID, planHash, ledgerTransactionID string, fundingRequiredCents int64, requestedBy string) (*commission.Commission, error) {
	c := commission.NewWithRole(dealID, brokerID, contractID, role, percentage, amount, tenantID)
	c.RequestedBy = strings.TrimSpace(requestedBy)
	c.OwnerID = ownerID
	c.CustomerID = customerID
	c.PropertyID = propertyID
	c.SourceEventID = strings.TrimSpace(sourceEventID)
	c.PlanHash = strings.TrimSpace(planHash)
	c.LedgerTransactionID = strings.TrimSpace(ledgerTransactionID)
	if fundingRequiredCents > 0 {
		if _, err := c.EnableRentalFundingGate(fundingRequiredCents); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, commission.ErrCommissionAlreadyExists) && c.SourceEventID != "" {
			existing, gerr := s.repo.GetBySourceEventID(ctx, tenantID, c.SourceEventID)
			if gerr != nil {
				return nil, gerr
			}
			if existing != nil {
				if fundingRequiredCents > 0 {
					return s.ensureRentalFundingGate(ctx, existing, fundingRequiredCents)
				}
				return existing, nil
			}
		}
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Commission",
		EntityID:   c.ID,
		After: map[string]any{
			"deal_id":                dealID,
			"broker_id":              brokerID,
			"contract_id":            contractID,
			"role":                   role,
			"percentage":             percentage,
			"amount":                 amount,
			"owner_id":               ownerID,
			"customer_id":            customerID,
			"property_id":            propertyID,
			"source_event_id":        c.SourceEventID,
			"ledger_transaction_id":  c.LedgerTransactionID,
			"funding_required_cents": c.FundingRequiredCents,
		},
	})
	return c, nil
}

func (s *CommissionService) ensureRentalFundingGate(ctx context.Context, existing *commission.Commission, requiredCents int64) (*commission.Commission, error) {
	expectedStatus := existing.Status
	expectedUpdatedAt := existing.UpdatedAt
	changed, err := existing.EnableRentalFundingGate(requiredCents)
	if err != nil {
		return nil, err
	}
	if !changed {
		return existing, nil
	}
	if err := s.persistLifecycleTransition(ctx, existing, expectedStatus, expectedUpdatedAt); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "commission.funding_gate_enabled",
		EntityType: "Commission",
		EntityID:   existing.ID,
		After: map[string]any{
			"funding_required_cents": existing.FundingRequiredCents,
			"status":                 existing.Status,
		},
	})
	return existing, nil
}

func (s *CommissionService) GetByID(ctx context.Context, tenantID, id string) (*commission.Commission, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *CommissionService) List(ctx context.Context, tenantID string, limit, offset int64) ([]*commission.Commission, int64, error) {
	return s.repo.List(ctx, tenantID, limit, offset)
}

func (s *CommissionService) ListByDeal(ctx context.Context, tenantID, dealID string) ([]*commission.Commission, error) {
	return s.repo.ListByDealID(ctx, tenantID, dealID)
}

func (s *CommissionService) ListByBroker(ctx context.Context, tenantID, brokerID string, limit, offset int64) ([]*commission.Commission, int64, error) {
	return s.repo.ListByBrokerID(ctx, tenantID, brokerID, limit, offset)
}

// RecordRentalFunding appends one collected-rent slice to a funding-gated
// commission. The status is intentionally unchanged; the same revision CAS
// used by lifecycle transitions prevents a concurrent deduction/status update
// from being overwritten.
func (s *CommissionService) RecordRentalFunding(ctx context.Context, tenantID, id string, entry commission.FundingEntry) (*commission.Commission, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("commission not found")
	}
	expectedStatus := c.Status
	expectedUpdatedAt := c.UpdatedAt
	changed, err := c.RecordFunding(entry)
	if err != nil {
		return nil, err
	}
	if !changed {
		return c, nil
	}
	if err := s.persistLifecycleTransition(ctx, c, expectedStatus, expectedUpdatedAt); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "commission.funded",
		EntityType: "Commission",
		EntityID:   c.ID,
		After: map[string]any{
			"funded_amount_cents":    c.FundedAmountCents,
			"funding_required_cents": c.FundingRequiredCents,
			"payment_id":             entry.PaymentID,
			"charge_id":              entry.ChargeID,
		},
	})
	return c, nil
}

func (s *CommissionService) lifecycleCommission(ctx context.Context, tenantID, id, payoutOrderID string) (*commission.Commission, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("commission not found")
	}
	if payoutOrderID == "" {
		if c.PayoutOrderID != "" {
			return nil, commission.ErrLinkedPayoutLifecycleManaged
		}
		return c, nil
	}
	if c.PayoutOrderID != payoutOrderID {
		return nil, commission.ErrLinkedPayoutMismatch
	}
	return c, nil
}

func (s *CommissionService) persistLifecycleTransition(ctx context.Context, c *commission.Commission, expected commission.Status, expectedUpdatedAt time.Time) error {
	if repo, ok := s.repo.(commissionTransitionRepository); ok {
		return repo.UpdateIfStatus(ctx, c, expected, expectedUpdatedAt)
	}
	// Compatibility for lightweight test/adapter repositories. The production
	// Postgres repository always implements the physical-revision CAS above.
	return s.repo.Update(ctx, c)
}

func (s *CommissionService) approve(ctx context.Context, tenantID, id, payoutOrderID, actor string) (*commission.Commission, error) {
	c, err := s.lifecycleCommission(ctx, tenantID, id, payoutOrderID)
	if err != nil {
		return nil, err
	}
	// APPROVED and PAID both prove that this obligation already crossed the
	// approval boundary. Treat a delayed/retried command as success without
	// rewriting the aggregate or duplicating audit history.
	if c.Status == commission.StatusApproved || c.Status == commission.StatusPaid {
		return c, nil
	}
	// SECURITY (achado A13 da revisão de permissões de 2026-09-19): no caminho
	// STANDALONE quem pediu a comissão não pode aprová-la. O estado carrega o
	// autor (RequestedBy) justamente para esta comparação; a checagem fica
	// depois do curto-circuito idempotente, porque uma repetição não exerce
	// autoridade nova. Comissão ligada a payout order segue a segregação do
	// payout, que é o aggregate root dela.
	if payoutOrderID == "" {
		if err := requireDistinctRequesterAndApprover(ctx, c.RequestedBy, actor); err != nil {
			return nil, err
		}
	}
	if err := s.requireRentalFundingConfiguration(ctx, c); err != nil {
		return nil, err
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.Approve(); err != nil {
		return nil, err
	}
	if c.Status == prev {
		return c, nil
	}
	// A aprovação passa a deixar o próprio aprovador no agregado: sem isso a
	// checagem aprovador ≠ pagador não teria com o que comparar depois. Vazio
	// (worker, comando legado) não sobrescreve dado existente.
	if payoutOrderID == "" {
		if trimmed := strings.TrimSpace(actor); trimmed != "" {
			c.ApprovedBy = trimmed
		}
	}
	if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "approve",
		EntityType: "Commission",
		EntityID:   c.ID,
		Before:     map[string]any{"status": prev},
		After:      map[string]any{"status": c.Status},
	})
	return c, nil
}

// Approve is only available for standalone commissions. Once linked, the
// payout order is the aggregate root for both lifecycles.
//
// `actor` identifica quem aprova (account_id do usuário autenticado) e é o que
// torna criador ≠ aprovador verificável — ver aprovação A13.
func (s *CommissionService) Approve(ctx context.Context, tenantID, id, actor string) (*commission.Commission, error) {
	return s.approve(ctx, tenantID, id, "", actor)
}

func (s *CommissionService) ApproveFromPayout(ctx context.Context, tenantID, id, payoutOrderID string) (*commission.Commission, error) {
	return s.approve(ctx, tenantID, id, payoutOrderID, "")
}

// MarkPaidWithProof aceita um proof_url (vindo do upload em /proofs) +
// nota opcional. Quando o caller é o webhook (sem proof), passa "" — o
// guard payout_proof_required fica na borda HTTP.
func (s *CommissionService) markPaidWithProof(ctx context.Context, tenantID, id, payoutOrderID, proofURL, note, actor string) (*commission.Commission, error) {
	c, err := s.lifecycleCommission(ctx, tenantID, id, payoutOrderID)
	if err != nil {
		return nil, err
	}
	if err := s.requireLinkedLedgerLifecycle(ctx, c); err != nil {
		return nil, err
	}
	if c.Status != commission.StatusPaid {
		// SECURITY (achado A13 da revisão de permissões de 2026-09-19): em
		// comissão STANDALONE quem aprovou não pode efetivar o pagamento. A
		// marca de dispensa vem da borda HTTP a partir do papel autenticado,
		// nunca do corpo da requisição.
		if err := requireDistinctApproverAndPayer(ctx, c.ApprovedBy, actor); err != nil {
			return nil, err
		}
		if err := s.requireRentalFundingConfiguration(ctx, c); err != nil {
			return nil, err
		}
	}
	prev := c.Status
	previousProofURL := c.ProofURL
	previousNote := c.Note
	expectedUpdatedAt := c.UpdatedAt
	if err := c.MarkPaid(proofURL, note); err != nil {
		return nil, err
	}
	statusChanged := c.Status != prev
	evidenceChanged := c.ProofURL != previousProofURL || c.Note != previousNote
	if statusChanged || evidenceChanged {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
		action := "mark_paid"
		if !statusChanged {
			action = "attach_payment_evidence"
		}
		audit.Record(ctx, audit.Entry{
			Action:     action,
			EntityType: "Commission",
			EntityID:   c.ID,
			Before: map[string]any{
				"status":    prev,
				"proof_url": previousProofURL != "",
				"note":      previousNote != "",
			},
			After: map[string]any{
				"status":    c.Status,
				"paid_at":   c.PaidAt,
				"proof_url": c.ProofURL != "",
				"note":      c.Note != "",
			},
		})
	}
	if c.LedgerTransactionID != "" {
		if _, err := s.manualTransactionService.settleFromCommission(ctx, c, c.PaidAt); err != nil {
			return nil, fmt.Errorf("settle linked transaction %s: %w", c.LedgerTransactionID, err)
		}
	}
	return c, nil
}

// MarkPaidWithProof é a confirmação manual de pagamento da comissão — o
// operador informa que pagou fora do sistema e anexa a evidência.
//
// A exigência de comprovante NÃO é aplicada aqui: a política do tenant é
// resolvida na borda HTTP, que é onde a confirmação manual do operador entra.
// MarkPaidFromPayout é a liquidação automática via ordem de pagamento e
// legitimamente não carrega comprovante próprio.
//
// `actor` é o pagador (account_id autenticado) e sustenta aprovador ≠ pagador
// (achado A13).
func (s *CommissionService) MarkPaidWithProof(ctx context.Context, tenantID, id, proofURL, note, actor string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.markPaidWithProof(txCtx, tenantID, id, "", proofURL, note, actor)
	})
}

func (s *CommissionService) MarkPaidFromPayout(ctx context.Context, tenantID, id, payoutOrderID, proofURL, note string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.markPaidWithProof(txCtx, tenantID, id, payoutOrderID, proofURL, note, "")
	})
}

// SECURITY (achado A13 da revisão de permissões de 2026-09-19): o antigo
// `Pay(ctx, tenantID, id)` era um atalho de compat sem ator e sem comprovante —
// por construção um caminho de pagamento que escapava da segregação de funções.
// Ele não tinha nenhum chamador (nem rota, nem teste) e foi removido; o endpoint
// `/commissions/{id}/pay` usa MarkPaidWithProof, que carrega o pagador.

func (s *CommissionService) cancel(ctx context.Context, tenantID, id, payoutOrderID, reason string) (*commission.Commission, error) {
	c, err := s.lifecycleCommission(ctx, tenantID, id, payoutOrderID)
	if err != nil {
		return nil, err
	}
	if err := s.requireLinkedLedgerLifecycle(ctx, c); err != nil {
		return nil, err
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.Cancel(reason); err != nil {
		return nil, err
	}
	if c.Status != prev {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "cancel",
			EntityType: "Commission",
			EntityID:   c.ID,
			Before:     map[string]any{"status": prev},
			After:      map[string]any{"status": c.Status, "reason": reason},
		})
	}
	if c.LedgerTransactionID != "" {
		if _, err := s.manualTransactionService.cancelFromCommission(ctx, c); err != nil {
			return nil, fmt.Errorf("cancel linked transaction %s: %w", c.LedgerTransactionID, err)
		}
	}
	return c, nil
}

func (s *CommissionService) Cancel(ctx context.Context, tenantID, id, reason string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.cancel(txCtx, tenantID, id, "", reason)
	})
}

// ReopenFromPayout é o inverso do CancelFromPayout: devolve a comissão
// cancelada ao estado PENDING e reabre o lançamento contábil vinculado, para a
// obrigação poder ser lançada de novo numa ordem regerada.
//
// Dois vínculos, de propósito: `linkedPayoutOrderID` é a ordem CANCELADA à qual
// a comissão está presa hoje (é o que a validação de ciclo de vida confere) e
// `newPayoutOrderID` é a ordem regerada que passa a ser a dona da obrigação.
// Passar só a ordem nova seria recusado — a comissão ainda aponta para a
// cancelada.
//
// Os dois lados andam juntos na mesma transação (comissão + lançamento): uma
// comissão viva com lançamento cancelado é exatamente a inconsistência que o
// cancelamento criou — reabrir só um lado deixaria o razão mentindo.
func (s *CommissionService) ReopenFromPayout(ctx context.Context, tenantID, id, linkedPayoutOrderID, newPayoutOrderID, actor, reason string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.reopen(txCtx, tenantID, id, linkedPayoutOrderID, newPayoutOrderID, actor, reason)
	})
}

func (s *CommissionService) reopen(ctx context.Context, tenantID, id, linkedPayoutOrderID, newPayoutOrderID, actor, reason string) (*commission.Commission, error) {
	c, err := s.lifecycleCommission(ctx, tenantID, id, linkedPayoutOrderID)
	if err != nil {
		return nil, err
	}
	if err := s.requireLinkedLedgerLifecycle(ctx, c); err != nil {
		return nil, err
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.Reopen(actor, reason); err != nil {
		return nil, err
	}
	if newPayoutOrderID != "" {
		c.PayoutOrderID = newPayoutOrderID
	}
	if c.Status != prev {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "reopen",
			EntityType: "Commission",
			EntityID:   c.ID,
			Before:     map[string]any{"status": prev, "payout_order_id": linkedPayoutOrderID},
			After:      map[string]any{"status": c.Status, "reason": reason, "payout_order_id": c.PayoutOrderID},
		})
	}
	if c.LedgerTransactionID != "" {
		if _, err := s.manualTransactionService.reopenFromCommission(ctx, c); err != nil {
			return nil, fmt.Errorf("reopen linked transaction %s: %w", c.LedgerTransactionID, err)
		}
	}
	return c, nil
}

func (s *CommissionService) CancelFromPayout(ctx context.Context, tenantID, id, payoutOrderID, reason string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.cancel(txCtx, tenantID, id, payoutOrderID, reason)
	})
}

// Reverse transita PAID/APPROVED → REVERSED para estorno contábil.
func (s *CommissionService) Reverse(ctx context.Context, tenantID, id, reason string) (*commission.Commission, error) {
	return s.runLifecycleTx(ctx, func(txCtx context.Context) (*commission.Commission, error) {
		return s.reverse(txCtx, tenantID, id, reason)
	})
}

func (s *CommissionService) reverse(ctx context.Context, tenantID, id, reason string) (*commission.Commission, error) {
	c, err := s.lifecycleCommission(ctx, tenantID, id, "")
	if err != nil {
		return nil, err
	}
	if err := s.requireLinkedLedgerLifecycle(ctx, c); err != nil {
		return nil, err
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.Reverse(reason); err != nil {
		return nil, err
	}
	if c.Status != prev {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "reverse",
			EntityType: "Commission",
			EntityID:   c.ID,
			Before:     map[string]any{"status": prev},
			After:      map[string]any{"status": c.Status, "reason": reason},
		})
	}
	if c.LedgerTransactionID != "" {
		if _, err := s.manualTransactionService.reverseFromCommission(ctx, c, reason); err != nil {
			return nil, fmt.Errorf("reverse linked transaction %s: %w", c.LedgerTransactionID, err)
		}
	}
	return c, nil
}

func (s *CommissionService) requireLinkedLedgerLifecycle(ctx context.Context, c *commission.Commission) error {
	if c == nil || strings.TrimSpace(c.LedgerTransactionID) == "" {
		return nil
	}
	if s.manualTransactionService == nil {
		return errors.New("linked manual commission transaction lifecycle is not configured")
	}
	if s.db == nil && database.TxFromCtx(ctx) == nil {
		return errors.New("linked manual commission lifecycle requires a database transaction")
	}
	return nil
}

// markPaidFromLedger mirrors a Transaction-driven settlement without calling
// back into TransactionService. A payout-linked Commission remains owned by
// the payout aggregate, so the transaction endpoint fails closed in that case.
func (s *CommissionService) markPaidFromLedger(ctx context.Context, c *commission.Commission, paidAt *time.Time) (*commission.Commission, error) {
	if c == nil {
		return nil, errors.New("linked commission not found")
	}
	if c.PayoutOrderID != "" {
		return nil, commission.ErrLinkedPayoutLifecycleManaged
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.MarkPaid("", ""); err != nil {
		return nil, err
	}
	if paidAt != nil && !paidAt.IsZero() {
		paid := *paidAt
		c.PaidAt = &paid
	}
	if c.Status != prev {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (s *CommissionService) reverseFromLedger(ctx context.Context, c *commission.Commission, reason string) (*commission.Commission, error) {
	if c == nil {
		return nil, errors.New("linked commission not found")
	}
	if c.PayoutOrderID != "" {
		return nil, commission.ErrLinkedPayoutLifecycleManaged
	}
	prev := c.Status
	expectedUpdatedAt := c.UpdatedAt
	if err := c.Reverse(reason); err != nil {
		return nil, err
	}
	if c.Status != prev {
		if err := s.persistLifecycleTransition(ctx, c, prev, expectedUpdatedAt); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func normalizeDeductionIdentity(tenantID, commissionID string, d commission.Deduction) commission.Deduction {
	d.ID = strings.TrimSpace(d.ID)
	d.Description = strings.TrimSpace(d.Description)
	if d.CreatedAt.IsZero() {
		// Competência do lançamento: carimbada uma única vez, no registro. Um
		// retry idempotente mantém a data original (a obrigação derivada precisa
		// da mesma competência para não mudar de mês a cada recomputação).
		d.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if d.ID == "" {
		d.ID = events.DeriveNotificationID(
			"commission_deduction",
			tenantID,
			commissionID,
			string(d.Kind),
			d.Description,
			strconv.FormatFloat(d.Amount, 'f', -1, 64),
		)
	}
	return d
}

// taxObligationCompetence espelha a competência usada na obrigação (mês do
// lançamento, carimbado em CreatedAt). Duplicado de propósito: o serviço da
// comissão precisa consultar a MESMA competência antes de gravar.
func taxObligationCompetence(d commission.Deduction) string {
	at := d.CreatedAt
	if at.IsZero() {
		at = time.Now()
	}
	return at.UTC().Format("2006-01")
}

func sameDeductionData(left, right commission.Deduction) bool {
	return left.Kind == right.Kind && left.Description == right.Description &&
		left.Amount == right.Amount && left.RatePct == right.RatePct && left.BaseCents == right.BaseCents
}

func (s *CommissionService) persistDeductionChange(ctx context.Context, c *commission.Commission) error {
	if c.PayoutOrderID == "" {
		return s.repo.Update(ctx, c)
	}
	updater, ok := s.repo.(commissionPayoutNetUpdater)
	if !ok {
		return commission.ErrLinkedPayoutSyncUnsupported
	}
	return updater.UpdateWithLinkedPayoutNet(ctx, c)
}

// AddDeduction appends a deduction exactly once and recalculates NetAmount.
// The stable deduction ID collapses HTTP retries. When the Commission is
// already linked, persistence also updates the pending payout to NetAmount in
// the same database transaction.
func (s *CommissionService) AddDeduction(ctx context.Context, tenantID, id string, d commission.Deduction) (*commission.Commission, error) {
	d = normalizeDeductionIdentity(tenantID, id, d)
	if s.taxObligations != nil {
		// Competência do desconto que está sendo lançado: se a guia daquele mês
		// já foi recolhida, lançar mais retenção nele deixaria o razão contando
		// um imposto maior do que o retido.
		if err := s.taxObligations.EnsureDeductionChangeAllowed(ctx, tenantID, d.Kind, taxObligationCompetence(d)); err != nil {
			return nil, err
		}
	}
	// Desconto de adiantamento é validado contra o SALDO do corretor antes de
	// gravar: abater mais do que existe reduziria o líquido sem dar baixa em
	// nada. A comissão é carregada aqui só para esse caso.
	if s.brokerAdvances != nil && commission.DeductionKind(d.Kind) == commission.DeductionAdvance {
		current, err := s.repo.GetByID(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, errors.New("commission not found")
		}
		if err := s.settleBrokerAdvance(ctx, current, d); err != nil {
			return nil, err
		}
	}
	if repo, ok := s.repo.(commissionDeductionAtomicRepository); ok {
		c, prevNet, changed, err := repo.AddDeductionAtomic(ctx, tenantID, id, d)
		if err != nil {
			return nil, err
		}
		if changed {
			audit.Record(ctx, audit.Entry{
				Action:     "add_deduction",
				EntityType: "Commission",
				EntityID:   c.ID,
				Before:     map[string]any{"net_amount": prevNet},
				After: map[string]any{
					"deduction_id": d.ID,
					"net_amount":   c.NetAmount,
					"kind":         d.Kind,
					"amount":       d.Amount,
					"description":  d.Description,
				},
			})
			s.syncTaxObligation(ctx, tenantID, c.ID)
		}
		return c, nil
	}

	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("commission not found")
	}
	if existing, ok := c.DeductionByID(d.ID); ok {
		if !sameDeductionData(existing, d) {
			return nil, commission.ErrDeductionConflict
		}
		return c, nil
	}
	// One-time compatibility bridge for rows created before deductions carried
	// IDs. Attach the deterministic ID instead of duplicating an identical line.
	for i, existing := range c.Deductions {
		if existing.ID == "" && sameDeductionData(existing, d) {
			c.Deductions[i].ID = d.ID
			c.UpdatedAt = time.Now()
			if err := s.persistDeductionChange(ctx, c); err != nil {
				return nil, err
			}
			return c, nil
		}
	}
	prevNet := c.NetAmount
	if err := c.AddDeduction(d); err != nil {
		return nil, err
	}
	if err := s.persistDeductionChange(ctx, c); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "add_deduction",
		EntityType: "Commission",
		EntityID:   c.ID,
		Before:     map[string]any{"net_amount": prevNet},
		After: map[string]any{
			"deduction_id": d.ID,
			"net_amount":   c.NetAmount,
			"kind":         d.Kind,
			"amount":       d.Amount,
			"description":  d.Description,
		},
	})
	return c, nil
}

func (s *CommissionService) RemoveDeduction(ctx context.Context, tenantID, id, deductionID string) (*commission.Commission, error) {
	deductionID = strings.TrimSpace(deductionID)
	if deductionID == "" {
		return nil, commission.ErrDeductionIDRequired
	}
	if s.taxObligations != nil || s.brokerAdvances != nil {
		current, err := s.repo.GetByID(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if current != nil {
			for _, existing := range current.Deductions {
				if existing.ID != deductionID {
					continue
				}
				if s.taxObligations != nil {
					// A remoção só é permitida enquanto o tributo daquela
					// competência não foi recolhido: depois da guia paga, o
					// desconto representa dinheiro que já saiu do corretor e foi
					// para o fisco.
					if err := s.taxObligations.EnsureDeductionChangeAllowed(ctx, tenantID, existing.Kind, taxObligationCompetence(existing)); err != nil {
						return nil, err
					}
				}
				if s.brokerAdvances != nil && commission.DeductionKind(existing.Kind) == commission.DeductionAdvance {
					// Desconto de adiantamento removido devolve o saldo ao corretor.
					if err := s.brokerAdvances.ReverseFromDeduction(ctx, tenantID, current.BrokerID, current.ID, existing.ID); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if repo, ok := s.repo.(commissionDeductionAtomicRepository); ok {
		c, prevNet, changed, err := repo.RemoveDeductionAtomic(ctx, tenantID, id, deductionID)
		if err != nil {
			return nil, err
		}
		if changed {
			audit.Record(ctx, audit.Entry{
				Action:     "remove_deduction",
				EntityType: "Commission",
				EntityID:   c.ID,
				Before:     map[string]any{"net_amount": prevNet},
				After:      map[string]any{"net_amount": c.NetAmount, "deduction_id": deductionID},
			})
			s.syncTaxObligation(ctx, tenantID, c.ID)
		}
		return c, nil
	}

	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("commission not found")
	}
	prevNet := c.NetAmount
	changed, err := c.RemoveDeductionByID(deductionID)
	if err != nil {
		return nil, err
	}
	if !changed {
		return c, nil
	}
	if err := s.persistDeductionChange(ctx, c); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "remove_deduction",
		EntityType: "Commission",
		EntityID:   c.ID,
		Before:     map[string]any{"net_amount": prevNet},
		After:      map[string]any{"net_amount": c.NetAmount, "deduction_id": deductionID},
	})
	s.syncTaxObligation(ctx, tenantID, c.ID)
	return c, nil
}

// LinkPayoutOrder associa a commission a um payout_order. Chamado pelo
// PayoutOrchestrationService quando materializa orders a partir de
// commissions PAID. Idempotente.
func (s *CommissionService) LinkPayoutOrder(ctx context.Context, tenantID, id, payoutOrderID string) (*commission.Commission, error) {
	if strings.TrimSpace(payoutOrderID) == "" {
		return nil, errors.New("payout_order_id is required")
	}
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("commission not found")
	}
	if c.PayoutOrderID == payoutOrderID {
		return c, nil
	}
	if c.PayoutOrderID != "" {
		return nil, commission.ErrLinkedPayoutMismatch
	}
	expectedStatus := c.Status
	expectedUpdatedAt := c.UpdatedAt
	c.PayoutOrderID = payoutOrderID
	c.UpdatedAt = time.Now()
	if err := s.persistLifecycleTransition(ctx, c, expectedStatus, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

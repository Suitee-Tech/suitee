package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

type TransactionService struct {
	repo                    transaction.RepositoryInterface
	db                      *database.PostgresDB
	manualCommissionService *CommissionService
	manualReceiptSettlement manualReceiptSettlementHook
	journalService          *JournalService
	manualSettings          manualReceiptSettings
}

// manualReceiptSettings resolves the recurring administration fee applied to
// manual RENT receipts. Nil means the service degrades to a zero admin fee,
// mirroring TransferChargeHook's behaviour when tenant settings are unwired.
type manualReceiptSettings interface {
	Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error)
}

type transactionVersionedRepository interface {
	UpdateIfVersion(ctx context.Context, t *transaction.Transaction, expected time.Time) (bool, error)
}

type transactionOperationRepository interface {
	ListByOperationID(ctx context.Context, tenantID, operationID string) ([]*transaction.Transaction, error)
}

type manualReceiptSettlementHook interface {
	EnsureForSettledReceipt(ctx context.Context, receipt *transaction.Transaction) error
	// CancelForReceipt cancels the owner obligation released by a manual
	// receipt when the receipt is reversed/cancelled. Fails closed when the
	// owner transfer already processed (cash moved to the owner).
	CancelForReceipt(ctx context.Context, receipt *transaction.Transaction) error
}

func NewTransactionService(repo transaction.RepositoryInterface) *TransactionService {
	return &TransactionService{repo: repo}
}

// WithDB makes multi-row lifecycle operations such as reversal atomic.
func (s *TransactionService) WithDB(db *database.PostgresDB) *TransactionService {
	s.db = db
	return s
}

// WithManualCommissionService makes the manual transaction batch the single
// write boundary for its Commission obligations. Production must configure it
// together with WithDB so both repositories participate in the same SQL tx.
func (s *TransactionService) WithManualCommissionService(commissions *CommissionService) *TransactionService {
	s.manualCommissionService = commissions
	return s
}

// WithManualReceiptSettlement connects a settled manual SALE receipt to the
// corresponding owner transfer inside the same database transaction.
func (s *TransactionService) WithManualReceiptSettlement(hook manualReceiptSettlementHook) *TransactionService {
	s.manualReceiptSettlement = hook
	return s
}

// WithJournalService records cash settlement for manual Commission ledger
// lines. Production wires it with the same database transaction used by the
// Transaction and Commission aggregates.
func (s *TransactionService) WithJournalService(journalService *JournalService) *TransactionService {
	s.journalService = journalService
	return s
}

// WithManualReceiptSettings resolves the recurring administration fee applied
// to manual RENT receipts. When nil, the service degrades to a zero admin fee.
func (s *TransactionService) WithManualReceiptSettings(settings manualReceiptSettings) *TransactionService {
	s.manualSettings = settings
	return s
}

func (s *TransactionService) updateIfCurrent(ctx context.Context, t *transaction.Transaction, expected time.Time) error {
	versioned, ok := s.repo.(transactionVersionedRepository)
	if !ok {
		return s.repo.Update(ctx, t)
	}
	updated, err := versioned.UpdateIfVersion(ctx, t, expected)
	if err != nil {
		return err
	}
	if !updated {
		return transaction.ErrConcurrentModification
	}
	return nil
}

func (s *TransactionService) Create(ctx context.Context, t *transaction.Transaction) error {
	if t == nil {
		return fmt.Errorf("transaction is required")
	}
	// Manual journal entries (entradas/saídas avulsas) may have no contract, as
	// long as they carry a description or category so the ledger stays
	// meaningful. Event-driven callers always pass a contract_id.
	if t.ContractID == "" && strings.TrimSpace(t.Description) == "" && strings.TrimSpace(t.Category) == "" {
		return fmt.Errorf("a manual transaction requires a description or category when contract_id is empty")
	}
	if err := t.Validate(); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "transaction.created",
		EntityType: "Transaction",
		EntityID:   t.ID,
		After: map[string]any{
			"type":        t.Type,
			"value":       t.Value,
			"status":      t.Status,
			"description": t.Description,
			"contract_id": t.ContractID,
		},
		Metadata: map[string]any{
			"tenant_id":       t.TenantID,
			"source_event_id": t.SourceEventID,
		},
	})
	return nil
}

func (s *TransactionService) CreateWithSourceEventID(ctx context.Context, sourceEventID string, t *transaction.Transaction) error {
	if strings.TrimSpace(sourceEventID) != "" && t != nil {
		t.SourceEventID = sourceEventID
	}
	err := s.Create(ctx, t)
	if errors.Is(err, transaction.ErrTransactionAlreadyExists) {
		return nil
	}
	return err
}

func (s *TransactionService) CreateFromRequest(ctx context.Context, sourceEventID string, t *transaction.Transaction) (*transaction.Transaction, error) {
	sourceEventID = strings.TrimSpace(sourceEventID)
	if sourceEventID == "" {
		if err := s.Create(ctx, t); err != nil {
			return nil, err
		}
		return t, nil
	}
	if t != nil {
		t.SourceEventID = sourceEventID
	}
	err := s.Create(ctx, t)
	if errors.Is(err, transaction.ErrTransactionAlreadyExists) {
		tenantID := ""
		if t != nil {
			tenantID = t.TenantID
		}
		existing, gerr := s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
		if gerr != nil {
			return nil, gerr
		}
		if existing != nil {
			return existing, nil
		}
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// BatchCreateInput is one line-item in a manual batch posting.
type BatchCreateInput struct {
	ContractID    string  `json:"contract_id"`
	DealID        string  `json:"deal_id,omitempty"`
	Type          string  `json:"type"`
	Description   string  `json:"description"`
	Value         float64 `json:"value"`
	DateStr       string  `json:"date"` // ISO-8601
	Category      string  `json:"category,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	BrokerID      string  `json:"broker_id,omitempty"`
	Role          string  `json:"role,omitempty"`
	Flow          string  `json:"flow,omitempty"` // sale | rent
	// DueDateStr is the programmed first due date brought from the pipeline
	// ("YYYY-MM-DD"). Optional: it becomes the posting's due_date provenance.
	DueDateStr string `json:"due_date,omitempty"`
	// PaymentPlan is the payment plan label programmed in the pipeline
	// (ex.: "Entrada + 11x de R$ 8.333,33"). Informational provenance.
	PaymentPlan string `json:"payment_plan,omitempty"`
}

// BatchCreateOutput wraps the result of a batch insert.
type BatchCreateOutput struct {
	Transaction *transaction.Transaction `json:"transaction"`
	Index       int                      `json:"index"`
	Error       string                   `json:"error,omitempty"`
}

// ManualCommissionInput is an obligation materialized together with a manual
// ledger batch. Values are magnitudes; role/type carries economic direction.
type ManualCommissionInput struct {
	DealID     string  `json:"deal_id"`
	ContractID string  `json:"contract_id,omitempty"`
	BrokerID   string  `json:"broker_id"`
	Role       string  `json:"role,omitempty"`
	Percentage float64 `json:"percentage,omitempty"`
	Amount     float64 `json:"amount"`
}

// ManualCommissionRetentionInput records the intentional part of a receipt's
// commission pool retained as agency revenue. It participates in validation
// and idempotency, but never materializes a Commission payable or a separate
// commission Transaction line.
type ManualCommissionRetentionInput struct {
	Percentage float64 `json:"percentage"`
	Amount     float64 `json:"amount"`
}

var manualCommissionRoles = map[string]struct{}{
	"captador":    {},
	"co_captador": {},
	"visitador":   {},
	"atendente":   {},
	"fechamento":  {},
}

type manualCommissionMirrorKey struct {
	contractID string
	brokerID   string
	role       string
	amount     int64
}

func manualTransactionMirrorKey(item BatchCreateInput) manualCommissionMirrorKey {
	return manualCommissionMirrorKey{
		contractID: strings.TrimSpace(item.ContractID),
		brokerID:   strings.TrimSpace(item.BrokerID),
		role:       strings.ToLower(strings.TrimSpace(item.Role)),
		amount:     int64(math.Round(item.Value * 100)),
	}
}

func manualAggregateMirrorKey(item ManualCommissionInput) manualCommissionMirrorKey {
	return manualCommissionMirrorKey{
		contractID: strings.TrimSpace(item.ContractID),
		brokerID:   strings.TrimSpace(item.BrokerID),
		role:       strings.ToLower(strings.TrimSpace(item.Role)),
		amount:     int64(math.Round(item.Amount * 100)),
	}
}

// BatchCreatePosting atomically persists the manual ledger lines and every
// linked Commission. The ordinary transaction-only endpoint remains valid,
// while a request with commissions fails closed if no shared DB transaction is
// configured.
//
// SECURITY (achado A13 da revisão de permissões de 2026-09-19): o solicitante da
// comissão avulsa é lido do CONTEXTO autenticado (`account_id`, o mesmo valor que
// a trilha de auditoria usa) em vez de virar mais um parâmetro. Aqui não existe
// o conflito de chamadores que justifica o parâmetro explícito em
// Approve/MarkPaidWithProof (onde o caminho de payout precisa dizer "sem ator"):
// quem chama esta função é sempre a borda HTTP, com a identidade da requisição
// já resolvida pelo middleware. Worker ou import sem identidade resulta em
// vazio, e vazio não cria conflito de segregação de funções.
func (s *TransactionService) BatchCreatePosting(ctx context.Context, tenantID, operationKey string, items []BatchCreateInput, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput) ([]BatchCreateOutput, error) {
	requestedBy, _ := pkgcontext.GetAccountID(ctx)
	return s.batchCreatePosting(ctx, tenantID, operationKey, requestedBy, items, commissions, retention)
}

func (s *TransactionService) batchCreatePosting(ctx context.Context, tenantID, operationKey, requestedBy string, items []BatchCreateInput, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput) ([]BatchCreateOutput, error) {
	tenantID = strings.TrimSpace(tenantID)
	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return nil, transaction.ErrBatchOperationKeyRequired
	}
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", transaction.ErrBatchValidation)
	}
	if len(commissions) == 0 {
		if err := validateCommissionMirrors(items, nil); err != nil {
			return nil, err
		}
		flow := manualReceiptFlow(items)
		if flow == "" {
			if retention != nil {
				return nil, fmt.Errorf("%w: commission_retention requires a receipt", transaction.ErrBatchValidation)
			}
			return s.BatchCreate(ctx, tenantID, operationKey, items)
		}
		if flow == "rent" && retention != nil {
			return nil, fmt.Errorf("%w: rental commission is frozen at signing; a rent receipt funds the intermediation plan", transaction.ErrBatchValidation)
		}
		adminFeePct := 0.0
		if flow == "rent" {
			var feeErr error
			adminFeePct, feeErr = s.resolveManualReceiptAdminFeePct(ctx, tenantID)
			if feeErr != nil {
				return nil, feeErr
			}
		}
		receiptContractID, err := manualCommissionReceiptContract(items)
		if err != nil {
			return nil, err
		}
		// A plain RENT receipt carries no commission pool: the administration fee
		// is the only agency slice and the owner keeps the remainder. SALE (and
		// rent-with-retention) still requires the commission pool to close 100%.
		var normalizedRetention *ManualCommissionRetentionInput
		if flow == "rent" && retention == nil {
			normalizedRetention = nil
		} else {
			normalizedRetention, err = validateManualCommissionRetention(receiptContractID != "", 0, 0, nil, retention)
			if err != nil {
				return nil, err
			}
		}
		receiptBreakdown, receiptDealID, err := buildManualReceiptBreakdown(items, nil, normalizedRetention, adminFeePct)
		if err != nil {
			return nil, err
		}
		work := func(execCtx context.Context) ([]BatchCreateOutput, error) {
			outputs, _, err := s.batchCreate(execCtx, tenantID, operationKey, items, nil, normalizedRetention, receiptBreakdown, receiptDealID)
			return outputs, err
		}
		if database.TxFromCtx(ctx) != nil || s.db == nil {
			return work(ctx)
		}
		var outputs []BatchCreateOutput
		err = s.db.RunInTx(ctx, func(dbTx *sqlx.Tx) error {
			var err error
			outputs, err = work(database.WithTx(ctx, dbTx))
			return err
		})
		return outputs, err
	}
	if s.manualCommissionService == nil {
		return nil, fmt.Errorf("manual commission persistence is not configured")
	}
	if flow := manualReceiptFlow(items); flow == "rent" {
		return nil, fmt.Errorf("%w: rental commission is frozen at signing; a rent receipt funds the intermediation plan instead of distributing commission", transaction.ErrBatchValidation)
	}
	receiptContractID, err := manualCommissionReceiptContract(items)
	if err != nil {
		return nil, err
	}
	hasReceipt := receiptContractID != ""
	normalizedCommissions := make([]ManualCommissionInput, len(commissions))
	var rawPercentageTotal float64
	for i, item := range commissions {
		item.DealID = strings.TrimSpace(item.DealID)
		item.ContractID = strings.TrimSpace(item.ContractID)
		item.BrokerID = strings.TrimSpace(item.BrokerID)
		item.Role = strings.ToLower(strings.TrimSpace(item.Role))
		if item.DealID == "" || item.BrokerID == "" {
			return nil, fmt.Errorf("%w: commission %d requires deal_id and broker_id", transaction.ErrBatchValidation, i)
		}
		if hasReceipt && item.ContractID != receiptContractID {
			return nil, fmt.Errorf("%w: commission %d contract_id %q does not match receipt contract_id %q", transaction.ErrBatchValidation, i, item.ContractID, receiptContractID)
		}
		if item.Role == "imobiliaria" {
			return nil, fmt.Errorf("%w: commission %d cannot make agency retention a payable", transaction.ErrBatchValidation, i)
		}
		if _, ok := manualCommissionRoles[item.Role]; !ok {
			return nil, fmt.Errorf("%w: commission %d requires a valid role", transaction.ErrBatchValidation, i)
		}
		if item.Amount <= 0 || math.IsNaN(item.Amount) || math.IsInf(item.Amount, 0) {
			return nil, fmt.Errorf("%w: commission %d amount must be positive and finite", transaction.ErrBatchValidation, i)
		}
		if !hasCentPrecision(item.Amount) {
			return nil, fmt.Errorf("%w: commission %d amount must have at most two decimal places", transaction.ErrBatchValidation, i)
		}
		if item.Percentage < 0 || item.Percentage > 100 || math.IsNaN(item.Percentage) || math.IsInf(item.Percentage, 0) {
			return nil, fmt.Errorf("%w: commission %d percentage must be between 0 and 100", transaction.ErrBatchValidation, i)
		}
		if hasReceipt && item.Percentage <= 0 {
			return nil, fmt.Errorf("%w: commission %d percentage must be in (0, 100] for a receipt distribution", transaction.ErrBatchValidation, i)
		}
		rawPercentageTotal += item.Percentage
		normalizedCommissions[i] = item
	}
	percentageTotal := roundManualCommissionPercentage(rawPercentageTotal)
	// Manual Commission entities are liabilities payable to brokers. Agency
	// retention is an explicit command component: it closes the distribution
	// and the fingerprint without becoming a payable or ledger mirror.
	if hasReceipt && len(normalizedCommissions) > 0 && percentageTotal <= 0 {
		return nil, fmt.Errorf("%w: commission percentages must total more than 0%%", transaction.ErrBatchValidation)
	}
	if percentageTotal > 100.01 {
		return nil, fmt.Errorf("%w: commission percentages total %.4f%%, above 100%%", transaction.ErrBatchValidation, percentageTotal)
	}
	normalizedRetention, err := validateManualCommissionRetention(hasReceipt, percentageTotal, rawPercentageTotal, normalizedCommissions, retention)
	if err != nil {
		return nil, err
	}
	if err := validateCommissionMirrors(items, normalizedCommissions); err != nil {
		return nil, err
	}
	adminFeePct := 0.0
	if flow := manualReceiptFlow(items); flow == "rent" {
		var feeErr error
		adminFeePct, feeErr = s.resolveManualReceiptAdminFeePct(ctx, tenantID)
		if feeErr != nil {
			return nil, feeErr
		}
	}
	receiptBreakdown, receiptDealID, err := buildManualReceiptBreakdown(items, normalizedCommissions, normalizedRetention, adminFeePct)
	if err != nil {
		return nil, err
	}

	work := func(execCtx context.Context) ([]BatchCreateOutput, error) {
		outputs, retry, err := s.batchCreate(execCtx, tenantID, operationKey, items, normalizedCommissions, normalizedRetention, receiptBreakdown, receiptDealID)
		if err != nil {
			return nil, err
		}
		ledgerTransactionIDs, err := bindManualCommissionLedgerTransactions(outputs, normalizedCommissions)
		if err != nil {
			return nil, err
		}
		if err := s.verifyManualCommissionCardinality(execCtx, tenantID, operationKey, normalizedCommissions, ledgerTransactionIDs, retry); err != nil {
			return nil, err
		}
		// A13: quem registra o lançamento é o solicitante das comissões que ele
		// cria — mesma regra do payout order (quem cria não aprova). Comissão de
		// recibo entra aqui como as demais: o operador que registra o recebimento
		// é quem pede as obrigações que ele distribui. A dispensa de autoridade
		// financeira (admin/gestor) segue valendo na borda e é o que mantém a
		// imobiliária de um operador só funcionando.
		commissionRequester := strings.TrimSpace(requestedBy)
		for i, item := range normalizedCommissions {
			sourceID := events.DeriveNotificationID(operationKey, "manual_commission", tenantID, strconv.Itoa(i))
			created, err := s.manualCommissionService.createWithRoleFromEventAndLedger(
				execCtx, sourceID, item.DealID, item.BrokerID, item.ContractID,
				item.Role, item.Percentage, item.Amount, tenantID, "", "", "", "", ledgerTransactionIDs[i], 0, commissionRequester,
			)
			if err != nil {
				return nil, fmt.Errorf("create manual commission %d: %w", i, err)
			}
			if !sameManualCommission(created, item, tenantID, sourceID, ledgerTransactionIDs[i]) {
				return nil, fmt.Errorf("%w: commission %d", transaction.ErrBatchIdempotencyConflict, i)
			}
			// GROSS model: every manual commission accrues the broker obligation
			// (commission_expense / commission_payable) at creation, mirroring the
			// event-driven signing accrual. The settlement then debits
			// commission_payable to clear the liability.
			if s.journalService != nil {
				amountCents := int64(math.Round(item.Amount * 100))
				flow := ""
				accrualDate := created.CreatedAt
				for _, output := range outputs {
					if output.Transaction != nil && output.Transaction.ID == ledgerTransactionIDs[i] {
						flow = items[output.Index].Flow
						if flow == "" && receiptBreakdown != nil {
							flow = receiptBreakdown.Flow
						}
						accrualDate = output.Transaction.Date
						break
					}
				}
				accrual, buildErr := journal.New(tenantID, item.ContractID,
					fmt.Sprintf("Comissão - %s (%.1f%%)", item.Role, item.Percentage),
					accrualDate,
					[]journal.Leg{
						{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: amountCents, Flow: flow},
						{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: amountCents, Flow: flow, BeneficiaryPartyID: item.BrokerID},
					},
				)
				if buildErr != nil {
					return nil, fmt.Errorf("build manual commission accrual %d: %w", i, buildErr)
				}
				accrual.OperationID = ledgerTransactionIDs[i]
				accrualSourceID := events.DeriveNotificationID(sourceID, "journal_commission")
				if postErr := s.journalService.CreateWithSourceEventID(execCtx, accrualSourceID, accrual); postErr != nil {
					return nil, fmt.Errorf("post manual commission accrual %d: %w", i, postErr)
				}
			}
		}
		return outputs, nil
	}

	if database.TxFromCtx(ctx) != nil {
		return work(ctx)
	}
	if s.db == nil {
		return nil, fmt.Errorf("atomic manual posting requires a database transaction")
	}
	var outputs []BatchCreateOutput
	err = s.db.RunInTx(ctx, func(dbTx *sqlx.Tx) error {
		var err error
		outputs, err = work(database.WithTx(ctx, dbTx))
		return err
	})
	return outputs, err
}

func manualCommissionReceiptContract(items []BatchCreateInput) (string, error) {
	receiptContractID := ""
	for i, item := range items {
		if transaction.Type(strings.TrimSpace(item.Type)) != transaction.TypeReceita {
			continue
		}
		contractID := strings.TrimSpace(item.ContractID)
		if contractID == "" {
			return "", fmt.Errorf("%w: receipt transaction %d requires contract_id when commissions are distributed", transaction.ErrBatchValidation, i)
		}
		if receiptContractID == "" {
			receiptContractID = contractID
			continue
		}
		if receiptContractID != contractID {
			return "", fmt.Errorf("%w: receipt commission distribution cannot span contracts %q and %q", transaction.ErrBatchValidation, receiptContractID, contractID)
		}
	}
	return receiptContractID, nil
}

func roundManualCommissionPercentage(value float64) float64 {
	return math.Round(value*10_000) / 10_000
}

// manualReceiptFlow returns the economic flow of the single receipt line in a
// manual batch, or "" when the batch has no distributed receipt. It accepts the
// explicit `flow` field and falls back to a description heuristic for legacy
// payloads that predate the field.
func manualReceiptFlow(items []BatchCreateInput) string {
	for _, item := range items {
		if transaction.Type(strings.TrimSpace(item.Type)) != transaction.TypeReceita {
			continue
		}
		flow := strings.ToLower(strings.TrimSpace(item.Flow))
		desc := strings.ToLower(item.Description)
		if flow == "sale" || (flow == "" && strings.Contains(desc, "venda")) {
			return "sale"
		}
		if flow == "rent" || (flow == "" && (strings.Contains(desc, "locação") || strings.Contains(desc, "locacao") || strings.Contains(desc, "aluguel"))) {
			return "rent"
		}
	}
	return ""
}

// resolveManualReceiptAdminFeePct loads the tenant administration fee for a
// manual RENT receipt. It degrades to zero when settings are not wired, so the
// sale path and focused tests remain unaffected.
func (s *TransactionService) resolveManualReceiptAdminFeePct(ctx context.Context, tenantID string) (float64, error) {
	if s.manualSettings == nil {
		return 0, nil
	}
	settings, err := s.manualSettings.Get(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("load tenant financial settings for manual receipt: %w", err)
	}
	if settings == nil {
		return 0, nil
	}
	if settings.AdminFeePct < 0 || settings.AdminFeePct > 100 {
		return 0, errors.New("tenant admin fee percentage is invalid")
	}
	return settings.AdminFeePct, nil
}

func buildManualReceiptBreakdown(items []BatchCreateInput, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput, adminFeePct float64) (*transaction.EconomicBreakdown, string, error) {
	var receipt *BatchCreateInput
	for i := range items {
		if transaction.Type(strings.TrimSpace(items[i].Type)) != transaction.TypeReceita {
			continue
		}
		if receipt != nil {
			return nil, "", fmt.Errorf("%w: a distributed manual receipt must contain exactly one receipt line", transaction.ErrBatchValidation)
		}
		receipt = &items[i]
	}
	if receipt == nil {
		return nil, "", nil
	}
	flow := strings.ToLower(strings.TrimSpace(receipt.Flow))
	desc := strings.ToLower(receipt.Description)
	if flow == "" && strings.Contains(desc, "venda") {
		flow = "sale"
	}
	if flow == "" && (strings.Contains(desc, "locação") || strings.Contains(desc, "locacao") || strings.Contains(desc, "aluguel")) {
		flow = "rent"
	}
	if flow != "sale" && flow != "rent" {
		return nil, "", nil
	}
	grossCents := int64(math.Round(receipt.Value * 100))
	var commissionPayableCents int64
	dealID := strings.TrimSpace(receipt.DealID)
	for _, item := range commissions {
		commissionPayableCents += int64(math.Round(item.Amount * 100))
		if dealID == "" {
			dealID = strings.TrimSpace(item.DealID)
		} else if dealID != strings.TrimSpace(item.DealID) {
			return nil, "", fmt.Errorf("%w: receipt commissions cannot span multiple deals", transaction.ErrBatchValidation)
		}
	}
	agencyRetainedCents := int64(0)
	if retention != nil {
		agencyRetainedCents = int64(math.Round(retention.Amount * 100))
	}
	commissionPoolCents := commissionPayableCents + agencyRetainedCents
	adminFeeCents := int64(0)
	if flow == "rent" {
		if adminFeePct < 0 || adminFeePct > 100 {
			return nil, "", errors.New("tenant admin fee percentage is invalid")
		}
		adminFeeCents = int64(math.Round(float64(grossCents) * adminFeePct / 100))
	}
	ownerPayableCents := grossCents - commissionPoolCents - adminFeeCents
	if grossCents <= 0 || ownerPayableCents < 0 {
		return nil, "", fmt.Errorf("%w: commissions and fees exceed the receipt value", transaction.ErrBatchValidation)
	}
	if dealID == "" {
		return nil, "", fmt.Errorf("%w: receipt requires deal_id", transaction.ErrBatchValidation)
	}
	return &transaction.EconomicBreakdown{
		Flow:                   flow,
		GrossCents:             grossCents,
		CommissionPoolCents:    commissionPoolCents,
		CommissionPayableCents: commissionPayableCents,
		AgencyRetainedCents:    agencyRetainedCents,
		OwnerPayableCents:      ownerPayableCents,
		AdminFeeCents:          adminFeeCents,
	}, dealID, nil
}

func validateManualCommissionRetention(hasReceipt bool, brokerPercentageTotal, rawBrokerPercentageTotal float64, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput) (*ManualCommissionRetentionInput, error) {
	if !hasReceipt {
		if retention != nil {
			return nil, fmt.Errorf("%w: commission_retention requires a receipt", transaction.ErrBatchValidation)
		}
		return nil, nil
	}

	if retention == nil {
		if brokerPercentageTotal >= 100 && brokerPercentageTotal <= 100.01 {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: commission_retention is required for the remaining %.4f%%", transaction.ErrBatchValidation, 100-brokerPercentageTotal)
	}
	if brokerPercentageTotal >= 100 {
		return nil, fmt.Errorf("%w: commission_retention is not allowed when broker commissions already total 100%%", transaction.ErrBatchValidation)
	}

	normalized := *retention
	normalized.Percentage = roundManualCommissionPercentage(normalized.Percentage)
	if math.IsNaN(normalized.Percentage) || math.IsInf(normalized.Percentage, 0) || normalized.Percentage <= 0 || normalized.Percentage > 100 {
		return nil, fmt.Errorf("%w: commission_retention percentage must be greater than 0 and at most 100", transaction.ErrBatchValidation)
	}
	expectedRetentionPercentage := roundManualCommissionPercentage(100 - brokerPercentageTotal)
	if math.Abs(normalized.Percentage-expectedRetentionPercentage) > 0.0000001 {
		return nil, fmt.Errorf("%w: commission_retention percentage is %.4f%%, want the exact %.4f%% remainder", transaction.ErrBatchValidation, normalized.Percentage, expectedRetentionPercentage)
	}
	if math.IsNaN(normalized.Amount) || math.IsInf(normalized.Amount, 0) || normalized.Amount < 0 {
		return nil, fmt.Errorf("%w: commission_retention amount must be non-negative and finite", transaction.ErrBatchValidation)
	}
	if !hasCentPrecision(normalized.Amount) {
		return nil, fmt.Errorf("%w: commission_retention amount must have at most two decimal places", transaction.ErrBatchValidation)
	}

	var brokerAmountCents int64
	for _, item := range commissions {
		brokerAmountCents += int64(math.Round(item.Amount * 100))
	}
	retentionAmountCents := int64(math.Round(normalized.Amount * 100))
	poolCents := brokerAmountCents + retentionAmountCents
	expectedBrokerCents := int64(math.Round(float64(poolCents) * brokerPercentageTotal / 100))
	if brokerAmountCents != expectedBrokerCents {
		return nil, fmt.Errorf("%w: broker commission amount is %d cents, want %d cents for %.4f%% of the %d-cent pool", transaction.ErrBatchValidation, brokerAmountCents, expectedBrokerCents, brokerPercentageTotal, poolCents)
	}
	expectedCommissionCents := allocateManualCommissionCents(expectedBrokerCents, commissions, rawBrokerPercentageTotal)
	for i, item := range commissions {
		actualCents := int64(math.Round(item.Amount * 100))
		if actualCents != expectedCommissionCents[i] {
			return nil, fmt.Errorf("%w: commission %d amount is %d cents, want %d cents from the deterministic broker allocation", transaction.ErrBatchValidation, i, actualCents, expectedCommissionCents[i])
		}
	}
	return &normalized, nil
}

func allocateManualCommissionCents(brokerPoolCents int64, commissions []ManualCommissionInput, percentageTotal float64) []int64 {
	allocated := make([]int64, len(commissions))
	if brokerPoolCents <= 0 || len(commissions) == 0 || percentageTotal <= 0 {
		return allocated
	}
	type remainderRank struct {
		index    int
		fraction float64
	}
	ranks := make([]remainderRank, len(commissions))
	var floorTotal int64
	for i, item := range commissions {
		exact := float64(brokerPoolCents) * item.Percentage / percentageTotal
		floorCents := int64(math.Floor(exact))
		allocated[i] = floorCents
		floorTotal += floorCents
		ranks[i] = remainderRank{index: i, fraction: exact - float64(floorCents)}
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].fraction == ranks[j].fraction {
			return ranks[i].index < ranks[j].index
		}
		return ranks[i].fraction > ranks[j].fraction
	})
	for remainder := brokerPoolCents - floorTotal; remainder > 0; remainder-- {
		allocated[ranks[(brokerPoolCents-floorTotal-remainder)%int64(len(ranks))].index]++
	}
	return allocated
}

func validateCommissionMirrors(items []BatchCreateInput, commissions []ManualCommissionInput) error {
	ledgerCounts := make(map[manualCommissionMirrorKey]int)
	commissionCounts := make(map[manualCommissionMirrorKey]int)
	for i, item := range items {
		if transaction.Type(strings.TrimSpace(item.Type)) != transaction.TypeComissao {
			continue
		}
		key := manualTransactionMirrorKey(item)
		if key.brokerID == "" {
			return fmt.Errorf("%w: commission transaction %d requires broker_id", transaction.ErrBatchValidation, i)
		}
		if key.role == "imobiliaria" {
			return fmt.Errorf("%w: commission transaction %d cannot make agency retention a payable", transaction.ErrBatchValidation, i)
		}
		if _, ok := manualCommissionRoles[key.role]; !ok {
			return fmt.Errorf("%w: commission transaction %d requires a valid role", transaction.ErrBatchValidation, i)
		}
		ledgerCounts[key]++
	}
	for _, item := range commissions {
		key := manualAggregateMirrorKey(item)
		commissionCounts[key]++
	}
	if len(ledgerCounts) != len(commissionCounts) {
		return fmt.Errorf("%w: commission transactions and aggregates must match one-to-one", transaction.ErrBatchValidation)
	}
	for key, expected := range commissionCounts {
		if ledgerCounts[key] != expected {
			return fmt.Errorf("%w: commission mirror mismatch for contract %q broker %q role %q amount_cents %d", transaction.ErrBatchValidation, key.contractID, key.brokerID, key.role, key.amount)
		}
	}
	return nil
}

func bindManualCommissionLedgerTransactions(outputs []BatchCreateOutput, commissions []ManualCommissionInput) ([]string, error) {
	ledgerIDs := make(map[manualCommissionMirrorKey][]string)
	for _, output := range outputs {
		if output.Transaction == nil || output.Transaction.Type != transaction.TypeComissao {
			continue
		}
		key := manualCommissionMirrorKey{
			contractID: strings.TrimSpace(output.Transaction.ContractID),
			brokerID:   strings.TrimSpace(output.Transaction.BrokerID),
			role:       strings.ToLower(strings.TrimSpace(output.Transaction.Role)),
			amount:     int64(math.Round(output.Transaction.Value * 100)),
		}
		ledgerIDs[key] = append(ledgerIDs[key], output.Transaction.ID)
	}

	bound := make([]string, len(commissions))
	for i, item := range commissions {
		key := manualAggregateMirrorKey(item)
		candidates := ledgerIDs[key]
		if len(candidates) == 0 {
			return nil, fmt.Errorf("%w: commission %d has no deterministic ledger mirror", transaction.ErrBatchValidation, i)
		}
		bound[i] = candidates[0]
		ledgerIDs[key] = candidates[1:]
	}
	return bound, nil
}

func sameManualCommission(created *commission.Commission, expected ManualCommissionInput, tenantID, sourceID, ledgerTransactionID string) bool {
	if created == nil {
		return false
	}
	return created.TenantID == tenantID &&
		created.SourceEventID == sourceID &&
		created.LedgerTransactionID == ledgerTransactionID &&
		created.DealID == strings.TrimSpace(expected.DealID) &&
		created.ContractID == strings.TrimSpace(expected.ContractID) &&
		created.BrokerID == strings.TrimSpace(expected.BrokerID) &&
		strings.EqualFold(strings.TrimSpace(created.Role), strings.TrimSpace(expected.Role)) &&
		math.Abs(created.Percentage-expected.Percentage) <= 0.0001 &&
		math.Abs(created.Amount-expected.Amount) <= 0.005
}

func (s *TransactionService) verifyManualCommissionCardinality(ctx context.Context, tenantID, operationKey string, expected []ManualCommissionInput, ledgerTransactionIDs []string, retry bool) error {
	for i, item := range expected {
		sourceID := events.DeriveNotificationID(operationKey, "manual_commission", tenantID, strconv.Itoa(i))
		existing, err := s.manualCommissionService.repo.GetBySourceEventID(ctx, tenantID, sourceID)
		if err != nil {
			return fmt.Errorf("lookup manual commission %d: %w", i, err)
		}
		if retry {
			if existing == nil || !sameManualCommission(existing, item, tenantID, sourceID, ledgerTransactionIDs[i]) {
				return fmt.Errorf("%w: commission %d", transaction.ErrBatchIdempotencyConflict, i)
			}
			continue
		}
		if existing != nil {
			return fmt.Errorf("%w: commission %d exists without its posting", transaction.ErrBatchIdempotencyConflict, i)
		}
	}

	extraSourceID := events.DeriveNotificationID(operationKey, "manual_commission", tenantID, strconv.Itoa(len(expected)))
	extra, err := s.manualCommissionService.repo.GetBySourceEventID(ctx, tenantID, extraSourceID)
	if err != nil {
		return fmt.Errorf("lookup extra manual commission: %w", err)
	}
	if extra != nil {
		return fmt.Errorf("%w: posting has more commissions than the retried command", transaction.ErrBatchIdempotencyConflict)
	}
	return nil
}

// BatchCreate writes an atomic manual posting. The operation key is mandatory
// and produces stable posting and source IDs per line. Every item is validated
// before the repository is called; a retry returns the rows already persisted
// under those source IDs rather than fresh request-side objects.
func (s *TransactionService) BatchCreate(ctx context.Context, tenantID string, operationKey string, items []BatchCreateInput) ([]BatchCreateOutput, error) {
	if database.TxFromCtx(ctx) != nil || s.db == nil {
		outputs, _, err := s.batchCreate(ctx, tenantID, operationKey, items, nil, nil, nil, "")
		return outputs, err
	}

	var outputs []BatchCreateOutput
	err := s.db.RunInTx(ctx, func(dbTx *sqlx.Tx) error {
		var err error
		outputs, _, err = s.batchCreate(database.WithTx(ctx, dbTx), tenantID, operationKey, items, nil, nil, nil, "")
		return err
	})
	return outputs, err
}

func (s *TransactionService) batchCreate(ctx context.Context, tenantID string, operationKey string, items []BatchCreateInput, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput, receiptBreakdown *transaction.EconomicBreakdown, receiptDealID string) ([]BatchCreateOutput, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return nil, false, transaction.ErrBatchOperationKeyRequired
	}
	if tenantID == "" {
		return nil, false, fmt.Errorf("%w: tenant_id is required", transaction.ErrBatchValidation)
	}
	if len(items) == 0 {
		return nil, false, fmt.Errorf("%w: batch must contain at least one item", transaction.ErrBatchValidation)
	}

	// Keep the pre-existing non-empty-key source derivation stable so retries of
	// batches posted before this hardening still hit the same unique index row.
	operationID := events.DeriveNotificationID(operationKey, "batch", tenantID)
	txs := make([]*transaction.Transaction, 0, len(items))
	outputs := make([]BatchCreateOutput, len(items))
	validationErrors := make([]string, 0)

	for i, item := range items {
		outputs[i].Index = i
		date, err := parseBatchTransactionDate(item.DateStr)
		if err != nil {
			outputs[i].Error = fmt.Sprintf("invalid date: %v", err)
			validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
			continue
		}
		tt := transaction.Type(strings.TrimSpace(item.Type))
		if !transaction.IsValidType(tt) {
			outputs[i].Error = fmt.Sprintf("invalid transaction type: %s", item.Type)
			validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
			continue
		}
		if math.IsNaN(item.Value) || math.IsInf(item.Value, 0) {
			outputs[i].Error = "transaction value must be finite"
			validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
			continue
		}
		if !hasCentPrecision(item.Value) {
			outputs[i].Error = "transaction value must have at most two decimal places"
			validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
			continue
		}
		tx := transaction.New(item.ContractID, tt, item.Description, item.Value, date, tenantID)
		tx.Category = item.Category
		tx.PaymentMethod = item.PaymentMethod
		tx.BrokerID = strings.TrimSpace(item.BrokerID)
		tx.Role = strings.ToLower(strings.TrimSpace(item.Role))
		tx.PaymentPlan = strings.TrimSpace(item.PaymentPlan)
		if strings.TrimSpace(item.DueDateStr) != "" {
			dueDate, dueErr := parseBatchTransactionDate(item.DueDateStr)
			if dueErr != nil {
				outputs[i].Error = fmt.Sprintf("invalid due_date: %v", dueErr)
				validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
				continue
			}
			tx.DueDate = &dueDate
		}
		if tt == transaction.TypeReceita && receiptBreakdown != nil {
			breakdown := *receiptBreakdown
			tx.DealID = receiptDealID
			tx.EconomicBreakdown = &breakdown
		}

		lineIndex := strconv.Itoa(i)
		tx.OperationID = operationID
		tx.SourceEventID = events.DeriveNotificationID(operationID, "batch_line", lineIndex)
		tx.ID = events.DeriveNotificationID(tx.SourceEventID, "posting")
		tx.PostingID = tx.ID
		if err := tx.Validate(); err != nil {
			outputs[i].Error = err.Error()
			validationErrors = append(validationErrors, fmt.Sprintf("item %d: %s", i, outputs[i].Error))
			continue
		}

		txs = append(txs, tx)
		outputs[i] = BatchCreateOutput{
			Transaction: tx,
			Index:       i,
		}
	}

	if len(validationErrors) > 0 {
		return outputs, false, fmt.Errorf("%w: %s", transaction.ErrBatchValidation, strings.Join(validationErrors, "; "))
	}

	fingerprint := manualPostingFingerprint(txs, commissions, retention)
	for _, tx := range txs {
		tx.BatchFingerprint = fingerprint
		tx.BatchLineCount = len(txs)
		tx.CommissionCount = len(commissions)
	}

	// Production serializes every command sharing an idempotency key before
	// examining or writing rows. This closes the race where two concurrent
	// requests with different cardinalities both observe an empty operation.
	if s.db != nil && database.TxFromCtx(ctx) != nil {
		lockKey := tenantID + ":manual-posting:" + operationKey
		if _, err := s.db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
			return nil, false, fmt.Errorf("lock manual posting: %w", err)
		}
	}

	canonicalOutputs, retry, err := s.preflightBatch(ctx, tenantID, operationID, txs)
	if err != nil {
		return nil, false, err
	}
	if retry {
		return canonicalOutputs, true, nil
	}

	if err := s.repo.CreateBatch(ctx, txs); err != nil {
		return nil, false, fmt.Errorf("batch create: %w", err)
	}

	// Always read the committed rows back. On a retry the INSERTs are no-ops on
	// the source_event_id index, so these are the canonical original entities.
	for i, tx := range txs {
		canonical, err := s.repo.GetBySourceEventID(ctx, tenantID, tx.SourceEventID)
		if err != nil {
			return nil, false, fmt.Errorf("batch canonical lookup for item %d: %w", i, err)
		}
		if canonical == nil {
			return nil, false, fmt.Errorf("batch canonical lookup for item %d returned no transaction", i)
		}
		if !sameBatchTransaction(canonical, tx) || !sameBatchManifest(canonical, tx) {
			return nil, false, fmt.Errorf("%w: transaction item %d", transaction.ErrBatchIdempotencyConflict, i)
		}
		outputs[i].Transaction = canonical
	}

	return outputs, false, nil
}

func (s *TransactionService) preflightBatch(ctx context.Context, tenantID, operationID string, expected []*transaction.Transaction) ([]BatchCreateOutput, bool, error) {
	canonical := make([]*transaction.Transaction, len(expected))
	found := 0
	for i, tx := range expected {
		persisted, err := s.repo.GetBySourceEventID(ctx, tenantID, tx.SourceEventID)
		if err != nil {
			return nil, false, fmt.Errorf("batch preflight lookup for item %d: %w", i, err)
		}
		canonical[i] = persisted
		if persisted != nil {
			found++
		}
	}

	extraSourceID := events.DeriveNotificationID(operationID, "batch_line", strconv.Itoa(len(expected)))
	extra, err := s.repo.GetBySourceEventID(ctx, tenantID, extraSourceID)
	if err != nil {
		return nil, false, fmt.Errorf("batch preflight lookup for extra item: %w", err)
	}
	if extra != nil {
		return nil, false, fmt.Errorf("%w: posting has more transaction lines than the retried command", transaction.ErrBatchIdempotencyConflict)
	}
	if found == 0 {
		return nil, false, nil
	}
	if found != len(expected) {
		return nil, false, fmt.Errorf("%w: posting has a partial or different transaction set", transaction.ErrBatchIdempotencyConflict)
	}

	outputs := make([]BatchCreateOutput, len(expected))
	for i := range expected {
		if !sameBatchTransaction(canonical[i], expected[i]) || !sameBatchManifest(canonical[i], expected[i]) {
			return nil, false, fmt.Errorf("%w: transaction item %d", transaction.ErrBatchIdempotencyConflict, i)
		}
		outputs[i] = BatchCreateOutput{Transaction: canonical[i], Index: i}
	}
	return outputs, true, nil
}

func sameBatchManifest(canonical, expected *transaction.Transaction) bool {
	if canonical == nil || expected == nil {
		return false
	}
	// Rows from the original batch endpoint predate command manifests. Exact
	// transaction-only retries remain compatible; they may not be augmented
	// with newly introduced Commission side effects under the old key.
	if canonical.BatchFingerprint == "" {
		return expected.CommissionCount == 0
	}
	return canonical.BatchFingerprint == expected.BatchFingerprint &&
		canonical.BatchLineCount == expected.BatchLineCount &&
		canonical.CommissionCount == expected.CommissionCount
}

func manualPostingFingerprint(txs []*transaction.Transaction, commissions []ManualCommissionInput, retention *ManualCommissionRetentionInput) string {
	h := sha256.New()
	version := "v1"
	if retention != nil {
		version = "v2"
	}
	for _, tx := range txs {
		if tx != nil && tx.EconomicBreakdown != nil {
			version = "v3"
			break
		}
	}
	fmt.Fprintf(h, "manual-posting:%s\nlines=%d\ncommissions=%d\n", version, len(txs), len(commissions))
	for i, tx := range txs {
		fmt.Fprintf(h, "tx[%d]=contract:%q,type:%q,description:%q,cents:%d,date:%q,category:%q,payment:%q\n",
			i,
			tx.ContractID,
			tx.Type,
			tx.Description,
			int64(math.Round(tx.Value*100)),
			tx.Date.UTC().Format(time.RFC3339Nano),
			tx.Category,
			tx.PaymentMethod,
		)
		// Proveniência condicional: postings antigos (sem due_date/plan) retêm
		// o fingerprint legado, então um retry pré-deploy continua idempotente.
		// Com proveniência presente, o hash diverge — retry com plano/vencimento
		// diferente é detectado como conflito.
		if tx.DueDate != nil || tx.PaymentPlan != "" {
			dueDate := ""
			if tx.DueDate != nil {
				dueDate = tx.DueDate.UTC().Format(time.RFC3339Nano)
			}
			fmt.Fprintf(h, "tx[%d].provenance=due:%q,plan:%q\n", i, dueDate, tx.PaymentPlan)
		}
		fmt.Fprintf(h, "tx[%d].commission-dimensions=broker:%q,role:%q\n", i, tx.BrokerID, tx.Role)
		if tx.EconomicBreakdown != nil {
			b := tx.EconomicBreakdown
			fmt.Fprintf(h, "tx[%d].economic=deal:%q,flow:%q,gross:%d,pool:%d,commissions:%d,partners:%d,agency:%d,owner:%d,discount:%d,admin:%d\n",
				i, tx.DealID, b.Flow, b.GrossCents, b.CommissionPoolCents, b.CommissionPayableCents,
				b.PartnerPayableCents, b.AgencyRetainedCents, b.OwnerPayableCents, b.DiscountCents, b.AdminFeeCents,
			)
		}
	}
	for i, item := range commissions {
		fmt.Fprintf(h, "commission[%d]=deal:%q,contract:%q,broker:%q,role:%q,percentage-1e6:%d,cents:%d\n",
			i,
			strings.TrimSpace(item.DealID),
			strings.TrimSpace(item.ContractID),
			strings.TrimSpace(item.BrokerID),
			strings.ToLower(strings.TrimSpace(item.Role)),
			int64(math.Round(item.Percentage*1_000_000)),
			int64(math.Round(item.Amount*100)),
		)
	}
	if retention != nil {
		fmt.Fprintf(h, "commission-retention=percentage-1e6:%d,cents:%d\n",
			int64(math.Round(retention.Percentage*1_000_000)),
			int64(math.Round(retention.Amount*100)),
		)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sameBatchTransaction(canonical, expected *transaction.Transaction) bool {
	if canonical == nil || expected == nil {
		return false
	}
	// Batch rows created before operation_id/posting_id became explicit still
	// have the same deterministic primary key and source_event_id. Accept that
	// legacy shape for an identical retry, but never accept a different posting
	// or any changed economic payload under the same idempotency key.
	postingMatches := canonical.PostingID == expected.PostingID ||
		(canonical.PostingID == "" && canonical.ID == expected.ID)
	operationMatches := canonical.OperationID == expected.OperationID || canonical.OperationID == ""
	return canonical.ID == expected.ID &&
		postingMatches &&
		operationMatches &&
		canonical.SourceEventID == expected.SourceEventID &&
		canonical.TenantID == expected.TenantID &&
		canonical.ContractID == expected.ContractID &&
		canonical.Type == expected.Type &&
		canonical.Description == expected.Description &&
		canonical.Category == expected.Category &&
		canonical.PaymentMethod == expected.PaymentMethod &&
		canonical.BrokerID == expected.BrokerID &&
		canonical.PaymentPlan == expected.PaymentPlan &&
		sameOptionalDate(canonical.DueDate, expected.DueDate) &&
		strings.EqualFold(strings.TrimSpace(canonical.Role), strings.TrimSpace(expected.Role)) &&
		canonical.DealID == expected.DealID &&
		sameTransactionEconomicBreakdown(canonical.EconomicBreakdown, expected.EconomicBreakdown) &&
		canonical.Date.Equal(expected.Date) &&
		math.Abs(canonical.Value-expected.Value) <= 0.005
}

func sameTransactionEconomicBreakdown(a, b *transaction.EconomicBreakdown) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameOptionalDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func hasCentPrecision(value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	scaled := value * 100
	return math.Abs(scaled-math.Round(scaled)) <= 1e-7
}

func parseBatchTransactionDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339")
}

func (s *TransactionService) GetByID(ctx context.Context, tenantID, id string) (*transaction.Transaction, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, transaction.ErrTransactionNotFound
	}
	return t, nil
}

// GetBySourceEventID exposes the repository lookup to orchestration code that
// must bridge deterministic identifiers across releases. A missing row is a
// normal nil result.
func (s *TransactionService) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	if strings.TrimSpace(sourceEventID) == "" {
		return nil, nil
	}
	return s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
}

func (s *TransactionService) Update(ctx context.Context, t *transaction.Transaction) error {
	if t == nil {
		return fmt.Errorf("transaction is required")
	}
	existing, err := s.repo.GetByID(ctx, t.TenantID, t.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return transaction.ErrTransactionNotFound
	}
	// Source/batch rows are immutable ledger facts. Generic PUT cannot rewrite
	// their amount, dimensions, command manifest, or lifecycle state; settle,
	// cancel and reverse are the only supported corrections.
	if strings.TrimSpace(existing.SourceEventID) != "" ||
		strings.TrimSpace(existing.OperationID) != "" ||
		strings.TrimSpace(existing.BatchFingerprint) != "" {
		return fmt.Errorf("%w: source-linked transactions cannot be edited; use a lifecycle operation", transaction.ErrInvalidStatusTransition)
	}
	if err := transaction.ValidateStatusTransition(existing.Status, t.Status); err != nil {
		return err
	}
	// Optimistic locking: prevents lost-update when two concurrent
	// requests update the same transaction.
	if !t.UpdatedAt.IsZero() && !existing.UpdatedAt.Equal(t.UpdatedAt) {
		return fmt.Errorf("%w; please refresh and retry", transaction.ErrConcurrentModification)
	}
	prev := audit.Snapshot(existing)
	t.TenantID = existing.TenantID
	t.ContractID = existing.ContractID
	t.CreatedAt = existing.CreatedAt
	if err := t.Validate(); err != nil {
		return err
	}
	if err := s.updateIfCurrent(ctx, t, existing.UpdatedAt); err != nil {
		return err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "transaction.updated",
		EntityType: "Transaction",
		EntityID:   t.ID,
		Before:     prev,
		After:      audit.Snapshot(t),
	})
	return nil
}

// Settle records a manual baixa on a transaction: marks it Pago with the given
// payment date/method. Mirrors CommissionService.MarkPaidWithProof.
// Settle records a manual baixa (pagamento) WITHOUT proof evidence —
// compatibility wrapper kept for callers that do not attach a document.
func (s *TransactionService) Settle(ctx context.Context, id, tenantID string, paymentDate *time.Time, method string) (*transaction.Transaction, error) {
	return s.SettleWithProof(ctx, id, tenantID, paymentDate, method, "", "")
}

// SettleWithProof records a manual baixa (pagamento) on a transaction:
// transitions it to Pago with the given payment date/method and, whenever a
// comprovante/observação é anexado, persiste a evidência no lançamento.
// A baixa de entradas/saídas/comissões/repasse gerados manualmente registra o
// movimento no caixa (journal de dupla entrada, idempotente) — o plano de
// contas e a posição de caixa passam a refletir o pagamento.
func (s *TransactionService) SettleWithProof(ctx context.Context, id, tenantID string, paymentDate *time.Time, method, proofURL, note string) (*transaction.Transaction, error) {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		var settled *transaction.Transaction
		err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			var err error
			settled, err = s.settleAndSync(database.WithTx(ctx, tx), id, tenantID, paymentDate, method, proofURL, note)
			return err
		})
		return settled, err
	}
	return s.settleAndSync(ctx, id, tenantID, paymentDate, method, proofURL, note)
}

func (s *TransactionService) settleAndSync(ctx context.Context, id, tenantID string, paymentDate *time.Time, method, proofURL, note string) (*transaction.Transaction, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, transaction.ErrTransactionNotFound
	}
	if tenantID != "" && t.TenantID != tenantID {
		return nil, transaction.ErrTransactionNotFound
	}
	if err := s.requireManualLedgerLifecycle(ctx, t); err != nil {
		return nil, err
	}
	manualReceipt := isDistributedManualReceipt(t)
	if manualReceipt {
		if err := s.ensureManualReceiptBreakdown(ctx, t); err != nil {
			return nil, err
		}
		if t.EconomicBreakdown != nil && isManualReceiptFlow(t.EconomicBreakdown.Flow) && s.manualReceiptSettlement == nil {
			return nil, errors.New("manual sale receipt settlement is not configured")
		}
	}
	linked, err := s.manualCommissionForLedger(ctx, t)
	if err != nil {
		return nil, err
	}
	// A retried linked lifecycle is idempotent. If an older non-atomic writer
	// left only the Transaction paid, the same command completes the Commission
	// side now instead of rejecting the repair. Lançamentos avulsos já pagos
	// aceitam retry somente para anexar comprovante/observação (sem duplicar
	// o journal de caixa).
	if transaction.CanonicalStatus(t.Status) == transaction.StatusPaid &&
		(linked != nil || manualReceipt || strings.TrimSpace(proofURL) != "" || strings.TrimSpace(note) != "") {
		if err := s.attachProofIfProvided(ctx, t, proofURL, note); err != nil {
			return nil, err
		}
		if linked != nil && linked.Status != commission.StatusPaid {
			if _, err := s.manualCommissionService.markPaidFromLedger(ctx, linked, t.PaymentDate); err != nil {
				return nil, fmt.Errorf("mark linked commission %s paid: %w", linked.ID, err)
			}
		}
		if linked != nil {
			if err := s.postManualCommissionSettlementJournal(ctx, t, linked); err != nil {
				return nil, err
			}
		}
		if manualReceipt && t.EconomicBreakdown != nil && isManualReceiptFlow(t.EconomicBreakdown.Flow) {
			if err := s.manualReceiptSettlement.EnsureForSettledReceipt(ctx, t); err != nil {
				return nil, fmt.Errorf("release owner transfer from manual receipt: %w", err)
			}
		}
		return t, nil
	}
	prev := t.Status
	expectedUpdatedAt := t.UpdatedAt
	pd := time.Time{}
	if paymentDate != nil {
		pd = *paymentDate
	}
	if err := t.MarkPaid(pd, method); err != nil {
		return nil, err
	}
	// Evidência do pagamento (comprovante/observação) persiste junto com a
	// baixa — o recibo e o audit trail passam a exibi-la como nos demais fluxos.
	t.ProofURL = strings.TrimSpace(proofURL)
	t.Note = strings.TrimSpace(note)
	// Funding updates the frozen receipt composition. Persist that same
	// composition with the paid status so the journal, transfer and receipt
	// continue to agree after reload and on an idempotent retry.
	if manualReceipt && t.EconomicBreakdown != nil && isManualReceiptFlow(t.EconomicBreakdown.Flow) {
		if err := s.manualReceiptSettlement.EnsureForSettledReceipt(ctx, t); err != nil {
			return nil, fmt.Errorf("release owner transfer from manual receipt: %w", err)
		}
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "settle",
		EntityType: "Transaction",
		EntityID:   t.ID,
		Before:     map[string]any{"status": prev},
		After: map[string]any{
			"status": t.Status, "payment_date": t.PaymentDate,
			"payment_method": t.PaymentMethod, "proof_url": t.ProofURL, "note": t.Note,
		},
		Metadata: map[string]any{"tenant_id": t.TenantID},
	})
	// Lançamento manual sem fluxo econômico próprio (entrada/saída avulsa,
	// repasse/comissão legada): a baixa precisa mover o caixa no ledger para
	// o plano de contas/posição refletirem o pagamento.
	if err := s.postPlainCashJournal(ctx, t); err != nil {
		return nil, fmt.Errorf("post plain cash journal: %w", err)
	}
	if linked != nil {
		if _, err := s.manualCommissionService.markPaidFromLedger(ctx, linked, t.PaymentDate); err != nil {
			return nil, fmt.Errorf("mark linked commission %s paid: %w", linked.ID, err)
		}
		if err := s.postManualCommissionSettlementJournal(ctx, t, linked); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// attachProofIfProvided persiste comprovante/observação num lançamento já
// pago (retry idempotente): o operador pode anexar o documento depois da
// baixa sem reverter o pagamento.
func (s *TransactionService) attachProofIfProvided(ctx context.Context, t *transaction.Transaction, proofURL, note string) error {
	if t == nil || (strings.TrimSpace(proofURL) == "" && strings.TrimSpace(note) == "") {
		return nil
	}
	if t.ProofURL == strings.TrimSpace(proofURL) && t.Note == strings.TrimSpace(note) {
		return nil
	}
	expected := t.UpdatedAt
	t.ProofURL = strings.TrimSpace(proofURL)
	t.Note = strings.TrimSpace(note)
	if err := s.updateIfCurrent(ctx, t, expected); err != nil {
		return fmt.Errorf("attach settlement proof: %w", err)
	}
	audit.Record(ctx, audit.Entry{
		Action:     "settle.proof",
		EntityType: "Transaction",
		EntityID:   t.ID,
		After:      map[string]any{"proof_url": t.ProofURL, "note": t.Note},
		Metadata:   map[string]any{"tenant_id": t.TenantID},
	})
	return nil
}

// isManualReceiptFlow reports whether an economic breakdown flow belongs to a
// distributed manual receipt (sale or rent) whose settlement releases an owner
// transfer.
func isManualReceiptFlow(flow string) bool {
	return flow == "sale" || flow == "rent"
}

func isDistributedManualReceipt(t *transaction.Transaction) bool {
	if t == nil || t.Type != transaction.TypeReceita || strings.TrimSpace(t.BatchFingerprint) == "" {
		return false
	}
	return t.CommissionCount > 0 || (t.EconomicBreakdown != nil && isManualReceiptFlow(t.EconomicBreakdown.Flow))
}

func (s *TransactionService) ensureManualReceiptBreakdown(ctx context.Context, receipt *transaction.Transaction) error {
	if receipt == nil || !isDistributedManualReceipt(receipt) {
		return nil
	}
	if receipt.EconomicBreakdown != nil {
		return validateManualReceiptBreakdown(receipt)
	}
	operationRepo, ok := s.repo.(transactionOperationRepository)
	if !ok {
		return errors.New("transaction repository cannot resolve the manual posting operation")
	}
	if s.manualCommissionService == nil {
		return errors.New("manual commission persistence is not configured")
	}
	lines, err := operationRepo.ListByOperationID(ctx, receipt.TenantID, receipt.OperationID)
	if err != nil {
		return fmt.Errorf("load manual posting operation: %w", err)
	}
	var brokerCents int64
	var percentageTotal float64
	dealID := ""
	commissionCount := 0
	for _, line := range lines {
		if line == nil || line.Type != transaction.TypeComissao {
			continue
		}
		linked, err := s.manualCommissionForLedger(ctx, line)
		if err != nil {
			return err
		}
		if linked == nil {
			continue
		}
		commissionCount++
		brokerCents += int64(math.Round(linked.Amount * 100))
		percentageTotal += linked.Percentage
		if dealID == "" {
			dealID = linked.DealID
		} else if dealID != linked.DealID {
			return errors.New("manual receipt commissions span multiple deals")
		}
	}
	if commissionCount != receipt.CommissionCount || commissionCount == 0 {
		return fmt.Errorf("manual receipt resolves %d commission obligations, expected %d", commissionCount, receipt.CommissionCount)
	}
	desc := strings.ToLower(receipt.Description)
	flow := ""
	if strings.Contains(desc, "venda") {
		flow = "sale"
	} else if strings.Contains(desc, "locação") || strings.Contains(desc, "locacao") || strings.Contains(desc, "aluguel") {
		flow = "rent"
	}
	if flow != "sale" && flow != "rent" {
		return nil
	}
	if percentageTotal <= 0 || percentageTotal > 100.01 {
		return fmt.Errorf("manual receipt commission percentages total %.4f%%", percentageTotal)
	}
	commissionPoolCents := int64(math.Round(float64(brokerCents) * 100 / percentageTotal))
	grossCents := int64(math.Round(receipt.Value * 100))
	adminFeeCents := int64(0)
	if flow == "rent" {
		adminFeePct, feeErr := s.resolveManualReceiptAdminFeePct(ctx, receipt.TenantID)
		if feeErr != nil {
			return feeErr
		}
		adminFeeCents = int64(math.Round(float64(grossCents) * adminFeePct / 100))
	}
	ownerCents := grossCents - commissionPoolCents - adminFeeCents
	if ownerCents < 0 {
		return errors.New("manual receipt commission pool and fees exceed gross receipt")
	}
	receipt.DealID = dealID
	receipt.EconomicBreakdown = &transaction.EconomicBreakdown{
		Flow:                   flow,
		GrossCents:             grossCents,
		CommissionPoolCents:    commissionPoolCents,
		CommissionPayableCents: brokerCents,
		AgencyRetainedCents:    commissionPoolCents - brokerCents,
		OwnerPayableCents:      ownerCents,
		AdminFeeCents:          adminFeeCents,
	}
	return validateManualReceiptBreakdown(receipt)
}

func validateManualReceiptBreakdown(receipt *transaction.Transaction) error {
	if receipt == nil || receipt.EconomicBreakdown == nil {
		return nil
	}
	b := receipt.EconomicBreakdown
	if (b.Flow != "sale" && b.Flow != "rent") || strings.TrimSpace(receipt.DealID) == "" || b.GrossCents <= 0 ||
		b.CommissionPoolCents < 0 || b.CommissionPayableCents < 0 || b.PartnerPayableCents < 0 ||
		b.AgencyRetainedCents < 0 || b.OwnerPayableCents < 0 || b.DiscountCents < 0 || b.AdminFeeCents < 0 {
		return errors.New("manual receipt has an invalid economic breakdown")
	}
	if b.CommissionFundedCents < 0 || b.CommissionFundedCents > b.CommissionPayableCents ||
		(!b.CommissionFundingKnown && b.CommissionFundedCents != 0) {
		return errors.New("manual receipt has invalid commission funding")
	}
	if b.CommissionPoolCents != b.CommissionPayableCents+b.PartnerPayableCents+b.AgencyRetainedCents ||
		b.GrossCents-b.DiscountCents != b.OwnerPayableCents+b.CommissionPoolCents+b.AdminFeeCents {
		return errors.New("manual receipt economic breakdown does not close")
	}
	return nil
}

func (s *TransactionService) Delete(ctx context.Context, id, tenantID string) error {
	tx, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if tx == nil {
		return transaction.ErrTransactionNotFound
	}

	// Event/batch rows are part of an immutable financial trail. Deleting one
	// would orphan its Charge/Commission/Payout counterpart and also makes a
	// deterministic retry collide with the soft-deleted primary key. Correct
	// them through reversal instead. Only unsourced manual drafts may be removed.
	if strings.TrimSpace(tx.SourceEventID) != "" {
		return fmt.Errorf("%w: source-linked transactions must be reversed, not deleted", transaction.ErrInvalidStatusTransition)
	}
	if status := transaction.CanonicalStatus(tx.Status); status == transaction.StatusPaid || status == transaction.StatusReversed {
		return fmt.Errorf("%w: settled transactions must be reversed, not deleted", transaction.ErrInvalidStatusTransition)
	}

	if err := s.repo.Delete(ctx, id, tenantID); err != nil {
		return err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "transaction.deleted",
		EntityType: "Transaction",
		EntityID:   id,
		Before:     audit.Snapshot(tx),
	})
	return nil
}

// Reverse creates one idempotent reversal transaction for a paid posting. A
// deterministic source_event_id collapses concurrent/retried reversal calls,
// while the original remains in the terminal REVERSED state.
func (s *TransactionService) Reverse(ctx context.Context, id, tenantID, reason string, operatorID string) (*transaction.Transaction, error) {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		var reversed *transaction.Transaction
		err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			var err error
			reversed, err = s.reverseAndSync(database.WithTx(ctx, tx), id, tenantID, reason, operatorID)
			return err
		})
		return reversed, err
	}
	return s.reverseAndSync(ctx, id, tenantID, reason, operatorID)
}

func (s *TransactionService) reverseAndSync(ctx context.Context, id, tenantID, reason string, operatorID string) (*transaction.Transaction, error) {
	original, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if original == nil || original.TenantID != tenantID {
		return nil, transaction.ErrTransactionNotFound
	}
	distributed := isDistributedManualReceipt(original) && original.EconomicBreakdown != nil && isManualReceiptFlow(original.EconomicBreakdown.Flow)
	if err := s.requireManualLedgerLifecycle(ctx, original); err != nil {
		return nil, err
	}

	reversed, err := s.reverse(ctx, id, tenantID, reason, operatorID)
	if err != nil {
		return nil, err
	}
	linked, err := s.manualCommissionForLedger(ctx, original)
	if err != nil {
		return nil, err
	}
	if linked != nil {
		if _, err := s.manualCommissionService.reverseFromLedger(ctx, linked, reason); err != nil {
			return nil, fmt.Errorf("reverse linked commission %s: %w", linked.ID, err)
		}
	}
	if distributed {
		if err := s.reverseDistributedReceipt(ctx, original, reason, operatorID); err != nil {
			return nil, fmt.Errorf("reverse distributed receipt obligations: %w", err)
		}
	} else {
		// Lançamento avulso (entrada/saída/repasse/comissão legada): estorna o
		// journal de caixa criado na baixa.
		if err := s.reversePlainCashJournal(ctx, original, operatorID, reason); err != nil {
			return nil, fmt.Errorf("reverse plain cash journal: %w", err)
		}
	}
	return reversed, nil
}

// reverseDistributedReceipt desfaz as obrigações liberadas por um recebimento
// manual distribuído (comissões vinculadas, repasse ao proprietário e journal
// de liquidação). Repasse já PROCESSADO (caixa movido) falha fechado.
func (s *TransactionService) reverseDistributedReceipt(ctx context.Context, receipt *transaction.Transaction, reason, operatorID string) error {
	if s.manualCommissionService != nil && receipt.OperationID != "" {
		if err := s.reverseReceiptCommissions(ctx, receipt, reason, operatorID); err != nil {
			return err
		}
	}
	if s.manualReceiptSettlement != nil {
		if err := s.manualReceiptSettlement.CancelForReceipt(ctx, receipt); err != nil {
			return err
		}
	}
	if s.journalService != nil {
		entry, err := s.journalService.GetBySourceEventID(ctx, receipt.TenantID, manualReceiptJournalSourceID(receipt))
		if err != nil {
			return err
		}
		if entry != nil {
			if _, err := s.journalService.Reverse(ctx, entry.ID, receipt.TenantID, "Recebimento revertido: "+reason, operatorID); err != nil {
				return fmt.Errorf("reverse receipt settlement journal: %w", err)
			}
		}
	}
	return nil
}

func (s *TransactionService) reverseReceiptCommissions(ctx context.Context, receipt *transaction.Transaction, reason, operatorID string) error {
	operationRepo, ok := s.repo.(transactionOperationRepository)
	if !ok {
		return errors.New("transaction repository cannot resolve the manual posting operation")
	}
	lines, err := operationRepo.ListByOperationID(ctx, receipt.TenantID, receipt.OperationID)
	if err != nil {
		return fmt.Errorf("load manual posting operation: %w", err)
	}
	for _, line := range lines {
		if line == nil || line.Type != transaction.TypeComissao {
			continue
		}
		linked, err := s.manualCommissionForLedger(ctx, line)
		if err != nil {
			return err
		}
		if linked == nil {
			continue
		}
		switch linked.Status {
		case commission.StatusPaid:
			if _, err := s.manualCommissionService.reverseFromLedger(ctx, linked, reason); err != nil {
				return fmt.Errorf("reverse linked commission %s: %w", linked.ID, err)
			}
		case commission.StatusPending, commission.StatusApproved:
			if _, err := s.manualCommissionService.Cancel(ctx, linked.TenantID, linked.ID, reason); err != nil {
				return fmt.Errorf("cancel linked commission %s: %w", linked.ID, err)
			}
		default:
			return fmt.Errorf("linked commission %s is in state %s; cannot reverse receipt", linked.ID, linked.Status)
		}
	}
	return nil
}

// plainCashJournalCovered reports whether a transaction's settlement already
// produces a ledger journal (distributed manual receipt or linked commission)
// — in those cases the plain cash posting must NOT be duplicated.
func plainCashJournalCovered(t *transaction.Transaction) bool {
	if t == nil {
		return true
	}
	// Estorno é o reverso de outro lançamento: o journal da baixa original já
	// foi estornado pelo reverse do respectivo fluxo.
	if t.Type == transaction.TypeEstorno {
		return true
	}
	if t.Type == transaction.TypeReceita && isDistributedManualReceipt(t) &&
		t.EconomicBreakdown != nil && isManualReceiptFlow(t.EconomicBreakdown.Flow) {
		return true
	}
	if isManualCommissionLedger(t) {
		return true
	}
	return false
}

// postPlainCashJournal registra no caixa (journal de dupla entrada) a baixa de
// lançamentos manuais sem fluxo econômico próprio:
//   - entrada  → cash D / revenue C
//   - saída    → expense D / cash C
//   - repasse  → owner_payable D / cash C
//   - comissão avulsa legada → acrual (commission_expense D / commission_payable
//     C) + settlement (commission_payable D / cash C)
//
// Idempotente por source_event_id determinístico. Sem esse passo, a baixa
// marcava o lançamento como pago mas o caixa do plano de contas não se movia,
// e o fluxo de comissões/repasse não tinha o caixa para cobrir.
func (s *TransactionService) postPlainCashJournal(ctx context.Context, t *transaction.Transaction) error {
	if s.journalService == nil || t == nil || plainCashJournalCovered(t) {
		return nil
	}
	if transaction.CanonicalStatus(t.Status) != transaction.StatusPaid {
		return nil
	}
	if t.Type == transaction.TypeComissao {
		accrual, err := newPlainCommissionAccrualEntry(t)
		if err != nil {
			return err
		}
		accrualSourceID := events.DeriveNotificationID("transaction.commission.accrual", t.TenantID, t.ID)
		if err := s.journalService.CreateWithSourceEventID(ctx, accrualSourceID, accrual); err != nil {
			return err
		}
	}
	entry, err := newPlainCashJournalEntry(t)
	if err != nil {
		return err
	}
	sourceID := events.DeriveNotificationID("transaction.cash.journal", t.TenantID, t.ID)
	return s.journalService.CreateWithSourceEventID(ctx, sourceID, entry)
}

func newPlainCashJournalEntry(t *transaction.Transaction) (*journal.Entry, error) {
	cents := int64(math.Round(t.Value * 100))
	if cents <= 0 {
		return nil, fmt.Errorf("transaction %s has non-positive value", t.ID)
	}
	flow := ""
	if t.EconomicBreakdown != nil {
		flow = t.EconomicBreakdown.Flow
	}
	date := t.Date
	if t.PaymentDate != nil {
		date = *t.PaymentDate
	}
	// Valor negativo é nota de crédito: inverte as pernas em vez de gravar
	// débito/crédito negativos. Sem isso o estorno de uma receita avulsa não
	// teria representação contábil (o TypeEstorno só reverte transação paga).
	flip := cents < 0
	if flip {
		cents = -cents
	}
	var legs []journal.Leg
	switch t.Type {
	case transaction.TypeReceita:
		legs = []journal.Leg{
			{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: cents, Flow: flow},
			{Account: journal.AccountRevenue, Direction: journal.Credit, AmountCents: cents, Flow: flow},
		}
	case transaction.TypeDespesa:
		legs = []journal.Leg{
			{Account: journal.AccountExpense, Direction: journal.Debit, AmountCents: cents, Flow: flow},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: cents, Flow: flow},
		}
	case transaction.TypeRepasse:
		legs = []journal.Leg{
			{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: cents, Flow: flow},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: cents, Flow: flow},
		}
	case transaction.TypeComissao:
		legs = []journal.Leg{
			{Account: journal.AccountCommissionPayable, Direction: journal.Debit, AmountCents: cents, Flow: flow},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: cents, Flow: flow},
		}
	case transaction.TypeEstorno:
		// Nota de crédito avulsa: liquida a obrigação com o terceiro que
		// recebeu a mais. Reversões de transação paga têm caminho próprio.
		legs = []journal.Leg{
			{Account: journal.AccountOtherPayable, Direction: journal.Debit, AmountCents: cents, Flow: flow},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: cents, Flow: flow},
		}
	default:
		return nil, fmt.Errorf("transaction %s has no plain cash journal mapping for type %s", t.ID, t.Type)
	}
	if flip {
		for i := range legs {
			if legs[i].Direction == journal.Debit {
				legs[i].Direction = journal.Credit
			} else {
				legs[i].Direction = journal.Debit
			}
		}
	}
	return journal.New(t.TenantID, t.ContractID, fmt.Sprintf("Baixa - %s", t.Description), date, legs)
}

func newPlainCommissionAccrualEntry(t *transaction.Transaction) (*journal.Entry, error) {
	cents := int64(math.Round(t.Value * 100))
	if cents <= 0 {
		return nil, fmt.Errorf("transaction %s has non-positive value", t.ID)
	}
	flow := ""
	if t.EconomicBreakdown != nil {
		flow = t.EconomicBreakdown.Flow
	}
	date := t.Date
	if t.PaymentDate != nil {
		date = *t.PaymentDate
	}
	return journal.New(t.TenantID, t.ContractID, fmt.Sprintf("Comissão acruada - %s", t.Description), date, []journal.Leg{
		{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: cents, Flow: flow},
		{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: cents, Flow: flow},
	})
}

// reversePlainCashJournal estorna (reversão) o journal de caixa de um
// lançamento manual sem fluxo econômico próprio — espelho do
// reverseDistributedReceipt para entradas/saídas/repasse/comissão avulsa.
func (s *TransactionService) reversePlainCashJournal(ctx context.Context, original *transaction.Transaction, operatorID, reason string) error {
	if s.journalService == nil || original == nil || plainCashJournalCovered(original) {
		return nil
	}
	if original.Type == transaction.TypeComissao {
		if err := s.reverseJournalBySource(ctx, original, "transaction.commission.accrual", operatorID, reason); err != nil {
			return err
		}
	}
	return s.reverseJournalBySource(ctx, original, "transaction.cash.journal", operatorID, reason)
}

func (s *TransactionService) reverseJournalBySource(ctx context.Context, original *transaction.Transaction, sourcePrefix, operatorID, reason string) error {
	sourceID := events.DeriveNotificationID(sourcePrefix, original.TenantID, original.ID)
	entry, err := s.journalService.GetBySourceEventID(ctx, original.TenantID, sourceID)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	_, err = s.journalService.Reverse(ctx, entry.ID, original.TenantID, "Lançamento revertido: "+reason, operatorID)
	return err
}

// Cancel cancela um lançamento manual ainda não pago (PENDING/OVERDUE). Para um
// recebimento distribuído, também cancela as comissões vinculadas pendentes.
// Um lançamento pago exige reversão (estorno), não cancelamento.
func (s *TransactionService) Cancel(ctx context.Context, id, tenantID, reason string) (*transaction.Transaction, error) {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		var cancelled *transaction.Transaction
		err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			var err error
			cancelled, err = s.cancelAndCascade(database.WithTx(ctx, tx), id, tenantID, reason)
			return err
		})
		return cancelled, err
	}
	return s.cancelAndCascade(ctx, id, tenantID, reason)
}

func (s *TransactionService) cancelAndCascade(ctx context.Context, id, tenantID, reason string) (*transaction.Transaction, error) {
	t, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if t == nil || t.TenantID != tenantID {
		return nil, transaction.ErrTransactionNotFound
	}
	switch transaction.CanonicalStatus(t.Status) {
	case transaction.StatusCancelled:
		return t, nil
	case transaction.StatusPending, transaction.StatusOverdue:
	default:
		return nil, fmt.Errorf("%w: only PENDING or OVERDUE transactions can be cancelled (current: %s)", transaction.ErrInvalidStatusTransition, t.Status)
	}
	if isDistributedManualReceipt(t) {
		if err := s.cancelReceiptCommissions(ctx, t, reason); err != nil {
			return nil, err
		}
	}
	expectedUpdatedAt := t.UpdatedAt
	if err := t.MarkCancelled(); err != nil {
		return nil, err
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransactionService) cancelReceiptCommissions(ctx context.Context, receipt *transaction.Transaction, reason string) error {
	operationRepo, ok := s.repo.(transactionOperationRepository)
	if !ok {
		return errors.New("transaction repository cannot resolve the manual posting operation")
	}
	lines, err := operationRepo.ListByOperationID(ctx, receipt.TenantID, receipt.OperationID)
	if err != nil {
		return fmt.Errorf("load manual posting operation: %w", err)
	}
	for _, line := range lines {
		if line == nil || line.Type != transaction.TypeComissao {
			continue
		}
		linked, err := s.manualCommissionForLedger(ctx, line)
		if err != nil {
			return err
		}
		if linked == nil {
			continue
		}
		if transaction.CanonicalStatus(line.Status) != transaction.StatusPending && transaction.CanonicalStatus(line.Status) != transaction.StatusOverdue {
			return fmt.Errorf("linked commission %s is %s; reverse the receipt instead of cancelling", linked.ID, line.Status)
		}
		if _, err := s.manualCommissionService.Cancel(ctx, linked.TenantID, linked.ID, reason); err != nil {
			return fmt.Errorf("cancel linked commission %s: %w", linked.ID, err)
		}
	}
	return nil
}

func (s *TransactionService) reverse(ctx context.Context, id, tenantID, reason string, operatorID string) (*transaction.Transaction, error) {
	original, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, transaction.ErrTransactionNotFound
	}
	if original.TenantID != tenantID {
		return nil, transaction.ErrTransactionNotFound
	}
	if original.Type == transaction.TypeEstorno {
		return nil, fmt.Errorf("%w: cannot reverse an estorno transaction", transaction.ErrInvalidStatusTransition)
	}

	reversalSourceEventID := events.DeriveNotificationID("transaction.reversal", tenantID, original.ID)
	existingReversal, err := s.repo.GetBySourceEventID(ctx, tenantID, reversalSourceEventID)
	if err != nil {
		return nil, fmt.Errorf("lookup existing reversal: %w", err)
	}
	if existingReversal != nil {
		return existingReversal, nil
	}
	if transaction.CanonicalStatus(original.Status) != transaction.StatusPaid {
		return nil, fmt.Errorf("%w: only PAID transactions can be reversed (current: %s)", transaction.ErrInvalidStatusTransition, original.Status)
	}

	reversed, err := original.Reverse(reason, original.ID)
	if err != nil {
		return nil, err
	}
	reversed.SourceEventID = reversalSourceEventID

	before := audit.Snapshot(original)
	prevStatus := original.Status
	prevUpdatedAt := original.UpdatedAt
	expectedUpdatedAt := original.UpdatedAt
	if err := original.MarkReversed(); err != nil {
		return nil, err
	}
	if err := s.updateIfCurrent(ctx, original, expectedUpdatedAt); err != nil {
		if errors.Is(err, transaction.ErrConcurrentModification) {
			canonical, getErr := s.repo.GetBySourceEventID(ctx, tenantID, reversalSourceEventID)
			if getErr != nil {
				return nil, fmt.Errorf("lookup concurrent reversal: %w", getErr)
			}
			if canonical != nil {
				return canonical, nil
			}
		}
		return nil, fmt.Errorf("failed to mark original as reversed: %w", err)
	}

	if err := s.Create(ctx, reversed); err != nil {
		if errors.Is(err, transaction.ErrTransactionAlreadyExists) {
			canonical, getErr := s.repo.GetBySourceEventID(ctx, tenantID, reversalSourceEventID)
			if getErr != nil {
				return nil, fmt.Errorf("lookup canonical reversal: %w", getErr)
			}
			if canonical != nil {
				return canonical, nil
			}
		}
		// Lightweight fakes/legacy callers may not inject a DB. Preserve their
		// compensating update; production rolls both writes back atomically.
		if s.db == nil && database.TxFromCtx(ctx) == nil {
			original.Status = prevStatus
			original.UpdatedAt = prevUpdatedAt
			_ = s.repo.Update(ctx, original)
		}
		return nil, fmt.Errorf("failed to create reversal transaction: %w", err)
	}

	audit.Record(ctx, audit.Entry{
		Action:     "transaction.reversed",
		EntityType: "Transaction",
		EntityID:   original.ID,
		Before:     before,
		After:      map[string]any{"status": transaction.StatusReversed, "reversed_by": reversed.ID},
		Metadata: map[string]any{
			"reason":      reason,
			"operator_id": operatorID,
			"tenant_id":   tenantID,
		},
	})

	return reversed, nil
}

func isManualCommissionLedger(t *transaction.Transaction) bool {
	return t != nil &&
		t.Type == transaction.TypeComissao &&
		t.CommissionCount > 0 &&
		strings.TrimSpace(t.BatchFingerprint) != ""
}

func (s *TransactionService) requireManualLedgerLifecycle(ctx context.Context, t *transaction.Transaction) error {
	if !isManualCommissionLedger(t) {
		return nil
	}
	if s.manualCommissionService == nil {
		return errors.New("linked manual commission lifecycle is not configured")
	}
	if s.db == nil && database.TxFromCtx(ctx) == nil {
		return errors.New("linked manual commission lifecycle requires a database transaction")
	}
	return nil
}

func (s *TransactionService) manualCommissionForLedger(ctx context.Context, t *transaction.Transaction) (*commission.Commission, error) {
	if t == nil || t.Type != transaction.TypeComissao {
		return nil, nil
	}
	if s.manualCommissionService == nil {
		if isManualCommissionLedger(t) {
			return nil, errors.New("linked manual commission lifecycle is not configured")
		}
		return nil, nil
	}
	linked, err := s.manualCommissionService.commissionByLedgerTransactionID(ctx, t.TenantID, t.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve commission for ledger transaction %s: %w", t.ID, err)
	}
	if linked == nil && isManualCommissionLedger(t) {
		return nil, fmt.Errorf("linked commission for ledger transaction %s not found", t.ID)
	}
	if linked != nil {
		if err := validateManualCommissionLedgerPair(t, linked); err != nil {
			return nil, err
		}
	}
	return linked, nil
}

func validateManualCommissionLedgerPair(t *transaction.Transaction, c *commission.Commission) error {
	if t == nil || c == nil {
		return errors.New("manual commission ledger pair is incomplete")
	}
	if t.TenantID != c.TenantID || t.ID != c.LedgerTransactionID || t.Type != transaction.TypeComissao ||
		t.ContractID != c.ContractID || t.BrokerID != c.BrokerID ||
		!strings.EqualFold(strings.TrimSpace(t.Role), strings.TrimSpace(c.Role)) ||
		math.Abs(t.Value-c.Amount) > 0.005 {
		return fmt.Errorf("manual commission %s diverges from ledger transaction %s", c.ID, t.ID)
	}
	return nil
}

func manualCommissionPaymentJournalSourceID(c *commission.Commission) string {
	if c == nil {
		return ""
	}
	return events.DeriveNotificationID("manual_commission.payment", c.TenantID, c.ID)
}

func (s *TransactionService) manualReceiptForCommission(ctx context.Context, ledger *transaction.Transaction) (*transaction.Transaction, error) {
	if ledger == nil || strings.TrimSpace(ledger.OperationID) == "" {
		return nil, nil
	}
	repo, ok := s.repo.(transactionOperationRepository)
	if !ok {
		return nil, errors.New("transaction repository cannot resolve the manual commission operation")
	}
	lines, err := repo.ListByOperationID(ctx, ledger.TenantID, ledger.OperationID)
	if err != nil {
		return nil, fmt.Errorf("load manual commission operation: %w", err)
	}
	var receipt *transaction.Transaction
	for _, line := range lines {
		if line == nil || line.Type != transaction.TypeReceita || line.EconomicBreakdown == nil || !isManualReceiptFlow(line.EconomicBreakdown.Flow) {
			continue
		}
		if receipt != nil && receipt.ID != line.ID {
			return nil, errors.New("manual commission operation has multiple receipts")
		}
		receipt = line
	}
	if receipt != nil && transaction.CanonicalStatus(receipt.Status) != transaction.StatusPaid {
		return nil, fmt.Errorf("%w: commission cannot be paid before its receipt", transaction.ErrInvalidStatusTransition)
	}
	return receipt, nil
}

func newManualCommissionSettlementJournalEntry(ledger *transaction.Transaction, c *commission.Commission) (*journal.Entry, error) {
	if ledger == nil || c == nil {
		return nil, errors.New("manual commission ledger pair is required")
	}
	grossCents := int64(math.Round(c.GrossAmount * 100))
	if grossCents == 0 {
		grossCents = int64(math.Round(c.Amount * 100))
	}
	if grossCents <= 0 {
		return nil, errors.New("manual commission settlement amount must be positive")
	}
	var taxCents, otherCents, advanceCents int64
	for _, deduction := range c.Deductions {
		amountCents := int64(math.Round(deduction.Amount * 100))
		if amountCents < 0 {
			return nil, errors.New("manual commission deduction cannot be negative")
		}
		switch deduction.Kind {
		case commission.DeductionIRRF, commission.DeductionINSS, commission.DeductionISS:
			taxCents += amountCents
		case commission.DeductionOutros:
			otherCents += amountCents
		case commission.DeductionAdvance:
			// Abatimento de adiantamento: não é despesa nem tributo — é a
			// realização do valor a receber do corretor (ativo que a concessão
			// criou). A perna vai para contas a receber.
			advanceCents += amountCents
		default:
			return nil, errors.New("manual commission has an invalid deduction kind")
		}
	}
	netCents := int64(math.Round(c.NetAmount * 100))
	if c.NetAmount == 0 && taxCents+otherCents == 0 {
		netCents = grossCents
	}
	if netCents < 0 || netCents+taxCents+otherCents+advanceCents != grossCents {
		return nil, errors.New("manual commission settlement does not close in cents")
	}
	// GROSS model: the obligation was accrued as commission_payable at creation
	// (signing or manual batch), so the settlement always clears that liability.
	legs := []journal.Leg{{
		Account: journal.AccountCommissionPayable, Direction: journal.Debit, AmountCents: grossCents, BeneficiaryPartyID: c.BrokerID,
	}}
	if netCents > 0 {
		legs = append(legs, journal.Leg{
			Account: journal.AccountCash, Direction: journal.Credit, AmountCents: netCents, BeneficiaryPartyID: c.BrokerID,
		})
	}
	if taxCents > 0 {
		legs = append(legs, journal.Leg{Account: journal.AccountTaxWithheld, Direction: journal.Credit, AmountCents: taxCents})
	}
	if otherCents > 0 {
		legs = append(legs, journal.Leg{Account: journal.AccountOtherPayable, Direction: journal.Credit, AmountCents: otherCents})
	}
	if advanceCents > 0 {
		legs = append(legs, journal.Leg{Account: journal.AccountAccountsReceivable, Direction: journal.Credit, AmountCents: advanceCents, BeneficiaryPartyID: c.BrokerID})
	}
	paidAt := ledger.Date
	if c.PaidAt != nil && !c.PaidAt.IsZero() {
		paidAt = *c.PaidAt
	} else if ledger.PaymentDate != nil && !ledger.PaymentDate.IsZero() {
		paidAt = *ledger.PaymentDate
	}
	paidAt = paidAt.UTC().Truncate(time.Microsecond)
	entry, err := journal.New(c.TenantID, c.ContractID, "Pagamento da comissão "+c.ID, paidAt, legs)
	if err != nil {
		return nil, err
	}
	entry.OperationID = c.ID
	return entry, nil
}

func (s *TransactionService) postManualCommissionSettlementJournal(ctx context.Context, ledger *transaction.Transaction, c *commission.Commission) error {
	if s.journalService == nil {
		return nil
	}
	receipt, err := s.manualReceiptForCommission(ctx, ledger)
	if err != nil {
		return err
	}
	if receipt != nil {
		sourceID := manualReceiptJournalSourceID(receipt)
		posted, err := s.journalService.GetBySourceEventID(ctx, receipt.TenantID, sourceID)
		if err != nil {
			return err
		}
		if posted == nil || posted.Status != journal.StatusPosted {
			return errors.New("manual receipt journal must exist before commission payment")
		}
	}
	entry, err := newManualCommissionSettlementJournalEntry(ledger, c)
	if err != nil {
		return err
	}
	// Clear the same modality provisioned by a manual posting. Read the
	// original accrual so historical entries without a flow stay consistent.
	if c.SourceEventID != "" {
		accrual, lookupErr := s.journalService.GetBySourceEventID(ctx, c.TenantID, events.DeriveNotificationID(c.SourceEventID, "journal_commission"))
		if lookupErr != nil {
			return lookupErr
		}
		if accrual != nil {
			for _, leg := range accrual.Legs {
				if leg.Account == journal.AccountCommissionPayable && leg.Direction == journal.Credit {
					for i := range entry.Legs {
						entry.Legs[i].Flow = leg.Flow
					}
					break
				}
			}
		}
	}
	if _, err := s.journalService.CreateCanonicalWithSourceEventID(ctx, manualCommissionPaymentJournalSourceID(c), entry); err != nil {
		return fmt.Errorf("post manual commission payment journal: %w", err)
	}
	return nil
}

func (s *TransactionService) reverseManualCommissionSettlementJournal(ctx context.Context, c *commission.Commission, reason string) error {
	if s.journalService == nil {
		return nil
	}
	sourceID := manualCommissionPaymentJournalSourceID(c)
	entry, err := s.journalService.GetBySourceEventID(ctx, c.TenantID, sourceID)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.New("manual commission payment journal is missing")
	}
	if _, err := s.journalService.Reverse(ctx, entry.ID, c.TenantID, reason, "commission:"+c.ID); err != nil {
		return fmt.Errorf("reverse manual commission payment journal: %w", err)
	}
	return nil
}

func (s *TransactionService) transactionForCommission(ctx context.Context, c *commission.Commission) (*transaction.Transaction, error) {
	if c == nil || strings.TrimSpace(c.LedgerTransactionID) == "" {
		return nil, errors.New("commission has no ledger_transaction_id")
	}
	t, err := s.repo.GetByID(ctx, c.TenantID, c.LedgerTransactionID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("linked transaction %s not found", c.LedgerTransactionID)
	}
	if err := validateManualCommissionLedgerPair(t, c); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransactionService) settleFromCommission(ctx context.Context, c *commission.Commission, paidAt *time.Time) (*transaction.Transaction, error) {
	t, err := s.transactionForCommission(ctx, c)
	if err != nil {
		return nil, err
	}
	switch transaction.CanonicalStatus(t.Status) {
	case transaction.StatusPaid:
		if err := s.postManualCommissionSettlementJournal(ctx, t, c); err != nil {
			return nil, err
		}
		return t, nil
	case transaction.StatusPending, transaction.StatusOverdue:
	default:
		return nil, fmt.Errorf("%w: linked transaction cannot be settled from %s", transaction.ErrInvalidStatusTransition, t.Status)
	}
	expectedUpdatedAt := t.UpdatedAt
	paymentDate := time.Time{}
	if paidAt != nil {
		paymentDate = *paidAt
	}
	if err := t.MarkPaid(paymentDate, t.PaymentMethod); err != nil {
		return nil, err
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, err
	}
	if err := s.postManualCommissionSettlementJournal(ctx, t, c); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransactionService) cancelFromCommission(ctx context.Context, c *commission.Commission) (*transaction.Transaction, error) {
	t, err := s.transactionForCommission(ctx, c)
	if err != nil {
		return nil, err
	}
	if transaction.CanonicalStatus(t.Status) == transaction.StatusCancelled {
		return t, nil
	}
	expectedUpdatedAt := t.UpdatedAt
	if err := t.MarkCancelled(); err != nil {
		return nil, err
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

// reopenFromCommission reabre o lançamento cancelado que representa a
// obrigação da comissão — espelho de cancelFromCommission, usado quando a
// ordem de pagamento é regerada. Um lançamento pago não reabre (estorno), e a
// recusa sobe com o motivo para o operador saber o que fazer.
func (s *TransactionService) reopenFromCommission(ctx context.Context, c *commission.Commission) (*transaction.Transaction, error) {
	t, err := s.transactionForCommission(ctx, c)
	if err != nil {
		return nil, err
	}
	if transaction.CanonicalStatus(t.Status) == transaction.StatusPending {
		return t, nil
	}
	expectedUpdatedAt := t.UpdatedAt
	if err := t.Reopen(); err != nil {
		return nil, err
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransactionService) reverseFromCommission(ctx context.Context, c *commission.Commission, reason string) (*transaction.Transaction, error) {
	t, err := s.transactionForCommission(ctx, c)
	if err != nil {
		return nil, err
	}
	switch transaction.CanonicalStatus(t.Status) {
	case transaction.StatusPending, transaction.StatusOverdue:
		return s.cancelFromCommission(ctx, c)
	case transaction.StatusPaid:
		reversal, err := s.reverse(ctx, t.ID, t.TenantID, reason, "commission:"+c.ID)
		if err != nil {
			return nil, err
		}
		if err := s.reverseManualCommissionSettlementJournal(ctx, c, reason); err != nil {
			return nil, err
		}
		return reversal, nil
	case transaction.StatusCancelled:
		return t, nil
	case transaction.StatusReversed:
		reversalSourceEventID := events.DeriveNotificationID("transaction.reversal", t.TenantID, t.ID)
		reversal, lookupErr := s.repo.GetBySourceEventID(ctx, t.TenantID, reversalSourceEventID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if reversal == nil {
			return nil, fmt.Errorf("reversed transaction %s has no counter-posting", t.ID)
		}
		if err := s.reverseManualCommissionSettlementJournal(ctx, c, reason); err != nil {
			return nil, err
		}
		return reversal, nil
	default:
		return nil, fmt.Errorf("%w: linked transaction has unsupported status %s", transaction.ErrInvalidStatusTransition, t.Status)
	}
}

func (s *TransactionService) List(ctx context.Context, tenantID, contractID string, limit, offset int64) ([]*transaction.Transaction, int64, error) {
	return s.repo.List(ctx, tenantID, contractID, limit, offset)
}

func (s *TransactionService) Search(ctx context.Context, tenantID string, filter transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return s.repo.Search(ctx, tenantID, filter)
}

// SettleReceivableBySourceEventID settles the receivable ledger entry that was
// materialized from the same upstream event as a paid charge.
//
// A contract can own unrelated transactions (commission, expense, transfer or
// even another receivable installment), so contract_id alone is not a safe
// settlement key. Until Transaction has a dedicated charge_id, an exact
// source_event_id match is the only supported link. Charges without that link
// intentionally leave the ledger untouched instead of guessing by contract.
func (s *TransactionService) SettleReceivableBySourceEventID(ctx context.Context, tenantID, contractID, sourceEventID string, paymentDate time.Time, method string) (*transaction.Transaction, error) {
	sourceEventID = strings.TrimSpace(sourceEventID)
	if sourceEventID == "" {
		return nil, nil
	}

	t, err := s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
	if err != nil {
		return nil, fmt.Errorf("tx settle: lookup by source event: %w", err)
	}
	if t == nil {
		return nil, nil
	}
	if t.TenantID != tenantID || t.ContractID != contractID {
		return nil, fmt.Errorf("tx settle: source event %q does not belong to charge contract", sourceEventID)
	}
	if t.Type != transaction.TypeReceita {
		return nil, nil
	}
	if t.Status != transaction.StatusPending && t.Status != transaction.StatusOverdue {
		return nil, nil
	}

	expectedUpdatedAt := t.UpdatedAt
	if err := t.MarkPaid(paymentDate, method); err != nil {
		return nil, fmt.Errorf("tx settle: %w", err)
	}
	if err := s.updateIfCurrent(ctx, t, expectedUpdatedAt); err != nil {
		return nil, fmt.Errorf("tx settle: update %s: %w", t.ID, err)
	}
	return t, nil
}

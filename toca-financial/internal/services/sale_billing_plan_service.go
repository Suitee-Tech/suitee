package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
)

// SaleBillingPlanService is the SALE sibling of RentalChargeConfigService.
// While rental configs emit a recurring stream of monthly charges until
// the contract ends, a sale billing plan pre-generates the entire payment
// calendar at signing time — either one charge (SINGLE / à vista) or N
// installments (INSTALLMENTS). Each emitted charge carries a deterministic
// SourceEventID ("sale.<plan_id>.installment.<i>") so broker re-deliveries
// collapse on the partial unique index from migration 20260518120000.
type SaleBillingPlanService struct {
	repo          sale_billing_plan.Repository
	chargeService *ChargeService
	// draftRepo (opcional) habilita o caminho pré-assinatura: operador
	// grava a forma de pagamento na esteira via UpsertDraft; o
	// contract.signed handler depois chama CreateFromDraft para
	// materializar. Nil-safe: sem ele, CreateFromDraft devolve
	// (nil, 0, nil) e o handler cai no fallback SINGLE atual.
	draftRepo sale_billing_plan_draft.Repository
}

func NewSaleBillingPlanService(repo sale_billing_plan.Repository, chargeService *ChargeService) *SaleBillingPlanService {
	return &SaleBillingPlanService{repo: repo, chargeService: chargeService}
}

// WithDraftRepo wires o repositório de drafts. Padrão fluent para
// permitir construção em main.go/routes.go sem mudar a assinatura
// existente de New. Sem chamada → service segue compatível com testes
// e ambientes sem o draft.
func (s *SaleBillingPlanService) WithDraftRepo(r sale_billing_plan_draft.Repository) *SaleBillingPlanService {
	s.draftRepo = r
	return s
}

// CreateAndGenerateCharges is the one entry point used by the SALE branch
// of ContractSignedEventHandler. It is idempotent on (tenant, contract) and
// converges any missing deterministic charge when replaying an existing plan.
//
// Charge calendar (cash legs only — dação/troca don't emit charges):
//   - DownPayment > 0   → "Sinal / Entrada" charge on FirstDueDate.
//   - Installments      → N "Parcela i/N" charges at one calendar month each.
//   - Saldo > 0         → single "Saldo" charge on SaldoDueDate.
//
// Composite is the optional bag with the non-cash legs (dação, troca)
// plus the deferred saldo. Pass a zero-value CompositeInput when none of
// those legs apply — the simple "downpayment + installments" path stays
// unchanged.
func (s *SaleBillingPlanService) CreateAndGenerateCharges(
	ctx context.Context,
	tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID string,
	mode sale_billing_plan.Mode,
	totalCents, downPaymentCents, installmentAmountCents int64,
	installmentCount int,
	firstDueDate time.Time,
	recurrenceIntervalDays int,
	composite sale_billing_plan.CompositeInput,
) (*sale_billing_plan.SaleBillingPlan, int, error) {
	existing, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, 0, err
	}
	if existing != nil {
		created, err := s.ensureCharges(ctx, existing)
		return existing, created, err
	}

	plan, err := sale_billing_plan.New(tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID, mode, totalCents, downPaymentCents, installmentAmountCents, installmentCount, firstDueDate, recurrenceIntervalDays, composite)
	if err != nil {
		return nil, 0, err
	}
	canonical, err := s.repo.Create(ctx, plan)
	if err != nil {
		return nil, 0, err
	}
	if err := validateCanonicalSaleBillingPlan(canonical, tenantID, contractID); err != nil {
		return nil, 0, err
	}

	created, err := s.ensureCharges(ctx, canonical)
	if err != nil {
		return nil, 0, err
	}
	return canonical, created, nil
}

func validateCanonicalSaleBillingPlan(plan *sale_billing_plan.SaleBillingPlan, tenantID, contractID string) error {
	if plan == nil {
		return errors.New("sale billing plan repository returned no canonical plan")
	}
	if plan.TenantID != tenantID || plan.ContractID != contractID {
		return fmt.Errorf("canonical sale billing plan scope mismatch: tenant=%q contract=%q", plan.TenantID, plan.ContractID)
	}
	return nil
}

// ensureCharges converges every deterministic cash leg. It is safe on broker
// retries and repairs a legacy partial materialization instead of returning
// early merely because the plan row exists.
func (s *SaleBillingPlanService) ensureCharges(ctx context.Context, plan *sale_billing_plan.SaleBillingPlan) (int, error) {
	if s.chargeService == nil {
		return 0, errors.New("charge service not configured")
	}
	charges := s.buildCharges(plan)
	created := 0
	for _, ch := range charges {
		_, wasCreated, err := s.chargeService.EnsureWithSourceEventID(ctx, ch.SourceEventID, ch.ContractID, plan.DealID, plan.BuyerPartyID, ch.TenantPartyID, ch.OwnerID, ch.PropertyID, ch.TenantID, ch.Type, ch.Description, ch.ReferenceMonth, ch.Amount.Float(), ch.DueDate)
		if err != nil {
			return 0, fmt.Errorf("failed to create sale charge %s: %w", ch.SourceEventID, err)
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (s *SaleBillingPlanService) GetByContractID(ctx context.Context, tenantID, contractID string) (*sale_billing_plan.SaleBillingPlan, error) {
	return s.repo.GetByContractID(ctx, tenantID, contractID)
}

// UpsertDraft grava (ou substitui) a escolha de forma de pagamento
// SALE feita pelo operador na esteira ANTES da assinatura. Idempotente
// por (tenant_id, deal_id) via partial unique index — re-submit
// substitui o draft anterior preservando o row id.
//
// O draft fica disponível para CreateFromDraft consumir quando o
// contract.signed dispara. UI também pode buscar via GetActiveDraft
// para popular o wizard ao reabrir.
func (s *SaleBillingPlanService) UpsertDraft(ctx context.Context, d *sale_billing_plan_draft.Draft) error {
	if s.draftRepo == nil {
		return fmt.Errorf("draft repository not configured")
	}
	return s.draftRepo.Upsert(ctx, d)
}

// GetActiveDraft devolve o draft ativo para o deal (consumed_at IS NULL),
// ou nil quando não há um. Nil-safe: sem draftRepo wired retorna
// (nil, nil) para preservar compat.
func (s *SaleBillingPlanService) GetActiveDraft(ctx context.Context, tenantID, dealID string) (*sale_billing_plan_draft.Draft, error) {
	if s.draftRepo == nil {
		return nil, nil
	}
	return s.draftRepo.GetActiveByDeal(ctx, tenantID, dealID)
}

// GetActiveDraftForService é a leitura usada pelas rotas internas
// (pré-contrato): sem claims de JWT no contexto, o pool do app não enxerga o
// draft e o plano configurado parecia ausente.
func (s *SaleBillingPlanService) GetActiveDraftForService(ctx context.Context, tenantID, dealID string) (*sale_billing_plan_draft.Draft, error) {
	if s.draftRepo == nil {
		return nil, nil
	}
	return s.draftRepo.GetActiveByDealForService(ctx, tenantID, dealID)
}

// CreateFromDraft is called by the contract.signed SALE branch: it validates
// the active draft against the frozen contract total, materializes the plan
// plus manual receivable charges, then marks the draft consumed.
//
// Returns (nil, 0, nil) when no draft exists; no fabricated SINGLE fallback
// is created because it would disagree with the operator preview.
//
// Idempotente: re-delivery do contract.signed após o draft consumido
// não recria charges — o early return de CreateAndGenerateCharges
// (existing plan != nil) cuida disso; o MarkConsumed é no-op em draft
// já consumido.
func (s *SaleBillingPlanService) CreateFromDraft(ctx context.Context, tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID string, expectedTotalCents int64) (*sale_billing_plan.SaleBillingPlan, int, error) {
	if expectedTotalCents <= 0 {
		return nil, 0, errors.New("frozen contract total must be greater than zero")
	}
	existing, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, 0, err
	}
	if existing != nil {
		if existing.TotalAmountCents != expectedTotalCents {
			return nil, 0, fmt.Errorf("billing plan total %d does not match frozen contract total %d cents", existing.TotalAmountCents, expectedTotalCents)
		}
		created, err := s.ensureCharges(ctx, existing)
		if err != nil {
			return nil, 0, err
		}
		if s.draftRepo != nil {
			if err := s.draftRepo.MarkConsumed(ctx, tenantID, dealID); err != nil {
				return nil, 0, fmt.Errorf("mark existing sale draft consumed: %w", err)
			}
		}
		return existing, created, nil
	}
	if s.draftRepo == nil {
		return nil, 0, nil
	}
	draft, err := s.draftRepo.GetActiveByDeal(ctx, tenantID, dealID)
	if err != nil {
		return nil, 0, fmt.Errorf("draft lookup failed: %w", err)
	}
	if draft == nil {
		return nil, 0, nil
	}
	if draft.TotalAmountCents != expectedTotalCents {
		return nil, 0, fmt.Errorf("draft total %d does not match frozen contract total %d cents", draft.TotalAmountCents, expectedTotalCents)
	}
	plan, created, err := s.CreateAndGenerateCharges(
		ctx, tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID,
		draft.Mode,
		draft.TotalAmountCents,
		draft.DownPaymentCents,
		draft.InstallmentAmountCents,
		draft.InstallmentCount,
		draft.FirstDueDate,
		draft.RecurrenceIntervalDays,
		draft.Composite,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("draft materialization failed: %w", err)
	}
	// The signed-contract pipeline runs this method in one SQL transaction.
	// Consuming the draft is part of that atomic materialization: if it fails,
	// plan and charges must roll back as well instead of leaving an active draft
	// that no longer represents what was persisted.
	if err := s.draftRepo.MarkConsumed(ctx, tenantID, dealID); err != nil {
		return nil, 0, fmt.Errorf("mark sale draft consumed: %w", err)
	}
	return plan, created, nil
}

// CreatePlanFromDraftOnly materializa o SaleBillingPlan a partir do
// draft do operador SEM gerar Charges — o cronograma de parcelas fica
// registrado como base de controle, mas nada é cobrado do comprador
// (modelo manual, sem gateway). Sem draft ativo, retorna (nil, nil) sem
// erro (nenhum fallback de cobrança à vista). Idempotente por contrato.
func (s *SaleBillingPlanService) CreatePlanFromDraftOnly(ctx context.Context, tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID string, expectedTotalCents int64) (*sale_billing_plan.SaleBillingPlan, error) {
	if s.draftRepo == nil {
		return nil, nil
	}
	draft, err := s.draftRepo.GetActiveByDeal(ctx, tenantID, dealID)
	if err != nil {
		return nil, fmt.Errorf("draft lookup failed: %w", err)
	}
	if draft == nil {
		return nil, nil
	}
	if expectedTotalCents <= 0 {
		return nil, errors.New("frozen contract total must be greater than zero")
	}
	if draft.TotalAmountCents != expectedTotalCents {
		return nil, fmt.Errorf("draft total %d does not match frozen contract total %d cents", draft.TotalAmountCents, expectedTotalCents)
	}
	existing, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := s.draftRepo.MarkConsumed(ctx, tenantID, dealID); err != nil {
			return nil, fmt.Errorf("mark existing sale draft consumed: %w", err)
		}
		return existing, nil
	}
	plan, err := sale_billing_plan.New(tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID, draft.Mode, draft.TotalAmountCents, draft.DownPaymentCents, draft.InstallmentAmountCents, draft.InstallmentCount, draft.FirstDueDate, draft.RecurrenceIntervalDays, draft.Composite)
	if err != nil {
		return nil, err
	}
	canonical, err := s.repo.Create(ctx, plan)
	if err != nil {
		return nil, err
	}
	if err := validateCanonicalSaleBillingPlan(canonical, tenantID, contractID); err != nil {
		return nil, err
	}
	if err := s.draftRepo.MarkConsumed(ctx, tenantID, dealID); err != nil {
		return nil, fmt.Errorf("mark sale draft consumed: %w", err)
	}
	return canonical, nil
}

// BuildChargesPreview produces, in memory, the exact list of charges
// that would be created by CreateAndGenerateCharges for the given plan
// — but without persisting, without contacting the gateway, and without
// touching the repository. Reused by FinancialPreviewService to render
// the SALE side of the "Financeiro (Prévia)" tab in the esteira.
//
// The plan must be a fully-constructed *SaleBillingPlan (typically
// built via sale_billing_plan.New). Each returned charge carries the
// same SourceEventID it would receive in production
// (sale.<plan.ID>.downpayment / sale.<plan.ID>.installment.<i> /
// sale.<plan.ID>.saldo) so the preview matches the materialization 1:1.
func (s *SaleBillingPlanService) BuildChargesPreview(plan *sale_billing_plan.SaleBillingPlan) []*charge.Charge {
	if plan == nil {
		return nil
	}
	return s.buildCharges(plan)
}

func (s *SaleBillingPlanService) GetByID(ctx context.Context, tenantID, id string) (*sale_billing_plan.SaleBillingPlan, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// buildCharges produces the in-memory list of charges to emit (cash legs
// only — non-cash legs like dação/troca are recorded on the plan but
// don't trigger a charge). They are not persisted here; the
// caller hands each one off to ChargeService.CreateWithSourceEventID so
// the shared idempotency/audit path applies.
//
// Order: down payment (if any) → N installments → saldo (if any). Each
// charge gets a deterministic SourceEventID so broker re-deliveries
// collapse on the partial unique index.
func (s *SaleBillingPlanService) buildCharges(plan *sale_billing_plan.SaleBillingPlan) []*charge.Charge {
	estimated := plan.InstallmentCount + 2
	out := make([]*charge.Charge, 0, estimated)

	appendCashCharge := func(eventSuffix, description string, amountCents int64, due time.Time) {
		if amountCents <= 0 {
			return
		}
		ch, _ := charge.New(plan.ContractID, plan.BuyerPartyID, plan.OwnerID, plan.PropertyID, plan.TenantID,
			charge.TypeSale, description, monthKey(due),
			centsToFloat(amountCents), due)
		if ch == nil {
			return
		}
		ch.SourceEventID = fmt.Sprintf("sale.%s.%s", plan.ID, eventSuffix)
		out = append(out, ch)
	}

	if plan.Mode == sale_billing_plan.ModeSingle {
		// SINGLE may still split into down payment + installment when
		// the operator wanted a single non-zero entrada plus a single
		// "parcela" representing the rest. Most often only one of the
		// two is non-zero; we emit only what's > 0 so the calendar
		// stays clean.
		appendCashCharge("downpayment", "Sinal / Entrada", plan.DownPaymentCents, plan.FirstDueDate)
		dueForInstallment := plan.FirstDueDate
		if plan.DownPaymentCents > 0 && plan.InstallmentAmountCents > 0 {
			dueForInstallment = addMonthsClamped(plan.FirstDueDate, 1)
		}
		appendCashCharge("installment.1", "Pagamento à vista", plan.InstallmentAmountCents, dueForInstallment)
	} else {
		appendCashCharge("downpayment", "Sinal / Entrada", plan.DownPaymentCents, plan.FirstDueDate)
		firstInstallmentMonth := 0
		if plan.HasDownPayment() {
			firstInstallmentMonth = 1
		}
		for i := 1; i <= plan.InstallmentCount; i++ {
			appendCashCharge(
				fmt.Sprintf("installment.%d", i),
				fmt.Sprintf("Parcela %d/%d", i, plan.InstallmentCount),
				plan.InstallmentAmountCents,
				addMonthsClamped(plan.FirstDueDate, firstInstallmentMonth+i-1),
			)
		}
	}

	if plan.HasSaldo() && plan.SaldoDueDate != nil {
		desc := plan.SaldoDescription
		if desc == "" {
			desc = "Saldo"
		}
		appendCashCharge("saldo", desc, plan.SaldoAmountCents, *plan.SaldoDueDate)
	}
	return out
}

// addMonthsClamped advances a due date by whole calendar months, preserving
// the day-of-month and clamping to the last valid day of the target month
// (Jan 31 → Feb 28/29 → Mar 31). This is the defined monthly cadence for
// SALE installments: a fixed 30-day interval would drift across short
// months and push later vencimentos off their contracted month.
func addMonthsClamped(from time.Time, months int) time.Time {
	return setDayOfMonth(from.Year(), from.Month()+time.Month(months), from.Day(), from.Location())
}

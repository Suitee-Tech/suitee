package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
)

// PayoutOrderService is the thin CRUD+state-machine surface over the
// payout_order repository. The richer orchestration (compute splits from a
// paid charge) lives in PayoutOrchestrationService; this service only
// handles the operator-driven verbs (list, approve, group, mark-paid,
// cancel) and direct creation.
type PayoutOrderService struct {
	repo                  payout_order.Repository
	commissionService     *CommissionService
	partnerRoyaltyService *PartnerRoyaltyService
	proofPolicy           proofPolicySettings
}

// ErrLinkedCommissionNotFound: o repasse a corretor não tem comissão vinculada,
// então uma ordem regerada não poderia ser aprovada (o cascade da aprovação exige
// a comissão). Melhor recusar a regeração do que devolver uma ordem inutilizável.
var ErrLinkedCommissionNotFound = errors.New("comissão vinculada não encontrada para a obrigação")

func NewPayoutOrderService(repo payout_order.Repository) *PayoutOrderService {
	return &PayoutOrderService{repo: repo}
}

func (s *PayoutOrderService) SetCommissionService(service *CommissionService) {
	s.commissionService = service
}

func (s *PayoutOrderService) SetPartnerRoyaltyService(service *PartnerRoyaltyService) {
	s.partnerRoyaltyService = service
}

// SetProofPolicy conecta a política de comprovante do tenant. Antes a exigência
// era incondicional no domínio, então o setting payout_proof_required não
// governava nada. Sem isso vale o default do domínio, que exige.
func (s *PayoutOrderService) SetProofPolicy(settings proofPolicySettings) {
	s.proofPolicy = settings
}

func (s *PayoutOrderService) Create(ctx context.Context, order *payout_order.PayoutOrder) error {
	return s.repo.Create(ctx, order)
}

func (s *PayoutOrderService) GetByID(ctx context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *PayoutOrderService) List(ctx context.Context, tenantID string, filter payout_order.ListFilter) ([]*payout_order.PayoutOrder, int64, error) {
	return s.repo.List(ctx, tenantID, filter)
}

// Totals retorna totais agregados server-side para KPIs da inbox,
// independente da paginação (FIN-COM-13).
func (s *PayoutOrderService) Totals(ctx context.Context, tenantID string, filter payout_order.ListFilter) (*payout_order.PayoutOrderTotals, error) {
	return s.repo.Totals(ctx, tenantID, filter)
}

func (s *PayoutOrderService) persistTransition(ctx context.Context, order *payout_order.PayoutOrder, expected payout_order.Status, expectedUpdatedAt time.Time) error {
	repo, ok := s.repo.(payout_order.TransitionRepository)
	if !ok {
		return payout_order.ErrTransitionCompareAndSetMissing
	}
	return repo.UpdateIfStatus(ctx, order, expected, expectedUpdatedAt)
}

func (s *PayoutOrderService) needsAtomicCommissionCascade(order *payout_order.PayoutOrder) bool {
	if order == nil {
		return false
	}
	if isRoyaltyPayoutCandidate(order) {
		return true
	}
	if order.RecipientType != payout_order.RecipientBroker {
		return false
	}
	dealID, _ := order.SourceRef["deal_id"].(string)
	return dealID != ""
}

func isRoyaltyPayoutCandidate(order *payout_order.PayoutOrder) bool {
	return order != nil && (order.SourceType == payout_order.SourceRoyaltySettlement || order.RecipientType == payout_order.RecipientRoyaltyPayee)
}

// persistTransitionAndCascade writes the Commission side first and the payout
// CAS second inside one transaction. If the CAS loses a race, the Commission
// write rolls back; if the cascade fails, the payout is never advanced. The
// commission-first lock order matches deduction synchronization and avoids a
// payout↔commission deadlock cycle.
func (s *PayoutOrderService) persistTransitionAndCascade(
	ctx context.Context,
	order *payout_order.PayoutOrder,
	expected payout_order.Status,
	expectedUpdatedAt time.Time,
	cascade func(context.Context) error,
) error {
	action := func(txCtx context.Context) error {
		if cascade != nil {
			if err := cascade(txCtx); err != nil {
				return err
			}
		}
		return s.persistTransition(txCtx, order, expected, expectedUpdatedAt)
	}
	if isRoyaltyPayoutCandidate(order) {
		runner, ok := s.repo.(payout_order.RoyaltyTransitionTransactionRepository)
		if !ok {
			return payout_order.ErrRoyaltyServiceTransactionMissing
		}
		return runner.RunRoyaltyTransitionTransaction(ctx, action)
	}
	if runner, ok := s.repo.(payout_order.TransitionTransactionRepository); ok {
		return runner.RunTransitionTransaction(ctx, action)
	}
	if s.needsAtomicCommissionCascade(order) {
		return payout_order.ErrTransitionTransactionMissing
	}
	return action(ctx)
}

func (s *PayoutOrderService) runCommissionCascadeAtomically(ctx context.Context, order *payout_order.PayoutOrder, cascade func(context.Context) error) error {
	if isRoyaltyPayoutCandidate(order) {
		runner, ok := s.repo.(payout_order.RoyaltyTransitionTransactionRepository)
		if !ok {
			return payout_order.ErrRoyaltyServiceTransactionMissing
		}
		return runner.RunRoyaltyTransitionTransaction(ctx, cascade)
	}
	if !s.needsAtomicCommissionCascade(order) {
		return cascade(ctx)
	}
	runner, ok := s.repo.(payout_order.TransitionTransactionRepository)
	if !ok {
		return payout_order.ErrTransitionTransactionMissing
	}
	return runner.RunTransitionTransaction(ctx, cascade)
}

func (s *PayoutOrderService) reloadAfterConcurrentTransition(ctx context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, payout_order.ErrConcurrentTransition
	}
	return current, nil
}

// Approve drives the PENDING_REVIEW → APPROVED transition. The actor is
// stamped into the audit history so the team can answer "who approved
// this" without consulting external logs.
func (s *PayoutOrderService) Approve(ctx context.Context, tenantID, id, actor string) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	// P1/A13 — criador ≠ aprovador (mesma regra do fluxo de managed bills).
	if err := requireDistinctRequesterAndApprover(ctx, order.RequestedBy, actor); err != nil {
		return nil, err
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.Approve(actor); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.cascadeCommissionStatus(txCtx, order, commission.StatusApproved, "")
	}); err != nil {
		return nil, fmt.Errorf("approve payout and linked commission: %w", err)
	}
	return order, nil
}

func (s *PayoutOrderService) ApproveIdempotent(ctx context.Context, tenantID, id, actor string) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	if order.Status == payout_order.StatusApproved {
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.cascadeCommissionStatus(txCtx, order, commission.StatusApproved, "")
		}); err != nil {
			return nil, fmt.Errorf("commission approval cascade failed: %w", err)
		}
		return order, nil
	}
	if err := requireDistinctRequesterAndApprover(ctx, order.RequestedBy, actor); err != nil {
		return nil, err
	}
	if order.Status == payout_order.StatusPaid {
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.markLinkedCommissionPaid(txCtx, order, markPaidInputFromOrder(order))
		}); err != nil {
			return nil, fmt.Errorf("commission paid cascade failed: %w", err)
		}
		return order, nil
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.Approve(actor); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.cascadeCommissionStatus(txCtx, order, commission.StatusApproved, "")
	}); err != nil {
		if !errors.Is(err, payout_order.ErrConcurrentTransition) {
			return nil, err
		}
		order, err = s.reloadAfterConcurrentTransition(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		switch order.Status {
		case payout_order.StatusApproved:
			if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
				return s.cascadeCommissionStatus(txCtx, order, commission.StatusApproved, "")
			}); err != nil {
				return nil, fmt.Errorf("commission approval cascade failed: %w", err)
			}
			return order, nil
		case payout_order.StatusPaid:
			if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
				return s.markLinkedCommissionPaid(txCtx, order, markPaidInputFromOrder(order))
			}); err != nil {
				return nil, fmt.Errorf("commission paid cascade failed: %w", err)
			}
			return order, nil
		default:
			return nil, payout_order.ErrConcurrentTransition
		}
	}
	return order, nil
}

func (s *PayoutOrderService) markLinkedCommissionPaid(ctx context.Context, order *payout_order.PayoutOrder, in MarkPaidInput) error {
	if handled, err := s.cascadePartnerRoyaltyStatus(ctx, order, payout_order.StatusPaid); handled {
		return err
	}
	if order == nil || order.RecipientType != payout_order.RecipientBroker {
		return nil
	}
	dealID, _ := order.SourceRef["deal_id"].(string)
	if dealID == "" {
		return nil
	}
	if s.commissionService == nil {
		return fmt.Errorf("commission service is required for broker payout %s", order.ID)
	}
	commissions, err := s.commissionService.ListByDeal(ctx, order.TenantID, dealID)
	if err != nil {
		return fmt.Errorf("list linked commissions: %w", err)
	}
	comm := s.linkedCommissionForOrder(ctx, order, commissions)
	if comm == nil {
		return fmt.Errorf("linked commission not found for broker payout %s", order.ID)
	}
	if _, err := s.commissionService.MarkPaidFromPayout(ctx, order.TenantID, comm.ID, comm.PayoutOrderID, in.ProofURL, in.Note); err != nil {
		return fmt.Errorf("mark linked commission %s paid: %w", comm.ID, err)
	}
	return nil
}

// cascadeCommissionStatus mirrors PayoutOrder status transitions to linked
// Commission entities. This keeps the operator's inbox view consistent:
// approving/cancelling a PayoutOrder automatically approves/cancels the
// underlying Commission row. MarkPaid is handled separately via
// markLinkedCommissionPaid because it requires proof_url.
//
// FIN-COM-06: lifecycle Commission ↔ PayoutOrder unificado.
func (s *PayoutOrderService) cascadeCommissionStatus(ctx context.Context, order *payout_order.PayoutOrder, target commission.Status, reason string) error {
	royaltyTarget := payout_order.Status("")
	switch target {
	case commission.StatusApproved:
		royaltyTarget = payout_order.StatusApproved
	case commission.StatusCancelled:
		royaltyTarget = payout_order.StatusCancelled
	}
	if handled, err := s.cascadePartnerRoyaltyStatus(ctx, order, royaltyTarget); handled {
		return err
	}
	if order == nil || order.RecipientType != payout_order.RecipientBroker {
		return nil
	}
	dealID, _ := order.SourceRef["deal_id"].(string)
	if dealID == "" {
		return nil
	}
	if s.commissionService == nil {
		return fmt.Errorf("commission service is required for broker payout %s", order.ID)
	}
	commissions, err := s.commissionService.ListByDeal(ctx, order.TenantID, dealID)
	if err != nil {
		return fmt.Errorf("list linked commissions: %w", err)
	}
	comm := s.linkedCommissionForOrder(ctx, order, commissions)
	if comm == nil {
		return fmt.Errorf("linked commission not found for broker payout %s", order.ID)
	}
	switch target {
	// O vínculo passado é o que a comissão JÁ carrega: depois de uma regeração
	// ela pode apontar para outra ordem da mesma obrigação, e exigir o id desta
	// ordem faria a aprovação/cancelamento falharem com "commission is not linked
	// to this payout order". A pertinência à obrigação é garantida por
	// linkedCommissionForOrder, um nível acima.
	case commission.StatusApproved:
		if _, err := s.commissionService.ApproveFromPayout(ctx, order.TenantID, comm.ID, comm.PayoutOrderID); err != nil {
			return fmt.Errorf("approve linked commission %s: %w", comm.ID, err)
		}
	case commission.StatusCancelled:
		if _, err := s.commissionService.CancelFromPayout(ctx, order.TenantID, comm.ID, comm.PayoutOrderID, reason); err != nil {
			return fmt.Errorf("cancel linked commission %s: %w", comm.ID, err)
		}
	}
	return nil
}

func (s *PayoutOrderService) cascadePartnerRoyaltyStatus(ctx context.Context, order *payout_order.PayoutOrder, target payout_order.Status) (bool, error) {
	if order == nil {
		return false, nil
	}
	royaltyCandidate := isRoyaltyPayoutCandidate(order)
	if !royaltyCandidate {
		return false, nil
	}
	if order.SourceType != payout_order.SourceRoyaltySettlement || order.RecipientType != payout_order.RecipientRoyaltyPayee {
		return true, fmt.Errorf("royalty payout %s has inconsistent source/recipient types", order.ID)
	}
	royaltyID, _ := order.SourceRef["royalty_id"].(string)
	if royaltyID == "" || order.SourceID == "" || royaltyID != order.SourceID || order.RecipientID == "" || order.ContractID == "" {
		return true, fmt.Errorf("royalty payout %s has an invalid royalty identity", order.ID)
	}
	if s.partnerRoyaltyService == nil {
		return true, fmt.Errorf("partner royalty service is required for payout %s", order.ID)
	}
	link := RoyaltyPayoutLink{
		RoyaltyID:     royaltyID,
		PayoutOrderID: order.ID,
		PayerTenant:   order.TenantID,
		PayeeTenant:   order.RecipientID,
		ContractID:    order.ContractID,
		AmountCents:   order.AmountCents,
	}
	var err error
	switch target {
	case payout_order.StatusApproved:
		_, err = s.partnerRoyaltyService.ApproveFromPayout(ctx, link)
	case payout_order.StatusPaid:
		_, err = s.partnerRoyaltyService.MarkPaidFromPayout(ctx, link)
	case payout_order.StatusCancelled:
		_, err = s.partnerRoyaltyService.CancelFromPayout(ctx, link)
	default:
		err = fmt.Errorf("unsupported royalty payout target status %s", target)
	}
	if err != nil {
		return true, fmt.Errorf("cascade payout %s to royalty %s: %w", order.ID, royaltyID, err)
	}
	return true, nil
}

// MarkPaid records that the operator settled the payout off-platform.
// Requires a proof URL (the settlement happened outside our system, the
// proof is our audit trail).
type MarkPaidInput struct {
	Method        string
	ProofURL      string
	ProofFilename string
	Note          string
}

func markPaidInputFromOrder(order *payout_order.PayoutOrder) MarkPaidInput {
	if order == nil {
		return MarkPaidInput{}
	}
	return MarkPaidInput{
		Method:        string(order.SettlementMethod),
		ProofURL:      order.ProofURL,
		ProofFilename: order.ProofFilename,
	}
}

func (s *PayoutOrderService) MarkPaid(ctx context.Context, tenantID, id, actor string, in MarkPaidInput) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	// P1/A13 — pagador ≠ aprovador.
	if err := requireDistinctApproverAndPayer(ctx, order.ApprovedBy, actor); err != nil {
		return nil, err
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.MarkPaid(actor, in.Method, in.ProofURL, in.ProofFilename, in.Note, resolveManualProofRequired(ctx, s.proofPolicy, tenantID)); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.markLinkedCommissionPaid(txCtx, order, in)
	}); err != nil {
		return nil, fmt.Errorf("mark payout and linked commission paid: %w", err)
	}
	return order, nil
}

func (s *PayoutOrderService) MarkPaidIdempotent(ctx context.Context, tenantID, id, actor string, in MarkPaidInput) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	if order.Status == payout_order.StatusPaid {
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.markLinkedCommissionPaid(txCtx, order, markPaidInputFromOrder(order))
		}); err != nil {
			return nil, fmt.Errorf("commission paid cascade failed: %w", err)
		}
		return order, nil
	}
	// P1/A13 — pagador ≠ aprovador.
	if err := requireDistinctApproverAndPayer(ctx, order.ApprovedBy, actor); err != nil {
		return nil, err
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.MarkPaid(actor, in.Method, in.ProofURL, in.ProofFilename, in.Note, resolveManualProofRequired(ctx, s.proofPolicy, tenantID)); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.markLinkedCommissionPaid(txCtx, order, in)
	}); err != nil {
		if !errors.Is(err, payout_order.ErrConcurrentTransition) {
			return nil, err
		}
		order, err = s.reloadAfterConcurrentTransition(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if order.Status != payout_order.StatusPaid {
			return nil, payout_order.ErrConcurrentTransition
		}
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.markLinkedCommissionPaid(txCtx, order, markPaidInputFromOrder(order))
		}); err != nil {
			return nil, fmt.Errorf("commission paid cascade failed: %w", err)
		}
		return order, nil
	}
	return order, nil
}

func (s *PayoutOrderService) Cancel(ctx context.Context, tenantID, id, actor, reason string) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.Cancel(actor, reason); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.cascadeCommissionStatus(txCtx, order, commission.StatusCancelled, reason)
	}); err != nil {
		return nil, fmt.Errorf("cancel payout and linked commission: %w", err)
	}
	return order, nil
}

// Regenerate recria a ordem cancelada como uma NOVA ordem, pronta para ser
// aprovada e lançada outra vez, e reabre a obrigação vinculada (comissão e
// lançamento contábil) na MESMA transação.
//
// Antes: cancelar encerrava a ordem e a comissão em cascata e não havia volta —
// o operador que cancelasse por engano (ou para corrigir o destinatário) ficava
// sem caminho no sistema, com a obrigação morta. Aqui a ordem cancelada vira
// histórico e `regenerated_from_id` liga as duas.
//
// Idempotente: se já existe ordem VIVA para a mesma origem, ela é devolvida em
// vez de criar uma segunda — dois cliques no botão não geram duas obrigações.
func (s *PayoutOrderService) Regenerate(ctx context.Context, tenantID, id, actor string) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	if order.Status != payout_order.StatusCancelled {
		return nil, payout_order.ErrNotCancelled
	}
	if order.SourceEventID != "" {
		live, err := s.repo.GetBySourceEventID(ctx, tenantID, order.SourceEventID)
		if err != nil {
			return nil, err
		}
		if live != nil && live.ID != order.ID && live.Status != payout_order.StatusCancelled {
			return live, nil
		}
	}

	fresh, err := order.Regenerate(actor)
	if err != nil {
		return nil, err
	}

	// Ordem nova + reabertura da obrigação na mesma transação: uma ordem viva
	// apontando para uma comissão cancelada reprovaria na aprovação (o cascade
	// exige a comissão compatível) e deixaria o contrato sem o repasse.
	publish := func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, fresh); err != nil {
			return fmt.Errorf("create regenerated payout: %w", err)
		}
		if cascadeErr := s.reopenLinkedObligation(txCtx, order, fresh, actor); cascadeErr != nil {
			return cascadeErr
		}
		return nil
	}
	if isRoyaltyPayoutCandidate(order) {
		runner, ok := s.repo.(payout_order.RoyaltyTransitionTransactionRepository)
		if !ok {
			return nil, payout_order.ErrRoyaltyServiceTransactionMissing
		}
		if err := runner.RunRoyaltyTransitionTransaction(ctx, publish); err != nil {
			return nil, err
		}
		return fresh, nil
	}
	if runner, ok := s.repo.(payout_order.TransitionTransactionRepository); ok {
		if err := runner.RunTransitionTransaction(ctx, publish); err != nil {
			return nil, err
		}
		return fresh, nil
	}
	if err := publish(ctx); err != nil {
		return nil, err
	}
	return fresh, nil
}

// reopenLinkedObligation reabre a comissão (e o lançamento) que o cancelamento
// desta ordem derrubou, religando-a à ordem nova. Espelha o cascade do
// cancelamento; obrigação já viva é no-op idempotente.
func (s *PayoutOrderService) reopenLinkedObligation(ctx context.Context, cancelled, fresh *payout_order.PayoutOrder, actor string) error {
	if handled, err := s.reopenPartnerRoyalty(ctx, cancelled, fresh); handled {
		return err
	}
	if cancelled.RecipientType != payout_order.RecipientBroker {
		return nil
	}
	dealID, _ := cancelled.SourceRef["deal_id"].(string)
	if dealID == "" {
		return nil
	}
	if s.commissionService == nil {
		return fmt.Errorf("commission service is required for broker payout %s", cancelled.ID)
	}
	commissions, err := s.commissionService.ListByDeal(ctx, cancelled.TenantID, dealID)
	if err != nil {
		return fmt.Errorf("list linked commissions: %w", err)
	}
	comm := s.linkedCommissionForOrder(ctx, cancelled, commissions)
	if comm != nil {
		if _, err := s.commissionService.ReopenFromPayout(ctx, cancelled.TenantID, comm.ID, comm.PayoutOrderID, fresh.ID, actor,
			"reaberta para relançamento da ordem "+fresh.ID); err != nil {
			return fmt.Errorf("reopen linked commission %s: %w", comm.ID, err)
		}
	}
	if comm == nil {
		// Repasse a corretor SEM comissão vinculada: a ordem regerada nasceria
		// impossível de aprovar (o cascade da aprovação exige a comissão), então
		// falhamos aqui, com o motivo, em vez de entregar uma ordem quebrada.
		return fmt.Errorf("%w: nenhuma comissão do negócio %s está vinculada à obrigação %s",
			ErrLinkedCommissionNotFound, dealID, cancelled.SourceEventID)
	}
	return nil
}

// linkedCommissionForOrder resolve a comissão que representa a obrigação desta
// ordem.
//
// O vínculo direto (`PayoutOrderID == order.ID`) é o caso comum e tem
// precedência. Mas a comissão pode estar presa a OUTRA ordem da MESMA origem: a
// cada regeração nasce uma ordem nova, e o operador pode cancelar a nova e
// regerar a partir de uma cancelada anterior. Aí a comparação por id não acha
// nada, a comissão fica morta com uma ordem viva na mão e a APROVAÇÃO falha com
// "linked commission not found for broker payout" (foi o que o beta mostrou ao
// tentar cancelar a ordem regerada antes deste ajuste).
//
// A identidade da obrigação é o `source_event_id` — é ele que diz "esta é a
// mesma comissão do mesmo repasse" —, então o segundo passo compara a origem da
// ordem vinculada à comissão com a origem desta ordem.
func (s *PayoutOrderService) linkedCommissionForOrder(ctx context.Context, order *payout_order.PayoutOrder, commissions []*commission.Commission) *commission.Commission {
	if order == nil {
		return nil
	}
	for _, comm := range commissions {
		if comm != nil && comm.PayoutOrderID == order.ID {
			return comm
		}
	}
	if order.SourceEventID == "" {
		return nil
	}
	for _, comm := range commissions {
		if comm == nil || comm.PayoutOrderID == "" {
			continue
		}
		orders, err := s.repo.ListByIDs(ctx, order.TenantID, []string{comm.PayoutOrderID})
		if err != nil || len(orders) == 0 || orders[0] == nil {
			continue
		}
		if orders[0].SourceEventID != "" && orders[0].SourceEventID == order.SourceEventID {
			return comm
		}
	}
	return nil
}

// reopenPartnerRoyalty reabre o royalty de parceiro que o cancelamento
// derrubou, religando-o à ordem nova. Espelha cascadePartnerRoyaltyStatus.
func (s *PayoutOrderService) reopenPartnerRoyalty(ctx context.Context, cancelled, fresh *payout_order.PayoutOrder) (bool, error) {
	if cancelled == nil || !isRoyaltyPayoutCandidate(cancelled) {
		return false, nil
	}
	if cancelled.SourceType != payout_order.SourceRoyaltySettlement || cancelled.RecipientType != payout_order.RecipientRoyaltyPayee {
		return true, fmt.Errorf("royalty payout %s has inconsistent source/recipient types", cancelled.ID)
	}
	royaltyID, _ := cancelled.SourceRef["royalty_id"].(string)
	if royaltyID == "" || cancelled.SourceID == "" || royaltyID != cancelled.SourceID || cancelled.RecipientID == "" || cancelled.ContractID == "" {
		return true, fmt.Errorf("royalty payout %s has an invalid royalty identity", cancelled.ID)
	}
	if s.partnerRoyaltyService == nil {
		return true, fmt.Errorf("partner royalty service is required for payout %s", cancelled.ID)
	}
	link := RoyaltyPayoutLink{
		RoyaltyID:     royaltyID,
		PayoutOrderID: fresh.ID,
		PayerTenant:   cancelled.TenantID,
		PayeeTenant:   cancelled.RecipientID,
		ContractID:    cancelled.ContractID,
		AmountCents:   cancelled.AmountCents,
	}
	if _, err := s.partnerRoyaltyService.ReopenFromPayout(ctx, link); err != nil {
		return true, fmt.Errorf("reopen payout %s royalty %s: %w", cancelled.ID, royaltyID, err)
	}
	return true, nil
}

// CommissionSummary é o retrato da comissão que originou a ordem de pagamento.
//
// A ordem carrega o valor LÍQUIDO (o que será transferido), mas quem opera
// precisa ver de onde ele veio: comissão bruta, o que foi retido/abatido
// (tributos e adiantamentos) e o líquido que sobrou. Sem isso o operador só vê
// um número solto e não consegue conferir a retenção que acabou de lançar.
type CommissionSummary struct {
	CommissionID string                       `json:"commission_id"`
	BrokerID     string                       `json:"broker_id,omitempty"`
	Role         string                       `json:"role,omitempty"`
	GrossCents   int64                        `json:"gross_cents"`
	NetCents     int64                        `json:"net_cents"`
	Deductions   []CommissionDeductionSummary `json:"deductions,omitempty"`
}

type CommissionDeductionSummary struct {
	ID          string  `json:"id,omitempty"`
	Kind        string  `json:"kind"`
	Description string  `json:"description,omitempty"`
	AmountCents int64   `json:"amount_cents"`
	RatePct     float64 `json:"rate_pct,omitempty"`
	BaseCents   int64   `json:"base_cents,omitempty"`
}

// CommissionSummaryForOrder resolve a comissão vinculada a esta ordem (repasse a
// corretor). Devolve nil quando não há comissão ou quando ela é de outro tipo de
// destinatário — a tela simplesmente não mostra o bloco.
func (s *PayoutOrderService) CommissionSummaryForOrder(ctx context.Context, order *payout_order.PayoutOrder) (*CommissionSummary, error) {
	if s == nil || s.commissionService == nil || order == nil {
		return nil, nil
	}
	if order.RecipientType != payout_order.RecipientBroker {
		return nil, nil
	}
	dealID, _ := order.SourceRef["deal_id"].(string)
	if dealID == "" {
		return nil, nil
	}
	commissions, err := s.commissionService.ListByDeal(ctx, order.TenantID, dealID)
	if err != nil {
		return nil, err
	}
	for _, c := range commissions {
		if c == nil || c.PayoutOrderID != order.ID {
			continue
		}
		gross := c.GrossAmount
		if gross == 0 {
			gross = c.Amount
		}
		summary := &CommissionSummary{
			CommissionID: c.ID,
			BrokerID:     c.BrokerID,
			Role:         c.Role,
			GrossCents:   int64(math.Round(gross * 100)),
			NetCents:     int64(math.Round(c.NetAmount * 100)),
		}
		if summary.NetCents == 0 && len(c.Deductions) == 0 {
			summary.NetCents = summary.GrossCents
		}
		for _, d := range c.Deductions {
			summary.Deductions = append(summary.Deductions, CommissionDeductionSummary{
				ID:          d.ID,
				Kind:        string(d.Kind),
				Description: d.Description,
				AmountCents: int64(math.Round(d.Amount * 100)),
				RatePct:     d.RatePct,
				BaseCents:   d.BaseCents,
			})
		}
		return summary, nil
	}
	return nil, nil
}

func (s *PayoutOrderService) CancelIdempotent(ctx context.Context, tenantID, id, actor, reason string) (*payout_order.PayoutOrder, error) {
	order, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("payout_order not found")
	}
	if order.Status == payout_order.StatusCancelled {
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.cascadeCommissionStatus(txCtx, order, commission.StatusCancelled, reason)
		}); err != nil {
			return nil, fmt.Errorf("commission cancellation cascade failed: %w", err)
		}
		return order, nil
	}
	if order.Status == payout_order.StatusFailed {
		return order, nil
	}
	expected := order.Status
	expectedUpdatedAt := order.UpdatedAt
	if err := order.Cancel(actor, reason); err != nil {
		return nil, err
	}
	if err := s.persistTransitionAndCascade(ctx, order, expected, expectedUpdatedAt, func(txCtx context.Context) error {
		return s.cascadeCommissionStatus(txCtx, order, commission.StatusCancelled, reason)
	}); err != nil {
		if !errors.Is(err, payout_order.ErrConcurrentTransition) {
			return nil, err
		}
		order, err = s.reloadAfterConcurrentTransition(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if order.Status != payout_order.StatusCancelled {
			return nil, payout_order.ErrConcurrentTransition
		}
		if err := s.runCommissionCascadeAtomically(ctx, order, func(txCtx context.Context) error {
			return s.cascadeCommissionStatus(txCtx, order, commission.StatusCancelled, reason)
		}); err != nil {
			return nil, fmt.Errorf("commission cancellation cascade failed: %w", err)
		}
		return order, nil
	}
	return order, nil
}

// Group attaches a fresh group_id to a list of orders. The group_id is
// generated server-side (uuid). Each order goes through AssignGroup which
// validates the status; orders not in PENDING_REVIEW or APPROVED are
// skipped silently — the operator sees the unchanged status in the response.
func (s *PayoutOrderService) Group(ctx context.Context, tenantID string, ids []string, actor string) (string, []*payout_order.PayoutOrder, error) {
	return s.GroupWithID(ctx, tenantID, ids, actor, uuid.New().String())
}

func (s *PayoutOrderService) GroupWithID(ctx context.Context, tenantID string, ids []string, actor string, groupID string) (string, []*payout_order.PayoutOrder, error) {
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("at least one order id is required")
	}
	orders, err := s.repo.ListByIDs(ctx, tenantID, ids)
	if err != nil {
		return "", nil, err
	}
	if groupID == "" {
		return "", nil, fmt.Errorf("group_id is required")
	}
	updated := make([]*payout_order.PayoutOrder, 0, len(orders))
	action := func(txCtx context.Context) error {
		for _, o := range orders {
			expectedStatus := o.Status
			expectedUpdatedAt := o.UpdatedAt
			if err := o.AssignGroup(groupID, actor); err != nil {
				// Skip orders not in an assignable status; the caller sees the
				// unchanged copy from a follow-up GET.
				continue
			}
			// Grouping serializes the whole order, so it needs the same revision
			// CAS as lifecycle transitions. Otherwise a stale group request can
			// overwrite a concurrent PAID/CANCELLED state or deduction amount.
			if err := s.persistTransition(txCtx, o, expectedStatus, expectedUpdatedAt); err != nil {
				return err
			}
			updated = append(updated, o)
		}
		if len(updated) == 0 {
			return fmt.Errorf("no orders eligible for grouping")
		}
		return nil
	}
	if runner, ok := s.repo.(payout_order.TransitionTransactionRepository); ok {
		if err := runner.RunTransitionTransaction(ctx, action); err != nil {
			return "", nil, err
		}
	} else if err := action(ctx); err != nil {
		return "", nil, err
	}
	return groupID, updated, nil
}

package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Broker split and checklist mutations are operational subdomains of a deal
// and are kept out of its creation/query lifecycle.

// AddBrokerParticipation adds a broker with a role to a deal (idempotent by broker+role).
func (s *DealService) AddBrokerParticipation(ctx context.Context, tenantID, dealID, scopeBrokerID, brokerID string, role deal.BrokerParticipationRole, commission, percentage float64) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, scopeBrokerID)
	if err != nil {
		return nil, err
	}
	d.AddBrokerParticipation(brokerID, role, commission, percentage)
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ErrInvalidSplitSum indica que a soma dos percentuais informados não fecha
// em 100% (tolerância de ±0.01 absorve arredondamento). Mapeada para HTTP
// 422 no handler. Issue #420 — quando admin/gestor ajusta a distribuição
// 40/60 (ou 30/20/50, etc.) o backend valida antes de persistir para o
// frontend nunca conseguir gravar um split inconsistente.
var ErrInvalidSplitSum = errors.New("broker_participations percentages must sum to 100% (±0.01)")

// ErrInvalidCommissionPercentage protects the downstream signed-event
// pipeline from persisting a deal override it cannot materialize. A nil
// pointer means "keep the current value"; a supplied override must be finite
// and greater than zero. Tenant defaults remain the supported way to disable
// commission with an explicit zero until the event schema carries presence.
var ErrInvalidCommissionPercentage = errors.New("commission percentage must be greater than 0 and at most 100")

// ErrSplitLockedAfterSigned indica que o contrato relacionado já foi
// assinado e a distribuição de comissão está congelada. Issue #420 — o
// frontend mostra a trava como badge, mas o backend é a fonte de verdade
// porque o evento contract.signed já gerou Commission entities baseadas no
// split anterior (alterar depois descasaria UI da realidade financeira).
var ErrSplitLockedAfterSigned = errors.New("broker_participations are locked because the contract is SIGNED")

// ErrBrokerNotInTenant — a divisão nomeia corretor que não existe no tenant
// (N14). É erro do CHAMADOR (4xx), não falha de infraestrutura: o id veio do
// corpo da requisição.
var ErrBrokerNotInTenant = errors.New("broker_participations references a broker that does not belong to this company")

// SetBrokerParticipations replaces the entire BrokerParticipations array on
// a deal in one atomic update. Issue #420 — used by the admin/gestor UI
// when ajustando a distribuição de comissão (captador / visitador /
// fechamento) antes da assinatura.
//
// Validações:
//   - len(participations) ≥ 1 (não dá pra zerar quem recebe comissão)
//   - Soma de percentages == 100% (±0.01) — protege contra UI bug ou
//     payload manual quebrado
//   - Status do deal não pode estar em CONTRACT já assinado — ver
//     d.IsBrokerSplitLocked(). Backend é a fonte de verdade porque o
//     evento contract.signed já consumiu o split anterior.
//
// O caller passa scopeBrokerID quando a chamada vem de um corretor (a
// permissão `deals:write` já foi validada no handler — aqui só fazemos
// scope-by-broker para impedir corretor mexer em deal alheio). Para
// admin/gestor passa string vazia e o getDealByScope retorna sem filtro.
func (s *DealService) SetBrokerParticipations(ctx context.Context, tenantID, dealID, scopeBrokerID, actorID, actorEmail string, commissionPct *float64, participations []deal.BrokerParticipation) (*deal.Deal, error) {
	if len(participations) == 0 {
		return nil, fmt.Errorf("%w: at least one participation is required", ErrInvalidSplitSum)
	}
	if commissionPct != nil && (math.IsNaN(*commissionPct) || math.IsInf(*commissionPct, 0) || *commissionPct <= 0 || *commissionPct > 100) {
		return nil, fmt.Errorf("%w: got %.2f", ErrInvalidCommissionPercentage, *commissionPct)
	}
	// Sum check com tolerância para arredondamento. 0.01 = 1 centésimo
	// de percentual — folga suficiente pra somar 33.33 + 33.33 + 33.34
	// sem disparar erro, mas tight o bastante pra pegar UI bug que envia
	// 50 + 49 = 99.
	var sum float64
	seen := make(map[string]struct{}, len(participations))
	for _, p := range participations {
		brokerID := strings.TrimSpace(p.BrokerID)
		if brokerID == "" && p.Role != deal.BrokerRoleImobiliaria {
			return nil, fmt.Errorf("%w: broker_id is required for every participation", ErrInvalidSplitSum)
		}
		if math.IsNaN(p.Percentage) || math.IsInf(p.Percentage, 0) || p.Percentage < 0 || p.Percentage > 100 {
			return nil, fmt.Errorf("%w: percentage must be between 0 and 100 (got %.2f for broker %s)", ErrInvalidSplitSum, p.Percentage, p.BrokerID)
		}
		key := brokerID + "\x1f" + string(p.Role)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate broker and role (%s, %s)", ErrInvalidSplitSum, brokerID, p.Role)
		}
		seen[key] = struct{}{}
		sum += p.Percentage
	}
	if sum < 100-0.01 || sum > 100+0.01 {
		return nil, fmt.Errorf("%w: got %.2f", ErrInvalidSplitSum, sum)
	}

	d, err := s.getDealByScope(ctx, tenantID, dealID, scopeBrokerID)
	if err != nil {
		return nil, err
	}
	// Trava após assinatura — pos-signed o split já gerou Commission
	// entities em toca-financial (via contract.signed handler #414). Mexer
	// aqui desincronizaria UI (mostra novo split) da realidade financeira
	// (Commission entities têm o split antigo). Solução correta seria
	// emitir um evento de "split adjusted" + recriar Commissions, fora
	// de escopo desta primeira rodada.
	if d.IsBrokerSplitLocked() {
		return nil, ErrSplitLockedAfterSigned
	}

	// N14 (reauditoria de 2026-09-20): os ids nomeados na divisão precisam
	// existir NO TENANT. A divisão direciona repasse financeiro, e sem esta
	// conferência ela podia apontar para corretor de outra imobiliária (ou para
	// um id inexistente) — a checagem vem antes de qualquer mutação.
	if err := s.assertBrokersBelongToTenant(ctx, tenantID, participations); err != nil {
		return nil, err
	}

	beforeSnapshot := audit.Snapshot(d.BrokerParticipations)
	d.BrokerParticipations = append([]deal.BrokerParticipation{}, participations...) // defensive copy

	// Atualiza o percentual de comissão do deal se o caller enviou.
	// UI do CommissionSplitEditor sincroniza comissão total com splits.
	if commissionPct != nil {
		prevCommission := d.Commission
		d.Commission = *commissionPct
		d.AddHistory(d.Status, actorID, fmt.Sprintf("Commission updated from %.1f%% to %.1f%%", prevCommission, d.Commission))
	}

	d.UpdatedAt = time.Now()
	d.AddHistory(d.Status, actorID, "Broker participations updated")

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "deal.broker_participations.updated",
		EntityType: "Deal",
		EntityID:   d.ID,
		Before:     map[string]any{"broker_participations": beforeSnapshot},
		After:      map[string]any{"broker_participations": audit.Snapshot(d.BrokerParticipations)},
		Metadata: map[string]any{
			"actor_id":    actorID,
			"actor_email": actorEmail,
			"tenant_id":   tenantID,
			"deal_id":     d.ID,
			"sum":         sum,
		},
	})

	return d, nil
}

// ── Checklist operations ──

func (s *DealService) InitDealChecklist(ctx context.Context, tenantID, dealID, brokerID string, items []deal.ChecklistItem, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	d.InitChecklist(items)
	d.AddHistory(d.Status, userID, "Checklist initialized")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) AddChecklistItem(ctx context.Context, tenantID, dealID, brokerID, name string, category deal.ChecklistItemCategory, required bool, description, partyID string, requestMode string, requestedFields []deal.ChecklistRequestedField, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	d.AddChecklistItemConfigured(name, category, required, description, requestMode, requestedFields)
	if partyID != "" && len(d.Checklist) > 0 {
		d.Checklist[len(d.Checklist)-1].PartyID = partyID
	}
	d.AddHistory(d.Status, userID, "Checklist item added: "+name)
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) RemoveChecklistItem(ctx context.Context, tenantID, dealID, brokerID, itemID, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if d.ChecklistItemByID(itemID) == nil {
		return nil, deal.ErrChecklistItemNotFound
	}
	d.RemoveChecklistItem(itemID)
	d.AddHistory(d.Status, userID, "Checklist item removed")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) UpdateChecklistItemStatus(ctx context.Context, tenantID, dealID, brokerID, itemID string, status deal.ChecklistItemStatus, rejectionReason, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	// Capture the snapshot the audit row needs *before* mutating —
	// d.UpdateChecklistItemStatus rewrites in place. Item name keeps
	// the audit row readable months later when the operator has long
	// forgotten the UUID.
	var (
		oldStatus deal.ChecklistItemStatus
		itemName  string
		isReq     bool
	)
	for _, it := range d.Checklist {
		if it.ID == itemID {
			oldStatus = it.Status
			itemName = it.Name
			isReq = it.Required
			break
		}
	}

	d.UpdateChecklistItemStatus(itemID, status, rejectionReason)
	d.AddHistory(d.Status, userID, "Checklist item status updated to "+string(status))
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	// One audit action per transition so dashboards can count approvals
	// and rejections separately. The generic "checklist.status_changed"
	// is the catch-all.
	action := "checklist.status_changed"
	switch status {
	case deal.ChecklistApproved:
		action = "checklist.approved"
	case deal.ChecklistRejected:
		action = "checklist.rejected"
	case deal.ChecklistUploaded:
		action = "checklist.uploaded"
	}
	audit.Record(ctx, audit.Entry{
		Action:     action,
		EntityType: "ChecklistItem",
		EntityID:   itemID,
		Before:     map[string]any{"status": string(oldStatus)},
		After:      map[string]any{"status": string(status)},
		Metadata: map[string]any{
			"deal_id":          d.ID,
			"item_name":        itemName,
			"required":         isReq,
			"rejection_reason": rejectionReason,
		},
	})

	return d, nil
}

func (s *DealService) SolicitChecklistItem(ctx context.Context, tenantID, dealID, brokerID, itemID, partyID string, requestedFields []deal.ChecklistRequestedField, requestMode string, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	var itemName string
	for _, it := range d.Checklist {
		if it.ID == itemID {
			itemName = it.Name
			break
		}
	}

	if requestedFields != nil && len(requestedFields) > 0 && partyID == "" {
		return nil, errors.New("party_id is required when requesting fields")
	}
	mode := deal.NormalizeRequestMode(requestMode)
	if mode == deal.RequestModeForm && partyID == "" {
		return nil, errors.New("party_id is required for form-only requests")
	}
	d.SolicitChecklistItemScoped(itemID, partyID, requestedFields, string(mode))
	d.AddHistory(d.Status, userID, "Checklist item solicited")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "checklist.solicited",
		EntityType: "ChecklistItem",
		EntityID:   itemID,
		Metadata: map[string]any{
			"deal_id":      d.ID,
			"party_id":     partyID,
			"item_name":    itemName,
			"request_mode": string(mode),
		},
	})

	return d, nil
}

// GetDealChecklist returns checklist items for a deal (used by portal).
//
// Lê pelo repositório do PORTAL (pool de serviço): o cliente é cross-tenant por
// definição e, sob a RLS do pool de aplicação, a leitura voltava vazia —
// a aba de documentos do negócio não carregava.
func (s *DealService) GetDealChecklist(ctx context.Context, tenantID, dealID string) ([]deal.ChecklistItem, error) {
	d, err := s.portal().GetByID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	s.populateChecklistURLs(ctx, d)
	return d.Checklist, nil
}

// GetChecklistItemDownloadURL returns a presigned URL or public URL for a checklist item.
func (s *DealService) GetChecklistItemDownloadURL(ctx context.Context, tenantID, dealID, brokerID, itemID string) (string, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return "", err
	}

	var item *deal.ChecklistItem
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			item = &d.Checklist[i]
			break
		}
	}
	if item == nil {
		return "", errors.New("checklist item not found")
	}

	if item.StorageKey == "" && item.FileURL != "" {
		// Fallback for older items that only have FileURL
		return item.FileURL, nil
	}

	if item.StorageKey == "" {
		return "", errors.New("no file uploaded for this item")
	}

	if s.storage == nil {
		return "", errors.New("storage service not configured")
	}

	return s.storage.GetPresignedURL(ctx, "toca-documents", item.StorageKey, presignedURLTTL)
}

// assertBrokersBelongToTenant recusa a divisão que nomeia corretor fora do
// tenant (N14).
//
// `crossRepo == nil` pula a checagem, mesmo contrato das outras checagens
// cross-tenant do serviço: nos testes o repositório não é injetado e uma
// instalação sem o schema compartilhado não pode derrubar a operação. Falha de
// LEITURA, ao contrário, propaga: sem saber se o corretor existe não se grava
// destinação de repasse.
func (s *DealService) assertBrokersBelongToTenant(ctx context.Context, tenantID string, participations []deal.BrokerParticipation) error {
	if s.crossRepo == nil {
		return nil
	}

	ids := make([]string, 0, len(participations))
	for _, p := range participations {
		if id := strings.TrimSpace(p.BrokerID); id != "" {
			ids = append(ids, id)
		}
	}
	// Divisão só com o papel `imobiliaria` (sem broker_id) não tem id a validar.
	if len(ids) == 0 {
		return nil
	}

	found, err := s.crossRepo.BrokersInTenant(ctx, tenantID, ids)
	if err != nil {
		return fmt.Errorf("validar corretores da divisão de comissão: %w", err)
	}

	var unknown []string
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: %s", ErrBrokerNotInTenant, strings.Join(unknown, ", "))
	}
	return nil
}

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// LeadAcceptanceEvent é publicado quando um corretor aceita ou rejeita um
// lead que estava em PENDING_ACCEPTANCE. Permite que toca-comm dispare
// notificações (admin/gestor) e métricas de SLA atualizem em tempo real.
type LeadAcceptanceEvent struct {
	LeadID     string    `json:"lead_id"`
	TenantID   string    `json:"tenant_id"`
	BrokerID   string    `json:"broker_id"`
	Decision   string    `json:"decision"` // "accepted" | "rejected"
	Reason     string    `json:"reason,omitempty"`
	OldStatus  string    `json:"old_status"`
	NewStatus  string    `json:"new_status"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e LeadAcceptanceEvent) EventID() string {
	return fmt.Sprintf("lead.acceptance-%s-%d", e.LeadID, e.OccurredAt.UnixNano())
}

func (e LeadAcceptanceEvent) Name() string {
	if e.Decision == "rejected" {
		return "lead.acceptance_rejected"
	}
	return "lead.acceptance_accepted"
}

func (e LeadAcceptanceEvent) Payload() interface{} { return e }

// AcceptAssignment é chamado pelo corretor para aceitar formalmente o
// lead que a roleta lhe atribuiu. Pré-condições:
//
//   - Lead deve existir e ter assigned_broker_id == brokerID
//   - Lead.Status deve ser PENDING_ACCEPTANCE (ou NEW se o BDD permitir
//     aceite sem passar pelo SLA — mantemos restrito a PENDING)
//
// Pós-condições:
//
//   - Lead.Status → CONTACTED (próximo estado natural do funil)
//   - Lead.AssignedAt mantém o timestamp original (roleta) — usado pelo
//     SLA Worker para medir "tempo até aceite"
//   - Audit "lead.assignment_accepted" + métrica + evento WS
func (s *LeadService) AcceptAssignment(ctx context.Context, leadID, brokerID string) (*lead.Lead, error) {
	if strings.TrimSpace(leadID) == "" {
		return nil, errors.New("lead_id is required")
	}
	if strings.TrimSpace(brokerID) == "" {
		return nil, errors.New("broker_id is required")
	}

	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, errors.New("lead not found")
	}
	if l.AssignedBrokerID == "" {
		return nil, errors.New("lead is not assigned")
	}
	if l.AssignedBrokerID != brokerID {
		// Não vazar para o corretor se o lead pertence a outro — devolve
		// "not assigned to caller" sem detalhes do owner real.
		return nil, errors.New("lead is not assigned to caller")
	}
	if l.Status != lead.LeadStatusPendingAcceptance {
		return nil, fmt.Errorf("lead status %q does not allow acceptance (expected %q)",
			l.Status, lead.LeadStatusPendingAcceptance)
	}

	oldStatus := l.Status
	l.Status = lead.LeadStatusContacted
	// ExpiresAt zera — o SLA de aceite foi cumprido. SLA de primeiro
	// contato (1h) é responsabilidade de outro worker e usa AssignedAt
	// como referência. Manter AssignedAt para histórico.
	l.ExpiresAt = time.Time{}
	if err := s.repo.Update(ctx, l); err != nil {
		return nil, err
	}

	// Audit (#425) — registra quem aceitou, quando, e a transição.
	audit.Record(ctx, audit.Entry{
		Action:     "lead.assignment_accepted",
		EntityType: "lead",
		EntityID:   l.ID,
		Before:     map[string]any{"status": string(oldStatus)},
		After:      map[string]any{"status": string(l.Status)},
		Metadata: map[string]any{
			"broker_id":   brokerID,
			"assigned_at": l.AssignedAt,
			// SLA de aceite (PENDING_ACCEPTANCE → CONTACTED) — tempo até
			// aceite em segundos, útil para queries de produtividade.
			"acceptance_seconds": int(time.Since(l.AssignedAt).Seconds()),
		},
	})

	metrics.IncLeadStatusChanged(string(oldStatus), string(l.Status))

	// Domain event (#408 — frontend WS). toca-comm propaga para o canal
	// pessoal do broker (lead.assignment_accepted notification) + admins.
	s.publishAsync(ctx, LeadAcceptanceEvent{
		LeadID:     l.ID,
		TenantID:   l.TenantID,
		BrokerID:   brokerID,
		Decision:   "accepted",
		OldStatus:  string(oldStatus),
		NewStatus:  string(l.Status),
		OccurredAt: time.Now(),
	})

	return l, nil
}

// RejectAssignment é chamado pelo corretor para devolver o lead à roleta
// (sem capacity, conflito de interesse, especialidade errada). Reason é
// obrigatório para o admin auditar o pool da roleta.
//
// Comportamento:
//
//   - Lead.Status volta para NEW + assigned_broker_id desatribuído
//   - Roleta worker pega na próxima rodada (excluindo o broker que
//     rejeitou via routing_history.RejectedBrokerIDs)
//   - Audit + métricas + evento publicado
func (s *LeadService) RejectAssignment(ctx context.Context, leadID, brokerID, reason string) (*lead.Lead, error) {
	if strings.TrimSpace(leadID) == "" {
		return nil, errors.New("lead_id is required")
	}
	if strings.TrimSpace(brokerID) == "" {
		return nil, errors.New("broker_id is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("reason is required for reject")
	}

	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, errors.New("lead not found")
	}
	if l.AssignedBrokerID == "" {
		return nil, errors.New("lead is not assigned")
	}
	if l.AssignedBrokerID != brokerID {
		return nil, errors.New("lead is not assigned to caller")
	}
	if l.Status != lead.LeadStatusPendingAcceptance {
		return nil, fmt.Errorf("lead status %q does not allow rejection (expected %q)",
			l.Status, lead.LeadStatusPendingAcceptance)
	}

	oldStatus := l.Status
	previousBroker := l.AssignedBrokerID

	l.Status = lead.LeadStatusNew
	l.AssignedBrokerID = ""
	l.ExpiresAt = time.Time{}
	// AssignedAt fica preservado em histórico via RouletteHistory —
	// não zeramos para evitar perder métrica de tempo da primeira atribuição.
	if err := s.repo.Update(ctx, l); err != nil {
		return nil, err
	}

	// Audit (#426) — registra rejeição com reason para forense.
	audit.Record(ctx, audit.Entry{
		Action:     "lead.assignment_rejected",
		EntityType: "lead",
		EntityID:   l.ID,
		Before: map[string]any{
			"status":             string(oldStatus),
			"assigned_broker_id": previousBroker,
		},
		After: map[string]any{
			"status":             string(l.Status),
			"assigned_broker_id": "",
		},
		Metadata: map[string]any{
			"broker_id":          previousBroker,
			"reason":             reason,
			"acceptance_seconds": int(time.Since(l.AssignedAt).Seconds()),
		},
	})

	metrics.IncLeadStatusChanged(string(oldStatus), string(l.Status))

	// Domain event publicado — toca-comm avisa admins ("lead X rejeitado
	// por Carla — motivo: sem capacity"). Roleta pega lead em
	// AssignUnassignedLeads na próxima rodada.
	s.publishAsync(ctx, LeadAcceptanceEvent{
		LeadID:     l.ID,
		TenantID:   l.TenantID,
		BrokerID:   previousBroker,
		Decision:   "rejected",
		Reason:     reason,
		OldStatus:  string(oldStatus),
		NewStatus:  string(l.Status),
		OccurredAt: time.Now(),
	})

	return l, nil
}

// Garante uso de events package mesmo se publishAsync for trocado.
var _ events.Event = LeadAcceptanceEvent{}

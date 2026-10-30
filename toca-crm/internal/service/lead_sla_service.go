package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// SLAWorkerInterval is how often the SLA worker scans for violations.
// Exposed as a var (not const) so tests can shorten it.
var SLAWorkerInterval = 1 * time.Minute

// FirstContactSLAMinutes — janela do SLA de primeiro contato (Issue
// #427). Default 60min (1h). Var em vez de const para tests poderem
// encurtar; também customizável via LEAD_FIRST_CONTACT_SLA_MINUTES
// no ambiente (lido em main.go do toca-crm).
var FirstContactSLAMinutes = 60

// FirstContactSLABatchLimit — quantos leads o worker processa por
// tick. Limit defensivo: se acumula muito atraso (deploy quebrado por
// 1 dia, p.ex.), processa em batches para não saturar event bus.
var FirstContactSLABatchLimit = int64(50)

type LeadSLAService struct {
	leadRepo     lead.Repository
	rouletteRepo lead.RouletteRepository
	brokerRepo   broker.Repository
	eventBus     events.EventBus
	auditRepo    lead.RouletteAuditRepository
}

func NewLeadSLAService(lRepo lead.Repository, rRepo lead.RouletteRepository, bRepo broker.Repository, eBus events.EventBus, aRepo lead.RouletteAuditRepository) *LeadSLAService {
	return &LeadSLAService{
		leadRepo:     lRepo,
		rouletteRepo: rRepo,
		brokerRepo:   bRepo,
		eventBus:     eBus,
		auditRepo:    aRepo,
	}
}

// RunSLAWorker scans for SLA violations every SLAWorkerInterval and
// triggers escalation/redirect actions. The goroutine exits cleanly when
// ctx is cancelled — the caller is responsible for cancelling on shutdown.
//
// Pre-Wave-P0 this function used context.Background() and a ticker that
// was never stopped, which leaked goroutines on graceful restart and
// silenced every error from CheckSLAViolations. Both are fixed here.
func (s *LeadSLAService) RunSLAWorker(ctx context.Context) {
	ticker := time.NewTicker(SLAWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.CheckSLAViolations(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("sla.check_violations failed", "error", err.Error())
			}
		}
	}
}

func (s *LeadSLAService) CheckSLAViolations(ctx context.Context) error {
	tenantIDs, err := s.rouletteRepo.ListEnabledTenantIDs(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, tenantID := range tenantIDs {
		tenantCtx := pkgctx.WithTenantID(ctx, tenantID)
		cfg, err := s.rouletteRepo.GetConfig(tenantCtx, tenantID)
		if err != nil || cfg == nil || !cfg.Enabled {
			continue
		}

		for _, tier := range cfg.SLATiers {
			cutoff := now.Add(-time.Duration(tier.TimeLimitMin) * time.Minute)

			// Find expired leads for this tier that haven't exhausted retries
			leads, err := s.leadRepo.ListForSLARedirect(tenantCtx, tenantID, []string{string(lead.LeadStatusPendingAcceptance)}, tier.MaxRetries, cutoff, 50)
			if err != nil {
				logger.Error("Error finding SLA violations", "tenant_id", tenantID, "error", err.Error())
				continue
			}

			for _, l := range leads {
				l.RouletteRetries++

				if l.RouletteRetries >= tier.MaxRetries {
					// Max retries reached, trigger escalation
					l.Status = lead.LeadStatusNew
					l.AssignedBrokerID = ""
					l.ExpiresAt = time.Time{}
					_ = s.leadRepo.Update(tenantCtx, l)

					s.triggerEscalation(tenantCtx, l, cfg)
				} else {
					// Not max retries, unassign and send back to roulette
					if tier.Actions.AutoPauseBroker {
						if s.brokerRepo != nil && l.AssignedBrokerID != "" {
							// Wave P1.7 — pausa com cooldown. Cooldown vem do tier
							// (em minutos); zero/negativo cai no default do
							// domínio (4h) para evitar pausa indefinida.
							cooldown := time.Duration(tier.Actions.AutoPauseCooldownMin) * time.Minute
							if cooldown <= 0 {
								cooldown = broker.DefaultBrokerPauseCooldown
							}
							pausedUntil := time.Now().Add(cooldown)
							if ok, _ := s.brokerRepo.SetPausedUntil(tenantCtx, l.AssignedBrokerID, tenantID, true, &pausedUntil); ok {
								if s.auditRepo != nil {
									_ = s.auditRepo.Create(tenantCtx, &lead.RouletteAuditEntry{
										ID:       uuid.New().String(),
										TenantID: tenantID,
										ActorID:  "system",
										Action:   "broker.paused",
										TargetID: l.AssignedBrokerID,
										Diff: map[string]interface{}{
											"reason":       "sla_violation",
											"lead_id":      l.ID,
											"paused_until": pausedUntil.UTC().Format(time.RFC3339),
											"cooldown_min": int(cooldown / time.Minute),
										},
										CreatedAt: time.Now(),
									})
								}
								metrics.IncRouletteSLAAction(tenantID, "broker_paused")
							}
						}
					}

					l.AssignedBrokerID = ""
					l.Status = lead.LeadStatusNew
					l.ExpiresAt = time.Time{}
					_ = s.leadRepo.Update(tenantCtx, l)
					metrics.IncRouletteSLAAction(tenantID, "redirect")

					// Trigger re-assignment via event
					if s.eventBus != nil {
						evt := LeadCreatedEvent{
							LeadID:   l.ID,
							TenantID: l.TenantID,
						}
						if err := s.eventBus.PublishReliable(tenantCtx, evt); err != nil {
							logger.Warn("sla.reassign publish failed",
								"event", evt.Name(),
								"tenant_id", l.TenantID,
								"lead_id", l.ID,
								"broker_id", l.AssignedBrokerID,
								"error", err.Error(),
							)
						}
					}
				}
			}
		}
	}
	return nil
}

func (s *LeadSLAService) triggerEscalation(ctx context.Context, l *lead.Lead, cfg *lead.RouletteConfig) {
	if s.eventBus == nil || len(cfg.Escalation) == 0 {
		return
	}

	maxEscalations := cfg.MaxEscalations
	if maxEscalations <= 0 {
		maxEscalations = lead.DefaultMaxEscalations
	}

	// Already at or above the cap → mark the lead as LOST and stop escalating.
	if l.EscalationLevel >= maxEscalations {
		l.Status = lead.LeadStatusLost
		_ = s.leadRepo.Update(ctx, l)
		if s.auditRepo != nil {
			_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
				ID:        uuid.New().String(),
				TenantID:  l.TenantID,
				ActorID:   "system",
				Action:    "lead.escalation_capped",
				TargetID:  l.ID,
				Diff:      map[string]interface{}{"level": l.EscalationLevel, "max": maxEscalations},
				CreatedAt: time.Now(),
			})
		}
		metrics.IncRouletteSLAAction(l.TenantID, "lost_capped")
		return
	}

	previousLevel := l.EscalationLevel
	step := cfg.Escalation[previousLevel%len(cfg.Escalation)]
	l.EscalationLevel = previousLevel + 1
	_ = s.leadRepo.Update(ctx, l)
	metrics.IncRouletteSLAAction(l.TenantID, "escalate")

	evt := LeadEscalatedEvent{
		LeadID:        l.ID,
		TenantID:      l.TenantID,
		Action:        step.Action,
		Level:         l.EscalationLevel,
		PreviousLevel: previousLevel,
		StepLabel:     step.Label,
		StepID:        step.ID,
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		logger.Warn("sla.escalate publish failed",
			"event", evt.Name(),
			"tenant_id", l.TenantID,
			"lead_id", l.ID,
			"step_id", step.ID,
			"level", l.EscalationLevel,
			"error", err.Error(),
		)
	}

	if s.auditRepo != nil {
		_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
			ID:       uuid.New().String(),
			TenantID: l.TenantID,
			ActorID:  "system",
			Action:   "lead.escalated",
			TargetID: l.ID,
			Diff: map[string]interface{}{
				"previous_level": previousLevel,
				"level":          l.EscalationLevel,
				"step_id":        step.ID,
				"step_label":     step.Label,
				"action":         step.Action,
			},
			CreatedAt: time.Now(),
		})
	}
}

type LeadEscalatedEvent struct {
	LeadID        string `json:"lead_id"`
	TenantID      string `json:"tenant_id"`
	Action        string `json:"action"`
	Level         int    `json:"level"`
	PreviousLevel int    `json:"previous_level"`
	StepLabel     string `json:"step_label"`
	StepID        string `json:"step_id,omitempty"`
}

func (e LeadEscalatedEvent) EventID() string      { return e.LeadID }
func (e LeadEscalatedEvent) Name() string         { return "lead.escalated" }
func (e LeadEscalatedEvent) Payload() interface{} { return e }

// ──────────────────── #427 First Contact SLA ────────────────────

// LeadFirstContactSLAViolatedEvent — disparado pelo worker quando um
// lead atravessa o limite de 1h sem primeiro contato. Consumer no
// toca-comm notifica gestor + corretor responsável via in-app + email
// + (se configurado) WhatsApp.
//
// Idempotência: marca first_contact_sla_violated_at no lead antes de
// publicar; reentries do worker não disparam o evento de novo.
type LeadFirstContactSLAViolatedEvent struct {
	LeadID           string `json:"lead_id"`
	TenantID         string `json:"tenant_id"`
	AssignedBrokerID string `json:"assigned_broker_id,omitempty"`
	AssignedAt       string `json:"assigned_at"` // RFC3339
	ViolatedAt       string `json:"violated_at"` // RFC3339
	MinutesOverdue   int    `json:"minutes_overdue"`
	SLAMinutes       int    `json:"sla_minutes"`
}

func (e LeadFirstContactSLAViolatedEvent) EventID() string { return e.LeadID + ":fc_sla" }
func (e LeadFirstContactSLAViolatedEvent) Name() string {
	return "lead.first_contact_sla_violated"
}
func (e LeadFirstContactSLAViolatedEvent) Payload() interface{} { return e }

// CheckFirstContactSLAViolations — varre todos os tenants enabled e
// detecta leads que cruzaram o SLA de primeiro contato. Marca cada
// lead violador como notificado (first_contact_sla_violated_at) e
// publica evento. Chamado pelo worker a cada SLAWorkerInterval.
//
// Designed para ser independente do CheckSLAViolations existente —
// roda em paralelo no mesmo tick. Se o evento falhar de publicar,
// loga warn mas continua (audit + log estruturado servem de fallback).
func (s *LeadSLAService) CheckFirstContactSLAViolations(ctx context.Context) error {
	tenantIDs, err := s.rouletteRepo.ListEnabledTenantIDs(ctx)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-time.Duration(FirstContactSLAMinutes) * time.Minute)
	for _, tenantID := range tenantIDs {
		tenantCtx := pkgctx.WithTenantID(ctx, tenantID)
		leads, err := s.leadRepo.ListFirstContactSLABreaches(tenantCtx, tenantID, cutoff, FirstContactSLABatchLimit)
		if err != nil {
			logger.Error("sla.first_contact list failed", "tenant_id", tenantID, "error", err.Error())
			continue
		}
		for _, l := range leads {
			// Defesa em profundidade: o filtro primário vive na query
			// ListFirstContactSLABreaches. Guardamos aqui também — um lead
			// já notificado (FirstContactSLAViolatedAt setado) ou nunca
			// atribuído (AssignedAt zero) jamais re-dispara o evento, mesmo
			// que a query regrida. Sem isto, #427 reabre a enxurrada de
			// notificações repetidas com horas de atraso infladas.
			if l.FirstContactSLAViolatedAt != nil || l.AssignedAt.IsZero() {
				continue
			}
			now := time.Now()
			l.FirstContactSLAViolatedAt = &now
			if err := s.leadRepo.Update(tenantCtx, l); err != nil {
				logger.Warn("sla.first_contact update failed",
					"tenant_id", tenantID, "lead_id", l.ID, "error", err.Error())
				continue
			}

			minutesOverdue := int(now.Sub(l.AssignedAt).Minutes()) - FirstContactSLAMinutes
			if minutesOverdue < 0 {
				minutesOverdue = 0
			}
			evt := LeadFirstContactSLAViolatedEvent{
				LeadID:           l.ID,
				TenantID:         l.TenantID,
				AssignedBrokerID: l.AssignedBrokerID,
				AssignedAt:       l.AssignedAt.UTC().Format(time.RFC3339),
				ViolatedAt:       now.UTC().Format(time.RFC3339),
				MinutesOverdue:   minutesOverdue,
				SLAMinutes:       FirstContactSLAMinutes,
			}
			metrics.IncRouletteSLAAction(tenantID, "first_contact_violated")
			if s.eventBus != nil {
				if err := s.eventBus.PublishReliable(tenantCtx, evt); err != nil {
					logger.Warn("sla.first_contact publish failed",
						"event", evt.Name(),
						"tenant_id", l.TenantID,
						"lead_id", l.ID,
						"error", err.Error())
				}
			}
			if s.auditRepo != nil {
				_ = s.auditRepo.Create(tenantCtx, &lead.RouletteAuditEntry{
					ID:       uuid.New().String(),
					TenantID: tenantID,
					ActorID:  "system",
					Action:   "lead.first_contact_sla_violated",
					TargetID: l.ID,
					Diff: map[string]interface{}{
						"sla_minutes":     FirstContactSLAMinutes,
						"minutes_overdue": minutesOverdue,
						"broker_id":       l.AssignedBrokerID,
					},
					CreatedAt: now,
				})
			}
		}
	}
	return nil
}

// RunFirstContactSLAWorker — goroutine paralela ao RunSLAWorker, com
// o mesmo tick. Mantemos separadas as duas verificações (timer da
// roleta vs. SLA primeiro contato) porque o SLA da roleta filtra por
// status PENDING_ACCEPTANCE e tier-aware, enquanto o primeiro contato
// é global e baseado em assigned_at.
func (s *LeadSLAService) RunFirstContactSLAWorker(ctx context.Context) {
	ticker := time.NewTicker(SLAWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.CheckFirstContactSLAViolations(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("sla.first_contact_check failed", "error", err.Error())
			}
		}
	}
}

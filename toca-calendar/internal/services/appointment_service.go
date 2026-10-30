package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

var ErrInspectionManaged = errors.New("Gerencie esta vistoria pelo contrato de origem")

type appointmentEvent struct {
	name    string
	payload interface{}
}

func (e *appointmentEvent) Name() string         { return e.name }
func (e *appointmentEvent) Payload() interface{} { return e.payload }

type AppointmentService struct {
	repo     appointment.Repository
	eventBus events.EventBus
	// links assina o download do .ics enviado por e-mail. Inerte quando o
	// segredo ou a URL pública não estão configurados.
	links *CalendarLink
}

func NewAppointmentService(repo appointment.Repository, eventBus events.EventBus) *AppointmentService {
	return &AppointmentService{
		repo:     repo,
		eventBus: eventBus,
	}
}

// WithCalendarLinks liga o assinador do link de calendário enviado por e-mail.
func (s *AppointmentService) WithCalendarLinks(links *CalendarLink) *AppointmentService {
	s.links = links
	return s
}

func (s *AppointmentService) Create(ctx context.Context, appt *appointment.Appointment) error {
	if err := appt.Validate(nil); err != nil {
		return err
	}
	if appt.AgentID != "" && appt.Date != "" && appt.Time != "" && appt.EndTime != "" {
		conflicts, err := s.repo.FindConflicts(ctx, appt.TenantID, appt.AgentID, appt.Date, appt.Time, appt.EndTime, "")
		if err != nil {
			return fmt.Errorf("failed to check conflicts: %w", err)
		}
		if len(conflicts) > 0 {
			return ErrConflict
		}
	}

	if err := s.repo.Create(ctx, appt); err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	// Auditoria central: criação de agenda/visita. Sem PII — IDs e horário
	// apenas (description pode carregar nome de cliente e fica de fora).
	audit.Record(ctx, audit.Entry{
		Action:     "appointment.created",
		EntityType: "Appointment",
		EntityID:   appt.ID,
		After: audit.Snapshot(appt,
			"date", "time", "end_time", "status", "agent_id", "lead_id",
			"customer_account_id", "property_id", "deal_id"),
	})

	appt.CalendarURL = s.CalendarURL(ctx, appt)
	s.publishEvent(ctx, "appointment.created", appt)
	return nil
}

func (s *AppointmentService) GetByID(ctx context.Context, id string) (*appointment.Appointment, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if appt == nil {
		return nil, ErrNotFound
	}
	return appt, nil
}

// CreateIdempotent lets internal callers recover a lost response without duplicating visits.
func (s *AppointmentService) CreateIdempotent(ctx context.Context, appt *appointment.Appointment) error {
	if err := appt.Validate(nil); err != nil {
		return err
	}
	reuse := func() (bool, error) {
		existing, err := s.GetByID(ctx, appt.ID)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if existing.TenantID != appt.TenantID || existing.LeadID != appt.LeadID || existing.CustomerAccountID != appt.CustomerAccountID {
			return false, fmt.Errorf("appointment identity mismatch")
		}
		if existing.Date != appt.Date || existing.Time != appt.Time || existing.EndTime != appt.EndTime || existing.PropertyID != appt.PropertyID || existing.Description != appt.Description || existing.AgentID != appt.AgentID {
			return false, appointment.ErrConcurrentModification
		}
		*appt = *existing
		return true, nil
	}
	if found, err := reuse(); found || err != nil {
		return err
	}
	if err := s.Create(ctx, appt); err != nil {
		// A parallel retry may have inserted the same UUID in the meantime.
		if found, reuseErr := reuse(); reuseErr != nil {
			return reuseErr
		} else if found {
			return nil
		}
		return err
	}
	return nil
}

func (s *AppointmentService) Update(ctx context.Context, appt *appointment.Appointment) error {
	existing, err := s.repo.GetByID(ctx, appt.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("appointment not found")
	}
	if existing.InspectionID != "" {
		return ErrInspectionManaged
	}
	if !appt.UpdatedAt.Equal(existing.UpdatedAt) {
		return appointment.ErrConcurrentModification
	}
	if err := appt.Validate(existing); err != nil {
		return err
	}

	appt.TenantID = existing.TenantID
	appt.CreatedAt = existing.CreatedAt
	if existing.CustomerAccountID != "" {
		appt.CustomerAccountID = existing.CustomerAccountID
	}
	if existing.CustomerAccountID != "" {
		appt.DealID = existing.DealID
	}
	if existing.CustomerAccountID != "" {
		appt.LeadID = existing.LeadID
	}

	agentChanged := appt.AgentID != existing.AgentID
	dateChanged := appt.Date != existing.Date
	timeChanged := appt.Time != existing.Time || appt.EndTime != existing.EndTime
	if existing.CustomerAccountID != "" && (dateChanged || timeChanged) && appt.Status == appointment.StatusConfirmado && existing.Status == appointment.StatusConfirmado {
		appt.Status = appointment.StatusReagendado
	}

	if appt.Status != appointment.StatusCancelado && (agentChanged || dateChanged || timeChanged || existing.Status == appointment.StatusCancelado) && appt.AgentID != "" && appt.Date != "" && appt.Time != "" && appt.EndTime != "" {
		conflicts, err := s.repo.FindConflicts(ctx, appt.TenantID, appt.AgentID, appt.Date, appt.Time, appt.EndTime, appt.ID)
		if err != nil {
			return fmt.Errorf("failed to check conflicts: %w", err)
		}
		if len(conflicts) > 0 {
			return ErrConflict
		}
	}

	statusChanged := appt.Status != existing.Status

	if err := s.repo.Update(ctx, appt); err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	// Auditoria central: quando o status mudou, o evento é de transição
	// (before→after); senão, é o diff de agenda (data/hora/agente).
	if statusChanged {
		audit.Record(ctx, audit.Entry{
			Action:     "appointment.status_changed",
			EntityType: "Appointment",
			EntityID:   appt.ID,
			Before:     audit.Snapshot(existing, "status"),
			After:      audit.Snapshot(appt, "status"),
		})
	} else if agentChanged || dateChanged || timeChanged {
		audit.Record(ctx, audit.Entry{
			Action:     "appointment.updated",
			EntityType: "Appointment",
			EntityID:   appt.ID,
			Before:     audit.Snapshot(existing, "date", "time", "end_time", "agent_id"),
			After:      audit.Snapshot(appt, "date", "time", "end_time", "agent_id"),
		})
	}

	if s.eventBus != nil {
		evt := contracts.AppointmentUpdatedEvent{
			Data: contracts.AppointmentUpdatedEventPayload{
				AppointmentID:     appt.ID,
				DealID:            appt.DealID,
				TenantID:          appt.TenantID,
				Status:            string(appt.Status),
				Date:              appt.Date,
				Time:              appt.Time,
				EndTime:           appt.EndTime,
				LeadID:            appt.LeadID,
				CustomerAccountID: appt.CustomerAccountID,
				// Beta de e-mail transacional (F2): corretor responsável e
				// título do compromisso para notificação por e-mail.
				AgentID: appt.AgentID,
				Title:   appt.Title,
				// Link do .ics: só quando o compromisso é um horário marcado.
				CalendarURL: s.CalendarURL(ctx, appt),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue appointment.updated event for appointment=%s: %v", appt.ID, err)
		}
	}
	if statusChanged {
		s.publishEvent(ctx, "appointment.status_changed", map[string]interface{}{
			"id":         appt.ID,
			"tenant_id":  appt.TenantID,
			"old_status": string(existing.Status),
			"new_status": string(appt.Status),
		})
	}

	return nil
}

func (s *AppointmentService) Delete(ctx context.Context, id, tenantID string) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("appointment not found")
	}
	if existing.InspectionID != "" {
		return ErrInspectionManaged
	}
	if existing.TenantID != tenantID {
		return fmt.Errorf("access denied")
	}

	// P2/item 36: o tenant desce até a escrita (o `existing.TenantID != tenantID`
	// acima continua valendo como checagem de autorização, mas a sentença também
	// é escopada).
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}

	audit.Record(ctx, audit.Entry{
		Action:     "appointment.deleted",
		EntityType: "Appointment",
		EntityID:   existing.ID,
		Before: audit.Snapshot(existing,
			"date", "time", "end_time", "status", "agent_id", "lead_id",
			"customer_account_id", "property_id", "deal_id"),
	})

	if s.eventBus != nil {
		evt := contracts.AppointmentCancelledEvent{
			Data: contracts.AppointmentCancelledEventPayload{
				AppointmentID:     existing.ID,
				DealID:            existing.DealID,
				TenantID:          existing.TenantID,
				Reason:            "",
				CustomerAccountID: existing.CustomerAccountID,
				// Beta de e-mail transacional (F2): corretor, título e
				// horário original para o e-mail de cancelamento.
				AgentID: existing.AgentID,
				Title:   existing.Title,
				Date:    existing.Date,
				Time:    existing.Time,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue appointment.cancelled event for appointment=%s: %v", existing.ID, err)
		}
	}
	return nil
}

func (s *AppointmentService) List(ctx context.Context, tenantID string, filters appointment.ListFilters) ([]*appointment.Appointment, int64, error) {
	return s.repo.List(ctx, tenantID, filters)
}

func (s *AppointmentService) publishEvent(ctx context.Context, name string, payload interface{}) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.PublishReliable(ctx, &appointmentEvent{name: name, payload: payload}); err != nil {
		log.Printf("WARN: failed to enqueue %s event: %v", name, err)
	}
}

var ErrConflict = appointment.ErrConflict
var ErrNotFound = errors.New("appointment not found")

func (e *appointmentEvent) EventID() string {
	return ""
}

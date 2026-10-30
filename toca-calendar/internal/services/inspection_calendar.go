package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"time"
	_ "time/tzdata"
)

// The inspection owns this appointment. Its deterministic ID and source version
// make at-least-once delivery safe; repository optimistic locking handles races.
func NewInspectionCalendarHandler(repo appointment.Repository) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		data, err := json.Marshal(event.Payload())
		if err != nil {
			return err
		}
		var p contracts.InspectionCalendarPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		if p.InspectionID == "" || p.TenantID == "" || p.ScheduledDate.IsZero() || p.UpdatedAt.IsZero() {
			return fmt.Errorf("invalid inspection calendar snapshot")
		}
		a, err := inspectionAppointment(p)
		if err != nil {
			return err
		}
		current, err := repo.GetByID(ctx, a.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current == nil {
			return repo.Create(ctx, a)
		}
		if current.TenantID != p.TenantID || current.InspectionID != p.InspectionID {
			return fmt.Errorf("inspection calendar identity mismatch")
		}
		if !current.SourceUpdatedAt.Before(p.UpdatedAt) {
			return nil
		}
		a.CreatedAt = current.CreatedAt
		a.UpdatedAt = current.UpdatedAt
		return repo.Update(ctx, a)
	}
}

func inspectionAppointment(p contracts.InspectionCalendarPayload) (*appointment.Appointment, error) {
	zone := p.TimeZone
	if zone == "" {
		zone = "America/Sao_Paulo"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	start := p.ScheduledDate.In(loc)
	end := start.Add(time.Hour)
	endTime := end.Format("15:04")
	// Calendar slots belong to a single civil day.
	if end.Format("2006-01-02") != start.Format("2006-01-02") {
		endTime = "23:59"
	}
	kind := map[string]string{"ENTRY": "entrada", "EXIT": "saída", "PERIODIC": "periódica"}[p.Type]
	if kind == "" {
		kind = "agendada"
	}
	a := appointment.New(p.TenantID, "Vistoria de "+kind, start.Format("2006-01-02"), start.Format("15:04"), endTime, appointment.TypeVistoria, appointment.PriorityMedia, "", "")
	a.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("inspection-calendar:"+p.TenantID+":"+p.InspectionID)).String()
	a.PropertyID = p.PropertyID
	a.AgentID = p.ScheduledBy
	a.Visibility = appointment.VisibilityTeam
	a.InspectionID = p.InspectionID
	a.InspectionContractID = p.ContractID
	a.InspectorID = p.InspectorID
	a.InspectorName = p.InspectorName
	a.SourceUpdatedAt = p.UpdatedAt
	a.Description = "Vistoriador: " + p.InspectorName + ". Gerencie o agendamento nas vistorias do contrato."
	switch p.Status {
	case "Agendada":
		a.Status = appointment.StatusAgendado
	case "Em Andamento":
		a.Status = appointment.StatusEmAndamento
	case "Concluída":
		a.Status = appointment.StatusConcluido
	case "Cancelada":
		a.Status = appointment.StatusCancelado
	default:
		return nil, fmt.Errorf("invalid inspection status %q", p.Status)
	}
	return a, nil
}

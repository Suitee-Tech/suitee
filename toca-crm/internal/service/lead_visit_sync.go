package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// appointmentCreatedPayload espelha os campos publicados por toca-calendar no
// evento `appointment.created` (payload = appointment.Appointment serializado).
type appointmentCreatedPayload struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Type         string `json:"type"`
	LeadID       string `json:"lead_id"`
	InspectionID string `json:"inspection_id"`
	Date         string `json:"date"`
	Time         string `json:"time"`
	EndTime      string `json:"end_time"`
	PropertyID   string `json:"property_id"`
	Description  string `json:"description"`
	Status       string `json:"status"`
}

// NewAppointmentVisitSyncHandler adota, no lead, a visita que o OPERADOR
// agendou direto na agenda da imobiliária.
//
// Contexto do bug: o portal do cliente só enxerga a visita quando o lead tem
// `visit_request` — campo que só existia quando o PRÓPRIO cliente solicitava
// (fluxo que exige imóvel recomendado) ou quando havia negócio/contrato. Uma
// visita criada pelo operador na agenda (toca-calendar, type=Visita,
// lead_id=<lead>) ficava invisível na home do portal porque ninguém escrevia o
// visit_request.
//
// Regras:
//   - só age em appointments de tipo "Visita" vinculados a um lead;
//   - vistorias de contrato (inspection_id) não são visita de lead;
//   - nunca sobrescreve uma visita ABERTA do cliente que aponte para outro
//     appointment (o agendamento do operador só entra quando o lead não tem
//     visita, quando a que tem já foi encerrada, ou quando é o mesmo
//     appointment — redelivery/reconciliação);
//   - idempotente: visit_request idêntico não é regravado e redeliveries
//     convergem sem efeito.
func NewAppointmentVisitSyncHandler(repo lead.Repository) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		var appt appointmentCreatedPayload
		raw, err := json.Marshal(event.Payload())
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &appt); err != nil {
			return err
		}
		if appt.ID == "" || appt.TenantID == "" || appt.LeadID == "" {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(appt.Type), "Visita") {
			return nil
		}
		if appt.InspectionID != "" {
			return nil
		}

		l, err := repo.GetByIDAnyTenant(ctx, appt.LeadID)
		if err != nil {
			return err
		}
		if l == nil {
			return nil
		}
		// O appointment pertence ao tenant do lead; qualquer outra combinação
		// é anomalia e não deve mover o visit_request.
		if appt.TenantID != l.TenantID {
			return nil
		}
		if l.VisitRequest != nil && !visitClosed(l.VisitRequest) &&
			l.VisitRequest.AppointmentID != "" && l.VisitRequest.AppointmentID != appt.ID {
			// Visita aberta do cliente com outro appointment: não sobrescrever.
			return nil
		}

		status := appt.Status
		if status == "Pendente" || status == "Agendado" {
			status = "Aguardando"
		}
		next := &lead.VisitRequest{
			AppointmentID: appt.ID,
			PropertyID:    appt.PropertyID,
			Date:          appt.Date,
			Time:          appt.Time,
			EndTime:       appt.EndTime,
			Note:          appt.Description,
			Status:        status,
		}
		if reflect.DeepEqual(l.VisitRequest, next) {
			return nil
		}

		// Optimistic lock com retry curto (mesmo estilo de beginVisitOperation):
		// entre a leitura e o Patch outro writer pode ter mexido no lead.
		for attempt := 0; attempt < 3; attempt++ {
			saved, ok, perr := repo.Patch(ctx, l.ID, l.TenantID, leadRevision(l), map[string]interface{}{"visit_request": next})
			if perr != nil {
				return perr
			}
			if ok {
				if saved != nil {
					l = saved
				}
				break
			}
			current, gerr := repo.GetByIDAnyTenant(ctx, appt.LeadID)
			if gerr != nil {
				return gerr
			}
			if current == nil {
				return nil
			}
			if current.VisitRequest != nil && !visitClosed(current.VisitRequest) &&
				current.VisitRequest.AppointmentID != "" && current.VisitRequest.AppointmentID != appt.ID {
				return nil
			}
			if reflect.DeepEqual(current.VisitRequest, next) {
				return nil
			}
			l = current
		}
		return nil
	}
}

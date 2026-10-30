package service

import (
	"context"
	"errors"
	"log"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func leadRevision(l *lead.Lead) *lead.LeadExpectedVersion {
	return &lead.LeadExpectedVersion{UpdatedSeq: &l.UpdatedSeq, UpdatedAt: &l.UpdatedAt}
}

// visitOperationRepo é a superfície que o repositório de leads precisa expor
// para as escritas de visita rodarem no POOL DE SERVIÇO. Definida aqui (e não
// na interface de domínio) para não obrigar todos os fakes de teste a
// implementá-la — mesmo padrão já usado por ListPendingVisitOperations.
type visitOperationRepo interface {
	PatchService(context.Context, string, string, *lead.LeadExpectedVersion, map[string]interface{}) (*lead.Lead, bool, error)
}

// patchVisitLead escreve o estado da visita pelo pool de serviço.
//
// Por quê: o portal do cliente é cross-tenant por definição (o tenant do token
// do cliente é a conta local dele, não a imobiliária dona do lead) e o pool de
// aplicação é sujeito a RLS — o UPDATE não encontrava a linha e o cliente não
// conseguia solicitar, reagendar nem cancelar visita. A autorização continua
// sendo feita ANTES (posse do lead por account_id/e-mail, em visitLead) e o
// WHERE do patch segue carregando o tenant do lead.
func (s *LeadService) patchVisitLead(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	if sr, ok := s.repo.(visitOperationRepo); ok {
		return sr.PatchService(ctx, leadID, tenantID, expected, updates)
	}
	return s.repo.Patch(ctx, leadID, tenantID, expected, updates)
}

// getLeadViaService lê o lead pelos caminhos que NÃO podem depender do tenant
// do chamador: o portal do cliente (tenant = conta local do cliente, não a
// imobiliária dona do lead) e as máquinas de visita. A leitura sai pelo pool de
// serviço e o tenant do lead é conferido aqui (fail-closed).
func (s *LeadService) getLeadViaService(ctx context.Context, leadID, tenantID string) (*lead.Lead, error) {
	l, err := s.repo.GetByIDAnyTenant(ctx, leadID)
	if err != nil || l == nil {
		return nil, err
	}
	if tenantID != "" && l.TenantID != tenantID {
		return nil, nil
	}
	return l, nil
}

// Reserve the operation before HTTP. Its UUID, payload and prior CRM state
// survive a lost response or process restart. Only the lease holder executes it.
func (s *LeadService) beginVisitOperation(ctx context.Context, before *lead.Lead, desired *lead.VisitRequest, create bool) error {
	ctx = pkgcontext.WithTenantID(ctx, before.TenantID)
	if before.VisitOperation != nil {
		if err := s.recoverVisitOperation(ctx, before); err != nil {
			return err
		}
		if !sameVisitIntent(before.VisitOperation.Desired, desired) {
			return ErrVisitConflict
		}
		return nil
	}
	if create {
		copy := *desired
		copy.AppointmentID = uuid.NewString()
		desired = &copy
	}
	customerID, _ := pkgcontext.GetAccountID(ctx)
	op := &lead.VisitOperation{CustomerAccountID: customerID, ID: uuid.NewString(), Token: uuid.NewString(), Desired: desired, Before: before.VisitRequest,
		BrokerID: before.AssignedBrokerID, AccountID: before.AccountID, Create: create, LeaseUntil: time.Now().Add(2 * time.Minute)}
	if !create {
		prior, err := s.calendarClient.GetAppointment(ctx, desired.AppointmentID, before.TenantID)
		if err != nil {
			return visitAgendaError(err)
		}
		if prior == nil || prior.TenantID != before.TenantID || prior.LeadID != before.ID {
			return ErrVisitForbidden
		}
		op.ExpectedAgendaUpdatedAt = &prior.UpdatedAt
	}
	for attempt := 0; attempt < 3; attempt++ {
		saved, ok, err := s.patchVisitLead(ctx, before.ID, before.TenantID, leadRevision(before), map[string]interface{}{"visit_operation": op})
		if err != nil {
			return err
		}
		if ok {
			return s.executeVisitOperation(ctx, saved, op)
		}
		current, err := s.repo.GetByID(ctx, before.TenantID, before.ID)
		if err != nil {
			return err
		}
		if current == nil {
			return ErrVisitNotFound
		}
		if current.VisitOperation != nil || current.AccountID != before.AccountID || current.AssignedBrokerID != before.AssignedBrokerID || !reflect.DeepEqual(current.VisitRequest, before.VisitRequest) {
			return ErrVisitConflict
		}
		before = current
	}
	return ErrVisitConflict
}

func (s *LeadService) recoverVisitOperation(ctx context.Context, l *lead.Lead) error {
	ctx = pkgcontext.WithTenantID(ctx, l.TenantID)
	if l.VisitOperation == nil {
		return nil
	}
	if l.VisitOperation.LeaseUntil.After(time.Now()) {
		return ErrVisitConflict
	}
	op := *l.VisitOperation
	op.Token, op.LeaseUntil = uuid.NewString(), time.Now().Add(2*time.Minute)
	saved, ok, err := s.patchVisitLead(ctx, l.ID, l.TenantID, leadRevision(l), map[string]interface{}{"visit_operation": &op})
	if err != nil {
		return err
	}
	if !ok {
		return ErrVisitConflict
	}
	return s.executeVisitOperation(ctx, saved, &op)
}

func (s *LeadService) executeVisitOperation(ctx context.Context, l *lead.Lead, op *lead.VisitOperation) error {
	ctx = pkgcontext.WithTenantID(ctx, l.TenantID)
	if l.AccountID != op.AccountID || l.AssignedBrokerID != op.BrokerID || !reflect.DeepEqual(l.VisitRequest, op.Before) {
		return s.finishVisitOperation(ctx, l, op, nil, true)
	}
	desired := op.Desired
	if desired == nil || desired.AppointmentID == "" {
		return ErrVisitInvalid
	}
	var appt *clients.Appointment
	var err error
	if op.Create {
		req, buildErr := s.buildCalendarVisit(ctx, l, desired.PropertyID, desired.Date, desired.Time, VisitDetails{EndTime: desired.EndTime, Note: desired.Note})
		if buildErr != nil {
			return buildErr
		}
		req.ID = desired.AppointmentID
		if req.CustomerAccountID == "" {
			req.CustomerAccountID = op.CustomerAccountID
		}
		current, getErr := s.calendarClient.GetAppointment(ctx, req.ID, l.TenantID)
		if getErr == nil && current != nil {
			if current.TenantID != l.TenantID || current.LeadID != l.ID || current.CustomerAccountID != req.CustomerAccountID {
				err = ErrVisitForbidden
			} else {
				appt = current
			}
		} else if errors.Is(getErr, clients.ErrCalendarNotFound) {
			appt, err = s.calendarClient.CreateAppointment(ctx, req)
		} else {
			err = getErr
		}
	} else {
		req := clients.UpdateAppointmentRequest{TenantID: l.TenantID, Status: desired.Status, ExpectedUpdatedAt: op.ExpectedAgendaUpdatedAt}
		if desired.Status == "Aguardando" {
			req.Date, req.Time, req.EndTime, req.PropertyID = desired.Date, desired.Time, desired.EndTime, desired.PropertyID
			if snapshot := l.RecommendedPropertySnapshot; snapshot != nil && snapshot.ID == desired.PropertyID {
				req.Property = snapshot.Title
			}
			req.Status, req.Description = "Pendente", &desired.Note
		}
		current, getErr := s.calendarClient.GetAppointment(ctx, desired.AppointmentID, l.TenantID)
		if getErr != nil {
			err = getErr
		} else if current == nil || current.TenantID != l.TenantID || current.LeadID != l.ID {
			err = ErrVisitForbidden
		} else if calendarHasIntent(current, desired) {
			appt = current
		} else {
			appt, err = s.calendarClient.UpdateAppointment(ctx, desired.AppointmentID, req)
		}
	}
	if err != nil {
		// Release only our lease, keeping the durable intent for a safe retry.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		current, getErr := s.repo.GetByID(releaseCtx, l.TenantID, l.ID)
		if getErr == nil && current != nil && current.VisitOperation != nil && current.VisitOperation.Token == op.Token {
			released := *op
			released.LeaseUntil = time.Time{}
			if errors.Is(err, clients.ErrCalendarConflict) || errors.Is(err, clients.ErrCalendarChanged) {
				_, _, _ = s.patchVisitLead(releaseCtx, l.ID, l.TenantID, leadRevision(current), map[string]interface{}{"visit_operation": nil})
			} else {
				_, _, _ = s.patchVisitLead(releaseCtx, l.ID, l.TenantID, leadRevision(current), map[string]interface{}{"visit_operation": &released})
			}
		}
		return visitAgendaError(err)
	}
	if appt == nil || appt.ID != desired.AppointmentID || appt.TenantID != l.TenantID || appt.LeadID != l.ID {
		return clients.ErrCalendarUnavailable
	}
	next := &lead.VisitRequest{AppointmentID: appt.ID, PropertyID: appt.PropertyID, Date: appt.Date, Time: appt.Time, EndTime: appt.EndTime, Note: appt.Description, Status: appt.Status}
	if next.Status == "Pendente" || next.Status == "Agendado" {
		next.Status = "Aguardando"
	}
	return s.finishVisitOperation(ctx, l, op, next, false)
}

func (s *LeadService) finishVisitOperation(ctx context.Context, l *lead.Lead, op *lead.VisitOperation, next *lead.VisitRequest, invalid bool) error {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := s.getLeadViaService(ctx, l.ID, l.TenantID)
		if err != nil {
			return err
		}
		if current == nil {
			return ErrVisitNotFound
		}
		if current.VisitOperation == nil {
			if reflect.DeepEqual(current.VisitRequest, next) {
				return nil
			}
			return ErrVisitConflict
		}
		if current.VisitOperation.Token != op.Token {
			return ErrVisitConflict
		}
		invalid = invalid || current.AccountID != op.AccountID || current.AssignedBrokerID != op.BrokerID || !reflect.DeepEqual(current.VisitRequest, op.Before)
		updates := map[string]interface{}{"visit_operation": nil}
		if invalid {
			// A newly created orphan is cancelled, with its history preserved. Until
			// cancellation succeeds the intent remains available to the recovery worker.
			if op.Create {
				appt, getErr := s.calendarClient.GetAppointment(ctx, op.Desired.AppointmentID, l.TenantID)
				if getErr != nil && !errors.Is(getErr, clients.ErrCalendarNotFound) {
					return getErr
				}
				if appt != nil {
					if appt.TenantID != l.TenantID || appt.LeadID != l.ID {
						return ErrVisitForbidden
					}
					if _, err = s.calendarClient.UpdateAppointment(ctx, appt.ID, clients.UpdateAppointmentRequest{TenantID: l.TenantID, Status: "Cancelado"}); err != nil {
						return err
					}
				}
			}
		} else {
			updates["visit_request"] = next
		}
		_, ok, err := s.patchVisitLead(ctx, l.ID, l.TenantID, leadRevision(current), updates)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if invalid {
			return ErrVisitConflict
		}
		if next.Status == "Aguardando" {
			s.publishAsync(ctx, ClientVisitRequestedEvent{LeadID: l.ID, BrokerID: op.BrokerID, TenantID: l.TenantID, ClientName: l.Name, ClientEmail: l.Email, PropertyID: next.PropertyID, Date: next.Date, Time: next.Time, AppointmentID: next.AppointmentID})
		}
		if next.Status == "Confirmado" {
			s.publishAsync(ctx, ClientVisitConfirmedEvent{LeadID: l.ID, BrokerID: op.BrokerID, TenantID: l.TenantID, ClientEmail: l.Email, Date: next.Date, Time: next.Time})
		}
		return nil
	}
	return ErrVisitConflict
}

// Recovery also runs after restart; HTTP requests do not own its lifetime.
func (s *LeadService) RunVisitRecoveryWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		if err := s.recoverPendingVisits(batchCtx); err != nil {
			log.Printf("visit recovery failed: %v", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sameVisitIntent(a, b *lead.VisitRequest) bool {
	if a == nil || b == nil {
		return a == b
	}
	left, right := *a, *b
	left.AppointmentID, right.AppointmentID = "", ""
	return reflect.DeepEqual(left, right)
}

func calendarHasIntent(a *clients.Appointment, v *lead.VisitRequest) bool {
	status := a.Status
	if status == "Pendente" || status == "Agendado" {
		status = "Aguardando"
	}
	if v.Status != "Aguardando" {
		return status == v.Status
	}
	return status == v.Status && a.PropertyID == v.PropertyID && a.Date == v.Date && a.Time == v.Time && a.EndTime == v.EndTime && a.Description == v.Note
}

func (s *LeadService) recoverPendingVisits(ctx context.Context) error {
	source, ok := s.repo.(interface {
		ListPendingVisitOperations(context.Context, int) ([]*lead.Lead, error)
	})
	if !ok {
		return nil
	}
	pending, err := source.ListPendingVisitOperations(ctx, 100)
	if err != nil {
		return err
	}
	var failures []error
	for _, l := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.recoverVisitOperation(ctx, l); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

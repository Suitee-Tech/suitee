package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Keep visit validation working in minimal containers without zoneinfo.
	"unicode/utf8"

	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/domain"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type visitCalendar interface {
	CreateAppointment(context.Context, clients.CreateAppointmentRequest) (*clients.Appointment, error)
	UpdateAppointment(context.Context, string, clients.UpdateAppointmentRequest) (*clients.Appointment, error)
	GetAppointment(context.Context, string, string) (*clients.Appointment, error)
}

type VisitDetails struct {
	EndTime string `json:"end_time"`
	Note    string `json:"note"`
}

var ErrVisitNotFound = errors.New("Visita ou oportunidade não encontrada.")
var ErrVisitForbidden = errors.New("Você não tem acesso a esta oportunidade.")
var ErrVisitInvalid = errors.New("Informe uma data e horários válidos para a visita.")
var ErrVisitClosed = errors.New("Esta visita foi encerrada. Solicite uma nova visita.")

func (s *LeadService) visitLead(ctx context.Context, id string, client bool) (*lead.Lead, error) {
	// SECURITY (achado C9): o caminho do operador é tenant-scoped. O portal do
	// cliente NÃO é — a identidade dele é provada por account id/e-mail logo
	// abaixo, e o tenant de um token OTP de cliente é a própria conta local
	// dele (pode divergir do tenant da empresa dona do lead). Por isso o
	// escape hatch explícito GetByIDAnyTenant.
	var l *lead.Lead
	var err error
	if client {
		l, err = s.repo.GetByIDAnyTenant(ctx, id)
	} else {
		tenantID, terr := tenantFromContext(ctx)
		if terr != nil {
			return nil, ErrVisitNotFound
		}
		l, err = s.repo.GetByID(ctx, tenantID, id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrVisitNotFound
		}
		return nil, err
	}
	if l == nil {
		return nil, ErrVisitNotFound
	}
	if client {
		accountID, _ := pkgcontext.GetAccountID(ctx)
		email, _ := pkgcontext.GetEmail(ctx)
		if accountID == "" || (l.AccountID != accountID && (strings.TrimSpace(email) == "" || !strings.EqualFold(strings.TrimSpace(l.Email), strings.TrimSpace(email)))) {
			return nil, ErrVisitForbidden
		}
	} else {
		tenantID, _ := pkgcontext.GetTenantID(ctx)
		if tenantID == "" || l.TenantID != tenantID {
			return nil, ErrVisitForbidden
		}
	}
	// Do not mutate a repository-owned object before an upstream call succeeds.
	copy := *l
	if l.VisitRequest != nil {
		visit := *l.VisitRequest
		copy.VisitRequest = &visit
	}
	return &copy, nil
}

func visitDetails(loc *time.Location, date, start string, extras []VisitDetails) (VisitDetails, error) {
	details := VisitDetails{}
	if len(extras) > 0 {
		details = extras[0]
	}
	location := loc
	if location == nil {
		location = timeutil.Location()
	}
	begins, err := time.ParseInLocation("2006-01-02 15:04", date+" "+start, location)
	if err != nil || !begins.After(time.Now()) {
		return details, ErrVisitInvalid
	}
	if details.EndTime == "" {
		details.EndTime = begins.Add(time.Hour).Format("15:04")
	}
	ends, err := time.ParseInLocation("2006-01-02 15:04", date+" "+details.EndTime, location)
	if err != nil || !ends.After(begins) || utf8.RuneCountInString(details.Note) > 500 {
		return details, ErrVisitInvalid
	}
	details.Note = strings.TrimSpace(details.Note)
	return details, nil
}

func visitClosed(v *lead.VisitRequest) bool {
	return v != nil && (v.Status == "Cancelado" || v.Status == "Concluido" || v.Status == "Concluído")
}

// refreshVisitFromAgenda makes the agenda authoritative for confirmation,
// rescheduling, completion and cancellation, including missed realtime events.
func (s *LeadService) refreshVisitFromAgenda(ctx context.Context, l *lead.Lead) error {
	if l.VisitRequest == nil || l.VisitRequest.AppointmentID == "" {
		return nil
	}
	if s.calendarClient == nil {
		return clients.ErrCalendarUnavailable
	}
	appt, err := s.calendarClient.GetAppointment(ctx, l.VisitRequest.AppointmentID, l.TenantID)
	if errors.Is(err, clients.ErrCalendarNotFound) {
		v := *l.VisitRequest
		v.Status = "Cancelado"
		l.VisitRequest = &v
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %v", clients.ErrCalendarUnavailable, err)
	}
	if appt == nil || appt.ID != l.VisitRequest.AppointmentID || appt.TenantID != l.TenantID || appt.LeadID != l.ID {
		return clients.ErrCalendarUnavailable
	}
	v := *l.VisitRequest
	v.Date, v.Time, v.EndTime = appt.Date, appt.Time, appt.EndTime
	v.PropertyID = appt.PropertyID
	v.Note = appt.Description
	v.Status = appt.Status
	if v.Status == "Pendente" || v.Status == "Agendado" {
		v.Status = "Aguardando"
	}
	l.VisitRequest = &v
	return nil
}

func (s *LeadService) buildCalendarVisit(ctx context.Context, l *lead.Lead, propertyID, date, start string, details VisitDetails) (clients.CreateAppointmentRequest, error) {
	tenantID, err := domain.NewTenantID(l.TenantID)
	if err != nil {
		return clients.CreateAppointmentRequest{}, err
	}
	propertyLabel := ""
	if snapshot := l.RecommendedPropertySnapshot; snapshot != nil && snapshot.ID == propertyID {
		propertyLabel = snapshot.Title
	}
	agentName := "Corretor responsável"
	if s.brokerRepo != nil && l.AssignedBrokerID != "" {
		// A19: tenant do próprio lead (fonte de verdade), como no overview.
		// Leitura pelo pool de serviço: este caminho também é do portal do
		// cliente, que é cross-tenant por definição.
		if broker := s.lookupBrokerForPortal(ctx, l.TenantID, l.AssignedBrokerID); broker != nil {
			agentName = broker.Name
		}
	}
	return clients.CreateAppointmentRequest{
		TenantID: tenantID, Title: "Visita solicitada · " + l.Name,
		Date: date, Time: start, EndTime: details.EndTime, Type: "Visita", Priority: "Media",
		Client: l.Name, LeadID: l.ID, PropertyID: propertyID, Property: propertyLabel,
		CustomerAccountID: l.AccountID, Agent: agentName, AgentID: l.AssignedBrokerID,
		Status: "Pendente", Description: details.Note,
	}, nil
}

func (s *LeadService) RequestLeadVisit(ctx context.Context, leadID, propertyID, date, start string, extras ...VisitDetails) error {
	l, err := s.visitLead(ctx, leadID, true)
	if err != nil {
		return err
	}
	before := visitSnapshot(l) // Preserve persisted visit/version before hydrating agenda state.
	details, err := visitDetails(timeutil.LocationForTenant(ctx, l.TenantID), date, start, extras)
	if err != nil {
		return err
	}
	if s.calendarClient == nil {
		return clients.ErrCalendarUnavailable
	}
	if err := s.refreshVisitFromAgenda(ctx, l); err != nil {
		return err
	}
	create := l.VisitRequest == nil || l.VisitRequest.AppointmentID == "" || visitClosed(l.VisitRequest)
	if propertyID == "" {
		propertyID = l.RecommendedPropertyID
	}
	// CRIAR visita exige um imóvel (a recomendação do corretor). REAGENDAR uma
	// visita já aberta — inclusive a agendada pelo operador na agenda, que pode
	// não ter imóvel vinculado — não exige: a visita continua sem imóvel.
	if propertyID == "" && create {
		return ErrVisitInvalid
	}
	id := ""
	if !create {
		id = l.VisitRequest.AppointmentID
		if l.VisitRequest.PropertyID == propertyID && l.VisitRequest.Date == date && l.VisitRequest.Time == start && l.VisitRequest.EndTime == details.EndTime && l.VisitRequest.Note == details.Note && before.VisitOperation == nil {
			return nil
		}
	}
	next := &lead.VisitRequest{PropertyID: propertyID, Date: date, Time: start, EndTime: details.EndTime, Note: details.Note, Status: "Aguardando", AppointmentID: id}
	return s.beginVisitOperation(ctx, before, next, create)
}

func visitAgendaError(err error) error {
	if errors.Is(err, clients.ErrCalendarConflict) || errors.Is(err, clients.ErrCalendarChanged) {
		return err
	}
	return fmt.Errorf("%w: %v", clients.ErrCalendarUnavailable, err)
}

func (s *LeadService) UpdateLeadVisit(ctx context.Context, id, date, start string, extras ...VisitDetails) error {
	l, err := s.visitLead(ctx, id, true)
	if err != nil {
		return err
	}
	if err := s.refreshVisitFromAgenda(ctx, l); err != nil {
		return err
	}
	if l.VisitRequest == nil {
		return ErrVisitNotFound
	}
	if visitClosed(l.VisitRequest) {
		return ErrVisitClosed
	}
	if len(extras) == 0 {
		extras = []VisitDetails{{Note: l.VisitRequest.Note}}
	}
	return s.RequestLeadVisit(ctx, id, l.VisitRequest.PropertyID, date, start, extras...)
}

func (s *LeadService) CancelLeadVisit(ctx context.Context, id string) error {
	l, err := s.visitLead(ctx, id, true)
	if err != nil {
		return err
	}
	before := visitSnapshot(l)
	if err := s.refreshVisitFromAgenda(ctx, l); err != nil {
		return err
	}
	if l.VisitRequest == nil {
		return ErrVisitNotFound
	}
	if l.VisitRequest.Status == "Cancelado" {
		return nil
	}
	if visitClosed(l.VisitRequest) {
		return ErrVisitClosed
	}
	l.VisitRequest.Status = "Cancelado"
	return s.beginVisitOperation(ctx, before, l.VisitRequest, false)
}

func (s *LeadService) ConfirmLeadVisit(ctx context.Context, id string) error {
	l, err := s.visitLead(ctx, id, false)
	if err != nil {
		return err
	}
	before := visitSnapshot(l)
	if err := s.refreshVisitFromAgenda(ctx, l); err != nil {
		return err
	}
	if l.VisitRequest == nil {
		return ErrVisitNotFound
	}
	if visitClosed(l.VisitRequest) {
		return ErrVisitClosed
	}
	if l.VisitRequest.AppointmentID == "" {
		return clients.ErrCalendarUnavailable
	}
	l.VisitRequest.Status = "Confirmado"
	return s.beginVisitOperation(ctx, before, l.VisitRequest, false)
}

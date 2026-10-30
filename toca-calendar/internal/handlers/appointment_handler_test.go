package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

func TestInvalidAppointmentWritesReturnBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, internal := range []bool{false, true} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(method+" internal="+fmt.Sprint(internal), func(t *testing.T) {
				repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{"appointment": appointment.New("agency", "Entrega", "2099-10-20", "09:00", "10:00", appointment.TypeEntrega, appointment.PriorityMedia, "Cliente", "Corretor")}}
				repo.items["appointment"].ID = "appointment"
				repo.items["appointment"].AgentID = "acc-1"
				h := NewAppointmentHandler(services.NewAppointmentService(repo, nil))
				r := gin.New()
				r.Use(func(c *gin.Context) {
					ctx := pkgcontext.WithTenantID(c.Request.Context(), "agency")
					c.Request = c.Request.WithContext(pkgcontext.WithAccountID(ctx, "acc-1"))
					c.Next()
				})
				if internal {
					// P1/A3: a rota interna resolve o tenant pela credencial
					// (InternalOrUserMiddleware em produção).
					r.Use(internalServiceIdentity())
					r.POST("/appointments", h.InternalCreate)
					r.PUT("/appointments/:id", h.InternalUpdate)
				} else {
					r.POST("/appointments", h.Create)
					r.PUT("/appointments/:id", h.Update)
				}
				path := "/appointments"
				if method == http.MethodPut {
					path += "/appointment"
				}
				req := httptest.NewRequest(method, path, strings.NewReader(`{"tenant_id":"agency","title":"Entrega","date":"2099-02-30","time":"09:00","end_time":"10:00","type":"Entrega","client":"Cliente","agent":"Corretor"}`))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("got %d: %s", w.Code, w.Body.String())
				}
				if len(repo.items) != 1 || repo.items["appointment"].Date != "2099-10-20" {
					t.Fatal("invalid write changed records")
				}
			})
		}
	}
}

type visitHandlerRepo struct {
	appointment.Repository
	items    map[string]*appointment.Appointment
	conflict bool
	reads    int
	onRead   func(int)
}

func (r *visitHandlerRepo) Create(_ context.Context, a *appointment.Appointment) error {
	copy := *a
	r.items[a.ID] = &copy
	return nil
}
func (r *visitHandlerRepo) GetByIDForLink(ctx context.Context, id string) (*appointment.Appointment, error) {
	return r.GetByID(ctx, id)
}

func (r *visitHandlerRepo) GetByID(_ context.Context, id string) (*appointment.Appointment, error) {
	r.reads++
	if r.onRead != nil {
		r.onRead(r.reads)
	}
	if a := r.items[id]; a != nil {
		copy := *a
		return &copy, nil
	}
	return nil, nil
}
func (r *visitHandlerRepo) Update(_ context.Context, a *appointment.Appointment) error {
	copy := *a
	r.items[a.ID] = &copy
	return nil
}
func (r *visitHandlerRepo) FindConflicts(_ context.Context, _, _, _, _, _, _ string) ([]*appointment.Appointment, error) {
	if r.conflict {
		return []*appointment.Appointment{{ID: "other"}}, nil
	}
	return nil, nil
}

func TestInternalVisitEndpointsPreserveStatusDurationAndIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{}}
	handler := NewAppointmentHandler(services.NewAppointmentService(repo, nil))
	router := gin.New()
	// P1/A3: as rotas internas resolvem o tenant pela credencial; aqui injetamos
	// a identidade de serviço que o InternalOrUserMiddleware deixaria no contexto.
	router.Use(internalServiceIdentity())
	router.POST("/appointments", handler.InternalCreate)
	router.GET("/appointments/:id", handler.InternalGetByID)
	router.PUT("/appointments/:id", handler.InternalUpdate)
	request := func(method, path, body string, expected int) *httptest.ResponseRecorder {
		t.Helper()
		result := httptest.NewRecorder()
		if method == http.MethodPut && strings.HasPrefix(body, "{") {
			body = strings.Replace(body, "{", `{"tenant_id":"agency",`, 1)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(result, req)
		if result.Code != expected {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, result.Code, expected, result.Body.String())
		}
		return result
	}
	id := "11111111-1111-4111-8111-111111111111"
	body := `{"id":"` + id + `","tenant_id":"agency","title":"Visita","date":"2099-10-20","time":"14:00","end_time":"15:30","type":"Visita","client":"Lucas","agent":"Corretor","agent_id":"broker","lead_id":"lead","customer_account_id":"client","property_id":"property","status":"Pendente","description":"Garagem"}`
	request(http.MethodPost, "/appointments", body, http.StatusCreated)
	if repo.items[id].Status != appointment.StatusPendente || repo.items[id].EndTime != "15:30" || repo.items[id].CustomerAccountID != "client" {
		t.Fatal("portal details/status were ignored on creation")
	}
	request(http.MethodPost, "/appointments", body, http.StatusCreated)
	if len(repo.items) != 1 {
		t.Fatal("replayed request duplicated the appointment")
	}
	request(http.MethodPut, "/appointments/"+id, `{"status":"Confirmado"}`, http.StatusOK)
	request(http.MethodPut, "/appointments/"+id, `{"date":"2099-10-21","time":"16:00","end_time":"17:30","property_id":"new-property","description":""}`, http.StatusOK)
	if repo.items[id].Status != appointment.StatusReagendado || repo.items[id].EndTime != "17:30" || repo.items[id].PropertyID != "new-property" || repo.items[id].Description != "" {
		t.Fatal("rescheduling or clearing note was not applied")
	}
	result := request(http.MethodGet, "/appointments/"+id+"?tenant_id=agency", "", http.StatusOK)
	var response struct {
		Data appointment.Appointment `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil || response.Data.Time != "16:00" {
		t.Fatalf("invalid authoritative visit read: %v", err)
	}
	request(http.MethodGet, "/appointments/"+id+"?tenant_id=other", "", http.StatusForbidden)
	// Sem tenant explícito o resolver recusa como validação (400) — antes o
	// handler devolvia 403 "tenant context required".
	request(http.MethodGet, "/appointments/"+id, "", http.StatusBadRequest)
	repo.conflict = true
	request(http.MethodPut, "/appointments/"+id, `{"time":"18:00","end_time":"19:00"}`, http.StatusConflict)
	if repo.items[id].Time != "16:00" {
		t.Fatal("conflicting update changed persisted appointment")
	}
	request(http.MethodPut, "/appointments/"+id, `{"status":"Cancelado"}`, http.StatusOK)
	if repo.items[id].Status != appointment.StatusCancelado {
		t.Fatal("cancellation lost visit history")
	}
}

func TestCalendarConcurrentStatusPatchDoesNotOverwriteTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := appointment.New("agency", "Original", "2099-10-20", "09:00", "10:00", appointment.TypeVisita, appointment.PriorityMedia, "Cliente", "Corretor")
	a.ID = "appointment"
	a.AgentID = "acc-1"
	repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{a.ID: a}}
	repo.onRead = func(n int) {
		if n == 2 {
			repo.items[a.ID].Title = "Editado por outro usuário"
			repo.items[a.ID].UpdatedAt = a.UpdatedAt.Add(time.Second)
		}
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "agency")
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(ctx, "acc-1"))
		c.Next()
	})
	router.PUT("/appointments/:id", NewAppointmentHandler(services.NewAppointmentService(repo, nil)).Update)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/appointments/appointment", strings.NewReader(`{"status":"Confirmado"}`))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)
	if w.Code != 409 || repo.items[a.ID].Title != "Editado por outro usuário" || repo.items[a.ID].Status == appointment.StatusConfirmado {
		t.Fatalf("concurrent edit overwritten: %d %s", w.Code, w.Body.String())
	}
}

func TestCalendarExplicitClearAndDealLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, portal := range []bool{false, true} {
		a := appointment.New("agency", "Visita", "2099-10-20", "09:00", "10:00", appointment.TypeVisita, appointment.PriorityMedia, "Cliente", "Corretor")
		a.ID = "appointment"
		a.AgentID = "acc-1"
		a.PropertyID = "property"
		a.Property = "Imóvel"
		a.DealID = "original"
		if portal {
			a.CustomerAccountID = "client"
			a.LeadID = "lead"
		}
		repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{a.ID: a}}
		router := gin.New()
		router.Use(func(c *gin.Context) {
			ctx := pkgcontext.WithTenantID(c.Request.Context(), "agency")
			c.Request = c.Request.WithContext(pkgcontext.WithAccountID(ctx, "acc-1"))
			c.Next()
		})
		router.PUT("/appointments/:id", NewAppointmentHandler(services.NewAppointmentService(repo, nil)).Update)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/appointments/appointment", strings.NewReader(`{"property_id":"","property":"","deal_id":"selected","lead_id":"other"}`))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		saved := repo.items[a.ID]
		if saved.PropertyID != "" || saved.Property != "" {
			t.Fatal("explicit clear ignored")
		}
		if !portal && saved.DealID != "selected" {
			t.Fatal("selected deal was not persisted")
		}
		if portal && (saved.DealID != "original" || saved.LeadID != "lead" || saved.CustomerAccountID != "client") {
			t.Fatal("portal identity links changed")
		}
	}
}

type portalAppointmentListRepo struct {
	appointment.Repository
	items   []*appointment.Appointment
	calls   []appointment.ListFilters
	tenants []string
	err     error
}

func (r *portalAppointmentListRepo) List(_ context.Context, tenant string, filter appointment.ListFilters) ([]*appointment.Appointment, int64, error) {
	r.calls = append(r.calls, filter)
	r.tenants = append(r.tenants, tenant)
	if r.err != nil {
		return nil, 0, r.err
	}
	matches := []*appointment.Appointment{}
	for _, a := range r.items {
		if a.TenantID != tenant || (filter.DealID != "" && a.DealID != filter.DealID) || (filter.LeadID != "" && a.LeadID != filter.LeadID) {
			continue
		}
		matches = append(matches, a)
	}
	end := filter.Offset + filter.Limit
	if end > int64(len(matches)) {
		end = int64(len(matches))
	}
	return matches[filter.Offset:end], int64(len(matches)), nil
}

func TestInternalDealAppointmentsPreserveLeadVisitsAndTenant(t *testing.T) {
	repo := &portalAppointmentListRepo{items: []*appointment.Appointment{
		{ID: "deal-visit", TenantID: "agency", DealID: "deal", LeadID: "client-lead"},
		{ID: "lead-visit", TenantID: "agency", LeadID: "client-lead"},
		{ID: "other-deal", TenantID: "agency", DealID: "another-deal", LeadID: "client-lead"},
		{ID: "another-client", TenantID: "agency", LeadID: "other-lead"},
		{ID: "another-agency", TenantID: "other-agency", DealID: "deal", LeadID: "client-lead"},
		{ID: "private-linked", TenantID: "agency", DealID: "deal", LeadID: "client-lead", Visibility: appointment.VisibilityPrivate},
	}}
	// Exercise more than one agenda page, preserving every pre-deal appointment.
	for i := 0; i < 101; i++ {
		repo.items = append(repo.items, &appointment.Appointment{ID: fmt.Sprintf("historical-%03d", i), TenantID: "agency", LeadID: "client-lead"})
	}
	h := NewAppointmentHandler(services.NewAppointmentService(repo, nil))
	r := gin.New()
	// P1/A3: a rota interna exige identidade (chave de serviço ou JWT).
	r.Use(internalServiceIdentity())
	r.GET("/deals/:id/appointments", h.ListByDeal)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/deals/deal/appointments?tenant_id=agency&lead_id=client-lead&lead_id=client-lead", nil))
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []*appointment.Appointment `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 103 {
		t.Fatalf("missing or duplicated visits: %d", len(body.Data))
	}
	ids := map[string]bool{}
	for _, a := range body.Data {
		if ids[a.ID] || a.TenantID != "agency" || a.ID == "other-deal" || a.ID == "another-client" {
			t.Fatalf("leaked or duplicated visit: %+v", a)
		}
		ids[a.ID] = true
	}
	if ids["private-linked"] {
		t.Fatal("private appointment leaked into the client portal")
	}
	if !ids["lead-visit"] || !ids["deal-visit"] || len(repo.calls) != 3 {
		t.Fatalf("visits=%v calls=%+v", ids, repo.calls)
	}
	for _, tenant := range repo.tenants {
		if tenant != "agency" {
			t.Fatal("lost tenant scope")
		}
	}
}

func TestInternalDealAppointmentsRejectMissingTenantAndReportAgendaFailure(t *testing.T) {
	for _, tc := range []struct {
		query   string
		status  int
		failure error
	}{ // P1/A3: sem tenant explícito o resolver recusa como validação (400).
		{"", http.StatusBadRequest, nil},
		{"?tenant_id=agency", http.StatusInternalServerError, errors.New("agenda unavailable")},
	} {
		repo := &portalAppointmentListRepo{err: tc.failure}
		h := NewAppointmentHandler(services.NewAppointmentService(repo, nil))
		r := gin.New()
		r.Use(internalServiceIdentity())
		r.GET("/deals/:id/appointments", h.ListByDeal)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/deals/deal/appointments"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("agenda failure hidden: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestPublicCalendarScopesOwnAgendaAndManagerTeamView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, profile, query string
		wantTeam             bool
	}{
		{name: "broker cannot expand scope", profile: "member", query: "?scope=team"},
		{name: "manager defaults to own", profile: "manager"},
		{name: "manager explicitly views team", profile: "manager", query: "?scope=team", wantTeam: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &portalAppointmentListRepo{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), "agency")
				c.Request = c.Request.WithContext(pkgcontext.WithAccountID(ctx, "acc-1"))
				c.Set("permission", tc.profile)
				c.Set("functional_role", map[bool]string{true: "gestor", false: "corretor"}[tc.profile == "manager"])
				c.Next()
			})
			router.GET("/appointments", NewAppointmentHandler(services.NewAppointmentService(repo, nil)).List)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments"+tc.query, nil))
			if w.Code != http.StatusOK || len(repo.calls) != 1 {
				t.Fatalf("list failed: %d %s", w.Code, w.Body.String())
			}
			got := repo.calls[0]
			if got.AccessAgentID != "acc-1" || got.TeamScope != tc.wantTeam {
				t.Fatalf("wrong access scope: %+v", got)
			}
			if tc.wantTeam && got.AgentID != "" {
				t.Fatalf("team view was still restricted to one agent: %+v", got)
			}
			if !tc.wantTeam && got.AgentID != "acc-1" {
				t.Fatalf("own view was not restricted: %+v", got)
			}
		})
	}
}

func TestManagerCannotOpenAnotherBrokersPrivateAppointment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	private := appointment.New("agency", "Consulta médica", "2099-10-20", "09:00", "10:00", appointment.Type("Outro"), appointment.PriorityMedia, "", "Outro corretor")
	private.ID, private.AgentID, private.Visibility = "private", "acc-2", appointment.VisibilityPrivate
	team := appointment.New("agency", "Visita", "2099-10-20", "10:00", "11:00", appointment.TypeVisita, appointment.PriorityMedia, "Cliente", "Outro corretor")
	team.ID, team.AgentID, team.Visibility = "team", "acc-2", appointment.VisibilityTeam
	repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{"private": private, "team": team}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "agency")
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(ctx, "manager"))
		c.Set("permission", "manager")
		c.Set("functional_role", "gestor")
		c.Next()
	})
	router.GET("/appointments/:id", NewAppointmentHandler(services.NewAppointmentService(repo, nil)).GetByID)
	for id, want := range map[string]int{"private": http.StatusForbidden, "team": http.StatusOK} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments/"+id, nil))
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", id, w.Code, want, w.Body.String())
		}
	}
}

func TestCalendarFileRouteServesOnlyEligibleAppointmentsOfTheTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &visitHandlerRepo{items: map[string]*appointment.Appointment{
		"marked":  {ID: "marked", TenantID: "agency", Title: "Visita", Date: "2099-10-10", Time: "14:00", EndTime: "15:00", Type: appointment.TypeVisita, Status: appointment.StatusConfirmado},
		"pending": {ID: "pending", TenantID: "agency", Title: "Visita", Date: "2099-10-10", Time: "14:00", EndTime: "15:00", Type: appointment.TypeVisita, Status: appointment.StatusPendente},
		"other":   {ID: "other", TenantID: "outra", Title: "Visita", Date: "2099-10-10", Time: "14:00", EndTime: "15:00", Type: appointment.TypeVisita, Status: appointment.StatusConfirmado},
	}}
	link := services.NewCalendarLink("segredo-de-teste", "https://api.example")
	h := NewAppointmentHandler(services.NewAppointmentService(repo, nil).WithCalendarLinks(link))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), "agency"))
		c.Next()
	})
	r.GET("/appointments/:id/calendar.ics", h.CalendarFile)
	r.GET("/public/appointments/:id/calendar.ics", h.PublicCalendarFile)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	ok := get("/appointments/marked/calendar.ics")
	if ok.Code != http.StatusOK {
		t.Fatalf("esperado 200, veio %d: %s", ok.Code, ok.Body.String())
	}
	if ct := ok.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Errorf("content-type: %s", ct)
	}
	if cd := ok.Header().Get("Content-Disposition"); !strings.Contains(cd, ".ics") {
		t.Errorf("content-disposition: %s", cd)
	}
	if !strings.Contains(ok.Body.String(), "BEGIN:VCALENDAR") {
		t.Error("corpo não é um arquivo de calendário")
	}

	// Pendente não gera arquivo, e compromisso de outra imobiliária não existe
	// para quem está autenticado aqui.
	if got := get("/appointments/pending/calendar.ics").Code; got != http.StatusConflict {
		t.Errorf("pendente: esperado 409, veio %d", got)
	}
	if got := get("/appointments/other/calendar.ics").Code; got != http.StatusNotFound {
		t.Errorf("outro tenant: esperado 404, veio %d", got)
	}

	// Link assinado: token válido do próprio compromisso funciona; token
	// adulterado e token de outro compromisso não.
	url := link.URL("marked", "agency", time.Now().Add(time.Hour))
	token := url[strings.Index(url, "token=")+len("token="):]
	if got := get("/public/appointments/marked/calendar.ics?token=" + token).Code; got != http.StatusOK {
		t.Errorf("token válido: esperado 200, veio %d", got)
	}
	if got := get("/public/appointments/marked/calendar.ics?token=adulterado").Code; got != http.StatusNotFound {
		t.Errorf("token inválido: esperado 404, veio %d", got)
	}
	if got := get("/public/appointments/pending/calendar.ics?token=" + token).Code; got != http.StatusConflict {
		t.Errorf("pendente por link: esperado 409, veio %d", got)
	}
}

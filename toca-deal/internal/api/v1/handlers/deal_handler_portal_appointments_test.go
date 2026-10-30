package handlers

import (
	"context"
	"encoding/json"
	"github.com/oginoino/toca/pkg/clients"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

type portalAppointmentAccessRepo struct{ deal.Repository }

func (r portalAppointmentAccessRepo) FindBySignerIdentity(context.Context, string, string) ([]*deal.Deal, error) {
	return []*deal.Deal{{ID: "deal", TenantID: "agency"}}, nil
}

func TestPortalAppointmentAgendaFailureIsNotAnEmptySuccess(t *testing.T) {
	t.Setenv("CALENDAR_SERVICE_URL", "")
	t.Setenv("INTERNAL_API_KEY", "test-key")
	service := services.NewDealService(portalAppointmentAccessRepo{}, nil, nil, nil, nil)
	h := NewDealHandler(service)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), "client"))
	})
	r.GET("/client-portal/deals/:id/appointments", h.GetMyDealAppointments)
	for _, tc := range []struct {
		id     string
		status int
	}{{"deal", http.StatusServiceUnavailable}, {"not-authorized", http.StatusForbidden}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/client-portal/deals/"+tc.id+"/appointments", nil))
		if w.Code != tc.status {
			t.Fatalf("expected %d got %d: %s", tc.status, w.Code, w.Body.String())
		}
	}
}

type portalVisitActionRepo struct{ deal.Repository }

func (r portalVisitActionRepo) FindBySignerIdentity(context.Context, string, string) ([]*deal.Deal, error) {
	return []*deal.Deal{{ID: "deal", TenantID: "agency", CustomerID: "lead", Parties: []deal.Party{{AccountID: "client", LeadID: "lead", Role: deal.PartyRoleBuyer}}}}, nil
}

func TestPortalVisitMutationRoutesUseAuthorizedAgendaAndPreserveVisit(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			updatedAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
			a := clients.Appointment{ID: "visit", TenantID: "agency", DealID: "deal", LeadID: "lead", CustomerAccountID: "client", Type: "Visita", Status: "Confirmado", Date: "2099-10-20", Time: "14:00", EndTime: "15:00", UpdatedAt: updatedAt}
			writes := 0
			agenda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					var req clients.UpdateAppointmentRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if req.ExpectedUpdatedAt == nil || !req.ExpectedUpdatedAt.Equal(updatedAt) || req.TenantID != "agency" {
						t.Errorf("unguarded write: %+v", req)
					}
					writes++
					a.Status = req.Status
					if req.Time != "" {
						a.Date, a.Time, a.EndTime = req.Date, req.Time, req.EndTime
					}
					a.UpdatedAt = a.UpdatedAt.Add(time.Second)
				} else if r.Method != http.MethodGet {
					t.Errorf("unexpected agenda method %s", r.Method)
				}
				json.NewEncoder(w).Encode(gin.H{"data": a})
			}))
			defer agenda.Close()
			t.Setenv("CALENDAR_SERVICE_URL", agenda.URL)
			t.Setenv("INTERNAL_API_KEY", "test-key")
			h := NewDealHandler(services.NewDealService(portalVisitActionRepo{}, nil, nil, nil, nil))
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), "client"))
			})
			r.PUT("/client-portal/deals/:id/appointments/:appointmentId", h.UpdateMyDealAppointment)
			r.DELETE("/client-portal/deals/:id/appointments/:appointmentId", h.CancelMyDealAppointment)
			body := `{"expected_updated_at":"2026-09-17T12:00:00Z","date":"2099-10-21","time":"16:00","end_time":"17:00"}`
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, "/client-portal/deals/deal/appointments/visit", strings.NewReader(body)))
			expectedStatus := "Reagendado"
			if method == http.MethodDelete {
				expectedStatus = "Cancelado"
			}
			if w.Code != 200 || writes != 1 || !strings.Contains(w.Body.String(), `"status":"`+expectedStatus+`"`) || !strings.Contains(w.Body.String(), `"updated_at"`) {
				t.Fatalf("mutation failed: writes=%d response=%d %s", writes, w.Code, w.Body.String())
			}
			// Replay of the old UI revision cannot overwrite the new appointment.
			w = httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, "/client-portal/deals/deal/appointments/visit", strings.NewReader(body)))
			if w.Code != 409 || writes != 1 {
				t.Fatalf("stale mutation accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPortalCalendarRouteOnlyServesTheClientsOwnMarkedAppointment(t *testing.T) {
	agenda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []clients.Appointment{
			{ID: "marked", TenantID: "agency", DealID: "deal", LeadID: "lead", Status: "Confirmado", Title: "Visita", Date: "2099-10-20", Time: "14:00", EndTime: "15:00"},
			{ID: "pending", TenantID: "agency", DealID: "deal", LeadID: "lead", Status: "Pendente", Title: "Visita", Date: "2099-10-20", Time: "14:00", EndTime: "15:00"},
		}})
	}))
	defer agenda.Close()
	t.Setenv("CALENDAR_SERVICE_URL", agenda.URL)
	t.Setenv("INTERNAL_API_KEY", "test-key")

	repo := portalVisitActionRepo{}
	service := services.NewDealService(repo, nil, nil, nil, nil)
	h := NewDealHandler(service)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithAccountID(c.Request.Context(), "client"))
	})
	r.GET("/client-portal/deals/:id/appointments/:appointmentId/calendar.ics", h.DownloadMyDealAppointmentCalendar)

	get := func(appointmentID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/client-portal/deals/deal/appointments/"+appointmentID+"/calendar.ics", nil))
		return w
	}

	ok := get("marked")
	if ok.Code != http.StatusOK {
		t.Fatalf("esperado 200, veio %d: %s", ok.Code, ok.Body.String())
	}
	if ct := ok.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Errorf("content-type: %s", ct)
	}
	if !strings.Contains(ok.Body.String(), "BEGIN:VCALENDAR") {
		t.Error("corpo não é um arquivo de calendário")
	}
	// Pedido ainda pendente não vira evento no calendário do cliente.
	if got := get("pending").Code; got != http.StatusConflict {
		t.Errorf("pendente: esperado 409, veio %d", got)
	}
	// Compromisso de outro negócio (ou de outro cliente) não existe aqui.
	if got := get("de-outro-negocio").Code; got != http.StatusConflict {
		t.Errorf("compromisso alheio: esperado 409, veio %d", got)
	}
}

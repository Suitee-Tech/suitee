package clients

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type visitTransport func(*http.Request) (*http.Response, error)

func (f visitTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCalendarVisitErrorsAndScopedRead(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{409, ErrCalendarConflict}, {404, ErrCalendarNotFound}, {503, ErrCalendarUnavailable}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			base := NewBaseClient("http://calendar.test")
			base.InternalAPIKey = "test-key"
			base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
			})
			client := &CalendarClient{BaseClient: base}
			_, err := client.UpdateAppointment(context.Background(), "visit", UpdateAppointmentRequest{TenantID: "agency", Status: "Confirmado"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	base := NewBaseClient("http://calendar.test")
	base.InternalAPIKey = "test-key"
	base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("tenant_id") != "agency" || r.Header.Get("X-Internal-Key") != "test-key" {
			t.Fatal("missing tenant or internal authentication")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"id":"visit","tenant_id":"other"}}`)), Header: http.Header{}}, nil
	})
	_, err := (&CalendarClient{BaseClient: base}).GetAppointment(context.Background(), "visit", "agency")
	if !errors.Is(err, ErrCalendarUnavailable) {
		t.Fatalf("cross-tenant response accepted: %v", err)
	}
}

func TestCalendarDealAppointmentsRejectUnscopedAndCrossTenantLists(t *testing.T) {
	base := NewBaseClient("http://calendar.test")
	base.InternalAPIKey = "test-key"
	calls := 0
	base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("tenant_id") != "agency" || r.Header.Get("X-Internal-Key") != "test-key" {
			t.Fatal("lost internal tenant scope")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"visit","tenant_id":"other"}]}`)), Header: http.Header{}}, nil
	})
	client := &CalendarClient{BaseClient: base}
	_, err := client.ListAppointmentsByDeal(context.Background(), "deal", DealAppointmentScope{})
	if !errors.Is(err, ErrCalendarInvalid) || calls != 0 {
		t.Fatal("unscoped list reached agenda")
	}
	_, err = client.ListAppointmentsByDeal(context.Background(), "deal", DealAppointmentScope{TenantID: "agency"})
	if !errors.Is(err, ErrCalendarUnavailable) {
		t.Fatalf("cross-tenant list accepted: %v", err)
	}
}

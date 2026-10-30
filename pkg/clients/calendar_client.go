package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/domain"
)

type Appointment struct {
	CanManage         bool      `json:"-"`
	CustomerAccountID string    `json:"customer_account_id"`
	DealID            string    `json:"deal_id"`
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	LeadID            string    `json:"lead_id"`
	PropertyID        string    `json:"property_id"`
	AgentID           string    `json:"agent_id"`
	Title             string    `json:"title"`
	Date              string    `json:"date"`
	Time              string    `json:"time"`
	EndTime           string    `json:"end_time"`
	Type              string    `json:"type"`
	Status            string    `json:"status"`
	Description       string    `json:"description,omitempty"`
	Property          string    `json:"property,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type CalendarClient struct {
	BaseClient *BaseClient
}

func NewCalendarClient() *CalendarClient {
	return &CalendarClient{
		BaseClient: NewBaseClient(os.Getenv("CALENDAR_SERVICE_URL")),
	}
}

// CreateAppointmentRequest is the payload to create an appointment.
//
// TenantID is the typed tenant identifier — see pkg/domain.TenantID for
// the rationale. CustomerAccountID is a *different* concept (the
// customer's portal account) and intentionally remains a string until
// pkg/domain.AccountID lands; mixing the two at a call site was one of
// the reasons we typed TenantID in the first place.
type CreateAppointmentRequest struct {
	ID                string          `json:"id,omitempty"`
	TenantID          domain.TenantID `json:"tenant_id"`
	Title             string          `json:"title"`
	Date              string          `json:"date"`
	Time              string          `json:"time"`
	EndTime           string          `json:"end_time"`
	Type              string          `json:"type"`
	Priority          string          `json:"priority"`
	Client            string          `json:"client"`
	LeadID            string          `json:"lead_id,omitempty"`
	Property          string          `json:"property,omitempty"`
	CustomerAccountID string          `json:"customer_account_id,omitempty"`
	DealID            string          `json:"deal_id"`
	PropertyID        string          `json:"property_id"`
	Agent             string          `json:"agent"`
	AgentID           string          `json:"agent_id,omitempty"`
	Status            string          `json:"status"`
	Description       string          `json:"description"`
}

// CreateAppointment creates an appointment in calendar service
func (c *CalendarClient) CreateAppointment(ctx context.Context, req CreateAppointmentRequest) (*Appointment, error) {
	path := "/api/v1/calendar/internal/appointments"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, calendarResponseError(resp)
	}

	var response struct {
		Data *Appointment `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if response.Data == nil || response.Data.ID == "" {
		return nil, ErrCalendarUnavailable
	}

	return response.Data, nil
}

// UpdateAppointmentRequest is the payload to update an appointment.
type UpdateAppointmentRequest struct {
	ExpectedUpdatedAt *time.Time `json:"expected_updated_at,omitempty"`
	TenantID          string     `json:"tenant_id"`
	CustomerAccountID string     `json:"customer_account_id,omitempty"`
	Title             string     `json:"title"`
	Date              string     `json:"date"`
	Time              string     `json:"time"`
	EndTime           string     `json:"end_time"`
	Type              string     `json:"type"`
	Priority          string     `json:"priority"`
	Client            string     `json:"client"`
	LeadID            string     `json:"lead_id,omitempty"`
	Property          string     `json:"property,omitempty"`
	PropertyID        string     `json:"property_id,omitempty"`
	Agent             string     `json:"agent"`
	AgentID           string     `json:"agent_id,omitempty"`
	DealID            string     `json:"deal_id,omitempty"`
	Status            string     `json:"status"`
	Description       *string    `json:"description,omitempty"`
}

// UpdateAppointment atualiza um appointment (data/hora/status) no calendar service.
func (c *CalendarClient) UpdateAppointment(ctx context.Context, id string, req UpdateAppointmentRequest) (*Appointment, error) {
	path := fmt.Sprintf("/api/v1/calendar/internal/appointments/%s", id)
	resp, err := c.BaseClient.Do(ctx, http.MethodPut, path, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, calendarResponseError(resp)
	}
	var response struct {
		Data *Appointment `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if response.Data == nil || response.Data.ID != id {
		return nil, ErrCalendarUnavailable
	}
	return response.Data, nil
}

var ErrCalendarChanged = errors.New("A agenda foi alterada. Atualize a página antes de tentar novamente.")

var ErrCalendarConflict = errors.New("O corretor já possui um compromisso nesse horário. Escolha outro horário.")
var ErrCalendarNotFound = errors.New("appointment not found")
var ErrCalendarInvalid = errors.New("Informe dados válidos para o compromisso.")
var ErrCalendarUnavailable = errors.New("Não foi possível sincronizar a visita com a agenda. Tente novamente.")

func calendarStatusError(status int) error {
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return ErrCalendarInvalid
	}
	if status == http.StatusConflict {
		return ErrCalendarConflict
	}
	if status == http.StatusNotFound {
		return ErrCalendarNotFound
	}
	return fmt.Errorf("%w (status %d)", ErrCalendarUnavailable, status)
}

// GetAppointment reads the authoritative agenda state within the lead's tenant.
func (c *CalendarClient) GetAppointment(ctx context.Context, id, tenantID string) (*Appointment, error) {
	path := fmt.Sprintf("/api/v1/calendar/internal/appointments/%s?tenant_id=%s", url.PathEscape(id), url.QueryEscape(tenantID))
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}
	defer drainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, calendarResponseError(resp)
	}
	var result struct {
		Data *Appointment `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}
	if result.Data == nil || result.Data.ID != id || result.Data.TenantID != tenantID {
		return nil, ErrCalendarUnavailable
	}
	return result.Data, nil
}

// DeleteAppointment remove um appointment do calendar service.
//
// O tenant vai explícito porque a rota interna o resolve pela CREDENCIAL
// (`auth.ResolveTenantForInternalRequest`, achado A3): sem ele o serviço
// responde 400. Sem chamador hoje — a assinatura exige o tenant para que o
// primeiro uso não repita o defeito do portal (400 traduzido em 403).
func (c *CalendarClient) DeleteAppointment(ctx context.Context, tenantID, id string) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrCalendarInvalid
	}
	path := fmt.Sprintf("/api/v1/calendar/internal/appointments/%s?tenant_id=%s", id, url.QueryEscape(tenantID))
	resp, err := c.BaseClient.Do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("calendar delete returned status: %d", resp.StatusCode)
	}
	return nil
}

// DealAppointmentScope is derived by the deal service after authorizing the client.
// LeadIDs belong to that client, not to every participant in the negotiation.
type DealAppointmentScope struct {
	TenantID string
	LeadIDs  []string
}

func (c *CalendarClient) ListAppointmentsByDeal(ctx context.Context, dealID string, scope DealAppointmentScope) ([]*Appointment, error) {
	if strings.TrimSpace(scope.TenantID) == "" {
		return nil, ErrCalendarInvalid
	}
	params := url.Values{"tenant_id": {scope.TenantID}}
	for _, leadID := range scope.LeadIDs {
		if leadID != "" {
			params.Add("lead_id", leadID)
		}
	}
	path := fmt.Sprintf("/api/v1/calendar/internal/deals/%s/appointments?%s", url.PathEscape(dealID), params.Encode())
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, calendarResponseError(resp)
	}

	var response struct {
		Data []*Appointment `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnavailable, err)
	}

	for _, appointment := range response.Data {
		if appointment != nil && appointment.TenantID != scope.TenantID {
			return nil, ErrCalendarUnavailable
		}
	}
	return response.Data, nil
}

func calendarResponseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusConflict {
		var body struct {
			Message string `json:"message"`
		}
		if json.NewDecoder(resp.Body).Decode(&body) == nil && strings.Contains(body.Message, "agenda foi alterada") {
			return ErrCalendarChanged
		}
	}
	return calendarStatusError(resp.StatusCode)
}

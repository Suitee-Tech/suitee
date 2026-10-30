package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type MaintenanceTicket struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Category      string     `json:"category"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	Description   string     `json:"description"`
	Attachments   []string   `json:"attachments,omitempty"`
	PropertyTitle string     `json:"property_title"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type OpsClient struct {
	BaseClient *BaseClient
}

func NewOpsClient() *OpsClient {
	return &OpsClient{
		BaseClient: NewBaseClient(os.Getenv("OPS_SERVICE_URL")),
	}
}

// CreateMaintenanceTicketRequest represents the payload for creating a ticket
type CreateMaintenanceTicketRequest struct {
	TenantID      string   `json:"tenant_id"`
	Type          string   `json:"type"`
	Category      string   `json:"category"`
	PropertyID    string   `json:"property_id"`
	PropertyTitle string   `json:"property_title"`
	Requester     string   `json:"requester"`
	Status        string   `json:"status"`
	Priority      string   `json:"priority"`
	Description   string   `json:"description"`
	Attachments   []string `json:"attachments,omitempty"`
}

// CreateMaintenanceTicket creates a maintenance ticket in ops service
func (c *OpsClient) CreateMaintenanceTicket(ctx context.Context, req CreateMaintenanceTicketRequest) (*MaintenanceTicket, error) {
	path := "/api/v1/ops/internal/maintenance-tickets"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data *MaintenanceTicket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

// ListMaintenanceTicketsByProperty lists maintenance tickets for a given property.
//
// O tenant é obrigatório (achado A3 da revisão de permissões de 2026-09-19): a
// rota interna resolve o tenant pela credencial e recusa sem ele. O chamador é
// quem sabe o tenant do recurso (ex.: o dono do negócio no portal do cliente).
func (c *OpsClient) ListMaintenanceTicketsByProperty(ctx context.Context, tenantID, propertyID string) ([]*MaintenanceTicket, error) {
	path := fmt.Sprintf("/api/v1/ops/internal/properties/%s/maintenance-tickets?tenant_id=%s", propertyID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []*MaintenanceTicket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

type Ticket struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	DealID             string          `json:"deal_id,omitempty"`
	PropertyID         string          `json:"property_id,omitempty"`
	Type               string          `json:"type"`
	Category           string          `json:"category"`
	Subject            string          `json:"subject"`
	Description        string          `json:"description,omitempty"`
	Status             string          `json:"status"`
	Priority           string          `json:"priority"`
	RequesterAccountID string          `json:"requester_account_id"`
	RequesterName      string          `json:"requester_name,omitempty"`
	RequesterEmail     string          `json:"requester_email,omitempty"`
	AssignedToID       string          `json:"assigned_to_id,omitempty"`
	AssignedToName     string          `json:"assigned_to_name,omitempty"`
	Comments           []TicketComment `json:"comments,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ResolvedAt         *time.Time      `json:"resolved_at,omitempty"`
	ClosedAt           *time.Time      `json:"closed_at,omitempty"`
}

type TicketComment struct {
	ID         string `json:"id"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name,omitempty"`
	AuthorType string `json:"author_type"`
	// Visibility: "public" (default) ou "internal". Notas internas devem
	// ser filtradas pelo caller antes de retornar ao portal cliente.
	Visibility  string    `json:"visibility,omitempty"`
	Content     string    `json:"content"`
	Attachments []string  `json:"attachments,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateTicketRequest struct {
	TenantID           string `json:"tenant_id"`
	DealID             string `json:"deal_id,omitempty"`
	PropertyID         string `json:"property_id,omitempty"`
	Type               string `json:"type"`
	Category           string `json:"category"`
	Subject            string `json:"subject"`
	Description        string `json:"description,omitempty"`
	Priority           string `json:"priority"`
	RequesterAccountID string `json:"requester_account_id"`
	RequesterName      string `json:"requester_name,omitempty"`
	RequesterEmail     string `json:"requester_email,omitempty"`
}

type AddTicketCommentRequest struct {
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name,omitempty"`
	AuthorType string `json:"author_type"`
	// Visibility: "public" (default) ou "internal". Sempre enviar "public"
	// quando o request vem do portal cliente — nunca confiar no input do
	// cliente para marcar uma nota como interna.
	Visibility  string   `json:"visibility,omitempty"`
	Content     string   `json:"content"`
	Attachments []string `json:"attachments,omitempty"`
	// ClientRequestID: idempotency token opcional (UUID v4 do frontend).
	// toca-ops dedup pelo conteúdo embedded — retry de rede não duplica
	// o comment. Issue #453.
	ClientRequestID string `json:"client_request_id,omitempty"`
}

func (c *OpsClient) CreateTicket(ctx context.Context, tenantID string, req CreateTicketRequest) (*Ticket, error) {
	req.TenantID = tenantID
	path := "/api/v1/ops/internal/tickets"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data *Ticket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *OpsClient) ListTicketsByRequester(ctx context.Context, tenantID, accountID string) ([]*Ticket, error) {
	path := fmt.Sprintf("/api/v1/ops/internal/tickets/by-requester/%s?tenant_id=%s", accountID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []*Ticket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *OpsClient) GetTicketByID(ctx context.Context, tenantID, id string) (*Ticket, error) {
	path := fmt.Sprintf("/api/v1/ops/internal/tickets/%s?tenant_id=%s", id, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data *Ticket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// InspectionSummary is the minimal projection toca-deal needs to gate the
// contract termination flow on "vistoria de saída concluída + laudo".
type InspectionSummary struct {
	ID         string `json:"id"`
	ContractID string `json:"contract_id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	ReportURL  string `json:"report_url,omitempty"`
}

// ListInspectionsByContract calls the internal ops endpoint to retrieve the
// inspections (optionally filtered by type/status) attached to the contract.
// Returns an empty slice when nothing is found; non-nil error only on
// transport failure or non-2xx response.
func (c *OpsClient) ListInspectionsByContract(ctx context.Context, tenantID, contractID, inspType, status string) ([]*InspectionSummary, error) {
	q := fmt.Sprintf("?tenant_id=%s&contract_id=%s", tenantID, contractID)
	if inspType != "" {
		q += "&type=" + inspType
	}
	if status != "" {
		q += "&status=" + status
	}
	path := "/api/v1/ops/internal/inspections" + q
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []*InspectionSummary `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return []*InspectionSummary{}, nil
	}
	return response.Data, nil
}

// HasCompletedExitInspection checks whether at least one EXIT inspection has
// status=Concluída AND a non-empty ReportURL for the contract. Implements
// service.ExitInspectionChecker in toca-deal.
func (c *OpsClient) HasCompletedExitInspection(ctx context.Context, tenantID, contractID string) (bool, error) {
	list, err := c.ListInspectionsByContract(ctx, tenantID, contractID, "EXIT", "")
	if err != nil {
		return false, err
	}
	for _, i := range list {
		if i == nil {
			continue
		}
		if i.Status == "Concluída" && i.ReportURL != "" {
			return true, nil
		}
	}
	return false, nil
}

func (c *OpsClient) AddTicketComment(ctx context.Context, tenantID, ticketID string, comment AddTicketCommentRequest) (*TicketComment, error) {
	path := fmt.Sprintf("/api/v1/ops/internal/tickets/%s/comments?tenant_id=%s", ticketID, tenantID)
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, comment)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ops service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data *TicketComment `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

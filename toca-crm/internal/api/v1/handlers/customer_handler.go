package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type CustomerHandler struct {
	leadRepo    lead.Repository
	httpClient  *http.Client
	dealURL     string
	financeURL  string
	opsURL      string
	calendarURL string
}

func NewCustomerHandler(leadRepo lead.Repository) *CustomerHandler {
	return &CustomerHandler{
		leadRepo: leadRepo,
		httpClient: &http.Client{
			Timeout: 7 * time.Second,
		},
		dealURL:     strings.TrimRight(os.Getenv("DEAL_SERVICE_URL"), "/"),
		financeURL:  strings.TrimRight(os.Getenv("FINANCIAL_SERVICE_URL"), "/"),
		opsURL:      strings.TrimRight(os.Getenv("OPS_SERVICE_URL"), "/"),
		calendarURL: strings.TrimRight(os.Getenv("CALENDAR_SERVICE_URL"), "/"),
	}
}

type svcResponse[T any] struct {
	Data T                        `json:"data"`
	Meta *response.PaginationMeta `json:"meta,omitempty"`
}

func (h *CustomerHandler) authedGet(ctx context.Context, baseURL, path, token string, out any) error {
	if baseURL == "" {
		return fmt.Errorf("baseURL not configured for %s", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream %s returned status: %d", baseURL, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (h *CustomerHandler) resolveCustomer(ctx context.Context, tenantID, id string) (string, *lead.Lead, []*lead.Lead, error) {
	// SECURITY (achados C9/A19): leitura tenant-scoped. Antes, o lead de outro
	// tenant era resolvido e devolvido como primary_lead desta empresa.
	l, err := h.leadRepo.GetByID(ctx, tenantID, id)
	if err == nil && l != nil {
		if l.AccountID == "" {
			return "", l, []*lead.Lead{l}, nil
		}
		leads, err := h.leadRepo.ListByAccountID(ctx, tenantID, l.AccountID, 200)
		if err != nil {
			return "", nil, nil, err
		}
		if len(leads) == 0 {
			return l.AccountID, l, []*lead.Lead{l}, nil
		}
		return l.AccountID, leads[0], leads, nil
	}

	leads, err := h.leadRepo.ListByAccountID(ctx, tenantID, id, 200)
	if err != nil {
		return "", nil, nil, err
	}
	if len(leads) == 0 {
		return "", nil, nil, fmt.Errorf("customer not found")
	}
	return id, leads[0], leads, nil
}

func (h *CustomerHandler) GetProfile(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	id := c.Param("id")
	accountID, primary, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, "Customer retrieved successfully", gin.H{
		"account_id":   accountID,
		"primary_lead": primary,
		"leads_count":  len(leads),
	})
}

func (h *CustomerHandler) ListLeads(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	id := c.Param("id")
	_, _, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, "Customer leads retrieved successfully", leads)
}

type dealDTO struct {
	ID        string  `json:"id"`
	DisplayID string  `json:"display_id,omitempty"`
	Type      string  `json:"type"`
	Status    string  `json:"status"`
	Value     float64 `json:"value"`
	CreatedAt string  `json:"created_at"`
}

type chargeDTO struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	DueDate     string  `json:"due_date"`
	Status      string  `json:"status"`
	TotalAmount float64 `json:"total_amount"`
}

type ticketDTO struct {
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	CreatedAt string `json:"created_at"`
}

type appointmentDTO struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Date     string `json:"date"`
	Time     string `json:"time"`
	EndTime  string `json:"end_time"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Property string `json:"property,omitempty"`
}

func (h *CustomerHandler) GetSummary(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	token, err := appctx.GetAuthToken(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	accountID, primary, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	customerIDs := make([]string, 0, len(leads))
	for _, l := range leads {
		if l != nil && l.ID != "" {
			customerIDs = append(customerIDs, l.ID)
		}
	}
	csvIDs := strings.Join(customerIDs, ",")

	leadsCount := len(leads)
	if leadsCount == 0 && primary != nil {
		leadsCount = 1
	}

	var dealsCount int64
	if h.dealURL != "" && csvIDs != "" {
		var out svcResponse[[]dealDTO]
		if err := h.authedGet(ctx, h.dealURL, fmt.Sprintf("/api/v1/deals?customer_ids=%s&limit=1&offset=0", csvIDs), token, &out); err == nil && out.Meta != nil {
			dealsCount = out.Meta.Total
		}
	}

	var ticketsCount int64
	if h.opsURL != "" && accountID != "" {
		var out svcResponse[[]ticketDTO]
		if err := h.authedGet(ctx, h.opsURL, fmt.Sprintf("/api/v1/tickets?requester_account_id=%s&limit=1&offset=0", accountID), token, &out); err == nil && out.Meta != nil {
			ticketsCount = out.Meta.Total
		}
	}

	var openCharges float64
	if h.financeURL != "" && csvIDs != "" {
		var out svcResponse[[]chargeDTO]
		if err := h.authedGet(ctx, h.financeURL, fmt.Sprintf("/api/v1/charges?party_ids=%s&status=PENDING,OVERDUE&limit=200&offset=0", csvIDs), token, &out); err == nil {
			for _, ch := range out.Data {
				if strings.EqualFold(ch.Status, "PAID") || strings.EqualFold(ch.Status, "CANCELLED") {
					continue
				}
				openCharges += ch.TotalAmount
			}
		}
	}

	var nextVisit string
	if h.calendarURL != "" && len(customerIDs) > 0 {
		today := time.Now().Format("2006-01-02")
		best := time.Time{}
		for _, leadID := range customerIDs {
			var out svcResponse[[]appointmentDTO]
			err := h.authedGet(ctx, h.calendarURL, fmt.Sprintf("/api/v1/appointments?lead_id=%s&start_date=%s&limit=1&offset=0", leadID, today), token, &out)
			if err != nil || len(out.Data) == 0 {
				continue
			}
			a := out.Data[0]
			tm, err := time.Parse("2006-01-02 15:04", a.Date+" "+a.Time)
			if err != nil {
				continue
			}
			if best.IsZero() || tm.Before(best) {
				best = tm
			}
		}
		if !best.IsZero() {
			nextVisit = best.Format(time.RFC3339)
		}
	}

	response.Success(c, "Customer summary retrieved successfully", gin.H{
		"leads":           leadsCount,
		"deals":           dealsCount,
		"tickets_count":   ticketsCount,
		"open_charges":    openCharges,
		"next_visit":      nextVisit,
		"unread_messages": 0,
		"customer": gin.H{
			"id":         primary.ID,
			"name":       primary.Name,
			"email":      primary.Email,
			"phone":      primary.Phone,
			"role":       primary.Role,
			"is_client":  primary.IsClient,
			"account_id": accountID,
		},
	})
}

func (h *CustomerHandler) ListDeals(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	token, err := appctx.GetAuthToken(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	_, _, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	customerIDs := make([]string, 0, len(leads))
	for _, l := range leads {
		if l != nil && l.ID != "" {
			customerIDs = append(customerIDs, l.ID)
		}
	}
	csvIDs := strings.Join(customerIDs, ",")

	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	var out svcResponse[[]dealDTO]
	if err := h.authedGet(ctx, h.dealURL, fmt.Sprintf("/api/v1/deals?customer_ids=%s&limit=%s&offset=%s", csvIDs, limit, offset), token, &out); err != nil {
		response.InternalError(c, err)
		return
	}
	lim, _ := strconv.Atoi(limit)
	off, _ := strconv.Atoi(offset)
	page := 1
	if lim > 0 {
		page = (off / lim) + 1
	}
	total := int64(len(out.Data))
	if out.Meta != nil {
		total = out.Meta.Total
		if out.Meta.Limit > 0 {
			lim = out.Meta.Limit
		}
	}
	response.Paginated(c, "Customer deals retrieved successfully", out.Data, page, lim, total)
}

func (h *CustomerHandler) ListCharges(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	token, err := appctx.GetAuthToken(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	_, _, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	partyIDs := make([]string, 0, len(leads))
	for _, l := range leads {
		if l != nil && l.ID != "" {
			partyIDs = append(partyIDs, l.ID)
		}
	}
	csvIDs := strings.Join(partyIDs, ",")

	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")
	status := c.Query("status")
	if status == "" {
		status = "PENDING,OVERDUE"
	}

	var out svcResponse[[]chargeDTO]
	if err := h.authedGet(ctx, h.financeURL, fmt.Sprintf("/api/v1/charges?party_ids=%s&status=%s&limit=%s&offset=%s", csvIDs, status, limit, offset), token, &out); err != nil {
		response.InternalError(c, err)
		return
	}
	lim, _ := strconv.Atoi(limit)
	off, _ := strconv.Atoi(offset)
	page := 1
	if lim > 0 {
		page = (off / lim) + 1
	}
	total := int64(len(out.Data))
	if out.Meta != nil {
		total = out.Meta.Total
		if out.Meta.Limit > 0 {
			lim = out.Meta.Limit
		}
	}
	response.Paginated(c, "Customer charges retrieved successfully", out.Data, page, lim, total)
}

func (h *CustomerHandler) ListAppointments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	token, err := appctx.GetAuthToken(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	_, _, leads, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if len(leads) == 0 {
		response.Success(c, "Customer appointments retrieved successfully", []appointmentDTO{})
		return
	}

	leadID := leads[0].ID
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	path := fmt.Sprintf("/api/v1/appointments?lead_id=%s&limit=%s&offset=%s", leadID, limit, offset)
	if startDate != "" {
		path += "&start_date=" + startDate
	}
	if endDate != "" {
		path += "&end_date=" + endDate
	}

	var out svcResponse[[]appointmentDTO]
	if err := h.authedGet(ctx, h.calendarURL, path, token, &out); err != nil {
		response.InternalError(c, err)
		return
	}
	lim, _ := strconv.Atoi(limit)
	off, _ := strconv.Atoi(offset)
	page := 1
	if lim > 0 {
		page = (off / lim) + 1
	}
	total := int64(len(out.Data))
	if out.Meta != nil {
		total = out.Meta.Total
		if out.Meta.Limit > 0 {
			lim = out.Meta.Limit
		}
	}
	response.Paginated(c, "Customer appointments retrieved successfully", out.Data, page, lim, total)
}

func (h *CustomerHandler) ListTickets(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	token, err := appctx.GetAuthToken(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	accountID, _, _, err := h.resolveCustomer(ctx, tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if accountID == "" {
		response.Success(c, "Customer tickets retrieved successfully", []ticketDTO{})
		return
	}

	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	var out svcResponse[[]ticketDTO]
	if err := h.authedGet(ctx, h.opsURL, fmt.Sprintf("/api/v1/tickets?requester_account_id=%s&limit=%s&offset=%s", accountID, limit, offset), token, &out); err != nil {
		response.InternalError(c, err)
		return
	}
	lim, _ := strconv.Atoi(limit)
	off, _ := strconv.Atoi(offset)
	page := 1
	if lim > 0 {
		page = (off / lim) + 1
	}
	total := int64(len(out.Data))
	if out.Meta != nil {
		total = out.Meta.Total
		if out.Meta.Limit > 0 {
			lim = out.Meta.Limit
		}
	}
	response.Paginated(c, "Customer tickets retrieved successfully", out.Data, page, lim, total)
}

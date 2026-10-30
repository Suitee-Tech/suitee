package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// OwnerStatementHandler expõe o demonstrativo mensal consolidado por
// proprietário. Existem dois sabores:
//   - Admin: GET /owner-statements/:ownerLeadId?year=&month=&format=
//     (finance:read)
//   - Portal: GET /client-portal/owner-statements?year=&month=&format=
//     (sem :ownerLeadId — resolve via crossRepo.LeadIDsForAccount; quando
//     o account é dono de N leads, soma todos).
type OwnerStatementHandler struct {
	service   *service.OwnerStatementService
	crossRepo repository.CrossTenantRepository
	// tenantCompany resolve o nome real da imobiliária no cabeçalho do PDF.
	// Opcional: sem ele o renderer omite o emissor, nunca usa marca fixa.
	tenantCompany tenantCompanyLookup
	// crmClient resolve nome/documento do proprietário. Opcional e nil-safe.
	crmClient crmLeadLookup
}

func NewOwnerStatementHandler(s *service.OwnerStatementService, crossRepo repository.CrossTenantRepository) *OwnerStatementHandler {
	return &OwnerStatementHandler{service: s, crossRepo: crossRepo}
}

// WithTenantCompanyLookup injeta o resolvedor do nome da imobiliária logada.
func (h *OwnerStatementHandler) WithTenantCompanyLookup(lookup tenantCompanyLookup) *OwnerStatementHandler {
	h.tenantCompany = lookup
	return h
}

// WithCRMClient injeta o cliente usado para nome/documento do proprietário.
func (h *OwnerStatementHandler) WithCRMClient(c crmLeadLookup) *OwnerStatementHandler {
	h.crmClient = c
	return h
}

// GetAdmin godoc
// @Summary      Get monthly owner statement (admin)
// @Tags         owner-statements
// @Produce      json
// @Security     BearerAuth
// @Param        ownerLeadId path string true "Owner lead ID"
// @Param        year query int true "Year"
// @Param        month query int true "Month (1-12)"
// @Param        format query string false "json | pdf (default json)"
// @Success      200 {object} response.Response{data=service.MonthlyStatement}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /owner-statements/{ownerLeadId} [get]
func (h *OwnerStatementHandler) GetAdmin(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	ownerLeadID := c.Param("ownerLeadId")
	if ownerLeadID == "" {
		response.ValidationError(c, "ownerLeadId is required")
		return
	}
	year, month, err := parseYearMonth(c)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	stmt, err := h.service.GenerateMonthly(ctx, tenantID, ownerLeadID, year, month)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	h.respondStatement(c, stmt)
}

// GetMine godoc — portal endpoint: caller is the owner (auth account).
// Soma todos os leads vinculados ao account_id do JWT no tenant.
// @Summary      Get monthly owner statement (my)
// @Tags         portal
// @Produce      json
// @Security     BearerAuth
// @Param        year query int true "Year"
// @Param        month query int true "Month (1-12)"
// @Param        format query string false "json | pdf"
// @Success      200 {object} response.Response{data=service.MonthlyStatement}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /client-portal/owner-statements [get]
func (h *OwnerStatementHandler) GetMine(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := context.GetAccountID(ctx)
	if accountID == "" {
		response.Forbidden(c, "account context required")
		return
	}
	leadIDs, err := h.crossRepo.LeadIDsForAccount(ctx, tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if len(leadIDs) == 0 {
		// LGPD: sem leads vinculados ao account → 200 com statement vazio
		// para não vazar a existência (ou não) de owner-id no tenant.
		response.Success(c, "No owner leads found for this account", []any{})
		return
	}
	year, month, err := parseYearMonth(c)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	// Quando o account tem múltiplos leads (raro mas possível: investidor
	// com várias entidades cadastradas), agregamos todos os statements
	// num único response somando os totais.
	first, err := h.service.GenerateMonthly(ctx, tenantID, leadIDs[0], year, month)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	for _, lid := range leadIDs[1:] {
		extra, err := h.service.GenerateMonthly(ctx, tenantID, lid, year, month)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		first.GrossRent += extra.GrossRent
		first.AdminFeeAmount += extra.AdminFeeAmount
		first.MaintenanceDeductions += extra.MaintenanceDeductions
		first.OtherDeductions += extra.OtherDeductions
		first.NetAmount += extra.NetAmount
		first.OwnerBalanceTotal += extra.OwnerBalanceTotal
		first.ReservedAmount += extra.ReservedAmount
		first.BlockedAmount += extra.BlockedAmount
		first.AvailableAmount += extra.AvailableAmount
		first.BalanceCoveredCount += extra.BalanceCoveredCount
		first.ManagedBillsTotal += extra.ManagedBillsTotal
		first.ManagedBills = append(first.ManagedBills, extra.ManagedBills...)
		first.JournalTotals.Revenue += extra.JournalTotals.Revenue
		first.JournalTotals.OwnerPayable += extra.JournalTotals.OwnerPayable
		first.JournalTotals.AccountsReceivable += extra.JournalTotals.AccountsReceivable
		first.JournalTotals.Cash += extra.JournalTotals.Cash
		first.Transfers = append(first.Transfers, extra.Transfers...)
	}
	first.RebuildFlowSummaries()
	// OwnerLeadID na resposta agregada vira o do account (não há um
	// único lead "primário"); para o PDF, mostramos a soma e citamos
	// "Account ID" no header.
	first.OwnerLeadID = "account:" + accountID
	h.respondStatement(c, first)
}

// GetBalance godoc
// @Summary      Get current owner balance (admin widget)
// @Tags         owner-statements
// @Produce      json
// @Security     BearerAuth
// @Param        ownerLeadId path string true "Owner lead ID"
// @Success      200 {object} response.Response{data=service.OwnerBalanceSummary}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /owner-statements/{ownerLeadId}/balance [get]
func (h *OwnerStatementHandler) GetBalance(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	ownerLeadID := c.Param("ownerLeadId")
	if ownerLeadID == "" {
		response.ValidationError(c, "ownerLeadId is required")
		return
	}
	summary, err := h.service.LatestBalance(ctx, tenantID, ownerLeadID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Owner balance retrieved successfully", summary)
}

func (h *OwnerStatementHandler) respondStatement(c *gin.Context, stmt *service.MonthlyStatement) {
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format == "" {
		format = "json"
	}
	if format == "json" {
		response.Success(c, "Owner statement retrieved successfully", stmt)
		return
	}
	if format != "pdf" {
		response.ValidationError(c, "unsupported format; expected json | pdf")
		return
	}
	rctx := service.OwnerStatementPDFContext{Statement: stmt}
	if tenantID, err := context.GetTenantID(c.Request.Context()); err == nil {
		// Best-effort: o nome da imobiliária é enriquecimento; se a leitura
		// falhar, o renderer omite o emissor em vez de imprimir "Toca".
		if h.tenantCompany != nil {
			if name, lookupErr := h.tenantCompany.CompanyName(c.Request.Context(), tenantID); lookupErr == nil {
				rctx.OrgName = name
			}
		}
		if h.crmClient != nil && stmt.OwnerLeadID != "" && !strings.HasPrefix(stmt.OwnerLeadID, "account:") {
			if owner, ownerErr := h.crmClient.GetLead(c.Request.Context(), tenantID, stmt.OwnerLeadID); ownerErr == nil && owner != nil {
				rctx.OwnerName = owner.Name
				rctx.OwnerDocument = owner.Documentation
			}
		}
	}
	pdf, err := service.RenderMonthlyOwnerStatementPDF(rctx)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"demonstrativo-%s.pdf\"", stmt.ReferenceMonth))
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func parseYearMonth(c *gin.Context) (int, int, error) {
	year, _ := strconv.Atoi(c.Query("year"))
	month, _ := strconv.Atoi(c.Query("month"))
	if year < 2020 || year > 2100 {
		return 0, 0, errors.New("year must be between 2020 and 2100")
	}
	if month < 1 || month > 12 {
		return 0, 0, errors.New("month must be between 1 and 12")
	}
	return year, month, nil
}

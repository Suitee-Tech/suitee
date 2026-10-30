package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of the report package for Swagger.
var _ = report.FinancialSummary{}

type FinancialReportHandler struct {
	service *service.FinancialReportService
}

func NewFinancialReportHandler(svc *service.FinancialReportService) *FinancialReportHandler {
	return &FinancialReportHandler{service: svc}
}

// ChartOfAccounts godoc
// @Summary      Plano de contas de vendas e locações
// @Description  Read-only catalogue of ledger accounts and automatic economic classifications.
// @Tags         reports
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=journal.ChartOfAccounts}
// @Failure      403 {object} response.Response
// @Router       /reports/financial/chart-of-accounts [get]
func (h *FinancialReportHandler) ChartOfAccounts(c *gin.Context) {
	if _, err := pkgcontext.GetTenantID(c.Request.Context()); err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	response.Success(c, "Chart of accounts retrieved", journal.GetChartOfAccounts())
}

// Summary godoc
// @Summary      Consolidated financial report
// @Description  Tenant-scoped summary of receivables, withdrawals, distributions and delinquency. Optional from/to window (RFC3339 or YYYY-MM-DD).
// @Tags         reports
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "Period start"
// @Param        to   query string false "Period end"
// @Success      200  {object}  response.Response{data=report.FinancialSummary}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /reports/financial/summary [get]
func (h *FinancialReportHandler) Summary(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// Mesmo parser do DRE e do CashFlow, deliberadamente: com parseReportDate
	// direto, um `to` em data-only virava 00:00:00Z e o último dia do período
	// era silenciosamente descartado aqui, embora fosse incluído nos outros
	// relatórios. O mesmo intervalo visível precisa significar a mesma coisa em
	// todos os endpoints.
	from, to, err := reportRange(c.Request.Context(), tenantID, c.Query("from"), c.Query("to"))
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	summary, err := h.service.Summary(c.Request.Context(), tenantID, &from, &to)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Financial report retrieved", summary)
}

// parseReportDate accepts RFC3339 or YYYY-MM-DD; an empty string means no bound.
func parseReportDate(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return &t, nil
	}
	return nil, errors.New("invalid date")
}

// DRE godoc
// @Summary      Demonstrativo de resultado (DRE)
// @Tags         reports
// @Produce      json
// @Security     BearerAuth
// @Param        from query string true "Period start (RFC3339 or YYYY-MM-DD)"
// @Param        to query string true "Period end (RFC3339 or YYYY-MM-DD)"
// @Success      200 {object} response.Response{data=report.DREReport}
// @Router       /reports/financial/dre [get]
func (h *FinancialReportHandler) DRE(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	from, to, err := reportRange(c.Request.Context(), tenantID, c.Query("from"), c.Query("to"))
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	out, err := h.service.DRE(c.Request.Context(), tenantID, from, to)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "DRE retrieved successfully", out)
}

// CashFlow godoc
// @Summary      Cash flow time series
// @Tags         reports
// @Produce      json
// @Security     BearerAuth
// @Param        from query string true "Period start"
// @Param        to query string true "Period end"
// @Param        granularity query string false "day | week | month (default day)"
// @Success      200 {object} response.Response{data=report.CashFlowReport}
// @Router       /reports/financial/cashflow [get]
func (h *FinancialReportHandler) CashFlow(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	from, to, err := reportRange(c.Request.Context(), tenantID, c.Query("from"), c.Query("to"))
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	gran := report.CashFlowGranularity(strings.ToLower(strings.TrimSpace(c.Query("granularity"))))
	switch gran {
	case report.CashFlowDaily, report.CashFlowWeekly, report.CashFlowMonthly, "":
	default:
		response.ValidationError(c, "granularity must be day | week | month")
		return
	}
	out, err := h.service.CashFlow(c.Request.Context(), tenantID, from, to, gran)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Cashflow retrieved successfully", out)
}

// KPIs godoc
// @Summary      Dashboard KPI snapshot
// @Tags         reports
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=report.KPIs}
// @Router       /reports/financial/kpis [get]
func (h *FinancialReportHandler) KPIs(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	out, err := h.service.Kpis(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "KPIs retrieved successfully", out)
}

// parseReportRange interpreta from/to no dia civil UTC.
//
// Mantida porque é a convenção documentada até aqui e porque os testes de
// intervalo a exercitam diretamente. Os handlers usam reportRange, que aplica o
// fuso da empresa (FIN-11) sobre a mesma forma de entrada.
func parseReportRange(fromStr, toStr string) (time.Time, time.Time, error) {
	return resolveFinancialRange(fromStr, toStr, time.UTC)
}

// reportRange é o intervalo usado pelos relatórios: mesma entrada, mas o dia
// civil é o do fuso configurado pela imobiliária (Configurações → "Fuso
// horário"), não UTC.
func reportRange(ctx context.Context, tenantID, fromStr, toStr string) (time.Time, time.Time, error) {
	return resolveFinancialRange(fromStr, toStr, FinancialDayLocationFor(ctx, tenantID))
}

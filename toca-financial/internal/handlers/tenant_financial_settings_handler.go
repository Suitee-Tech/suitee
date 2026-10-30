package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// TenantFinancialSettingsHandler expõe GET/PUT /financial-settings.
// Permissões:
//   - GET: finance:read (qualquer perfil financeiro consulta defaults)
//   - PUT: finance:write (admin do tenant)
type TenantFinancialSettingsHandler struct {
	service *service.TenantFinancialSettingsService
}

func NewTenantFinancialSettingsHandler(s *service.TenantFinancialSettingsService) *TenantFinancialSettingsHandler {
	return &TenantFinancialSettingsHandler{service: s}
}

// UpdateTenantFinancialSettingsRequest é o payload do PUT. Cada campo é
// pointer para distinguir "não enviado" de "zerado" — o caller pode
// atualizar só multa sem mexer no resto.
type UpdateTenantFinancialSettingsRequest struct {
	DefaultLateFeePct         *float64 `json:"default_late_fee_pct"`
	DefaultMonthlyInterestPct *float64 `json:"default_monthly_interest_pct"`
	DefaultDueDay             *int     `json:"default_due_day"`
	DefaultCommissionPctRent  *float64 `json:"default_commission_pct_rent"`
	DefaultCommissionPctSale  *float64 `json:"default_commission_pct_sale"`
	DefaultCurrency           *string  `json:"default_currency"`
	PayoutProofRequired       *bool    `json:"payout_proof_required"`
	// Alíquotas padrão das retenções sobre comissão (0 desliga a retenção).
	DefaultINSSPct         *float64 `json:"default_inss_pct"`
	DefaultISSPct          *float64 `json:"default_iss_pct"`
	AdminFeePct            *float64 `json:"admin_fee_pct"`
	DefaultLeaseTermMonths *int     `json:"default_lease_term_months"`
	// Política de inadimplência (opcional; ausente = mantém o que está gravado).
	DelinquencyAutoCaseAfterDays *int  `json:"delinquency_auto_case_after_days"`
	HoldTransfersOnDelinquency   *bool `json:"hold_transfers_on_delinquency"`
	DelinquencyHoldMinSharePct   *int  `json:"delinquency_hold_min_share_pct"`
}

// Get godoc
// @Summary      Get tenant financial settings
// @Tags         financial-settings
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=tenant_financial_settings.Settings}
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /financial-settings [get]
func (h *TenantFinancialSettingsHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	s, err := h.service.Get(ctx, tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Financial settings retrieved successfully", s)
}

// Update godoc
// @Summary      Update tenant financial settings
// @Tags         financial-settings
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.UpdateTenantFinancialSettingsRequest true "Update Request"
// @Success      200 {object} response.Response{data=tenant_financial_settings.Settings}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /financial-settings [put]
func (h *TenantFinancialSettingsHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpdateTenantFinancialSettingsRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// Load current and apply only the fields that came in the body —
	// partial update sem perder os defaults nem o updated_by anterior.
	current, err := h.service.Get(ctx, tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if current == nil {
		current = tenant_financial_settings.New(tenantID)
	}
	current.TenantID = tenantID
	if req.DefaultINSSPct != nil {
		current.DefaultINSSPct = *req.DefaultINSSPct
	}
	if req.DefaultISSPct != nil {
		current.DefaultISSPct = *req.DefaultISSPct
	}
	if req.DefaultLateFeePct != nil {
		current.DefaultLateFeePct = *req.DefaultLateFeePct
	}
	if req.DefaultMonthlyInterestPct != nil {
		current.DefaultMonthlyInterestPct = *req.DefaultMonthlyInterestPct
	}
	if req.DefaultDueDay != nil {
		current.DefaultDueDay = *req.DefaultDueDay
	}
	if req.DefaultCommissionPctRent != nil {
		current.DefaultCommissionPctRent = *req.DefaultCommissionPctRent
	}
	if req.DefaultCommissionPctSale != nil {
		current.DefaultCommissionPctSale = *req.DefaultCommissionPctSale
	}
	if req.DefaultCurrency != nil {
		current.DefaultCurrency = *req.DefaultCurrency
	}
	if req.PayoutProofRequired != nil {
		current.PayoutProofRequired = *req.PayoutProofRequired
	}
	if req.AdminFeePct != nil {
		current.AdminFeePct = *req.AdminFeePct
	}
	if req.DelinquencyAutoCaseAfterDays != nil {
		current.DelinquencyAutoCaseAfterDays = *req.DelinquencyAutoCaseAfterDays
	}
	if req.HoldTransfersOnDelinquency != nil {
		current.HoldTransfersOnDelinquency = *req.HoldTransfersOnDelinquency
	}
	if req.DelinquencyHoldMinSharePct != nil {
		current.DelinquencyHoldMinSharePct = *req.DelinquencyHoldMinSharePct
	}
	if req.DefaultLeaseTermMonths != nil {
		current.DefaultLeaseTermMonths = *req.DefaultLeaseTermMonths
	}
	accountID, _ := context.GetAccountID(ctx)
	current.UpdatedBy = accountID

	updated, err := h.service.Update(ctx, current)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Financial settings updated successfully", updated)
}

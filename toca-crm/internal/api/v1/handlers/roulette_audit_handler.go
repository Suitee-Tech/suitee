package handlers

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/dto"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type RouletteAuditHandler struct {
	repo lead.RouletteAuditRepository
}

func NewRouletteAuditHandler(repo lead.RouletteAuditRepository) *RouletteAuditHandler {
	return &RouletteAuditHandler{repo: repo}
}

func (h *RouletteAuditHandler) ListRouletteAudit(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	filter := lead.RouletteAuditFilter{
		Limit:    limit,
		Offset:   offset,
		Action:   c.Query("action"),
		TargetID: c.Query("target_id"),
	}
	if raw := c.Query("from"); raw != "" {
		if t, err := parseAuditDate(raw); err == nil {
			filter.From = &t
		}
	}
	if raw := c.Query("to"); raw != "" {
		if t, err := parseAuditDate(raw); err == nil {
			// Inclui o dia inteiro quando o cliente passou "YYYY-MM-DD".
			if len(raw) == 10 {
				eod := t.Add(24*time.Hour - time.Nanosecond)
				filter.To = &eod
			} else {
				filter.To = &t
			}
		}
	}

	items, total, err := h.repo.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Roulette audit retrieved successfully", gin.H{
		"items":  dto.MapRouletteAuditsToDTO(items),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// parseAuditDate aceita "YYYY-MM-DD" (interpretado como meia-noite UTC)
// ou RFC3339. Mantém compatibilidade com clientes que mandam date pickers
// nativos do browser (HTML <input type="date">).
func parseAuditDate(raw string) (time.Time, error) {
	if len(raw) == 10 {
		return time.Parse("2006-01-02", raw)
	}
	return time.Parse(time.RFC3339, raw)
}

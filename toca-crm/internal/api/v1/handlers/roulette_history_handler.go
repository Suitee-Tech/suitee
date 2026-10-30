package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/dto"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type RouletteHistoryHandler struct {
	repo lead.RouletteHistoryRepository
}

func NewRouletteHistoryHandler(repo lead.RouletteHistoryRepository) *RouletteHistoryHandler {
	return &RouletteHistoryHandler{repo: repo}
}

// ListRouletteHistory godoc
// @Summary      List roulette history
// @Description  List roulette history entries for the tenant
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/roulette/history [get]
func (h *RouletteHistoryHandler) ListRouletteHistory(c *gin.Context) {
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

	items, total, err := h.repo.List(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Roulette history retrieved successfully", gin.H{
				"items":  []dto.RouletteHistoryResponse{},
				"total":  int64(0),
				"limit":  limit,
				"offset": offset,
			})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Roulette history retrieved successfully", gin.H{
		"items":  dto.MapRouletteHistoriesToDTO(items),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

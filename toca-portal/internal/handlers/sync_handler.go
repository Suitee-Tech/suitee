package handlers

import (
	"errors"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

type SyncHandler struct {
	syncService *service.SyncService
}

func NewSyncHandler(syncService *service.SyncService) *SyncHandler {
	return &SyncHandler{syncService: syncService}
}

// syncError responde o erro do caminho de sync: não encontrado (inclusive
// integração de outro tenant) → 404, resto → 500 genérico com o detalhe só no
// log do servidor.
func syncError(c *gin.Context, op string, err error) {
	if errors.Is(err, integration.ErrNotFound) {
		response.NotFound(c, "Integration not found")
		return
	}
	log.Printf("[SyncHandler] %s failed: %v", op, err)
	response.InternalServerError(c, "failed to process sync request")
}

// TriggerSync godoc
// @Summary      Trigger sync
// @Description  Manually trigger a sync run for an integration (rebuilds the feed). The integration must belong to the caller's tenant.
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Integration ID"
// @Success      200  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /integrations/{id}/sync [post]
func (h *SyncHandler) TriggerSync(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	syncRecord, err := h.syncService.TriggerSync(c.Request.Context(), tenantID, id)
	if err != nil {
		syncError(c, "TriggerSync", err)
		return
	}

	response.Success(c, "Sync triggered successfully", syncRecord)
}

// ListSyncs godoc
// @Summary      List sync history
// @Description  Return paginated sync history for an integration of the caller's tenant
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      string  true   "Integration ID"
// @Param        limit   query     int     false  "Limit (default 20)"
// @Param        offset  query     int     false  "Offset"
// @Success      200     {object}  response.Response
// @Failure      404     {object}  response.Response
// @Failure      500     {object}  response.Response
// @Router       /integrations/{id}/syncs [get]
func (h *SyncHandler) ListSyncs(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	syncs, total, err := h.syncService.ListSyncs(c.Request.Context(), tenantID, id, limit, offset)
	if err != nil {
		syncError(c, "ListSyncs", err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Syncs retrieved successfully", syncs, page, int(limit), total)
}

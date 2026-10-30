package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

type FeedHandler struct {
	integService *service.IntegrationService
	feedService  *service.FeedService
}

func NewFeedHandler(integService *service.IntegrationService, feedService *service.FeedService) *FeedHandler {
	return &FeedHandler{
		integService: integService,
		feedService:  feedService,
	}
}

// ServeFeed godoc
// @Summary      Serve portal XML feed
// @Description  Public endpoint crawled by portal bots (ZAP, OLX, VivaReal). Returns cached VRSync XML.
// @Tags         portals
// @Produce      xml
// @Param        token  path      string  true  "Feed token (per-integration)"
// @Success      200    {string}  string  "XML feed"
// @Failure      400    {string}  string  "Missing feed token"
// @Failure      404    {string}  string  "Feed not found"
// @Failure      503    {string}  string  "Feed not available yet"
// @Router       /portals/feed/{token} [get]
func (h *FeedHandler) ServeFeed(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "Missing feed token")
		return
	}

	// Validate User-Agent (optional, log unknown agents)
	// userAgent := c.GetHeader("User-Agent")
	// Known agents: "VivaRealBot/1.0", "ZapBot/1.0", etc.

	// Look up integration by feed token
	integ, err := h.integService.GetByFeedToken(c.Request.Context(), token)
	if err != nil {
		c.String(http.StatusNotFound, "Feed not found")
		return
	}

	if integ.Status != integration.IntegrationActive {
		c.String(http.StatusServiceUnavailable, "Integration is not active")
		return
	}

	// Serve cached XML
	xmlContent, err := h.feedService.GetCachedFeed(c.Request.Context(), integ)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Feed not available yet. Please try again later.")
		return
	}

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=3600")
	c.String(http.StatusOK, xmlContent)
}

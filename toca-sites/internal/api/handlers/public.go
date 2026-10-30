package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
	"github.com/oginoino/toca/toca-sites/internal/services"
)

// PublicHandler é a borda anônima da vitrine: beacon de view e formulário de
// interesse. Ambos resolvem o tenant PELO HOST; nada no corpo escolhe a
// imobiliária.
type PublicHandler struct {
	engagement *services.EngagementService
}

func NewPublicHandler(engagement *services.EngagementService) *PublicHandler {
	return &PublicHandler{engagement: engagement}
}

// CreateView godoc
// @Summary      Record a listing pageview
// @Tags         sites-public
// @Router       /sites/public/views [post]
func (h *PublicHandler) CreateView(c *gin.Context) {
	var in site.ViewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.ValidationError(c, "payload inválido")
		return
	}
	if err := h.engagement.RecordView(c.Request.Context(), siteHost(c), in, requestMeta(c)); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "view registrada"})
}

// CreateLead godoc
// @Summary      Receive an interest lead from the public site
// @Tags         sites-public
// @Router       /sites/public/leads [post]
func (h *PublicHandler) CreateLead(c *gin.Context) {
	var in site.LeadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "message": "payload muito grande"})
			return
		}
		response.ValidationError(c, "payload inválido")
		return
	}
	lead, err := h.engagement.CreateLead(c.Request.Context(), siteHost(c), in, requestMeta(c))
	if err != nil {
		writeError(c, err)
		return
	}
	status := http.StatusCreated
	message := "Recebemos seu contato. A imobiliária falará com você."
	if lead.Status == site.LeadDuplicate {
		status = http.StatusOK
		message = "Recebemos seu contato. A imobiliária falará com você."
	}
	c.JSON(status, gin.H{
		"success": true,
		"message": message,
		"data": gin.H{
			"lead_id": lead.ID,
			"status":  lead.Status,
		},
	})
}

// siteHost devolve o host efetivo do site. O Next (SSR) encaminha o Host
// original em X-Site-Host; browser chama direto e o Host da conexão é o do
// gateway — nesse caso o proxy preserva X-Forwarded-Host, mas o header
// explícito é a fonte preferida.
func siteHost(c *gin.Context) string {
	if h := strings.TrimSpace(c.GetHeader("X-Site-Host")); h != "" {
		return h
	}
	if h := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); h != "" {
		return h
	}
	return c.Request.Host
}

func requestMeta(c *gin.Context) services.RequestMeta {
	country := c.GetHeader("CF-IPCountry")
	if country == "" {
		country = c.GetHeader("X-Country")
	}
	return services.RequestMeta{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Country:   country,
	}
}

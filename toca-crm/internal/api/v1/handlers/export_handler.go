// Handler de exports — Issue #474. Endpoints HTTP em cima do
// pkg/exports.Service. Mantemos no toca-crm porque é o serviço que
// possui os dados (leads); outros services que ofereçam export podem
// repetir esse handler com a entity type deles.
package handlers

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/exports"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

type ExportHandler struct {
	service *exports.Service
}

func NewExportHandler(svc *exports.Service) *ExportHandler {
	return &ExportHandler{service: svc}
}

// CreateLeadExportRequest — payload do POST /leads/export. Filtros
// repassados como map para o generator decidir o que aceita; mantém
// o handler agnóstico do shape do generator. UI passa o mesmo filtro
// usado na listagem (status, source, broker_id, created_from/to).
type CreateLeadExportRequest struct {
	Format  string                 `json:"format" binding:"required"`
	Filters map[string]interface{} `json:"filters,omitempty"`
}

// CreateLeadExport godoc
// @Summary      Criar export async de leads (#474)
// @Description  Enfileira CSV/PDF dos leads filtrados. Retorna 202 +
// @Description  job ID; cliente faz polling em GET /exports/:id ou
// @Description  abre a tela "Meus exports".
// @Tags         exports
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body CreateLeadExportRequest true "Export request"
// @Success      202 {object} response.Response{data=exports.Job}
// @Failure      400 {object} response.Response
// @Router       /leads/export [post]
func (h *ExportHandler) CreateLeadExport(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "account context required")
		return
	}

	var req CreateLeadExportRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	format := exports.Format(req.Format)
	if !format.IsValid() {
		response.ValidationError(c, "format inválido — use csv ou pdf")
		return
	}

	job, err := h.service.Submit(c.Request.Context(), exports.SubmitInput{
		TenantID:    tenantID,
		RequestedBy: accountID,
		EntityType:  "Lead",
		Format:      format,
		Filters:     req.Filters,
	})
	if err != nil {
		if errors.Is(err, exports.ErrGeneratorNotFound) {
			response.ValidationError(c, "format/entity sem generator registrado")
			return
		}
		response.InternalError(c, err)
		return
	}
	c.JSON(202, gin.H{"success": true, "message": "Export enqueued", "data": job})
}

// GetExport godoc
// @Summary      Status de um export job
// @Tags         exports
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Job ID"
// @Success      200 {object} response.Response{data=exports.Job}
// @Failure      404 {object} response.Response
// @Router       /exports/{id} [get]
func (h *ExportHandler) GetExport(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	id := c.Param("id")
	job, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, exports.ErrJobNotFound) {
			response.NotFound(c, "export job not found")
			return
		}
		response.InternalError(c, err)
		return
	}

	// P0/A21: o job carrega `ResultURL` (URL pré-assinada do CSV completo de
	// leads, válida por dias). O filtro por tenant não bastava: qualquer colega
	// do mesmo tenant lia o export alheio. Só o solicitante — ou quem tem perfil
	// de gestão — acessa.
	if !callerOwnsExport(c, job.RequestedBy) {
		response.NotFound(c, "export job not found")
		return
	}

	response.Success(c, "Export job retrieved", job)
}

// callerOwnsExport decide se o chamador pode ler um job de export.
//
// SECURITY (achado A21): o payload carrega a URL pré-assinada do CSV completo de
// leads. PerfIS de gestão (admin/owner/manager/gestor/operacoes) continuam
// podendo inspecionar jobs do tenant — é uma necessidade operacional —, os
// demais só veem os próprios.
func callerOwnsExport(c *gin.Context, requestedBy string) bool {
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if requestedBy == "" || accountID == requestedBy {
		return true
	}
	profile, _ := pkgcontext.GetUserPermission(c.Request.Context())
	role, _ := pkgcontext.GetFunctionalRole(c.Request.Context())
	baseRole, _ := pkgcontext.GetBaseRole(c.Request.Context())
	return pkgAuth.HasTenantWideScope(profile, role, baseRole)
}

// ListMyExports godoc
// @Summary      Listar exports do usuário corrente
// @Tags         exports
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int false "Limit (default 50, max 200)"
// @Param        offset query int false "Offset (default 0)"
// @Success      200 {object} response.Response{data=[]exports.Job}
// @Router       /exports/mine [get]
func (h *ExportHandler) ListMyExports(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "account context required")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	jobs, err := h.service.ListByRequester(c.Request.Context(), tenantID, accountID, limit, offset)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Exports retrieved", jobs)
}

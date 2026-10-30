package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

type TemplateHandler struct {
	service *service.TemplateService
}

func NewTemplateHandler(service *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{
		service: service,
	}
}

type CreateTemplateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Content     string   `json:"content" binding:"required"` // Base64 encoded DOCX
	Variables   []string `json:"variables"`
	// Kind — "contract" (default) ou "distrato" (#436). Vazio = contract.
	Kind string `json:"kind"`
}

// resolveTemplateKind normaliza o kind vindo do request: vazio vira
// contract; valor desconhecido devolve ok=false para o handler responder
// 400 em vez de gravar um kind inválido.
func resolveTemplateKind(raw string) (contract.TemplateKind, bool) {
	if raw == "" {
		return contract.TemplateKindContract, true
	}
	k := contract.TemplateKind(raw)
	return k, contract.ValidTemplateKind(k)
}

func (h *TemplateHandler) Create(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Unauthorized(c, "tenant_id required")
		return
	}

	var req CreateTemplateRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	kind, ok := resolveTemplateKind(req.Kind)
	if !ok {
		response.ValidationError(c, "invalid template kind")
		return
	}

	tpl := &contract.ContractTemplate{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Variables:   req.Variables,
		Kind:        kind,
		TenantID:    tenantID,
	}

	if err := h.service.CreateTemplate(c.Request.Context(), tpl); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Template created successfully", tpl)
}

func (h *TemplateHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Unauthorized(c, "tenant_id required")
		return
	}

	templates, err := h.service.ListTemplates(c.Request.Context(), tenantID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Templates retrieved successfully", []*contract.ContractTemplate{})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Templates retrieved successfully", templates)
}

func (h *TemplateHandler) GetByID(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Unauthorized(c, "tenant_id required")
		return
	}

	id := c.Param("id")
	tpl, err := h.service.GetTemplate(c.Request.Context(), tenantID, id)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if tpl == nil {
		response.NotFound(c, "template not found")
		return
	}

	response.Success(c, "Template retrieved successfully", tpl)
}

func (h *TemplateHandler) Update(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Unauthorized(c, "tenant_id required")
		return
	}

	id := c.Param("id")
	var req CreateTemplateRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	kind, ok := resolveTemplateKind(req.Kind)
	if !ok {
		response.ValidationError(c, "invalid template kind")
		return
	}

	tpl := &contract.ContractTemplate{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Variables:   req.Variables,
		Kind:        kind,
		TenantID:    tenantID,
	}

	if err := h.service.UpdateTemplate(c.Request.Context(), tpl); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Template updated successfully", tpl)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Unauthorized(c, "tenant_id required")
		return
	}

	id := c.Param("id")
	if err := h.service.DeleteTemplate(c.Request.Context(), id, tenantID); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Template deleted successfully", nil)
}

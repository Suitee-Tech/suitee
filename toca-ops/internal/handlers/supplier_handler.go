package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

// SupplierHandler expõe o cadastro de profissionais/fornecedores da imobiliária.
type SupplierHandler struct {
	service *services.SupplierService
}

func NewSupplierHandler(svc *services.SupplierService) *SupplierHandler {
	return &SupplierHandler{service: svc}
}

type CreateSupplierRequest struct {
	Name        string `json:"name" binding:"required"`
	ServiceType string `json:"service_type" binding:"required"`
	Kind        string `json:"kind"`
	Document    string `json:"document"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Notes       string `json:"notes"`
	City        string `json:"city"`
}

// UpdateSupplierRequest usa ponteiros para distinguir "não informado" de
// "limpar o campo".
type UpdateSupplierRequest struct {
	Name        *string `json:"name"`
	ServiceType *string `json:"service_type"`
	Kind        *string `json:"kind"`
	Document    *string `json:"document"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Notes       *string `json:"notes"`
	City        *string `json:"city"`
	Active      *bool   `json:"active"`
}

func (h *SupplierHandler) Create(c *gin.Context) {
	var req CreateSupplierRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	created, reused, err := h.service.Create(c.Request.Context(), supplier.New(
		req.Name, req.ServiceType, req.Kind, req.Document, req.Email, req.Phone, req.Notes, req.City, tenantID,
	))
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	if reused {
		// Cadastro já existente: devolve o registro para o chamado reutilizar o
		// mesmo prestador em vez de criar um duplicado.
		response.Success(c, "Profissional já cadastrado; reutilizando o registro existente", created)
		return
	}
	response.Created(c, "Profissional cadastrado com sucesso", created)
}

func (h *SupplierHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	items, total, err := h.service.List(c.Request.Context(), tenantID, supplier.Filter{
		Query:       c.Query("q"),
		ServiceType: c.Query("service_type"),
		ActiveOnly:  c.Query("active_only") == "true",
		Limit:       int64(limit),
		Offset:      int64(offset),
	})
	if err != nil {
		response.InternalError(c, err)
		return
	}
	page := offset/limit + 1
	response.Paginated(c, "Profissionais listados com sucesso", items, page, limit, total)
}

// ServiceTypes alimenta o formulário com a nomenclatura já usada pela
// imobiliária, evitando variações do mesmo serviço.
func (h *SupplierHandler) ServiceTypes(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	types, err := h.service.ServiceTypes(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Tipos de serviço listados com sucesso", types)
}

func (h *SupplierHandler) GetByID(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	v, err := h.service.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Profissional recuperado com sucesso", v)
}

func (h *SupplierHandler) Update(c *gin.Context) {
	var req UpdateSupplierRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	updated, err := h.service.Update(c.Request.Context(), tenantID, c.Param("id"), supplier.Changes{
		Name:        req.Name,
		ServiceType: req.ServiceType,
		Kind:        req.Kind,
		Document:    req.Document,
		Email:       req.Email,
		Phone:       req.Phone,
		Notes:       req.Notes,
		City:        req.City,
		Active:      req.Active,
	})
	if err != nil {
		if strings.Contains(err.Error(), "não encontrado") {
			response.NotFound(c, err.Error())
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Profissional atualizado com sucesso", updated)
}

func (h *SupplierHandler) Delete(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	if err := h.service.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Profissional removido com sucesso", nil)
}

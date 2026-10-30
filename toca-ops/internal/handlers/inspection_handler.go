package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

// Limites do upload multipart (#440). Conservadores: 5 fotos × 10MB
// cada = 50MB max no parse. Compressão JPEG no client antes do upload
// é encorajada, mas o backend tolera bytes maiores até esse teto.
const (
	maxPhotosPerUpload      = 5
	maxPhotoBytes           = 10 << 20 // 10 MiB por foto
	maxMultipartMemoryBytes = 5 * 10 << 20
)

type InspectionHandler struct {
	service *services.InspectionService
}

func NewInspectionHandler(svc *services.InspectionService) *InspectionHandler {
	return &InspectionHandler{service: svc}
}

type CreateInspectionRequest struct {
	TimeZone      string               `json:"time_zone"`
	PropertyID    string               `json:"property_id" binding:"required"`
	ContractID    string               `json:"contract_id"`
	Type          string               `json:"type" binding:"required"`
	ScheduledDate time.Time            `json:"scheduled_date" binding:"required"`
	InspectorID   string               `json:"inspector_id" binding:"required"`
	InspectorName string               `json:"inspector_name" binding:"required"`
	Notes         string               `json:"notes"`
	ReportURL     string               `json:"report_url"`
	Findings      []inspection.Finding `json:"findings"`
}

type UpdateInspectionRequest struct {
	TimeZone      string               `json:"time_zone"`
	Type          string               `json:"type"`
	ScheduledDate *time.Time           `json:"scheduled_date"`
	CompletedDate *time.Time           `json:"completed_date"`
	InspectorID   string               `json:"inspector_id"`
	InspectorName string               `json:"inspector_name"`
	Status        string               `json:"status"`
	Notes         *string              `json:"notes"`
	ReportURL     string               `json:"report_url"`
	Findings      []inspection.Finding `json:"findings"`
}

func (h *InspectionHandler) Create(c *gin.Context) {
	var req CreateInspectionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	insp := inspection.New(
		req.PropertyID, req.ContractID, req.Type,
		req.InspectorID, req.InspectorName, tenantID,
		req.ScheduledDate,
	)
	if req.TimeZone == "" {
		req.TimeZone = "America/Sao_Paulo"
	}
	if _, err := time.LoadLocation(req.TimeZone); err != nil {
		response.ValidationError(c, "Fuso horário inválido")
		return
	}
	insp.TimeZone = req.TimeZone
	insp.ScheduledBy, _ = context.GetAccountID(c.Request.Context())
	insp.Notes = req.Notes
	insp.ReportURL = req.ReportURL
	if req.Findings != nil {
		insp.Findings = req.Findings
	}

	if err := h.service.CreateAssigned(c.Request.Context(), insp); err != nil {
		if errors.Is(err, services.ErrInvalidInspector) {
			response.ValidationError(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Inspection created successfully", insp)
}

// ListByContractInternal serves the internal sibling call from toca-deal so the
// contract termination handler can validate "exit inspection completed" without
// going through tenant-scoped auth. Requires internal key (wired in routes.go).
// Returns up to 50 inspections for the contract.
func (h *InspectionHandler) ListByContractInternal(c *gin.Context) {
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	contractID := c.Query("contract_id")
	if contractID == "" {
		response.ValidationError(c, "contract_id is required")
		return
	}
	inspType := c.Query("type")
	status := c.Query("status")

	inspections, total, err := h.service.List(c.Request.Context(), inspection.ListFilter{
		TenantID:   tenantID,
		ContractID: contractID,
		Status:     status,
		Type:       inspType,
		Limit:      50,
	})
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Paginated(c, "Inspections retrieved successfully", inspections, 1, 50, total)
}

func (h *InspectionHandler) GetByID(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	insp, err := h.service.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Inspection retrieved successfully", insp)
}

// GenerateLaudo godoc
// @Summary      Gerar PDF do laudo de vistoria (#435)
// @Description  Renderiza o laudo a partir das Findings registradas
// @Description  (HTML → Gotenberg), faz upload no storage e grava a
// @Description  ReportURL na inspeção. Regerar sobrescreve a versão
// @Description  anterior. Necessário antes de marcar a vistoria como
// @Description  Concluída (que exige ReportURL preenchida).
// @Tags         inspections
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Inspection ID"
// @Success      200 {object} response.Response{data=inspection.Inspection}
// @Failure      404 {object} response.Response
// @Failure      503 {object} response.Response "renderer de PDF indisponível"
// @Router       /inspections/{id}/generate-laudo [post]
func (h *InspectionHandler) GenerateLaudo(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	insp, err := h.service.GenerateAndAttachLaudo(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPDFRendererUnavailable):
			response.Error(c, http.StatusServiceUnavailable,
				"Geração de PDF temporariamente indisponível. Anexe o laudo manualmente ou tente mais tarde.")
		case errors.Is(err, services.ErrInspectionNotFound):
			response.NotFound(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}
	response.Success(c, "Laudo gerado com sucesso", insp)
}

func (h *InspectionHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateInspectionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.ScheduledDate != nil {
		existing.ScheduledDate = *req.ScheduledDate
	}
	if req.CompletedDate != nil {
		existing.CompletedDate = req.CompletedDate
	}
	if req.InspectorID != "" {
		existing.InspectorID = req.InspectorID
	}
	if req.InspectorName != "" {
		existing.InspectorName = req.InspectorName
	}
	if req.Status != "" {
		existing.Status = req.Status
	}
	if req.Notes != nil {
		existing.Notes = *req.Notes
	}
	if req.TimeZone != "" {
		if _, err := time.LoadLocation(req.TimeZone); err != nil {
			response.ValidationError(c, "Fuso horário inválido")
			return
		}
		existing.TimeZone = req.TimeZone
	}
	if req.ReportURL != "" {
		existing.ReportURL = req.ReportURL
	}
	if req.Findings != nil {
		existing.Findings = req.Findings
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		if errors.Is(err, services.ErrInvalidInspector) {
			response.ValidationError(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Inspection updated successfully", existing)
}

// RecordDeductionsRequest is the input for POST /inspections/:id/deductions.
// Each item becomes a Charge in toca-financial via the
// inspection.deductions_recorded event consumer.
type RecordDeductionsRequest struct {
	Deductions []inspection.Deduction `json:"deductions" binding:"required"`
}

func (h *InspectionHandler) RecordDeductions(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req RecordDeductionsRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	insp, err := h.service.RecordDeductions(c.Request.Context(), tenantID, id, req.Deductions)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Deductions recorded successfully", insp)
}

func (h *InspectionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Inspection deleted successfully", nil)
}

// InspectionTemplateRoom espelha o RoomTemplate do frontend. Issue #432:
// permite cômodos configuráveis por tenant. Para v1 servimos o template
// default residencial; tenants que precisem customizar podem ter
// overrides cadastrados via seed/admin tool (próximo PR adiciona a
// persistência por tenant).
type InspectionTemplateRoom struct {
	Name  string                       `json:"name"`
	Items []InspectionTemplateRoomItem `json:"items"`
}

type InspectionTemplateRoomItem struct {
	Description string `json:"description"`
}

// defaultResidentialTemplate é a baseline residencial — 5 cômodos com
// itens comuns para inquilino + locador checarem na vistoria. Match
// o DEFAULT_RESIDENTIAL_TEMPLATE do frontend (toka/components/inspection
// /inspectionTemplates.ts) — mantemos as duas fontes sincronizadas até
// um próximo PR mover a fonte de verdade para o backend exclusivamente.
var defaultResidentialTemplate = []InspectionTemplateRoom{
	{Name: "Sala", Items: []InspectionTemplateRoomItem{
		{Description: "Piso"},
		{Description: "Paredes / Pintura"},
		{Description: "Teto"},
		{Description: "Janelas e esquadrias"},
		{Description: "Tomadas e interruptores"},
	}},
	{Name: "Cozinha", Items: []InspectionTemplateRoomItem{
		{Description: "Pia e bancada"},
		{Description: "Armários"},
		{Description: "Azulejos"},
		{Description: "Ralo e encanamento"},
		{Description: "Exaustor / coifa"},
	}},
	{Name: "Quarto", Items: []InspectionTemplateRoomItem{
		{Description: "Piso"},
		{Description: "Paredes / Pintura"},
		{Description: "Janelas e esquadrias"},
		{Description: "Closet / armários embutidos"},
	}},
	{Name: "Banheiro", Items: []InspectionTemplateRoomItem{
		{Description: "Pia e box"},
		{Description: "Vaso sanitário"},
		{Description: "Azulejos"},
		{Description: "Chuveiro e torneiras"},
		{Description: "Ralos e encanamento"},
	}},
	{Name: "Área Externa", Items: []InspectionTemplateRoomItem{
		{Description: "Varanda / sacada"},
		{Description: "Piso externo"},
		{Description: "Iluminação externa"},
	}},
}

// GetTemplate godoc
//
// @Summary      Get inspection template for tenant
// @Description  Devolve o template de checklist da vistoria configurado para o tenant atual. Quando não há override, devolve o template default residencial. Issue #432.
// @Tags         inspections
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]InspectionTemplateRoom}
// @Failure      403  {object}  response.Response  "tenant context required"
// @Router       /inspections/template [get]
func (h *InspectionHandler) GetTemplate(c *gin.Context) {
	_, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	response.Success(c, "Inspection template retrieved", defaultResidentialTemplate)
}

func (h *InspectionHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	propertyID := c.Query("property_id")
	contractID := c.Query("contract_id")
	status := c.Query("status")
	inspType := c.Query("type")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	inspections, total, err := h.service.List(c.Request.Context(), inspection.ListFilter{
		TenantID:   tenantID,
		PropertyID: propertyID,
		ContractID: contractID,
		Status:     status,
		Type:       inspType,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Inspections retrieved successfully", []*inspection.Inspection{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Inspections retrieved successfully", inspections, page, int(limit), total)
}

// UploadFindingPhotos godoc
//
// @Summary      Upload photos to an inspection finding
// @Description  Multipart endpoint que aceita até 5 fotos para uma Finding específica da Inspection. Backend processa (resize 1920px largest side, JPEG q=85), faz upload para storage privado prefixed por tenant_id, devolve URLs e anexa em Finding.Photos. Issue #440.
// @Tags         inspections
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Inspection ID"
// @Param        finding_id path string true "Finding ID (UUID estável dentro da Inspection)"
// @Param        files formData file true "Foto (1..5 arquivos no campo files)"
// @Success      200  {object}  response.Response{data=object}
// @Failure      400  {object}  response.Response  "request malformado"
// @Failure      403  {object}  response.Response  "tenant context required"
// @Failure      404  {object}  response.Response  "inspection / finding not found"
// @Failure      422  {object}  response.Response  "nenhuma foto válida processada"
// @Failure      500  {object}  response.Response
// @Router       /inspections/{id}/findings/{finding_id}/photos [post]
func (h *InspectionHandler) UploadFindingPhotos(c *gin.Context) {
	inspectionID := c.Param("id")
	findingID := c.Param("finding_id")
	if inspectionID == "" || findingID == "" {
		response.ValidationError(c, "inspection id and finding id are required")
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actorID, _ := context.GetAccountID(c.Request.Context())
	actorName, _ := context.GetUserName(c.Request.Context())

	if err := c.Request.ParseMultipartForm(maxMultipartMemoryBytes); err != nil {
		response.ValidationError(c, "invalid multipart form: "+err.Error())
		return
	}

	form := c.Request.MultipartForm
	if form == nil || form.File == nil {
		response.ValidationError(c, "multipart form is empty")
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		if single, ok := form.File["file"]; ok && len(single) > 0 {
			files = single
		}
	}
	if len(files) == 0 {
		response.ValidationError(c, "no files in 'files' field of multipart form")
		return
	}
	if len(files) > maxPhotosPerUpload {
		response.ValidationError(c, fmt.Sprintf("max %d photos per request", maxPhotosPerUpload))
		return
	}

	uploads := make([]services.FindingPhotoUpload, 0, len(files))
	for _, fh := range files {
		if fh.Size > maxPhotoBytes {
			response.ValidationError(c, fmt.Sprintf("file %q exceeds %d bytes", fh.Filename, maxPhotoBytes))
			return
		}
		f, oerr := fh.Open()
		if oerr != nil {
			response.InternalError(c, oerr)
			return
		}
		buf, rerr := io.ReadAll(f)
		_ = f.Close()
		if rerr != nil {
			response.InternalError(c, rerr)
			return
		}
		uploads = append(uploads, services.FindingPhotoUpload{
			FileName:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			RawBytes:    buf,
		})
	}

	result, err := h.service.UploadFindingPhotos(c.Request.Context(), tenantID, inspectionID, findingID, actorID, actorName, uploads)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrFindingNotFound):
			response.NotFound(c, err.Error())
			return
		case errors.Is(err, services.ErrTooManyPhotos):
			response.ValidationError(c, err.Error())
			return
		case errors.Is(err, services.ErrInspectionNotFound):
			response.NotFound(c, err.Error())
			return
		case err.Error() == "storage service not configured":
			response.InternalError(c, err)
			return
		case err.Error() == "no photos were processed successfully":
			response.UnprocessableEntity(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	urls := result.URLs
	response.Success(c, "Photos uploaded successfully", gin.H{
		"inspection_id": inspectionID,
		"finding_id":    findingID,
		"urls":          urls,
		"thumb_urls":    result.Thumbnails,
		"uploaded":      len(urls),
	})
}

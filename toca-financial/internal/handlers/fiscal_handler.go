package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of fiscal package for Swagger
var _ = fiscal.Nfse{}

type FiscalHandler struct {
	service *service.FiscalService
}

func NewFiscalHandler(service *service.FiscalService) *FiscalHandler {
	return &FiscalHandler{service: service}
}

// ── Calendar ──

// GetCalendar godoc
// @Summary      Get fiscal calendar
// @Description  Returns fiscal deadlines for the year
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        year query int false "Year"
// @Success      200  {object}  response.Response{data=[]fiscal.FiscalDeadline}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/calendar [get]
func (h *FiscalHandler) GetCalendar(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	year, _ := strconv.Atoi(c.Query("year"))

	deadlines, err := h.service.GetCalendar(c.Request.Context(), tenantID, year)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Fiscal calendar retrieved successfully", deadlines)
}

// ── DIMOB ──

// GetDimobStatus godoc
// @Summary      Get DIMOB report status
// @Description  Returns the DIMOB report status for a given year
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        year path int true "Year"
// @Success      200  {object}  response.Response{data=fiscal.DimobReport}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/dimob/{year} [get]
func (h *FiscalHandler) GetDimobStatus(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	year, err := strconv.Atoi(c.Param("year"))
	if err != nil {
		response.ValidationError(c, "invalid year")
		return
	}

	report, err := h.service.GetDimobStatus(c.Request.Context(), tenantID, year)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "DIMOB status retrieved successfully", report)
}

type GenerateDimobRequest struct {
	Year int `json:"year" binding:"required"`
	// CompanyCNPJ / CompanyName identificam a declarante (a imobiliária)
	// no registro R01 do TXT da DIMOB (#418). Opcionais no binding para
	// não quebrar callers atuais, mas necessários para baixar o arquivo —
	// sem CNPJ, GET /fiscal/dimob/{year}/file responde 400.
	CompanyCNPJ string `json:"company_cnpj"`
	CompanyName string `json:"company_name"`
}

// GenerateDimob godoc
// @Summary      Generate DIMOB report
// @Description  Generates the DIMOB report for a given year
// @Tags         fiscal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.GenerateDimobRequest true "Generate DIMOB Request"
// @Success      201  {object}  response.Response{data=fiscal.DimobReport}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/dimob/generate [post]
func (h *FiscalHandler) GenerateDimob(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req GenerateDimobRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	report, err := h.service.GenerateDimob(c.Request.Context(), tenantID, req.Year, req.CompanyCNPJ, req.CompanyName)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Created(c, "DIMOB report generated successfully", report)
}

// DownloadDimob godoc
// @Summary      Download DIMOB TXT file
// @Description  Builds and streams the DIMOB declaration as a fixed-width TXT file in the RFB import layout
// @Tags         fiscal
// @Produce      plain
// @Security     BearerAuth
// @Param        year path int true "Year"
// @Success      200  {file}    file
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/dimob/{year}/file [get]
func (h *FiscalHandler) DownloadDimob(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	year, err := strconv.Atoi(c.Param("year"))
	if err != nil {
		response.ValidationError(c, "invalid year")
		return
	}

	filename, content, warnings, err := h.service.BuildDimobFile(c.Request.Context(), tenantID, year)
	if err != nil {
		// ErrDimobNotReady cobre os casos corrigíveis pelo usuário
		// (declaração não gerada / sem CNPJ / endereço ou CEP pendentes) — 400.
		if errors.Is(err, service.ErrDimobNotReady) {
			response.ValidationError(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}

	if len(warnings) > 0 {
		c.Header("X-Dimob-Warnings", headerSafeText(strings.Join(warnings, " | ")))
	}
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
}

// headerSafeText deixa um texto seguro para virar cabeçalho HTTP: cabeçalho
// não carrega acento (o toast do operador chegava com "locaÃ§Ã£o"), então os
// acentos são transliterados e o que sobrar de fora do ASCII imprimível vira
// espaço. O conteúdo do arquivo não passa por aqui — é só a mensagem.
func headerSafeText(value string) string {
	accents := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"Á", "A", "À", "A", "Ã", "A", "Â", "A", "Ä", "A",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
		"Ó", "O", "Ò", "O", "Õ", "O", "Ô", "O", "Ö", "O",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"ç", "c", "Ç", "C", "ñ", "n", "Ñ", "N",
	)
	// Pontuação "tipográfica" também não cabe no cabeçalho: vira o equivalente
	// ASCII para a mensagem não perder o sentido (travessão virava três espaços).
	punctuation := strings.NewReplacer(
		"\u2014", "-", "\u2013", "-", "\u2012", "-", "\u2015", "-",
		"\u2018", "'", "\u2019", "'", "\u201c", "\"", "\u201d", "\"",
		"\u2026", "...", "\u00a0", " ", "\u00ba", "o", "\u00aa", "a",
	)
	clean := []rune(punctuation.Replace(accents.Replace(value)))
	out := make([]rune, 0, len(clean))
	for _, r := range clean {
		if r >= 0x20 && r <= 0x7E {
			out = append(out, r)
			continue
		}
		out = append(out, ' ')
	}
	return string(out)
}

// ── NFS-e ──

// ListNfse godoc
// @Summary      List NFS-e
// @Description  List issued NFS-e with optional filters
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        month  query string false "Reference month (YYYY-MM)"
// @Param        status query string false "Status filter"
// @Param        limit  query int    false "Limit"
// @Param        offset query int    false "Offset"
// @Success      200  {object}  response.Response{data=[]fiscal.Nfse}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/nfse [get]
func (h *FiscalHandler) ListNfse(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	month := c.Query("month")
	status := c.Query("status")
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	nfses, total, err := h.service.ListNfse(c.Request.Context(), tenantID, month, status, limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "NFS-e retrieved successfully", nfses, page, int(limit), total)
}

type IssueNfseRequest struct {
	ReferenceMonth     string  `json:"reference_month" binding:"required"`
	RecipientID        string  `json:"recipient_id" binding:"required"`
	Amount             float64 `json:"amount" binding:"required,gt=0"`
	ServiceDescription string  `json:"service_description" binding:"required"`
}

// IssueNfse godoc
// @Summary      Issue NFS-e
// @Description  Issues a new NFS-e
// @Tags         fiscal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.IssueNfseRequest true "Issue NFS-e Request"
// @Success      201  {object}  response.Response{data=fiscal.Nfse}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/nfse [post]
func (h *FiscalHandler) IssueNfse(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req IssueNfseRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	nfse, err := h.service.IssueNfse(c.Request.Context(), tenantID, req.ReferenceMonth, req.RecipientID, req.Amount, req.ServiceDescription)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Created(c, "NFS-e issued successfully", nfse)
}

// ── Informes ──

// ListInformes godoc
// @Summary      List Informes de Rendimentos
// @Description  Returns informes de rendimentos for a given year
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        year query int false "Year"
// @Success      200  {object}  response.Response{data=[]fiscal.Informe}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/informes [get]
func (h *FiscalHandler) ListInformes(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	year, _ := strconv.Atoi(c.Query("year"))

	informes, err := h.service.ListInformes(c.Request.Context(), tenantID, year)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Informes retrieved successfully", informes)
}

type GenerateInformesRequest struct {
	Year int `json:"year" binding:"required"`
}

// GenerateInformes godoc
// @Summary      Generate Informes de Rendimentos
// @Description  Generates informes de rendimentos for all owners for a given year
// @Tags         fiscal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.GenerateInformesRequest true "Generate Informes Request"
// @Success      201  {object}  response.Response{data=[]fiscal.Informe}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/informes/generate [post]
func (h *FiscalHandler) GenerateInformes(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req GenerateInformesRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	informes, err := h.service.GenerateInformes(c.Request.Context(), tenantID, req.Year)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Created(c, "Informes generated successfully", informes)
}

// DownloadInformePDF godoc
// @Summary      Download informe de rendimentos PDF
// @Description  Generates and streams an owner's income report (informe de rendimentos) as a PDF
// @Tags         fiscal
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Informe ID"
// @Success      200  {file}  binary
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/informes/{id}/pdf [get]
func (h *FiscalHandler) DownloadInformePDF(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	informe, err := h.service.GetInformeByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil || informe == nil {
		response.NotFound(c, "informe not found")
		return
	}

	pdf, err := service.RenderInformePDF(informe)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.Header("Content-Disposition", "inline; filename=\"informe-rendimentos-"+strconv.Itoa(informe.Year)+".pdf\"")
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// ── Report History ──

// ListReports godoc
// @Summary      List fiscal report history
// @Description  Returns generated fiscal reports history
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]fiscal.ReportHistory}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/reports [get]
func (h *FiscalHandler) ListReports(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	// Filtros opcionais do gerenciador de relatórios: tipo e ano-calendário.
	year, _ := strconv.Atoi(c.Query("year"))

	reports, total, err := h.service.ListReports(c.Request.Context(), tenantID, fiscal.ReportHistoryFilter{
		Type:   c.Query("type"),
		Year:   year,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondInternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Reports retrieved successfully", reports, page, int(limit), total)
}

// ArchiveReport godoc
// @Summary      Arquiva um relatório do histórico
// @Description  Soft delete do registro (deleted_at): sai da listagem, permanece para auditoria.
// @Tags         fiscal
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Report ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /fiscal/reports/{id} [delete]
func (h *FiscalHandler) ArchiveReport(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	if err := h.service.ArchiveReport(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		response.NotFound(c, "relatório não encontrado")
		return
	}
	response.Success(c, "Relatório arquivado", nil)
}

// DownloadReport godoc
// @Summary      Download a fiscal report
// @Description  Downloads a generated fiscal report file
// @Tags         fiscal
// @Produce      octet-stream
// @Security     BearerAuth
// @Param        id path string true "Report ID"
// @Success      200  {file}    file
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /fiscal/reports/{id}/download [get]
func (h *FiscalHandler) DownloadReport(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	id := c.Param("id")

	report, err := h.service.GetReportByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if report.FileURL == "" {
		// O arquivo do relatório ainda não foi gerado. Antes devolvíamos
		// 200 + JSON: o frontend salva a resposta como arquivo (o endpoint é
		// um download), então o usuário baixava um ".txt"/".json" com a
		// mensagem de placeholder em vez do relatório. Um 409 é honesto e o
		// cliente mostra o erro em vez de baixar lixo.
		response.Conflict(c, "report file not yet generated")
		return
	}

	// In production, this would redirect to the file URL or stream the file
	c.Redirect(http.StatusTemporaryRedirect, report.FileURL)
}

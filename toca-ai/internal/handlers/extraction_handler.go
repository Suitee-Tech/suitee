package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-ai/internal/domain"
	"github.com/oginoino/toca/toca-ai/internal/services"
)

// ExtractionHandler expõe a extração de campos de documentos (OCR + IA). É uma
// rota interna (X-Internal-Key), chamada pelo toca-deal após o upload — não pelo
// browser. A resposta sempre traz um ExtractedDocument; falhas de OCR/LLM viram
// Status "failed"/"empty" no corpo, não erro HTTP.
type ExtractionHandler struct {
	svc *services.ExtractionService
}

func NewExtractionHandler(svc *services.ExtractionService) *ExtractionHandler {
	return &ExtractionHandler{svc: svc}
}

func (h *ExtractionHandler) Extract(c *gin.Context) {
	var req domain.ExtractDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "corpo inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.DocumentURL) == "" {
		response.ValidationError(c, "document_url é obrigatório")
		return
	}
	if len(req.TargetFields) == 0 {
		response.ValidationError(c, "target_fields é obrigatório")
		return
	}

	result, err := h.svc.Extract(c.Request.Context(), req)
	if err != nil {
		// M17: o detalhe da extração fica no log; o cliente recebe a mensagem
		// genérica do helper.
		response.InternalError(c, err)
		return
	}
	response.Success(c, "ok", result)
}

package handlers

import (
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Portal checklist reads, downloads and direct/multipart upload flows.

// GetMyDealChecklist godoc
// @Summary      Get my deal checklist
// @Description  Get the document checklist for a deal where the current user is a signer
// @Tags         portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200  {object}  response.Response{data=[]deal.ChecklistItem}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id}/checklist [get]
func (h *DealHandler) GetMyDealChecklist(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	deals, err := h.service.ListBySignerIdentity(c.Request.Context(), accountID, email)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	var foundDeal *deal.Deal
	for _, d := range deals {
		if d.ID == id {
			foundDeal = d
			break
		}
	}

	if foundDeal == nil {
		response.Forbidden(c, "deal not found or access denied")
		return
	}

	checklist, err := h.service.GetDealChecklist(c.Request.Context(), foundDeal.TenantID, id)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	my := findMyParty(foundDeal, accountID, email)
	myID := ""
	if my != nil {
		myID = my.ID
	}

	filtered := make([]deal.ChecklistItem, 0, len(checklist))
	for _, item := range checklist {
		if item.PartyID == "" || (myID != "" && item.PartyID == myID) {
			filtered = append(filtered, item)
		}
	}

	response.Success(c, "Checklist retrieved successfully", filtered)
}

// DownloadMyChecklistItem godoc
// @Summary      Download checklist item file
// @Description  Get a presigned URL to download a checklist item file and redirect to it
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Success      302  {string}  string "Redirects to presigned URL"
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id}/checklist/{itemId}/download [get]
func (h *DealHandler) DownloadMyChecklistItem(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")

	// B9: o middleware de auth publica `user_id` (canônico) e, desde o P2,
	// também `account_id` com o mesmo valor. Ler `account_id` sem o fallback
	// fazia a checagem degradar em silêncio para "só por e-mail".
	accountID := c.GetString("user_id")
	if accountID == "" {
		accountID = c.GetString("account_id")
	}
	email := c.GetString("email")

	// Verify the user has access to this deal and get tenant ID
	deals, err := h.service.ListBySignerIdentity(c.Request.Context(), accountID, email)
	if err != nil {
		response.NotFound(c, "deal not found or you don't have access")
		return
	}

	var foundTenantID string
	for _, d := range deals {
		if d.ID == id {
			foundTenantID = d.TenantID
			break
		}
	}

	if foundTenantID == "" {
		response.NotFound(c, "deal not found")
		return
	}

	url, err := h.service.GetChecklistItemDownloadURL(c.Request.Context(), foundTenantID, id, "", itemID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// B8: storage local servido pelo endpoint AUTENTICADO (o mount estático
	// /uploads foi removido); fora do storage local cai no redirect abaixo.
	if serveLocalUploadIfPresent(c, url) {
		return
	}

	c.Redirect(http.StatusFound, url)
}

// DownloadChecklistItem godoc
// @Summary      Download checklist item file (CRM)
// @Description  Get a presigned URL to download a checklist item file and redirect to it
// @Tags         deals
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Success      302  {string}  string "Redirects to presigned URL"
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/checklist/{itemId}/download [get]
func (h *DealHandler) DownloadChecklistItem(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	tenantID := c.GetString("tenant_id")

	brokerID, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	url, err := h.service.GetChecklistItemDownloadURL(c.Request.Context(), tenantID, id, brokerID, itemID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// B8: storage local servido pelo endpoint AUTENTICADO (o mount estático
	// /uploads foi removido); fora do storage local cai no redirect abaixo.
	if serveLocalUploadIfPresent(c, url) {
		return
	}

	c.Redirect(http.StatusFound, url)
}

// UploadMyChecklistItem godoc
// @Summary      Upload my checklist item
// @Description  Mark a checklist item as uploaded for a deal where the current user is a signer
// @Tags         portal
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Param        file formData file false "File"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id}/checklist/{itemId}/upload [post]
func (h *DealHandler) UploadMyChecklistItem(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var (
		file   multipart.File
		header *multipart.FileHeader
	)
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/") {
		f, h2, err := c.Request.FormFile("file")
		if err != nil && !errors.Is(err, http.ErrMissingFile) {
			response.ValidationError(c, "invalid file upload")
			return
		}
		file = f
		header = h2
		if file != nil {
			defer file.Close()
		}
	}

	filename := ""
	contentType := ""
	size := int64(0)
	if header != nil {
		filename = header.Filename
		contentType = header.Header.Get("Content-Type")
		size = header.Size
	}

	if size > virusscan.MaxUploadBytes {
		respondUploadError(c, fmt.Errorf("%w: %d bytes", virusscan.ErrFileTooLarge, size))
		return
	}

	d, err := h.service.UploadMyChecklistItem(c.Request.Context(), accountID, email, name, id, itemID, filename, contentType, size, file)
	if err != nil {
		respondUploadError(c, err)
		return
	}

	response.Success(c, "Checklist item uploaded", d)
}

type GetMyChecklistItemUploadURLRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size"`
}

// GetMyChecklistItemUploadURL godoc
// @Summary      Get presigned upload URL for my checklist item
// @Description  Generates a presigned PUT URL so the client can upload directly to S3/MinIO
// @Tags         portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Param        request body handlers.GetMyChecklistItemUploadURLRequest true "Upload URL Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id}/checklist/{itemId}/upload-url [post]
func (h *DealHandler) GetMyChecklistItemUploadURL(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req GetMyChecklistItemUploadURLRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	out, err := h.service.GetMyChecklistItemUploadURL(c.Request.Context(), accountID, email, id, itemID, req.FileName, req.ContentType, req.FileSize)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "checklist item not found" {
			response.NotFound(c, err.Error())
			return
		}
		if err.Error() == "storage service not configured" || strings.Contains(err.Error(), "presigned PUT not supported") {
			response.Conflict(c, "direct upload unavailable, use multipart upload")
			return
		}
		// O cliente cai no multipart quando este caminho falha: o log guarda a
		// causa e a mensagem diz que o envio direto não está disponível.
		log.Printf("ERROR: presigned upload URL failed for deal=%s item=%s: %v", id, itemID, err)
		response.Error(c, http.StatusConflict, "direct upload unavailable, use multipart upload")
		return
	}

	response.Success(c, "Presigned upload URL generated", out)
}

type CompleteMyChecklistItemUploadRequest struct {
	StorageKey  string `json:"storage_key" binding:"required"`
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size"`
}

// CompleteMyChecklistItemUpload godoc
// @Summary      Complete checklist item upload
// @Description  Marks a checklist item as uploaded after the client successfully PUT the file to the presigned URL
// @Tags         portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Param        request body handlers.CompleteMyChecklistItemUploadRequest true "Upload Complete Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id}/checklist/{itemId}/upload-complete [post]
func (h *DealHandler) CompleteMyChecklistItemUpload(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req CompleteMyChecklistItemUploadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	d, err := h.service.CompleteMyChecklistItemUpload(c.Request.Context(), accountID, email, name, id, itemID, req.StorageKey, req.FileName, req.ContentType, req.FileSize)
	if err != nil {
		respondUploadError(c, err)
		return
	}

	response.Success(c, "Checklist item upload completed", d)
}

type UpdateMyChecklistPartyFieldsRequest struct {
	Name         string            `json:"name,omitempty"`
	Document     string            `json:"document,omitempty"`
	BirthDate    string            `json:"birth_date,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// @Router       /client-portal/deals/{id}/checklist/{itemId}/party-fields [patch]
func (h *DealHandler) UpdateMyChecklistPartyFields(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req UpdateMyChecklistPartyFieldsRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	d, err := h.service.UpdateMyChecklistPartyFields(
		c.Request.Context(),
		accountID,
		email,
		id,
		itemID,
		req.Name,
		req.Document,
		req.BirthDate,
		req.Phone,
		req.CustomFields,
	)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Checklist party fields updated", d)
}

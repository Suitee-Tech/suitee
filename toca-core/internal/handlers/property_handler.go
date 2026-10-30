package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca-core/internal/addressverification"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/auth"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
)

// Force usage of property package for Swagger
var _ = property.Property{}

type PropertyHandler struct {
	service  service.PropertyService
	storage  storage.Service
	eventBus events.EventBus
}

// NewPropertyHandler accepts an optional event bus so AddCapturingBroker
// and AddOwner (Phase 7.3) can fan-out events when a captador joins or an
// owner is added to an existing property — closing the loop with toca-comm
// notifications for the "imóvel já cadastrado" branch of the wizard.
// Pass nil if events are not wired (no-op then).
func NewPropertyHandler(service service.PropertyService, storageSvc storage.Service, eventBus events.EventBus) *PropertyHandler {
	return &PropertyHandler{
		service:  service,
		storage:  storageSvc,
		eventBus: eventBus,
	}
}

// Create godoc
// @Summary      Create property
// @Description  Create a new property
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreatePropertyRequest true "Create Property Request"
// @Success      201  {object}  response.Response{data=property.Property}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties [post]
func (h *PropertyHandler) Create(c *gin.Context) {
	var req CreatePropertyRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	p := req.ToDomain()

	if err := h.service.Create(c.Request.Context(), p); err != nil {
		var amenitiesErr *service.AmenitiesValidationError
		if errors.Is(err, service.ErrExclusiveCaptureNotShareable) ||
			errors.As(err, &amenitiesErr) ||
			addressverification.IsValidationError(err) {
			// A mensagem de comodidades já é a que o operador deve ler.
			if amenitiesErr != nil {
				response.ValidationError(c, amenitiesErr.Message)
				return
			}
			response.ValidationError(c, err.Error())
			return
		}
		if addressverification.IsUnavailable(err) {
			response.Error(c, http.StatusServiceUnavailable, "address verification is temporarily unavailable")
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Property created successfully", p)
}

// GetByID godoc
// @Summary      Get property by ID
// @Description  Get a property by its ID
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Success      200  {object}  response.Response{data=property.Property}
// @Failure      404  {object}  response.Response
// @Router       /properties/{id} [get]
func (h *PropertyHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	p, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, "Property retrieved successfully", p)
}

// GetByIDInternal é a leitura serviço-a-serviço usada pelo toca-financial
// para montar a DIMOB (endereço/CEP/UF do imóvel). O tenant vem da
// credencial interna via auth.ResolveTenantForInternalRequest — nunca de um
// JWT de usuário nem do contexto.
func (h *PropertyHandler) GetByIDInternal(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	p, err := h.service.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, "property not found")
		return
	}
	response.Success(c, "Property retrieved successfully", p)
}

// ListInternal é a listagem serviço-a-serviço usada pelo toca-sites para o
// backfill da vitrine quando o site é ligado (o evento só cobre mudanças
// futuras; o estoque existente precisa ser projetado uma vez).
//
// O tenant vem da credencial interna (nunca de JWT de usuário) e o retorno é
// paginado. Uso interno: o payload tem os mesmos campos do agregado.
func (h *PropertyHandler) ListInternal(c *gin.Context) {
	tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "100"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	ctx := pkgctx.WithTenantID(c.Request.Context(), tenantID)
	properties, total, err := h.service.GetAll(ctx, property.PropertyFilter{Limit: limit, Offset: offset})
	if err != nil {
		response.InternalError(c, err)
		return
	}

	page := int(offset/limit) + 1
	response.Paginated(c, "Properties retrieved successfully", properties, page, int(limit), total)
}

// GetAll godoc
// @Summary      Get all properties
// @Description  Get all properties with pagination and filters
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        q query string false "Search query"
// @Param        status query string false "Status"
// @Param        type query string false "Property Type"
// @Param        listing_type query string false "Listing Type"
// @Param        available_for query string false "Available business type"
// @Param        min_price query number false "Min Price"
// @Param        max_price query number false "Max Price"
// @Param        min_bedrooms query int false "Min Bedrooms"
// @Param        min_bathrooms query int false "Min Bathrooms"
// @Param        min_area query number false "Min Area"
// @Param        max_area query number false "Max Area"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]property.Property}
// @Failure      500  {object}  response.Response
// @Router       /properties [get]
func (h *PropertyHandler) GetAll(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	filter := property.PropertyFilter{
		Limit:  limit,
		Offset: offset,
		Query:  c.Query("q"),
	}
	filter.Address = c.Query("address")
	filter.Neighborhood = c.Query("neighborhood")
	filter.City = c.Query("city")
	filter.State = c.Query("state")
	filter.ZipCode = c.Query("zip_code")

	if status := c.Query("status"); status != "" {
		filter.Status = property.PropertyStatus(status)
	}
	if pType := c.Query("type"); pType != "" {
		filter.PropertyType = property.PropertyType(pType)
	}
	if lType := c.Query("listing_type"); lType != "" {
		filter.ListingType = property.ListingType(lType)
	}
	if availableFor := c.Query("available_for"); availableFor != "" {
		filter.AvailableFor = property.ListingType(availableFor)
	}

	if val := c.Query("min_price"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			filter.MinPrice = &v
		}
	}
	if val := c.Query("max_price"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			filter.MaxPrice = &v
		}
	}

	if val := c.Query("min_bedrooms"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			filter.MinBedrooms = &v
		}
	}
	if val := c.Query("min_bathrooms"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			filter.MinBathrooms = &v
		}
	}

	if val := c.Query("min_area"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			filter.MinArea = &v
		}
	}
	if val := c.Query("max_area"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			filter.MaxArea = &v
		}
	}

	properties, total, err := h.service.GetAll(c.Request.Context(), filter)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Properties retrieved successfully", []*property.Property{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Properties retrieved successfully", properties, page, int(limit), total)
}

// Update godoc
// @Summary      Update property
// @Description  Update a property by its ID
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        request body handlers.UpdatePropertyRequest true "Update Property Request"
// @Success      200  {object}  response.Response{data=property.Property}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id} [put]
func (h *PropertyHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req UpdatePropertyRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	p := req.ToDomain(id)

	if err := h.service.Update(c.Request.Context(), p); err != nil {
		var amenitiesErr *service.AmenitiesValidationError
		if errors.Is(err, service.ErrExclusiveCaptureNotShareable) ||
			errors.As(err, &amenitiesErr) ||
			addressverification.IsValidationError(err) {
			// A mensagem de comodidades já é a que o operador deve ler.
			if amenitiesErr != nil {
				response.ValidationError(c, amenitiesErr.Message)
				return
			}
			response.ValidationError(c, err.Error())
			return
		}
		if addressverification.IsUnavailable(err) {
			response.Error(c, http.StatusServiceUnavailable, "address verification is temporarily unavailable")
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Property updated successfully", p)
}

// Delete godoc
// @Summary      Delete property
// @Description  Delete a property by its ID
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Success      200  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id} [delete]
func (h *PropertyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Property deleted successfully", nil)
}

func (h *PropertyHandler) UploadImage(c *gin.Context) {
	id := c.Param("id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.ValidationError(c, "file is required: "+err.Error())
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	ext, contentType, err := storage.ValidateWithMIME("toca-property-images", header.Filename, header.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiFile := io.MultiReader(bytes.NewReader(head[:n]), file)
	filename := id + "/" + uuid.NewString() + ext

	// Padrão de bucket: toca-property-images
	url, err := h.storage.Upload(c.Request.Context(), "toca-property-images", filename, contentType, multiFile)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// Phase 8.1 — persistir o URL em Property.Media.Images[] atomicamente.
	// Antes desta mudança o handler retornava só {url, filename} e o front
	// tinha que chamar Update separadamente; agora a property é atualizada
	// na mesma request. Resposta inclui a property completa para o cliente
	// usar como source-of-truth sem refetch.
	updatedProp, persistErr := h.appendImageToProperty(c.Request.Context(), id, url, header.Filename)
	resp := gin.H{
		"url":      url,
		"filename": filename,
	}
	if persistErr != nil {
		// O upload físico funcionou — não falha o request por erro de
		// persistência. Frontend antigo (que faz manual update) continua
		// trabalhando; frontend novo recebe `property: null` e pode
		// fallback para refetch.
		resp["property"] = nil
		resp["persist_error"] = persistErr.Error()
	} else {
		resp["property"] = updatedProp
	}

	response.Created(c, "Property image uploaded successfully", resp)
}

// appendImageToProperty fetches the property tenant-scoped, appends the
// image URL to Media.Images[] (deduped), and persists. Returns the updated
// property so the handler can echo it back.
func (h *PropertyHandler) appendImageToProperty(ctx context.Context, propertyID, url, originalName string) (*property.Property, error) {
	// Guard for tests that wire a nil service (only the storage layer is
	// exercised in `property_media_test.go`).
	if h.service == nil {
		return nil, fmt.Errorf("property service not configured")
	}
	// Append atômico no JSONB: uploads simultâneos do mesmo formulário não
	// podem fazer read-modify-write e perder as imagens anteriores.
	p, err := h.service.AppendImage(ctx, propertyID, property.Image{
		URL:         url,
		Description: originalName,
	})
	if err != nil {
		return nil, fmt.Errorf("append property image: %w", err)
	}
	return p, nil
}

func (h *PropertyHandler) DeleteImage(c *gin.Context) {
	id := c.Param("id")
	filename := c.Query("filename")
	if filename == "" {
		response.ValidationError(c, "filename is required")
		return
	}
	if !strings.HasPrefix(filename, id+"/") {
		response.Forbidden(c, "access denied")
		return
	}

	if err := h.storage.Delete(c.Request.Context(), "toca-property-images", filename); err != nil {
		response.InternalError(c, err)
		return
	}

	// Phase 8.1 — remove the image from Media.Images[] atomically. We match
	// by URL substring (the storage URL contains the filename path), since
	// the frontend sends just the storage filename, not the full URL.
	updatedProp, _ := h.removeImageFromProperty(c.Request.Context(), id, filename)
	if updatedProp != nil {
		response.Success(c, "Property image deleted successfully", gin.H{"property": updatedProp})
		return
	}
	c.Status(204)
}

func (h *PropertyHandler) removeImageFromProperty(ctx context.Context, propertyID, filename string) (*property.Property, error) {
	if h.service == nil {
		return nil, nil
	}
	p, err := h.service.GetByID(ctx, propertyID)
	if err != nil || p == nil {
		return nil, err
	}
	kept := make([]property.Image, 0, len(p.Media.Images))
	for _, img := range p.Media.Images {
		// A storage URL usually contains the filename as a substring (e.g.
		// `https://.../toca-property-images/<id>/<uuid>.jpg`). We match on
		// substring so this works across providers (S3 / Supabase / local).
		if strings.Contains(img.URL, filename) {
			continue
		}
		kept = append(kept, img)
	}
	if len(kept) == len(p.Media.Images) {
		return p, nil // nothing matched — no-op
	}
	// Re-index Order so the carousel stays sequential.
	for i := range kept {
		kept[i].Order = i
	}
	p.Media.Images = kept
	if err := h.service.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddInterestedLead godoc
// @Summary      Add interested lead to property
// @Description  Register a lead as interested in a property (single source of truth)
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        request body handlers.AddInterestedLeadRequest true "Add Interested Lead"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id}/interested-leads [post]
func (h *PropertyHandler) AddInterestedLead(c *gin.Context) {
	id := c.Param("id")
	var req AddInterestedLeadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.AddInterestedLead(c.Request.Context(), id, req.LeadID, req.Source); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Lead added to property interest list", nil)
}

// RemoveInterestedLead godoc
// @Summary      Remove interested lead from property
// @Description  Remove a lead from the property's interest list
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        leadId path string true "Lead ID"
// @Success      200  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id}/interested-leads/{leadId} [delete]
func (h *PropertyHandler) RemoveInterestedLead(c *gin.Context) {
	id := c.Param("id")
	leadID := c.Param("leadId")

	if err := h.service.RemoveInterestedLead(c.Request.Context(), id, leadID); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Lead removed from property interest list", nil)
}

// AddCapturingBroker godoc
// @Summary      Add capturing broker to property
// @Description  Register a capturing broker on a property from the time of captação
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        request body handlers.AddCapturingBrokerRequest true "Add Capturing Broker"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id}/capturing-brokers [post]
func (h *PropertyHandler) AddCapturingBroker(c *gin.Context) {
	id := c.Param("id")
	var req AddCapturingBrokerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	broker := property.CapturingBrokerRef{
		ID:            req.BrokerID,
		Creci:         req.Creci,
		Role:          property.BrokerRole(req.Role),
		CommissionPct: req.CommissionPct,
	}

	if err := h.service.AddCapturingBroker(c.Request.Context(), id, broker); err != nil {
		response.InternalError(c, err)
		return
	}

	// Phase 7.3 — emit `property.capturing_broker_added` so toca-comm can
	// notify the broker (and analytics can update commission projections).
	// Best-effort: a publish failure shouldn't fail the HTTP request.
	if h.eventBus != nil {
		tenantID, _ := pkgctx.GetTenantID(c.Request.Context())
		evt := contracts.CapturingBrokerAddedEvent{
			Data: contracts.CapturingBrokerAddedEventPayload{
				PropertyID: id,
				TenantID:   tenantID,
				BrokerID:   req.BrokerID,
				Role:       req.Role,
				Creci:      req.Creci,
				Timestamp:  contracts.NowRFC3339(),
			},
		}
		if err := h.eventBus.PublishReliable(c.Request.Context(), evt); err != nil {
			log.Printf("WARN: failed to enqueue property.capturing_broker_added event for property=%s broker=%s: %v", id, req.BrokerID, err)
		}
	}

	response.Success(c, "Capturing broker added to property", nil)
}

// AddOwner godoc
// @Summary      Add owner to property
// @Description  Add an owner to the property's owners list with optional percentage
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        request body handlers.AddOwnerRequest true "Add Owner"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id}/owners [post]
func (h *PropertyHandler) AddOwner(c *gin.Context) {
	id := c.Param("id")
	var req AddOwnerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	owner := property.OwnerRef{
		ID:         req.OwnerID,
		Type:       req.OwnerType,
		Percentage: req.Percentage,
	}

	if err := h.service.AddOwner(c.Request.Context(), id, owner); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Owner added to property", nil)
}

// SetActiveDeal godoc
// @Summary      Link active deal to property
// @Description  Link or unlink an active deal/negotiation to a property
// @Tags         properties
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Property ID"
// @Param        request body handlers.SetActiveDealRequest true "Set Active Deal"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /properties/{id}/active-deal [patch]
func (h *PropertyHandler) SetActiveDeal(c *gin.Context) {
	id := c.Param("id")
	var req SetActiveDealRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.SetActiveDeal(c.Request.Context(), id, req.DealID); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Active deal updated on property", nil)
}

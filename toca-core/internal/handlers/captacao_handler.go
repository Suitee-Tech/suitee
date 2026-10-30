package handlers

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

// CaptacaoHandler is the convenience entrypoint that performs every step of
// the captação flow in a single call:
//  1. Persist the Property (status=`prospecting`).
//  2. Append `owners[]` provided in the body.
//  3. Append `capturing_brokers[]` provided in the body.
//  4. Emit `property.captured` so toca-comm can notify the owner-lead.
//
// Why aggregator instead of letting the front make 3 calls: the Phase 1
// validation already rejects malformed combinations, but the front would
// still leave half-baked rows on a partial failure between calls. This
// endpoint persists the property + owners + brokers in service order and
// returns the populated property — same shape as POST /properties so
// existing front clients can swap with minimal change.
//
// Idempotency: if the same `X-Idempotency-Key` is replayed, behaviour is
// "create-once-then-no-op" because the underlying handlers (AddOwner,
// AddCapturingBroker) are idempotent on their natural keys; only the
// initial Property creation is non-idempotent and would currently produce
// a duplicate. Tracking the key in a durable cache is a future iteration —
// for now we document the soft guarantee.
type CaptacaoHandler struct {
	service  service.PropertyService
	eventBus events.EventBus
}

func NewCaptacaoHandler(svc service.PropertyService, eventBus events.EventBus) *CaptacaoHandler {
	return &CaptacaoHandler{service: svc, eventBus: eventBus}
}

// CaptacaoRequest mirrors CreatePropertyRequest plus owner-lead + capturing
// broker arrays. Required fields are the bare minimum for a captação; the
// rest comes from PropertyModal-style optional fields.
type CaptacaoRequest struct {
	Property         CreatePropertyRequest       `json:"property" binding:"required"`
	Owners           []AddOwnerRequest           `json:"owners" binding:"required,min=1,dive"`
	CapturingBrokers []AddCapturingBrokerRequest `json:"capturing_brokers" binding:"required,min=1,dive"`
}

// Capturar godoc
// @Summary      Captar imóvel
// @Description  One-shot captação: cria Property + owners + capturing_brokers e emite property.captured
// @Tags         captacao
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CaptacaoRequest true "Captação Request"
// @Success      201  {object}  response.Response{data=property.Property}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /captacao [post]
func (h *CaptacaoHandler) Capturar(c *gin.Context) {
	var req CaptacaoRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// 1. Build + persist the property. Pre-fill owners and capturing brokers
	//    so the existing tenant validation in propertyService.Create catches
	//    cross-tenant ids in a single trip.
	p := req.Property.ToDomain()
	if strings.TrimSpace(string(p.Status)) == "" {
		p.Status = property.PropertyStatusProspecting
	}
	for _, o := range req.Owners {
		p.Owners = append(p.Owners, property.OwnerRef{
			ID:         o.OwnerID,
			Type:       o.OwnerType,
			Percentage: o.Percentage,
		})
	}
	for _, b := range req.CapturingBrokers {
		p.CapturingBrokers = append(p.CapturingBrokers, property.CapturingBrokerRef{
			ID:            b.BrokerID,
			Creci:         b.Creci,
			Role:          property.BrokerRole(b.Role),
			CommissionPct: b.CommissionPct,
		})
	}

	if err := h.service.Create(c.Request.Context(), p); err != nil {
		response.InternalError(c, err)
		return
	}

	// 2. Emit property.captured. Best-effort: if the bus is missing we still
	//    return the captured property so the admin sees the result.
	if h.eventBus != nil {
		ownerLeadID := ""
		if len(p.Owners) > 0 {
			ownerLeadID = p.Owners[0].ID
		}
		brokerIDs := make([]string, 0, len(p.CapturingBrokers))
		for _, b := range p.CapturingBrokers {
			brokerIDs = append(brokerIDs, b.ID)
		}
		evt := contracts.PropertyCapturedEvent{
			Data: contracts.PropertyCapturedEventPayload{
				PropertyID:         p.ID,
				TenantID:           p.TenantID,
				OwnerLeadID:        ownerLeadID,
				CapturingBrokerIDs: brokerIDs,
				PropertyTitle:      p.Title,
				Status:             string(p.Status),
				ListingType:        string(p.ListingType),
				Timestamp:          contracts.NowRFC3339(),
			},
		}
		if err := h.eventBus.PublishReliable(c.Request.Context(), evt); err != nil {
			log.Printf("WARN: failed to enqueue property.captured event for property=%s: %v", p.ID, err)
		}
	}

	response.Created(c, "Captação registrada com sucesso", p)
}

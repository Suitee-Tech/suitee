package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// AddParticipantRequest holds the payload for adding a participant
type AddParticipantRequest struct {
	ParticipantID string `json:"participant_id" binding:"required"`
}

// AddParticipant godoc
// @Summary      Add participant to channel
// @Description  Adds a participant to an existing channel
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Param        request body handlers.AddParticipantRequest true "Add Participant Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /channels/{id}/participants [post]
func (h *MessageHandler) AddParticipant(c *gin.Context) {
	channelID := c.Param("id")

	var req AddParticipantRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.AddParticipant(c.Request.Context(), channelID, req.ParticipantID); err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Participant added successfully", nil)
}

// RemoveParticipant godoc
// @Summary      Remove participant from channel
// @Description  Removes a participant from an existing channel
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Param        participantId path string true "Participant ID"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/participants/{participantId} [delete]
func (h *MessageHandler) RemoveParticipant(c *gin.Context) {
	channelID := c.Param("id")
	participantID := c.Param("participantId")

	if err := h.service.RemoveParticipant(c.Request.Context(), channelID, participantID); err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Participant removed successfully", nil)
}

// UpdateChannelStatusRequest holds the payload for updating channel status
type UpdateChannelStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateStatus godoc
// @Summary      Update channel status
// @Description  Updates the status of an existing channel (e.g. open/closed)
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Param        request body handlers.UpdateChannelStatusRequest true "Update Status Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/status [patch]
func (h *MessageHandler) UpdateStatus(c *gin.Context) {
	channelID := c.Param("id")

	var req UpdateChannelStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Casting string to channel status type from message package (need to import message domain or keep it decoupled)
	// Since we are not importing message here directly to avoid clutter, we use string in handler and convert in service if needed.
	// Oh wait, UpdateChannelStatus accepts message.ChannelStatus. We should import it.

	if err := h.service.UpdateChannelStatus(c.Request.Context(), channelID, message.ChannelStatus(req.Status)); err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel status updated successfully", nil)
}

package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"strings"
)

type GroupHandler struct {
	groupService *service.GroupService
}

func NewGroupHandler(groupService *service.GroupService) *GroupHandler {
	return &GroupHandler{
		groupService: groupService,
	}
}

// respondGroupError mapeia o erro do serviço de grupos para o status correto.
//
// P2/M4 — a revisão de permissões registrou que falhas de AUTORIZAÇÃO nesses
// handlers saíam como **500** (e algumas validações como 500 também): sem 403 não
// havia nem semântica correta nem trilha de negação. Agora:
//
//	unauthorized            → 403 + `authz.denied` na auditoria
//	not found               → 404
//	regra de negócio        → 400
//	resto                   → 500
func respondGroupError(c *gin.Context, err error, entityID string) {
	msg := err.Error()
	switch {
	case msg == "unauthorized":
		auth.DenyGinRequest(c, "unauthorized", "company:manage_members")
	case strings.Contains(msg, "not found"):
		response.NotFound(c, msg)
	case strings.Contains(msg, "cannot delete a default profile group"),
		strings.Contains(msg, "validation"),
		strings.Contains(msg, "is required"):
		response.ValidationError(c, msg)
	default:
		// M17: erro não classificado é interno — detalhe só no log.
		response.InternalError(c, err)
	}
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req group.CreateGroupRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	g, err := h.groupService.CreateGroup(c.Request.Context(), userID.(string), companyID, &req)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Created(c, "Group created successfully", g)
}

func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req group.UpdateGroupRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	g, err := h.groupService.UpdateGroup(c.Request.Context(), userID.(string), companyID, groupID, &req)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "Group updated successfully", g)
}

func (h *GroupHandler) GetGroup(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	g, err := h.groupService.GetGroup(c.Request.Context(), userID.(string), companyID, groupID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, "Group retrieved successfully", g)
}

func (h *GroupHandler) ListGroups(c *gin.Context) {
	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	groups, err := h.groupService.ListGroups(c.Request.Context(), userID.(string), companyID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Groups retrieved successfully", []*group.Group{})
			return
		}
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "Groups retrieved successfully", groups)
}

func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.groupService.DeleteGroup(c.Request.Context(), userID.(string), companyID, groupID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "group deleted successfully", nil)
}

func (h *GroupHandler) AddMember(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req struct {
		AccountID string `json:"account_id" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	err := h.groupService.AddMemberToGroup(c.Request.Context(), userID.(string), companyID, groupID, req.AccountID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "member added successfully", nil)
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id")
	memberID := c.Param("member_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.groupService.RemoveMemberFromGroup(c.Request.Context(), userID.(string), companyID, groupID, memberID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "member removed successfully", nil)
}

func (h *GroupHandler) MoveMember(c *gin.Context) {
	companyID := c.Param("id")
	groupID := c.Param("group_id") // Source group
	memberID := c.Param("member_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req struct {
		TargetGroupID string `json:"target_group_id" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	err := h.groupService.MoveMemberBetweenGroups(c.Request.Context(), userID.(string), companyID, groupID, req.TargetGroupID, memberID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}

	response.Success(c, "member moved successfully", nil)
}

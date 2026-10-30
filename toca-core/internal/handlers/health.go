package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) HealthCheck(c *gin.Context) {
	response.Success(c, "Service is healthy", nil)
}

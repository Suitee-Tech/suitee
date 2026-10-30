package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type CustomerHandler struct {
}

func NewCustomerHandler() *CustomerHandler {
	return &CustomerHandler{}
}

func (h *CustomerHandler) GetSummary(c *gin.Context) {
	accountId := c.Param("accountId")
	if accountId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accountId is required"})
		return
	}

	// Mocking data as allowed by requirements
	nextVisit := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	c.JSON(http.StatusOK, gin.H{
		"leads":           3,
		"deals":           2,
		"tickets_count":   1,
		"open_charges":    1500.50,
		"next_visit":      nextVisit,
		"unread_messages": 5,
	})
}

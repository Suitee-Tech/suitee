package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/response"
)

// respondInternalError keeps repository timeout semantics consistent across
// every financial/fiscal handler. A depleted pool, canceled context or
// Postgres statement timeout is an upstream timeout (504), not an internal
// application failure (500).
func respondInternalError(c *gin.Context, err error) {
	if database.IsTimeout(err) {
		response.GatewayTimeout(c, "request timeout while accessing financial data")
		return
	}
	response.InternalError(c, err)
}

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
)

// SetupRecovery returns gin's panic-recovery middleware wired through
// pkg/logger (gin.Recovery writes to gin's own log target which doesn't
// honor our structured-log config). Always responds with 500 + a JSON
// envelope when no response has been written yet; if the handler had
// already started streaming, the connection is just torn down.
//
// Logs via *Context so request_id / trace_id are preserved in the
// panic log line — operators can correlate the crash with the client's
// failed request.
func SetupRecovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.ErrorContext(c.Request.Context(), "panic recovered", "error", recovered)
		if !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Internal Server Error",
			})
		}
	})
}

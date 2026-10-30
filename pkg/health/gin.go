package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterGin mounts the standard health endpoints on the router:
//   - GET /live   : liveness probe, always 200 if the process is up
//   - GET /ready  : readiness probe, 200 only when every Checker passes
//   - GET /health/deep : diagnostic, runs the same readiness checkers
//                        plus any explicit "deep" probes the service
//                        registered via RegisterDeepChecks
//
// Kubernetes / Docker should target /live for restart decisions and
// /ready for traffic shaping. /health/deep is for human operators
// during incidents — it doesn't gate any automation.
func RegisterGin(r *gin.Engine, serviceName string, readiness *ReadinessChecker) {
	r.GET("/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": serviceName})
	})

	r.GET("/ready", func(c *gin.Context) {
		if readiness == nil {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": serviceName})
			return
		}

		ok, details := readiness.IsReady(c.Request.Context())
		if ok {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": serviceName, "checks": details})
			return
		}

		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "service": serviceName, "checks": details})
	})
}

// RegisterDeepGin mounts the /health/deep endpoint. Pass the regular
// readiness checker plus any deeper probes (DB write+read, Redis SET/GET,
// downstream service ping) so operators can verify a healthy "ready"
// response actually corresponds to a system that can do work end-to-end.
//
// Separate function from RegisterGin so services without a Redis or
// downstream dependency don't have to wire deep probes at all.
func RegisterDeepGin(r *gin.Engine, serviceName string, deep *ReadinessChecker) {
	if r == nil || deep == nil {
		return
	}
	r.GET("/health/deep", func(c *gin.Context) {
		ok, details := deep.IsReady(c.Request.Context())
		body := gin.H{
			"service": serviceName,
			"checks":  details,
		}
		if ok {
			body["status"] = "ok"
			c.JSON(http.StatusOK, body)
			return
		}
		body["status"] = "degraded"
		c.JSON(http.StatusServiceUnavailable, body)
	})
}

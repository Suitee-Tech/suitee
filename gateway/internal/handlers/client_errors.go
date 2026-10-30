package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type clientErrorDiagnostic struct {
	Kind      string `json:"kind"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
	Operation string `json:"operation"`
	Status    int    `json:"status,omitempty"`
	Release   string `json:"release"`
}

var diagnosticRelease = regexp.MustCompile(`^(unknown|v?\d+\.\d+\.\d+([-+][\w.-]+)?|[a-f0-9]{7,40})$`)

// Anonymous, rate-limited by the gateway's existing IP middleware. Strict
// allowlists keep URLs, exception messages, identities and form data out of logs.
func RegisterClientErrors(r *gin.Engine) {
	r.POST("/api/v1/client-errors", clientErrors)
}
func clientErrors(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var d clientErrorDiagnostic
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		c.Status(http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(d.Code); err != nil || len(d.Code) != 36 {
		c.Status(http.StatusBadRequest)
		return
	}
	if d.RequestID != "" {
		if _, err := uuid.Parse(d.RequestID); err != nil || len(d.RequestID) != 36 {
			c.Status(http.StatusBadRequest)
			return
		}
	}
	if len(d.Release) > 80 || !diagnosticRelease.MatchString(d.Release) || d.Status < 0 || d.Status > 599 {
		c.Status(http.StatusBadRequest)
		return
	}
	if d.Kind == "render" {
		if d.Operation != "render" {
			c.Status(http.StatusBadRequest)
			return
		}
	} else if d.Kind == "http" {
		parts := strings.Split(d.Operation, " ")
		// A segunda metade é a FAMÍLIA da rota, nunca a URL: sem `brokers` um
		// erro real do sync de corretores chegava como "POST other" e não dava
		// para saber qual tela estava falhando.
		if len(parts) != 2 || !diagnosticMember(parts[0], "GET", "POST", "PUT", "PATCH", "DELETE", "OTHER") || !diagnosticMember(parts[1], "appointments", "brokers", "calendar", "contracts", "deals", "leads", "portal", "financial", "charges", "bills", "auth", "properties", "tickets", "channels", "other") {
			c.Status(http.StatusBadRequest)
			return
		}
	} else {
		c.Status(http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(d)
	log.Printf("client_error %s", payload)
	c.Status(http.StatusAccepted)
}
func diagnosticMember(value string, allowed ...string) bool {
	for _, option := range allowed {
		if value == option {
			return true
		}
	}
	return false
}

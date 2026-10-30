package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// M1 — o estado do backstop de RLS precisa sair do log e virar métrica
// alertável: sem ela, "as políticas de RLS são inertes" só aparecia no boot.
func TestRLSBackstopGaugeIsExported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/metrics", Handler())

	scrape := func() string {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		return recorder.Body.String()
	}

	SetRLSBackstopActive(false)
	if body := scrape(); !strings.Contains(body, "database_rls_backstop_active 0") {
		t.Fatalf("métrica ausente quando o RLS é inerte:\n%.200s", body)
	}
	SetRLSBackstopActive(true)
	body := scrape()
	if !strings.Contains(body, "database_rls_backstop_active 1") {
		t.Fatalf("métrica ausente quando o RLS é backstop:\n%.200s", body)
	}
	if !strings.Contains(body, "# TYPE database_rls_backstop_active gauge") {
		t.Fatal("métrica sem declaração de tipo (gauge)")
	}
}

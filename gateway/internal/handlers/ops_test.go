package handlers

import (
	"github.com/gin-gonic/gin"
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuppliersDispatchToOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(r.Method + " " + r.URL.RequestURI() + " " + string(body)))
	}))
	defer upstream.Close()
	router := gin.New()
	RegisterOps(router, gwconfig.Config{OpsServiceURL: upstream.URL}, proxy.New())
	server := httptest.NewServer(router)
	defer server.Close()
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/suppliers", `{"name":"Ana","service_type":"Vistoria"}`},
		{"GET", "/api/v1/suppliers?active_only=true", ""},
		{"GET", "/api/v1/suppliers/service-types", ""},
		{"PUT", "/api/v1/suppliers/supplier-id", `{"active":false}`},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(tc.body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 201 || string(body) != tc.method+" "+tc.path+" "+tc.body {
			t.Fatalf("dispatch: %d %s", res.StatusCode, body)
		}
	}
}

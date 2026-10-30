package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgctx "github.com/oginoino/toca/pkg/context"
)

func TestRequestID_PropagatesGeneratedIDToRequestContextAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		requestID := c.Request.Header.Get(RequestIDHeader)
		if requestID == "" {
			t.Fatal("request did not receive a generated X-Request-ID")
		}
		if contextID := pkgctx.GetRequestID(c.Request.Context()); contextID != requestID {
			t.Fatalf("context request id: want %q, got %q", requestID, contextID)
		}
		c.String(http.StatusOK, requestID)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, recorder.Code)
	}
	if responseID := recorder.Header().Get(RequestIDHeader); responseID == "" {
		t.Fatal("response did not receive X-Request-ID")
	} else if responseID != recorder.Body.String() {
		t.Fatalf("response request id: header %q, handler %q", responseID, recorder.Body.String())
	}
}

func TestRequestID_PreservesInboundIDEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const inboundID = "edge-request-123"
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		if requestID := c.Request.Header.Get(RequestIDHeader); requestID != inboundID {
			t.Fatalf("request header: want %q, got %q", inboundID, requestID)
		}
		if contextID := pkgctx.GetRequestID(c.Request.Context()); contextID != inboundID {
			t.Fatalf("context request id: want %q, got %q", inboundID, contextID)
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, inboundID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status: want %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if responseID := recorder.Header().Get(RequestIDHeader); responseID != inboundID {
		t.Fatalf("response header: want %q, got %q", inboundID, responseID)
	}
}

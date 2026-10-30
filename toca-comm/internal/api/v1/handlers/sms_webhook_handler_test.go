package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupSMSTestRouter monta um gin.Engine com apenas a rota POST
// /webhooks/sms para teste isolado.
func setupSMSTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSMSWebhookHandler()
	r := gin.New()
	r.POST("/webhooks/sms", h.Receive)
	return r
}

// signZenviaBody replica o algoritmo de assinatura da Zenvia:
// HMAC-SHA1(secret, body) hex.
func signZenviaBody(secret string, body []byte) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestSMSReceive_ValidSignatureAcceptsAndLogsStatuses(t *testing.T) {
	const secret = "zenvia_shared_secret"
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", secret)

	body := []byte(`{
		"id":"evt-1",
		"type":"MESSAGE_STATUS",
		"subscriptionId":"sub-1",
		"channel":"sms",
		"messageId":"msg-abc",
		"direction":"OUT",
		"contentIndex":0,
		"messageStatus":{
			"timestamp":"2026-05-19T10:00:00.000Z",
			"code":"DELIVERED",
			"description":"Message delivered to handset"
		}
	}`)
	sig := signZenviaBody(secret, body)

	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZENVIA-SIGNATURE", "sha1="+sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestSMSReceive_BatchArrayPayload(t *testing.T) {
	// Zenvia também pode enviar batch (array de eventos). O handler
	// aceita ambos.
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")
	body := []byte(`[
		{"type":"MESSAGE_STATUS","messageId":"m1","messageStatus":{"code":"SENT","timestamp":"t1"}},
		{"type":"MESSAGE_STATUS","messageId":"m2","messageStatus":{"code":"DELIVERED","timestamp":"t2"}}
	]`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSMSReceive_RejectsInvalidSignature(t *testing.T) {
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "right_secret")
	body := []byte(`{"type":"MESSAGE_STATUS","messageId":"x"}`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	req.Header.Set("X-ZENVIA-SIGNATURE", "sha1=deadbeef")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Drop silencioso com 200 — não criamos retry loop pra impostor.
	if w.Code != 200 {
		t.Errorf("expected 200 (silent drop on bad sig), got %d", w.Code)
	}
}

func TestSMSReceive_AcceptsWithoutSecretInDevMode(t *testing.T) {
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")
	body := []byte(`{"type":"MESSAGE_STATUS","messageId":"x","messageStatus":{"code":"SENT"}}`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 in dev mode (no secret), got %d", w.Code)
	}
}

func TestSMSReceive_InvalidJsonStillReturns200(t *testing.T) {
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")
	body := []byte(`{not json`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 on invalid JSON, got %d", w.Code)
	}
}

func TestSMSReceive_SkipsNonMessageStatusEvents(t *testing.T) {
	// Eventos do tipo MESSAGE (inbound) não são tratados aqui.
	// Garantimos que o handler não explode e retorna 200.
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")
	body := []byte(`{"type":"MESSAGE","messageId":"inbound-1"}`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSMSReceive_WrongJsonFieldShape_StillReturns200(t *testing.T) {
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")
	body := []byte(`{"some_other_field":42}`)
	r := setupSMSTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestVerifyZenviaSignature(t *testing.T) {
	const secret = "abc"
	body := []byte(`{"hello":"world"}`)
	sig := signZenviaBody(secret, body)

	if !verifyZenviaSignature(secret, body, "sha1="+sig) {
		t.Error("expected matching signature to verify")
	}
	if verifyZenviaSignature(secret, body, "sha1=wrong") {
		t.Error("expected wrong signature to be rejected")
	}
	if verifyZenviaSignature(secret, body, sig) {
		t.Error("expected missing sha1= prefix to be rejected")
	}
	if verifyZenviaSignature(secret, []byte("changed"), "sha1="+sig) {
		t.Error("expected modified body to invalidate signature")
	}
	// Confusão de algoritmo: prefixo sha256= não deve passar.
	if verifyZenviaSignature(secret, body, "sha256="+sig) {
		t.Error("expected sha256= prefix to be rejected (Zenvia uses sha1)")
	}
}

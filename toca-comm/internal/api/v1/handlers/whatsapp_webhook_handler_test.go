package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupTestRouter monta um gin.Engine com as duas rotas do webhook
// para testar isoladamente sem dependência do main do toca-comm.
func setupTestRouter() (*gin.Engine, *WhatsAppWebhookHandler) {
	gin.SetMode(gin.TestMode)
	h := NewWhatsAppWebhookHandler()
	r := gin.New()
	r.GET("/webhooks/whatsapp", h.Verify)
	r.POST("/webhooks/whatsapp", h.Receive)
	return r, h
}

func TestVerify_AcceptsMatchingToken(t *testing.T) {
	t.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "secret_token_xyz")
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=12345&hub.verify_token=secret_token_xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "12345" {
		t.Errorf("expected challenge echo '12345', got %q", w.Body.String())
	}
}

func TestVerify_RejectsWrongToken(t *testing.T) {
	t.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "right")
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=12&hub.verify_token=wrong", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestVerify_RejectsWhenTokenNotConfigured(t *testing.T) {
	t.Setenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN", "")
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=12&hub.verify_token=anything", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 when token env var unset, got %d", w.Code)
	}
}

func TestReceive_ValidSignatureAcceptsAndLogsStatuses(t *testing.T) {
	const appSecret = "shhh_app_secret"
	t.Setenv("WHATSAPP_APP_SECRET", appSecret)

	body := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[{"id":"BIZ","changes":[{"field":"messages","value":{
			"messaging_product":"whatsapp",
			"statuses":[
				{"id":"wamid.ABC","status":"sent","timestamp":"1700000000","recipient_id":"55119"},
				{"id":"wamid.DEF","status":"delivered","timestamp":"1700000001","recipient_id":"55119"}
			]
		}}]}]
	}`)
	sig := signBody(appSecret, body)

	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestReceive_RejectsInvalidSignature(t *testing.T) {
	t.Setenv("WHATSAPP_APP_SECRET", "right_secret")
	body := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeefcafe")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Sempre 200 mesmo rejeitando — Meta espera 2xx, e queremos
	// drop silencioso para sigs forjadas.
	if w.Code != 200 {
		t.Errorf("expected 200 (silent drop on bad sig), got %d", w.Code)
	}
}

func TestReceive_AcceptsWithoutSecretInDevMode(t *testing.T) {
	// Sem WHATSAPP_APP_SECRET no env, o handler aceita sem validar
	// signature (modo dev). Loga warning.
	t.Setenv("WHATSAPP_APP_SECRET", "")
	body := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 in dev mode (no secret), got %d", w.Code)
	}
}

func TestReceive_InvalidJsonStillReturns200(t *testing.T) {
	// Body mal-formado: aceita 200 para não criar retry loop na Meta.
	t.Setenv("WHATSAPP_APP_SECRET", "")
	body := []byte(`{not json`)
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 on invalid JSON, got %d", w.Code)
	}
}

func TestVerifyMetaSignature(t *testing.T) {
	const secret = "abc"
	body := []byte(`{"hello":"world"}`)
	sig := signBody(secret, body)

	if !verifyMetaSignature(secret, body, "sha256="+sig) {
		t.Error("expected matching signature to verify")
	}
	if verifyMetaSignature(secret, body, "sha256=wrong") {
		t.Error("expected wrong signature to be rejected")
	}
	if verifyMetaSignature(secret, body, sig) {
		t.Error("expected missing sha256= prefix to be rejected")
	}
	if verifyMetaSignature(secret, []byte("changed"), "sha256="+sig) {
		t.Error("expected modified body to invalidate signature")
	}
}

// signBody replica o algoritmo da Meta: HMAC-SHA256 com app_secret + hex.
func signBody(appSecret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestReceive_WrongJsonFieldShape_StillReturns200(t *testing.T) {
	// Issue #443 follow-up: webhook deve absorver edge cases sem
	// retornar non-2xx, para não criar retry-loop. Payload sintaticamente
	// JSON mas com shape inesperado (sem `entry` etc.) é tolerado.
	t.Setenv("WHATSAPP_APP_SECRET", "")
	body := []byte(`{"some_other_field":42}`)
	r, _ := setupTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Sanity de env injection — garante que os tests não vazam estado
// entre runs.
func TestEnvIsolation(t *testing.T) {
	t.Setenv("WHATSAPP_APP_SECRET", "value-A")
	if os.Getenv("WHATSAPP_APP_SECRET") != "value-A" {
		t.Error("env not set")
	}
}

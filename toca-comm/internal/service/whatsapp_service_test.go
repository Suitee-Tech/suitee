package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

// newTestWhatsAppClient cria um WhatsAppBusinessClient apontando para
// `server` — toda comunicação fica em memória.
func newTestWhatsAppClient(t *testing.T, server *httptest.Server) *clients.WhatsAppBusinessClient {
	t.Helper()
	c, err := clients.NewWhatsAppBusinessClient(clients.WhatsAppConfig{
		AccessToken:   "T",
		PhoneNumberID: "P",
		APIVersion:    "v18.0",
		BaseURL:       server.URL,
	})
	if err != nil {
		t.Fatalf("NewWhatsAppBusinessClient: %v", err)
	}
	return c
}

// ────────────────────────────── WhatsAppService ──────────────────────────────

func TestWhatsAppService_NilReceiver_IsConfiguredFalse(t *testing.T) {
	// `nil` service deve responder IsConfigured()=false sem panic — protege
	// callers que façam `var svc *WhatsAppService; svc.IsConfigured()`.
	var svc *WhatsAppService
	if svc.IsConfigured() {
		t.Error("nil service should not be configured")
	}
}

func TestWhatsAppService_NotConfigured_ReturnsError(t *testing.T) {
	svc := NewWhatsAppService(WhatsAppServiceConfig{Client: nil})
	if svc.IsConfigured() {
		t.Error("service with nil client should not be configured")
	}
	_, err := svc.Send(context.Background(), "5511999999999", "subj", "body")
	if !errors.Is(err, clients.ErrWhatsAppNotConfigured) {
		t.Errorf("expected ErrWhatsAppNotConfigured, got %v", err)
	}
}

func TestWhatsAppService_TextMode_SendsBodyDirectly(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.TXT"}]}`)
	}))
	defer server.Close()

	svc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:      newTestWhatsAppClient(t, server),
		UseTemplate: false,
	})
	res, err := svc.Send(context.Background(), "5511999999999", "ignored subject", "Olá!")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.UsedTemplate {
		t.Error("expected UsedTemplate=false")
	}
	if res.WAMID != "wamid.TXT" {
		t.Errorf("WAMID: %q", res.WAMID)
	}
	if received["type"] != "text" {
		t.Errorf("expected type=text, got %v", received["type"])
	}
}

func TestWhatsAppService_TemplateMode_UsesDefaultTemplateWhenSubjectEmpty(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.TPL"}]}`)
	}))
	defer server.Close()

	svc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:           newTestWhatsAppClient(t, server),
		UseTemplate:      true,
		TemplateName:     "cobranca_default",
		TemplateLanguage: "pt_BR",
	})
	res, err := svc.Send(context.Background(), "5511999999999", "", "Sua cobrança de R$ 1.500")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !res.UsedTemplate || res.TemplateName != "cobranca_default" {
		t.Errorf("expected template=cobranca_default used, got UsedTemplate=%v Name=%q", res.UsedTemplate, res.TemplateName)
	}
	tpl, _ := received["template"].(map[string]any)
	if tpl["name"] != "cobranca_default" {
		t.Errorf("template name in payload: %v", tpl["name"])
	}
}

func TestWhatsAppService_TemplateMode_SubjectOverridesDefault(t *testing.T) {
	// Caller (régua de cobrança) pode mandar um template específico via
	// `subject` — ex.: "cobranca_atraso" para charges OVERDUE,
	// "cobranca_vencimento_proximo" para charges com lembrete 3 dias antes.
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.X"}]}`)
	}))
	defer server.Close()

	svc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:       newTestWhatsAppClient(t, server),
		UseTemplate:  true,
		TemplateName: "default_one",
	})
	_, err := svc.Send(context.Background(), "5511999999999", "cobranca_atraso", "param1")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	tpl, _ := received["template"].(map[string]any)
	if tpl["name"] != "cobranca_atraso" {
		t.Errorf("expected subject to override default template, got %v", tpl["name"])
	}
}

func TestWhatsAppService_TemplateMode_MissingTemplateNameErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when template name resolution fails")
	}))
	defer server.Close()

	svc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:      newTestWhatsAppClient(t, server),
		UseTemplate: true,
		// No TemplateName and no subject below → both empty
	})
	_, err := svc.Send(context.Background(), "5511999999999", "", "body")
	if !errors.Is(err, clients.ErrMetaAPIError) {
		t.Errorf("expected ErrMetaAPIError when no template configured, got %v", err)
	}
}

// ───────────────────────────── NotificationService ───────────────────────────

func TestNotificationService_UnsupportedType_Rejects(t *testing.T) {
	svc := NewNotificationService(nil)
	n := notification.New(notification.TypePush, "device-id", "subj", "body")
	if err := svc.Send(context.Background(), n); err == nil {
		t.Error("expected unsupported-type error for PUSH")
	}
}

func TestNotificationService_WhatsAppNotWired_ReturnsDescriptiveError(t *testing.T) {
	svc := NewNotificationService(nil) // sem WhatsApp wirado
	n := notification.New(notification.TypeWhatsApp, "5511999999999", "subj", "body")
	err := svc.Send(context.Background(), n)
	if err == nil || err.Error() != "whatsapp service not configured" {
		t.Errorf("expected 'whatsapp service not configured', got %v", err)
	}
}

func TestNotificationService_WhatsAppWired_DispatchesAndMarksSent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.ABC"}]}`)
	}))
	defer server.Close()

	waSvc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:      newTestWhatsAppClient(t, server),
		UseTemplate: false,
	})
	svc := NewNotificationService(nil).WithWhatsAppService(waSvc)

	n := notification.New(notification.TypeWhatsApp, "5511999999999", "subj", "Olá")
	if err := svc.Send(context.Background(), n); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if n.Status != notification.StatusSent {
		t.Errorf("expected status SENT, got %v", n.Status)
	}
	if n.SentAt == nil {
		t.Error("expected SentAt to be set after successful send")
	}
}

func TestNotificationService_WhatsAppMetaError_MarksFailed(t *testing.T) {
	// Meta retornou erro — NotificationService deve marcar status FAILED
	// e propagar o erro para o caller decidir o que fazer (loggar como
	// SkippedNote, retry, etc.).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, `{"error":{"message":"Invalid token","type":"OAuthException","code":190}}`)
	}))
	defer server.Close()

	waSvc := NewWhatsAppService(WhatsAppServiceConfig{
		Client:      newTestWhatsAppClient(t, server),
		UseTemplate: false,
	})
	svc := NewNotificationService(nil).WithWhatsAppService(waSvc)

	n := notification.New(notification.TypeWhatsApp, "5511999999999", "subj", "Olá")
	err := svc.Send(context.Background(), n)
	if err == nil {
		t.Fatal("expected error when Meta rejects token")
	}
	if !errors.Is(err, clients.ErrInvalidToken) {
		t.Errorf("expected error wrapped with ErrInvalidToken, got %v", err)
	}
	if n.Status != notification.StatusFailed {
		t.Errorf("expected status FAILED, got %v", n.Status)
	}
	if n.Error == "" {
		t.Error("expected Error field populated with diagnostic detail")
	}
}

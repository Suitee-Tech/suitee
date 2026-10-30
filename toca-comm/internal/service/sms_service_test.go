package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

// newTestSMSClient cria um SMSZenviaClient apontando para `server` —
// toda comunicação fica em memória. Retry desabilitado por padrão para
// os tests que não estão exercitando essa lógica (já coberta em
// pkg/clients).
func newTestSMSClient(t *testing.T, server *httptest.Server) *clients.SMSZenviaClient {
	t.Helper()
	c, err := clients.NewSMSZenviaClient(clients.SMSZenviaConfig{
		APIToken:         "T",
		From:             "TocaImoveis",
		BaseURL:          server.URL,
		RetryMaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("NewSMSZenviaClient: %v", err)
	}
	return c
}

// ────────────────────────────── SMSService ──────────────────────────────

func TestSMSService_NilReceiver_IsConfiguredFalse(t *testing.T) {
	// `nil` service deve responder IsConfigured()=false sem panic —
	// protege callers que façam `var svc *SMSService; svc.IsConfigured()`.
	var svc *SMSService
	if svc.IsConfigured() {
		t.Error("nil service should not be configured")
	}
}

func TestSMSService_NotConfigured_ReturnsError(t *testing.T) {
	svc := NewSMSService(SMSServiceConfig{Client: nil})
	if svc.IsConfigured() {
		t.Error("service with nil client should not be configured")
	}
	_, err := svc.Send(context.Background(), "5511999999999", "body")
	if !errors.Is(err, clients.ErrSMSNotConfigured) {
		t.Errorf("expected ErrSMSNotConfigured, got %v", err)
	}
}

func TestSMSService_SendsBodyAndReturnsMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"id":"msg-from-zenvia","from":"TocaImoveis","to":"5511999999999","direction":"OUT"}`)
	}))
	defer server.Close()

	svc := NewSMSService(SMSServiceConfig{Client: newTestSMSClient(t, server)})
	res, err := svc.Send(context.Background(), "5511999999999", "Sua cobrança vence amanhã.")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "msg-from-zenvia" {
		t.Errorf("expected msg-from-zenvia, got %q", res.MessageID)
	}
	if res.Recipient != "5511999999999" {
		t.Errorf("expected recipient normalized, got %q", res.Recipient)
	}
}

func TestSMSService_PropagatesBodyTooLong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when body > 160 chars")
	}))
	defer server.Close()

	svc := NewSMSService(SMSServiceConfig{Client: newTestSMSClient(t, server)})
	long := ""
	for i := 0; i < 161; i++ {
		long += "x"
	}
	_, err := svc.Send(context.Background(), "5511999999999", long)
	if !errors.Is(err, clients.ErrSMSBodyTooLong) {
		t.Errorf("expected ErrSMSBodyTooLong to bubble, got %v", err)
	}
}

// ───────────────────────────── NotificationService SMS ───────────────────

func TestNotificationService_SMSNotWired_ReturnsDescriptiveError(t *testing.T) {
	svc := NewNotificationService(nil) // sem SMS wirado
	n := notification.New(notification.TypeSMS, "5511999999999", "subj", "body")
	err := svc.Send(context.Background(), n)
	if err == nil || err.Error() != "sms service not configured" {
		t.Errorf("expected 'sms service not configured', got %v", err)
	}
}

func TestNotificationService_SMSWired_DispatchesAndMarksSent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"id":"msg-sms-abc","from":"TocaImoveis","to":"5511999999999","direction":"OUT"}`)
	}))
	defer server.Close()

	smsSvc := NewSMSService(SMSServiceConfig{Client: newTestSMSClient(t, server)})
	svc := NewNotificationService(nil).WithSMSService(smsSvc)

	n := notification.New(notification.TypeSMS, "5511999999999", "", "Lembrete: pagamento amanhã.")
	if err := svc.Send(context.Background(), n); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if n.Status != notification.StatusSent {
		t.Errorf("expected status SENT, got %v", n.Status)
	}
	if n.SentAt == nil {
		t.Error("expected SentAt populated")
	}
	if n.Error != "" {
		t.Errorf("expected Error empty on success, got %q", n.Error)
	}
}

func TestNotificationService_SMSZenviaError_MarksFailed(t *testing.T) {
	// Zenvia retornou 401 — NotificationService marca FAILED e propaga
	// erro tipado para o caller decidir o tratamento (skipNote, retry).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, `{"code":"AUTH_FAILED","message":"invalid token"}`)
	}))
	defer server.Close()

	smsSvc := NewSMSService(SMSServiceConfig{Client: newTestSMSClient(t, server)})
	svc := NewNotificationService(nil).WithSMSService(smsSvc)

	n := notification.New(notification.TypeSMS, "5511999999999", "", "x")
	err := svc.Send(context.Background(), n)
	if err == nil {
		t.Fatal("expected error when Zenvia rejects token")
	}
	if !errors.Is(err, clients.ErrSMSInvalidToken) {
		t.Errorf("expected error wrapped with ErrSMSInvalidToken, got %v", err)
	}
	if n.Status != notification.StatusFailed {
		t.Errorf("expected status FAILED, got %v", n.Status)
	}
	if n.Error == "" {
		t.Error("expected Error populated with diagnostic detail")
	}
}

func TestNotificationService_SMSWithNilClient_ReturnsConfigError(t *testing.T) {
	// Service wirado mas com client nil (cenário esquisito de mau wiring
	// no main.go) deve falhar de forma reconhecível, não panic.
	smsSvc := NewSMSService(SMSServiceConfig{Client: nil})
	svc := NewNotificationService(nil).WithSMSService(smsSvc)

	n := notification.New(notification.TypeSMS, "5511999999999", "", "x")
	err := svc.Send(context.Background(), n)
	if err == nil || err.Error() != "sms service not configured" {
		t.Errorf("expected 'sms service not configured' when client is nil, got %v", err)
	}
}

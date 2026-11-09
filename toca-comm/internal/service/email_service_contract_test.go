package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/sony/gobreaker"
)

func newResendContractService(server *httptest.Server, opts ...clients.HTTPClientOption) *EmailService {
	return newEmailService(config.EmailConfig{
		ResendAPIKey: "sandbox-key",
		FromAddress:  "Suitee <noreply@sandbox.example>",
	}, server.URL, time.Second, opts...)
}

func TestResendContract_SendRawEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Errorf("request = %s %s, want POST /emails", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sandbox-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["from"] != "Suitee <noreply@sandbox.example>" || body["subject"] != "Contrato disponível" {
			t.Errorf("unexpected raw-email payload: %#v", body)
		}
		to, _ := body["to"].([]any)
		if len(to) != 1 || to[0] != "cliente@example.com" {
			t.Errorf("unexpected recipients: %#v", body["to"])
		}
		if _, hasTemplate := body["template"]; hasTemplate {
			t.Errorf("raw email must not include template: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_sandbox_1"}`))
	}))
	defer server.Close()

	service := newResendContractService(server)
	if err := service.SendRawEmail(context.Background(), "cliente@example.com", "Contrato disponível", "<p>Olá</p>"); err != nil {
		t.Fatalf("SendRawEmail: %v", err)
	}
}

func TestResendContract_SendTemplateEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			From     string   `json:"from"`
			To       []string `json:"to"`
			Template struct {
				ID        string         `json:"id"`
				Variables map[string]any `json:"variables"`
			} `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Template.ID != "fc0c800d-ebad-46bf-96f5-279b9f08ef5b" {
			t.Errorf("template id = %q", body.Template.ID)
		}
		if body.Template.Variables["NAME"] != "Ana" || body.Template.Variables["SUITEE_URL"] != "https://sandbox.example/app" {
			t.Errorf("unexpected template variables: %#v", body.Template.Variables)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_sandbox_2"}`))
	}))
	defer server.Close()

	service := newResendContractService(server)
	if err := service.SendWelcomeEmail(context.Background(), "ana@example.com", "Ana", "https://sandbox.example/app"); err != nil {
		t.Fatalf("SendWelcomeEmail: %v", err)
	}
}

func TestResendContract_PasswordResetAndInvitationTemplates(t *testing.T) {
	var templates []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			To       []string `json:"to"`
			Template struct {
				ID string `json:"id"`
			} `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		templates = append(templates, body.Template.ID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_sandbox_3"}`))
	}))
	defer server.Close()

	service := newResendContractService(server)
	if err := service.SendPasswordResetEmail(context.Background(), "ana@example.com", "https://sandbox.example/reset"); err != nil {
		t.Fatalf("SendPasswordResetEmail: %v", err)
	}
	if err := service.SendInvitationEmail(context.Background(), "ana@example.com", "Imobiliária Exemplo", "https://sandbox.example/invite"); err != nil {
		t.Fatalf("SendInvitationEmail: %v", err)
	}
	want := []string{
		"91bf153d-a3d5-4f84-af68-9e49ee710613",
		"0551da37-6822-4675-b08f-9d2b9d0ee87b",
	}
	if len(templates) != len(want) {
		t.Fatalf("templates = %v, want %v", templates, want)
	}
	for i := range want {
		if templates[i] != want[i] {
			t.Fatalf("template[%d] = %q, want %q", i, templates[i], want[i])
		}
	}
}

func TestResendContract_MapsErrorAndEnforcesTimeout(t *testing.T) {
	t.Run("error body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"name":"validation_error","message":"invalid from"}`))
		}))
		defer server.Close()

		service := newResendContractService(server, clients.WithMaxAttempts(1))
		err := service.SendRawEmail(context.Background(), "ana@example.com", "assunto", "<p>x</p>")
		if err == nil || !strings.Contains(err.Error(), "status 422") || !strings.Contains(err.Error(), "invalid from") {
			t.Fatalf("expected structured Resend error, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
				_, _ = w.Write([]byte(`{"id":"late"}`))
			}
		}))
		defer server.Close()

		service := newEmailService(config.EmailConfig{
			ResendAPIKey: "sandbox-key",
			FromAddress:  "noreply@sandbox.example",
		}, server.URL, 10*time.Millisecond, clients.WithMaxAttempts(1))
		started := time.Now()
		err := service.SendRawEmail(context.Background(), "ana@example.com", "assunto", "<p>x</p>")
		if err == nil {
			t.Fatal("expected timeout")
		}
		if elapsed := time.Since(started); elapsed >= 300*time.Millisecond {
			t.Fatalf("configured timeout was not enforced: elapsed=%s", elapsed)
		}
	})
}

func TestResendContract_OpensCircuitAfterConsecutiveFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	service := newResendContractService(server)
	for i := 0; i < 5; i++ {
		if err := service.SendRawEmail(context.Background(), "ana@example.com", "assunto", "<p>x</p>"); err == nil {
			t.Fatalf("attempt %d: expected upstream failure", i+1)
		}
	}
	err := service.SendRawEmail(context.Background(), "ana@example.com", "assunto", "<p>x</p>")
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected open circuit, got %v", err)
	}
	if got := calls.Load(); got != 5 {
		t.Fatalf("open circuit should reject locally: calls=%d", got)
	}
}

package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/auth"
)

// O admin generate_link do GoTrue lê `redirect_to` no TOPO do payload; dentro de
// `options` ele é ignorado em silêncio e o link sai apontando para o site_url.
//
// Consequência real observada: o usuário clicava no email de recuperação e caía
// na raiz/login com a sessão de recuperação no fragmento da URL, sem tela para
// redefinir a senha. Este teste trava o formato do payload.
func TestGenerateRecoveryLinkSendsRedirectToAtTopLevel(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/admin/generate_link" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action_link":"http://auth/verify?token=x&type=recovery&redirect_to=http://app/reset-password"}`))
	}))
	defer server.Close()

	client := &AuthClient{
		baseURL:        server.URL,
		anonKey:        "anon",
		serviceRoleKey: "service",
		httpClient:     server.Client(),
	}

	link, err := client.GenerateRecoveryLink(context.Background(), "usuario@example.com", "http://app/reset-password")
	if err != nil {
		t.Fatalf("GenerateRecoveryLink: %v", err)
	}
	if link == "" {
		t.Fatal("link vazio")
	}

	if got["redirect_to"] != "http://app/reset-password" {
		t.Fatalf("redirect_to no topo = %v, want http://app/reset-password", got["redirect_to"])
	}
	// `options.redirect_to` era a forma antiga e é ignorada pelo GoTrue.
	if _, exists := got["options"]; exists {
		t.Fatalf("payload não deve usar options: %v", got["options"])
	}
	if got["type"] != "recovery" || got["email"] != "usuario@example.com" {
		t.Fatalf("campos básicos ausentes: %v", got)
	}
}

// Sem redirect o payload não deve ganhar a chave, para não enviar vazio.
func TestGenerateRecoveryLinkOmitsRedirectWhenEmpty(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action_link":"http://auth/verify?token=x"}`))
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "a", serviceRoleKey: "s", httpClient: server.Client()}
	if _, err := client.GenerateRecoveryLink(context.Background(), "u@example.com", ""); err != nil {
		t.Fatalf("GenerateRecoveryLink: %v", err)
	}
	if _, exists := got["redirect_to"]; exists {
		t.Fatalf("redirect_to não deveria existir: %v", got["redirect_to"])
	}
}

// O link próprio usa o hashed_token do generate_link e assina o TTL no
// backend; este teste garante que o token é extraído corretamente.
func TestGenerateRecoveryTokenHashExtractsHashedToken(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/admin/generate_link" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action_link":"http://auth/verify?token=x","hashed_token":"hash-abc","email_otp":"123"}`))
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "a", serviceRoleKey: "s", httpClient: server.Client()}
	hash, err := client.GenerateRecoveryTokenHash(context.Background(), "u@example.com")
	if err != nil {
		t.Fatalf("GenerateRecoveryTokenHash: %v", err)
	}
	if hash != "hash-abc" {
		t.Fatalf("hash = %q", hash)
	}
	if got["type"] != "recovery" || got["email"] != "u@example.com" {
		t.Fatalf("payload = %v", got)
	}
	if _, exists := got["redirect_to"]; exists {
		t.Fatalf("token hash não precisa de redirect_to: %v", got["redirect_to"])
	}
}

func TestVerifyRecoveryTokenHashExchangesForSession(t *testing.T) {
	var gotPath, gotAPIKey, gotAuth string
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAPIKey, gotAuth = r.URL.Path, r.Header.Get("apikey"), r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-hash","refresh_token":"refresh-hash","user":{"id":"user-hash","email":"u@example.com"}}`))
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "anon", serviceRoleKey: "service", httpClient: server.Client()}
	sess, err := client.VerifyRecoveryTokenHash(context.Background(), "hash-abc")
	if err != nil {
		t.Fatalf("VerifyRecoveryTokenHash: %v", err)
	}
	if gotPath != "/auth/v1/verify" || gotAPIKey != "anon" || gotAuth != "" {
		t.Fatalf("verify request path/key/auth = %q/%q/%q", gotPath, gotAPIKey, gotAuth)
	}
	if got["type"] != "recovery" || got["token_hash"] != "hash-abc" {
		t.Fatalf("payload = %v", got)
	}
	if sess.AccessToken != "access-hash" || sess.RefreshToken != "refresh-hash" || sess.UserID != "user-hash" {
		t.Fatalf("session = %+v", sess)
	}
}

// GetUser valida o access token do link de recuperação no provedor e devolve o
// dono da sessão. Precisa mandar a anon key (não a service role) e o token como
// Bearer — é o mesmo contrato que o cliente Supabase usava no navegador.
func TestGetUserUsesAnonKeyAndBearerToken(t *testing.T) {
	var gotAuth, gotAPIKey, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("apikey")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"user-123","email":"usuario@example.com"}`))
	}))
	defer server.Close()

	client := &AuthClient{
		baseURL:        server.URL,
		anonKey:        "anon-key",
		serviceRoleKey: "service-role-key",
		httpClient:     server.Client(),
	}

	userID, err := client.GetUser(context.Background(), "access-do-link")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("user id = %q, want user-123", userID)
	}
	if gotPath != "/auth/v1/user" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAPIKey != "anon-key" || gotAuth != "Bearer access-do-link" {
		t.Fatalf("headers inesperados: apikey=%q authorization=%q", gotAPIKey, gotAuth)
	}
}

func TestGetUserRejectsEmptyToken(t *testing.T) {
	client := &AuthClient{baseURL: "http://127.0.0.1:1", anonKey: "anon"}
	if _, err := client.GetUser(context.Background(), "   "); err == nil {
		t.Fatal("token vazio deveria falhar sem chamar o provedor")
	}
}

// O Admin API cria o usuário antes do grant de senha. Se o grant falhar (por
// exemplo, provider de e-mail desligado), o usuário precisa ser removido para
// não ficar órfão e bloquear um novo cadastro com "email_exists".
func TestSignUpDeletesCreatedUserWhenSignInFails(t *testing.T) {
	var created, deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/admin/users":
			created = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"user-orphan","email":"pessoa@example.com"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/token":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"code":422,"error_code":"email_provider_disabled","msg":"Email logins are disabled"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/auth/v1/admin/users/user-orphan":
			deleted = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "anon", serviceRoleKey: "service", httpClient: server.Client()}
	_, err := client.SignUp(context.Background(), "pessoa@example.com", "Test1234!")
	if !errors.Is(err, auth.ErrSignUpFailed) {
		t.Fatalf("erro = %v, want ErrSignUpFailed", err)
	}
	if !created || !deleted {
		t.Fatalf("created=%v deleted=%v; o usuário criado precisa ser removido", created, deleted)
	}
	for _, secret := range []string{"user-orphan", "email_provider_disabled", "Email logins"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("erro expõe detalhe interno %q: %v", secret, err)
		}
	}
}

// O admin create devolve email_exists (e "already been registered"); o matcher
// antigo só conhecia user_already_exists/"already registered" e deixava o erro
// bruto subir como 500.
func TestSignUpMapsExistingEmailToConflictSentinel(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/admin/users":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"code":422,"error_code":"email_exists","msg":"A user with this email address has already been registered"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/token":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		case r.Method == http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "anon", serviceRoleKey: "service", httpClient: server.Client()}
	_, err := client.SignUp(context.Background(), "pessoa@example.com", "Test1234!")
	if !errors.Is(err, auth.ErrEmailAlreadyExists) {
		t.Fatalf("erro = %v, want ErrEmailAlreadyExists", err)
	}
	if deleted {
		t.Fatal("não houve criação nesta chamada; DELETE não deveria ser executado")
	}
	if strings.Contains(err.Error(), "pessoa@example.com") {
		t.Fatalf("erro expõe o e-mail: %v", err)
	}
}

func TestSignUpReturnsSessionWhenSignInSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/admin/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"user-1","email":"pessoa@example.com"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","user":{"id":"user-1","email":"pessoa@example.com"}}`))
		default:
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "anon", serviceRoleKey: "service", httpClient: server.Client()}
	sess, err := client.SignUp(context.Background(), "pessoa@example.com", "Test1234!")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if sess == nil || sess.UserID != "user-1" || sess.AccessToken != "access" {
		t.Fatalf("sessão inesperada: %+v", sess)
	}
}

func TestSignUpRollbackFailureStillReturnsGenericError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/admin/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"user-orphan","email":"pessoa@example.com"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/token":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error_code":"email_provider_disabled"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"msg":"cannot delete user-orphan"}`))
		default:
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := &AuthClient{baseURL: server.URL, anonKey: "anon", serviceRoleKey: "service", httpClient: server.Client()}
	_, err := client.SignUp(context.Background(), "pessoa@example.com", "Test1234!")
	if !errors.Is(err, auth.ErrSignUpFailed) {
		t.Fatalf("erro = %v, want ErrSignUpFailed", err)
	}
	if strings.Contains(err.Error(), "user-orphan") || strings.Contains(err.Error(), "cannot delete") {
		t.Fatalf("erro de rollback vazou detalhe: %v", err)
	}
}

func TestIsDuplicateUserError(t *testing.T) {
	duplicates := []string{
		`supabase request failed: status 422: {"error_code":"email_exists"}`,
		`supabase request failed: status 422: {"error_code":"user_already_exists"}`,
		`supabase request failed: status 422: {"msg":"User already registered"}`,
		`supabase request failed: status 422: {"msg":"A user with this email address has already been registered"}`,
	}
	for _, msg := range duplicates {
		if !isDuplicateUserError(errors.New(msg)) {
			t.Fatalf("deveria reconhecer duplicidade: %s", msg)
		}
	}
	if isDuplicateUserError(errors.New(`supabase request failed: status 422: {"error_code":"email_provider_disabled"}`)) {
		t.Fatal("provider desligado não é duplicidade")
	}
}

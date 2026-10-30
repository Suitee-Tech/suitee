package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/auth"
)

// errFakeRefresh simula o provedor recusando o refresh token (link expirado /
// já usado).
var errFakeRefresh = errors.New("invalid refresh token")

// O link de recuperação precisa cair na TELA DE REDEFINIÇÃO.
//
// O Supabase anexa a sessão de recuperação no fragmento da URL
// (#access_token=...&type=recovery) da página de redirect. Quando o alvo era
// /login, o usuário via o formulário de login e os tokens ficavam ignorados no
// fragmento — exatamente o "o link não carrega a tela de redefinir senha".
func TestForgotPasswordRedirectsToResetPasswordPage(t *testing.T) {
	fake := &fakeSupabase{}
	cfg := &app.Config{FrontendURL: "http://localhost:3000"}
	authSvc := &AuthService{config: cfg, supabaseAuth: fake}
	svc := NewPasswordService(nil, authSvc, nil, cfg)

	if err := svc.ForgotPassword(context.Background(), &auth.ForgotPasswordRequest{Email: "usuario@example.com"}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}

	// O fake devolve redirectTo + fragmento de recuperação.
	if !strings.HasPrefix(fake.recoveryLink, "http://localhost:3000/reset-password#") {
		t.Fatalf("link = %q, want base .../reset-password com o fragmento de recuperação", fake.recoveryLink)
	}
	if !strings.Contains(fake.recoveryLink, "type=recovery") {
		t.Fatalf("link sem type=recovery: %q", fake.recoveryLink)
	}
}

// Barra final na configuração não pode gerar "//reset-password".
func TestForgotPasswordNormalizesTrailingSlash(t *testing.T) {
	fake := &fakeSupabase{}
	cfg := &app.Config{FrontendURL: "http://localhost:3000/"}
	authSvc := &AuthService{config: cfg, supabaseAuth: fake}
	svc := NewPasswordService(nil, authSvc, nil, cfg)

	if err := svc.ForgotPassword(context.Background(), &auth.ForgotPasswordRequest{Email: "usuario@example.com"}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if strings.Contains(fake.recoveryLink, "//reset-password") {
		t.Fatalf("barra duplicada no link: %q", fake.recoveryLink)
	}
}

// Reset de senha mediado pelo backend: o par do link de recuperação é validado
// no servidor e a senha é trocada pela API admin — o navegador não fala mais
// com o Supabase (era o motivo da anon key existir em /env.js).
func TestResetPasswordWithRecoveryAppliesPasswordAndRevokesSessions(t *testing.T) {
	fake := &fakeSupabase{}
	cfg := &app.Config{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)

	if err := svc.ResetPasswordWithRecovery(context.Background(), "access-from-link", "refresh-from-link", "NovaSenha1!"); err != nil {
		t.Fatalf("ResetPasswordWithRecovery: %v", err)
	}
	if fake.refreshIn != "refresh-from-link" {
		t.Fatalf("refresh token não foi trocado no provedor: %q", fake.refreshIn)
	}
	if len(fake.getUserTokens) != 1 || fake.getUserTokens[0] != "access-from-link" {
		t.Fatalf("access token do link não foi validado: %v", fake.getUserTokens)
	}
	if len(fake.passwordUpdates) != 1 {
		t.Fatalf("esperava 1 troca de senha, got %d", len(fake.passwordUpdates))
	}
	if got := fake.passwordUpdates[0]; got.userID != "acc1" || got.password != "NovaSenha1!" {
		t.Fatalf("troca de senha inesperada: %+v", got)
	}
	// A senha nova derruba as sessões antigas — com o access token NOVO (o do
	// link pode já estar expirado).
	if len(fake.logoutAllTokens) != 1 || fake.logoutAllTokens[0] != "access-refreshed" {
		t.Fatalf("sessões não foram revogadas com o token renovado: %v", fake.logoutAllTokens)
	}
}

func TestBuildRecoveryURLSignsTokenHashWithTTL(t *testing.T) {
	cfg := &app.Config{FrontendURL: "https://app.example.com/", PasswordResetSigningKey: "test-key", PasswordResetTTLMinutes: 60}
	svc := NewPasswordService(nil, nil, nil, cfg)

	link, err := svc.buildRecoveryURL("hash-abc")
	if err != nil {
		t.Fatalf("buildRecoveryURL: %v", err)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/reset-password" {
		t.Fatalf("path = %q", u.Path)
	}
	params, _ := url.ParseQuery(strings.TrimPrefix(u.Fragment, ""))
	if params.Get("token_hash") != "hash-abc" || params.Get("type") != "recovery" {
		t.Fatalf("fragment = %q", u.Fragment)
	}
	exp, err := strconv.ParseInt(params.Get("exp"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if exp < now+59*60 || exp > now+61*60 {
		t.Fatalf("exp = %d, want ~60 minutos", exp)
	}
	if params.Get("sig") != recoverySignature(cfg.PasswordResetSigningKey, "hash-abc", exp) {
		t.Fatal("assinatura do link não corresponde")
	}
}

func TestResetPasswordWithRecoveryTokenHashAppliesPassword(t *testing.T) {
	cfg := &app.Config{PasswordResetSigningKey: "test-key", PasswordResetTTLMinutes: 60}
	fake := &fakeSupabase{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	exp := time.Now().Add(time.Hour).Unix()
	sig := recoverySignature(cfg.PasswordResetSigningKey, "valid-recovery-hash", exp)

	if err := svc.ResetPasswordWithRecoveryTokenHash(context.Background(), "valid-recovery-hash", exp, sig, "NovaSenha1!"); err != nil {
		t.Fatalf("ResetPasswordWithRecoveryTokenHash: %v", err)
	}
	if len(fake.passwordUpdates) != 1 || fake.passwordUpdates[0].password != "NovaSenha1!" {
		t.Fatalf("senha não foi atualizada via token_hash: %+v", fake.passwordUpdates)
	}
	if fake.refreshIn != "refresh-from-hash" {
		t.Fatalf("refresh token da sessão do token_hash não foi usado: %q", fake.refreshIn)
	}
}

func TestResetPasswordWithRecoveryTokenHashRejectsInvalidHash(t *testing.T) {
	cfg := &app.Config{PasswordResetSigningKey: "test-key", PasswordResetTTLMinutes: 60}
	fake := &fakeSupabase{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	exp := time.Now().Add(time.Hour).Unix()
	sig := recoverySignature(cfg.PasswordResetSigningKey, "expired-hash", exp)

	err := svc.ResetPasswordWithRecoveryTokenHash(context.Background(), "expired-hash", exp, sig, "NovaSenha1!")
	if !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("esperava ErrRecoverySessionInvalid, got %v", err)
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("hash inválido não pode trocar senha")
	}
}

func TestResetPasswordWithRecoveryTokenHashRejectsExpiredSignature(t *testing.T) {
	cfg := &app.Config{PasswordResetSigningKey: "test-key", PasswordResetTTLMinutes: 60}
	fake := &fakeSupabase{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	exp := time.Now().Add(-time.Minute).Unix()
	sig := recoverySignature(cfg.PasswordResetSigningKey, "valid-recovery-hash", exp)

	err := svc.ResetPasswordWithRecoveryTokenHash(context.Background(), "valid-recovery-hash", exp, sig, "NovaSenha1!")
	if !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("link vencido deveria ser recusado, veio %v", err)
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("link vencido não pode trocar senha")
	}
}

func TestResetPasswordWithRecoveryTokenHashRejectsUnsignedLink(t *testing.T) {
	cfg := &app.Config{PasswordResetSigningKey: "test-key", PasswordResetTTLMinutes: 60}
	fake := &fakeSupabase{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)

	if err := svc.ResetPasswordWithRecoveryTokenHash(context.Background(), "valid-recovery-hash", 0, "", "NovaSenha1!"); !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("link sem assinatura deveria ser recusado, veio %v", err)
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("link sem assinatura não pode trocar senha")
	}
}

func TestResetPasswordWithRecoveryRejectsInvalidSession(t *testing.T) {
	cfg := &app.Config{}

	// Refresh token recusado pelo provedor.
	fake := &fakeSupabase{refreshErr: errFakeRefresh}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	err := svc.ResetPasswordWithRecovery(context.Background(), "access", "refresh", "NovaSenha1!")
	if !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("esperava ErrRecoverySessionInvalid, got %v", err)
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("não pode trocar senha com sessão inválida")
	}

	// Access token de OUTRO usuário: o par não é o do link.
	fake = &fakeSupabase{getUserID: "outro-user"}
	svc = NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	err = svc.ResetPasswordWithRecovery(context.Background(), "access-de-outro", "refresh", "NovaSenha1!")
	if !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("esperava ErrRecoverySessionInvalid para dono divergente, got %v", err)
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("não pode trocar senha com par divergente")
	}

	// Sem refresh token não há prova de sessão.
	fake = &fakeSupabase{}
	svc = NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)
	if err := svc.ResetPasswordWithRecovery(context.Background(), "access", "  ", "NovaSenha1!"); !errors.Is(err, ErrRecoverySessionInvalid) {
		t.Fatalf("esperava ErrRecoverySessionInvalid sem refresh token, got %v", err)
	}
}

// Access token expirado (o usuário demorou para abrir o link) não pode impedir
// o reset: a prova de identidade é o refresh token. GetUser só é usado para
// comparar o dono QUANDO o provedor ainda aceita o token.
func TestResetPasswordWithRecoveryToleratesExpiredAccessToken(t *testing.T) {
	cfg := &app.Config{}
	fake := &fakeSupabase{getUserErr: errFakeRefresh}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)

	if err := svc.ResetPasswordWithRecovery(context.Background(), "access-expirado", "refresh-valido", "NovaSenha1!"); err != nil {
		t.Fatalf("ResetPasswordWithRecovery: %v", err)
	}
	if len(fake.passwordUpdates) != 1 {
		t.Fatalf("esperava a troca mesmo com access token expirado, got %d", len(fake.passwordUpdates))
	}
}

func TestResetPasswordWithRecoveryEnforcesPasswordComplexity(t *testing.T) {
	cfg := &app.Config{}
	fake := &fakeSupabase{}
	svc := NewPasswordService(nil, &AuthService{config: cfg, supabaseAuth: fake}, nil, cfg)

	if err := svc.ResetPasswordWithRecovery(context.Background(), "access", "refresh", "fraca"); err == nil {
		t.Fatal("senha fraca deveria ser recusada")
	}
	if len(fake.passwordUpdates) != 0 {
		t.Fatal("senha fraca não pode chegar ao provedor")
	}
	if fake.refreshIn != "" {
		t.Fatal("senha fraca deve ser recusada antes de qualquer chamada ao provedor")
	}
}

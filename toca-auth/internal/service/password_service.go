package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	domainevents "github.com/oginoino/toca-auth/internal/domain/events"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
)

type PasswordService struct {
	credentialRepo auth.Repository
	authService    *AuthService
	eventBus       events.EventBus
	config         *app.Config
}

// NewPasswordService — o repositório de token local
// (`toca_auth.password_resets`) saiu da assinatura junto com o fluxo que o usava
// (P1/A14). A tabela continua existindo no banco; limpá-la é decisão de migração.
func NewPasswordService(
	credRepo auth.Repository,
	authSvc *AuthService,
	eventBus events.EventBus,
	cfg *app.Config,
) *PasswordService {
	return &PasswordService{
		credentialRepo: credRepo,
		authService:    authSvc,
		eventBus:       eventBus,
		config:         cfg,
	}
}

func (s *PasswordService) ForgotPassword(ctx context.Context, req *auth.ForgotPasswordRequest) error {
	if s.authService == nil || s.authService.supabaseAuth == nil {
		return nil
	}

	// O link precisa cair na TELA DE REDEFINIÇÃO, não no login.
	//
	// O Supabase devolve a sessão de recuperação no fragmento da URL
	// (#access_token=...&type=recovery) da página de redirect. Apontando para
	// /login, o usuário via o formulário de login com os tokens ignorados no
	// fragmento — o "link não abre a redefinição" relatado. A tela
	// /reset-password lê esse fragmento via getRecoverySessionFromUrl.
	redirectTo := ""
	if s.config.FrontendURL != "" {
		base := strings.TrimRight(s.config.FrontendURL, "/")
		redirectTo = fmt.Sprintf("%s/reset-password", base)
	}

	resetURL := ""
	var err error
	if hashed, hashErr := s.authService.supabaseAuth.GenerateRecoveryTokenHash(ctx, req.Email); hashErr == nil && hashed != "" {
		if signedURL, signErr := s.buildRecoveryURL(hashed); signErr == nil {
			resetURL = signedURL
		}
	}
	if resetURL == "" {
		// Fallback: link gerenciado pelo GoTrue (o redirect pode cair no Site
		// URL do projeto, mas mantém o fluxo funcionando se a assinatura local
		// não estiver configurada).
		resetURL, err = s.authService.supabaseAuth.GenerateRecoveryLink(ctx, req.Email, redirectTo)
	}
	if err != nil || resetURL == "" {
		return nil
	}

	logger.Info("Password reset link generated", "email", req.Email)

	published := false
	if s.eventBus != nil {
		eventPayload := domainevents.PasswordResetRequestedEvent{
			Email:    req.Email,
			Token:    "",
			ResetURL: resetURL,
		}
		if err := s.eventBus.Publish(ctx, eventPayload); err != nil {
			logger.Error("Failed to publish password reset event", "error", err)
		} else {
			published = true
		}
	}

	if !published {
		html := fmt.Sprintf("<p>Para redefinir sua senha, clique no link abaixo:</p><p><a href=\"%s\" target=\"_blank\" rel=\"noreferrer\">Redefinir senha</a></p><p>Se você não solicitou, ignore este e-mail.</p>", resetURL)
		comm := clients.NewCommClient()
		// Sem TenantID de propósito (P1/A3): quem pede a recuperação ainda não
		// tem tenant — o toca-comm registra a mensagem como SISTEMA em vez de
		// recusá-la, e a recuperação de conta continua funcionando.
		if err := comm.SendNotification(ctx, clients.SendNotificationRequest{
			Type:      "EMAIL",
			Recipient: req.Email,
			Subject:   "Recuperação de senha",
			Content:   html,
		}); err != nil {
			logger.Error("Failed to send password reset email fallback", "error", err)
		}
	}

	return nil
}

// P1/A14 — `ResetPassword` e `VerifyPasswordResetToken` foram REMOVIDOS.
//
// O fluxo local (token em `toca_auth.password_resets`) nunca funcionou:
// `passwordResetRepo.Create` não era chamado em lugar nenhum, então
// `/auth/reset-password` sempre respondia "invalid token" e o `/verify` sempre
// "invalid or expired token". A redefinição de senha acontece pelo link de
// recuperação do Supabase — `ForgotPassword` abaixo pede o link ao provedor e o
// cliente aplica a nova senha com a sessão de recuperação
// (`resetPasswordWithRecoverySession`). Manter um segundo emissor de token de
// recuperação seria uma credencial mais fraca que a do provedor.

// ErrRecoverySessionInvalid: o par access/refresh do link de recuperação não
// vale mais (expirado, já usado ou de outro projeto). Vira 401 no handler.
var ErrRecoverySessionInvalid = errors.New("invalid or expired recovery session")

// ResetPasswordWithRecovery aplica a nova senha a partir da SESSÃO DE
// RECUPERAÇÃO (par access/refresh que o link do Supabase entrega no fragmento
// da URL), sem o navegador falar com o Supabase.
//
// Antes, quem trocava a senha era o próprio browser (`supabase.auth.setSession`
// + `updateUser`), e esse era o ÚNICO motivo para o frontend carregar a URL e a
// anon key do Supabase — um valor com cara de segredo visível em /env.js. Aqui
// o par é validado no servidor e a senha é trocada pela API admin (service
// role), que nunca chega ao cliente.
//
// A prova de identidade é o refresh token: trocá-lo por uma sessão no provedor
// exige que ele seja legítimo. O access token, quando ainda válido, precisa ser
// do MESMO usuário — é o equivalente server-side do que o cliente Supabase
// validava ao montar a sessão.
func (s *PasswordService) ResetPasswordWithRecovery(ctx context.Context, accessToken, refreshToken, newPassword string) error {
	if s.authService == nil || s.authService.supabaseAuth == nil {
		return errors.New("authentication provider is not configured")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return ErrRecoverySessionInvalid
	}
	if err := auth.ValidatePasswordComplexity(newPassword); err != nil {
		return err
	}

	session, err := s.authService.supabaseAuth.RefreshToken(ctx, refreshToken)
	if err != nil || session == nil || session.UserID == "" {
		logger.Info("auth password reset rejected", "event", "auth.password_reset_rejected", "reason", "refresh_exchange_failed")
		return ErrRecoverySessionInvalid
	}

	// Access token expirado é normal (o link pode ter sido aberto um tempo
	// depois): só comparamos o dono quando o provedor ainda o aceita.
	if ownerID, ownerErr := s.authService.supabaseAuth.GetUser(ctx, accessToken); ownerErr == nil && ownerID != "" && ownerID != session.UserID {
		logger.Info("auth password reset rejected", "event", "auth.password_reset_rejected", "reason", "token_owner_mismatch")
		return ErrRecoverySessionInvalid
	}

	if err := s.authService.supabaseAuth.UpdateUserPassword(ctx, session.UserID, newPassword); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Senha nova derruba as sessões antigas (inclusive a de recuperação). Não
	// falha o reset se a revogação não responder: a senha já mudou e o usuário
	// não pode ver erro num fluxo que deu certo — o log guarda o rastro.
	if err := s.authService.supabaseAuth.LogoutAll(ctx, session.AccessToken); err != nil {
		logger.Error("auth password reset: failed to revoke sessions", "error", err, "account_id", session.UserID)
	}

	logger.Info("auth password reset",
		"event", "auth.password_reset",
		"account_id", session.UserID)

	return nil
}

// ResetPasswordWithRecoveryTokenHash troca o token_hash do link próprio por
// uma sessão no provedor e aplica a nova senha. Não depende do Site URL nem da
// allowlist de redirect do GoTrue — o link do e-mail aponta direto para a
// nossa tela /reset-password.
func (s *PasswordService) ResetPasswordWithRecoveryTokenHash(ctx context.Context, tokenHash string, expiresAt int64, signature, newPassword string) error {
	if s.authService == nil || s.authService.supabaseAuth == nil {
		return errors.New("authentication provider is not configured")
	}
	if strings.TrimSpace(tokenHash) == "" {
		return ErrRecoverySessionInvalid
	}
	if !s.validRecoverySignature(tokenHash, expiresAt, signature) {
		logger.Info("auth password reset rejected", "event", "auth.password_reset_rejected", "reason", "invalid_or_expired_signature")
		return ErrRecoverySessionInvalid
	}
	if err := auth.ValidatePasswordComplexity(newPassword); err != nil {
		return err
	}
	session, err := s.authService.supabaseAuth.VerifyRecoveryTokenHash(ctx, tokenHash)
	if err != nil || session == nil || session.AccessToken == "" || session.RefreshToken == "" {
		logger.Info("auth password reset rejected", "event", "auth.password_reset_rejected", "reason", "token_hash_exchange_failed")
		return ErrRecoverySessionInvalid
	}
	return s.ResetPasswordWithRecovery(ctx, session.AccessToken, session.RefreshToken, newPassword)
}

func (s *PasswordService) buildRecoveryURL(tokenHash string) (string, error) {
	base := strings.TrimRight(s.config.FrontendURL, "/")
	key := strings.TrimSpace(s.config.PasswordResetSigningKey)
	if base == "" || key == "" {
		return "", errors.New("password reset link signing is not configured")
	}
	ttlMinutes := s.config.PasswordResetTTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 60
	}
	expiresAt := time.Now().Add(time.Duration(ttlMinutes) * time.Minute).Unix()
	signature := recoverySignature(key, tokenHash, expiresAt)
	return fmt.Sprintf("%s/reset-password#token_hash=%s&exp=%d&sig=%s&type=recovery",
		base, url.QueryEscape(tokenHash), expiresAt, url.QueryEscape(signature)), nil
}

func recoverySignature(key, tokenHash string, expiresAt int64) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(tokenHash + "|" + strconv.FormatInt(expiresAt, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validRecoverySignature exige expiração e assinatura em TODO link com
// token_hash. Aceitar links sem assinatura permitiria remover exp/sig do link
// novo e contornar o TTL local.
func (s *PasswordService) validRecoverySignature(tokenHash string, expiresAt int64, signature string) bool {
	key := strings.TrimSpace(s.config.PasswordResetSigningKey)
	if key == "" || expiresAt <= time.Now().Unix() || strings.TrimSpace(signature) == "" {
		return false
	}
	expected := recoverySignature(key, tokenHash, expiresAt)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (s *PasswordService) ChangePassword(ctx context.Context, accessToken, accountID, currentPwd, newPwd string) error {
	if s.authService == nil || s.authService.supabaseAuth == nil {
		return errors.New("authentication provider is not configured")
	}

	acc, err := s.credentialRepo.GetByAccountID(ctx, accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return errors.New("credential not found")
	}

	if err := auth.ValidatePasswordComplexity(newPwd); err != nil {
		return err
	}

	// Confere a senha atual autenticando no provedor — é a única checagem que
	// reflete a credencial que o login usa.
	if _, err := s.authService.supabaseAuth.SignInWithPassword(ctx, acc.Email, currentPwd); err != nil {
		return errors.New("invalid current password")
	}

	if err := s.authService.supabaseAuth.UpdateUserPassword(ctx, accountID, newPwd); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// A senha mudou: as sessões antigas precisam cair. O access token do
	// chamador é o que o Supabase espera (não o account id).
	if err := s.authService.LogoutAll(ctx, accessToken); err != nil {
		return fmt.Errorf("password updated but failed to revoke sessions: %w", err)
	}

	logger.Info("auth password changed",
		"event", "auth.password_changed",
		"account_id", accountID)

	return nil
}

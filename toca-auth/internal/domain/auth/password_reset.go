package auth

import (
	"errors"
	"strings"

	"time"
)

type PasswordResetToken struct {
	ID        string    `bson:"_id,omitempty"`
	Email     string    `bson:"email"`
	Token     string    `bson:"token"`
	ExpiresAt time.Time `bson:"expires_at"`
	Used      bool      `bson:"used"`
	CreatedAt time.Time `bson:"created_at"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordWithSessionRequest carrega a SESSÃO DE RECUPERAÇÃO que o link do
// Supabase entrega no fragmento da URL (#access_token=...&refresh_token=...
// &type=recovery) mais a senha nova.
//
// O par de tokens vai para o backend, que valida e troca a senha no provedor
// com a service role. Substitui o `ResetPasswordRequest` (token local em
// `toca_auth.password_resets`), removido no P1/A14 porque o token nunca era
// criado — e substitui também o caminho em que o navegador chamava o Supabase
// direto, que era o único motivo para a anon key existir em /env.js.
type ResetPasswordWithSessionRequest struct {
	// Links novos entregam token_hash no fragmento e a troca acontece no
	// backend (POST /auth/v1/verify). O par access/refresh continua aceito
	// para links antigos já emitidos.
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenHash    string `json:"token_hash,omitempty"`
	// ExpiresAt/Signature são gerados pelo nosso link (HMAC do token_hash +
	// timestamp). Links antigos podem vir sem eles.
	ExpiresAt   int64  `json:"exp,omitempty"`
	Signature   string `json:"sig,omitempty"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

func (r *ResetPasswordWithSessionRequest) Validate() error {
	if err := validate.Struct(r); err != nil {
		return err
	}
	if strings.TrimSpace(r.TokenHash) == "" && (strings.TrimSpace(r.AccessToken) == "" || strings.TrimSpace(r.RefreshToken) == "") {
		return errors.New("informe o token_hash ou a sessão de recuperação")
	}
	return nil
}

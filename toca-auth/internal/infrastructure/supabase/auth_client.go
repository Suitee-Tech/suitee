package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
)

type AuthClient struct {
	baseURL        string
	anonKey        string
	serviceRoleKey string
	httpClient     *http.Client
}

type Client interface {
	SignUp(ctx context.Context, email, password string) (*Session, error)
	SignInWithPassword(ctx context.Context, email, password string) (*Session, error)
	RefreshToken(ctx context.Context, refreshToken string) (*Session, error)
	Logout(ctx context.Context, accessToken string) error
	LogoutAll(ctx context.Context, accessToken string) error
	GenerateEmailOTP(ctx context.Context, email string) (string, error)
	GenerateRecoveryLink(ctx context.Context, email string, redirectTo string) (string, error)
	// SendOTP envia o código. `createUser` decide se o Supabase pode criar uma
	// identidade nova para o contato — M14: o fluxo de OTP só envia para contato
	// existente, então passa false (antes era `true` fixo, o que criava usuário
	// para telefone arbitrário).
	SendOTP(ctx context.Context, email, phone, channel string, createUser bool) error
	VerifyOTP(ctx context.Context, typ, email, phone, token string) (*Session, error)
	UpdateAppMetadata(ctx context.Context, userID string, appMetadata map[string]interface{}) error
	// UpdateUserPassword troca a senha na fonte de verdade (Supabase/GoTrue).
	UpdateUserPassword(ctx context.Context, userID, password string) error
	// GetUser devolve o id do dono de um access token (validação server-side da
	// sessão de recuperação enviada pelo navegador).
	GetUser(ctx context.Context, accessToken string) (string, error)
	// GenerateRecoveryTokenHash devolve o token_hash de recuperação para o
	// link próprio (com validade assinada pelo backend).
	GenerateRecoveryTokenHash(ctx context.Context, email string) (string, error)
	// VerifyRecoveryTokenHash troca o token_hash do link de recuperação por uma
	// sessão. É o caminho que não depende do Site URL/allowlist do GoTrue.
	VerifyRecoveryTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteUser(ctx context.Context, userID string) error
}

type Session struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	Email        string
}

func NewAuthClientFromEnv() (*AuthClient, error) {
	baseURL := strings.TrimRight(config.GetEnv("SUPABASE_URL", ""), "/")
	anonKey := strings.TrimSpace(config.GetEnv("SUPABASE_ANON_KEY", ""))
	serviceRoleKey := strings.TrimSpace(config.GetEnv("SUPABASE_SERVICE_ROLE_KEY", ""))
	if baseURL == "" || anonKey == "" || serviceRoleKey == "" {
		return nil, fmt.Errorf("missing required supabase env vars")
	}
	return &AuthClient{
		baseURL:        baseURL,
		anonKey:        anonKey,
		serviceRoleKey: serviceRoleKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

func (c *AuthClient) SignUp(ctx context.Context, email, password string) (*Session, error) {
	payload := map[string]interface{}{
		"email":         email,
		"password":      password,
		"email_confirm": true,
	}

	var adminResp struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := c.doServiceJSON(ctx, http.MethodPost, "/auth/v1/admin/users", payload, &adminResp); err != nil {
		if isDuplicateUserError(err) {
			// O e-mail já existe no provedor. Se a senha conferir, devolvemos a
			// sessão do usuário existente (retry após um cadastro interrompido);
			// caso contrário, o conflito é genérico e não confirma a conta.
			sess, signInErr := c.SignInWithPassword(ctx, email, password)
			if signInErr == nil && sess != nil {
				return sess, nil
			}
			return nil, auth.ErrEmailAlreadyExists
		}
		return nil, err
	}

	sess, err := c.SignInWithPassword(ctx, email, password)
	if err != nil || sess == nil {
		// O usuário já foi criado no GoTrue. Sem sessão ele ficaria órfão e
		// bloquearia um novo cadastro. A limpeza usa contexto próprio para não
		// depender do cancelamento da requisição do cliente e é best-effort: o
		// erro devolvido não carrega detalhe do provedor nem da conta.
		if adminResp.ID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			deleteErr := c.DeleteUser(cleanupCtx, adminResp.ID)
			cancel()
			if deleteErr != nil {
				logger.FromContext(ctx).WarnContext(ctx,
					"supabase user rollback failed after sign-in error",
					"user_id", adminResp.ID, "error", deleteErr)
			}
		}
		logger.FromContext(ctx).WarnContext(ctx,
			"supabase sign-in failed after user creation",
			"user_id", adminResp.ID, "error", err)
		return nil, auth.ErrSignUpFailed
	}

	return sess, nil
}

// isDuplicateUserError reconhece as variações que o GoTrue devolve no Admin
// API quando o e-mail já existe. Inclui `email_exists` e "already been
// registered", que a checagem antiga deixava escapar para a borda como 500.
func isDuplicateUserError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"email_exists",
		"user_already_exists",
		"already registered",
		"already been registered",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func (c *AuthClient) SignInWithPassword(ctx context.Context, email, password string) (*Session, error) {
	payload := map[string]interface{}{
		"email":    email,
		"password": password,
	}

	var resp authResponse
	if err := c.doAnonJSON(ctx, http.MethodPost, "/auth/v1/token?grant_type=password", payload, &resp); err != nil {
		return nil, err
	}
	if sess := resp.toSession(); sess != nil {
		return sess, nil
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.User.ID == "" {
		return nil, fmt.Errorf("supabase login did not return a session")
	}
	return nil, fmt.Errorf("supabase login did not return a session")
}

func (c *AuthClient) RefreshToken(ctx context.Context, refreshToken string) (*Session, error) {
	payload := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	var resp authResponse
	if err := c.doAnonJSON(ctx, http.MethodPost, "/auth/v1/token?grant_type=refresh_token", payload, &resp); err != nil {
		return nil, err
	}
	if sess := resp.toSession(); sess != nil {
		return sess, nil
	}
	return nil, fmt.Errorf("supabase refresh did not return a session")
}

func (c *AuthClient) VerifyRecoveryTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	if strings.TrimSpace(tokenHash) == "" {
		return nil, errors.New("recovery token hash required")
	}
	var resp authResponse
	if err := c.doAnonJSON(ctx, http.MethodPost, "/auth/v1/verify", map[string]interface{}{
		"type":       "recovery",
		"token_hash": tokenHash,
	}, &resp); err != nil {
		return nil, err
	}
	if sess := resp.toSession(); sess != nil {
		return sess, nil
	}
	return nil, fmt.Errorf("supabase recovery verify did not return a session")
}

func (c *AuthClient) Logout(ctx context.Context, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/v1/logout", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(res.Body)
	return fmt.Errorf("supabase logout failed: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}

func (c *AuthClient) LogoutAll(ctx context.Context, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/v1/logout?scope=global", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(res.Body)
	return fmt.Errorf("supabase logout-all failed: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}

func (c *AuthClient) GenerateEmailOTP(ctx context.Context, email string) (string, error) {
	payload := map[string]interface{}{
		"type":  "magiclink",
		"email": email,
	}

	var resp map[string]interface{}
	if err := c.doServiceJSON(ctx, http.MethodPost, "/auth/v1/admin/generate_link", payload, &resp); err != nil {
		return "", err
	}

	if v, ok := resp["email_otp"].(string); ok && v != "" {
		return v, nil
	}
	if v, ok := resp["emailOtp"].(string); ok && v != "" {
		return v, nil
	}
	if data, ok := resp["data"].(map[string]interface{}); ok {
		if props, ok := data["properties"].(map[string]interface{}); ok {
			if v, ok := props["email_otp"].(string); ok && v != "" {
				return v, nil
			}
			if v, ok := props["emailOtp"].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	if props, ok := resp["properties"].(map[string]interface{}); ok {
		if v, ok := props["email_otp"].(string); ok && v != "" {
			return v, nil
		}
		if v, ok := props["emailOtp"].(string); ok && v != "" {
			return v, nil
		}
	}

	return "", fmt.Errorf("supabase generate_link did not return email_otp")
}

func (c *AuthClient) GenerateRecoveryTokenHash(ctx context.Context, email string) (string, error) {
	var resp map[string]interface{}
	if err := c.doServiceJSON(ctx, http.MethodPost, "/auth/v1/admin/generate_link", map[string]interface{}{
		"type":  "recovery",
		"email": email,
	}, &resp); err != nil {
		return "", err
	}
	hashed := recoveryHashedToken(resp)
	if hashed == "" {
		return "", fmt.Errorf("supabase generate_link did not return hashed_token")
	}
	return hashed, nil
}

func recoveryHashedToken(resp map[string]interface{}) string {
	if v, ok := resp["hashed_token"].(string); ok && v != "" {
		return v
	}
	for _, key := range []string{"properties", "data"} {
		nested, ok := resp[key].(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := nested["hashed_token"].(string); ok && v != "" {
			return v
		}
		if props, ok := nested["properties"].(map[string]interface{}); ok {
			if v, ok := props["hashed_token"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func (c *AuthClient) GenerateRecoveryLink(ctx context.Context, email string, redirectTo string) (string, error) {
	payload := map[string]interface{}{
		"type":  "recovery",
		"email": email,
	}
	if redirectTo != "" {
		// O admin generate_link do GoTrue lê `redirect_to` no TOPO do payload.
		// Dentro de `options` ele é ignorado em silêncio: o link sai apontando
		// para o site_url, e o usuário cai na raiz com a sessão de recuperação no
		// fragmento, sem tela para redefinir. (Verificado contra o serviço local.)
		payload["redirect_to"] = redirectTo
	}

	var resp map[string]interface{}
	if err := c.doServiceJSON(ctx, http.MethodPost, "/auth/v1/admin/generate_link", payload, &resp); err != nil {
		return "", err
	}

	if v, ok := resp["action_link"].(string); ok && v != "" {
		return v, nil
	}
	if v, ok := resp["actionLink"].(string); ok && v != "" {
		return v, nil
	}
	if data, ok := resp["data"].(map[string]interface{}); ok {
		if v, ok := data["action_link"].(string); ok && v != "" {
			return v, nil
		}
		if v, ok := data["actionLink"].(string); ok && v != "" {
			return v, nil
		}
		if props, ok := data["properties"].(map[string]interface{}); ok {
			if v, ok := props["action_link"].(string); ok && v != "" {
				return v, nil
			}
			if v, ok := props["actionLink"].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	if props, ok := resp["properties"].(map[string]interface{}); ok {
		if v, ok := props["action_link"].(string); ok && v != "" {
			return v, nil
		}
		if v, ok := props["actionLink"].(string); ok && v != "" {
			return v, nil
		}
	}

	return "", fmt.Errorf("supabase generate_link did not return action_link")
}

func (c *AuthClient) SendOTP(ctx context.Context, email, phone, channel string, createUser bool) error {
	payload := map[string]interface{}{
		"create_user": createUser,
	}
	if email != "" {
		payload["email"] = email
	}
	if phone != "" {
		payload["phone"] = phone
		if channel != "" {
			payload["channel"] = channel
		}
	}

	return c.doAnonJSON(ctx, http.MethodPost, "/auth/v1/otp", payload, nil)
}

func (c *AuthClient) VerifyOTP(ctx context.Context, typ, email, phone, token string) (*Session, error) {
	payload := map[string]interface{}{
		"type":  typ,
		"token": token,
	}
	if email != "" {
		payload["email"] = email
	}
	if phone != "" {
		payload["phone"] = phone
	}

	var resp authResponse
	if err := c.doAnonJSON(ctx, http.MethodPost, "/auth/v1/verify", payload, &resp); err != nil {
		return nil, err
	}
	if sess := resp.toSession(); sess != nil {
		return sess, nil
	}
	return nil, fmt.Errorf("supabase verify otp did not return a session")
}

func (c *AuthClient) UpdateAppMetadata(ctx context.Context, userID string, appMetadata map[string]interface{}) error {
	payload := map[string]interface{}{
		"app_metadata": appMetadata,
	}
	return c.doServiceJSON(ctx, http.MethodPut, "/auth/v1/admin/users/"+userID, payload, nil)
}

// UpdateUserPassword troca a senha na fonte de verdade da autenticação
// (Supabase/GoTrue), via API admin com service role.
//
// SECURITY (achado A14 da revisão de permissões de 2026-09-19): a troca de senha
// gravava em `toca_auth.credentials`, tabela que NENHUM caminho de login lê (o
// login é do Supabase) — ou seja, "senha alterada com sucesso" e a senha antiga
// continuava valendo. Este método fecha isso.
func (c *AuthClient) UpdateUserPassword(ctx context.Context, userID, password string) error {
	if strings.TrimSpace(userID) == "" || password == "" {
		return fmt.Errorf("update password: user id and password are required")
	}
	payload := map[string]interface{}{
		"password": password,
	}
	return c.doServiceJSON(ctx, http.MethodPut, "/auth/v1/admin/users/"+userID, payload, nil)
}

func (c *AuthClient) ConfirmEmail(ctx context.Context, userID string) error {
	payload := map[string]interface{}{
		"email_confirm": true,
	}
	return c.doServiceJSON(ctx, http.MethodPut, "/auth/v1/admin/users/"+userID, payload, nil)
}

// GetUser resolve o dono de um access token (GET /auth/v1/user com a anon key).
//
// Existe para o reset de senha mediado pelo backend: o par access/refresh que o
// link de recuperação carrega é validado aqui, no servidor, em vez de o
// navegador falar com o Supabase (era o último motivo para o frontend precisar
// da URL e da anon key — ver docs/deployment/hostinger.md § env.js).
func (c *AuthClient) GetUser(ctx context.Context, accessToken string) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", fmt.Errorf("get user: access token is required")
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/auth/v1/user", nil, &resp, c.anonKey, accessToken); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *AuthClient) DeleteUser(ctx context.Context, userID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/auth/v1/admin/users/"+userID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(res.Body)
	return fmt.Errorf("supabase delete user failed: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Session      *struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"session"`
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

func (r *authResponse) toSession() *Session {
	if r == nil {
		return nil
	}
	access := r.AccessToken
	refresh := r.RefreshToken
	if r.Session != nil {
		if r.Session.AccessToken != "" {
			access = r.Session.AccessToken
		}
		if r.Session.RefreshToken != "" {
			refresh = r.Session.RefreshToken
		}
	}
	if access == "" || refresh == "" || r.User.ID == "" {
		return nil
	}
	return &Session{
		AccessToken:  access,
		RefreshToken: refresh,
		UserID:       r.User.ID,
		Email:        r.User.Email,
	}
}

func (c *AuthClient) doAnonJSON(ctx context.Context, method, path string, payload interface{}, out interface{}) error {
	return c.doJSON(ctx, method, path, payload, out, c.anonKey, "")
}

func (c *AuthClient) doServiceJSON(ctx context.Context, method, path string, payload interface{}, out interface{}) error {
	return c.doJSON(ctx, method, path, payload, out, c.serviceRoleKey, c.serviceRoleKey)
}

func (c *AuthClient) doJSON(ctx context.Context, method, path string, payload interface{}, out interface{}, apiKey string, bearer string) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", apiKey)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("supabase request failed: status %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

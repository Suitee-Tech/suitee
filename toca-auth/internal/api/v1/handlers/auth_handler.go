package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/api/v1/dto"
	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/invitation"
	"github.com/oginoino/toca-auth/internal/domain/user"
	"github.com/oginoino/toca-auth/internal/service"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

type AuthHandler struct {
	authService       *service.AuthService
	regService        *service.RegistrationService
	companyService    *service.CompanyService
	invitationService *service.InvitationService
	prefsService      *service.PreferencesService
	passwordService   *service.PasswordService
	config            *app.Config
}

func NewAuthHandler(
	authService *service.AuthService,
	regService *service.RegistrationService,
	companyService *service.CompanyService,
	invitationService *service.InvitationService,
	prefsService *service.PreferencesService,
	passwordService *service.PasswordService,
	cfg *app.Config,
) *AuthHandler {
	return &AuthHandler{
		authService:       authService,
		regService:        regService,
		companyService:    companyService,
		invitationService: invitationService,
		prefsService:      prefsService,
		passwordService:   passwordService,
		config:            cfg,
	}
}

func (h *AuthHandler) RequestOTP(c *gin.Context) {
	var req dto.OTPRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	if err := h.authService.RequestOTP(c.Request.Context(), req.Contact, req.Method); err != nil {
		// M14: cota por contato estourada → 429 com Retry-After (a janela é
		// deslizante; o cliente não deve tentar de novo imediatamente).
		if errors.Is(err, service.ErrOTPRateLimited) {
			c.Header("Retry-After", strconv.Itoa(int(otpRetryAfterSeconds())))
			response.Error(c, http.StatusTooManyRequests, "Muitas solicitações para este contato. Tente novamente mais tarde.")
			return
		}
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, "OTP requested successfully", nil)
}

// otpRetryAfterSeconds espelha a janela do limitador por contato para o header
// Retry-After (default 15 min).
func otpRetryAfterSeconds() int64 {
	raw := strings.TrimSpace(os.Getenv("OTP_RATE_LIMIT_WINDOW"))
	if raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return int64(parsed.Seconds())
		}
	}
	return int64((15 * time.Minute).Seconds())
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req dto.OTPVerify
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	tokens, err := h.authService.VerifyOTP(c.Request.Context(), req.Contact, req.Code)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, err.Error())
		return
	}

	h.respondWithTokens(c, "OTP verified successfully", tokens, http.StatusOK, true)
}

func (h *AuthHandler) EnsureClientAccount(c *gin.Context) {
	var req dto.EnsureClientRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	accID, err := h.authService.EnsureClientAccount(c.Request.Context(), req.Email, req.Name, req.Phone)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, "Client account ensured", gin.H{
		"account_id": accID,
	})
}

func (h *AuthHandler) ListCompanyMembersInternal(c *gin.Context) {
	companyID := c.Param("id")
	functionalRole := c.Query("functional_role")

	members, err := h.companyService.ListCompanyMembersInternal(c.Request.Context(), companyID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if functionalRole != "" {
		filtered := make([]service.CompanyMemberDetail, 0)
		for _, m := range members {
			if m.Role == functionalRole {
				filtered = append(filtered, m)
			}
		}
		members = filtered
	}

	response.Success(c, "Members retrieved successfully", members)
}

// InvitationResponse é a projeção pública de um convite: tudo menos o `Token`.
//
// SECURITY (achado C2): ver comentário em ListInvitations.
type InvitationResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CompanyID string    `json:"company_id"`
	Role      string    `json:"role"`
	GroupIDs  []string  `json:"group_ids,omitempty"`
	Status    string    `json:"status"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func mapInvitationToDTO(inv *invitation.Invitation) *InvitationResponse {
	if inv == nil {
		return nil
	}
	return &InvitationResponse{
		ID:        inv.ID,
		Email:     inv.Email,
		CompanyID: inv.CompanyID,
		Role:      inv.Role,
		GroupIDs:  inv.GroupIDs,
		Status:    inv.Status,
		CreatedBy: inv.CreatedBy,
		CreatedAt: inv.CreatedAt,
		ExpiresAt: inv.ExpiresAt,
	}
}

func mapInvitationsToDTO(invites []*invitation.Invitation) []*InvitationResponse {
	out := make([]*InvitationResponse, 0, len(invites))
	for _, inv := range invites {
		out = append(out, mapInvitationToDTO(inv))
	}
	return out
}

const refreshTokenCookieName = "refresh_token"
const portalRefreshTokenCookieName = "portal_refresh_token"
const refreshTokenCookiePath = "/api/v1/auth"

// resolveCookieAttrs picks the right (SameSite, Secure, Domain) for the
// refresh-token cookie based on env + request, with safe defaults.
//
//   - In development we keep SameSite=Lax + Secure=false so cookies work over
//     plain http://localhost.
//   - In production we default to SameSite=None + Secure=true, which is what
//     modern browsers require to send the cookie on cross-origin XHR (the
//     frontend at https://app.example.com calling the API at
//     https://api.example.com is a cross-origin request even though they
//     share an eTLD+1).
//   - REFRESH_COOKIE_SAMESITE / REFRESH_COOKIE_DOMAIN can override the
//     defaults if the deployment topology is different (single-origin behind
//     a single host, custom CDN setup, etc.).
func (h *AuthHandler) resolveCookieAttrs(c *gin.Context) (sameSite http.SameSite, secure bool, domain string) {
	isDev := h.config.AppEnv == "development"
	httpsCtx := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	secure = !isDev && httpsCtx

	// SameSite: env override wins; otherwise None in prod / Lax in dev.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REFRESH_COOKIE_SAMESITE"))) {
	case "none":
		sameSite = http.SameSiteNoneMode
	case "lax":
		sameSite = http.SameSiteLaxMode
	case "strict":
		sameSite = http.SameSiteStrictMode
	default:
		if isDev {
			sameSite = http.SameSiteLaxMode
		} else {
			sameSite = http.SameSiteNoneMode
		}
	}

	// SameSite=None requires Secure=true per spec — browsers will silently
	// drop the cookie otherwise. In dev we don't have HTTPS so force Lax.
	if sameSite == http.SameSiteNoneMode && !secure {
		if isDev {
			sameSite = http.SameSiteLaxMode
		} else {
			// Production but the request didn't come in over HTTPS — force
			// Secure so the cookie at least exists; the client will retry
			// over HTTPS in front of the gateway.
			secure = true
		}
	}

	domain = strings.TrimSpace(os.Getenv("REFRESH_COOKIE_DOMAIN"))
	return
}

// setRefreshTokenCookie sets the refresh token as an HTTP-only secure cookie.
func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, refreshToken string, isPortal bool) {
	maxAge := int(h.config.JWT.RefreshTokenExpiration.Seconds())
	sameSite, secure, domain := h.resolveCookieAttrs(c)
	c.SetSameSite(sameSite)

	cookieName := refreshTokenCookieName
	if isPortal {
		cookieName = portalRefreshTokenCookieName
	}

	c.SetCookie(cookieName, refreshToken, maxAge, refreshTokenCookiePath, domain, secure, true)
}

// clearRefreshTokenCookie removes the refresh token cookie. Must mirror the
// SameSite/Secure/Domain attributes used to set it — otherwise browsers won't
// match the cookie to be deleted.
func (h *AuthHandler) clearRefreshTokenCookie(c *gin.Context, isPortal bool) {
	sameSite, secure, domain := h.resolveCookieAttrs(c)
	c.SetSameSite(sameSite)

	cookieName := refreshTokenCookieName
	if isPortal {
		cookieName = portalRefreshTokenCookieName
	}

	c.SetCookie(cookieName, "", -1, refreshTokenCookiePath, domain, secure, true)
}

// getRefreshToken reads the refresh token from cookie (priority) or request body (legacy fallback).
func getRefreshToken(c *gin.Context, isPortal bool) (string, bool) {
	cookieName := refreshTokenCookieName
	if isPortal {
		cookieName = portalRefreshTokenCookieName
	}

	// Try cookie first
	if token, err := c.Cookie(cookieName); err == nil && token != "" {
		return token, true
	}
	// Fallback: JSON body (legacy clients)
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !validation.BindJSONOptional(c, &body) {
		return "", false
	}
	return body.RefreshToken, true
}

// respondWithTokens sends the access token in the JSON body and refresh token as HTTP-only cookie.
// For legacy clients (X-Auth-Version: legacy), also includes refresh_token in the body.
func (h *AuthHandler) respondWithTokens(c *gin.Context, msg string, tokens *auth.TokenPair, statusCode int, isPortal ...bool) {
	portal := false
	if len(isPortal) > 0 {
		portal = isPortal[0]
	}

	h.setRefreshTokenCookie(c, tokens.RefreshToken, portal)

	body := gin.H{"access_token": tokens.AccessToken}
	if c.GetHeader("X-Auth-Version") == "legacy" {
		body["refresh_token"] = tokens.RefreshToken
	}

	if statusCode == http.StatusCreated {
		response.Created(c, msg, body)
	} else {
		response.Success(c, msg, body)
	}
}

// Register godoc
// @Summary      Register a new user
// @Description  Register a new user with the provided details
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body auth.RegisterRequest true "Register Request"
// @Success      201  {object}  response.Response{data=auth.TokenPair}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req auth.RegisterRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	tokens, err := h.regService.Register(c.Request.Context(), &req)
	if err != nil {
		errMsg := err.Error()

		// Conflito de cadastro: 409 com texto genérico. Não confirmamos qual
		// dado colidiu nem detalhes da conta — e-mail e telefone respondem
		// igual, para não transformar o cadastro em oráculo de enumeração.
		if isDuplicateRegistrationError(err) {
			response.Error(c, http.StatusConflict, registrationConflictMessage)
			return
		}
		if errMsg == "password must be at least 8 characters" ||
			errMsg == "password must contain at least one uppercase letter" ||
			errMsg == "password must contain at least one lowercase letter" ||
			errMsg == "password must contain at least one digit" ||
			errMsg == "password must contain at least one special character" {
			response.ValidationError(c, errMsg)
			return
		}
		logger.FromContext(c.Request.Context()).Error("register failed", "error", err)
		msg := "Ocorreu um erro interno."
		if h.config.AppEnv == "development" && c.GetHeader("X-Debug") == "true" {
			msg = errMsg
		}
		response.InternalServerError(c, msg)
		return
	}

	h.respondWithTokens(c, "User registered successfully", tokens, http.StatusCreated)
}

const registrationConflictMessage = "Não foi possível concluir o cadastro com os dados informados."

// isDuplicateRegistrationError centraliza o reconhecimento de conflito de
// cadastro. Os sentinelas cobrem o serviço e o cliente Supabase; as duas
// strings mantêm compatibilidade com repositórios que ainda devolvem erro
// textual de unicidade.
func isDuplicateRegistrationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, auth.ErrEmailAlreadyExists) || errors.Is(err, auth.ErrPhoneAlreadyExists) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"email already exists",
		"phone already exists",
		"user_already_exists",
		"email_exists",
		"already registered",
		"already been registered",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Login godoc
// @Summary      Login user
// @Description  Login with email and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body auth.LoginRequest true "Login Request"
// @Success      200  {object}  response.Response{data=auth.TokenPair}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req auth.LoginRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	tokens, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		logger.FromContext(c.Request.Context()).Warn("login failed", "error", err)
		// P2/M7: a conta desativada é recusada DEPOIS de a senha conferir no
		// Supabase, então responder 403 explícito não revela existência de conta
		// para quem não tem a credencial — e evita o "senha inválida" eterno de
		// quem foi bloqueado.
		if errors.Is(err, service.ErrAccountInactive) {
			response.Error(c, http.StatusForbidden, "Conta desativada: fale com o administrador da empresa")
			return
		}
		response.Unauthorized(c, "Credenciais inválidas")
		return
	}

	h.respondWithTokens(c, "Login successful", tokens, http.StatusOK)
}

func (h *AuthHandler) OAuthLogin(c *gin.Context) {
	var req auth.OAuthLoginRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	tokens, err := h.authService.OAuthLogin(c.Request.Context(), &req)
	if err != nil {
		logger.FromContext(c.Request.Context()).Warn("oauth login failed", "error", err)
		if errors.Is(err, service.ErrAccountInactive) {
			response.Error(c, http.StatusForbidden, "Conta desativada: fale com o administrador da empresa")
			return
		}
		response.Unauthorized(c, "Falha ao autenticar")
		return
	}

	h.respondWithTokens(c, "Login successful", tokens, http.StatusOK)
}

// RefreshToken godoc
// @Summary      Refresh access token
// @Description  Get a new access token using a refresh token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body auth.RefreshTokenRequest true "Refresh Token Request"
// @Success      200  {object}  response.Response{data=auth.TokenPair}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	refreshToken, ok := getRefreshToken(c, false)
	if !ok {
		return
	}
	if refreshToken == "" {
		response.ValidationError(c, "refresh token is required")
		return
	}

	tokens, err := h.authService.RefreshToken(c.Request.Context(), refreshToken, false)
	if err != nil {
		h.clearRefreshTokenCookie(c, false)
		response.Unauthorized(c, err.Error())
		return
	}

	h.respondWithTokens(c, "Token refreshed successfully", tokens, http.StatusOK, false)
}

// PortalRefreshToken refreshes the token for portal users using portal_refresh_token
func (h *AuthHandler) PortalRefreshToken(c *gin.Context) {
	refreshToken, ok := getRefreshToken(c, true)
	if !ok {
		return
	}
	if refreshToken == "" {
		response.ValidationError(c, "refresh token is required")
		return
	}

	tokens, err := h.authService.RefreshToken(c.Request.Context(), refreshToken, true)
	if err != nil {
		h.clearRefreshTokenCookie(c, true)
		response.Unauthorized(c, err.Error())
		return
	}

	h.respondWithTokens(c, "Token refreshed successfully", tokens, http.StatusOK, true)
}

// Me godoc
// @Summary      Get current user info
// @Description  Get information about the currently logged in user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=map[string]interface{}}
// @Failure      401  {object}  response.Response
// @Router       /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.Unauthorized(c, "user_id missing from token")
		return
	}

	userAcc, err := h.authService.GetUserInfo(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if userAcc == nil {
		response.NotFound(c, "account not found")
		return
	}

	// Surface functional_role + account_type from JWT claims so the frontend
	// can distinguish portal clients (functional_role = "cliente") from
	// company users (corretor, admin, staff, etc.) — critical for keeping
	// admin and portal auth contexts isolated.
	functionalRole, _ := c.Get("functional_role")
	accountType, _ := c.Get("type")

	body := gin.H{
		"id":              userAcc.ID,
		"functional_role": functionalRole,
		"account_type":    accountType,
	}

	// Individual accounts (including portal clients) have a user profile; company
	// accounts may not. Fall back to empty strings so the frontend always gets a
	// consistent shape instead of crashing on nil.
	if res := dto.MapUserToDTO(userAcc.Individual); res != nil {
		body["firstname"] = res.FirstName
		body["lastname"] = res.LastName
		body["fullname"] = res.FullName
		body["email"] = res.Email
		body["role"] = res.Role
		body["is_active"] = res.IsActive
		body["phone"] = res.Phone
		body["bio"] = res.Bio
		body["avatar_url"] = res.AvatarURL
		body["address"] = res.Address
		body["notifications"] = res.Notifications
		// Banking opcional (#463). Nil quando o usuário nunca preencheu.
		body["banking"] = res.Banking
	} else {
		body["firstname"] = ""
		body["lastname"] = ""
		body["fullname"] = ""
		body["email"] = ""
		body["role"] = ""
		body["is_active"] = false
		body["phone"] = ""
		body["bio"] = ""
		body["avatar_url"] = ""
		body["address"] = nil
		body["notifications"] = nil
		body["banking"] = nil
	}

	response.Success(c, "User info retrieved successfully", body)
}

type UpdateMeRequest struct {
	FirstName     string              `json:"firstname"`
	LastName      string              `json:"lastname"`
	Phone         string              `json:"phone"`
	Address       *user.Address       `json:"address,omitempty"`
	Notifications *user.Notifications `json:"notifications,omitempty"`
	// Banking opcional (#463). Quando presente, AuthService valida via
	// BankingInfo.Validate antes de salvar — 400 com payload tipado se
	// inválido. Quando ausente (nil), preserva o que estiver no banco.
	// Para LIMPAR banking, mandar objeto vazio explícito {}.
	Banking *user.BankingInfo `json:"banking,omitempty"`
}

func (h *AuthHandler) UpdateMe(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.Unauthorized(c, "user_id missing from token")
		return
	}

	var req UpdateMeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// We only allow updating certain fields to prevent privilege escalation
	updates := map[string]interface{}{}
	if req.FirstName != "" {
		updates["individual.firstname"] = req.FirstName
	}
	if req.LastName != "" {
		updates["individual.lastname"] = req.LastName
	}
	if req.Phone != "" {
		updates["individual.phone"] = req.Phone
	}
	if req.Address != nil {
		updates["individual.address"] = req.Address
	}
	if req.Notifications != nil {
		updates["individual.notifications"] = req.Notifications
	}
	if req.Banking != nil {
		// Pre-valida no handler para devolver 400 com payload
		// estruturado em vez de 500 vindo do service. BankingValidationError
		// expõe Field+Reason para o frontend mostrar mensagem específica.
		if err := req.Banking.Validate(); err != nil {
			var bvErr *user.BankingValidationError
			if errors.As(err, &bvErr) {
				response.ValidationError(c, bvErr.Reason)
				return
			}
			response.ValidationError(c, err.Error())
			return
		}
		updates["individual.banking"] = req.Banking
	}

	if len(updates) == 0 {
		response.Success(c, "No changes made", nil)
		return
	}

	err := h.authService.UpdateUserInfo(c.Request.Context(), userID, updates)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "User info updated successfully", nil)
}

// CreateCompany godoc
// @Summary      Create a new company
// @Description  Create a new company for the user
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body auth.CreateCompanyRequest true "Create Company Request"
// @Success      201  {object}  response.Response{data=dto.AccountResponse}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies [post]
func (h *AuthHandler) CreateCompany(c *gin.Context) {
	var req auth.CreateCompanyRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	account, err := h.companyService.CreateCompany(c.Request.Context(), userID.(string), &req)
	if err != nil {
		if err.Error() == "company with this tax_id already exists" {
			response.Conflict(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Created(c, "Company created successfully", dto.MapAccountToDTO(account))
}

// GetCompany godoc
// @Summary      Get company details
// @Description  Get details of a specific company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Success      200  {object}  response.Response{data=dto.AccountResponse}
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id} [get]
func (h *AuthHandler) GetCompany(c *gin.Context) {
	companyID := c.Param("id")

	// SECURITY (achado C1): o vínculo do chamador com a empresa é verificado no
	// serviço. Antes, qualquer token lia os dados cadastrais e a lista de membros
	// de qualquer empresa pelo UUID.
	requestorID, _ := c.Get("user_id")
	requestor, _ := requestorID.(string)

	compAcc, err := h.companyService.GetCompany(c.Request.Context(), requestor, companyID)
	if err != nil {
		if err.Error() == "company not found" {
			response.NotFound(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Company retrieved successfully", dto.MapAccountToDTO(compAcc))
}

// UpdateCompany godoc
// @Summary      Update company details
// @Description  Update details of a specific company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        request body dto.UpdateCompanyRequest true "Update Company Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id} [put]
func (h *AuthHandler) UpdateCompany(c *gin.Context) {
	companyID := c.Param("id")

	var req dto.UpdateCompanyRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	comp := company.Company{
		Name:      req.Name,
		LegalName: req.LegalName,
		TaxID:     req.TaxID,
		Email:     req.Email,
		Phone:     req.Phone,
		Type:      req.Type,
		Address: company.Address{
			Street:     req.Address.Street,
			Number:     req.Address.Number,
			Complement: req.Address.Complement,
			District:   req.Address.District,
			City:       req.Address.City,
			State:      req.Address.State,
			ZipCode:    req.Address.ZipCode,
		},
	}

	// SECURITY (achado C1): só proprietário/admin da própria empresa altera o
	// cadastro. Antes o corpo era aplicado em qualquer empresa pelo id do path.
	requestorID, _ := c.Get("user_id")
	requestor, _ := requestorID.(string)

	if err := h.companyService.UpdateCompanyAccount(c.Request.Context(), requestor, companyID, &comp); err != nil {
		if err.Error() == "company account not found" {
			response.NotFound(c, "Empresa não encontrada.")
			return
		}
		if err.Error() == "unauthorized" {
			response.Forbidden(c, "Você não tem permissão para alterar esta empresa.")
			return
		}
		logger.FromContext(c.Request.Context()).Error("update company failed", "error", err, "company_id", companyID)
		response.InternalServerError(c, "Não conseguimos atualizar a empresa agora.")
		return
	}

	response.Success(c, "Company updated successfully", nil)
}

// AddUserToCompany godoc
// @Summary      Add user to company
// @Description  Add an existing user to a company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        request body auth.AddUserRequest true "Add User Request"
// @Success      201  {object}  response.Response{data=dto.AccountResponse}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/users [post]
func (h *AuthHandler) AddUserToCompany(c *gin.Context) {
	var req auth.AddUserRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	userAcc, err := h.companyService.AddUserToCompany(c.Request.Context(), companyID, userID.(string), &req)
	if err != nil {
		switch err.Error() {
		case "unauthorized":
			response.Unauthorized(c, err.Error())
		case "company not found", "user not found", "user is already a member":
			response.Error(c, http.StatusBadRequest, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Created(c, "User added to company successfully", dto.MapAccountToDTO(userAcc))
}

// ListCompanies godoc
// @Summary      List user companies
// @Description  List all companies the user belongs to
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]dto.AccountResponse}
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /auth/companies [get]
func (h *AuthHandler) ListCompanies(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	companies, err := h.companyService.GetUserCompanies(c.Request.Context(), userID.(string))
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Companies retrieved successfully", dto.MapAccountsToDTO(companies))
}

// SwitchContext godoc
// @Summary      Switch company context
// @Description  Switch the active company context and get new tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body object{company_id=string} true "Switch Context Request"
// @Success      200  {object}  response.Response{data=auth.TokenPair}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /auth/switch-context [post]
func (h *AuthHandler) SwitchContext(c *gin.Context) {
	var req struct {
		CompanyID    string `json:"company_id" binding:"required"`
		RefreshToken string `json:"refresh_token"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	refreshToken, _ := c.Cookie(refreshTokenCookieName)
	if refreshToken == "" {
		refreshToken = req.RefreshToken
	}
	if refreshToken == "" {
		response.Unauthorized(c, "refresh token is required")
		return
	}

	tokens, err := h.authService.SwitchContext(c.Request.Context(), userID.(string), req.CompanyID, refreshToken)
	if err != nil {
		// P2/M7: vínculo suspenso é 403 com motivo — o chamador já está
		// autenticado, então não há o que esconder, e a UI precisa distinguir
		// "suspenso" de "erro".
		if errors.Is(err, service.ErrMembershipInactive) {
			response.Error(c, http.StatusForbidden, "Vínculo suspenso nesta empresa")
			return
		}
		response.Unauthorized(c, err.Error())
		return
	}

	h.respondWithTokens(c, "Context switched successfully", tokens, http.StatusOK)
}

// CreateInvitation godoc
// @Summary      Create invitation
// @Description  Invite a user to join a company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        request body invitation.CreateInvitationRequest true "Invitation Request"
// @Success      201  {object}  response.Response{data=invitation.Invitation}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/invite [post]
func (h *AuthHandler) CreateInvitation(c *gin.Context) {
	var req invitation.CreateInvitationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	invite, err := h.invitationService.CreateInvitation(c.Request.Context(), userID.(string), companyID, &req)
	if err != nil {
		switch err.Error() {
		case "company not found", "user is already a member", "invitation already pending":
			response.Error(c, http.StatusBadRequest, err.Error())
		case "unauthorized":
			response.Unauthorized(c, err.Error())
		case "invitation limit exceeded: maximum 20 invitations per day per company":
			response.Error(c, http.StatusTooManyRequests, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Created(c, "Invitation created successfully", invite)
}

// ListCompanyMembers godoc
// @Summary      List company members
// @Description  List all active members with details for a company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/members [get]
func (h *AuthHandler) ListCompanyMembers(c *gin.Context) {
	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	members, err := h.companyService.ListCompanyMembers(c.Request.Context(), userID.(string), companyID)
	if err != nil {
		if err.Error() == "company not found" || err.Error() == "unauthorized" {
			response.Error(c, http.StatusBadRequest, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Members retrieved successfully", members)
}

// RemoveMemberFromCompany godoc
// @Summary      Remove member
// @Description  Remove a member from the company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        member_id path string true "Member ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/members/{member_id} [delete]
func (h *AuthHandler) RemoveMemberFromCompany(c *gin.Context) {
	companyID := c.Param("id")
	memberID := c.Param("member_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.companyService.RemoveMemberFromCompany(c.Request.Context(), userID.(string), companyID, memberID)
	if err != nil {
		respondCompanyMemberError(c, err)
		return
	}

	response.Success(c, "Member removed successfully", nil)
}

// UpdateCompanyMember godoc
// @Summary      Update company member
// @Description  Change the functional role/access profile of a member or suspend/reactivate their membership
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        member_id path string true "Member ID"
// @Param        request body auth.UpdateMemberRequest true "Fields to change (all optional)"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/members/{member_id} [patch]
//
// P2/M6+M7 — endpoint que faltava: sem ele o papel de um membro existente só
// mudava por efeito colateral (grupo/convite) e não havia como suspender alguém
// sem removê-lo da empresa.
func (h *AuthHandler) UpdateCompanyMember(c *gin.Context) {
	companyID := c.Param("id")
	memberID := c.Param("member_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req auth.UpdateMemberRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.companyService.UpdateCompanyMember(c.Request.Context(), userID.(string), companyID, memberID, &req)
	if err != nil {
		respondCompanyMemberError(c, err)
		return
	}

	response.Success(c, "Member updated successfully", member)
}

// respondCompanyMemberError centraliza o mapeamento dos erros de vínculo
// (P2/M4): negação de autoridade é 403 com trilha de autorização — antes virava
// 400, indistinguível de corpo inválido —, alvo inexistente é 404 e regra de
// negócio é 400.
func respondCompanyMemberError(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case msg == "unauthorized":
		pkgAuth.DenyGinRequest(c, "unauthorized", "company:manage_members")
	case strings.Contains(msg, "not found"):
		response.NotFound(c, msg)
	case strings.Contains(msg, "invalid"),
		strings.Contains(msg, "cannot"),
		strings.Contains(msg, "only the company owner"):
		response.ValidationError(c, msg)
	default:
		// M17: erro não classificado é interno — o detalhe vai para o log, não
		// para o cliente.
		response.InternalError(c, err)
	}
}

// ListInvitations godoc
// @Summary      List invitations
// @Description  List all pending/accepted/canceled invitations for a company
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/invitations [get]
func (h *AuthHandler) ListInvitations(c *gin.Context) {
	companyID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	invites, err := h.invitationService.ListInvitations(c.Request.Context(), userID.(string), companyID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Invitations retrieved successfully", []*invitation.Invitation{})
			return
		}
		if err.Error() == "company not found" || err.Error() == "unauthorized" {
			response.Error(c, http.StatusBadRequest, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	// SECURITY (achado C2): o `Token` do convite NUNCA é serializado. Ele é um
	// bearer de uso único que cria conta para o e-mail convidado e, com papel
	// admin, um principal root no tenant — expô-lo a qualquer membro permitia
	// escalada. O reenvio usa o id do convite, não o token.
	response.Success(c, "Invitations retrieved successfully", mapInvitationsToDTO(invites))
}

// ResendInvitation godoc
// @Summary      Resend invitation
// @Description  Resend an invitation email
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        invitation_id path string true "Invitation ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/invitations/{invitation_id}/resend [post]
func (h *AuthHandler) ResendInvitation(c *gin.Context) {
	companyID := c.Param("id")
	invitationID := c.Param("invitation_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.invitationService.ResendInvitation(c.Request.Context(), userID.(string), companyID, invitationID)
	if err != nil {
		if err.Error() == "company not found" || err.Error() == "unauthorized" || err.Error() == "invitation not found" || err.Error() == "invitation already accepted" {
			response.Error(c, http.StatusBadRequest, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Invitation resent successfully", nil)
}

// CancelInvitation godoc
// @Summary      Cancel invitation
// @Description  Cancel a pending invitation
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Company ID"
// @Param        invitation_id path string true "Invitation ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/{id}/invitations/{invitation_id}/cancel [post]
func (h *AuthHandler) CancelInvitation(c *gin.Context) {
	companyID := c.Param("id")
	invitationID := c.Param("invitation_id")
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.invitationService.CancelInvitation(c.Request.Context(), userID.(string), companyID, invitationID)
	if err != nil {
		if err.Error() == "company not found" || err.Error() == "unauthorized" || err.Error() == "invitation not found" || err.Error() == "cannot cancel an accepted invitation" {
			response.Error(c, http.StatusBadRequest, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Invitation canceled successfully", nil)
}

// AcceptInvitation godoc
// @Summary      Accept invitation
// @Description  Accept an invitation to join a company
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body invitation.AcceptInvitationRequest true "Accept Invitation Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /auth/accept-invite [post]
func (h *AuthHandler) AcceptInvitation(c *gin.Context) {
	var req invitation.AcceptInvitationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
	}

	err := h.invitationService.AcceptInvitation(c.Request.Context(), userID.(string), &req)
	if err != nil {
		if err.Error() == "invalid invitation token" || err.Error() == "invitation is not pending" || err.Error() == "invitation expired" || err.Error() == "already a member" || err.Error() == "email mismatch" {
			response.Error(c, http.StatusBadRequest, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Invitation accepted successfully", nil)
}

// P1/A14 — `GET /auth/reset-password/verify` foi REMOVIDO com o fluxo local:
// `passwordResetRepo.Create` nunca era chamado, então este endpoint só sabia
// responder "token inválido ou expirado" para qualquer entrada.

// GetInvitation fetches public invite info
func (h *AuthHandler) GetInvitation(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.ValidationError(c, "token is required")
		return
	}
	invite, isRegistered, err := h.invitationService.GetInvitation(c.Request.Context(), token)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	// Return safe data
	res := map[string]interface{}{
		"email":         invite.Email,
		"company_id":    invite.CompanyID,
		"role":          invite.Role,
		"status":        invite.Status,
		"expires_at":    invite.ExpiresAt,
		"is_registered": isRegistered,
	}
	response.Success(c, "Invitation found", res)
}

// AcceptPublicInvitation accepts an invitation from public link and registers if needed
func (h *AuthHandler) AcceptPublicInvitation(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.ValidationError(c, "token is required")
		return
	}
	var req auth.RegisterRequest
	if !validation.BindJSONOptional(c, &req) {
		return
	}

	pair, err := h.invitationService.AcceptPublicInvitation(c.Request.Context(), token, &req)
	if err != nil {
		// Mesmo conflito genérico do cadastro: não devolver a mensagem interna
		// do serviço/provedor, que poderia confirmar a existência da conta.
		if isDuplicateRegistrationError(err) {
			response.Error(c, http.StatusConflict, registrationConflictMessage)
			return
		}
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	h.respondWithTokens(c, "Invitation accepted", pair, http.StatusOK)
}

// ForgotPassword godoc
// @Summary      Request password reset
// @Description  Request a password reset email
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body auth.ForgotPasswordRequest true "Forgot Password Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req auth.ForgotPasswordRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Always return success to avoid user enumeration; log internally on failure.
	if err := h.passwordService.ForgotPassword(c.Request.Context(), &req); err != nil {
		logger.FromContext(c.Request.Context()).Warn("ForgotPassword failed (silent to client to avoid enumeration)", "error", err)
	}

	response.Success(c, "If the email exists, a reset link has been sent", nil)
}

// P1/A14 — o `POST /auth/reset-password` do fluxo local de token foi REMOVIDO
// (nunca funcionava: o token não era criado). A rota voltou com outro contrato:
// recebe a SESSÃO DE RECUPERAÇÃO que o link do Supabase entrega no fragmento da
// URL e aplica a senha no servidor. Antes quem trocava a senha era o browser
// (com a anon key do Supabase em /env.js); agora o navegador não fala com o
// provedor.
//
// `POST /auth/forgot-password` continua sendo quem pede o link.

// ResetPassword godoc
// @Summary      Reset password with a recovery session
// @Description  Applies a new password using the access/refresh pair from the Supabase recovery link fragment
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      auth.ResetPasswordWithSessionRequest  true  "Recovery session and new password"
// @Success      200   {object}  response.Response
// @Failure      400   {object}  response.Response
// @Failure      401   {object}  response.Response
// @Router       /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req auth.ResetPasswordWithSessionRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if validation.RespondIfValidationError(c, req.Validate()) {
		return
	}

	var err error
	if strings.TrimSpace(req.TokenHash) != "" {
		err = h.passwordService.ResetPasswordWithRecoveryTokenHash(c.Request.Context(), req.TokenHash, req.ExpiresAt, req.Signature, req.NewPassword)
	} else {
		err = h.passwordService.ResetPasswordWithRecovery(c.Request.Context(), req.AccessToken, req.RefreshToken, req.NewPassword)
	}
	if err != nil {
		if errors.Is(err, service.ErrRecoverySessionInvalid) {
			response.Error(c, http.StatusUnauthorized, "Link de redefinição inválido ou expirado. Solicite um novo.")
			return
		}
		// Erro de complexidade de senha vem do domínio com mensagem própria.
		if msg := err.Error(); strings.Contains(strings.ToLower(msg), "senha") || strings.Contains(strings.ToLower(msg), "password") {
			response.Error(c, http.StatusBadRequest, msg)
			return
		}
		logger.FromContext(c.Request.Context()).Error("ResetPassword failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "Não foi possível redefinir a senha. Tente novamente.")
		return
	}

	response.Success(c, "Senha redefinida com sucesso", nil)
}

// Logout godoc
// @Summary      Logout current session
// @Description  Invalidates the current session and clears the refresh token cookie
// @Tags         auth
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	functionalRole, _ := c.Get("functional_role")
	roleStr, _ := functionalRole.(string)

	accessToken, _ := c.Get("auth_token")
	tokenStr, _ := accessToken.(string)
	if tokenStr != "" {
		_ = h.authService.Logout(c.Request.Context(), tokenStr)
	}

	if roleStr == "cliente" {
		h.clearRefreshTokenCookie(c, true)
	} else {
		h.clearRefreshTokenCookie(c, false)
	}

	response.Success(c, "Logged out successfully", nil)
}

// LogoutAll godoc
// @Summary      Logout all sessions
// @Description  Invalidates all sessions for the current user across all devices
// @Tags         auth
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /auth/logout-all [post]
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	functionalRole, _ := c.Get("functional_role")
	roleStr, _ := functionalRole.(string)
	accessToken, _ := c.Get("auth_token")
	tokenStr, _ := accessToken.(string)
	if tokenStr != "" {
		_ = h.authService.LogoutAll(c.Request.Context(), tokenStr)
	}

	if roleStr == "cliente" {
		h.clearRefreshTokenCookie(c, true)
	} else {
		h.clearRefreshTokenCookie(c, false)
	}

	response.Success(c, "All sessions logged out successfully", nil)
}

// CompanyLookupResponse is the minimal, non-sensitive projection returned by
// the CNPJ lookup endpoint. It deliberately omits Members and OwnerID so a
// caller can confirm a company exists (e.g. to send a partnership invite)
// without leaking who belongs to it.
type CompanyLookupResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LegalName string `json:"legal_name"`
	TaxID     string `json:"tax_id"`
	Type      string `json:"type"`
	IsActive  bool   `json:"is_active"`
	City      string `json:"city"`
	State     string `json:"state"`
}

// LookupCompanyByTaxID godoc
// @Summary      Lookup company by CNPJ
// @Description  Find a company by its CNPJ (tax_id). Returns a minimal, non-sensitive projection so a company can be identified before sending a partnership/franchise invite. The tax_id query param accepts masked or unmasked CNPJ.
// @Tags         companies
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        tax_id query string true "Company CNPJ (masked or digits-only)"
// @Success      200  {object}  response.Response{data=handlers.CompanyLookupResponse}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /companies/lookup [get]
func (h *AuthHandler) LookupCompanyByTaxID(c *gin.Context) {
	taxID := strings.TrimSpace(c.Query("tax_id"))
	if taxID == "" {
		response.ValidationError(c, "tax_id query parameter is required")
		return
	}

	compAcc, err := h.companyService.LookupCompanyByTaxID(c.Request.Context(), taxID)
	if err != nil {
		switch err.Error() {
		case "company not found":
			response.NotFound(c, "no company found for the provided tax_id")
		case "tax_id is required":
			response.ValidationError(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	comp := compAcc.Company
	resp := CompanyLookupResponse{
		ID:        compAcc.ID,
		Name:      comp.Name,
		LegalName: comp.LegalName,
		TaxID:     comp.TaxID,
		Type:      comp.Type,
		IsActive:  comp.IsActive,
		City:      comp.Address.City,
		State:     comp.Address.State,
	}

	response.Success(c, "Company retrieved successfully", resp)
}

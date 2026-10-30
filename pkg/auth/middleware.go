package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// Token type constants
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type AppMetadata struct {
	// AccountID é a IDENTIDADE (conta local do produto). P2/M9: antes o
	// `tenant_id` fazia as vezes de identidade para `cliente` (o convite de
	// cliente guardava a conta local ali), então `/me`, `/lgpd/me/account` e a
	// checagem de vínculo operavam sobre o id da EMPRESA. Agora a identidade tem
	// claim própria e `tenant_id` volta a significar só tenant.
	AccountID        string   `json:"account_id,omitempty"`
	TenantID         string   `json:"tenant_id,omitempty"`
	PartnerTenantIDs []string `json:"partner_tenant_ids,omitempty"`
	Permission       string   `json:"permission,omitempty"`
	FunctionalRole   string   `json:"functional_role,omitempty"`
	// BaseRole é o papel do catálogo de onde um PERFIL DE ACESSO (`cr_…`) partiu.
	// Vem no app_metadata junto do `functional_role` e é copiado para o claim
	// equivalente — sem esta cópia o token chega com o campo e o serviço não o vê
	// (foi o que manteve o perfil personalizado recortado na primeira tentativa).
	BaseRole string `json:"base_role,omitempty"`
	Type     string `json:"type,omitempty"`
}

// Claims defines the JWT claims structure
type Claims struct {
	jwt.RegisteredClaims
	AccountID      string `json:"account_id"` // Legacy
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	Role           string `json:"role"`
	Permission     string `json:"permission"`
	FunctionalRole string `json:"functional_role"` // NOVO (do CompanyMember)
	// BaseRole é o papel do catálogo de onde um PERFIL DE ACESSO (`cr_…`) partiu.
	// Vem no token (emitido por toca-auth a partir de core.role_permissions).
	BaseRole         string       `json:"base_role,omitempty"`
	Type             string       `json:"type"`                         // individual or company
	TenantID         string       `json:"tenant_id,omitempty"`          // Legacy
	PartnerTenantIDs []string     `json:"partner_tenant_ids,omitempty"` // Legacy
	TokenType        string       `json:"token_type,omitempty"`         // "access" or "refresh"
	SessionID        string       `json:"session_id,omitempty"`
	FamilyID         string       `json:"family_id,omitempty"`
	AppMetadata      *AppMetadata `json:"app_metadata,omitempty"` // Supabase custom claims
}

// Config defines the configuration for the auth middleware
type Config struct {
	SupabaseJWKSURL string // NOVO: URL do JWKS do Supabase
}

var (
	jwks *keyfunc.JWKS
)

// InitJWKS initializes the JWKS fetcher (should be called on app startup)
func InitJWKS(jwksURL string) error {
	if jwksURL == "" {
		logger.Warn("JWKS URL is empty, Supabase JWT validation will fail", nil)
		return nil
	}

	options := keyfunc.Options{
		RefreshInterval: time.Hour, // Refresh the JWKS every hour
		RefreshErrorHandler: func(err error) {
			logger.Error("Failed to refresh JWKS", map[string]interface{}{"error": err.Error()})
		},
	}

	var err error
	jwks, err = keyfunc.Get(jwksURL, options)
	if err != nil {
		return fmt.Errorf("failed to get JWKS from %s: %w", jwksURL, err)
	}

	logger.Info("JWKS initialized successfully", map[string]interface{}{"url": jwksURL})
	return nil
}

// Middleware creates a new auth middleware
func Middleware(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Whitelist check
		path := c.Request.URL.Path
		if isPublicEndpoint(path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		tokenString := ""

		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		} else {
			// Preferred WebSocket auth path: Sec-WebSocket-Protocol header
			// carrying "Bearer.<token>" (dot is the canonical separator since
			// the protocol value must be a single token per RFC 6455). Avoids
			// the token landing in URL access logs of Nginx/Traefik.
			if proto := c.GetHeader("Sec-WebSocket-Protocol"); proto != "" {
				for _, candidate := range strings.Split(proto, ",") {
					candidate = strings.TrimSpace(candidate)
					if strings.HasPrefix(candidate, "Bearer.") {
						tokenString = strings.TrimPrefix(candidate, "Bearer.")
						// Echo the selected subprotocol so the upgrade
						// handler can reply with it — gorilla/websocket
						// will refuse the upgrade if the client offered a
						// subprotocol but the server didn't acknowledge one.
						c.Set("ws_subprotocol", candidate)
						break
					}
				}
			}

			// Legacy fallback: token in the URL query. Kept for backwards
			// compatibility with existing clients but logged so we can track
			// migration progress and eventually remove it.
			if tokenString == "" {
				tokenString = c.Query("token")
				if tokenString != "" {
					logger.Warn("WebSocket auth via ?token= is deprecated, use Sec-WebSocket-Protocol: Bearer.<jwt>", map[string]interface{}{
						"path":      path,
						"client_ip": c.ClientIP(),
					})
				}
			}
		}

		if tokenString == "" {
			logger.Warn("Unauthorized access attempt: missing token", map[string]interface{}{
				"ip":     c.ClientIP(),
				"path":   path,
				"method": c.Request.Method,
			})
			response.Unauthorized(c, "authorization token is required")
			c.Abort()
			return
		}

		c.Set("auth_token", tokenString)

		// Determine validation strategy
		var token *jwt.Token
		var err error

		if jwks == nil {
			response.Unauthorized(c, "auth jwks not initialized")
			c.Abort()
			return
		}

		token, err = jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if t.Method == nil || (t.Method.Alg() != jwt.SigningMethodRS256.Alg() && t.Method.Alg() != jwt.SigningMethodES256.Alg()) {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return jwks.Keyfunc(t)
		})

		if err != nil {
			logger.Warn("Unauthorized access attempt: invalid token", map[string]interface{}{
				"ip":     c.ClientIP(),
				"path":   path,
				"method": c.Request.Method,
				"error":  err.Error(),
			})
			response.Unauthorized(c, "invalid token: "+err.Error())
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*Claims); ok && token.Valid {
			if claims.TokenType == TokenTypeRefresh {
				logger.Warn("Refresh token used as access token", map[string]interface{}{
					"ip":         c.ClientIP(),
					"path":       path,
					"account_id": claims.AccountID,
				})
				response.Unauthorized(c, "refresh tokens cannot be used for API access")
				c.Abort()
				return
			}

			if claims.TokenType != "" && claims.TokenType != TokenTypeAccess {
				response.Unauthorized(c, "invalid token type")
				c.Abort()
				return
			}

			// Check expiration for refresh warning (e.g., if < 5 minutes remaining)
			if claims.ExpiresAt != nil {
				timeRemaining := time.Until(claims.ExpiresAt.Time)
				if timeRemaining < 5*time.Minute && timeRemaining > 0 {
					c.Header("X-Token-Expiring-Soon", "true")
				}
			}

			// Extract Supabase claims if available
			tenantID := claims.TenantID
			partnerTenantIDs := claims.PartnerTenantIDs
			accountID := claims.AccountID

			// Handle Supabase default claims (sub is the user id)
			if accountID == "" && claims.Subject != "" {
				accountID = claims.Subject
			}
			// Portal cliente (OTP): a conta local pode divergir do `sub` do
			// Supabase, então a identidade vem da claim DEDICADA
			// (`app_metadata.account_id`), não do tenant (P2/M9).
			if claims.AppMetadata != nil && claims.AppMetadata.AccountID != "" {
				accountID = claims.AppMetadata.AccountID
			} else if claims.AppMetadata != nil && claims.AppMetadata.FunctionalRole == "cliente" && claims.AppMetadata.TenantID != "" {
				// Transição: tokens emitidos ANTES do P2/M9 guardavam a conta local
				// em `tenant_id`. Continuam válidos até expirar (1h), com aviso
				// para que a frota saia do formato antigo — sem isso, todo cliente
				// logado no momento do deploy veria "account not found".
				accountID = claims.AppMetadata.TenantID
				logger.Warn("auth.legacy_cliente_account_claim", map[string]interface{}{
					"event":      "auth.legacy_cliente_account_claim",
					"account_id": accountID,
					"path":       path,
					"hint":       "token emitido antes do P2/M9: a identidade vinha de app_metadata.tenant_id; expira em até 1h",
				})
			}

			if strings.TrimSpace(accountID) == "" {
				logger.Warn("Unauthorized access attempt: empty account_id", map[string]interface{}{
					"ip":     c.ClientIP(),
					"path":   path,
					"method": c.Request.Method,
				})
				response.Unauthorized(c, "invalid token: missing account_id")
				c.Abort()
				return
			}

			if claims.AppMetadata != nil {
				if claims.AppMetadata.TenantID != "" {
					tenantID = claims.AppMetadata.TenantID
				}
				if len(claims.AppMetadata.PartnerTenantIDs) > 0 {
					partnerTenantIDs = claims.AppMetadata.PartnerTenantIDs
				}
				if claims.AppMetadata.Permission != "" {
					claims.Permission = claims.AppMetadata.Permission
				}
				if claims.AppMetadata.FunctionalRole != "" {
					claims.FunctionalRole = claims.AppMetadata.FunctionalRole
				}
				if claims.AppMetadata.BaseRole != "" {
					claims.BaseRole = claims.AppMetadata.BaseRole
				}
				if claims.AppMetadata.Type != "" {
					claims.Type = claims.AppMetadata.Type
				}
			}

			// P1/A15 — vínculo revogado (membro removido da empresa, papel
			// alterado): o access token continua válido até expirar, então a
			// remoção precisa de um ponto de checagem por requisição. A chave é
			// escrita pelo toca-auth no ato da remoção e tem TTL do tamanho do
			// token.
			//
			// Fail-open DELIBERADO em falha do Redis: isto é uma mitigação
			// suplementar (a autorização por permissão, essa sim, falha fechada),
			// e derrubar todo o tráfego autenticado por indisponibilidade de cache
			// seria pior que a janela de até 1h que o mecanismo cobre. A falha fica
			// registrada para alarme.
			if checker := RevocationCheckerFromEnv(); checker != nil && tenantID != "" && accountID != "" {
				// A decisão olha QUANDO ESTE token foi emitido quando o checker
				// sabe fazer isso (`RevocationSinceChecker`): papel/perfil trocado
				// grava um timestamp de corte, e um login feito DEPOIS da troca
				// precisa passar — senão o usuário legítimo fica preso no /login
				// até a marca expirar (incidente de 2026-09-21).
				var issuedAt time.Time
				if claims.IssuedAt != nil {
					issuedAt = claims.IssuedAt.Time
				}
				revoked, rerr := isRevokedForToken(c.Request.Context(), checker, tenantID, accountID, issuedAt)
				if rerr != nil {
					logger.Warn("auth.revocation_check_failed", map[string]interface{}{
						"event":      "auth.revocation_check_failed",
						"tenant_id":  tenantID,
						"account_id": accountID,
						"error":      rerr.Error(),
					})
				} else if revoked {
					logger.Warn("auth.membership_revoked", map[string]interface{}{
						"event":      "auth.membership_revoked",
						"tenant_id":  tenantID,
						"account_id": accountID,
						"path":       path,
					})
					response.Unauthorized(c, "membership revoked: sign in again")
					c.Abort()
					return
				}
			}

			// Store user info in context.
			//
			// B9: além de `user_id`, publica `account_id` com o MESMO valor. Havia
			// handler lendo `c.GetString("account_id")` (que o middleware nunca
			// gravava) e degradando em silêncio para uma checagem mais fraca.
			c.Set("user_id", accountID)
			c.Set("account_id", accountID)
			c.Set("email", claims.Email)
			c.Set("name", claims.Name)
			c.Set("avatar_url", claims.AvatarURL)
			c.Set("role", claims.Role)
			c.Set("permission", claims.Permission)
			c.Set("functional_role", claims.FunctionalRole)
			c.Set("base_role", claims.BaseRole)
			c.Set("type", claims.Type)
			c.Set("tenant_id", tenantID)
			c.Set("partner_tenant_ids", partnerTenantIDs)
			if claims.SessionID != "" {
				c.Set("session_id", claims.SessionID)
			}
			if claims.ID != "" {
				c.Set("jti", claims.ID)
			}

			// Propagate context to request context for use in service/repository layers
			reqCtx := c.Request.Context()
			reqCtx = appCtx.WithAccountID(reqCtx, accountID)
			reqCtx = appCtx.WithUserEmail(reqCtx, claims.Email)
			reqCtx = appCtx.WithUserName(reqCtx, claims.Name)
			reqCtx = appCtx.WithAvatarURL(reqCtx, claims.AvatarURL)
			reqCtx = appCtx.WithUserRole(reqCtx, claims.Role)
			reqCtx = appCtx.WithUserPermission(reqCtx, claims.Permission)
			reqCtx = appCtx.WithFunctionalRole(reqCtx, claims.FunctionalRole)
			reqCtx = appCtx.WithBaseRole(reqCtx, claims.BaseRole)
			reqCtx = appCtx.WithUserType(reqCtx, claims.Type)
			reqCtx = appCtx.WithTenantID(reqCtx, tenantID)
			reqCtx = appCtx.WithPartnerTenantIDs(reqCtx, partnerTenantIDs)
			reqCtx = appCtx.WithAuthToken(reqCtx, tokenString)
			c.Request = c.Request.WithContext(reqCtx)

			c.Next()
		} else {
			logger.Warn("Unauthorized access attempt: invalid token claims", map[string]interface{}{
				"ip":     c.ClientIP(),
				"path":   path,
				"method": c.Request.Method,
			})
			response.Unauthorized(c, "invalid token claims")
			c.Abort()
			return
		}
	}
}

// isPublicEndpoint returns true for paths that MUST always bypass the JWT
// middleware (health probes, metrics scraping, ...).
//
// Admin routes (/admin/*) are NOT in this list anymore. They must apply
// their own auth at the route-group level — typically RequireInternalKey()
// for service-only endpoints (e.g. pkg/events/dlq_consumer.go) or
// RequirePermission(...) for human-facing admin actions. Adding /admin/*
// here would silently expose every future admin route to the public if a
// global Middleware is later wired at the engine root.
func isPublicEndpoint(path string) bool {
	publicEndpoints := []string{
		"/health",
		"/metrics",
		"/api/v1/health",
		// Add other public endpoints here
	}

	for _, endpoint := range publicEndpoints {
		if path == endpoint {
			return true
		}
	}
	return false
}

// adminRateLimiter applies a token-bucket per client IP to /admin/* style
// endpoints. Defaults to 60 req/min/IP, override via ADMIN_RATE_LIMIT_PER_MIN.
// Kept in-process (no Redis dep) because the protection is defense-in-depth on
// top of INTERNAL_API_KEY — accuracy across replicas isn't critical here.
type adminRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*adminBucket
	capacity float64
	refill   float64 // tokens per second
}

type adminBucket struct {
	tokens     float64
	lastRefill time.Time
}

var (
	adminLimiter     *adminRateLimiter
	adminLimiterOnce sync.Once
)

func getAdminLimiter() *adminRateLimiter {
	adminLimiterOnce.Do(func() {
		perMin := 60.0
		if v := strings.TrimSpace(os.Getenv("ADMIN_RATE_LIMIT_PER_MIN")); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
				perMin = n
			}
		}
		adminLimiter = &adminRateLimiter{
			buckets:  map[string]*adminBucket{},
			capacity: perMin,
			refill:   perMin / 60.0,
		}
	})
	return adminLimiter
}

// SetAdminRateLimitForTest troca o limitador de /admin por um com a cota
// indicada e devolve a função de restauração. Existe porque a cota é resolvida
// uma única vez por processo (sync.Once): sem esta costura um teste de rota não
// consegue provar de forma determinística que a guarda aplica o limite — o
// primeiro teste do pacote a passar por uma rota interna congela a cota.
//
// Uso: defer auth.SetAdminRateLimitForTest(3)()
func SetAdminRateLimitForTest(perMin float64) func() {
	if perMin <= 0 {
		perMin = 60
	}
	previous := getAdminLimiter()
	adminLimiter = &adminRateLimiter{
		buckets:  map[string]*adminBucket{},
		capacity: perMin,
		refill:   perMin / 60.0,
	}
	return func() { adminLimiter = previous }
}

// allow returns true if the IP has tokens available; false if rate-limited.
func (r *adminRateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	b, ok := r.buckets[ip]
	if !ok {
		b = &adminBucket{tokens: r.capacity, lastRefill: now}
		r.buckets[ip] = b
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * r.refill
		if b.tokens > r.capacity {
			b.tokens = r.capacity
		}
		b.lastRefill = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// internalKeyFingerprint returns a short SHA-256 prefix of the supplied key
// so admin audit logs can correlate calls without leaking the secret itself.
// Empty input yields "" so log filters can detect missing-key attempts.
func internalKeyFingerprint(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8]) // 16 hex chars — enough to correlate, useless on its own
}

// RequireInternalKey gates a route group behind INTERNAL_API_KEY with three
// layers: (1) per-IP rate limit (default 60/min) to slow brute force, (2)
// constant-time comparison to avoid timing side-channels, (3) structured
// audit log on every call (success or denial) so /admin/* access is
// observable. The key itself never reaches logs — only its fingerprint.
func RequireInternalKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		path := c.Request.URL.Path
		method := c.Request.Method

		if !getAdminLimiter().allow(ip) {
			logger.Warn("admin.access_rate_limited", map[string]interface{}{
				"event":  "admin.access_rate_limited",
				"ip":     ip,
				"path":   path,
				"method": method,
			})
			c.Header("Retry-After", "60")
			response.Forbidden(c, "rate limit exceeded for admin endpoint")
			c.Abort()
			return
		}

		expected := os.Getenv("INTERNAL_API_KEY")
		provided := c.GetHeader("X-Internal-Key")

		// Chaves NOMEADAS (INTERNAL_SERVICE_KEYS) têm precedência: permitem
		// vincular a credencial a um serviço e, opcionalmente, a uma allowlist de
		// tenants (achado A3). A chave compartilhada continua valendo como
		// identidade "legacy" para não quebrar a frota durante a migração.
		identity, named := ResolveServiceIdentity(provided)

		// Constant-time comparison protects against timing attacks even
		// though the surface is small. Empty expected key is always a deny.
		valid := named ||
			(expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1)

		if !valid {
			logger.Warn("admin.access_denied", map[string]interface{}{
				"event":               "admin.access_denied",
				"ip":                  ip,
				"path":                path,
				"method":              method,
				"key_fingerprint":     internalKeyFingerprint(provided),
				"expected_configured": expected != "",
			})
			response.Unauthorized(c, "unauthorized internal access")
			c.Abort()
			return
		}

		logger.Info("admin.access", map[string]interface{}{
			"event":           "admin.access",
			"ip":              ip,
			"path":            path,
			"method":          method,
			"service":         identity.Name,
			"legacy_key":      identity.Legacy,
			"key_fingerprint": internalKeyFingerprint(provided),
		})

		// Registra a identidade para que handlers internos possam resolver o
		// tenant pelo credencial (auth.ResolveTenantForInternalRequest).
		setServiceContext(c, identity)
		c.Next()
	}
}

func InternalOrMiddleware(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// N2 (reauditoria de 2026-09-20): este ramo comparava a chave com `==`
		// (não constante) enquanto o M8 já usava `subtle.ConstantTimeCompare` no
		// `RequireInternalKey`. A comparação passa a ser a MESMA resolução —
		// `ResolveServiceIdentity` —, que é constante e ainda devolve a
		// identidade da credencial (nome + allowlist de tenants).
		provided := c.GetHeader("X-Internal-Key")
		if provided != "" {
			if identity, ok := ResolveServiceIdentity(provided); ok {
				// Mesma proteção anti-força-bruta do RequireInternalKey. Sem
				// isto, este ramo era a única superfície que validava segredo
				// sem limite de tentativas.
				if !getAdminLimiter().allow(c.ClientIP()) {
					logger.Warn("admin.access_rate_limited",
						"event", "admin.access_rate_limited",
						"ip", c.ClientIP(), "path", c.Request.URL.Path, "method", c.Request.Method)
					c.Header("Retry-After", "60")
					response.Forbidden(c, "rate limit exceeded for admin endpoint")
					c.Abort()
					return
				}

				tenantID := strings.TrimSpace(c.GetHeader("X-Tenant-Id"))
				if tenantID == "" {
					response.Unauthorized(c, "missing X-Tenant-Id")
					c.Abort()
					return
				}
				// Allowlist da credencial nomeada: uma chave restrita ao tenant A
				// não pode atuar no tenant B só escolhendo o header. Sem allowlist
				// (chave compartilhada legada) o comportamento é o de antes —
				// restringi-la é rollout de ambiente (A2).
				if !identity.AllowsTenant(tenantID) {
					logger.Warn("admin.access_tenant_denied",
						"event", "admin.access_tenant_denied",
						"ip", c.ClientIP(), "path", c.Request.URL.Path,
						"service", identity.Name, "tenant_id", tenantID)
					response.Forbidden(c, "internal credential is not allowed for this tenant")
					c.Abort()
					return
				}

				setServiceContext(c, identity)

				c.Set("user_id", "internal")
				c.Set("email", "internal@toca")
				c.Set("name", "Internal Service")
				c.Set("role", "service")
				// IMPORTANTE: não usar "admin"/"owner" aqui, pois isso bypassa todas as checagens de permissão.
				// O acesso de chamadas internas deve ser explicitamente permitido nas rotas necessárias.
				c.Set("permission", "service")
				c.Set("functional_role", "service")
				c.Set("type", "service")
				c.Set("tenant_id", tenantID)
				c.Set("partner_tenant_ids", []string{})

				reqCtx := c.Request.Context()
				reqCtx = appCtx.WithAccountID(reqCtx, "internal")
				reqCtx = appCtx.WithUserEmail(reqCtx, "internal@toca")
				reqCtx = appCtx.WithUserName(reqCtx, "Internal Service")
				reqCtx = appCtx.WithUserRole(reqCtx, "service")
				reqCtx = appCtx.WithUserPermission(reqCtx, "service")
				reqCtx = appCtx.WithFunctionalRole(reqCtx, "service")
				reqCtx = appCtx.WithUserType(reqCtx, "service")
				reqCtx = appCtx.WithTenantID(reqCtx, tenantID)
				reqCtx = appCtx.WithPartnerTenantIDs(reqCtx, []string{})
				c.Request = c.Request.WithContext(reqCtx)

				c.Next()
				return
			}
		}

		// Sem `X-Internal-Key` (ou com chave que não resolve para nenhuma
		// credencial conhecida): segue para o JWT de usuário, como antes.
		Middleware(cfg)(c)
	}
}

func RequireFunctionalRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("functional_role")
		if !exists {
			recordAuthzDenial(c, "no functional role assigned", strings.Join(roles, "|"))
			response.Forbidden(c, "permission denied: no functional role assigned")
			c.Abort()
			return
		}
		roleStr, ok := roleVal.(string)
		if !ok || roleStr == "" {
			recordAuthzDenial(c, "invalid functional role", strings.Join(roles, "|"))
			response.Forbidden(c, "permission denied: invalid functional role")
			c.Abort()
			return
		}
		for _, r := range roles {
			if roleStr == r {
				c.Next()
				return
			}
		}
		recordAuthzDenial(c, "role not allowed here", strings.Join(roles, "|"))
		response.Forbidden(c, "permission denied: insufficient permissions")
		c.Abort()
	}
}

// RejectFunctionalRole barra os papéis listados.
//
// P2/M2 — antes esta guarda era FAIL-OPEN: sem `functional_role` no contexto
// (claim ausente ou vazia) ela simplesmente deixava passar, então um token sem
// papel atravessava grupos marcados como "de operador" (`/companies`, `/deals`,
// `/channels`, ...). Agora falha fechado — com UMA exceção deliberada: perfil
// root (owner/admin), que legitimamente pode não carregar papel funcional e já
// tem bypass registrado em `auth.permission_bypass` (mesma decisão do
// RequirePermission).
func RejectFunctionalRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("functional_role")
		roleStr, ok := roleVal.(string)
		if !exists || !ok || strings.TrimSpace(roleStr) == "" {
			// Requisição NÃO autenticada (o middleware de auth libera por
			// whitelist caminhos públicos como /health, /metrics, webhooks e
			// login): não há papel a rejeitar aqui, e negar derrubaria os probes
			// de infraestrutura — foi exatamente o que aconteceu quando o
			// fail-closed do achado M2 entrou em produção
			// (`/health`/`/metrics` respondendo 403 e o container ficando
			// unhealthy). A guarda do achado M2 vale para quem ESTÁ autenticado:
			// token válido sem papel não passa por grupo de operador.
			if _, authenticated := c.Get("user_id"); !authenticated {
				c.Next()
				return
			}
			profile, _ := c.Get("permission")
			profileStr, _ := profile.(string)
			if IsRootProfile(profileStr) {
				tenantID := c.GetString("tenant_id")
				auditRootBypass(c.Request.Context(), tenantID, profileStr, "RejectFunctionalRole")
				c.Next()
				return
			}
			denyAuthz(c, "no functional role assigned", strings.Join(roles, "|"))
			c.Abort()
			return
		}
		for _, r := range roles {
			if roleStr == r {
				recordAuthzDenial(c, "role rejected on operator surface", strings.Join(roles, "|"))
				response.Forbidden(c, "permission denied: insufficient permissions")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// RejectFunctionalRoleAllowUnassigned é o RejectFunctionalRole do bootstrap de
// tenant (`POST /api/v1/companies`) — e só dele.
//
// Por que existe: `functional_role` é derivado do CompanyMember, e o vínculo do
// dono é criado PELO PRÓPRIO `POST /companies` (company_service.go: Profile
// "owner", FunctionalRole "admin"). Uma conta recém-cadastrada chega nessa rota,
// por construção, com `functional_role` vazio. O fail-closed do achado M2 —
// correto para os grupos de operador, onde um token sem papel não pode ler nem
// regravar dados de outro tenant — respondia então
// 403 "no functional role assigned" (`authz.denied` na auditoria) e travava o
// onboarding: sem empresa não há papel, e sem papel não há empresa.
//
// Esta variante preserva todas as demais propriedades de RejectFunctionalRole:
// quem não está autenticado não é barrado aqui (o InternalOrMiddleware do grupo
// já resolveu, e os probes de infraestrutura continuam passando) e os papéis
// listados continuam barrados — em especial `cliente`, que segue fora da
// superfície de operador (P0/A11, exigido por scripts/check-route-guards). A
// única diferença é deixar passar o caso "autenticado, ainda sem papel".
func RejectFunctionalRoleAllowUnassigned(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("functional_role")
		roleStr, ok := roleVal.(string)
		if !exists || !ok || strings.TrimSpace(roleStr) == "" {
			// Sem papel atribuído não há papel a rejeitar: ou é o bootstrap de
			// onboarding (conta autenticada, ainda sem vínculo), ou é uma
			// requisição não autenticada liberada por whitelist de caminho
			// público — mesmo tratamento que RejectFunctionalRole dá ao
			// segundo caso.
			c.Next()
			return
		}
		for _, r := range roles {
			if roleStr == r {
				recordAuthzDenial(c, "role rejected on operator surface", strings.Join(roles, "|"))
				response.Forbidden(c, "permission denied: insufficient permissions")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// RequireRole is a legacy middleware to check if the user has the required static role (AccountRole/Profile)
//
// Sem chamador no repositório (verificado na reauditoria de 2026-09-20): as
// rotas usam `RequirePermission`/`RequireFunctionalRole`. Segue exportada por ser
// API de `pkg/auth`, mas a negação dela agora deixa trilha como as demais — antes
// era o único guarda que podia barrar sem aparecer na auditoria.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use the "permission" claim which maps to the AccountRole (Profile: admin, manager, member)
		userProfile, exists := c.Get("permission")
		if !exists {
			recordAuthzDenial(c, "profile not found", strings.Join(roles, "|"))
			response.Forbidden(c, "permission denied: profile not found")
			c.Abort()
			return
		}

		profileStr, ok := userProfile.(string)
		if !ok {
			recordAuthzDenial(c, "invalid profile format", strings.Join(roles, "|"))
			response.Forbidden(c, "permission denied: invalid profile format")
			c.Abort()
			return
		}

		for _, role := range roles {
			if profileStr == role {
				c.Next()
				return
			}
		}

		recordAuthzDenial(c, "profile not in allowed set", strings.Join(roles, "|"))
		response.Forbidden(c, "permission denied: insufficient permissions")
		c.Abort()
	}
}

// PermissionProvider is an interface to fetch tenant permissions
type PermissionProvider interface {
	GetRolesMap(ctx context.Context, tenantID, requiredPermission string) (map[string]bool, error)
}

// RequirePermissionOrInternalKey permite chamadas internas (X-Internal-Key validado por InternalOrMiddleware)
// passarem sem consultar o PermissionProvider. Deve ser usado somente em rotas explicitamente aprovadas
// para tráfego inter-serviço.
func RequirePermissionOrInternalKey(provider PermissionProvider, requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		userType, _ := c.Get("type")
		if userID == "internal" && userType == "service" {
			c.Next()
			return
		}
		RequirePermission(provider, requiredPermission)(c)
	}
}

// RequirePermission is an ABAC/RBAC middleware that checks granular permissions
// HasPermission é o equivalente não-middleware do RequirePermission —
// usado dentro de handlers para checar condicionalmente uma permission
// (ex.: gate de aprovação por valor #416, onde o caller PODE chamar o
// endpoint mas só pode mover deals altos se tiver permission específica).
//
// Retorna true quando:
//   - Profile é root (admin/owner) — mesmo bypass do middleware
//   - rolesMap[functional_role] = true para essa permission
//
// Tudo o que o middleware retornaria como 403 vira false aqui; o caller
// decide o que fazer (geralmente outro errro contextual). Defaults
// defensivos: sem tenant, profile, role, ou provider → false.
func HasPermission(c *gin.Context, provider PermissionProvider, permission string) bool {
	if strings.TrimSpace(permission) == "" {
		// P2/B3 — contrato endurecido. Permissão vazia significa "só autenticação"
		// nas rotas que usam `RequirePermission(provider, "")` de propósito; aqui,
		// como chamada DENTRO de um handler, vazio é quase sempre engano (o autor
		// queria exigir algo e passou a constante errada). Continua devolvendo true
		// para não mudar rota existente, mas deixa rastro para o engano aparecer
		// em log/observabilidade em vez de virar um `true` silencioso.
		logger.Warn("authz.empty_permission_check",
			"event", "authz.empty_permission_check",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"hint", "HasPermission com permissão vazia = apenas autenticação; confirme que é intencional",
		)
		return true
	}
	tenantIDVal, exists := c.Get("tenant_id")
	if !exists {
		return false
	}
	tenantID, ok := tenantIDVal.(string)
	if !ok || tenantID == "" {
		return false
	}

	userProfileVal, exists := c.Get("permission")
	if exists {
		if userProfile, ok := userProfileVal.(string); ok && IsRootProfile(userProfile) {
			return true
		}
	}

	functionalRoleVal, _ := c.Get("functional_role")
	functionalRole := ""
	if functionalRoleVal != nil {
		if s, ok := functionalRoleVal.(string); ok {
			functionalRole = s
		}
	}
	if functionalRole == "" {
		return false
	}

	if provider == nil {
		return false
	}

	rolesMap, err := provider.GetRolesMap(c.Request.Context(), tenantID, permission)
	if err != nil || rolesMap == nil {
		return false
	}
	allowed, ok := rolesMap[functionalRole]
	return ok && allowed
}

// isRevokedForToken consulta o checker preferindo a decisão por instante de
// emissão. Checkers que não implementam `RevocationSinceChecker` (dublês
// antigos, implantações parciais) caem no `IsRevoked`, que bloqueia enquanto a
// marca existir — o comportamento conservador.
func isRevokedForToken(ctx context.Context, checker RevocationChecker, tenantID, accountID string, issuedAt time.Time) (bool, error) {
	if since, ok := checker.(RevocationSinceChecker); ok {
		return since.IsRevokedSince(ctx, tenantID, accountID, issuedAt)
	}
	return checker.IsRevoked(ctx, tenantID, accountID)
}

func RequirePermission(provider PermissionProvider, requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantIDVal, exists := c.Get("tenant_id")
		if !exists {
			denyAuthz(c, "tenant not found", requiredPermission)
			c.Abort()
			return
		}
		tenantID := tenantIDVal.(string)

		userProfileVal, exists := c.Get("permission")
		if !exists {
			denyAuthz(c, "profile not found", requiredPermission)
			c.Abort()
			return
		}
		userProfile := userProfileVal.(string)

		functionalRoleVal, _ := c.Get("functional_role")
		functionalRole := ""
		if functionalRoleVal != nil {
			functionalRole = functionalRoleVal.(string)
		}

		// Root profile bypass. Owner/admin profiles are granted blanket
		// access via IsRootProfile (pkg/auth/roles.go) — the same check
		// is mirrored inside CorePermissionProvider.GetRolesMap so the
		// decision is consistent whether middleware short-circuits or
		// the provider is called directly. Kept in the middleware path
		// too because root profiles may not carry a functional_role and
		// the catalog lookup below requires one.
		//
		// auditRootBypass writes a structured log line tagged with
		// event=auth.permission_bypass for SIEM / forensics. See audit
		// doc §8.11.
		if IsRootProfile(userProfile) {
			auditRootBypass(c.Request.Context(), tenantID, userProfile, requiredPermission)
			c.Next()
			return
		}

		// Chamadores INTERNOS (X-Internal-Key) carregam permission=service e não
		// têm identidade de usuário: rotas de permissão de USUÁRIO não podem
		// autorizá-los — as rotas internas usam RequireInternalKey. Sem este
		// ramo o provider falhava ao procurar um token que não existe e o
		// chamador recebia 503 "infraestrutura", um falso alarme: aqui é
		// decisão (403) com trilha própria (T-10).
		if userProfile == "service" {
			denyAuthz(c, "internal service callers cannot use user permission routes", requiredPermission)
			c.Abort()
			return
		}

		// Empty functional role means they haven't switched context properly or are just logging in
		if functionalRole == "" {
			denyAuthz(c, "no functional role assigned", requiredPermission)
			c.Abort()
			return
		}

		// If the required permission is empty, it means we only needed to enforce the functional role check
		// (e.g. just ensuring the user is properly logged in and has a role context, but no specific granular permission)
		if requiredPermission == "" {
			c.Next()
			return
		}

		// Fetch dynamic permissions for the tenant
		if provider == nil {
			denyAuthz(c, "permission provider not configured", requiredPermission)
			c.Abort()
			return
		}

		rolesMap, err := provider.GetRolesMap(c.Request.Context(), tenantID, requiredPermission)
		if err != nil {
			// FALHA DE INFRAESTRUTURA não é decisão de autorização.
			//
			// O ramo respondia 403 com "no permissions configured for tenant" e
			// ENGOLIA o erro do provider: o operador via "acesso negado" (e o
			// incidente dos agendamentos de 2026-09-21 ficou 50 minutos sem causa
			// visível) quando a verdade era "não foi possível verificar". A
			// resposta continua fechada — nada passa —, mas com 503 e o erro
			// registrado. Mesmo princípio do incidente 3 (`ErrPortalUpstreamUnavailable`):
			// falha de destino nunca vira "você não tem permissão".
			recordAuthzDenial(c, "permission provider failed", requiredPermission)
			logger.Error("authz.provider_failed",
				"event", "authz.provider_failed",
				"error", err.Error(),
				"permission", requiredPermission,
				"tenant_id", tenantID,
				"actor_id", c.GetString("user_id"),
				"functional_role", functionalRole,
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
			)
			response.Error(c, http.StatusServiceUnavailable, "permission check unavailable: try again")
			c.Abort()
			return
		}
		if rolesMap == nil {
			denyAuthz(c, "no permissions configured for tenant", requiredPermission)
			c.Abort()
			return
		}

		if allowed, ok := rolesMap[functionalRole]; ok && allowed {
			c.Next()
			return
		}

		denyAuthz(c, "role lacks the required permission", requiredPermission)
		c.Abort()
	}
}

func RequireAnyPermission(provider PermissionProvider, perms ...string) gin.HandlerFunc {
	// N8 (reauditoria de 2026-09-20): lista vazia ou entrada vazia é erro de
	// PROGRAMAÇÃO na montagem da rota, não decisão de autorização. Antes as duas
	// liberavam (`c.Next()`), e a entrada vazia — pior das duas —
	// curto-circuitava ANTES de avaliar as outras permissões, então
	// `RequireAnyPermission(p, "a", "")` autorizava todo papel, mesmo sem "a".
	// Falha FECHADO com trilha: o defeito fica visível sem abrir a rota. Não é
	// `panic` porque `pkg/auth` não usa panic em nenhum outro ponto e derrubar o
	// boot de um serviço por causa de uma rota seria pior que negá-la.
	invalidPerms := len(perms) == 0
	for _, p := range perms {
		if strings.TrimSpace(p) == "" {
			invalidPerms = true
			break
		}
	}

	return func(c *gin.Context) {
		if invalidPerms {
			denyAuthz(c, "empty permission requirement", strings.Join(perms, "|"))
			c.Abort()
			return
		}

		tenantIDVal, exists := c.Get("tenant_id")
		if !exists {
			denyAuthz(c, "tenant not found", strings.Join(perms, "|"))
			c.Abort()
			return
		}
		tenantID, ok := tenantIDVal.(string)
		if !ok {
			denyAuthz(c, "invalid tenant claim", strings.Join(perms, "|"))
			c.Abort()
			return
		}

		userProfileVal, exists := c.Get("permission")
		if !exists {
			denyAuthz(c, "profile not found", strings.Join(perms, "|"))
			c.Abort()
			return
		}
		userProfile, ok := userProfileVal.(string)
		if !ok {
			denyAuthz(c, "invalid profile claim", strings.Join(perms, "|"))
			c.Abort()
			return
		}

		functionalRoleVal, _ := c.Get("functional_role")
		functionalRole := ""
		if functionalRoleVal != nil {
			role, ok := functionalRoleVal.(string)
			if !ok {
				denyAuthz(c, "invalid functional role claim", strings.Join(perms, "|"))
				c.Abort()
				return
			}
			functionalRole = role
		}

		if IsRootProfile(userProfile) {
			auditRootBypass(c.Request.Context(), tenantID, userProfile, joinPerms(perms))
			c.Next()
			return
		}

		if functionalRole == "" {
			denyAuthz(c, "no functional role assigned", strings.Join(perms, "|"))
			c.Abort()
			return
		}

		if provider == nil {
			denyAuthz(c, "permission provider not configured", strings.Join(perms, "|"))
			c.Abort()
			return
		}

		for _, p := range perms {
			rolesMap, err := provider.GetRolesMap(c.Request.Context(), tenantID, p)
			if err != nil {
				// Mesmo critério do RequirePermission: falha do provider é
				// infraestrutura, não ausência de permissão. Segue negando
				// (fail-closed), mas com 503 e o erro no log.
				recordAuthzDenial(c, "permission provider failed", p)
				logger.Error("authz.provider_failed",
					"event", "authz.provider_failed",
					"error", err.Error(),
					"permission", p,
					"tenant_id", tenantID,
					"actor_id", c.GetString("user_id"),
					"functional_role", functionalRole,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
				)
				response.Error(c, http.StatusServiceUnavailable, "permission check unavailable: try again")
				c.Abort()
				return
			}
			if rolesMap == nil {
				continue
			}
			if allowed, ok := rolesMap[functionalRole]; ok && allowed {
				c.Next()
				return
			}
		}

		denyAuthz(c, "role lacks any of the required permissions", strings.Join(perms, "|"))
		c.Abort()
	}
}

// joinPerms collapses a permission slice into a single comma-separated
// string for the audit log entry. Empty / single-element slices are
// passed through unchanged for readability.
func joinPerms(perms []string) string {
	if len(perms) == 0 {
		return ""
	}
	if len(perms) == 1 {
		return perms[0]
	}
	return strings.Join(perms, ",")
}

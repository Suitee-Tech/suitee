package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/logger"
)

// BaseConfig holds common configuration for all services
type BaseConfig struct {
	AppEnv         string
	AppMode        string
	Port           string
	InternalAPIKey string
	Database       DatabaseConfig
	Logger         logger.Config
	CORS           CORSConfig
	JWT            JWTConfig
	RateLimit      RateLimitConfig
	RabbitMQ       RabbitMQConfig
	Email          EmailConfig
}

type EmailConfig struct {
	ResendAPIKey string
	FromAddress  string
	FrontendURL  string
	PortalURL    string

	// SMTP local (Inbucket do Supabase) tem precedência sobre o Resend quando
	// SMTP_HOST está definido. Em produção fica vazio: o provider é o Resend.
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// Notificações transacionais por e-mail (beta — docs/features/comunicacao/
	// notificacoes-email-beta.md). COMM_EMAIL_ENABLED é o kill-switch global:
	// desligado por padrão para que deploys existentes não mudem de
	// comportamento até o operador ligar explicitamente. As categorias ficam
	// ligadas por padrão e só disparam com o kill-switch global ligado.
	TransactionalEmailEnabled bool
	EmailLeadAssigned         bool
	EmailDealUpdates          bool
	EmailProposals            bool
	EmailAppointments         bool
	EmailRecommendations      bool
	EmailChatReminders        bool
	// EmailCooldownMinutes é o intervalo mínimo entre dois e-mails da MESMA
	// categoria para o MESMO destinatário (0 = sem cooldown). Marcos críticos
	// (ex.: lead atribuído) usam Force e ignoram o cooldown.
	EmailCooldownMinutes int
	// EmailChatReminderAfterMinutes é a idade mínima de uma mensagem não lida
	// para o lembrete de chat disparar (beta F3). 0 usa o default do motor.
	EmailChatReminderAfterMinutes int
}

type DatabaseConfig struct {
	URI         string
	ServiceURI  string
	Name        string
	MaxPoolSize uint64
	MinPoolSize uint64
}

type RabbitMQConfig struct {
	URI string
}

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

type JWTConfig struct {
	SupabaseJWKSURL        string // NOVO: URL do JWKS do Supabase
	Issuer                 string
	Audience               string
	Expiration             time.Duration
	AccessTokenExpiration  time.Duration
	RefreshTokenExpiration time.Duration
}

type RateLimitConfig struct {
	RPS   float64
	Burst int
}

// LoadBaseConfig loads the base configuration from environment variables
func LoadBaseConfig() BaseConfig {
	appEnv := GetEnv("APP_ENV", "development")
	appMode := GetEnv("APP_MODE", "debug")

	// Supabase URL e Configurações
	supabaseJWKSURL := GetEnv("SUPABASE_JWKS_URL", "")
	if supabaseJWKSURL == "" {
		supabaseURL := GetEnv("SUPABASE_URL", "")
		if supabaseURL != "" {
			supabaseJWKSURL = strings.TrimRight(supabaseURL, "/") + "/auth/v1/.well-known/jwks.json"
		}
	}

	if supabaseJWKSURL == "" {
		logger.Error("missing required env var: SUPABASE_JWKS_URL (or SUPABASE_URL) must be provided")
		os.Exit(1)
	}

	dbURI, _ := os.LookupEnv("DATABASE_URI")
	dbURI = strings.TrimSpace(dbURI)
	if dbURI == "" {
		dbURI = GetEnv("SUPABASE_DB_URL", "postgresql://postgres:postgres@localhost:5432/toca?sslmode=disable")
	}

	serviceURI, err := resolveServiceURI(appEnv, appMode, dbURI, os.Getenv("DATABASE_URI_SERVICE"))
	if err != nil {
		logger.Error("invalid database configuration", "reason", err.Error())
		os.Exit(1)
	}

	corsAllowedOrigins := splitAndTrim(GetEnv("CORS_ALLOWED_ORIGINS", "*"))
	corsAllowedMethods := splitAndTrim(GetEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"))
	// O default precisa cobrir os headers que o frontend manda nas chamadas de
	// API (Idempotency-Key, X-Idempotency-Key, X-Auth-Version, X-Request-ID):
	// header enviado e não permitido é bloqueado no preflight, antes de sair do
	// browser. A borda pública (gateway) mantém a própria lista; esta vale para
	// serviços expostos diretamente em dev/test.
	corsAllowedHeaders := splitAndTrim(GetEnv("CORS_ALLOWED_HEADERS", "Authorization,Content-Type,Accept,Origin,X-Tenant-Id,X-Auth-Version,Idempotency-Key,X-Idempotency-Key,X-Request-ID"))
	corsAllowCredentials := GetEnvBool("CORS_ALLOW_CREDENTIALS", true)

	if isRelease(appEnv, appMode) {
		if len(corsAllowedOrigins) == 0 {
			logger.Error("invalid CORS configuration", "reason", "empty_origins")
			os.Exit(1)
		}
		if contains(corsAllowedOrigins, "*") && corsAllowCredentials {
			logger.Error("invalid CORS configuration", "reason", "wildcard_with_credentials")
			os.Exit(1)
		}
	}

	internalAPIKey, _ := os.LookupEnv("INTERNAL_API_KEY")
	internalAPIKey = strings.TrimSpace(internalAPIKey)
	if internalAPIKey == "" {
		logger.Error("missing required env var", "key", "INTERNAL_API_KEY")
		os.Exit(1)
	}

	return BaseConfig{
		AppEnv:         appEnv,
		AppMode:        appMode,
		Port:           GetEnv("PORT", "8080"),
		InternalAPIKey: internalAPIKey,
		Database: DatabaseConfig{
			URI:         dbURI,
			ServiceURI:  serviceURI,
			Name:        GetEnv("DATABASE_NAME", "toca"),
			MaxPoolSize: uint64(GetEnvInt("DATABASE_MAX_POOL_SIZE", 5)),
			MinPoolSize: uint64(GetEnvInt("DATABASE_MIN_POOL_SIZE", 0)),
		},
		Logger: logger.Config{
			Level:  GetEnv("LOG_LEVEL", "info"),
			Format: GetEnv("LOG_FORMAT", "json"),
		},
		CORS: CORSConfig{
			AllowedOrigins:   corsAllowedOrigins,
			AllowedMethods:   corsAllowedMethods,
			AllowedHeaders:   corsAllowedHeaders,
			AllowCredentials: corsAllowCredentials,
		},
		JWT: JWTConfig{
			SupabaseJWKSURL:        supabaseJWKSURL,
			Issuer:                 GetEnv("JWT_ISSUER", "toca-api"),
			Audience:               GetEnv("JWT_AUDIENCE", "toca-audience"),
			Expiration:             GetEnvDuration("JWT_EXPIRATION", 24*time.Hour),
			AccessTokenExpiration:  GetEnvDuration("ACCESS_TOKEN_EXPIRATION", 15*time.Minute),
			RefreshTokenExpiration: GetEnvDuration("REFRESH_TOKEN_EXPIRATION", 7*24*time.Hour),
		},
		RateLimit: RateLimitConfig{
			RPS:   GetEnvFloat("RATE_LIMIT_RPS", 10.0),
			Burst: GetEnvInt("RATE_LIMIT_BURST", 20),
		},
		RabbitMQ: RabbitMQConfig{
			URI: GetEnv("RABBITMQ_URI", ""),
		},
		Email: EmailConfig{
			ResendAPIKey: GetEnv("RESEND_API_KEY", GetEnv("EMAIL_API_KEY", "")),
			FromAddress:  GetEnv("RESEND_FROM_ADDRESS", GetEnv("EMAIL_FROM", GetEnv("ACME_EMAIL", "update@suitee.com.br"))),
			FrontendURL:  GetEnv("FRONTEND_URL", ""),
			PortalURL:    GetEnv("PORTAL_URL", ""),
			SMTPHost:     GetEnv("SMTP_HOST", ""),
			SMTPPort:     GetEnv("SMTP_PORT", "587"),
			SMTPUsername: GetEnv("SMTP_USERNAME", ""),
			SMTPPassword: GetEnv("SMTP_PASSWORD", ""),
			SMTPFrom:     GetEnv("SMTP_FROM", ""),

			TransactionalEmailEnabled:     GetEnvBool("COMM_EMAIL_ENABLED", false),
			EmailLeadAssigned:             GetEnvBool("COMM_EMAIL_LEAD_ASSIGNED", true),
			EmailDealUpdates:              GetEnvBool("COMM_EMAIL_DEAL_UPDATES", true),
			EmailProposals:                GetEnvBool("COMM_EMAIL_PROPOSALS", true),
			EmailAppointments:             GetEnvBool("COMM_EMAIL_APPOINTMENTS", true),
			EmailRecommendations:          GetEnvBool("COMM_EMAIL_RECOMMENDATIONS", true),
			EmailChatReminders:            GetEnvBool("COMM_EMAIL_CHAT_REMINDERS", true),
			EmailCooldownMinutes:          GetEnvInt("COMM_EMAIL_COOLDOWN_MINUTES", 60),
			EmailChatReminderAfterMinutes: GetEnvInt("COMM_EMAIL_CHAT_REMINDER_AFTER_MINUTES", 30),
		},
	}
}

func resolveServiceURI(appEnv, appMode, dbURI, configured string) (string, error) {
	serviceURI := strings.TrimSpace(configured)
	if serviceURI == "" {
		if isRelease(appEnv, appMode) {
			return "", fmt.Errorf("DATABASE_URI_SERVICE is required in release environments")
		}
		// Local/test environments commonly use a superuser without RLS and may
		// intentionally share the pool. Production must always provide an
		// explicit service-role connection.
		return dbURI, nil
	}
	if isRelease(appEnv, appMode) && serviceURI == dbURI {
		return "", fmt.Errorf("DATABASE_URI_SERVICE must differ from DATABASE_URI in release environments")
	}
	return serviceURI, nil
}

// IsReleaseEnv é o critério ÚNICO de "ambiente de release" (APP_MODE=release ou
// APP_ENV=production|prod). Achado M16 da revisão de permissões de 2026-09-19:
// cada serviço repetia a própria cópia do critério, e uma divergência bastava
// para deixar Swagger/métricas expostos em produção. Exportado para que os
// gates de documentação (pkg/apidocs) e de proxies confiáveis
// (pkg/middleware) usem exatamente a mesma regra da validação de CORS/DB.
func IsReleaseEnv(appEnv, appMode string) bool {
	if strings.EqualFold(appMode, "release") {
		return true
	}
	return strings.EqualFold(appEnv, "production") || strings.EqualFold(appEnv, "prod")
}

func isRelease(appEnv, appMode string) bool {
	return IsReleaseEnv(appEnv, appMode)
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			return boolValue
		}
	}
	return fallback
}

func GetEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		duration, err := time.ParseDuration(value)
		if err == nil {
			return duration
		}
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			return intValue
		}
	}
	return fallback
}

func GetEnvFloat(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return floatValue
		}
	}
	return fallback
}

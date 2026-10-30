package config

import (
	"os"
	"strings"
)

type Config struct {
	Port                string
	FrontendURL         string
	FrontendCORSURL     string
	AuthServiceURL      string
	CoreServiceURL      string
	FinancialServiceURL string
	CRMServiceURL       string
	DealServiceURL      string
	CommServiceURL      string
	OpsServiceURL       string
	CalendarServiceURL  string
	PortalServiceURL    string
	AIServiceURL        string
	SignatureServiceURL string
	SitesServiceURL     string
	RedisURI            string
	AppEnv              string
	AppMode             string
	PlanAccessEnabled   bool
	CORSAllowedOrigins  string
	// PublicSiteCORSOrigin é o padrão de origem da zona pública das
	// imobiliárias (ex.: `https://*.imoveis.suitee.com.br`). Vai para a
	// allowlist de CORS SEM virar wildcard global: o matcher aceita apenas
	// subdomínios do sufixo configurado.
	PublicSiteCORSOrigin string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	frontendCORSURL := frontendURL
	frontendURL = strings.Replace(frontendURL, "localhost", "host.docker.internal", 1)
	frontendURL = strings.Replace(frontendURL, "127.0.0.1", "host.docker.internal", 1)

	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		authServiceURL = "http://toca-auth:8081"
	}

	coreServiceURL := os.Getenv("CORE_SERVICE_URL")
	if coreServiceURL == "" {
		coreServiceURL = "http://toca-core:8082"
	}

	financialServiceURL := os.Getenv("FINANCIAL_SERVICE_URL")
	if financialServiceURL == "" {
		financialServiceURL = "http://toca-financial:8083"
	}

	crmServiceURL := os.Getenv("CRM_SERVICE_URL")
	if crmServiceURL == "" {
		crmServiceURL = "http://toca-crm:8084"
	}

	dealServiceURL := os.Getenv("DEAL_SERVICE_URL")
	if dealServiceURL == "" {
		dealServiceURL = "http://toca-deal:8085"
	}

	commServiceURL := os.Getenv("COMM_SERVICE_URL")
	if commServiceURL == "" {
		commServiceURL = "http://toca-comm:8086"
	}

	opsServiceURL := os.Getenv("OPS_SERVICE_URL")
	if opsServiceURL == "" {
		opsServiceURL = "http://toca-ops:8087"
	}

	calendarServiceURL := os.Getenv("CALENDAR_SERVICE_URL")
	if calendarServiceURL == "" {
		calendarServiceURL = "http://toca-calendar:8088"
	}

	portalServiceURL := os.Getenv("PORTAL_SERVICE_URL")
	if portalServiceURL == "" {
		portalServiceURL = "http://toca-portal:8089"
	}

	aiServiceURL := os.Getenv("AI_SERVICE_URL")
	if aiServiceURL == "" {
		aiServiceURL = "http://toca-ai:8092"
	}

	sitesServiceURL := os.Getenv("SITES_SERVICE_URL")
	if sitesServiceURL == "" {
		sitesServiceURL = "http://toca-sites:8094"
	}

	signatureServiceURL := os.Getenv("SIGNATURE_SERVICE_URL")
	if signatureServiceURL == "" {
		signatureServiceURL = "http://toca-signature:8093"
	}

	redisURI := os.Getenv("REDIS_URI")
	if redisURI == "" {
		redisURI = "redis://redis:6379"
	}

	appEnv := os.Getenv("APP_ENV")
	appMode := os.Getenv("APP_MODE")
	planAccessEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("PLATFORM_ACCESS_CONTROL_ENABLED")), "true")

	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	}

	return Config{
		Port:                 port,
		FrontendURL:          frontendURL,
		FrontendCORSURL:      frontendCORSURL,
		AuthServiceURL:       authServiceURL,
		CoreServiceURL:       coreServiceURL,
		FinancialServiceURL:  financialServiceURL,
		CRMServiceURL:        crmServiceURL,
		DealServiceURL:       dealServiceURL,
		CommServiceURL:       commServiceURL,
		OpsServiceURL:        opsServiceURL,
		CalendarServiceURL:   calendarServiceURL,
		PortalServiceURL:     portalServiceURL,
		AIServiceURL:         aiServiceURL,
		SignatureServiceURL:  signatureServiceURL,
		SitesServiceURL:      sitesServiceURL,
		PublicSiteCORSOrigin: os.Getenv("PUBLIC_SITE_CORS_ORIGIN"),
		RedisURI:             redisURI,
		AppEnv:               appEnv,
		AppMode:              appMode,
		PlanAccessEnabled:    planAccessEnabled,
		CORSAllowedOrigins:   allowedOrigins,
	}
}

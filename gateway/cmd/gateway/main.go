package main

import (
	"context"
	"log"

	"github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/repositories"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
	"github.com/oginoino/toca/pkg/tracing"
)

func main() {
	cfg := config.Load()

	shutdown, err := tracing.InitProvider("gateway")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}

	rdb := repositories.NewRedisClient(cfg.RedisURI)

	metricsServer := startMetricsServer(metricsPortFromEnv(), buildMetricsEngine())
	defer shutdownMetricsServer(metricsServer)

	public := buildPublicEngine(cfg, proxy.New(), rdb)

	log.Printf("Gateway running on port %s", cfg.Port)
	log.Printf("Auth Service: %s", cfg.AuthServiceURL)
	log.Printf("Core Service: %s", cfg.CoreServiceURL)
	log.Printf("Financial Service: %s", cfg.FinancialServiceURL)
	log.Printf("CRM Service: %s", cfg.CRMServiceURL)
	log.Printf("Deal Service: %s", cfg.DealServiceURL)
	log.Printf("Comm Service: %s", cfg.CommServiceURL)
	log.Printf("Ops Service: %s", cfg.OpsServiceURL)
	log.Printf("Calendar Service: %s", cfg.CalendarServiceURL)
	log.Printf("Portal Service: %s", cfg.PortalServiceURL)
	log.Printf("AI Service: %s", cfg.AIServiceURL)
	log.Printf("Signature Service: %s", cfg.SignatureServiceURL)
	log.Printf("Sites Service: %s", cfg.SitesServiceURL)

	if err := public.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}

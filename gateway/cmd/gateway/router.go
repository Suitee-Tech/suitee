package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"os"
	"strings"

	"github.com/gin-gonic/gin"
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/handlers"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
	pkgconfig "github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/tracing"
	"github.com/redis/go-redis/v9"
)

// defaultMetricsPort é a porta do listener interno de métricas do gateway.
//
// SECURITY (achados B1/A16 da revisão de permissões de 2026-09-19): `/metrics`
// não é servido pela engine pública. A rota vivia no mesmo router que a borda
// expõe (Host público + Traefik), o que em beta deixava as métricas acessíveis
// sem credencial — e uma label de rate limit ainda aceitava valor escolhido
// pelo cliente. As métricas agora ficam numa porta que NÃO é publicada em
// produção/beta (só a rede interna do compose alcança), usada pelo Prometheus.
const defaultMetricsPort = "9091"

// gatewayCORSAllowedHeaders é a lista que o browser valida no preflight. Header
// que o frontend envia e não está aqui faz a requisição morrer em "blocked by
// CORS policy" ANTES de sair do browser (sem log de servidor, sem erro de
// aplicação — só Network Error no console).
//
// `Idempotency-Key` (grafia canônica, a que o axios manda no fio) ficou de fora
// enquanto só `X-Idempotency-Key` estava na lista: em 30/09/2026 isso quebrou
// "marcar cobrança como paga" (POST /charges/:id/reconcile-manual) mesmo com o
// backend lendo as duas grafias. `scripts/check-cors-headers` (make check)
// compara esta lista com os headers que o frontend envia.
var gatewayCORSAllowedHeaders = []string{
	"Origin", "Content-Length", "Content-Type", "Authorization",
	"X-Auth-Version", "X-Tenant-Id", "Idempotency-Key", "X-Idempotency-Key", "X-Request-ID", "X-Site-Host",
}

// buildPublicEngine monta a engine que a borda expõe (porta 8080). Nenhuma
// rota administrativa/de observabilidade é registrada aqui.
func buildPublicEngine(cfg gwconfig.Config, pxy *proxy.Service, rdb *redis.Client) *gin.Engine {
	r := gin.Default()
	r.RedirectTrailingSlash = false

	// P2/A16: sem TRUSTED_PROXIES nenhum proxy é confiado e os cabeçalhos de
	// encaminhamento do cliente são ignorados por c.ClientIP().
	pkgmw.ApplyTrustedProxies(r)

	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("gateway"))
	r.Use(metrics.GinMiddleware("gateway"))
	r.Use(pkgmw.RequestTimeoutWithSkip(60*time.Second, func(c *gin.Context) bool {
		switch c.Request.URL.Path {
		case "/api/v1/ai/chat", "/api/v1/ws":
			return true
		default:
			return false
		}
	}))

	readiness := health.NewReadinessChecker()
	if rdb != nil {
		readiness.Add(health.NewRedisChecker(rdb))
	}
	health.RegisterGin(r, "gateway", readiness)
	deep := health.NewReadinessChecker()
	if rdb != nil {
		deep.Add(health.NewRedisDeepChecker(rdb, "health:probe:gateway"))
	}
	health.RegisterDeepGin(r, "gateway", deep)

	origins, err := gwmiddleware.AllowedCORSOrigins(cfg.AppEnv, cfg.AppMode, cfg.CORSAllowedOrigins, cfg.FrontendCORSURL)
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
	// Zona pública multi-tenant: `{marca}.imoveis.suitee.com.br`. A origem muda
	// por imobiliária, então a allowlist aceita o padrão com wildcard (o
	// matcher do pkg/middleware só aceita subdomínios do sufixo).
	if pattern := strings.TrimSpace(cfg.PublicSiteCORSOrigin); pattern != "" {
		origins = append(origins, pattern)
	}

	r.Use(pkgmw.SetupCORS(pkgconfig.CORSConfig{
		AllowedOrigins:   origins,
		AllowedHeaders:   gatewayCORSAllowedHeaders,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowCredentials: true,
	}))

	r.Use(pkgmw.SecurityHeaders())

	// P0/C10 + A2: a borda pública nunca serve rotas /internal/ (segredo
	// estático compartilhado + tenant escolhido pelo chamador). Vale para
	// qualquer serviço, inclusive rotas internas criadas no futuro.
	r.Use(gwmiddleware.RejectInternalPaths())

	r.Use(gwmiddleware.RedisCache(rdb))
	r.Use(gwmiddleware.NewIPRateLimiter().Middleware())
	r.Use(gwmiddleware.RequireActiveSubscription(cfg.FinancialServiceURL, cfg.PlanAccessEnabled))

	handlers.RegisterClientErrors(r)
	handlers.RegisterInfra(r)
	handlers.RegisterFrontend(r, cfg, pxy)
	handlers.RegisterAuth(r, cfg, pxy)
	handlers.RegisterFinancial(r, cfg, pxy)
	handlers.RegisterCRM(r, cfg, pxy)
	handlers.RegisterDeal(r, cfg, pxy)
	handlers.RegisterComm(r, cfg, pxy)
	handlers.RegisterOps(r, cfg, pxy)
	handlers.RegisterCalendar(r, cfg, pxy)
	handlers.RegisterPortal(r, cfg, pxy)
	handlers.RegisterAI(r, cfg, pxy)
	handlers.RegisterSignature(r, cfg, pxy)
	handlers.RegisterSites(r, cfg, pxy)
	handlers.RegisterCore(r, cfg, pxy)

	return r
}

// buildMetricsEngine monta o listener interno de métricas: apenas `/metrics`,
// sem CORS, sem rate limit e sem as rotas de API. A superfície é fechada por
// rede (a porta não é publicada nos stacks de release), não por token — é o
// mesmo desenho que os demais serviços já usam ao expor `/metrics` em `expose:`.
func buildMetricsEngine() *gin.Engine {
	engine := gin.New()
	handlers.RegisterMetrics(engine)
	return engine
}

// startMetricsServer sobe o listener interno de métricas. Porta vazia ou "0"
// desliga o listener (para ambientes que não fazem scrape).
func startMetricsServer(port string, engine *gin.Engine) *http.Server {
	if port == "" || port == "0" {
		log.Printf("Gateway metrics listener disabled")
		return nil
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("Gateway metrics listener on port %s (not exposed by the edge)", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Gateway metrics listener stopped: %v", err)
		}
	}()
	return server
}

func shutdownMetricsServer(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// metricsPortFromEnv resolve a porta do listener interno
// (GATEWAY_METRICS_PORT; vazio ou "0" desliga o listener).
func metricsPortFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("GATEWAY_METRICS_PORT")); v != "" {
		return v
	}
	return defaultMetricsPort
}

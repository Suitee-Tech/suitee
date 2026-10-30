package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
)

func RegisterDeal(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	signatureAccess := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "electronic_signature", cfg.PlanAccessEnabled, false)
	portalAccess := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "client_portal", cfg.PlanAccessEnabled, false)
	contractsRouter := func(c *gin.Context) {
		path := c.Param("path")
		if strings.HasSuffix(path, "/security-deposit-balance") {
			pxy.ReverseProxy(cfg.FinancialServiceURL)(c)
			return
		}
		if c.Request.Method == "POST" && (strings.HasSuffix(path, "/send-for-signature") || strings.HasSuffix(path, "/sign")) {
			signatureAccess(c)
			if c.IsAborted() {
				return
			}
		}
		pxy.ReverseProxy(cfg.DealServiceURL)(c)
	}

	dealsProxy := pxy.ReverseProxy(cfg.DealServiceURL)
	r.Any("/api/v1/deals", dealsProxy)
	r.Any("/api/v1/deals/*path", func(c *gin.Context) {
		if c.Request.Method == "POST" && strings.HasSuffix(c.Param("path"), "/invite") {
			portalAccess(c)
			if c.IsAborted() {
				return
			}
		}
		dealsProxy(c)
	})
	r.Any("/api/v1/contracts", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/contracts/*path", contractsRouter)
	// P0/C10: as rotas /api/v1/deal/internal são service-to-service e NÃO são
	// publicadas na borda. Antes, estes dois registros — somados ao repasse de
	// headers sem filtro — permitiam ler deals, contratos e bytes de documentos
	// de qualquer tenant com a INTERNAL_API_KEY, pela internet. O tráfego
	// interno legítimo usa DEAL_SERVICE_URL direto no serviço. Vale também a
	// denylist global em middleware.RejectInternalPaths.
	r.Any("/api/v1/contract-templates", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/contract-templates/*path", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/checklist-templates", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/checklist-templates/*path", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/documents", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/documents/*path", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/portal", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/portal/*path", pxy.ReverseProxy(cfg.DealServiceURL))

	clientPortalRouter := func(c *gin.Context) {
		switch clientPortalService(c.Param("path")) {
		case clientPortalFinancial:
			pxy.ReverseProxy(cfg.FinancialServiceURL)(c)
		case clientPortalCRM:
			pxy.ReverseProxy(cfg.CRMServiceURL)(c)
		default:
			pxy.ReverseProxy(cfg.DealServiceURL)(c)
		}
	}
	r.Any("/api/v1/client-portal", pxy.ReverseProxy(cfg.DealServiceURL))
	r.Any("/api/v1/client-portal/*path", clientPortalRouter)
}

// Serviço que atende um caminho de `/api/v1/client-portal`. O roteamento é uma
// decisão pura (e testável) porque escolher o serviço errado não dá erro de
// compilação — dá 404 na tela do cliente.
type clientPortalServiceName string

const (
	clientPortalFinancial clientPortalServiceName = "financial"
	clientPortalCRM       clientPortalServiceName = "crm"
	clientPortalDeal      clientPortalServiceName = "deal"
)

// clientPortalService mapeia o caminho (depois de `/api/v1/client-portal`).
//
//   - financeiro: cobranças, repasses, informes, extratos, locações;
//   - crm: overview do lead, vitrine de imóveis das imobiliárias do cliente
//     (`/properties`), visitas e canal por lead;
//   - negócio: todo o resto (`/deals/...`, incluindo a busca
//     `/deals/:id/properties`), que é o dono do agregado do negócio.
func clientPortalService(path string) clientPortalServiceName {
	switch {
	case strings.HasPrefix(path, "/rentals"),
		strings.HasPrefix(path, "/transfers"),
		strings.HasPrefix(path, "/charges"),
		strings.HasPrefix(path, "/informes"),
		strings.HasPrefix(path, "/owner-statements"):
		return clientPortalFinancial
	case strings.HasPrefix(path, "/overview"),
		strings.HasPrefix(path, "/properties"):
		return clientPortalCRM
	case strings.HasPrefix(path, "/leads/") &&
		(strings.HasSuffix(path, "/visit-request") ||
			strings.HasSuffix(path, "/visit") ||
			strings.HasSuffix(path, "/visit/calendar.ics") ||
			strings.HasSuffix(path, "/channel")):
		return clientPortalCRM
	}
	return clientPortalDeal
}

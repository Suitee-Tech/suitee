package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterFinancial(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	reportsAccess := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "reports", cfg.PlanAccessEnabled, false)
	r.POST("/webhooks/abacatepay", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/commissions", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/commissions/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/invoices", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/invoices/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/transactions", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/transactions/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/charges", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/charges/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/rental-charge-configs", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/rental-charge-configs/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/rentals", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/rentals/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/transfers", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/transfers/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/collections", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/collections/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/delinquency", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/delinquency/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/fiscal", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/fiscal/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/reminder-rules", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/reminder-rules/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/sale-billing-plans", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/sale-billing-plans/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/payout-orders", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/payout-orders/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/utility-accounts", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/utility-accounts/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/withdrawals", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/withdrawals/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/managed-bills", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/managed-bills/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	// Tributos retidos a recolher (IRRF/INSS/ISS retidos da comissão do corretor):
	// lista com totais, detalhe, baixa com comprovante e recomputação.
	// Adiantamentos da imobiliária ao corretor: saldo a recuperar e concessão.
	r.Any("/api/v1/broker-advances", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/broker-advances/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/tax-obligations", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/tax-obligations/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/reports", reportsAccess, pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/reports/*path", reportsAccess, pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/platform-subscription", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/platform-subscription/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/financial-settings", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/financial-settings/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/commission-rules", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/commission-rules/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/proofs", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/proofs/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/owner-statements", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/owner-statements/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/financial-preview", pxy.ReverseProxy(cfg.FinancialServiceURL))
	r.Any("/api/v1/financial-preview/*path", pxy.ReverseProxy(cfg.FinancialServiceURL))
}

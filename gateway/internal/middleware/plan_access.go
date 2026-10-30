package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oginoino/toca/pkg/logger"
)

const planAccessContextKey = "platformSubscriptionAccess"

type planAccessEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		ControlEnabled bool   `json:"access_control_enabled"`
		Entitled       bool   `json:"entitled"`
		Status         string `json:"status"`
		Reason         string `json:"reason"`
		Plan           *struct {
			Tier string `json:"tier"`
		} `json:"plan"`
		Features []string `json:"features"`
	} `json:"data"`
}

var planAccessClient = &http.Client{Timeout: 4 * time.Second}

// errPlanAccessNotApplicable indica que o serviço de assinatura recusou o
// PRÓPRIO token do chamador (401/403). O plano é uma restrição adicional da
// equipe da imobiliária: quem não é dessa equipe — o cliente no portal, por
// exemplo — não tem regra de plano a aplicar, e a autorização continua sendo do
// serviço de destino. Sem esta distinção, o portal do cliente recebia 403 ao
// gravar as próprias preferências (PATCH /settings/preferences) porque o lookup
// do plano usa o token do cliente e o endpoint de assinatura é da equipe.
var errPlanAccessNotApplicable = errors.New("plan access does not apply to this caller")

// errPlanAccessUnavailable marca falha de INFRAESTRUTURA ao consultar o
// serviço de assinatura: timeout, conexão recusada, resposta não-2xx ou
// envelope inutilizável. É diferente de uma decisão de plano (402 sem
// assinatura, 403 sem o recurso) — nesse caso o envelope chegou e a resposta é
// definitiva.
//
// MOTIVAÇÃO (incidente dos relatórios financeiros, out/2026): os relatórios
// devolviam 503 em beta e produção. A consulta de plano é uma chamada HTTP
// interna; qualquer resposta que não fosse 2xx virava "não foi possível
// confirmar" e derrubava a tela. Uma tela de LEITURA ficava refém do serviço de
// billing. Ver requirePlanOrFallbackOnRead.
var errPlanAccessUnavailable = errors.New("plan access unavailable")

// requirePlanOrFallbackOnRead decide o que fazer quando a consulta de plano
// falha por infraestrutura.
//
// Leitura (GET/HEAD) segue para o serviço de destino, que revalida a permissão
// do recurso: um relatório é leitura e não deve ser bloqueado por indisponi-
// bilidade do serviço de assinatura. Requisição que grava continua fechada —
// liberar escrita sem confirmação de plano seria burlar o bloqueio de
// assinatura, que é a razão de o gate existir.
//
// A permissão continua sendo exigida nos dois casos: a permissão é do serviço
// de destino (toca-financial exige reports:read/finance:read para os
// relatórios), e é isso que impede que esta liberação vire acesso indevido. Por
// isso o bypass emite log de aviso com o motivo — a liberação é observável.
func requirePlanOrFallbackOnRead(c *gin.Context, feature, reason string, err error) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		logger.Warn("plan access check unavailable; allowing read to destination permission check",
			"event", "gateway.plan_access.degraded_read",
			"feature", feature,
			"reason", reason,
			"error", err.Error(),
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"tenant_id", c.GetHeader("X-Tenant-Id"),
			"request_id", c.GetHeader("X-Request-ID"),
		)
		c.Next()
		return
	}
	writePlanAccessError(c, http.StatusServiceUnavailable)
}

// RequireActiveSubscription enforces a write lock after the paid period or
// first-access trial ends, while preserving reads so the tenant can retrieve
// existing data and choose a plan. Plan-management and session routes stay open.
func RequireActiveSubscription(financialServiceURL string, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled || c.Request.Method == http.MethodOptions || skipPlanAccess(c.Request.URL.Path) || strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Next()
			return
		}
		// Read access is preserved after expiry. Feature-specific gates still
		// resolve entitlements for Pro-only reads, while ordinary reads avoid an
		// extra network hop to toca-financial.
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		access, status, err := resolvePlanAccess(c, financialServiceURL)
		if errors.Is(err, errPlanAccessNotApplicable) {
			// Não é a equipe da imobiliária (portal do cliente): o bloqueio de
			// gravação por assinatura não se aplica.
			c.Next()
			return
		}
		if errors.Is(err, errPlanAccessUnavailable) {
			// Falha de infraestrutura: leitura segue para a permissão do
			// destino, escrita continua bloqueada.
			requirePlanOrFallbackOnRead(c, "subscription", "plan_access_unavailable", err)
			return
		}
		if err != nil || access == nil {
			writePlanAccessError(c, status)
			return
		}
		if !access.Data.ControlEnabled || access.Data.Entitled {
			c.Next()
			return
		}
		writePlanRequired(c, "subscription", access.Data.Plan, access.Data.Reason)
	}
}

// RequirePlanFeature asks toca-financial for the tenant-scoped entitlement
// decision. Financial validates the original user token and tenant context;
// the gateway never trusts a plan or feature list supplied by the browser.
func RequirePlanFeature(financialServiceURL, feature string, enabled, mutatingOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled || c.Request.Method == http.MethodOptions || (mutatingOnly && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead)) {
			c.Next()
			return
		}
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			// Leave authentication errors to the destination service.
			c.Next()
			return
		}
		access, status, err := resolvePlanAccess(c, financialServiceURL)
		if errors.Is(err, errPlanAccessNotApplicable) {
			// Mesma regra do bloqueio de gravação: quem não é da equipe resolve a
			// autorização no serviço de destino (permissão do recurso).
			c.Next()
			return
		}
		if err != nil || access == nil {
			if errors.Is(err, errPlanAccessUnavailable) {
				// Este é o caso do incidente dos relatórios: o serviço de
				// assinatura respondeu algo que não dá para confirmar (ex.: 500)
				// e uma tela de leitura devolvia 503. Leitura segue; escrita
				// continua fechada; a permissão do recurso é revalidada no
				// destino.
				requirePlanOrFallbackOnRead(c, feature, "plan_access_unavailable", err)
				return
			}
			writePlanAccessError(c, status)
			return
		}
		if !access.Data.ControlEnabled {
			c.Next()
			return
		}
		if !access.Data.Entitled {
			writePlanRequired(c, feature, access.Data.Plan, access.Data.Reason)
			return
		}
		for _, entitledFeature := range access.Data.Features {
			if entitledFeature == feature {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "plan_upgrade_required",
			"message": "Este recurso está disponível no plano Pro.",
			"data": gin.H{
				"feature":     feature,
				"plan":        access.Data.Plan,
				"upgrade_url": "/assinatura",
			},
		})
	}
}

func resolvePlanAccess(c *gin.Context, financialServiceURL string) (*planAccessEnvelope, int, error) {
	if cached, ok := c.Get(planAccessContextKey); ok {
		if access, ok := cached.(*planAccessEnvelope); ok {
			return access, http.StatusOK, nil
		}
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, strings.TrimRight(financialServiceURL, "/")+"/api/v1/platform-subscription/access", nil)
	if err != nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("%w: %v", errPlanAccessUnavailable, err)
	}
	request.Header.Set("Authorization", c.GetHeader("Authorization"))
	for _, header := range []string{"X-Tenant-Id", "X-Auth-Version", "X-Request-ID", "Cookie"} {
		if value := c.GetHeader(header); value != "" {
			request.Header.Set(header, value)
		}
	}
	response, err := planAccessClient.Do(request)
	if err != nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("%w: %v", errPlanAccessUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		// O endpoint de acesso é da equipe (rejeita o papel `cliente`) e exige
		// token válido. Recusa aqui significa "esta regra não é para este
		// chamador", não "bloqueie": o destino segue validando autenticação e
		// permissão, e um token expirado continua recebendo 401 de lá — com o
		// refresh do frontend funcionando igual.
		return nil, response.StatusCode, errPlanAccessNotApplicable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Um erro do serviço de assinatura não pode virar decisão de acesso:
		// sem envelope confiável a resposta é "não foi possível confirmar".
		// A mensagem carrega o status real do upstream — antes ele só existia
		// no log, e diagnosticar o 503 exigia entrar no host.
		return nil, http.StatusServiceUnavailable, fmt.Errorf("%w: plan access responded %d", errPlanAccessUnavailable, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("%w: %v", errPlanAccessUnavailable, err)
	}
	var envelope planAccessEnvelope
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("%w: plan access returned an unusable envelope", errPlanAccessUnavailable)
	}
	c.Set(planAccessContextKey, &envelope)
	return &envelope, http.StatusOK, nil
}

func skipPlanAccess(path string) bool {
	for _, prefix := range []string{
		"/api/v1/auth",
		"/api/v1/health",
		"/api/v1/me/permissions",
		"/api/v1/platform-subscription",
		"/api/v1/client-portal",
		"/api/v1/portal",
	} {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func writePlanRequired(c *gin.Context, feature string, plan any, reason string) {
	// A mensagem precisa dizer o que fazer em cada motivo. "Seu período de
	// acesso terminou" para quem nunca assinou (ou está com checkout pendente)
	// descreve um período que não existiu — foi o que a conta de produção viu ao
	// ligar o controle de acesso por plano.
	message := "Seu período de acesso terminou. Escolha um plano para continuar."
	switch reason {
	case "payment_required":
		message = "Regularize a assinatura para continuar usando os recursos de gravação."
	case "subscription_required":
		message = "Escolha um plano para liberar a criação e a edição de dados."
	}
	c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
		"success": false,
		"code":    "plan_required",
		"message": message,
		"data": gin.H{
			"feature":     feature,
			"plan":        plan,
			"upgrade_url": "/assinatura",
		},
	})
}

func writePlanAccessError(c *gin.Context, status int) {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		c.AbortWithStatus(status)
		return
	}
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
		"success": false,
		"code":    "plan_access_unavailable",
		"message": "Não foi possível confirmar o acesso do plano. Tente novamente.",
		"data":    nil,
	})
}

package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// P2/M4 — negação de autorização com trilha.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado M4) mostrou que falhas de autorização saíam como 500/400 em alguns
// handlers e, mesmo quando saíam como 403, não deixavam trilha: não havia como
// responder "quem tentou o quê e foi barrado".
//
// denyAuthz é o ponto único: registra a negação (log + audit.events) e responde
// 403. O Entry é pequeno de propósito — uma negação repetida não pode inflar a
// tabela de auditoria (retenção de 5 anos); a rota, o método e a permissão
// exigida são o que interessa para forense.
// recordAuthzDenial registra a negação (log + audit.events) SEM responder.
//
// N1 (reauditoria de 2026-09-20): os guardas LEGADOS (`RequireFunctionalRole`,
// `RejectFunctionalRole` e `RequireRole`) respondiam 403 sem deixar trilha — só
// a família `RequirePermission` chamava `denyAuthz`. A consequência era
// concreta: a fronteira do P0/A11 (papel `cliente` barrado na superfície de
// operador) era invisível na auditoria. A tabela do ambiente local tinha apenas
// `no functional role assigned`, e nenhuma negação de `cliente`.
//
// Estes guardas têm mensagem própria, então precisam registrar sem responder —
// é o que esta função permite; `denyAuthz` continua sendo o caminho de quem não
// tem mensagem específica.
func recordAuthzDenial(c *gin.Context, reason, requiredPermission string) {
	tenantID := c.GetString("tenant_id")
	actorID := c.GetString("user_id")
	role := c.GetString("functional_role")
	path := ""
	method := ""
	if c.Request != nil {
		path = c.Request.URL.Path
		method = c.Request.Method
	}

	logger.Warn("authz.denied",
		"event", "authz.denied",
		"reason", reason,
		"permission", requiredPermission,
		"tenant_id", tenantID,
		"actor_id", actorID,
		"functional_role", role,
		"path", path,
		"method", method,
		"ip", c.ClientIP(),
	)

	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "authz.denied",
		EntityType: "http_route",
		EntityID:   method + " " + path,
		Metadata: map[string]any{
			"reason":          reason,
			"required":        requiredPermission,
			"functional_role": role,
			"actor_id":        actorID,
		},
	})
}

// denyAuthz é o ponto único para quem não tem mensagem própria: registra a
// negação e responde 403.
func denyAuthz(c *gin.Context, reason, requiredPermission string) {
	recordAuthzDenial(c, reason, requiredPermission)
	response.Forbidden(c, "permission denied: insufficient permissions")
}

// DenyGinRequest é a versão pública de denyAuthz para handlers que detectam a
// negação fora de um middleware de permissão (ex.: o serviço devolveu
// "unauthorized"). Registra a trilha e responde 403.
func DenyGinRequest(c *gin.Context, reason, requiredPermission string) {
	denyAuthz(c, reason, requiredPermission)
}

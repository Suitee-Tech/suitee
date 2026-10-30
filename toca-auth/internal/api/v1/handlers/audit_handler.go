package handlers

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
)

// AuditHandler expõe o GET global de audit trail. Issue #470.
//
// Endpoint pensado para o perfil "auditor" e operações administrativas
// (incident response, forensics, compliance LGPD Art. 37/38). Gate de
// segurança é o middleware `system:view_audit_trail` — quem tem essa
// permission ganha acesso a tudo via filtros.
//
// Por que toca-auth e não toca-deal ou um service novo?
//   - toca-auth já tem audit.Recorder wirado e é a superfície natural
//     de admin (login_failed, banking_updated, invite_created).
//   - Criar um "toca-audit" service exclusivo agregaria operacional
//     (deploy + DB + RabbitMQ) sem ganho nenhum — o pkg/audit já
//     concentra o acesso ao schema audit.events.
//   - toca-deal está mais ligado à esteira de negócios; misturar GET
//     cross-domain ali polui o serviço.

type AuditHandler struct{}

func NewAuditHandler() *AuditHandler {
	return &AuditHandler{}
}

// ListAuditTrail godoc
// @Summary      Audit trail global (cross-domain)
// @Description  Lista de eventos de audit com filtros. Cross-tenant
//
//	quando o tenant_id query param é vazio (use só com
//	perfil auditor). Ordem: created_at DESC, paginação por
//	cursor via `created_before`.
//
// @Tags         audit
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        tenant_id      query string false "Filtro de tenant. Vazio = cross-tenant."
// @Param        action         query string false "Action exata (ex.: auth.banking_updated)."
// @Param        action_prefix  query string false "Prefix da action (ex.: auth.)."
// @Param        entity_type    query string false "Tipo de entidade (ex.: User, Deal)."
// @Param        entity_id      query string false "ID exato de entidade."
// @Param        actor_id       query string false "UUID do ator (quem fez a ação)."
// @Param        from           query string false "RFC3339 lower bound de created_at."
// @Param        to             query string false "RFC3339 upper bound de created_at."
// @Param        created_before query string false "Cursor: created_at do último item da página anterior."
// @Param        limit          query int    false "1..500 (default 100)."
// @Success      200 {object} response.Response{data=[]audit.LogEntry}
// @Failure      403 {object} response.Response
// @Router       /audit-trail [get]
// ErrCrossTenantAuditDenied é devolvido quando um principal de tenant tenta ler
// a trilha de outro tenant (ou sem tenant algum).
var ErrCrossTenantAuditDenied = errors.New("cross-tenant audit trail requires an internal service credential")

// auditFiltersFromRequest monta os filtros do GET /audit-trail decidindo o
// ESCOPO DE TENANT no servidor.
//
// SECURITY (achado A4 da revisão de permissões de 2026-09-19): antes o
// `tenant_id` do query string era repassado cru ao `pkg/audit.List` e, quando
// omitido, a consulta devolvia TODOS os tenants — o próprio comentário do código
// admitia ("se mandar tenant_id vazio, devolve cross-tenant mesmo"). Como o gate
// `system:view_audit_trail` inclui papéis DE TENANT (`auditor`, `operacoes`),
// qualquer auditor lia eventos de outros tenants, inclusive `auth.banking_updated`
// e `lgpd.*`.
//
// Regra nova:
//   - principal de tenant (JWT): o tenant do token é OBRIGATÓRIO; pedir outro
//     tenant (ou omitir) é 403;
//   - principal de serviço (X-Internal-Key, `type=service`): pode escolher o
//     tenant, mas precisa dizer qual (sem tenant explícito é 403) — é o caminho
//     do auditor de plataforma/forense.
func auditFiltersFromRequest(c *gin.Context) (audit.ListFilters, error) {
	f := audit.ListFilters{
		Action:        strings.TrimSpace(c.Query("action")),
		ActionPrefix:  strings.TrimSpace(c.Query("action_prefix")),
		EntityType:    strings.TrimSpace(c.Query("entity_type")),
		EntityID:      strings.TrimSpace(c.Query("entity_id")),
		ActorID:       strings.TrimSpace(c.Query("actor_id")),
		From:          strings.TrimSpace(c.Query("from")),
		To:            strings.TrimSpace(c.Query("to")),
		CreatedBefore: strings.TrimSpace(c.Query("created_before")),
	}
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			f.Limit = n
		}
	}

	requested := strings.TrimSpace(c.Query("tenant_id"))
	ctxTenant, _ := pkgcontext.GetTenantID(c.Request.Context())
	isInternalService := c.GetString("type") == "service" || c.GetString("functional_role") == "service"

	switch {
	case isInternalService:
		if requested == "" {
			return f, ErrCrossTenantAuditDenied
		}
		f.TenantID = requested
	case ctxTenant == "":
		return f, ErrCrossTenantAuditDenied
	case requested == "" || requested == ctxTenant:
		f.TenantID = ctxTenant
	default:
		return f, ErrCrossTenantAuditDenied
	}
	return f, nil
}

func (h *AuditHandler) ListAuditTrail(c *gin.Context) {
	// O gate de permission na rota (`system:view_audit_trail`) autoriza a
	// LEITURA; o escopo de tenant é decidido aqui (ver auditFiltersFromRequest).
	f, err := auditFiltersFromRequest(c)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}

	entries, err := audit.List(c.Request.Context(), f)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// Audit do próprio acesso ao audit (meta-audit). Garante que
	// inspeções administrativas ficam rastreáveis — quem auditou o
	// auditor? — pelo mesmo schema que estamos consultando.
	actorID, _ := pkgcontext.GetAccountID(c.Request.Context())
	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "audit.trail_queried",
		EntityType: "AuditTrail",
		EntityID:   actorID,
		Metadata: map[string]any{
			"filters":      filtersForLog(f),
			"result_count": len(entries),
		},
	})

	response.Success(c, "Audit trail retrieved", entries)
}

// filtersForLog produz um snapshot serializável dos filtros sem
// expor tokens/IDs sensíveis. Mantém só presença/valor dos filtros
// (não há PII em ListFilters mesmo, mas centraliza a forma).
func filtersForLog(f audit.ListFilters) map[string]any {
	return map[string]any{
		"tenant_id":      f.TenantID,
		"action":         f.Action,
		"action_prefix":  f.ActionPrefix,
		"entity_type":    f.EntityType,
		"entity_id":      f.EntityID,
		"actor_id":       f.ActorID,
		"from":           f.From,
		"to":             f.To,
		"created_before": f.CreatedBefore,
		"limit":          f.Limit,
	}
}

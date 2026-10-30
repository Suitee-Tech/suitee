package handlers

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// ctxNotificationRecipientDomain guarda o domínio do destinatário para o log.
const ctxNotificationRecipientDomain = "notification_recipient_domain"

// internalTenant resolve o tenant efetivo de uma rota interna.
//
// SECURITY (achado A3 da revisão de permissões de 2026-09-19): antes o tenant
// vinha cru do `?tenant_id=` do chamador. Agora:
//   - chamada com JWT de usuário → tenant do token (pedir outro é 403);
//   - chamada de serviço → tenant explícito, restrito à allowlist da credencial
//     quando INTERNAL_SERVICE_KEYS define uma.
func internalTenant(c *gin.Context) (string, bool) {
	return internalTenantRequested(c, c.Query("tenant_id"))
}

// internalTenantRequested é o mesmo resolver para as rotas internas em que o
// tenant chega no CORPO da requisição em vez da query string (ex.:
// `POST /comm/internal/channels/ensure` recebe `tenant_id` no JSON): o handler
// passa o valor recebido e usa o retorno, exatamente como faz com `?tenant_id=`.
func internalTenantRequested(c *gin.Context, requested string) (string, bool) {
	tenantID, err := auth.ResolveTenantForInternalRequest(c, requested)
	if err != nil {
		respondInternalTenantError(c, err)
		return "", false
	}
	return tenantID, true
}

// respondInternalTenantError traduz os erros do resolver de tenant interno para
// HTTP: cross-tenant é 403 (o chamador está autenticado, mas pediu outro
// tenant), tenant ausente é 400 (falta de parâmetro) e falta de credencial é 401.
func respondInternalTenantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInternalCrossTenant):
		response.Forbidden(c, err.Error())
	case errors.Is(err, auth.ErrInternalTenantRequired):
		response.ValidationError(c, err.Error())
	default:
		response.Unauthorized(c, err.Error())
	}
}

// resolveSendNotificationTenant decide a atribuição de uma notificação enviada
// por rota interna (SECURITY — achado A3 da revisão de permissões de
// 2026-09-19: `POST /notifications/send` era um primitivo de envio arbitrário
// alcançável pela borda).
//
//   - tenant informado no corpo → passa pelo resolver: JWT manda o tenant do
//     token (pedir outro é 403) e credencial de serviço fica restrita à
//     allowlist da chave;
//   - sem tenant no corpo mas com JWT → vale o tenant do token;
//   - sem tenant e com credencial de serviço → mensagem de SISTEMA, permitida e
//     registrada no log. É o caso do e-mail de recuperação de senha: quem pede
//     ainda não tem tenant, e recusar aqui quebraria a recuperação de conta.
func resolveSendNotificationTenant(c *gin.Context, requested string) (string, bool) {
	if strings.TrimSpace(requested) != "" {
		return internalTenantRequested(c, requested)
	}

	switch c.GetString(auth.CtxInternalAuthKind) {
	case auth.InternalAuthUser:
		return internalTenantRequested(c, "")
	case auth.InternalAuthService:
		// Sem tenant: só a credencial de serviço chega aqui (o middleware já
		// recusou quem não tem chave nem JWT). Classificar como sistema mantém a
		// trilha de auditoria honesta em vez de inventar um tenant.
		logger.Info("comm.notification_send_system",
			"event", "comm.notification_send_system",
			"service", c.GetString(auth.CtxInternalServiceName),
			"legacy_key", c.GetBool(auth.CtxInternalLegacyKey),
			"recipient_domain", recipientDomain(c),
			"path", c.Request.URL.Path)
		return "", true
	default:
		respondInternalTenantError(c, auth.ErrInternalUnauthenticated)
		return "", false
	}
}

// recipientDomain devolve só o domínio do destinatário — o log de auditoria não
// precisa (e não deve) guardar o e-mail completo de quem recebeu a mensagem.
func recipientDomain(c *gin.Context) string {
	// O corpo já foi consumido pelo binder; o domínio é extraído no handler.
	return c.GetString(ctxNotificationRecipientDomain)
}

// resolveParticipantListingScope decide o escopo da listagem por participante
// (`GET /comm/internal/channels?participant=`) a partir da credencial.
//
// SECURITY (achados A3/A9 da revisão de permissões de 2026-09-19).
//
// Retorna (tenantFilter, ok). tenantFilter vazio significa "sem recorte de
// tenant", e só é devolvido para chamador com JWT pedindo os PRÓPRIOS canais —
// caso do cliente do portal.
func resolveParticipantListingScope(c *gin.Context, requestedParticipant string) (string, bool) {
	switch c.GetString(auth.CtxInternalAuthKind) {
	case auth.InternalAuthUser:
		tokenAccount := strings.TrimSpace(c.GetString("account_id"))
		if tokenAccount == "" {
			response.Unauthorized(c, "token does not carry an account id")
			return "", false
		}
		if requestedParticipant != tokenAccount {
			// Um usuário só lista os próprios canais; pedir os de outro é 403 —
			// é isto que torna seguro não filtrar por tenant aqui.
			response.Forbidden(c, "participant does not match the authenticated account")
			return "", false
		}
		return "", true

	case auth.InternalAuthService:
		tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
		if err != nil {
			respondInternalTenantError(c, err)
			return "", false
		}
		return tenantID, true

	default:
		respondInternalTenantError(c, auth.ErrInternalUnauthenticated)
		return "", false
	}
}

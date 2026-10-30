package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
)

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
// `POST /ops/internal/tickets` recebe `tenant_id` no JSON): o handler passa o
// valor recebido e usa o retorno, exatamente como faz com `?tenant_id=`.
func internalTenantRequested(c *gin.Context, requested string) (string, bool) {
	tenantID, err := auth.ResolveTenantForInternalRequest(c, requested)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInternalCrossTenant):
			response.Forbidden(c, err.Error())
		case errors.Is(err, auth.ErrInternalTenantRequired):
			response.ValidationError(c, err.Error())
		default:
			response.Unauthorized(c, err.Error())
		}
		return "", false
	}
	return tenantID, true
}

// tenantForRequest resolve o tenant dos handlers que servem, ao mesmo tempo, uma
// rota interna e uma rota de usuário (`/ops/internal/*` e `/ops/*`): na rota
// interna vale a credencial (InternalOrUserMiddleware marca o tipo em
// `auth.CtxInternalAuthKind`); na rota de usuário o tenant já vem do token no
// request context, como antes.
func tenantForRequest(c *gin.Context, requested string) (string, bool) {
	if c.GetString(auth.CtxInternalAuthKind) != "" {
		return internalTenantRequested(c, requested)
	}
	tenantID, err := appctx.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return "", false
	}
	return tenantID, true
}

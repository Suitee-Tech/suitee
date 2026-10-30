package handlers

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
)

// portalScopeResolver é implementado pelo repositório cross-tenant.
type portalScopeResolver interface {
	LeadScopesForAccount(ctx context.Context, accountID, email string) ([]repository.PortalLeadScope, error)
}

// portalServiceContext prepara as rotas do PORTAL DO CLIENTE no financeiro.
//
// Dois problemas resolvidos de uma vez:
//
//  1. TENANT: as rotas usavam `context.GetTenantID` — que para um cliente OTP é
//     a conta local DELE, enquanto locações, cobranças, repasses e informes são
//     do tenant da IMOBILIÁRIA. Aqui os vínculos do cliente (leads por conta ou
//     e-mail) são resolvidos no pool de serviço e o tenant do vínculo entra no
//     contexto. Sem vínculo, nada muda (as rotas seguem devolvendo vazio).
//  2. RLS: o cliente é cross-tenant e as leituras filtravam tudo no pool de
//     aplicação; a rota roda numa transação do pool de serviço, com a
//     autorização feita pelos próprios handlers (account_id/e-mail do token ×
//     leads do cliente).
func portalServiceContext(runner *database.PostgresDB, crossRepo repository.CrossTenantRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		resolver, canResolve := crossRepo.(portalScopeResolver)
		if runner == nil || !canResolve {
			c.Next()
			return
		}

		accountID, _ := pkgcontext.GetAccountID(ctx)
		email, _ := pkgcontext.GetEmail(ctx)
		if scopes, err := resolver.LeadScopesForAccount(ctx, accountID, email); err == nil && len(scopes) > 0 {
			ctx = pkgcontext.WithTenantID(ctx, scopes[0].TenantID)
			c.Request = c.Request.WithContext(ctx)
		} else if err != nil {
			logger.Warn("portal scopes lookup failed", "error", err.Error())
		}

		err := runner.RunInServiceContext(c.Request.Context(), func(serviceCtx context.Context) error {
			c.Request = c.Request.WithContext(serviceCtx)
			c.Next()
			return nil
		})
		switch {
		case err == nil:
			return
		case errors.Is(err, database.ErrServiceRoleRequired):
			logger.Warn("client portal running without service pool", "path", c.Request.URL.Path)
			if !c.Writer.Written() {
				c.Next()
			}
		default:
			logger.Error("client portal service transaction failed", map[string]interface{}{
				"error": err.Error(), "path": c.Request.URL.Path,
			})
			if !c.Writer.Written() {
				response.InternalError(c, err)
			}
		}
	}
}

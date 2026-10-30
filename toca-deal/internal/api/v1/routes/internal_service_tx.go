package routes

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// serviceTxRunner é a parte do PostgresDB usada aqui (interface para o teste
// injetar um dublê sem banco).
type serviceTxRunner interface {
	RunInServiceContext(ctx context.Context, fn func(context.Context) error) error
}

// internalServiceTx roda as rotas INTERNAS (serviço-a-serviço) na transação do
// pool de serviço (BYPASSRLS).
//
// Por que: uma chamada interna chega com a credencial de serviço e SEM claims de
// JWT. A RLS de `deal.contracts`/`deal.deals` filtra por
// `tenant_id = jwt.app_metadata.tenant_id` — sem claims, o pool de aplicação não
// enxerga nenhuma linha. Medido no beta: o preview financeiro recebia
// "contract not found: sql: no rows" para um contrato que existe (tenant
// bc46e19a…), devolvendo 500 na tela de Propostas > Financeiro (Prévia).
//
// A autorização continua onde estava: `auth.RequireInternalKey()` valida a
// credencial e `auth.ResolveTenantForInternalRequest` resolve/limita o tenant
// pela credencial (allowlist), então o pool de serviço só remove o filtro de
// tenant que não se aplica a este ator.
func internalServiceTx(runner serviceTxRunner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if runner == nil {
			c.Next()
			return
		}
		err := runner.RunInServiceContext(c.Request.Context(), func(ctx context.Context) error {
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return nil
		})
		switch {
		case err == nil:
			return
		case errors.Is(err, database.ErrServiceRoleRequired):
			logger.Warn("internal route running without service pool; RLS will hide cross-tenant rows",
				"path", c.Request.URL.Path)
			if !c.Writer.Written() {
				c.Next()
			}
		default:
			logger.Error("internal service transaction failed", map[string]interface{}{
				"error": err.Error(),
				"path":  c.Request.URL.Path,
			})
			if !c.Writer.Written() {
				response.InternalError(c, err)
			}
		}
	}
}

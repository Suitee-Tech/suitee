package handlers

import (
	"github.com/gin-gonic/gin"
	pkgauth "github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P1/A25 — escopo por corretor nos sub-recursos do negócio.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A25) mostrou que alguns handlers de sub-recurso (reconcile-signers,
// contratos do negócio, contrato por id e documentos do contrato) só filtravam
// por tenant: dentro do tenant, um `corretor` alcançava negócio de outro corretor.
//
// A regra de escopo é a mesma dos 30+ endpoints vizinhos (scopedBrokerID): quem
// tem visão de tenant não tem recorte; qualquer outro papel é recortado pelo
// próprio account id. Aqui ela fica numa definição só, para que os handlers de
// contrato/documento não reimplementem a decisão.
//
// A decisão vem de `pkg/auth.HasTenantWideScope`, que olha PERFIL **e** PAPEL
// FUNCIONAL. Antes deste ajuste (incidente de 2026-09-21) o `toca-deal` olhava
// só o perfil (`admin/owner/manager`): atribuir o Grupo de Usuários "Gestor
// Comercial" grava `functional_role = gestor` mas deixa o perfil do vínculo
// como `member`, então o gestor recém-promovido continuava recortado pela
// própria carteira — Esteira vazia, inclusive para negócios criados antes da
// promoção. O CRM já decidia olhando os dois eixos; agora os dois concordam.
func callerBrokerScope(c *gin.Context) string {
	if pkgauth.HasTenantWideScope(
		mustUserPermission(c),
		mustFunctionalRole(c),
		mustBaseRole(c),
	) {
		return ""
	}
	accountID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil {
		return ""
	}
	return accountID
}

func mustUserPermission(c *gin.Context) string {
	profile, _ := pkgcontext.GetUserPermission(c.Request.Context())
	return profile
}

func mustFunctionalRole(c *gin.Context) string {
	role, _ := pkgcontext.GetFunctionalRole(c.Request.Context())
	return role
}

// mustBaseRole devolve o papel base do perfil de acesso ("" quando o papel
// funcional já é do catálogo). Sem ele, um perfil personalizado baseado em
// gestor/admin ficaria recortado pela própria carteira.
func mustBaseRole(c *gin.Context) string {
	role, _ := pkgcontext.GetBaseRole(c.Request.Context())
	return role
}

package database

import (
	"os"
	"strings"

	"github.com/oginoino/toca/pkg/config"
)

// SECURITY (achado M1 da revisão de permissões de 2026-09-19): o RLS não
// protegia os serviços.
//
// Os dois pools do pkg/database têm papéis opostos:
//
//   - pool de APLICAÇÃO (`DATABASE_URI`, `PostgresDB.DB`): atende requisição de
//     tenant, é quem instala `request.jwt.claims` e portanto é quem o RLS
//     deveria estar filtrando;
//   - pool de SERVIÇO (`DATABASE_URI_SERVICE`): executa trabalho cross-tenant
//     legítimo e PRECISA de BYPASSRLS (verificado no boot; sem isso o boot
//     falha com ErrServiceRoleRequired).
//
// Na medição do achado, o pool de aplicação conectava com um role que também
// tinha `rolbypassrls = t` — ou seja, as políticas criadas nas migrations eram
// inertes e todo `WHERE tenant_id` esquecido virava vazamento cross-tenant sem
// backstop nenhum. Isso não se conserta só com código: exige provisionar um
// role de aplicação sem BYPASSRLS (e as migrations do P2/M1 já deixaram as
// tabelas de identidade prontas para isso). O que o código faz aqui é tornar o
// estado OBSERVÁVEL e, opcionalmente, fatal:
//
//	REQUIRE_RLS_BACKSTOP=true → boot falha enquanto o pool de aplicação
//	                            bypassar RLS (fail-closed, opt-in).
//
// O default é fail-loud (ERROR no boot em release), porque tornar isso fatal de
// imediato derrubaria stacks que ainda conectam como `postgres`.

// RLSBackstop é o retrato das capacidades de RLS dos dois pools.
type RLSBackstop struct {
	// UserRoleBypassesRLS indica que o pool de aplicação ignora RLS
	// (rolbypassrls ou rolsuper): o backstop está DESLIGADO.
	UserRoleBypassesRLS bool
	// ServiceRoleBypassesRLS indica que o pool de serviço ignora RLS (estado
	// exigido para o trabalho cross-tenant).
	ServiceRoleBypassesRLS bool
	// DedicatedServiceRole indica que DATABASE_URI_SERVICE é distinto de
	// DATABASE_URI (pool separado, e não o mesmo pool reaproveitado).
	DedicatedServiceRole bool
}

// Active informa que o RLS é um backstop real: o pool de aplicação está sujeito
// às políticas.
func (b RLSBackstop) Active() bool { return !b.UserRoleBypassesRLS }

// RLSBackstopDecision é a decisão de boot (pura, testável sem banco).
type RLSBackstopDecision struct {
	// Level é "info" ou "error".
	Level string
	// Event é o nome estruturado do log.
	Event string
	// Message explica o estado e o que fazer.
	Message string
	// Fatal indica que o boot deve falhar (REQUIRE_RLS_BACKSTOP=true).
	Fatal bool
	// Err é o erro devolvido quando Fatal.
	Err error
}

// EvaluateRLSBackstop decide o log/erro a partir do retrato dos pools. É pura
// de propósito: a decisão é a parte que precisa de teste determinístico, sem
// depender de um banco com roles específicos.
func EvaluateRLSBackstop(backstop RLSBackstop) RLSBackstopDecision {
	release := config.IsReleaseEnv(os.Getenv("APP_ENV"), os.Getenv("APP_MODE"))
	require := envBool("REQUIRE_RLS_BACKSTOP")

	if backstop.Active() {
		return RLSBackstopDecision{
			Level: "info",
			Event: "database.rls_backstop_active",
			Message: "pool de aplicação sujeito a RLS (políticas ativas); " +
				"isolamento tem backstop de banco além do WHERE tenant_id",
		}
	}

	if release && require {
		return RLSBackstopDecision{
			Level: "error",
			Event: "database.rls_backstop_required",
			Message: "REQUIRE_RLS_BACKSTOP=true e o pool de aplicação conecta com role que ignora RLS " +
				"(rolbypassrls/rolsuper): provisione o role de aplicação sem BYPASSRLS antes de subir",
			Fatal: true,
			Err:   ErrRLSBackstopRequired,
		}
	}

	message := "pool de aplicação conecta com role que ignora RLS (rolbypassrls/rolsuper): " +
		"as políticas são inertes e o isolamento depende só do WHERE tenant_id. " +
		"Provisione um role sem BYPASSRLS para DATABASE_URI (o pool de serviço mantém o dele) " +
		"e use REQUIRE_RLS_BACKSTOP=true para tornar isso fatal"
	if release {
		return RLSBackstopDecision{Level: "error", Event: "database.rls_backstop_disabled", Message: message}
	}
	return RLSBackstopDecision{Level: "warn", Event: "database.rls_backstop_disabled", Message: message}
}

// RLSBackstop devolve o retrato observado no boot (útil para diagnóstico e
// para que um handler interno possa expor o estado).
func (p *PostgresDB) RLSBackstop() RLSBackstop {
	if p == nil {
		return RLSBackstop{}
	}
	return RLSBackstop{
		UserRoleBypassesRLS:    p.userRoleBypassesRLS,
		ServiceRoleBypassesRLS: p.serviceRoleBypassesRLS,
		DedicatedServiceRole:   p.serviceDB != nil && p.serviceDB != p.DB,
	}
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

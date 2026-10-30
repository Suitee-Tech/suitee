package auth

import "strings"

const (
	FunctionalRoleAdmin = "admin"
)

// Profile constants. `permission` (sometimes called "profile" in the UI)
// is the user's overarching access tier, orthogonal to functional_role.
// Owner/admin are root-level profiles that grant blanket access to every
// permission in the catalog — historically enforced via a hardcoded
// bypass in the middleware. The bypass now lives in the PermissionProvider
// so the catalog is the single source of authorization decisions; this
// constant exists so callers don't sprinkle string literals through the
// codebase.
const (
	ProfileOwner = "owner"
	ProfileAdmin = "admin"
)

// IsRootProfile returns true when the given profile string grants blanket
// access to the permission catalog. Matched case-insensitively because the
// value enters the system from multiple sources (JWT claims, app_metadata,
// internal API headers) with inconsistent casing.
func IsRootProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProfileOwner, ProfileAdmin:
		return true
	}
	return false
}

// AllFunctionalRoles is a list of all functional roles available in the system
var AllFunctionalRoles = []string{
	FunctionalRoleAdmin,
	"gestor",
	"corretor",
	"operacoes",
	"financeiro",
	"marketing",
	"auditor",
	"parceiro",
	"cliente",
}

// RoleLabels is a map of functional role IDs to their display labels in Portuguese
var RoleLabels = map[string]string{
	"admin":      "Administrador",
	"gestor":     "Gestor",
	"corretor":   "Corretor",
	"operacoes":  "Operações",
	"financeiro": "Financeiro",
	"marketing":  "Marketing",
	"auditor":    "Auditor",
	"parceiro":   "Parceiro",
	"cliente":    "Cliente",
}

// IsValidFunctionalRole checks if a given role string is a valid functional role
func IsValidFunctionalRole(r string) bool {
	for _, role := range AllFunctionalRoles {
		if role == r {
			return true
		}
	}
	return false
}

// AccessProfilePrefix é o prefixo reservado dos PERFIS DE ACESSO
// personalizados do tenant (Configurações → Permissões → Perfis
// Personalizados). O prefixo existe para que um id de perfil nunca possa
// colidir com — nem se passar por — um papel do catálogo: `admin` continua
// sendo só `admin`, e nenhum perfil salvo pelo cliente consegue se chamar
// `admin`.
const AccessProfilePrefix = "cr_"

// IsAccessProfileID informa se o id tem a forma de um perfil personalizado.
//
// É uma checagem de FORMA, não de existência: quem decide se o perfil existe —
// e se concede algo — é a matriz do tenant (`RolePermissions.CustomRoles`), que
// é escopada por tenant. Um id de perfil de OUTRA imobiliária não casa com
// perfil nenhum aqui e, portanto, não concede permissão alguma (fail-closed) —
// é isso que torna seguro aceitar o perfil na escrita sem uma consulta
// cross-service no caminho de atribuição.
func IsAccessProfileID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), AccessProfilePrefix) &&
		len(strings.TrimSpace(id)) > len(AccessProfilePrefix)
}

// tenantWideProfiles / tenantWideRoles são os eixos em que um principal vê o
// TENANT INTEIRO (e não só a própria carteira).
//
// Os dois eixos existem porque o mesmo valor chega por caminhos diferentes: o
// EIXO DE PERFIL (`permission`: owner/admin/manager — e, em tokens legados,
// também o nome do papel) e o EIXO DE PAPEL FUNCIONAL (`functional_role`:
// admin/gestor/operacoes). Quem decide visibilidade precisa olhar os DOIS.
var (
	tenantWideProfiles = map[string]bool{
		"admin": true, "owner": true, "manager": true, "gestor": true, "operacoes": true,
	}
	tenantWideRoles = map[string]bool{
		"admin": true, "owner": true, "manager": true, "gestor": true, "operacoes": true,
	}
)

// HasTenantWideScope informa se o principal enxerga o tenant inteiro — o que
// separa "gestão" de "a própria carteira".
//
// `baseRole` é o papel do catálogo de onde um PERFIL DE ACESSO personalizado
// partiu (claim `base_role`). Ele entra como TERCEIRO eixo porque um perfil
// "Gestor Comercial" (base `gestor`) concede as mesmas permissões do gestor, mas
// o id do perfil (`cr_…`) não casa com nenhum papel do catálogo: sem este eixo o
// usuário com perfil personalizado via a Esteira e os leads vazios, mesmo com
// permissão de leitura para tudo (incidente de 2026-09-21 — "o novo
// administrador não vê leads de corretores que deveria ver").
//
// FONTE ÚNICA (incidente de 2026-09-21: "Operações não está vendo nada" e
// "gestor e administrador atribuído depois não tem acesso a leads e esteiras
// criadas anteriormente"): cada serviço tinha a SUA versão da regra e elas
// discordavam. O CRM olhava perfil E papel; o `toca-deal` (negócios, contratos
// e documentos) olhava SÓ o perfil (`admin/owner/manager`). Como atribuir um
// papel pelo Grupo de Usuários grava `functional_role` e deixa o PERFIL do
// vínculo como `member`, quem era promovido a gestor/operações continuava
// recortado pela própria carteira: via a Esteira vazia, contratos de outros
// corretoras invisíveis e nenhum negócio criado antes da promoção.
//
// Vale para VISIBILIDADE. Escrever continua sendo decidido pela matriz de
// permissões (`RequirePermission`), que é por rota e por tenant — um `auditor`
// com acesso amplo de leitura continua sem as permissões de escrita.
func HasTenantWideScope(profile, functionalRole, baseRole string) bool {
	if tenantWideProfiles[strings.ToLower(strings.TrimSpace(profile))] {
		return true
	}
	if tenantWideRoles[strings.ToLower(strings.TrimSpace(functionalRole))] {
		return true
	}
	return tenantWideRoles[strings.ToLower(strings.TrimSpace(baseRole))]
}

// IsAssignableRole aceita o que pode ser ATRIBUÍDO a um membro (e apontado por
// um grupo): um papel do catálogo ou um perfil de acesso personalizado do
// tenant.
//
// Substitui `IsValidFunctionalRole` nos pontos de ATRIBUIÇÃO — grupo
// (`RoleID`), membro da empresa (`functional_role`), convite e perfil padrão da
// matriz. As checagens de catálogo continuam usando `IsValidFunctionalRole`
// (ex.: geração do catálogo de permissões, defaults da matriz).
func IsAssignableRole(r string) bool {
	return IsValidFunctionalRole(r) || IsAccessProfileID(r)
}

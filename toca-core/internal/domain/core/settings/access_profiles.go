package settings

import (
	"sort"
	"strings"

	"github.com/oginoino/toca/pkg/auth"
)

// CatalogIDs devolve o conjunto de ids de permissão do catálogo canônico —
// a allowlist usada por toda validação de permissão (matriz e perfis).
func CatalogIDs() map[string]bool {
	out := make(map[string]bool, len(auth.PermissionCatalog))
	for _, p := range auth.PermissionCatalog {
		out[p.ID] = true
	}
	return out
}

// FindCustomRole localiza um perfil de acesso personalizado pelo id.
func FindCustomRole(profiles []CustomRole, role string) (CustomRole, bool) {
	role = strings.TrimSpace(role)
	for _, profile := range profiles {
		if profile.ID == role {
			return profile, true
		}
	}
	return CustomRole{}, false
}

// RoleGrantsPermission é a decisão de autorização de UMA permissão para UM
// papel — o único lugar que sabe distinguir papel do catálogo de perfil
// personalizado.
//
// Regra (uma só, usada pelo enforcement do core E por `/me/permissions`, que é
// o que todos os outros serviços consultam):
//
//   - perfil de acesso personalizado → concede EXATAMENTE o que está no perfil.
//     Um perfil que não existe mais (id órfão no vínculo do membro) não concede
//     nada: falha FECHADO, nunca abre por herança do `baseRole`;
//   - papel do catálogo → a linha salva pelo tenant vence; sem linha, o default
//     do catálogo (EffectiveRoleMap).
func RoleGrantsPermission(stored, defaultMatrix []Permission, profiles []CustomRole, permissionID, role string) bool {
	if auth.IsAccessProfileID(role) {
		profile, ok := FindCustomRole(profiles, role)
		if !ok {
			return false
		}
		return profile.Grants(permissionID)
	}
	return EffectiveRoleMap(stored, defaultMatrix, permissionID)[role]
}

// RolePermissionsFor devolve os IDs concedidos a um papel, na ordem do catálogo.
// É o que `GET /me/permissions` responde e o que o middleware do core aplica.
func RolePermissionsFor(stored []Permission, profiles []CustomRole, role string) []string {
	if auth.IsAccessProfileID(role) {
		profile, ok := FindCustomRole(profiles, role)
		if !ok {
			return []string{}
		}
		granted := make(map[string]bool, len(profile.Permissions))
		for _, id := range profile.Permissions {
			granted[id] = true
		}
		out := make([]string, 0, len(granted))
		for _, p := range auth.PermissionCatalog {
			if granted[p.ID] {
				out = append(out, p.ID)
			}
		}
		return out
	}

	matrix := EffectiveMatrix(stored)
	out := make([]string, 0, len(auth.PermissionCatalog))
	for _, p := range auth.PermissionCatalog {
		if matrix[p.ID][role] {
			out = append(out, p.ID)
		}
	}
	return out
}

// EffectivePermissionsForRole devolve os IDs concedidos a um papel do catálogo,
// na ordem do catálogo. Mantido para os chamadores que só lidam com papéis de
// catálogo; para decidir sobre um perfil use `RolePermissionsFor`.
func EffectivePermissionsForRole(stored []Permission, functionalRole string) []string {
	return RolePermissionsFor(stored, nil, functionalRole)
}

// NormalizePermissionMatrix valida a matriz contra o catálogo canônico,
// preenchendo os papéis ausentes com `false` e devolvendo erro quando há ids ou
// papéis fora do catálogo.
//
// A matriz continua sendo o mapa dos NOVE papéis do catálogo: perfil de acesso
// personalizado não vira coluna aqui — o conjunto dele vive no próprio perfil
// (`CustomRole.Permissions`), para haver uma única representação de "quem pode o
// quê".
func NormalizePermissionMatrix(
	perms []Permission,
	catalogIDs map[string]bool,
	validRoles map[string]bool,
	allRoles []string,
) ([]Permission, error) {
	unknownIDs := map[string]bool{}
	unknownRoles := map[string]bool{}

	out := make([]Permission, 0, len(perms))
	for _, p := range perms {
		if !catalogIDs[p.ID] {
			unknownIDs[p.ID] = true
			continue
		}
		roles := make(map[string]bool, len(allRoles))
		for _, role := range allRoles {
			roles[role] = p.Roles[role] // ausente => false
		}
		for role := range p.Roles {
			if !validRoles[role] {
				unknownRoles[role] = true
			}
		}
		p.Roles = roles
		out = append(out, p)
	}

	if len(unknownIDs) > 0 || len(unknownRoles) > 0 {
		return nil, &InvalidPermissionMatrixError{
			UnknownPermissionIDs: sortedKeys(unknownIDs),
			UnknownRoles:         sortedKeys(unknownRoles),
		}
	}
	return out, nil
}

// NormalizeCustomRoles valida e normaliza os perfis de acesso do tenant.
//
// Regras (todas fail-closed, com a lista do que não bate devolvida ao cliente):
//   - id com o prefixo reservado `cr_` (nunca colide com papel do catálogo),
//     único dentro do tenant, e não-editável depois de criado;
//   - nome obrigatório;
//   - `baseRole` (quando informado) precisa existir no catálogo;
//   - permissões precisam existir no catálogo — ids desconhecidos são ERRO, não
//     descarte silencioso: era exatamente esse descarte que fazia a tela salvar
//     um perfil que o enforcement não aplicava.
//
// A ordem das permissões é normalizada para a ordem do catálogo (resposta
// estável para a UI e para os testes).
func NormalizeCustomRoles(profiles []CustomRole, catalogIDs map[string]bool) ([]CustomRole, error) {
	seenIDs := map[string]bool{}
	duplicated := map[string]bool{}
	invalidIDs := map[string]bool{}
	missingNames := map[string]bool{}
	unknownBaseRoles := map[string]bool{}
	unknownPerms := map[string]bool{}

	out := make([]CustomRole, 0, len(profiles))
	for _, profile := range profiles {
		profile.ID = strings.TrimSpace(profile.ID)
		profile.Name = strings.TrimSpace(profile.Name)
		profile.Description = strings.TrimSpace(profile.Description)
		profile.BaseRole = strings.ToLower(strings.TrimSpace(profile.BaseRole))
		profile.Color = strings.TrimSpace(profile.Color)

		if !auth.IsAccessProfileID(profile.ID) || auth.IsValidFunctionalRole(profile.ID) {
			invalidIDs[profile.ID] = true
			continue
		}
		if seenIDs[profile.ID] {
			duplicated[profile.ID] = true
			continue
		}
		seenIDs[profile.ID] = true

		if profile.Name == "" {
			missingNames[profile.ID] = true
		}
		if profile.BaseRole != "" && !auth.IsValidFunctionalRole(profile.BaseRole) {
			unknownBaseRoles[profile.BaseRole] = true
		}

		granted := map[string]bool{}
		for _, id := range profile.Permissions {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if !catalogIDs[id] {
				unknownPerms[id] = true
				continue
			}
			granted[id] = true
		}

		normalized := make([]string, 0, len(granted))
		for _, p := range auth.PermissionCatalog {
			if granted[p.ID] {
				normalized = append(normalized, p.ID)
			}
		}
		profile.Permissions = normalized
		out = append(out, profile)
	}

	if len(invalidIDs) > 0 || len(duplicated) > 0 || len(missingNames) > 0 ||
		len(unknownBaseRoles) > 0 || len(unknownPerms) > 0 {
		return nil, &InvalidAccessProfileError{
			InvalidIDs:           sortedKeys(invalidIDs),
			DuplicatedIDs:        sortedKeys(duplicated),
			MissingNames:         sortedKeys(missingNames),
			UnknownBaseRoles:     sortedKeys(unknownBaseRoles),
			UnknownPermissionIDs: sortedKeys(unknownPerms),
		}
	}

	return out, nil
}

func sortedProfileIDs(profiles []CustomRole) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.ID)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

package middleware

import (
	"context"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/auth"
)

type CorePermissionProvider interface {
	GetPermissions(ctx context.Context, tenantID string) (*settings.RolePermissions, error)
}

type PermissionAdapter struct {
	provider CorePermissionProvider
}

func NewPermissionAdapter(provider CorePermissionProvider) *PermissionAdapter {
	return &PermissionAdapter{provider: provider}
}

func (a *PermissionAdapter) GetRolesMap(ctx context.Context, tenantID, requiredPermission string) (map[string]bool, error) {
	rolePerms, err := a.provider.GetPermissions(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// A regra "linha do tenant vence; senão o default do catálogo" vive em
	// settings.RoleGrantsPermission e é a MESMA usada por /me/permissions (achado
	// A5: duas implementações divergiam e a tela mostrava permissão que o
	// middleware negava).
	//
	// É ela também que decide sobre um PERFIL DE ACESSO personalizado: o
	// `functional_role` do membro pode ser o id de um perfil do tenant
	// (Configurações → Permissões → Perfis Personalizados) e, nesse caso, quem
	// concede é o conjunto do próprio perfil. Id de perfil que não existe mais
	// NEGA (fail-closed) — nunca herda o papel base.
	var stored []settings.Permission
	var profiles []settings.CustomRole
	if rolePerms != nil {
		stored = rolePerms.Permissions
		profiles = rolePerms.CustomRoles
	}

	// O contrato do provider é "mapa de papéis → pode?"; o middleware lê a chave
	// do `functional_role` do token. O mapa cobre:
	//
	//   - as chaves da própria linha salva — inclusive papéis fora do catálogo que
	//     o tráfego interno usa: a credencial de serviço entra como
	//     `functional_role: "service"` e é a matriz do tenant que diz se ele pode;
	//   - os nove papéis do catálogo, para a linha ausente cair no default;
	//   - cada PERFIL DE ACESSO do tenant, porque o `functional_role` de um membro
	//     pode ser o id de um perfil. Id de perfil que não existe mais NEGA
	//     (fail-closed) — nunca herda o papel base.
	defaults := settings.BuildDefaultMatrix()
	storedRow := settings.EffectiveRoleMap(stored, defaults, requiredPermission)
	out := make(map[string]bool, len(storedRow)+len(auth.AllFunctionalRoles)+len(profiles))
	for role := range storedRow {
		out[role] = settings.RoleGrantsPermission(stored, defaults, profiles, requiredPermission, role)
	}
	for _, role := range auth.AllFunctionalRoles {
		if _, ok := out[role]; !ok {
			out[role] = settings.RoleGrantsPermission(stored, defaults, profiles, requiredPermission, role)
		}
	}
	for _, profile := range profiles {
		out[profile.ID] = settings.RoleGrantsPermission(stored, defaults, profiles, requiredPermission, profile.ID)
	}
	return out, nil
}

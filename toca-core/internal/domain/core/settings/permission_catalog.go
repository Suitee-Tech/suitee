package settings

import "github.com/oginoino/toca/pkg/auth"

func BuildDefaultMatrix() []Permission {
	var perms []Permission
	for _, p := range auth.PermissionCatalog {
		roles := make(map[string]bool)
		for _, role := range auth.AllFunctionalRoles {
			if allowed, ok := p.DefaultRoles[role]; ok && allowed {
				roles[role] = true
			} else {
				roles[role] = false
			}
		}
		perms = append(perms, Permission{
			ID:     p.ID,
			Label:  p.Label,
			Module: p.Module,
			Roles:  roles,
		})
	}
	return perms
}

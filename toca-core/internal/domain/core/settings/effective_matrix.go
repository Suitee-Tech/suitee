package settings

import "github.com/oginoino/toca/pkg/auth"

// EffectiveRoleMap resolve o mapa de papéis de UMA permissão para um tenant.
//
// FONTE DE DECISÃO ÚNICA (achado A5 da revisão de permissões de 2026-09-19):
// antes o enforcement (`middleware.PermissionAdapter`, usado por
// `RequirePermission`) e a listagem consumida pelo frontend
// (`GetEffectivePermissions` → `GET /me/permissions`) implementavam a MESMA regra
// em dois lugares, com uma divergência: `/me/permissions` curto-circuitava
// `functional_role == "admin"` para o catálogo inteiro, enquanto o middleware
// consultava a linha salva pelo tenant. Um tenant que revogasse algo de `admin`
// via a permissão sumir do enforcement mas continuar aparecendo na tela (e
// vice-versa, no caso de permissão concedida e depois removida).
//
// Regra (uma só): a linha salva pelo tenant vence; sem linha, vale o default do
// catálogo. Nunca há curto-circuito por papel — quem tem bypass é o PERFIL
// root (owner/admin), tratado em pkg/auth.IsRootProfile antes de chegar aqui.
func EffectiveRoleMap(stored, defaultMatrix []Permission, permissionID string) map[string]bool {
	for _, p := range stored {
		if p.ID == permissionID {
			return p.Roles
		}
	}
	for _, p := range defaultMatrix {
		if p.ID == permissionID {
			return p.Roles
		}
	}
	return map[string]bool{}
}

// EffectiveMatrix devolve, para cada permissão do catálogo, o mapa de papéis
// efetivo do tenant — a mesma decisão que o enforcement aplica.
func EffectiveMatrix(stored []Permission) map[string]map[string]bool {
	defaults := BuildDefaultMatrix()
	out := make(map[string]map[string]bool, len(auth.PermissionCatalog))
	for _, p := range auth.PermissionCatalog {
		out[p.ID] = EffectiveRoleMap(stored, defaults, p.ID)
	}
	return out
}

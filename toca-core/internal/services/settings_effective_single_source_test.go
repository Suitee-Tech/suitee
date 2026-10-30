package services

import (
	"context"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca-core/internal/middleware"
	"github.com/oginoino/toca/pkg/auth"
)

// P1 / A5 — fonte de decisão única entre `/me/permissions` e o enforcement.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A5) mostrou duas implementações da mesma regra:
//   - enforcement: `middleware.PermissionAdapter` → linha do tenant, senão default;
//   - `/me/permissions`: `GetEffectivePermissions` → CURTO-CIRCUITO para
//     `functional_role == "admin"` devolvendo o catálogo inteiro.
//
// Consequência: um tenant que salvasse `admin: false` numa permissão via a
// permissão sumir do enforcement mas continuar concedida na tela (e o inverso
// para permissão concedida depois removida do catálogo do FE).
//
// O invariante testado abaixo é o que importa: para QUALQUER papel, a lista de
// `/me/permissions` deve ser exatamente o conjunto de permissões que o adapter
// autorizaria.

func matrixWithAdminRevoked(permissionID string) []settings.Permission {
	perms := settings.BuildDefaultMatrix()
	for i := range perms {
		if perms[i].ID == permissionID {
			perms[i].Roles["admin"] = false
		}
	}
	return perms
}

func TestGetEffectivePermissions_AdminNaoIgnoraMatrizSalva(t *testing.T) {
	revoked := "leads:read"
	repo := &capturingPermissionsRepo{}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil)
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "corretor",
		Permissions: matrixWithAdminRevoked(revoked),
	}

	got, err := svc.GetEffectivePermissions(context.Background(), "tenant-1", auth.FunctionalRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range got {
		if id == revoked {
			t.Fatalf("DIVERGÊNCIA A5: /me/permissions concedeu %s a admin, mas o tenant revogou essa linha — o middleware vai negar", revoked)
		}
	}
	if len(got) != len(auth.PermissionCatalog)-1 {
		t.Fatalf("esperava catálogo menos a permissão revogada: %d de %d", len(got), len(auth.PermissionCatalog))
	}
}

// O invariante central: lista efetiva == decisão do enforcement, para todos os
// papéis e todas as permissões.
func TestEffectivePermissionsMatchesEnforcementDecision(t *testing.T) {
	stored := matrixWithAdminRevoked("leads:read")
	repo := &capturingPermissionsRepo{}
	repo.stored = &settings.RolePermissions{TenantID: "tenant-1", Permissions: stored}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil)
	adapter := middleware.NewPermissionAdapter(svc)

	ctx := context.Background()
	for _, role := range auth.AllFunctionalRoles {
		effective, err := svc.GetEffectivePermissions(ctx, "tenant-1", role)
		if err != nil {
			t.Fatal(err)
		}
		granted := make(map[string]bool, len(effective))
		for _, id := range effective {
			granted[id] = true
		}

		for _, p := range auth.PermissionCatalog {
			rolesMap, err := adapter.GetRolesMap(ctx, "tenant-1", p.ID)
			if err != nil {
				t.Fatalf("adapter(%s): %v", p.ID, err)
			}
			enforced := rolesMap[role]

			if granted[p.ID] != enforced {
				t.Fatalf("DIVERGÊNCIA: papel=%s permissão=%s listada=%v aplicada=%v (a tela e o enforcement precisam concordar)",
					role, p.ID, granted[p.ID], enforced)
			}
		}
	}
}

// Sem linha salva, vale o default do catálogo — para todo papel.
func TestEffectivePermissions_FallsBackToCatalogDefaults(t *testing.T) {
	repo := &capturingPermissionsRepo{} // GetByTenantID devolve nil
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil)

	for _, role := range auth.AllFunctionalRoles {
		got, err := svc.GetEffectivePermissions(context.Background(), "tenant-1", role)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		for _, p := range auth.PermissionCatalog {
			if p.DefaultRoles[role] {
				want++
			}
		}
		if len(got) != want {
			t.Fatalf("papel %s: %d permissões efetivas, esperava %d (defaults do catálogo)", role, len(got), want)
		}
	}
}

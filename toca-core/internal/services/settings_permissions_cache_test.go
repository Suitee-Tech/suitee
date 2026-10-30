package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/auth"
)

// P1 / A5 — a escrita da matriz invalida o cache dos consumidores.
//
// O cache de `/me/permissions` dos serviços dura 30 s (pkg/clients). Sem
// invalidação na escrita, uma revogação levava até 30 s para valer. O core
// incrementa a versão da matriz no Redis; os consumidores comparam a versão
// antes de usar o cache.

type fakeInvalidator struct {
	bumped []string
	err    error
}

func (f *fakeInvalidator) BumpPermissionsVersion(_ context.Context, tenantID string) error {
	f.bumped = append(f.bumped, tenantID)
	return f.err
}

func fullCatalogPayload() []settings.Permission {
	out := make([]settings.Permission, 0, len(auth.PermissionCatalog))
	for _, p := range auth.PermissionCatalog {
		roles := map[string]bool{}
		for role, allowed := range p.DefaultRoles {
			if allowed {
				roles[role] = true
			}
		}
		out = append(out, settings.Permission{ID: p.ID, Label: p.Label, Module: p.Module, Roles: roles})
	}
	return out
}

func TestUpdatePermissions_BumpaVersaoDoCache(t *testing.T) {
	repo := &capturingPermissionsRepo{}
	inv := &fakeInvalidator{}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil).WithPermissionsInvalidator(inv)

	req := &settings.RolePermissions{DefaultRole: "corretor", Permissions: fullCatalogPayload()}
	if err := svc.UpdatePermissions(context.Background(), "tenant-x", req); err != nil {
		t.Fatalf("salvar matriz: %v", err)
	}

	if len(inv.bumped) != 1 || inv.bumped[0] != "tenant-x" {
		t.Fatalf("cache não foi invalidado para o tenant: %v", inv.bumped)
	}
	if repo.calls != 1 {
		t.Fatalf("matriz não foi persistida (calls=%d)", repo.calls)
	}
}

// Matriz inválida não pode invalidar cache nem persistir nada.
func TestUpdatePermissions_MatrizInvalidaNaoBumpaNemPersiste(t *testing.T) {
	repo := &capturingPermissionsRepo{}
	inv := &fakeInvalidator{}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil).WithPermissionsInvalidator(inv)

	err := svc.UpdatePermissions(context.Background(), "tenant-x", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: []settings.Permission{{ID: "permissao-inexistente", Roles: map[string]bool{"admin": true}}},
	})
	if err == nil {
		t.Fatal("matriz inválida aceita")
	}
	if len(inv.bumped) != 0 {
		t.Fatalf("cache foi invalidado apesar da matriz inválida: %v", inv.bumped)
	}
	if repo.calls != 0 {
		t.Fatal("matriz inválida foi persistida")
	}
}

// O Redis fora do ar não pode impedir o salvamento da matriz (best-effort).
func TestUpdatePermissions_FalhaNoCacheNaoImpedeSalvar(t *testing.T) {
	repo := &capturingPermissionsRepo{}
	inv := &fakeInvalidator{err: errors.New("redis fora do ar")}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil).WithPermissionsInvalidator(inv)

	if err := svc.UpdatePermissions(context.Background(), "tenant-x", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: fullCatalogPayload(),
	}); err != nil {
		t.Fatalf("falha do cache não deveria propagar: %v", err)
	}
	if repo.calls != 1 {
		t.Fatal("matriz não foi persistida apesar do sucesso")
	}
}

// Sem invalidador configurado (REDIS_URI vazio) o serviço funciona igual.
func TestUpdatePermissions_SemInvalidadorContinuaFuncionando(t *testing.T) {
	repo := &capturingPermissionsRepo{}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil)

	if err := svc.UpdatePermissions(context.Background(), "tenant-x", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: fullCatalogPayload(),
	}); err != nil {
		t.Fatalf("sem invalidador deveria salvar normalmente: %v", err)
	}
	if repo.calls != 1 {
		t.Fatal("matriz não foi persistida")
	}
}

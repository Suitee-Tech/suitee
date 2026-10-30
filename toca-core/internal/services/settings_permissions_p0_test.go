package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/auth"
)

// P0.2 / A6 — validação ESTRITA da matriz de permissões.
//
// A revisão de permissões de 2026-09-19 (achado A6) mostrou que
// `PUT /company-settings/permissions` descartava em SILÊNCIO ids que não
// conhecia e não validava as chaves de papel dentro de `Roles`. O cliente
// recebia 200 "permissions updated" e o frontend (com catálogo próprio,
// divergente) acreditava ter salvado toggles que não tinham efeito.
//
// Contrato novo: erro explícito (`*settings.InvalidPermissionMatrixError`, que o
// handler mapeia para 400) listando os ids/papéis desconhecidos; papéis ausentes
// viram `false` explícito.

type capturingPermissionsRepo struct {
	saved  *settings.RolePermissions
	stored *settings.RolePermissions // matriz "já persistida" devolvida pelo GET
	calls  int
}

func (r *capturingPermissionsRepo) GetByTenantID(context.Context, string) (*settings.RolePermissions, error) {
	return r.stored, nil
}

func (r *capturingPermissionsRepo) Upsert(_ context.Context, s *settings.RolePermissions) error {
	r.saved = s
	r.calls++
	return nil
}

func a6Service() (*SettingsService, *capturingPermissionsRepo) {
	repo := &capturingPermissionsRepo{}
	return NewSettingsService(nil, nil, repo, nil, nil, nil, nil), repo
}

func TestUpdatePermissions_RejectsUnknownPermissionIDs(t *testing.T) {
	svc, repo := a6Service()

	req := &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: []settings.Permission{
			{ID: "settings:manage_commission", Roles: map[string]bool{"admin": true}}, // só existia no FE
			{ID: "company:manage_groups", Roles: map[string]bool{"admin": true}},      // só existia no FE
			{ID: "properties:create", Roles: map[string]bool{"admin": true}},          // fantasma
			{ID: "leads:read", Roles: map[string]bool{"admin": true}},                 // válido
		},
	}

	err := svc.UpdatePermissions(context.Background(), "tenant-1", req)
	if err == nil {
		t.Fatal("MATRIZ INVÁLIDA ACEITA EM SILÊNCIO: ids desconhecidos deveriam gerar erro, não 200")
	}
	var invalid *settings.InvalidPermissionMatrixError
	if !errors.As(err, &invalid) {
		t.Fatalf("esperava *InvalidPermissionMatrixError, veio %T: %v", err, err)
	}
	for _, want := range []string{"settings:manage_commission", "company:manage_groups", "properties:create"} {
		if !contains(invalid.UnknownPermissionIDs, want) {
			t.Errorf("erro não citou %s (citou %v)", want, invalid.UnknownPermissionIDs)
		}
	}
	if repo.calls != 0 {
		t.Fatal("matriz inválida foi PERSISTIDA — o tenant ficaria com configuração parcial")
	}
}

func TestUpdatePermissions_RejectsUnknownRoleKeys(t *testing.T) {
	svc, repo := a6Service()

	req := &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: []settings.Permission{
			{ID: "leads:read", Roles: map[string]bool{"admin": true, "root": true, "service": true}},
		},
	}

	err := svc.UpdatePermissions(context.Background(), "tenant-1", req)
	if err == nil {
		t.Fatal("chave de papel desconhecida aceita — viraria concessão viva na matriz")
	}
	var invalid *settings.InvalidPermissionMatrixError
	if !errors.As(err, &invalid) {
		t.Fatalf("esperava *InvalidPermissionMatrixError, veio %T", err)
	}
	if !contains(invalid.UnknownRoles, "root") || !contains(invalid.UnknownRoles, "service") {
		t.Errorf("erro não citou os papéis inválidos: %v", invalid.UnknownRoles)
	}
	if repo.calls != 0 {
		t.Fatal("matriz com papel inválido foi persistida")
	}
}

func TestUpdatePermissions_RejectsInvalidDefaultRole(t *testing.T) {
	svc, repo := a6Service()

	err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "superuser",
		Permissions: []settings.Permission{{ID: "leads:read", Roles: map[string]bool{}}},
	})
	if err == nil {
		t.Fatal("default role inválido aceito")
	}
	if repo.calls != 0 {
		t.Fatal("default role inválido foi persistido")
	}
}

func TestUpdatePermissions_AcceptsFullCatalogAndNormalizesRoles(t *testing.T) {
	svc, repo := a6Service()

	// O frontend envia a matriz completa (é o que a tela faz). Agora que o
	// catálogo do FE é GERADO do Go, todos os ids são conhecidos.
	req := &settings.RolePermissions{DefaultRole: "corretor"}
	for _, p := range auth.PermissionCatalog {
		roles := map[string]bool{}
		for role, allowed := range p.DefaultRoles {
			if allowed {
				roles[role] = true
			}
		}
		req.Permissions = append(req.Permissions, settings.Permission{ID: p.ID, Label: p.Label, Module: p.Module, Roles: roles})
	}

	if err := svc.UpdatePermissions(context.Background(), "tenant-1", req); err != nil {
		t.Fatalf("matriz completa do catálogo deveria ser aceita: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("matriz não foi persistida")
	}
	if len(repo.saved.Permissions) != len(auth.PermissionCatalog) {
		t.Fatalf("permissões persistidas = %d, catálogo = %d", len(repo.saved.Permissions), len(auth.PermissionCatalog))
	}
	// Papéis ausentes no payload viram false explícito (nenhum papel fica
	// indefinido no documento salvo).
	for _, p := range repo.saved.Permissions {
		for _, role := range auth.AllFunctionalRoles {
			if _, ok := p.Roles[role]; !ok {
				t.Fatalf("permissão %s sem a chave do papel %s após normalização", p.ID, role)
			}
		}
	}
}

// Guarda contra a regressão silenciosa: o id que o frontend usava para a aba de
// comissões não pode voltar a ser aceito, porque não existe enforcement dele.
func TestUpdatePermissions_RejectsCommissionSettingsPhantom(t *testing.T) {
	svc, _ := a6Service()
	err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: []settings.Permission{{ID: "settings:manage_commission", Roles: map[string]bool{"admin": true}}},
	})
	if err == nil {
		t.Fatal("settings:manage_commission aceita — a permissão da aba de comissões é finance:write")
	}
	if !strings.Contains(err.Error(), "settings:manage_commission") {
		t.Errorf("erro não cita o id problemático: %v", err)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

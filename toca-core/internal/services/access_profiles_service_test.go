package services

import (
	"context"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca-core/internal/middleware"
	"github.com/oginoino/toca/pkg/auth"
)

// Perfis de acesso personalizados — o invariante que faltava.
//
// Antes desta entrega "Perfis Personalizados" era um JSON decorativo: a tela
// criava `{id, name, baseRole, color}`, salvava junto da matriz e NADA consultava
// aquilo. Estes testes travam as três pontas que fazem o perfil existir de
// verdade:
//
//  1. o perfil concede EXATAMENTE o seu conjunto (nem o do `baseRole`, nem o do
//     papel do catálogo de mesmo nome);
//  2. `/me/permissions` (o que todos os serviços consultam) e o enforcement do
//     core decidem IGUAL — inclusive para perfil (A5);
//  3. perfil apagado/desconhecido NEGA (fail-closed), nunca herda o papel base.

func profileWith(id string, permissions ...string) settings.CustomRole {
	return settings.CustomRole{ID: id, Name: "Perfil " + id, BaseRole: "corretor", Permissions: permissions}
}

func TestRolePermissionsFor_CustomProfileGrantsOnlyItsOwnSet(t *testing.T) {
	stored := settings.BuildDefaultMatrix()
	profiles := []settings.CustomRole{profileWith("cr_vendas", "leads:read", "properties:read")}

	got := settings.RolePermissionsFor(stored, profiles, "cr_vendas")
	want := map[string]bool{"leads:read": true, "properties:read": true}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("perfil concedeu %s, que não está no conjunto dele", id)
		}
		delete(want, id)
	}
	if len(want) > 0 {
		t.Fatalf("perfil não concedeu: %v", want)
	}

	// O papel base (corretor) continua com o conjunto DELE — o perfil não
	// altera, nem empresta, as permissões do catálogo.
	corretor := settings.RolePermissionsFor(stored, profiles, "corretor")
	if len(corretor) == len(got) {
		t.Fatal("perfil e papel base devolveram o mesmo tamanho — o teste não distingue os dois")
	}
}

func TestRolePermissionsFor_DeletedProfileDeniesEverything(t *testing.T) {
	stored := settings.BuildDefaultMatrix()
	// O vínculo do membro aponta para um perfil que não existe mais (a tela de
	// Permissões apagou o perfil; o membro ficou com o id órfão).
	got := settings.RolePermissionsFor(stored, nil, "cr_apagado")
	if len(got) != 0 {
		t.Fatalf("FAIL-OPEN: perfil inexistente concedeu %v", got)
	}
}

// O invariante A5 estendido ao perfil: a lista de /me/permissions tem de ser
// exatamente o que o adapter do middleware autoriza.
func TestEffectivePermissionsMatchesEnforcementForAccessProfiles(t *testing.T) {
	repo := &capturingPermissionsRepo{}
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{
			profileWith("cr_vendas", "leads:read", "leads:write"),
			profileWith("cr_leitura", "properties:read"),
		},
	}
	svc := NewSettingsService(nil, nil, repo, nil, nil, nil, nil)
	adapter := middleware.NewPermissionAdapter(svc)
	ctx := context.Background()

	for _, role := range []string{"cr_vendas", "cr_leitura", "corretor", auth.FunctionalRoleAdmin, "cr_inexistente"} {
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
			if rolesMap[role] != granted[p.ID] {
				t.Fatalf("DIVERGÊNCIA: papel/perfil=%s permissão=%s listada=%v aplicada=%v",
					role, p.ID, granted[p.ID], rolesMap[role])
			}
		}
	}
}

// ───────────────────────── CRUD do perfil (serviço) ─────────────────────────

func TestCreateAccessProfile_SeedsFromTheTenantsBaseRole(t *testing.T) {
	svc, repo := a6Service()
	// O tenant revogou `finance:read` do corretor: o perfil "baseado em corretor"
	// precisa nascer do EFETIVO do tenant, não do default do catálogo.
	stored := settings.BuildDefaultMatrix()
	for i := range stored {
		if stored[i].ID == "finance:read" {
			stored[i].Roles["corretor"] = false
		}
	}
	repo.stored = &settings.RolePermissions{TenantID: "tenant-1", DefaultRole: "corretor", Permissions: stored}

	created, err := svc.CreateAccessProfile(context.Background(), "tenant-1", AccessProfileInput{Name: "Corretor Sênior", BaseRole: "corretor"})
	if err != nil {
		t.Fatalf("CreateAccessProfile: %v", err)
	}
	if !auth.IsAccessProfileID(created.ID) {
		t.Fatalf("perfil criado sem o prefixo reservado: %q", created.ID)
	}
	for _, id := range created.Permissions {
		if id == "finance:read" {
			t.Fatal("perfil nasceu com permissão que o papel base NÃO tem neste tenant")
		}
	}
	if len(created.Permissions) == 0 {
		t.Fatal("perfil nasceu sem nenhuma permissão")
	}
	if repo.saved == nil || len(repo.saved.CustomRoles) != 1 {
		t.Fatalf("perfil não foi persistido: %+v", repo.saved)
	}
}

// Um perfil não pode se passar por um papel do catálogo (nem ser gravado por
// uma rota que não é dele): o id precisa ter o prefixo reservado, a rota da
// matriz ignora perfis, e a decisão de um papel do catálogo continua saindo da
// matriz — nunca de um perfil homônimo.
func TestAccessProfileCannotImpersonateCatalogRole(t *testing.T) {
	// (1) A validação de perfil recusa id sem o prefixo reservado.
	_, err := settings.NormalizeCustomRoles(
		[]settings.CustomRole{{ID: "admin", Name: "sou admin", Permissions: []string{"settings:manage_permissions"}}},
		settings.CatalogIDs(),
	)
	if err == nil {
		t.Fatal("NormalizeCustomRoles aceitou perfil com id de papel do catálogo")
	}

	// (2) A rota da matriz NÃO grava perfis: o payload é ignorado e o que vale é
	// o documento persistido.
	svc, repo := a6Service()
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
	}
	if err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{{ID: "admin", Name: "sou admin", Permissions: []string{"settings:manage_permissions"}}},
	}); err != nil {
		t.Fatalf("UpdatePermissions: %v", err)
	}
	if len(repo.saved.CustomRoles) != 0 {
		t.Fatalf("perfil entrou por uma rota que não grava perfis: %+v", repo.saved.CustomRoles)
	}

	// (3) Mesmo que existisse um perfil homônimo no documento, `admin` seria
	// decidido pela matriz do catálogo (o perfil homônimo é ignorado) — e um id
	// de perfil inexistente não concede nada.
	profiles := []settings.CustomRole{{ID: "admin", Name: "sou admin"}}
	if !settings.RoleGrantsPermission(settings.BuildDefaultMatrix(), settings.BuildDefaultMatrix(), profiles, "settings:manage_permissions", "admin") {
		t.Fatal("o perfil homônimo sequestrou a decisão do papel `admin`")
	}
	if settings.RoleGrantsPermission(settings.BuildDefaultMatrix(), settings.BuildDefaultMatrix(), profiles, "settings:manage_permissions", "cr_inexistente") {
		t.Fatal("perfil inexistente concedeu permissão")
	}
}

func TestUpdateAccessProfile_ReplacesPermissionsAndKeepsID(t *testing.T) {
	svc, repo := a6Service()
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{profileWith("cr_vendas", "leads:read")},
	}

	updated, err := svc.UpdateAccessProfile(context.Background(), "tenant-1", "cr_vendas", AccessProfileInput{
		Name:        "Vendas (novo)",
		BaseRole:    "gestor",
		Permissions: []string{"leads:write", "nao:existe"},
	})
	if err == nil {
		t.Fatal("permissão fora do catálogo foi aceita em silêncio")
	}
	var invalid *settings.InvalidAccessProfileError
	if !asInvalidProfile(err, &invalid) {
		t.Fatalf("erro inesperado: %v", err)
	}

	updated, err = svc.UpdateAccessProfile(context.Background(), "tenant-1", "cr_vendas", AccessProfileInput{
		Name:        "Vendas (novo)",
		BaseRole:    "gestor",
		Permissions: []string{"leads:write"},
	})
	if err != nil {
		t.Fatalf("UpdateAccessProfile: %v", err)
	}
	if updated.ID != "cr_vendas" {
		t.Fatalf("o id do perfil mudou: %q", updated.ID)
	}
	if len(updated.Permissions) != 1 || updated.Permissions[0] != "leads:write" {
		t.Fatalf("permissões não foram substituídas: %v", updated.Permissions)
	}
	if updated.BaseRole != "gestor" {
		t.Fatalf("papel base não foi atualizado: %q", updated.BaseRole)
	}

	// Edição sem `permissions` preserva o conjunto (o editor pode mudar só o nome).
	kept, err := svc.UpdateAccessProfile(context.Background(), "tenant-1", "cr_vendas", AccessProfileInput{Name: "Só o nome"})
	if err != nil {
		t.Fatalf("UpdateAccessProfile (sem permissões): %v", err)
	}
	if len(kept.Permissions) != 1 || kept.Permissions[0] != "leads:write" {
		t.Fatalf("edição sem permissões apagou o conjunto: %v", kept.Permissions)
	}
}

func TestDeleteAccessProfile_RemovesGrantsAndFixesDefaultRole(t *testing.T) {
	svc, repo := a6Service()
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "cr_vendas",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{profileWith("cr_vendas", "leads:read")},
	}

	if err := svc.DeleteAccessProfile(context.Background(), "tenant-1", "cr_vendas"); err != nil {
		t.Fatalf("DeleteAccessProfile: %v", err)
	}
	if len(repo.saved.CustomRoles) != 0 {
		t.Fatalf("perfil não foi removido: %+v", repo.saved.CustomRoles)
	}
	if repo.saved.DefaultRole == "cr_vendas" {
		t.Fatal("papel padrão de novos usuários apontando para perfil apagado")
	}
	// O acesso cai imediatamente: quem tinha o perfil fica sem permissão (a
	// reatribuição é feita na tela de Equipe/Grupos, com a lista de impacto).
	if got := settings.RolePermissionsFor(repo.saved.Permissions, repo.saved.CustomRoles, "cr_vendas"); len(got) != 0 {
		t.Fatalf("perfil apagado ainda concede: %v", got)
	}

	if err := svc.DeleteAccessProfile(context.Background(), "tenant-1", "cr_vendas"); err == nil {
		t.Fatal("apagar perfil inexistente deveria falhar (404)")
	}
}

func TestUpdatePermissions_NeverTouchesAccessProfiles(t *testing.T) {
	svc, repo := a6Service()
	repo.stored = &settings.RolePermissions{
		TenantID:    "tenant-1",
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{profileWith("cr_vendas", "leads:read")},
	}

	// Papel padrão apontando para perfil que não existe: recusado (todo usuário
	// novo entraria sem permissão nenhuma).
	err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "cr_inexistente",
		Permissions: settings.BuildDefaultMatrix(),
	})
	if err == nil {
		t.Fatal("papel padrão apontando para perfil inexistente foi aceito")
	}

	// Papel padrão apontando para perfil existente: aceito.
	if err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "cr_vendas",
		Permissions: settings.BuildDefaultMatrix(),
	}); err != nil {
		t.Fatalf("papel padrão com perfil existente deveria ser aceito: %v", err)
	}

	// A matriz salva MANTÉM os perfis persistidos — inclusive quando o payload
	// não manda perfis nenhum (o caso que apagaria o acesso de quem usa o perfil).
	if len(repo.saved.CustomRoles) != 1 || repo.saved.CustomRoles[0].ID != "cr_vendas" {
		t.Fatalf("o PUT da matriz mexeu nos perfis: %+v", repo.saved.CustomRoles)
	}
	// E também não deixa um payload malicioso/vazio apagá-los.
	repo.stored = repo.saved
	if err := svc.UpdatePermissions(context.Background(), "tenant-1", &settings.RolePermissions{
		DefaultRole: "corretor",
		Permissions: settings.BuildDefaultMatrix(),
		CustomRoles: []settings.CustomRole{},
	}); err != nil {
		t.Fatalf("UpdatePermissions: %v", err)
	}
	if len(repo.saved.CustomRoles) != 1 || repo.saved.CustomRoles[0].ID != "cr_vendas" {
		t.Fatalf("payload vazio de perfis apagou o perfil: %+v", repo.saved.CustomRoles)
	}
	if got := settings.RolePermissionsFor(repo.saved.Permissions, repo.saved.CustomRoles, "cr_vendas"); len(got) != 1 || got[0] != "leads:read" {
		t.Fatalf("o perfil perdeu o conjunto depois de salvar a matriz: %v", got)
	}
}

// asInvalidProfile evita importar errors só para o teste de erro tipado.
func asInvalidProfile(err error, target **settings.InvalidAccessProfileError) bool {
	if e, ok := err.(*settings.InvalidAccessProfileError); ok {
		*target = e
		return true
	}
	return false
}

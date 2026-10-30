package service

import (
	"context"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/group"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
)

// Unificação "Grupos de Usuários" ↔ "Perfis Personalizados".
//
// O grupo é quem ATRIBUI o perfil: ao apontar para um PERFIL DE ACESSO do
// tenant (`cr_…`, criado em Configurações → Permissões), o membro passa a ter o
// id do perfil como `functional_role`, e a autorização sai do conjunto de
// permissões do perfil (settings.RolePermissionsFor, no toca-core). Antes disso
// o grupo só aceitava papel do catálogo — e o perfil criado na outra tela era um
// rótulo decorativo que ninguém conseguia atribuir.
func TestGroupCanAssignAccessProfile(t *testing.T) {
	svc, repo := newGroupServiceForRole(t)
	groups := svc.groupRepo.(*fakeGroupRepo)
	groups.groups["group-vendas"] = &group.Group{
		ID: "group-vendas", CompanyID: "company-1", Name: "Vendas Centro", RoleID: "cr_9f1c2b3a-0000-4000-8000-000000000001",
	}

	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if err := svc.AddMemberToGroup(context.Background(), "owner-1", "company-1", "group-vendas", "member-1"); err != nil {
		t.Fatalf("grupo com perfil de acesso deveria ser aplicável: %v", err)
	}

	got := repo.byID["company-1"].Company.Members[1].FunctionalRole
	if got != "cr_9f1c2b3a-0000-4000-8000-000000000001" {
		t.Fatalf("functional_role = %q, esperado o id do perfil", got)
	}
	if !pkgAuth.IsAccessProfileID(got) {
		t.Fatalf("o vínculo não ficou com um id de perfil: %q", got)
	}
	// Troca de papel continua revogando o token (A15): o token do membro carrega
	// o `functional_role` antigo e valeria até expirar.
	if len(revoker.keys) != 1 {
		t.Fatalf("troca para perfil não revogou o vínculo: %v", revoker.keys)
	}
}

func TestCreateGroup_AcceptsAccessProfileAndStillRejectsGarbage(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)

	created, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{
		Name:           "Corretores Sênior",
		FunctionalRole: "cr_9f1c2b3a-0000-4000-8000-000000000002",
	})
	if err != nil {
		t.Fatalf("CreateGroup com perfil de acesso: %v", err)
	}
	if created.RoleID != "cr_9f1c2b3a-0000-4000-8000-000000000002" {
		t.Fatalf("RoleID = %q, esperado o id do perfil", created.RoleID)
	}

	// O identificador sintético legado continua recusado (ele não casa com
	// perfil nenhum da matriz do tenant): fail-closed com explicação.
	for _, invalid := range []string{"super_admin", "custom_abc123", "cr_"} {
		if _, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{Name: "X", FunctionalRole: invalid}); err == nil {
			t.Fatalf("papel/perfil inválido %q foi aceito", invalid)
		}
	}
}

// Reparo do grupo legado (`custom_<uuid>`) apontando-o para um perfil de acesso
// do tenant — o caminho que o operador usa na tela de Grupos.
func TestUpdateGroup_RepairsLegacyRoleToAccessProfile(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)
	groups := svc.groupRepo.(*fakeGroupRepo)
	groups.groups["group-legado"] = &group.Group{ID: "group-legado", CompanyID: "company-1", Name: "Legado", RoleID: "custom_abc123"}

	profileID := "cr_9f1c2b3a-0000-4000-8000-000000000003"
	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", "group-legado", &group.UpdateGroupRequest{Name: "Legado", FunctionalRole: profileID}); err != nil {
		t.Fatalf("reparo para perfil de acesso: %v", err)
	}
	if got := groups.groups["group-legado"].RoleID; got != profileID {
		t.Fatalf("RoleID após reparo = %q, esperado %q", got, profileID)
	}
}

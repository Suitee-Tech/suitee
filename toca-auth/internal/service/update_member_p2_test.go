package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/group"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
)

// P2/M6+M7 — PATCH de membro (papel/perfil) e desativação de vínculo.
//
// A revisão de permissões de 2026-09-19 registrou duas lacunas funcionais:
// M6 (não havia endpoint para mudar papel/perfil de um membro existente — só
// efeito colateral de grupo/convite) e M7 (o `IsActive` era escrito `true` no
// cadastro e nunca lido: não havia como bloquear alguém sem removê-lo).
//
// O contrato testado aqui é o do serviço: autoridade, validação contra o
// catálogo, proteção do dono/self, trilha de auditoria e revogação do token.

func memberByID(t *testing.T, repo *fakeAccountRepo, companyID, accountID string) company.CompanyMember {
	t.Helper()
	acc, err := repo.GetByID(context.Background(), companyID)
	if err != nil || acc == nil || acc.Company == nil {
		t.Fatalf("company %s não encontrada no dublê", companyID)
	}
	for _, m := range acc.Company.Members {
		if m.AccountID == accountID {
			return m
		}
	}
	t.Fatalf("membro %s não encontrado", accountID)
	return company.CompanyMember{}
}

func TestUpdateCompanyMember_ValidatesFunctionalRoleAgainstCatalog(t *testing.T) {
	svc, repo := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	ctx := context.Background()
	companyID := "company-1"

	// Papel inexistente no catálogo: recusa (antes do endpoint nem havia caminho
	// para trocar papel; o que existia — grupo custom — gravava `custom_<uuid>`).
	err := func() error {
		_, err := svc.UpdateCompanyMember(ctx, "owner-1", companyID, "member-1", &auth.UpdateMemberRequest{FunctionalRole: "super_admin"})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "invalid functional role") {
		t.Fatalf("papel fora do catálogo deveria ser recusado, veio: %v", err)
	}
	if got := memberByID(t, repo, companyID, "member-1").FunctionalRole; got != "corretor" {
		t.Fatalf("papel foi alterado apesar da recusa: %q", got)
	}

	detail, err := svc.UpdateCompanyMember(ctx, "owner-1", companyID, "member-1", &auth.UpdateMemberRequest{FunctionalRole: "financeiro"})
	if err != nil {
		t.Fatalf("papel de catálogo recusado: %v", err)
	}
	if detail == nil || detail.Role != "financeiro" {
		t.Fatalf("detalhe devolvido não reflete o papel novo: %+v", detail)
	}
	if got := memberByID(t, repo, companyID, "member-1").FunctionalRole; got != "financeiro" {
		t.Fatalf("papel no vínculo = %q, want financeiro", got)
	}
}

func TestUpdateCompanyMember_RequiresCompanyAdminAuthority(t *testing.T) {
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
		company.CompanyMember{AccountID: "member-2", Profile: "member", FunctionalRole: "corretor"},
	))

	_, err := svc.UpdateCompanyMember(context.Background(), "member-1", "company-1", "member-2", &auth.UpdateMemberRequest{FunctionalRole: "gestor"})
	if err == nil || err.Error() != "unauthorized" {
		t.Fatalf("membro comum não pode alterar vínculo de outro; veio: %v", err)
	}
}

func TestUpdateCompanyMember_ProtectsOwnerAndSelfChanges(t *testing.T) {
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "admin-2", Profile: "admin", FunctionalRole: "gestor"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	ctx := context.Background()

	if _, err := svc.UpdateCompanyMember(ctx, "owner-1", "company-1", "owner-1", &auth.UpdateMemberRequest{FunctionalRole: "corretor"}); err == nil {
		t.Fatal("o dono não pode rebaixar a si mesmo (empresa ficaria sem administrador)")
	}
	if _, err := svc.UpdateCompanyMember(ctx, "owner-1", "company-1", "owner-1", &auth.UpdateMemberRequest{IsActive: boolPtr(false)}); err == nil {
		t.Fatal("o dono não pode ser desativado")
	}
	if _, err := svc.UpdateCompanyMember(ctx, "admin-2", "company-1", "admin-2", &auth.UpdateMemberRequest{FunctionalRole: "admin"}); err == nil {
		t.Fatal("ninguém altera o próprio vínculo (auto-elevação/auto-bloqueio)")
	}
}

func TestUpdateCompanyMember_OwnerProfileRequiresOwnerCaller(t *testing.T) {
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "admin-2", Profile: "admin", FunctionalRole: "gestor"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	ctx := context.Background()

	// `admin` de empresa só é concedido pelo dono (regra do P0/C3, replicada aqui).
	if _, err := svc.UpdateCompanyMember(ctx, "admin-2", "company-1", "member-1", &auth.UpdateMemberRequest{Profile: "admin"}); err == nil {
		t.Fatal("admin não-dono não pode promover a admin")
	}
	// `owner` nunca é atribuível: o dono é Company.OwnerID.
	if _, err := svc.UpdateCompanyMember(ctx, "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{Profile: "owner"}); err == nil {
		t.Fatal("perfil owner não pode ser atribuído")
	}
	if _, err := svc.UpdateCompanyMember(ctx, "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{Profile: "admin"}); err != nil {
		t.Fatalf("o dono deveria poder promover a admin: %v", err)
	}
}

// M7 — suspender o vínculo precisa ser efetivo: além de gravar `is_active=false`,
// a marca de revogação do A15 é escrita SEM TTL (a decisão administrativa não
// expira sozinha como a remoção/troca de papel).
func TestUpdateCompanyMember_SuspendsMembershipAndRevokesToken(t *testing.T) {
	collector := withAuditCollector(t)
	svc, repo := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	detail, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{IsActive: boolPtr(false)})
	if err != nil {
		t.Fatalf("suspender vínculo: %v", err)
	}
	if detail.IsActive {
		t.Fatalf("detalhe devolveu vínculo ativo após a suspensão: %+v", detail)
	}

	member := memberByID(t, repo, "company-1", "member-1")
	if member.Active() {
		t.Fatal("vínculo continua ativo na fonte de verdade")
	}
	if member.DeactivatedAt == nil {
		t.Fatal("suspensão precisa registrar quando aconteceu")
	}

	wantKey := pkgAuth.MembershipRevocationKey("company-1", "member-1")
	if len(revoker.keys) != 1 || revoker.keys[0] != wantKey {
		t.Fatalf("marca de revogação não gravada: %+v", revoker.keys)
	}
	if revoker.ttl != 0 {
		t.Fatalf("marca de SUSPENSÃO tem TTL %v; deveria ser durável (0)", revoker.ttl)
	}

	if _, ok := collector.find("company.member_deactivated"); !ok {
		t.Fatal("suspensão sem trilha de auditoria")
	}
}

func TestUpdateCompanyMember_ReactivatesAndClearsMark(t *testing.T) {
	svc, repo := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor", IsActive: boolPtr(false)},
	))
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	detail, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{IsActive: boolPtr(true)})
	if err != nil {
		t.Fatalf("reativar vínculo: %v", err)
	}
	if !detail.IsActive {
		t.Fatalf("detalhe devolveu vínculo inativo após reativar: %+v", detail)
	}
	member := memberByID(t, repo, "company-1", "member-1")
	if !member.Active() || member.DeactivatedAt != nil {
		t.Fatalf("vínculo reativado manteve estado de suspensão: %+v", member)
	}
	if len(revoker.deleted) != 1 || revoker.deleted[0] != pkgAuth.MembershipRevocationKey("company-1", "member-1") {
		t.Fatalf("marca de suspensão não foi limpa: %+v", revoker.deleted)
	}
}

func TestUpdateCompanyMember_RoleChangeRevokesAndAudits(t *testing.T) {
	collector := withAuditCollector(t)
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if _, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{FunctionalRole: "financeiro"}); err != nil {
		t.Fatalf("trocar papel: %v", err)
	}

	entry, ok := collector.find("company.member_role_changed")
	if !ok {
		t.Fatal("troca de papel sem trilha")
	}
	if entry.Before["functional_role"] != "corretor" || entry.After["functional_role"] != "financeiro" {
		t.Fatalf("trilha sem antes/depois do papel: %+v", entry)
	}
	// Token carrega o papel: sem revogação o portador segue com o papel antigo.
	if len(revoker.keys) != 1 {
		t.Fatalf("troca de papel deveria revogar o vínculo: %+v", revoker.keys)
	}
	if revoker.ttl == 0 {
		t.Fatal("revogação por troca de papel tem TTL do token, não é durável")
	}
}

// Operação sem mudança efetiva não pode derrubar o token de ninguém.
func TestUpdateCompanyMember_NoOpDoesNotRevoke(t *testing.T) {
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	))
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if _, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{FunctionalRole: "corretor"}); err != nil {
		t.Fatalf("PATCH idempotente: %v", err)
	}
	if len(revoker.keys) != 0 {
		t.Fatalf("PATCH sem mudança revogou o vínculo: %+v", revoker.keys)
	}
}

func TestUpdateCompanyMember_MemberNotFound(t *testing.T) {
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
	))
	if _, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "ghost", &auth.UpdateMemberRequest{FunctionalRole: "corretor"}); err == nil {
		t.Fatal("membro inexistente deveria falhar")
	}
}

// P2/M6 — grupos precisam apontar para um papel do catálogo; o identificador
// sintético `custom_<uuid>` gravava no membro um papel que a matriz do tenant não
// conhece (sem permissão nenhuma e fora de todos os grupos reais).
func TestCreateGroup_RequiresCatalogRole(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)

	if _, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{Name: "Time A", FunctionalRole: "super_admin"}); err == nil {
		t.Fatal("grupo com papel fora do catálogo deveria ser recusado")
	}

	created, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{Name: "Time A", FunctionalRole: "financeiro"})
	if err != nil {
		t.Fatalf("grupo com papel de catálogo: %v", err)
	}
	if created.RoleID != "financeiro" {
		t.Fatalf("RoleID = %q, want financeiro (papel do catálogo)", created.RoleID)
	}

	// Sem papel declarado, cai no default de menor privilégio — nunca no
	// identificador sintético que não existe em catálogo nenhum.
	defaulted, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{Name: "Time B"})
	if err != nil {
		t.Fatalf("grupo sem papel: %v", err)
	}
	if defaulted.RoleID != "corretor" {
		t.Fatalf("RoleID default = %q, want corretor", defaulted.RoleID)
	}
	if !pkgAuth.IsValidFunctionalRole(defaulted.RoleID) {
		t.Fatalf("RoleID %q não existe no catálogo", defaulted.RoleID)
	}
}

func TestAddMemberToGroup_LegacyCustomGroupFailsClosed(t *testing.T) {
	svc, repo := newGroupServiceForRole(t)
	// Grupo legado: gravado antes do P2/M6 com o identificador sintético.
	groups := svc.groupRepo.(*fakeGroupRepo)
	groups.groups["group-legado"] = &group.Group{ID: "group-legado", CompanyID: "company-1", RoleID: "custom_abc123"}

	err := svc.AddMemberToGroup(context.Background(), "owner-1", "company-1", "group-legado", "member-1")
	if err == nil {
		t.Fatal("grupo sem papel de catálogo não pode ser aplicado ao membro")
	}
	if !strings.Contains(err.Error(), "catalog functional role") {
		t.Fatalf("erro deveria explicar que falta papel de catálogo: %v", err)
	}
	if got := memberByID(t, repo, "company-1", "member-1").FunctionalRole; got != "corretor" {
		t.Fatalf("papel do membro foi corrompido pelo grupo legado: %q", got)
	}
}

func TestUpdateGroup_RepairsLegacyCustomRole(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)
	groups := svc.groupRepo.(*fakeGroupRepo)
	groups.groups["group-legado"] = &group.Group{ID: "group-legado", CompanyID: "company-1", Name: "Legado", RoleID: "custom_abc123"}

	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", "group-legado", &group.UpdateGroupRequest{Name: "Legado", FunctionalRole: "marketing"}); err != nil {
		t.Fatalf("reparo de grupo legado: %v", err)
	}
	if got := groups.groups["group-legado"].RoleID; got != "marketing" {
		t.Fatalf("RoleID após reparo = %q, want marketing", got)
	}

	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", "group-legado", &group.UpdateGroupRequest{Name: "Legado", FunctionalRole: "nao_existe"}); err == nil {
		t.Fatal("reparo para papel fora do catálogo deveria ser recusado")
	}
}

// Regressão: com grupos custom apontando para papéis do catálogo, "é default"
// não pode mais ser inferido do RoleID — senão o grupo custom vira
// impropriamente protegido (não renomeia/não apaga).
func TestGroupDefaultDetectionIsExplicit(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)
	groups := svc.groupRepo.(*fakeGroupRepo)
	groups.groups["group-custom"] = &group.Group{ID: "group-custom", CompanyID: "company-1", Name: "Custom", RoleID: "corretor", CreatedBy: "owner-1"}
	delete(groups.groups, "group-financeiro")
	groups.groups["group-default"] = &group.Group{ID: "group-default", CompanyID: "company-1", Name: "Financeiro", RoleID: "financeiro", CreatedBy: "system", IsDefault: true}

	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", "group-custom", &group.UpdateGroupRequest{Name: "Custom renomeado"}); err != nil {
		t.Fatalf("grupo custom deveria ser renomeável: %v", err)
	}
	if got := groups.groups["group-custom"].Name; got != "Custom renomeado" {
		t.Fatalf("renomeação ignorada em grupo custom: %q", got)
	}
	if err := svc.DeleteGroup(context.Background(), "owner-1", "company-1", "group-custom"); err != nil {
		t.Fatalf("grupo custom deveria ser apagável: %v", err)
	}
	if err := svc.DeleteGroup(context.Background(), "owner-1", "company-1", "group-default"); err == nil {
		t.Fatal("grupo de papel do sistema não pode ser apagado")
	}
	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", "group-default", &group.UpdateGroupRequest{Name: "x", FunctionalRole: "corretor"}); err == nil {
		t.Fatal("grupo de papel do sistema não troca de papel")
	}
}

func TestUpdateCompanyMember_IsActiveSurvivesLegacyRows(t *testing.T) {
	// Vínculo gravado antes do campo: `IsActive` ausente = ativo.
	legacy := company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"}
	if !legacy.Active() {
		t.Fatal("vínculo sem o campo is_active precisa continuar ativo")
	}
	svc, _ := newCompanyServiceWithRepo(t, companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		legacy,
	))
	detail, err := svc.UpdateCompanyMember(context.Background(), "owner-1", "company-1", "member-1", &auth.UpdateMemberRequest{Profile: "viewer"})
	if err != nil {
		t.Fatalf("alterar perfil de vínculo legado: %v", err)
	}
	if !detail.IsActive || detail.Profile != "viewer" {
		t.Fatalf("vínculo legado alterado incorretamente: %+v", detail)
	}
	_ = account.AccountTypeCompany
	_ = time.Now
}

func boolPtr(v bool) *bool { return &v }

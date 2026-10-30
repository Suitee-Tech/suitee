package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/group"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
)

// P1/A15 — remover membro precisa marcar o vínculo como revogado.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A15) registrou que remover membro não invalidava o token: o acesso
// continuava até o token expirar. O gravador é chamado no fim da remoção (depois
// de o vínculo sair da fonte de verdade) e o verificador do middleware
// (pkg/auth) consulta a mesma chave.

type recordingRevoker struct {
	keys    []string
	deleted []string
	ttl     time.Duration
	err     error
}

// Delete completa o contrato de limpeza da marca (P2/M7: reativação de vínculo).
func (r *recordingRevoker) Delete(_ context.Context, keys ...string) error {
	r.deleted = append(r.deleted, keys...)
	return nil
}

func (r *recordingRevoker) Set(_ context.Context, key string, _ interface{}, ttl time.Duration) error {
	if r.err != nil {
		return r.err
	}
	r.keys = append(r.keys, key)
	r.ttl = ttl
	return nil
}

func companyWithMembers(ownerID string, members ...company.CompanyMember) *account.Account {
	return &account.Account{
		ID:   "company-1",
		Type: account.AccountTypeCompany,
		Company: &company.Company{
			OwnerID: ownerID,
			Members: members,
		},
	}
}

func newCompanyServiceWithRepo(t *testing.T, acc *account.Account) (*CompanyService, *fakeAccountRepo) {
	t.Helper()
	repo := &fakeAccountRepo{byID: map[string]*account.Account{acc.ID: acc}}
	return NewCompanyService(repo, &fakeCredentialRepo{}, nil, nil), repo
}

func TestRemoveMemberFromCompany_RevokesMembership(t *testing.T) {
	acc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member"},
	)
	svc, repo := newCompanyServiceWithRepo(t, acc)
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if err := svc.RemoveMemberFromCompany(context.Background(), "owner-1", acc.ID, "member-1"); err != nil {
		t.Fatalf("RemoveMemberFromCompany: %v", err)
	}

	wantKey := pkgAuth.MembershipRevocationKey(acc.ID, "member-1")
	if len(revoker.keys) != 1 || revoker.keys[0] != wantKey {
		t.Fatalf("vínculo removido sem marcar revogação: %v (esperado %s)", revoker.keys, wantKey)
	}
	if revoker.ttl != pkgAuth.MembershipRevocationTTL() {
		t.Fatalf("TTL da marca = %v, esperado %v", revoker.ttl, pkgAuth.MembershipRevocationTTL())
	}

	// O membro saiu de fato da empresa (a marca é adicional, não substituta).
	stored := repo.byID[acc.ID]
	for _, m := range stored.Company.Members {
		if m.AccountID == "member-1" {
			t.Fatal("o membro continua na empresa")
		}
	}
}

// Falha ao marcar (Redis indisponível) NÃO pode desfazer a remoção nem devolver
// erro: o vínculo já saiu da fonte de verdade.
func TestRemoveMemberFromCompany_RevocationFailureDoesNotBlockRemoval(t *testing.T) {
	acc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member"},
	)
	svc, repo := newCompanyServiceWithRepo(t, acc)
	svc.WithMembershipRevoker(&recordingRevoker{err: errors.New("redis down")})

	if err := svc.RemoveMemberFromCompany(context.Background(), "owner-1", acc.ID, "member-1"); err != nil {
		t.Fatalf("falha de revogação não deveria propagar: %v", err)
	}
	stored := repo.byID[acc.ID]
	for _, m := range stored.Company.Members {
		if m.AccountID == "member-1" {
			t.Fatal("o membro continua na empresa")
		}
	}
}

// Sem Redis configurado e sem revoker injetado, a remoção segue (registrando o
// aviso) — o acesso do removido cai no TTL do próprio token.
func TestRemoveMemberFromCompany_WithoutRedisStillRemoves(t *testing.T) {
	acc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member"},
	)
	svc, _ := newCompanyServiceWithRepo(t, acc)
	t.Setenv("REDIS_URI", "")

	if err := svc.RemoveMemberFromCompany(context.Background(), "owner-1", acc.ID, "member-1"); err != nil {
		t.Fatalf("remoção sem Redis deveria funcionar: %v", err)
	}
}

// fakeGroupRepo é o mínimo do repositório de grupos para exercitar a troca de
// papel (que é onde a revogação por mudança de papel acontece).
type fakeGroupRepo struct {
	group.Repository
	groups map[string]*group.Group
}

func (r *fakeGroupRepo) GetByID(_ context.Context, id string) (*group.Group, error) {
	return r.groups[id], nil
}

func (r *fakeGroupRepo) ListByCompanyID(context.Context, string) ([]*group.Group, error) {
	return nil, nil
}

func (r *fakeGroupRepo) Create(_ context.Context, g *group.Group) error {
	if r.groups == nil {
		r.groups = map[string]*group.Group{}
	}
	r.groups[g.ID] = g
	return nil
}

func (r *fakeGroupRepo) Update(_ context.Context, g *group.Group) error {
	if r.groups == nil {
		r.groups = map[string]*group.Group{}
	}
	r.groups[g.ID] = g
	return nil
}

func (r *fakeGroupRepo) Delete(_ context.Context, companyID, id string) error {
	delete(r.groups, id)
	return nil
}

func newGroupServiceForRole(t *testing.T) (*GroupService, *fakeAccountRepo) {
	t.Helper()
	acc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
	)
	repo := &fakeAccountRepo{byID: map[string]*account.Account{acc.ID: acc}}
	groups := &fakeGroupRepo{groups: map[string]*group.Group{
		"group-financeiro": {ID: "group-financeiro", CompanyID: acc.ID, RoleID: "financeiro"},
	}}
	return NewGroupService(groups, repo), repo
}

// P1/A15 (continuação) — o token carrega `functional_role`: mudar o papel do
// membro sem invalidar o token deixaria o papel ANTIGO valendo até expirar.
func TestAddMemberToGroup_RevokesWhenRoleChanges(t *testing.T) {
	svc, repo := newGroupServiceForRole(t)
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if err := svc.AddMemberToGroup(context.Background(), "owner-1", "company-1", "group-financeiro", "member-1"); err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}

	wantKey := pkgAuth.MembershipRevocationKey("company-1", "member-1")
	if len(revoker.keys) != 1 || revoker.keys[0] != wantKey {
		t.Fatalf("troca de papel sem revogação: %v (esperado %s)", revoker.keys, wantKey)
	}
	if repo.byID["company-1"].Company.Members[1].FunctionalRole != "financeiro" {
		t.Fatal("o papel não foi atualizado")
	}
}

// Idempotência: reaplicar o MESMO papel (o que o fluxo de entrada na empresa faz)
// não pode marcar revogação — senão o recém-chegado é derrubado no primeiro
// request.
func TestAddMemberToGroup_DoesNotRevokeWhenRoleIsUnchanged(t *testing.T) {
	svc, _ := newGroupServiceForRole(t)
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	// member-1 já é "corretor"; a remoção de grupo reaplica o default "corretor".
	if err := svc.RemoveMemberFromGroup(context.Background(), "owner-1", "company-1", "group-x", "member-1"); err != nil {
		t.Fatalf("RemoveMemberFromGroup: %v", err)
	}
	if len(revoker.keys) != 0 {
		t.Fatalf("papel inalterado marcou revogação: %v", revoker.keys)
	}
}

// Primeira atribuição de papel (convite/entrada) também não marca: marcar ali
// derrubaria o membro recém-convidado.
func TestRoleChange_FirstAssignmentDoesNotRevoke(t *testing.T) {
	acc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "member-new", Profile: "member"},
	)
	repo := &fakeAccountRepo{byID: map[string]*account.Account{acc.ID: acc}}
	groups := &fakeGroupRepo{groups: map[string]*group.Group{
		"group-corretor": {ID: "group-corretor", CompanyID: acc.ID, RoleID: "corretor"},
	}}
	svc := NewGroupService(groups, repo)
	revoker := &recordingRevoker{}
	svc.WithMembershipRevoker(revoker)

	if err := svc.AddMemberToGroup(context.Background(), "owner-1", acc.ID, "group-corretor", "member-new"); err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	if len(revoker.keys) != 0 {
		t.Fatalf("primeira atribuição de papel marcou revogação: %v", revoker.keys)
	}
}

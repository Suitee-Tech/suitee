package service

import (
	"context"
	"sync"
	"testing"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca-auth/internal/domain/invitation"
	"github.com/oginoino/toca/pkg/audit"
)

// P2/M4+M5 — eventos de controle de acesso precisam de trilha.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achados M4 e M5) registrou que não havia auditoria em membro, papel/perfil,
// convite e grupo: não havia como reconstituir "quem elevou quem". Os testes
// abaixo usam o coletor de teste do pkg/audit (sem Postgres) e checam o
// **contrato da trilha**: ação, entidade e o essencial do antes/depois.

type auditCollector struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (c *auditCollector) collect(e audit.Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *auditCollector) find(action string) (audit.Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		if e.Action == action {
			return e, true
		}
	}
	return audit.Entry{}, false
}

func withAuditCollector(t *testing.T) *auditCollector {
	t.Helper()
	c := &auditCollector{}
	cleanup := audit.SetTestSink(c.collect)
	t.Cleanup(cleanup)
	return c
}

func a8Company() *account.Account {
	return &account.Account{
		ID:   "company-1",
		Type: account.AccountTypeCompany,
		Company: &company.Company{
			OwnerID: "owner-1",
			Name:    "Imobiliária Teste",
			Members: []company.CompanyMember{
				{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
				{AccountID: "member-1", Profile: "member", FunctionalRole: "corretor"},
			},
		},
	}
}

// Remover membro deixa trilha com quem removeu e quem foi removido.
func TestAudit_RemoveMemberFromCompany(t *testing.T) {
	collector := withAuditCollector(t)
	svc, _ := newCompanyServiceWithRepo(t, a8Company())
	svc.WithMembershipRevoker(&recordingRevoker{})

	if err := svc.RemoveMemberFromCompany(context.Background(), "owner-1", "company-1", "member-1"); err != nil {
		t.Fatalf("RemoveMemberFromCompany: %v", err)
	}

	entry, ok := collector.find("company.member_removed")
	if !ok {
		t.Fatalf("remoção de membro sem trilha; coletado: %+v", collector.entries)
	}
	if entry.EntityType != "company_member" || entry.EntityID != "member-1" {
		t.Fatalf("trilha com entidade errada: %+v", entry)
	}
	if entry.Metadata["company_id"] != "company-1" || entry.Metadata["actor_id"] != "owner-1" {
		t.Fatalf("trilha sem empresa/ator: %+v", entry.Metadata)
	}
}

// Troca de papel é o evento de elevação de privilégio: trilha com antes/depois.
func TestAudit_RoleChange(t *testing.T) {
	collector := withAuditCollector(t)
	svc, repo := newGroupServiceForRole(t)
	svc.WithMembershipRevoker(&recordingRevoker{})

	if err := svc.AddMemberToGroup(context.Background(), "owner-1", "company-1", "group-financeiro", "member-1"); err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}

	entry, ok := collector.find("company.member_role_changed")
	if !ok {
		t.Fatalf("troca de papel sem trilha; coletado: %+v", collector.entries)
	}
	if entry.Before["functional_role"] != "corretor" || entry.After["functional_role"] != "financeiro" {
		t.Fatalf("trilha sem antes/depois do papel: before=%v after=%v", entry.Before, entry.After)
	}
	if repo.byID["company-1"].Company.Members[1].FunctionalRole != "financeiro" {
		t.Fatal("o papel não foi aplicado")
	}
}

// Idempotência: reaplicar o mesmo papel não gera trilha de mudança (o evento é
// de MUDANÇA; ruído na trilha esconde o evento real).
func TestAudit_RoleUnchangedDoesNotRecordChange(t *testing.T) {
	collector := withAuditCollector(t)
	svc, _ := newGroupServiceForRole(t)
	svc.WithMembershipRevoker(&recordingRevoker{})

	// member-1 já é "corretor"; remover do grupo reaplica o default "corretor".
	if err := svc.RemoveMemberFromGroup(context.Background(), "owner-1", "company-1", "group-x", "member-1"); err != nil {
		t.Fatalf("RemoveMemberFromGroup: %v", err)
	}
	if _, ok := collector.find("company.member_role_changed"); ok {
		t.Fatal("papel inalterado gerou trilha de mudança de papel")
	}
}

// Grupos: criação, edição e exclusão auditadas.
func TestAudit_GroupMutations(t *testing.T) {
	collector := withAuditCollector(t)
	svc, _ := newGroupServiceForRole(t)

	created, err := svc.CreateGroup(context.Background(), "owner-1", "company-1", &group.CreateGroupRequest{Name: "Equipe Sul"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, ok := collector.find("company.group_created"); !ok {
		t.Fatalf("criação de grupo sem trilha; coletado: %+v", collector.entries)
	}

	if _, err := svc.UpdateGroup(context.Background(), "owner-1", "company-1", created.ID, &group.UpdateGroupRequest{Name: "Equipe Sul 2"}); err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if _, ok := collector.find("company.group_updated"); !ok {
		t.Fatalf("edição de grupo sem trilha; coletado: %+v", collector.entries)
	}

	// O grupo criado não é um papel padrão, então pode ser excluído.
	if err := svc.DeleteGroup(context.Background(), "owner-1", "company-1", created.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if _, ok := collector.find("company.group_deleted"); !ok {
		t.Fatalf("exclusão de grupo sem trilha; coletado: %+v", collector.entries)
	}
}

// Convite: criado/reenviado/cancelado — e sem o token nem o e-mail em claro.
func TestAudit_InvitationMutations(t *testing.T) {
	collector := withAuditCollector(t)
	svc, _, companyID := newInvitationServiceForAudit(t)

	inv, err := svc.CreateInvitation(context.Background(), "owner-1", companyID, &invitation.CreateInvitationRequest{Email: "novo@example.com", Role: "member"})
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	entry, ok := collector.find("company.invitation_created")
	if !ok {
		t.Fatalf("criação de convite sem trilha; coletado: %+v", collector.entries)
	}
	if entry.After["email"] == "novo@example.com" {
		t.Fatalf("e-mail em claro na trilha (retenção de 5 anos): %v", entry.After)
	}
	if _, hasToken := entry.After["token"]; hasToken {
		t.Fatal("token do convite não pode ir para a trilha")
	}

	if err := svc.CancelInvitation(context.Background(), "owner-1", companyID, inv.ID); err != nil {
		t.Fatalf("CancelInvitation: %v", err)
	}
	if _, ok := collector.find("company.invitation_canceled"); !ok {
		t.Fatalf("cancelamento de convite sem trilha; coletado: %+v", collector.entries)
	}
}

// ───────────── dublês do convite (só o necessário para a trilha) ─────────────

type auditInvitationRepo struct {
	invitation.Repository
	mu    sync.Mutex
	items map[string]*invitation.Invitation
}

func (r *auditInvitationRepo) Create(_ context.Context, inv *invitation.Invitation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = map[string]*invitation.Invitation{}
	}
	r.items[inv.ID] = inv
	return nil
}

func (r *auditInvitationRepo) GetByEmailAndCompany(context.Context, string, string) (*invitation.Invitation, error) {
	return nil, nil
}

func (r *auditInvitationRepo) GetByToken(context.Context, string) (*invitation.Invitation, error) {
	return nil, nil
}

func (r *auditInvitationRepo) UpdateStatus(_ context.Context, companyID, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv, ok := r.items[id]; ok {
		inv.Status = status
	}
	return nil
}

func (r *auditInvitationRepo) ListByCompany(context.Context, string) ([]*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*invitation.Invitation, 0, len(r.items))
	for _, inv := range r.items {
		out = append(out, inv)
	}
	return out, nil
}

func newInvitationServiceForAudit(t *testing.T) (*InvitationService, *auditInvitationRepo, string) {
	t.Helper()
	acc := a8Company()
	accRepo := &fakeAccountRepo{byID: map[string]*account.Account{acc.ID: acc}}
	inviteRepo := &auditInvitationRepo{}
	// AuthService vazio (não nil): CreateInvitation consulta tenantSettingsRepo
	// para o papel padrão e cai no default "corretor" quando ele é nil.
	svc := NewInvitationService(accRepo, &fakeCredentialRepo{}, inviteRepo, &AuthService{}, nil, nil, &app.Config{}, nil, nil, nil, nil)
	return svc, inviteRepo, acc.ID
}

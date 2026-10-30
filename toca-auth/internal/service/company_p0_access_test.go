package service

import (
	"context"
	"strings"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P0 / C1, C2, C3 — superfície de empresas, convites e vínculos do toca-auth.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md)
// mostrou que o grupo `/api/v1/companies/*` não tinha verificação de
// permissão nem de vínculo:
//   C1: GET/PUT /companies/:id liam e sobrescreviam qualquer empresa pelo UUID;
//   C2: GET /companies/:id/invitations devolvia o TOKEN do convite para
//       qualquer membro (bearer de uso único; com papel admin, principal root);
//   C3: POST /companies/:id/users gravava `profile`/`role` crus do corpo, e
//       `profile` vira `app_metadata.permission` no switch-context — fabricando
//       principais root.

func p0CompanyFixture() (*CompanyService, *fakeAccountRepo, *account.Account) {
	comp := &account.Account{
		ID:   "company-1",
		Type: account.AccountTypeCompany,
		Company: &company.Company{
			ID:      "company-1",
			Name:    "Imobiliária Alvo",
			TaxID:   "12345678000199",
			OwnerID: "owner-1",
			Members: []company.CompanyMember{
				{AccountID: "owner-1", Profile: "owner", FunctionalRole: "admin"},
				{AccountID: "admin-1", Profile: "admin", FunctionalRole: "gestor"},
				{AccountID: "broker-1", Profile: "member", FunctionalRole: "corretor"},
			},
		},
	}
	repo := &fakeAccountRepo{byID: map[string]*account.Account{
		"company-1": comp,
		"user-9":    {ID: "user-9", Type: account.AccountTypeIndividual},
	}}
	svc := NewCompanyService(repo, nil, nil, nil)
	return svc, repo, comp
}

// ── C1 ──────────────────────────────────────────────────────────────────────

func TestGetCompany_DeniesNonMember(t *testing.T) {
	svc, _, _ := p0CompanyFixture()
	ctx := pkgcontext.WithTenantID(context.Background(), "outro-tenant")

	if got, err := svc.GetCompany(ctx, "atacante-1", "company-1"); err == nil {
		t.Fatalf("VAZAMENTO CROSS-TENANT: empresa devolvida para não-membro: %+v", got)
	}
}

func TestGetCompany_AllowsMember(t *testing.T) {
	svc, _, _ := p0CompanyFixture()
	ctx := pkgcontext.WithTenantID(context.Background(), "company-1")

	got, err := svc.GetCompany(ctx, "broker-1", "company-1")
	if err != nil {
		t.Fatalf("membro deveria ver a própria empresa: %v", err)
	}
	if got == nil || got.ID != "company-1" {
		t.Fatalf("empresa errada: %+v", got)
	}
}

func TestUpdateCompanyAccount_DeniesNonAdmin(t *testing.T) {
	svc, _, comp := p0CompanyFixture()
	before := comp.Company.Name

	err := svc.UpdateCompanyAccount(context.Background(), "broker-1", "company-1", &company.Company{Name: "Invadida"})
	if err == nil {
		t.Fatal("ESCRITA CROSS-TENANT: corretor alterou os dados cadastrais da empresa")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("esperava unauthorized, veio %v", err)
	}
	if comp.Company.Name != before {
		t.Fatalf("nome da empresa mudou para %q", comp.Company.Name)
	}
}

func TestUpdateCompanyAccount_AllowsOwnerAndAdmin(t *testing.T) {
	for _, actor := range []string{"owner-1", "admin-1"} {
		svc, _, comp := p0CompanyFixture()
		if err := svc.UpdateCompanyAccount(context.Background(), actor, "company-1", &company.Company{Name: "Novo Nome"}); err != nil {
			t.Fatalf("ator %s deveria poder alterar: %v", actor, err)
		}
		if comp.Company.Name != "Novo Nome" {
			t.Fatalf("ator %s não alterou o nome", actor)
		}
	}
}

// ── C3 ──────────────────────────────────────────────────────────────────────

func TestAddUserToCompany_RejectsEscalatingProfileByNonOwner(t *testing.T) {
	svc, _, comp := p0CompanyFixture()

	for _, profile := range []string{"owner", "admin"} {
		_, err := svc.AddUserToCompany(context.Background(), "company-1", "admin-1", &auth.AddUserRequest{
			AccountID: "user-9", Profile: profile, Role: "corretor",
		})
		if err == nil {
			t.Fatalf("ESCALADA: admin não-proprietário concedeu profile %q", profile)
		}
		if len(comp.Company.Members) != 3 {
			t.Fatalf("vínculo foi criado apesar da recusa (profile=%s)", profile)
		}
	}
}

func TestAddUserToCompany_RejectsInvalidProfileAndRole(t *testing.T) {
	svc, _, comp := p0CompanyFixture()

	for _, tc := range []struct{ profile, role string }{
		{"superuser", "corretor"},
		{"member", "root"},
		{"member", "service"},
		{"member", "admin; DROP"},
	} {
		_, err := svc.AddUserToCompany(context.Background(), "company-1", "owner-1", &auth.AddUserRequest{
			AccountID: "user-9", Profile: tc.profile, Role: tc.role,
		})
		if err == nil {
			t.Fatalf("valores inválidos aceitos: profile=%q role=%q", tc.profile, tc.role)
		}
		if len(comp.Company.Members) != 3 {
			t.Fatalf("vínculo criado com valores inválidos (profile=%q role=%q)", tc.profile, tc.role)
		}
	}
}

func TestAddUserToCompany_OwnerCanGrantAdminAndAuditsIt(t *testing.T) {
	svc, _, comp := p0CompanyFixture()

	_, err := svc.AddUserToCompany(context.Background(), "company-1", "owner-1", &auth.AddUserRequest{
		AccountID: "user-9", Profile: "admin", Role: "gestor",
	})
	if err != nil {
		t.Fatalf("proprietário deveria poder conceder admin: %v", err)
	}
	if len(comp.Company.Members) != 4 {
		t.Fatalf("vínculo não foi criado: %d membros", len(comp.Company.Members))
	}
	last := comp.Company.Members[len(comp.Company.Members)-1]
	if last.Profile != "admin" || last.FunctionalRole != "gestor" {
		t.Fatalf("vínculo gravado errado: %+v", last)
	}
}

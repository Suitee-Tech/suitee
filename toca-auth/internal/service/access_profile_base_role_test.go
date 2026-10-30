package service

import (
	"context"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/user"
)

// Incidente de 2026-09-21 — "o novo administrador não vê leads de corretores
// que deveria ver assim como gestor".
//
// Quem tem PERFIL DE ACESSO personalizado (`cr_…`) recebe as permissões do
// perfil, mas o id não casa com nenhum papel do catálogo — e a decisão de
// VISIBILIDADE (carteira x tenant) usa o papel. O token do contexto da empresa
// passou a levar também o PAPEL BASE do perfil (`base_role`), resolvido da
// matriz do tenant na hora de emitir: é o que faz "Gestor Comercial" (base
// gestor) enxergar o tenant como o gestor enxerga.

type fakeTenantSettings struct {
	baseRole    string
	consultedID string
}

func (f *fakeTenantSettings) GetByTenantID(context.Context, string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (f *fakeTenantSettings) GetDefaultRole(context.Context, string) (string, error) {
	return "corretor", nil
}

func (f *fakeTenantSettings) GetAccessProfileBaseRole(_ context.Context, _, profileID string) (string, error) {
	f.consultedID = profileID
	return f.baseRole, nil
}

func switchContextWithRole(t *testing.T, role string, settings *fakeTenantSettings) (*fakeSupabase, error) {
	t.Helper()
	companyAcc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "acc1", Profile: "member", FunctionalRole: role},
	)
	repo := &fakeAccountRepo{byID: map[string]*account.Account{
		companyAcc.ID: companyAcc,
		"acc1": {ID: "acc1", Type: account.AccountTypeIndividual,
			Individual: &user.User{ID: "acc1", IsActive: true}},
	}}
	sb := &fakeSupabase{}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, sb)
	svc.tenantSettingsRepo = settings

	_, err := svc.SwitchContext(context.Background(), "acc1", companyAcc.ID, "refresh")
	return sb, err
}

func TestSwitchContext_PublishesBaseRoleOfAccessProfile(t *testing.T) {
	settings := &fakeTenantSettings{baseRole: "gestor"}
	sb, err := switchContextWithRole(t, "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301", settings)
	if err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}

	if got := sb.updateMeta["functional_role"]; got != "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301" {
		t.Fatalf("functional_role = %v, esperado o id do perfil", got)
	}
	if got := sb.updateMeta["base_role"]; got != "gestor" {
		t.Fatalf("base_role = %v, esperado gestor (papel base do perfil)", got)
	}
	if settings.consultedID != "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301" {
		t.Fatalf("o papel base foi resolvido para outro id: %q", settings.consultedID)
	}
}

func TestSwitchContext_CatalogRoleHasNoBaseRole(t *testing.T) {
	settings := &fakeTenantSettings{baseRole: "gestor"}
	sb, err := switchContextWithRole(t, "corretor", settings)
	if err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}

	if got, ok := sb.updateMeta["base_role"]; !ok || got != "" {
		t.Fatalf("papel do catálogo não deveria carregar base_role: %v (%v)", got, ok)
	}
	if settings.consultedID != "" {
		t.Fatalf("papel do catálogo não deveria consultar perfis: %q", settings.consultedID)
	}
}

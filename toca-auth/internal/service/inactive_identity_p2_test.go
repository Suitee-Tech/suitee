package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/user"
)

// P2/M7 — `IsActive` nunca era lido.
//
// A revisão de permissões de 2026-09-19 (achado M7) registrou que o campo era
// escrito `true` no cadastro e não existia caminho para bloquear um usuário sem
// apagar a conta: o fluxo LGPD marcava inativo e o login continuava funcionando.
// Estes testes cobrem os dois pontos citados na recomendação ("checar IsActive no
// middleware/login"): a emissão de token no login e a troca de contexto.
//
// (O "middleware" é coberto pelo mecanismo do A15 — a marca de revogação por
// (tenant, account) que o pkg/auth consulta em cada requisição —, escrita pelo
// UpdateCompanyMember na suspensão e testada em update_member_p2_test.go.)

func inactiveUserAccount() *account.Account {
	return &account.Account{
		ID:   "acc1",
		Type: account.AccountTypeIndividual,
		Individual: &user.User{
			ID:       "acc1",
			Email:    "inativo@example.com",
			FullName: "Usuário Inativo",
			IsActive: false,
		},
	}
}

func TestLogin_DeniesDeactivatedAccount(t *testing.T) {
	repo := &fakeAccountRepo{byID: map[string]*account.Account{"acc1": inactiveUserAccount()}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, &fakeSupabase{})

	_, err := svc.Login(context.Background(), &auth.LoginRequest{Email: "inativo@example.com", Password: "senha-valida"})
	if !errors.Is(err, ErrAccountInactive) {
		t.Fatalf("login de conta desativada devolveu %v, want ErrAccountInactive", err)
	}
}

func TestLogin_AllowsActiveAccount(t *testing.T) {
	active := inactiveUserAccount()
	active.Individual.IsActive = true
	repo := &fakeAccountRepo{byID: map[string]*account.Account{"acc1": active}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, &fakeSupabase{})

	tokens, err := svc.Login(context.Background(), &auth.LoginRequest{Email: "ativo@example.com", Password: "senha-valida"})
	if err != nil {
		t.Fatalf("login de conta ativa falhou: %v", err)
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Fatalf("login de conta ativa não devolveu token: %+v", tokens)
	}
}

func TestOAuthLogin_DeniesDeactivatedAccount(t *testing.T) {
	repo := &fakeAccountRepo{byID: map[string]*account.Account{"acc1": inactiveUserAccount()}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, &fakeSupabase{})

	_, err := svc.OAuthLogin(context.Background(), &auth.OAuthLoginRequest{RefreshToken: "refresh"})
	if !errors.Is(err, ErrAccountInactive) {
		t.Fatalf("OAuth de conta desativada devolveu %v, want ErrAccountInactive", err)
	}
}

func TestSwitchContext_DeniesSuspendedMembership(t *testing.T) {
	companyAcc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "acc1", Profile: "member", FunctionalRole: "corretor", IsActive: boolPtr(false)},
	)
	repo := &fakeAccountRepo{byID: map[string]*account.Account{
		companyAcc.ID: companyAcc,
		"acc1":        {ID: "acc1", Type: account.AccountTypeIndividual, Individual: &user.User{ID: "acc1", IsActive: true}},
	}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, &fakeSupabase{})

	_, err := svc.SwitchContext(context.Background(), "acc1", companyAcc.ID, "refresh")
	if !errors.Is(err, ErrMembershipInactive) {
		t.Fatalf("switch-context de vínculo suspenso devolveu %v, want ErrMembershipInactive", err)
	}
}

// Vínculo sem o campo (dados anteriores ao P2/M7) continua podendo trocar de
// contexto — a suspensão é explícita.
func TestSwitchContext_AllowsLegacyMembershipWithoutFlag(t *testing.T) {
	companyAcc := companyWithMembers("owner-1",
		company.CompanyMember{AccountID: "owner-1", Profile: "admin", FunctionalRole: "admin"},
		company.CompanyMember{AccountID: "acc1", Profile: "member", FunctionalRole: "corretor"},
	)
	repo := &fakeAccountRepo{byID: map[string]*account.Account{
		companyAcc.ID: companyAcc,
		"acc1":        {ID: "acc1", Type: account.AccountTypeIndividual, Individual: &user.User{ID: "acc1", IsActive: true}},
	}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, &fakeSupabase{})

	tokens, err := svc.SwitchContext(context.Background(), "acc1", companyAcc.ID, "refresh")
	if err != nil {
		t.Fatalf("vínculo legado deveria continuar válido: %v", err)
	}
	if tokens == nil {
		t.Fatal("switch-context não devolveu tokens")
	}
}

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/auth"
)

// P1/A14 — troca de senha precisa trocar a senha (e revogar as sessões).
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A14) registrou três defeitos no mesmo fluxo:
//
//  1. `UpdatePassword` gravava em `toca_auth.credentials`, tabela que NENHUM
//     caminho de login lê (o login é do Supabase) — a senha antiga continuava
//     valendo;
//  2. a senha atual era conferida contra o hash dessa tabela morta;
//  3. `LogoutAll(ctx, accountID)` passava o **account id** onde o Supabase espera
//     um access token, e o erro era descartado — nenhuma sessão era invalidada.
//
// Os testes abaixo travam o comportamento novo: verificação pelo provedor,
// troca NO provedor e revogação com o access token do chamador.

type a14Repo struct {
	auth.Repository
	cred *auth.Credential
}

func (r *a14Repo) GetByAccountID(context.Context, string) (*auth.Credential, error) {
	return r.cred, nil
}

func newA14Service(t *testing.T, sb *fakeSupabase, cred *auth.Credential) *PasswordService {
	t.Helper()
	authSvc := newTestAuthService(&fakeAccountRepo{}, &a14Repo{cred: cred}, sb)
	return NewPasswordService(&a14Repo{cred: cred}, authSvc, nil, &app.Config{BcryptCost: 4})
}

func a14Credential() *auth.Credential {
	return &auth.Credential{
		ID:           "cred1",
		AccountID:    "acc1",
		Email:        "user@example.com",
		PasswordHash: "$2a$10$hash-morto-que-ninguem-le",
	}
}

// Caminho feliz: senha verificada no provedor, trocada NO provedor e sessões
// revogadas com o ACCESS TOKEN (não o account id).
func TestChangePassword_UpdatesSupabaseAndRevokesWithAccessToken(t *testing.T) {
	sb := &fakeSupabase{}
	svc := newA14Service(t, sb, a14Credential())

	if err := svc.ChangePassword(context.Background(), "access-token-real", "acc1", "SenhaAtual1!", "NovaSenha2@"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if len(sb.passwordUpdates) != 1 {
		t.Fatalf("a senha NÃO foi trocada no provedor: %+v", sb.passwordUpdates)
	}
	if sb.passwordUpdates[0].userID != "acc1" || sb.passwordUpdates[0].password != "NovaSenha2@" {
		t.Fatalf("troca de senha com alvo/valor errados: %+v", sb.passwordUpdates[0])
	}

	if len(sb.logoutAllTokens) != 1 {
		t.Fatalf("sessões não foram revogadas: %+v", sb.logoutAllTokens)
	}
	if got := sb.logoutAllTokens[0]; got != "access-token-real" {
		t.Fatalf("VAZAMENTO DE REVOGAÇÃO: LogoutAll recebeu %q — o Supabase espera o ACCESS TOKEN, não o account id (A14)", got)
	}
}

// Senha atual errada: nada é trocado e nada é revogado.
func TestChangePassword_RejectsWrongCurrentPasswordWithoutChangingAnything(t *testing.T) {
	sb := &fakeSupabase{signInErr: errors.New("invalid login credentials")}
	svc := newA14Service(t, sb, a14Credential())

	err := svc.ChangePassword(context.Background(), "access", "acc1", "SenhaErrada!", "NovaSenha2@")
	if err == nil || !strings.Contains(err.Error(), "invalid current password") {
		t.Fatalf("esperava erro de senha atual, veio %v", err)
	}
	if len(sb.passwordUpdates) != 0 {
		t.Fatalf("a senha foi trocada apesar da senha atual inválida: %+v", sb.passwordUpdates)
	}
	if len(sb.logoutAllTokens) != 0 {
		t.Fatal("sessões foram revogadas mesmo sem troca de senha")
	}
}

// Falha ao revogar não pode passar como sucesso silencioso (era `_ =` antes).
func TestChangePassword_SurfacesRevocationFailure(t *testing.T) {
	sb := &fakeSupabase{logoutAllErr: errors.New("supabase indisponível")}
	svc := newA14Service(t, sb, a14Credential())

	err := svc.ChangePassword(context.Background(), "access", "acc1", "SenhaAtual1!", "NovaSenha2@")
	if err == nil || !strings.Contains(err.Error(), "revoke") {
		t.Fatalf("falha de revogação foi engolida: %v", err)
	}
}

// Complexidade fraca é recusada antes de tocar no provedor.
func TestChangePassword_EnforcesComplexityBeforeCallingProvider(t *testing.T) {
	sb := &fakeSupabase{}
	svc := newA14Service(t, sb, a14Credential())

	if err := svc.ChangePassword(context.Background(), "access", "acc1", "SenhaAtual1!", "123"); err == nil {
		t.Fatal("senha fraca foi aceita")
	}
	if len(sb.passwordUpdates) != 0 {
		t.Fatal("o provedor foi chamado com senha que não passa na política")
	}
}

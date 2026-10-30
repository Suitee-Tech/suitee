package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/company"
	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
)

// P0/A15 (reauditoria de 2026-09-20). O teste cobre o que a revisão anterior não
// cobria: o refresh DEPOIS que a marca de revogação do Redis expira.
//
// Antes da correção, `RefreshToken` devolvia o par novo sem olhar o vínculo — o
// Supabase reemite o `app_metadata` antigo (nem a remoção nem a troca de papel o
// limpam), então o ex-membro voltava a entrar assim que a marca (TTL default 1h)
// sumia, e um `admin` rebaixado mantinha o bypass de root. Estes casos falhavam
// antes e passam depois.

const (
	testTenant  = "tenant-abc"
	testAccount = "acc-1"
	testOther   = "acc-2"
)

// jwtWithContext monta um access token como o `SwitchContext` emite: tenant,
// conta, perfil e papel funcional no `app_metadata`. A assinatura é irrelevante
// para este gate — o token acabou de vir do Supabase e a decisão é só "devolver
// ou não ao cliente".
func jwtWithContext(t *testing.T, tenantID, accountID, permission, functionalRole string) string {
	t.Helper()
	appMeta := map[string]any{
		"tenant_id":  tenantID,
		"account_id": accountID,
	}
	if permission != "" {
		appMeta["permission"] = permission
	}
	if functionalRole != "" {
		appMeta["functional_role"] = functionalRole
	}
	payload, err := json.Marshal(map[string]any{"sub": accountID, "app_metadata": appMeta})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

// refreshFakeSupabase implementa só o RefreshToken; os demais métodos vêm da
// interface embutida (nil) e não são chamados por este caminho.
type refreshFakeSupabase struct {
	supabaseauth.Client
	accessToken string
}

func (f *refreshFakeSupabase) RefreshToken(_ context.Context, _ string) (*supabaseauth.Session, error) {
	return &supabaseauth.Session{
		AccessToken:  f.accessToken,
		RefreshToken: "refresh-new",
		UserID:       testAccount,
	}, nil
}

// refreshFakeAccountRepo responde GetByID com a conta montada no teste.
type refreshFakeAccountRepo struct {
	account.Repository
	acc *account.Account
	err error
}

func (r *refreshFakeAccountRepo) GetByID(_ context.Context, _ string) (*account.Account, error) {
	return r.acc, r.err
}

func inactive() *bool { v := false; return &v }
func active() *bool   { v := true; return &v }

// companyWith devolve a empresa do tenant com os vínculos informados.
func companyWith(ownerID string, members ...company.CompanyMember) *account.Account {
	return &account.Account{
		ID:      testTenant,
		Type:    account.AccountTypeCompany,
		Company: &company.Company{ID: testTenant, OwnerID: ownerID, Members: members},
	}
}

// individualAccount é o escopo PESSOAL gravado pelo `Login`
// (`tenant_id = acc.ID`) antes de o usuário escolher a empresa.
func individualAccount(id string) *account.Account {
	return &account.Account{ID: id, Type: account.AccountTypeIndividual}
}

func newRefreshService(t *testing.T, acc *account.Account, tenantID, accountID, permission, role string) *AuthService {
	t.Helper()
	return &AuthService{
		accountRepo:  &refreshFakeAccountRepo{acc: acc},
		supabaseAuth: &refreshFakeSupabase{accessToken: jwtWithContext(t, tenantID, accountID, permission, role)},
	}
}

func TestRefreshToken_RejectsRemovedMember(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: "owner-1", Profile: "owner", FunctionalRole: "admin"},
			company.CompanyMember{AccountID: testOther, Profile: "member", FunctionalRole: "corretor"},
		), testTenant, testAccount, "member", "corretor")

	tokens, err := svc.RefreshToken(context.Background(), "refresh-velho")
	if !errors.Is(err, ErrMembershipNoLongerActive) {
		t.Fatalf("membro removido: err = %v, want ErrMembershipNoLongerActive", err)
	}
	if tokens != nil {
		t.Fatalf("membro removido: esperava nenhum token, veio %+v", tokens)
	}
}

func TestRefreshToken_RejectsSuspendedMember(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member", FunctionalRole: "corretor", IsActive: inactive()},
		), testTenant, testAccount, "member", "corretor")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); !errors.Is(err, ErrMembershipNoLongerActive) {
		t.Fatalf("membro suspenso: err = %v, want ErrMembershipNoLongerActive", err)
	}
}

func TestRefreshToken_AllowsActiveMember(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member", FunctionalRole: "corretor", IsActive: active()},
		), testTenant, testAccount, "member", "corretor")

	tokens, err := svc.RefreshToken(context.Background(), "refresh-velho")
	if err != nil {
		t.Fatalf("membro ativo: err inesperado: %v", err)
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Fatalf("membro ativo: esperava tokens, veio %+v", tokens)
	}
}

// O RESIDUAL desta rodada: rebaixamento de PERFIL. O token ainda diz "admin", e
// `IsRootProfile` ignora a matriz inteira — sem esta checagem o rebaixado
// mantinha bypass de root até o token expirar por conta própria.
func TestRefreshToken_RejectsDemotedProfile(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member", FunctionalRole: "corretor"},
		), testTenant, testAccount, "admin", "admin")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); !errors.Is(err, ErrMembershipContextStale) {
		t.Fatalf("perfil rebaixado: err = %v, want ErrMembershipContextStale", err)
	}
}

// E rebaixamento de PAPEL, que governa a matriz de permissões do tenant.
func TestRefreshToken_RejectsDemotedRole(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member", FunctionalRole: "corretor"},
		), testTenant, testAccount, "member", "gestor")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); !errors.Is(err, ErrMembershipContextStale) {
		t.Fatalf("papel rebaixado: err = %v, want ErrMembershipContextStale", err)
	}
}

// Promoção também força novo login: o token vigente carrega o contexto antigo e
// é mais barato reemití-lo do que remendar.
func TestRefreshToken_RejectsPromotedRole(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member", FunctionalRole: "gestor"},
		), testTenant, testAccount, "member", "corretor")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); !errors.Is(err, ErrMembershipContextStale) {
		t.Fatalf("papel promovido: err = %v, want ErrMembershipContextStale", err)
	}
}

// ESTADO PÓS-LOGIN: `Login` grava `tenant_id = acc.ID` (a própria conta) e o
// usuário ainda vai escolher a empresa. Não há vínculo a revalidar, e recusar
// aqui derrubaria o refresh de quem está na tela de seleção.
func TestRefreshToken_AllowsPersonalScopeToken(t *testing.T) {
	svc := newRefreshService(t, individualAccount(testAccount), testAccount, testAccount, "", "")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); err != nil {
		t.Fatalf("escopo pessoal: err inesperado: %v", err)
	}
}

// O dono pode não constar da lista de membros — não pode ser barrado.
func TestRefreshToken_AllowsOwnerNotListedAsMember(t *testing.T) {
	svc := newRefreshService(t, companyWith(testAccount), testTenant, testAccount, "owner", "admin")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); err != nil {
		t.Fatalf("dono: err inesperado: %v", err)
	}
}

// Registro legado sem `functional_role`: o eixo vazio não é comparado, senão o
// token viraria logout por um campo que nunca existiu.
func TestRefreshToken_AllowsLegacyMemberWithoutRole(t *testing.T) {
	svc := newRefreshService(t,
		companyWith("owner-1",
			company.CompanyMember{AccountID: testAccount, Profile: "member"},
		), testTenant, testAccount, "member", "")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); err != nil {
		t.Fatalf("registro legado: err inesperado: %v", err)
	}
}

// Token pessoal (sem tenant) não tem vínculo a revalidar.
func TestRefreshToken_AllowsPersonalTokenWithoutTenant(t *testing.T) {
	svc := newRefreshService(t, nil, "", testAccount, "", "")

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); err != nil {
		t.Fatalf("token pessoal: err inesperado: %v", err)
	}
}

// O portal tem modelo de vínculo próprio (lead/deal, não CompanyMember): a
// checagem não se aplica e não pode derrubar o refresh do cliente.
func TestRefreshToken_PortalSkipsMembershipCheck(t *testing.T) {
	svc := newRefreshService(t, companyWith("owner-1"), testTenant, testAccount, "cliente", "cliente")

	if _, err := svc.RefreshToken(context.Background(), "refresh-portal", true); err != nil {
		t.Fatalf("portal: err inesperado: %v", err)
	}
}

// Falha de leitura não pode derrubar todo o refresh (fail-open deliberado, como
// no middleware), mas o caminho existe para não virar decisão implícita.
func TestRefreshToken_MembershipReadFailureDoesNotBlock(t *testing.T) {
	svc := &AuthService{
		accountRepo:  &refreshFakeAccountRepo{err: errors.New("db fora do ar")},
		supabaseAuth: &refreshFakeSupabase{accessToken: jwtWithContext(t, testTenant, testAccount, "member", "corretor")},
	}

	if _, err := svc.RefreshToken(context.Background(), "refresh-velho"); err != nil {
		t.Fatalf("falha de leitura não deve bloquear: %v", err)
	}
}

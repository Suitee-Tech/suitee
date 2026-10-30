package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca/pkg/logger"
)

// P0/A15 (reauditoria de 2026-09-20) — revogação DURÁVEL de vínculo.
//
// O mecanismo do A15 grava uma marca no Redis com TTL (default 1h) e o
// middleware a consulta a cada requisição. Isso protege a janela do token
// VIGENTE, mas não o que vem depois: a remoção não limpa o `app_metadata` do
// Supabase e `RefreshToken` apenas repassava o refresh ao GoTrue. Passada a
// hora, o ex-membro chamava `POST /auth/refresh`, recebia um token novo com o
// `app_metadata` antigo (tenant + papel) e voltava a entrar. A janela real era
// a vida do refresh token, não o TTL — remover um membro não revogava nada de
// forma durável.
//
// A correção não estende a marca: revalida o VÍNCULO na origem (o documento da
// empresa), que não expira. Cobre três casos, todos alimentados pelo mesmo
// refresh que reemite o `app_metadata` antigo:
//
//  1. removido — não consta mais da lista de membros;
//  2. suspenso — `is_active: false`;
//  3. contexto obsoleto — continua membro, mas com PERFIL ou PAPEL diferente do
//     que o token carrega (rebaixamento). Sem isto, um `admin` rebaixado para
//     `member` seguia com `permission: "admin"` no token, e `IsRootProfile`
//     ignora a matriz inteira: um bypass de root que sobrevivia à marca.
var (
	ErrMembershipNoLongerActive = errors.New("membership revoked: sign in again")
	// ErrMembershipContextStale força novo login (que reemite o contexto a
	// partir do vínculo atual) em vez de tentar remendar o token vigente.
	ErrMembershipContextStale = errors.New("session context changed: sign in again")
)

// tokenContext é o que o access token afirma sobre a sessão.
type tokenContext struct {
	TenantID       string
	AccountID      string
	Permission     string
	FunctionalRole string
}

// accessTokenContext lê o payload do access token recém-emitido pelo Supabase.
//
// A assinatura NÃO é verificada aqui de propósito. Este token não está sendo
// autenticado — ele acabou de chegar do Supabase, por TLS, dentro desta mesma
// chamada — e a decisão é apenas "devolver ou não este par ao cliente". Quem
// autentica continua sendo `pkg/auth.Middleware`, que verifica a assinatura em
// cada serviço.
func accessTokenContext(accessToken string) (tokenContext, bool) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return tokenContext{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenContext{}, false
	}
	var claims struct {
		Sub         string `json:"sub"`
		Permission  string `json:"permission"`
		Functional  string `json:"functional_role"`
		AppMetadata struct {
			TenantID       string `json:"tenant_id"`
			AccountID      string `json:"account_id"`
			Permission     string `json:"permission"`
			FunctionalRole string `json:"functional_role"`
		} `json:"app_metadata"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenContext{}, false
	}
	accountID := strings.TrimSpace(claims.AppMetadata.AccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(claims.Sub)
	}
	// `app_metadata` é a fonte (é o que o middleware lê via claims); as claims
	// de topo ficam como fallback para tokens emitidos por outra via.
	permission := strings.TrimSpace(claims.AppMetadata.Permission)
	if permission == "" {
		permission = strings.TrimSpace(claims.Permission)
	}
	role := strings.TrimSpace(claims.AppMetadata.FunctionalRole)
	if role == "" {
		role = strings.TrimSpace(claims.Functional)
	}
	return tokenContext{
		TenantID:       strings.TrimSpace(claims.AppMetadata.TenantID),
		AccountID:      accountID,
		Permission:     permission,
		FunctionalRole: role,
	}, true
}

// membershipState é o vínculo do token com o tenant que ele próprio afirma.
type membershipState struct {
	// applicable=false quando o token NÃO está escopado a uma empresa — é o
	// estado logo após o login, em que `Login` grava `tenant_id = acc.ID` (a
	// própria conta) e o usuário ainda vai escolher a empresa em
	// switch-context. Não há vínculo a revalidar, e recusar aqui derrubaria o
	// refresh de quem está na tela de seleção de empresa.
	applicable bool
	active     bool
	profile    string
	role       string
}

// resolveMembership descreve o vínculo de `accountID` com o tenant do token.
func (s *AuthService) resolveMembership(ctx context.Context, tenantID, accountID string) (membershipState, error) {
	acc, err := s.accountRepo.GetByID(ctx, tenantID)
	if err != nil {
		return membershipState{}, err
	}
	if acc == nil || acc.Company == nil {
		// Conta inexistente ou sem empresa: token de escopo pessoal.
		return membershipState{}, nil
	}

	for _, member := range acc.Company.Members {
		if member.AccountID != accountID {
			continue
		}
		return membershipState{
			applicable: true,
			active:     member.Active(),
			profile:    strings.ToLower(strings.TrimSpace(member.Profile)),
			role:       strings.ToLower(strings.TrimSpace(member.FunctionalRole)),
		}, nil
	}

	// O dono pode não constar da lista de membros (o resto do código o trata
	// como membro por `Company.OwnerID`). `SwitchContext` normaliza o dono para
	// o papel `admin`, que é o que um token dele carrega.
	if strings.TrimSpace(acc.Company.OwnerID) == accountID {
		return membershipState{applicable: true, active: true, profile: "owner", role: "admin"}, nil
	}

	// A empresa existe e a conta não é membro: removido.
	return membershipState{applicable: true, active: false}, nil
}

// contextMatches diz se perfil e papel do token ainda correspondem ao vínculo.
//
// Um eixo só é comparado quando o vínculo tem valor para ele: registro legado
// sem `functional_role` não pode virar logout. Quando os dois lados têm valor, a
// comparação é exata — é o contrato de `SwitchContext`, que grava
// `permission = member.Profile` e `functional_role = member.FunctionalRole`.
func contextMatches(state membershipState, tok tokenContext) bool {
	if state.profile != "" && strings.ToLower(strings.TrimSpace(tok.Permission)) != state.profile {
		return false
	}
	if state.role != "" && strings.ToLower(strings.TrimSpace(tok.FunctionalRole)) != state.role {
		return false
	}
	return true
}

// assertMembershipOnRefresh recusa o refresh quando o contexto que o token
// carrega não corresponde mais ao vínculo: removido, suspenso ou rebaixado.
//
// Falha de leitura NÃO derruba o refresh — mesma razão do fail-open deliberado
// do middleware (indisponibilidade de infraestrutura não pode parar todo o
// tráfego autenticado) — mas fica registrada com evento próprio para alarme.
func (s *AuthService) assertMembershipOnRefresh(ctx context.Context, session *supabaseauth.Session) error {
	if session == nil {
		return nil
	}
	tok, ok := accessTokenContext(session.AccessToken)
	if !ok {
		logger.Warn("auth.refresh_context_unreadable",
			"event", "auth.refresh_context_unreadable",
			"user_id", session.UserID)
		return nil
	}
	if tok.TenantID == "" || tok.AccountID == "" {
		return nil
	}

	state, err := s.resolveMembership(ctx, tok.TenantID, tok.AccountID)
	if err != nil {
		logger.Warn("auth.refresh_membership_check_failed",
			"event", "auth.refresh_membership_check_failed",
			"tenant_id", tok.TenantID, "account_id", tok.AccountID, "error", err)
		return nil
	}
	if !state.applicable {
		// Escopo pessoal (pós-login, antes do switch-context).
		return nil
	}
	if !state.active {
		logger.Warn("auth.refresh_membership_revoked",
			"event", "auth.refresh_membership_revoked",
			"tenant_id", tok.TenantID, "account_id", tok.AccountID)
		return ErrMembershipNoLongerActive
	}
	if !contextMatches(state, tok) {
		logger.Warn("auth.refresh_context_stale",
			"event", "auth.refresh_context_stale",
			"tenant_id", tok.TenantID, "account_id", tok.AccountID,
			"token_profile", tok.Permission, "member_profile", state.profile,
			"token_role", tok.FunctionalRole, "member_role", state.role)
		return ErrMembershipContextStale
	}
	return nil
}

package auth

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// P1/A15 — revogação de vínculo (remoção de membro não invalidava o token).
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A15) registrou que o middleware nunca revalida vínculo/sessão: o access
// token continua valendo até expirar, então remover um membro da empresa (ou
// trocar o papel dele) não tira o acesso dele.
//
// O mecanismo aqui é o mínimo que fecha a janela sem reescrever a autenticação:
//
//   - quem REMOVE o vínculo (toca-auth) grava `revoked:{tenant}:{account}` no
//     Redis com TTL igual à vida do access token (default 1h);
//   - quem AUTENTICA (qualquer serviço, via Middleware) consulta essa chave e
//     recusa a requisição quando ela existe.
//
// Um access token emitido ANTES da remoção continua válido por até o TTL de
// revogação — daí o TTL ser do tamanho do token, não maior. A alternativa
// completa (versão de vínculo no token, conferida contra o banco) é uma mudança
// de schema + claim + cache e está registrada como continuação.

const (
	// DefaultMembershipRevocationTTL cobre a vida típica de um access token do
	// Supabase (1h). Variável: MEMBERSHIP_REVOCATION_TTL.
	DefaultMembershipRevocationTTL = time.Hour

	revocationKeyPrefix = "revoked:"
	revocationTTLEnv    = "MEMBERSHIP_REVOCATION_TTL"

	// SuspendedMembershipValue é a marca DURÁVEL de vínculo suspenso: bloqueia
	// qualquer token enquanto existir (é limpa na reativação). Papel/perfil
	// trocado grava TIMESTAMP (ver RevokeMembership).
	SuspendedMembershipValue = "suspended"

	// minRevocationTimestamp separa um TIMESTAMP de uma marca LEGADA.
	//
	// As versões anteriores gravavam "1" (revogação) — um valor que `ParseInt`
	// aceita como número, mas que como timestamp significa 1970 e não cortaria
	// token nenhum. Qualquer valor abaixo de 2001-09-09 é, por construção, marca
	// antiga: bloqueia enquanto existir, como sempre fez.
	minRevocationTimestamp = 1_000_000_000
)

// RevocationChecker informa se o vínculo (tenant, account) foi revogado.
type RevocationChecker interface {
	IsRevoked(ctx context.Context, tenantID, accountID string) (bool, error)
}

// RevocationSinceChecker decide a revogação OLHANDO QUANDO O TOKEN FOI EMITIDO.
//
// Por que existe (incidente de 2026-09-21, "loop infinito no /login"): a marca
// do A15 dizia apenas "este vínculo mudou" e valia para QUALQUER token enquanto
// estivesse no Redis (TTL 1h). Consequência: depois de o admin trocar o perfil
// de acesso de alguém, o login NOVO daquela pessoa também era recusado por até
// uma hora — o front não conseguia restaurar a sessão, caía de volta no /login e
// repetia. A semântica correta do A15 é "credenciais emitidas ANTES da mudança
// não valem mais"; uma credencial emitida depois é, por definição, do contexto
// novo.
//
// Implementado como interface separada para não quebrar dublês existentes: quem
// não implementa cai no `IsRevoked` (bloqueia sempre), que é o comportamento
// conservador.
type RevocationSinceChecker interface {
	IsRevokedSince(ctx context.Context, tenantID, accountID string, issuedAt time.Time) (bool, error)
}

// MembershipRevocationKey é a chave usada pelo gravador e pelo verificador.
func MembershipRevocationKey(tenantID, accountID string) string {
	return revocationKeyPrefix + tenantID + ":" + accountID
}

// MembershipRevocationTTL lê o TTL configurado (default 1h).
func MembershipRevocationTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv(revocationTTLEnv))
	if raw == "" {
		return DefaultMembershipRevocationTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		logger.Warn("auth.revocation_invalid_ttl", "event", "auth.revocation_invalid_ttl", "value", raw)
		return DefaultMembershipRevocationTTL
	}
	return ttl
}

// MembershipRevoker grava a marca de revogação. Interface (e não *redis.Client)
// para que cada serviço use o seu cliente — o wrapper de Redis do toca-auth já
// satisfaz a assinatura — e para que o teste injete um dublê.
type MembershipRevoker interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

// MembershipRevocationDeleter limpa a marca (reativação de vínculo suspenso).
// O wrapper de Redis do toca-auth já satisfaz a assinatura.
type MembershipRevocationDeleter interface {
	Delete(ctx context.Context, keys ...string) error
}

// checkRevocationIdentity recusa identidade incompleta registrando o motivo.
//
// N9 (reauditoria de 2026-09-20): as três operações de marca (revogar, suspender,
// limpar) faziam `if revoker == nil || tenantID == "" || accountID == "" {
// return nil }`. O caso "sem revoker" é coberto por quem chama (que loga
// `auth.membership_revocation_unavailable`), mas o caso **identidade vazia**
// voltava `nil` sem log em lugar nenhum: a remoção acontecia, nenhuma marca era
// gravada, e o resultado era indistinguível de sucesso para o chamador.
func checkRevocationIdentity(tenantID, accountID, operation string) error {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		logger.Warn("auth.membership_revocation_skipped_empty_identity",
			"event", "auth.membership_revocation_skipped_empty_identity",
			"operation", operation,
			"tenant_id", tenantID,
			"account_id", accountID,
		)
	}
	return nil
}

// SuspendMembership marca o vínculo como suspenso SEM TTL (P2/M7).
//
// A suspensão é uma decisão administrativa durável: se a marca expirasse junto
// com o token, um refresh depois da janela devolveria acesso a um vínculo
// suspenso. A remoção/troca de papel continua com TTL (basta cobrir a vida do
// token vigente); a suspensão fica até alguém reativar.
func SuspendMembership(ctx context.Context, revoker MembershipRevoker, tenantID, accountID string) error {
	_ = checkRevocationIdentity(tenantID, accountID, "suspend")
	if revoker == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		return nil
	}
	// TTL 0 no Redis = chave persistente.
	return revoker.Set(ctx, MembershipRevocationKey(tenantID, accountID), "suspended", 0)
}

// ClearMembershipRevocation remove a marca (reativação). Sem a remoção, o vínculo
// reativado continuaria recusado até a chave expirar — ou para sempre, no caso da
// suspensão sem TTL.
func ClearMembershipRevocation(ctx context.Context, deleter MembershipRevocationDeleter, tenantID, accountID string) error {
	_ = checkRevocationIdentity(tenantID, accountID, "clear")
	if deleter == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		return nil
	}
	return deleter.Delete(ctx, MembershipRevocationKey(tenantID, accountID))
}

// RevokeMembership marca o vínculo como revogado pelo TTL configurado.
//
// Chamado quando o vínculo deixa de existir (membro removido da empresa) ou muda
// de papel de forma relevante. Não é uma operação de autorização: falhar aqui não
// deve derrubar a remoção em si — quem chama registra o erro e segue, porque o
// vínculo já foi removido na fonte de verdade.
func RevokeMembership(ctx context.Context, revoker MembershipRevoker, tenantID, accountID string) error {
	_ = checkRevocationIdentity(tenantID, accountID, "revoke")
	if revoker == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		return nil
	}
	// Timestamp do CORTE, arredondado para CIMA: um token emitido no mesmo
	// segundo da mudança fica com `iat` <= agora e, portanto, é revogado (sem o
	// +1 ele escaparia). Um token do segundo seguinte já é do contexto novo.
	return revoker.Set(ctx, MembershipRevocationKey(tenantID, accountID),
		strconv.FormatInt(time.Now().Unix()+1, 10), MembershipRevocationTTL())
}

// revocationMarkRevokesUntil lê a marca e responde se ela ainda corta um token
// emitido em `issuedAt`.
//
//   - valor ausente            -> não revoga;
//   - "suspended" ou não-numérico (marca legada "1") -> revoga SEMPRE;
//   - número (unix)            -> revoga tokens emitidos ANTES dele;
//   - `issuedAt` desconhecido  -> revoga (fail-closed: sem saber quando o token
//     foi emitido, vale o comportamento conservador).
func revocationMarkRevokesUntil(raw string, issuedAt time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if raw == SuspendedMembershipValue {
		return true
	}
	revokedAt, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revokedAt < minRevocationTimestamp {
		// Marca legada ("1"/"revoked"): sem timestamp, bloqueia enquanto existir.
		return true
	}
	if issuedAt.IsZero() {
		return true
	}
	return issuedAt.Unix() < revokedAt
}

// RedisRevocationChecker consulta o Redis.
type RedisRevocationChecker struct {
	client *redis.Client
}

// NewRedisRevocationChecker monta o verificador.
func NewRedisRevocationChecker(client *redis.Client) *RedisRevocationChecker {
	return &RedisRevocationChecker{client: client}
}

// IsRevoked devolve true quando a chave existe.
func (r *RedisRevocationChecker) IsRevoked(ctx context.Context, tenantID, accountID string) (bool, error) {
	if r == nil || r.client == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		return false, nil
	}
	n, err := r.client.Exists(ctx, MembershipRevocationKey(tenantID, accountID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// IsRevokedSince decide a revogação pelo momento de emissão do token.
func (r *RedisRevocationChecker) IsRevokedSince(ctx context.Context, tenantID, accountID string, issuedAt time.Time) (bool, error) {
	if r == nil || r.client == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" {
		return false, nil
	}
	raw, err := r.client.Get(ctx, MembershipRevocationKey(tenantID, accountID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return revocationMarkRevokesUntil(raw, issuedAt), nil
}

var (
	revocationOnce    sync.Once
	revocationChecker RevocationChecker
)

// RevocationCheckerFromEnv devolve o verificador configurado pelo ambiente
// (REDIS_URI), ou nil quando não há Redis — nesse caso a checagem de revogação
// fica desligada e o middleware segue como antes (registrado em log no boot).
func RevocationCheckerFromEnv() RevocationChecker {
	revocationOnce.Do(func() {
		uri := strings.TrimSpace(os.Getenv("REDIS_URI"))
		if uri == "" {
			return
		}
		opts, err := redis.ParseURL(uri)
		if err != nil {
			logger.Warn("auth.revocation_redis_invalid_uri", "event", "auth.revocation_redis_invalid_uri", "error", err.Error())
			return
		}
		revocationChecker = NewRedisRevocationChecker(redis.NewClient(opts))
	})
	return revocationChecker
}

// SetRevocationChecker injeta o verificador (usado pelos testes e por quem quiser
// montar o cliente fora do ambiente).
func SetRevocationChecker(checker RevocationChecker) {
	revocationChecker = checker
}

package service

import (
	"context"
	"os"
	"strings"
	"sync"

	redisclient "github.com/oginoino/toca-auth/internal/infrastructure/redis"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/logger"
)

// P1/A15 — cliente de Redis usado para gravar a marca de revogação de vínculo.
//
// Reusa o wrapper do próprio serviço (que já satisfaz `auth.MembershipRevoker`).
// Sem `REDIS_URI` (ou com URI inválida) devolve nil: quem chama registra o aviso e
// segue, porque a remoção do vínculo não pode depender da disponibilidade do
// cache.
var (
	revocationRedisOnce   sync.Once
	revocationRedisClient *redisclient.Client
)

func redisClientFromEnv() *redisclient.Client {
	revocationRedisOnce.Do(func() {
		uri := strings.TrimSpace(os.Getenv("REDIS_URI"))
		if uri == "" {
			return
		}
		client, err := redisclient.New(uri)
		if err != nil {
			logger.Warn("auth.revocation_redis_unavailable",
				"event", "auth.revocation_redis_unavailable", "error", err.Error())
			return
		}
		revocationRedisClient = client
	})
	return revocationRedisClient
}

// revokeMembershipMark grava a marca de revogação usando o gravador injetado ou,
// na ausência dele, o Redis do ambiente. Compartilhado entre os serviços que
// mudam vínculo (remoção de membro e troca de papel) para que a chave e o TTL
// tenham uma única definição.
func revokeMembershipMark(ctx context.Context, revoker pkgAuth.MembershipRevoker, tenantID, accountID string) {
	if revoker == nil {
		// Cuidado com o "typed nil": atribuir um *redisclient.Client nulo a uma
		// interface a torna NÃO nula, e a chamada seguinte derruba o processo ao
		// desreferenciar o receptor. Só atribui se o cliente existir de fato.
		if client := redisClientFromEnv(); client != nil {
			revoker = client
		}
	}
	if revoker == nil {
		logger.Warn("auth.membership_revocation_unavailable",
			"event", "auth.membership_revocation_unavailable",
			"tenant_id", tenantID, "account_id", accountID,
			"hint", "REDIS_URI não configurado: o token do membro vale até expirar")
		return
	}
	if err := pkgAuth.RevokeMembership(ctx, revoker, tenantID, accountID); err != nil {
		logger.Warn("auth.membership_revocation_failed",
			"event", "auth.membership_revocation_failed",
			"tenant_id", tenantID, "account_id", accountID, "error", err)
		return
	}
	logger.Info("auth.membership_revoked",
		"event", "auth.membership_revoked", "tenant_id", tenantID, "account_id", accountID)
}

// suspendMembershipMark grava a marca DURÁVEL de suspensão (P2/M7). Sem Redis o
// aviso é o mesmo da revogação: a suspensão segue válida na fonte de verdade
// (vínculo inativo) e o login/switch-context já recusam emitir token novo.
func suspendMembershipMark(ctx context.Context, revoker pkgAuth.MembershipRevoker, tenantID, accountID string) {
	if revoker == nil {
		if client := redisClientFromEnv(); client != nil {
			revoker = client
		}
	}
	if revoker == nil {
		logger.Warn("auth.membership_suspension_unavailable",
			"event", "auth.membership_suspension_unavailable",
			"tenant_id", tenantID, "account_id", accountID,
			"hint", "REDIS_URI não configurado: o token vigente vale até expirar; login/switch-context já recusam")
		return
	}
	if err := pkgAuth.SuspendMembership(ctx, revoker, tenantID, accountID); err != nil {
		logger.Warn("auth.membership_suspension_failed",
			"event", "auth.membership_suspension_failed",
			"tenant_id", tenantID, "account_id", accountID, "error", err)
		return
	}
	logger.Info("auth.membership_suspended",
		"event", "auth.membership_suspended", "tenant_id", tenantID, "account_id", accountID)
}

// clearMembershipMark remove a marca quando o vínculo é reativado (P2/M7).
func clearMembershipMark(ctx context.Context, deleter pkgAuth.MembershipRevocationDeleter, tenantID, accountID string) {
	if deleter == nil {
		if client := redisClientFromEnv(); client != nil {
			deleter = client
		}
	}
	if deleter == nil {
		return
	}
	if err := pkgAuth.ClearMembershipRevocation(ctx, deleter, tenantID, accountID); err != nil {
		logger.Warn("auth.membership_reactivation_failed",
			"event", "auth.membership_reactivation_failed",
			"tenant_id", tenantID, "account_id", accountID, "error", err)
		return
	}
	logger.Info("auth.membership_reactivated",
		"event", "auth.membership_reactivated", "tenant_id", tenantID, "account_id", accountID)
}

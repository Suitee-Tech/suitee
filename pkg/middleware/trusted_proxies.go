package middleware

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
)

// SECURITY (achado A16 da revisão de permissões de 2026-09-19): identidade de
// borda controlada pelo cliente.
//
// O gin nasce confiando em TODOS os proxies (`trustedProxies =
// ["0.0.0.0/0","::/0"]`), então `c.ClientIP()` devolve o valor que o cliente
// quiser colocar em `X-Forwarded-For`/`X-Real-IP`. Toda decisão derivada do IP
// — rate limit de `/auth/login`, limite do `RequireInternalKey`, allowlist de IP
// do webhook do Clicksign — vira rotação de bucket de graça.
//
// A correção é declarar explicitamente quem pode falar por outro (o proxy de
// entrada, normalmente o Traefik) e IGNORAR os cabeçalhos quando a requisição
// não vem de um deles:
//
//	TRUSTED_PROXIES=172.16.0.0/12     # rede interna do compose (Traefik)
//
// Sem a variável o comportamento é FALHA FECHADA: nenhum proxy é confiado,
// `c.ClientIP()` passa a ser o par TCP direto e os cabeçalhos de encaminhamento
// são descartados. Em release isso é logado como erro (a postura correta atrás
// de um proxy exige a variável); em desenvolvimento é um aviso.
//
// Entradas curinga (`0.0.0.0/0`, `::/0`) são RECUSADAS em release — aceitá-las
// reintroduziria exatamente o achado — e entradas inválidas nunca derrubam o
// processo: são descartadas com log, degradando para "não confia em ninguém".

// wildcardProxyEntries são as entradas que equivalem a "confie em qualquer um".
var wildcardProxyEntries = map[string]bool{
	"0.0.0.0/0": true,
	"::/0":      true,
	"0.0.0.0":   true,
	"::":        true,
}

// TrustedProxyPolicy é o retrato da resolução de TRUSTED_PROXIES.
type TrustedProxyPolicy struct {
	// Proxies é a lista efetivamente entregue ao gin (vazia = não confia em ninguém).
	Proxies []string
	// TrustNone indica que os cabeçalhos de encaminhamento serão ignorados.
	TrustNone bool
	// Release indica ambiente de release (APP_ENV/APP_MODE).
	Release bool
	// Configured indica que TRUSTED_PROXIES tinha conteúdo.
	Configured bool
	// WildcardsIgnored são entradas curinga recusadas (release).
	WildcardsIgnored []string
	// Invalid são entradas que não são IP/CIDR válidos.
	Invalid []string
}

// ResolveTrustedProxyPolicy interpreta TRUSTED_PROXIES. É pura (recebe a string
// crua e o flag de release) para poder ser testada sem ambiente.
func ResolveTrustedProxyPolicy(raw string, release bool) TrustedProxyPolicy {
	policy := TrustedProxyPolicy{Release: release, TrustNone: true}

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		policy.Configured = true

		if wildcardProxyEntries[entry] {
			if release {
				policy.WildcardsIgnored = append(policy.WildcardsIgnored, entry)
				continue
			}
			policy.Proxies = append(policy.Proxies, entry)
			continue
		}

		if !validProxyEntry(entry) {
			policy.Invalid = append(policy.Invalid, entry)
			continue
		}
		policy.Proxies = append(policy.Proxies, entry)
	}

	if len(policy.Proxies) > 0 {
		policy.TrustNone = false
	} else {
		policy.Proxies = nil
	}
	return policy
}

// validProxyEntry aceita CIDR (`10.0.0.0/8`) ou IP isolado (`10.0.0.1`), que o
// gin normaliza para /32 ou /128.
func validProxyEntry(entry string) bool {
	if strings.Contains(entry, "/") {
		_, _, err := net.ParseCIDR(entry)
		return err == nil
	}
	return net.ParseIP(entry) != nil
}

// ApplyTrustedProxies aplica a política na engine e registra o resultado. Deve
// ser chamada logo após a criação do router, antes de qualquer middleware que
// leia `c.ClientIP()`.
func ApplyTrustedProxies(r *gin.Engine) TrustedProxyPolicy {
	policy := ResolveTrustedProxyPolicy(
		os.Getenv("TRUSTED_PROXIES"),
		config.IsReleaseEnv(os.Getenv("APP_ENV"), os.Getenv("APP_MODE")),
	)

	for _, entry := range policy.WildcardsIgnored {
		logger.Error("edge.trusted_proxies_wildcard_ignored",
			"entry", entry,
			"reason", "curinga equivale a confiar em qualquer cliente; recusado em release",
			"fix", "declare os CIDRs do proxy de entrada em TRUSTED_PROXIES")
	}
	for _, entry := range policy.Invalid {
		logger.Error("edge.trusted_proxies_invalid",
			"entry", entry,
			"reason", "não é IP nem CIDR válido; entrada descartada")
	}

	if err := r.SetTrustedProxies(policy.Proxies); err != nil {
		// Fail-closed: se o gin recusou a lista, não confiamos em proxy nenhum.
		logger.Error("edge.trusted_proxies_rejected",
			"error", err.Error(),
			"fallback", "trust_none")
		policy.Proxies = nil
		policy.TrustNone = true
		_ = r.SetTrustedProxies(nil)
	}

	switch {
	case !policy.TrustNone:
		logger.Info("edge.trusted_proxies_applied",
			"proxies", strings.Join(policy.Proxies, ","),
			"count", len(policy.Proxies))
	case policy.Release:
		logger.Error("edge.trusted_proxies_unset",
			"effect", "X-Forwarded-For/X-Real-IP ignorados; c.ClientIP() é o par TCP direto",
			"fix", "declare os CIDRs do proxy de entrada em TRUSTED_PROXIES")
	default:
		logger.Warn("edge.trusted_proxies_unset",
			"effect", "X-Forwarded-For/X-Real-IP ignorados; c.ClientIP() é o par TCP direto",
			"fix", "declare os CIDRs do proxy de entrada em TRUSTED_PROXIES")
	}

	return policy
}

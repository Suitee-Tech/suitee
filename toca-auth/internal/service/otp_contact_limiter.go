package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/logger"
)

// M14 — enumeração de contas e OTP sem limite por contato.
//
// A revisão de permissões de 2026-09-19 registrou que `/auth/otp/request` podia
// ser martelado: o limite existente é por IP (borda), e o ataque que importa aqui
// rotaciona IP e mantém o ALVO (um telefone/e-mail): custo de envio, poluição da
// caixa do titular e — com `create_user=true` fixo no Supabase — criação de
// usuário para número arbitrário.
//
// Este limitador conta por CONTATO. A chave é o hash do contato (nunca o contato
// em claro: a chave do Redis não deve virar um diretório de PII), com contador no
// Redis quando disponível e memória do processo como fallback.
//
// Env:
//
//	OTP_RATE_LIMIT_PER_CONTACT (default 3)
//	OTP_RATE_LIMIT_WINDOW      (default 15m)
const (
	defaultOTPPerContact = 3
	defaultOTPWindow     = 15 * time.Minute
)

// ErrOTPRateLimited é devolvido quando o contato estourou a cota. O handler
// traduz para 429 com Retry-After.
var ErrOTPRateLimited = errOTPRateLimited{}

type errOTPRateLimited struct{}

func (errOTPRateLimited) Error() string { return "too many requests for this contact" }

// otpWindowEntry é a janela em memória (sem Redis).
type otpWindowEntry struct {
	expiresAt time.Time
	count     int
}

// OTPContactLimiter aplica a cota por contato.
type OTPContactLimiter struct {
	store  otpStore
	limit  int
	window time.Duration

	mu      sync.Mutex
	windows map[string]*otpWindowEntry
}

// NewOTPContactLimiter monta o limitador a partir do ambiente. `store` pode ser
// nil (sem Redis): a cota passa a valer por processo, o que já fecha o abuso
// trivial sem depender de infraestrutura.
func NewOTPContactLimiter(store otpStore) *OTPContactLimiter {
	// Interface com ponteiro TIPADO NULO passa por `store != nil` e só explode no
	// primeiro Get — em produção isso derrubou TODO pedido de OTP com panic de
	// ponteiro nulo quando o Redis não subiu antes do serviço (o cliente nasce
	// nulo e o processo segue assim até ser recriado). Aqui o valor nulo é
	// normalizado para nil de verdade e o limitador cai no fallback em memória,
	// que é a falha ABERTA prometida no Allow.
	if store != nil {
		if v := reflect.ValueOf(store); v.Kind() == reflect.Ptr && v.IsNil() {
			logger.Warn("auth.otp_limiter_store_unavailable",
				"event", "auth.otp_limiter_store_unavailable",
				"detail", "store sem conexão (cliente nulo): cota passa a valer por processo")
			store = nil
		}
	}

	limit := defaultOTPPerContact
	if raw := strings.TrimSpace(os.Getenv("OTP_RATE_LIMIT_PER_CONTACT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		} else {
			logger.Warn("auth.otp_rate_limit_invalid",
				"event", "auth.otp_rate_limit_invalid", "value", raw, "fallback", limit)
		}
	}
	window := defaultOTPWindow
	if raw := strings.TrimSpace(os.Getenv("OTP_RATE_LIMIT_WINDOW")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			window = parsed
		} else {
			logger.Warn("auth.otp_rate_limit_window_invalid",
				"event", "auth.otp_rate_limit_window_invalid", "value", raw, "fallback", window.String())
		}
	}
	return &OTPContactLimiter{store: store, limit: limit, window: window, windows: map[string]*otpWindowEntry{}}
}

// Limit devolve a cota e a janela (para mensagem/observabilidade).
func (l *OTPContactLimiter) Limit() (int, time.Duration) {
	if l == nil {
		return defaultOTPPerContact, defaultOTPWindow
	}
	return l.limit, l.window
}

// contactRateLimitKey devolve a chave do contador: prefixo de hash, sem o
// contato em claro.
func contactRateLimitKey(contact string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(contact))))
	return "otp:rl:" + hex.EncodeToString(sum[:8])
}

// Allow consome uma unidade da cota do contato. Devolve false quando a janela
// já teve `limit` requisições.
//
// Janela deslizante: o TTL reinicia a cada requisição aceita, então a cota é "N
// por janela", não um bloqueio permanente — o titular legítimo continua pedindo
// um novo código de tempo em tempo, e o ataque fica limitado a N por janela.
//
// Falha ABERTA em erro de armazenamento (mesma decisão da checagem de revogação
// do A15): indisponibilidade de cache não pode derrubar o login por OTP de todo
// mundo; o abuso continua limitado pelo fallback em memória do processo e pelo
// limite de borda por IP.
func (l *OTPContactLimiter) Allow(ctx context.Context, contact string) bool {
	if l == nil {
		return true
	}
	key := contactRateLimitKey(contact)

	if l.store != nil {
		if allowed, ok := l.allowWithStore(ctx, key); ok {
			return allowed
		}
	}
	return l.allowInMemory(key)
}

// allowWithStore usa o Redis como contador. ok=false quando o armazenamento não
// respondeu (o chamador cai para a memória).
func (l *OTPContactLimiter) allowWithStore(ctx context.Context, key string) (allowed bool, ok bool) {
	raw, err := l.store.Get(ctx, key)
	if err != nil {
		return true, false
	}
	count := 0
	if strings.TrimSpace(raw) != "" {
		parsed, perr := strconv.Atoi(strings.TrimSpace(raw))
		if perr != nil {
			return true, false
		}
		count = parsed
	}
	if count >= l.limit {
		return false, true
	}
	if err := l.store.Set(ctx, key, strconv.Itoa(count+1), l.window); err != nil {
		return true, false
	}
	return true, true
}

// allowInMemory é o fallback por processo (sem Redis).
func (l *OTPContactLimiter) allowInMemory(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	entry, exists := l.windows[key]
	if !exists || now.After(entry.expiresAt) {
		// Limpa janelas vencidas para o mapa não crescer sem limite.
		for k, e := range l.windows {
			if now.After(e.expiresAt) {
				delete(l.windows, k)
			}
		}
		l.windows[key] = &otpWindowEntry{expiresAt: now.Add(l.window), count: 1}
		return true
	}
	entry.count++
	return entry.count <= l.limit
}

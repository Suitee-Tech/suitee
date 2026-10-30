package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca/pkg/audit"
)

// M14 — cota POR CONTATO no /auth/otp/request.
//
// O limite de borda é por IP; o ataque do achado rotaciona IP e mantém o alvo.
// Estes testes fixam a cota, a janela deslizante, o isolamento entre contatos, o
// fallback sem Redis e o rastro de auditoria da recusa.

type fakeOTPStore struct {
	values map[string]string
	err    error
}

func newFakeOTPStore() *fakeOTPStore { return &fakeOTPStore{values: map[string]string{}} }

func (s *fakeOTPStore) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	if s.err != nil {
		return s.err
	}
	if str, ok := value.(string); ok {
		s.values[key] = str
		return nil
	}
	s.values[key] = "1"
	return nil
}

func (s *fakeOTPStore) Get(_ context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.values[key], nil
}

func (s *fakeOTPStore) Delete(_ context.Context, keys ...string) error {
	for _, k := range keys {
		delete(s.values, k)
	}
	return nil
}

func (s *fakeOTPStore) Exists(_ context.Context, key string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	_, ok := s.values[key]
	return ok, nil
}

func TestOTPContactLimiter_EnforcesQuotaPerWindow(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "3")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")
	limiter := NewOTPContactLimiter(newFakeOTPStore())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if !limiter.Allow(ctx, "cliente@example.com") {
			t.Fatalf("requisição %d deveria ser permitida (cota 3)", i+1)
		}
	}
	if limiter.Allow(ctx, "cliente@example.com") {
		t.Fatal("4ª requisição do mesmo contato deveria ser barrada")
	}
}

func TestOTPContactLimiter_IsolatesContacts(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "1")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")
	limiter := NewOTPContactLimiter(newFakeOTPStore())
	ctx := context.Background()

	if !limiter.Allow(ctx, "primeiro@example.com") {
		t.Fatal("primeira requisição deveria passar")
	}
	if limiter.Allow(ctx, "primeiro@example.com") {
		t.Fatal("segunda requisição do mesmo contato deveria ser barrada")
	}
	if !limiter.Allow(ctx, "segundo@example.com") {
		t.Fatal("outro contato não pode herdar a cota do primeiro (é o ponto do limite por contato)")
	}
}

// Mesmo contato com grafia diferente (caixa/espaços) é o MESMO alvo.
func TestOTPContactLimiter_NormalizesContactKey(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "1")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")
	limiter := NewOTPContactLimiter(newFakeOTPStore())
	ctx := context.Background()

	if !limiter.Allow(ctx, "Cliente@Example.com") {
		t.Fatal("primeira requisição deveria passar")
	}
	if limiter.Allow(ctx, "  cliente@example.com ") {
		t.Fatal("mesmo contato com outra grafia deveria compartilhar a cota")
	}
}

// Sem Redis a cota continua valendo (fallback em memória do processo).
func TestOTPContactLimiter_FallsBackToMemoryWithoutStore(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "2")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")
	limiter := NewOTPContactLimiter(nil)
	ctx := context.Background()

	if !limiter.Allow(ctx, "sem-redis@example.com") || !limiter.Allow(ctx, "sem-redis@example.com") {
		t.Fatal("as duas primeiras requisições deveriam passar")
	}
	if limiter.Allow(ctx, "sem-redis@example.com") {
		t.Fatal("terceira requisição deveria ser barrada sem Redis também")
	}
}

// Indisponibilidade do cache não pode derrubar o OTP de todo mundo (falha aberta
// documentada), mas o fallback em memória continua contando.
func TestOTPContactLimiter_FailsOpenOnStoreError(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "1")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")
	store := newFakeOTPStore()
	store.err = errors.New("redis indisponível")
	limiter := NewOTPContactLimiter(store)
	ctx := context.Background()

	if !limiter.Allow(ctx, "indisponivel@example.com") {
		t.Fatal("com o store fora do ar a primeira requisição deveria passar (falha aberta)")
	}
	if limiter.Allow(ctx, "indisponivel@example.com") {
		t.Fatal("o fallback em memória deveria continuar contando mesmo com o store fora")
	}
}

func TestOTPContactLimiter_InvalidEnvFallsBackToDefaults(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "zero")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "não-é-duração")
	limiter := NewOTPContactLimiter(nil)

	limit, window := limiter.Limit()
	if limit != defaultOTPPerContact {
		t.Fatalf("cota inválida deveria cair no default (%d), veio %d", defaultOTPPerContact, limit)
	}
	if window != defaultOTPWindow {
		t.Fatalf("janela inválida deveria cair no default (%s), veio %s", defaultOTPWindow, window)
	}
}

// A recusa precisa deixar rastro: quem foi limitado, com que cota e janela.
func TestRequestOTP_RateLimitedAuditsAndFails(t *testing.T) {
	t.Setenv("OTP_RATE_LIMIT_PER_CONTACT", "1")
	t.Setenv("OTP_RATE_LIMIT_WINDOW", "15m")

	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	contact := "cliente@example.com"
	sb := &fakeSupabase{}
	repo := &fakeAccountRepo{
		byID:    map[string]*account.Account{"acc-1": {ID: "acc-1", Type: account.AccountTypeIndividual}},
		byEmail: map[string]*account.Account{contact: {ID: "acc-1", Type: account.AccountTypeIndividual}},
	}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, sb)
	// Limiter próprio: o default do pacote é um singleton criado no primeiro uso
	// (a cota vem do ambiente do processo) e os outros testes deste pacote já
	// chamaram RequestOTP antes — sem isto o teste dependeria da ordem.
	svc.otpLimiter = NewOTPContactLimiter(nil)

	if err := svc.RequestOTP(context.Background(), contact, "email"); err != nil {
		// a primeira passa (o event bus pode não estar configurado; o que importa
		// é consumir a cota)
		t.Logf("primeira requisição: %v", err)
	}
	err := svc.RequestOTP(context.Background(), contact, "email")
	if !errors.Is(err, ErrOTPRateLimited) {
		t.Fatalf("segunda requisição devolveu %v, want ErrOTPRateLimited", err)
	}

	found := false
	for _, e := range entries {
		if e.Action == "auth.otp_rate_limited" {
			found = true
			if e.Metadata["limit"] != 1 {
				t.Fatalf("trilha sem a cota aplicada: %+v", e.Metadata)
			}
			if e.Metadata["contact_hash"] == "" || e.Metadata["contact_hash"] == contact {
				t.Fatalf("trilha deveria registrar o hash do contato, não o contato: %+v", e.Metadata)
			}
		}
	}
	if !found {
		t.Fatal("recusa por cota sem trilha de auditoria")
	}
}

// Regressão de produção: o serviço subiu antes do Redis existir no DNS, o
// cliente nasceu nulo e TODO POST /auth/otp/request passou a devolver 500 com
// panic de ponteiro nulo — porque um ponteiro tipado nulo dentro da interface
// `otpStore` passa por `store != nil`. O limitador precisa normalizar isso e
// seguir com o fallback em memória (falha ABERTA).
type storeNulo struct{}

func (s *storeNulo) Set(context.Context, string, interface{}, time.Duration) error {
	panic("método chamado em store nulo: o limitador não normalizou o ponteiro")
}

func (s *storeNulo) Get(context.Context, string) (string, error) {
	panic("método chamado em store nulo: o limitador não normalizou o ponteiro")
}

func (s *storeNulo) Delete(context.Context, ...string) error {
	panic("método chamado em store nulo: o limitador não normalizou o ponteiro")
}

func (s *storeNulo) Exists(context.Context, string) (bool, error) {
	panic("método chamado em store nulo: o limitador não normalizou o ponteiro")
}

func TestNewOTPContactLimiterComPonteiroTipadoNulo(t *testing.T) {
	var cliente *storeNulo
	limiter := NewOTPContactLimiter(cliente)

	if limiter.store != nil {
		t.Fatal("ponteiro tipado nulo deveria ser normalizado para nil")
	}
	for i := 0; i < 3; i++ {
		if !limiter.Allow(context.Background(), "cliente@exemplo.com") {
			t.Fatalf("cota em memória recusou a requisição %d antes do limite", i+1)
		}
	}
}

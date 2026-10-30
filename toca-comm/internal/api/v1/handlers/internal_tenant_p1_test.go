package handlers_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-comm/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-comm/internal/api/v1/routes"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
	"github.com/oginoino/toca/toca-comm/internal/repository"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// P1 / A3 — as rotas /api/v1/comm/internal/* não aceitam mais o tenant
// escolhido pelo chamador.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) mostrou que os handlers internos liam o tenant do corpo
// (`tenant_id` do canal / do `data` da notificação) com a chave interna
// compartilhada como única credencial: qualquer portador da chave criava canais
// e notificações em qualquer tenant.
//
// Agora o grupo usa `auth.InternalOrUserMiddleware` e os handlers resolvem o
// tenant com `auth.ResolveTenantForInternalRequest` (helper internalTenant):
// credencial de serviço com allowlist só atende os tenants dela; chamada com JWT
// resolve pelo token e pedir outro tenant é 403. O dublê de repositório registra
// o tenant com que foi consultado — a asserção não é só o status, é a prova de
// que o tenant estrangeiro nunca chegou ao repositório.

// recordingMessageRepo registra o tenant de toda leitura/escrita de canal
// (dublê por embedding: só os métodos observados são sobrescritos).
type recordingMessageRepo struct {
	repository.MessageRepository
	mu                    sync.Mutex
	listChannelTenants    []string
	createdChannelTenants []string
}

func (r *recordingMessageRepo) record(dst *[]string, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Dedupe consecutivo: um mesmo request pode consultar o repositório mais de
	// uma vez e as asserções continuam exatas.
	if n := len(*dst); n > 0 && (*dst)[n-1] == value {
		return
	}
	*dst = append(*dst, value)
}

func (r *recordingMessageRepo) snapshot(dst *[]string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), *dst...)
}

func (r *recordingMessageRepo) ListChannels(_ context.Context, tenantID, _, _ string) ([]*message.Channel, error) {
	r.record(&r.listChannelTenants, tenantID)
	return nil, nil
}

func (r *recordingMessageRepo) ClientAccountForLead(context.Context, string, string) (string, error) {
	return "", nil
}

func (r *recordingMessageRepo) GetChannelByReference(_ context.Context, tenantID, _, _ string) (*message.Channel, error) {
	r.record(&r.listChannelTenants, tenantID)
	return nil, message.ErrChannelNotFound
}

func (r *recordingMessageRepo) FindDirectChannel(_ context.Context, _ string, _ []string) (*message.Channel, error) {
	return nil, nil
}

func (r *recordingMessageRepo) CreateChannel(_ context.Context, ch *message.Channel) error {
	r.record(&r.createdChannelTenants, ch.TenantID)
	return nil
}

// recordingInAppRepo registra o tenant das notificações in-app gravadas.
type recordingInAppRepo struct {
	repository.InAppNotificationRepository
	mu             sync.Mutex
	createdTenants []string
}

func (r *recordingInAppRepo) Create(_ context.Context, n *notification.InAppNotification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.createdTenants) > 0 && r.createdTenants[len(r.createdTenants)-1] == n.TenantID {
		return nil
	}
	r.createdTenants = append(r.createdTenants, n.TenantID)
	return nil
}

func (r *recordingInAppRepo) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.createdTenants...)
}

// withNamedServiceKey publica uma credencial NOMEADA com allowlist de tenants no
// registro global lido pelo middleware (mesmo caminho de INTERNAL_SERVICE_KEYS em
// produção) e devolve a chave a apresentar no header X-Internal-Key.
func withNamedServiceKey(t *testing.T, service, key string, tenants ...string) string {
	t.Helper()
	entry := service + ":" + key
	if len(tenants) > 0 {
		entry += ":" + strings.Join(tenants, "|")
	}
	prev, had := os.LookupEnv("INTERNAL_SERVICE_KEYS")
	if err := os.Setenv("INTERNAL_SERVICE_KEYS", entry); err != nil {
		t.Fatalf("set INTERNAL_SERVICE_KEYS: %v", err)
	}
	auth.ReloadServiceKeyRegistry()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("INTERNAL_SERVICE_KEYS", prev)
		} else {
			_ = os.Unsetenv("INTERNAL_SERVICE_KEYS")
		}
		auth.ReloadServiceKeyRegistry()
	})
	return key
}

// commTestJWKS serve um JWKS local e assina tokens no formato que o
// auth.Middleware valida.
type commTestJWKS struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func newCommTestJWKS(t *testing.T) *commTestJWKS {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	j := &commTestJWKS{key: key, kid: "toca-comm-test-kid"}
	payload := map[string]any{"keys": []map[string]any{{
		"kty": "RSA",
		"kid": j.kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	j.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	return j
}

func (j *commTestJWKS) token(t *testing.T, tenantID string) string {
	t.Helper()
	return j.tokenWithRole(t, "acc-1", tenantID, "corretor")
}

// tokenWithRole assina um token com conta e papel escolhidos — necessário para
// representar o `cliente` do portal, cujo tenant no token é a própria conta.
func (j *commTestJWKS) tokenWithRole(t *testing.T, accountID, tenantID, functionalRole string) string {
	t.Helper()
	email := "corretor@example.com"
	name := "Corretor"
	if functionalRole == "cliente" {
		email = "cliente@example.com"
		name = "Cliente"
	}
	claims := auth.Claims{
		AccountID:      accountID,
		Email:          email,
		Name:           name,
		Role:           "member",
		Permission:     "member",
		FunctionalRole: functionalRole,
		Type:           "company",
		TenantID:       tenantID,
		TokenType:      auth.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = j.kid
	signed, err := tok.SignedString(j.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// commRouter sobe o router REAL do serviço (routes.SetupRouter): rotas,
// middlewares e handlers de produção, com repositórios dublês.
func commRouter(t *testing.T, jwksURL string, msgRepo *recordingMessageRepo, inAppRepo *recordingInAppRepo) *gin.Engine {
	t.Helper()
	return commRouterFull(t, jwksURL, msgRepo, inAppRepo)
}

// commRouterWithRepo é o mesmo router aceitando qualquer implementação de
// MessageRepository (os testes de chat usam um dublê em memória com canais).
func commRouterWithRepo(t *testing.T, jwksURL string, msgRepo repository.MessageRepository) *gin.Engine {
	t.Helper()
	return commRouterFull(t, jwksURL, msgRepo, &recordingInAppRepo{})
}

func commRouterFull(t *testing.T, jwksURL string, msgRepo repository.MessageRepository, inAppRepo repository.InAppNotificationRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL

	inAppSvc := service.NewInAppNotificationService(inAppRepo, nil)
	msgSvc := service.NewMessageService(msgRepo, nil, nil, inAppSvc)
	emailPrefsSvc := service.NewEmailPrefsService(nil)

	return routes.SetupRouter(
		cfg,
		handlers.NewMessageHandler(msgSvc),
		handlers.NewNotificationHandler(service.NewNotificationService(nil)),
		handlers.NewInAppNotificationHandler(inAppSvc),
		handlers.NewEmailPrefsHandler(emailPrefsSvc),
		handlers.NewWebSocketHandler(nil, nil),
	)
}

func doCommRequest(t *testing.T, r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const ensurePath = "/api/v1/comm/internal/channels/ensure"

// Sem chave interna e sem JWT a rota interna não responde nada de negócio:
// prova que o tenant não vem do corpo.
func TestInternalComm_RejectsRequestWithoutInternalIdentity(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)

	w := doCommRequest(t, r, http.MethodPost, ensurePath,
		`{"tenant_id":"tenant-vitima","deal_id":"deal-1","participants":["acc-1"]}`, nil)

	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("rota interna respondeu %d sem identidade de serviço/usuário: %s", w.Code, w.Body.String())
	}
	if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 0 {
		t.Fatalf("nenhum canal deveria ser criado sem identidade, viu %v", seen)
	}
}

// (a) Credencial de SERVIÇO restrita a tenant-a pedindo tenant-vitima: 403 e o
// repositório NUNCA vê o tenant estrangeiro — o cenário exato do A3.
func TestInternalChannelsEnsure_ServiceCredentialCannotPickForeignTenant(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

	w := doCommRequest(t, r, http.MethodPost, ensurePath,
		`{"tenant_id":"tenant-vitima","deal_id":"deal-1","participants":["acc-1"]}`,
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a criou canal em tenant-vitima (%d, tenants consultados: %v): %s",
			w.Code, msgRepo.snapshot(&msgRepo.listChannelTenants), w.Body.String())
	}
	if seen := msgRepo.snapshot(&msgRepo.listChannelTenants); len(seen) != 0 {
		t.Fatalf("o repositório foi consultado com %v — a requisição nem deveria chegar lá", seen)
	}
	if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 0 {
		t.Fatalf("nenhum canal deveria ser criado, viu %v", seen)
	}
}

// (a-legítimo) A mesma credencial, no tenant DA ALLOWLIST, continua funcionando e
// o canal é criado no tenant da credencial.
func TestInternalChannelsEnsure_ServiceCredentialOwnTenant_ReachesRepository(t *testing.T) {
	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, "", msgRepo, inAppRepo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

	w := doCommRequest(t, r, http.MethodPost, ensurePath,
		`{"tenant_id":"tenant-a","deal_id":"deal-1","participants":["acc-1"]}`,
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusOK {
		t.Fatalf("chamada legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
	}
	if seen := msgRepo.snapshot(&msgRepo.listChannelTenants); len(seen) != 1 || seen[0] != "tenant-a" {
		t.Fatalf("o repositório deveria ser consultado com tenant-a, viu %v", seen)
	}
	if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 1 || seen[0] != "tenant-a" {
		t.Fatalf("o canal deveria ser criado em tenant-a, viu %v", seen)
	}
}

// (b) JWT de usuário: o tenant do TOKEN manda. Pedir outro é 403; o mesmo tenant
// segue funcionando.
func TestInternalChannelsEnsure_UserTokenTenantWins(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
	r := commRouter(t, jwks.server.URL, msgRepo, inAppRepo)
	token := jwks.token(t, "tenant-do-token")

	w := doCommRequest(t, r, http.MethodPost, ensurePath,
		`{"tenant_id":"tenant-vitima","deal_id":"deal-1","participants":["acc-1"]}`,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-do-token criou canal em tenant-vitima (%d, tenants consultados: %v)",
			w.Code, msgRepo.snapshot(&msgRepo.listChannelTenants))
	}
	if seen := msgRepo.snapshot(&msgRepo.listChannelTenants); len(seen) != 0 {
		t.Fatalf("o repositório foi consultado com %v — a requisição nem deveria chegar lá", seen)
	}

	w = doCommRequest(t, r, http.MethodPost, ensurePath,
		`{"tenant_id":"tenant-do-token","deal_id":"deal-1","participants":["acc-1"]}`,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Fatalf("chamada de usuário no próprio tenant deveria responder 200, veio %d: %s", w.Code, w.Body.String())
	}
	if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 1 || seen[0] != "tenant-do-token" {
		t.Fatalf("o canal deveria ser criado com o tenant do TOKEN, viu %v", seen)
	}
}

// POST /comm/internal/channels/direct — tenant no corpo.
func TestInternalChannelsDirect_BodyTenantIsEnforced(t *testing.T) {
	const directPath = "/api/v1/comm/internal/channels/direct"

	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, "", msgRepo, inAppRepo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCommRequest(t, r, http.MethodPost, directPath,
			`{"tenant_id":"tenant-vitima","name":"Conversa","participants":["acc-1","acc-2"]}`,
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a criou canal direto em tenant-vitima (%d, canais criados: %v): %s",
				w.Code, msgRepo.snapshot(&msgRepo.createdChannelTenants), w.Body.String())
		}
		if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 0 {
			t.Fatalf("nenhum canal deveria ser criado, viu %v", seen)
		}
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, "", msgRepo, inAppRepo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCommRequest(t, r, http.MethodPost, directPath,
			`{"tenant_id":"tenant-a","name":"Conversa","participants":["acc-1","acc-2"]}`,
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusOK {
			t.Fatalf("chamada legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := msgRepo.snapshot(&msgRepo.createdChannelTenants); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("o canal direto deveria ser criado em tenant-a, viu %v", seen)
		}
	})
}

// POST /comm/internal/notifications/:accountId — o tenant vinha de
// `data.tenant_id` no corpo.
func TestInternalPushNotification_BodyTenantIsEnforced(t *testing.T) {
	const pushPath = "/api/v1/comm/internal/notifications/acc-1"

	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, "", msgRepo, inAppRepo)
		key := withNamedServiceKey(t, "toca-financial", "key-fin", "tenant-a")

		w := doCommRequest(t, r, http.MethodPost, pushPath,
			`{"title":"Cobrança","message":"vence hoje","data":{"tenant_id":"tenant-vitima","charge_id":"ch-1"}}`,
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a notificou em tenant-vitima (%d, notificações gravadas: %v): %s",
				w.Code, inAppRepo.snapshot(), w.Body.String())
		}
		if seen := inAppRepo.snapshot(); len(seen) != 0 {
			t.Fatalf("nenhuma notificação deveria ser gravada, viu %v", seen)
		}
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, "", msgRepo, inAppRepo)
		key := withNamedServiceKey(t, "toca-financial", "key-fin", "tenant-a")

		w := doCommRequest(t, r, http.MethodPost, pushPath,
			`{"title":"Cobrança","message":"vence hoje","data":{"tenant_id":"tenant-a","charge_id":"ch-1"}}`,
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusOK {
			t.Fatalf("chamada legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := inAppRepo.snapshot(); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("a notificação deveria ser gravada em tenant-a, viu %v", seen)
		}
	})

	t.Run("usuário pedindo outro tenant no corpo", func(t *testing.T) {
		jwks := newCommTestJWKS(t)
		defer jwks.server.Close()

		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, jwks.server.URL, msgRepo, inAppRepo)
		token := jwks.token(t, "tenant-a")

		w := doCommRequest(t, r, http.MethodPost, pushPath,
			`{"title":"Cobrança","message":"vence hoje","data":{"tenant_id":"tenant-vitima","charge_id":"ch-1"}}`,
			map[string]string{"Authorization": "Bearer " + token})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-a notificou em tenant-vitima (%d, notificações gravadas: %v)",
				w.Code, inAppRepo.snapshot())
		}
		if seen := inAppRepo.snapshot(); len(seen) != 0 {
			t.Fatalf("nenhuma notificação deveria ser gravada, viu %v", seen)
		}
	})

	t.Run("serviço sem tenant explícito", func(t *testing.T) {
		msgRepo, inAppRepo := &recordingMessageRepo{}, &recordingInAppRepo{}
		r := commRouter(t, "", msgRepo, inAppRepo)
		key := withNamedServiceKey(t, "toca-financial", "key-fin")

		w := doCommRequest(t, r, http.MethodPost, pushPath,
			`{"title":"Cobrança","message":"vence hoje","data":{"charge_id":"ch-1"}}`,
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("serviço sem tenant explícito deveria ser recusado como validação, veio %d", w.Code)
		}
		if seen := inAppRepo.snapshot(); len(seen) != 0 {
			t.Fatalf("nenhuma notificação deveria ser gravada, viu %v", seen)
		}
	})
}

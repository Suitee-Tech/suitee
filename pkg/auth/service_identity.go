package auth

import (
	"crypto/subtle"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// Context keys usadas pelas rotas internas.
const (
	CtxInternalAuthKind     = "internal_auth_kind" // "user" | "service"
	CtxInternalServiceName  = "internal_service"   // nome do serviço (chave nomeada)
	CtxInternalAllowedScope = "internal_allowed_tenants"
	CtxInternalLegacyKey    = "internal_legacy_key" // true quando autenticou pela chave compartilhada antiga
)

// Auth kinds.
const (
	InternalAuthUser    = "user"
	InternalAuthService = "service"
)

// Erros de escopo de tenant em rotas internas (achado A3 da revisão de
// permissões de 2026-09-19: as rotas internas aceitavam `?tenant_id=` de
// qualquer chamador).
var (
	// ErrInternalTenantRequired — chamador de serviço precisa dizer o tenant.
	ErrInternalTenantRequired = errors.New("tenant_id is required for service callers")
	// ErrInternalCrossTenant — o tenant pedido não é permitido para este chamador.
	ErrInternalCrossTenant = errors.New("tenant_id is not allowed for this caller")
	// ErrInternalUnauthenticated — a rota não passou por um middleware interno.
	ErrInternalUnauthenticated = errors.New("internal route requires service key or user token")
)

// ServiceIdentity descreve qual serviço apresentou a chave interna.
type ServiceIdentity struct {
	Name string
	// AllowedTenants restringe os tenants que este serviço pode atender.
	// Vazio = sem restrição (chave compartilhada legada ou serviço que atende
	// todos os tenants, como o próprio toca-deal).
	AllowedTenants []string
	// Legacy marca a chave compartilhada INTERNAL_API_KEY.
	Legacy bool
}

// AllowsTenant informa se a credencial pode atuar no tenant pedido.
func (s ServiceIdentity) AllowsTenant(tenantID string) bool {
	if len(s.AllowedTenants) == 0 {
		return true
	}
	for _, t := range s.AllowedTenants {
		if t == tenantID {
			return true
		}
	}
	return false
}

// ServiceKeyRegistry resolve chaves internas nomeadas.
//
// Formato de INTERNAL_SERVICE_KEYS (opcional):
//
//	nome:chave[:tenantA|tenantB],nome2:chave2
//
// Ex.: `toca-financial:9f2c...:t1|t2,toca-crm:7ab1...`
//
// Sem essa variável o sistema segue no modelo antigo (uma única
// INTERNAL_API_KEY compartilhada, sem restrição de tenant) — o middleware
// registra `auth.internal_legacy_key` para que a migração seja observável.
type ServiceKeyRegistry struct {
	byKey map[string]ServiceIdentity
}

var (
	serviceKeysOnce sync.Once
	serviceKeys     *ServiceKeyRegistry
)

// LoadServiceKeyRegistry lê INTERNAL_SERVICE_KEYS do ambiente (idempotente: a
// configuração é resolvida uma vez no boot).
func LoadServiceKeyRegistry() *ServiceKeyRegistry {
	serviceKeysOnce.Do(func() {
		serviceKeys = ParseServiceKeyRegistry(os.Getenv("INTERNAL_SERVICE_KEYS"))
	})
	return serviceKeys
}

// ReloadServiceKeyRegistry relê o ambiente. Existe para os testes não
// dependerem da ordem de execução (o Once é do processo) e para um operador que
// queira reavaliar a configuração sem reiniciar.
func ReloadServiceKeyRegistry() *ServiceKeyRegistry {
	serviceKeys = ParseServiceKeyRegistry(os.Getenv("INTERNAL_SERVICE_KEYS"))
	return serviceKeys
}

// ParseServiceKeyRegistry interpreta o formato acima. Entradas malformadas são
// descartadas com log (nunca derrubam o processo: uma env torta degrada para o
// modelo legado, não para "todo mundo é aceito").
func ParseServiceKeyRegistry(raw string) *ServiceKeyRegistry {
	reg := &ServiceKeyRegistry{byKey: map[string]ServiceIdentity{}}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			logger.Warn("INTERNAL_SERVICE_KEYS: entrada sem 'nome:chave' ignorada", "entry_masked", maskEntry(entry))
			continue
		}
		name := strings.TrimSpace(parts[0])
		key := strings.TrimSpace(parts[1])
		if name == "" || key == "" {
			logger.Warn("INTERNAL_SERVICE_KEYS: entrada com nome/chave vazio ignorada")
			continue
		}
		identity := ServiceIdentity{Name: name}
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			for _, t := range strings.Split(parts[2], "|") {
				if t = strings.TrimSpace(t); t != "" {
					identity.AllowedTenants = append(identity.AllowedTenants, t)
				}
			}
		}
		if _, dup := reg.byKey[key]; dup {
			logger.Warn("INTERNAL_SERVICE_KEYS: chave duplicada ignorada", "service", name)
			continue
		}
		reg.byKey[key] = identity
	}
	return reg
}

func (r *ServiceKeyRegistry) Resolve(key string) (ServiceIdentity, bool) {
	if r == nil || key == "" {
		return ServiceIdentity{}, false
	}
	// Comparação em tempo constante por entrada para não vazar prefixo de chave.
	for candidate, identity := range r.byKey {
		if subtleCompare(candidate, key) {
			return identity, true
		}
	}
	return ServiceIdentity{}, false
}

func (r *ServiceKeyRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byKey)
}

// ServiceKeyRollout descreve a configuração de credencial interna do processo.
//
// P1/A2 — a migração para chave por serviço é CONFIGURAÇÃO (cada chamador
// apresenta a própria chave; cada receptor lista `nome:chave[:tenants]` em
// INTERNAL_SERVICE_KEYS), mas sem um sinal no boot ninguém percebe que um serviço
// ficou no modelo antigo: a chave compartilhada continua funcionando como
// identidade `legacy`, sem vínculo de tenant.
type ServiceKeyRollout struct {
	// NamedKeys é a quantidade de credenciais nomeadas configuradas.
	NamedKeys int
	// LegacyKey indica que INTERNAL_API_KEY (chave compartilhada) está definida.
	LegacyKey bool
	// Release indica ambiente de produção (APP_MODE/APP_ENV).
	Release bool
}

// NeedsMigration informa que o processo está em produção usando apenas a chave
// compartilhada — o estado em que qualquer portador da chave atende qualquer
// tenant.
func (r ServiceKeyRollout) NeedsMigration() bool {
	return r.Release && r.NamedKeys == 0 && r.LegacyKey
}

// ServiceKeyRolloutFromEnv monta o retrato a partir do ambiente.
func ServiceKeyRolloutFromEnv() ServiceKeyRollout {
	named := LoadServiceKeyRegistry().Len()
	legacy := strings.TrimSpace(os.Getenv("INTERNAL_API_KEY")) != ""
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODE")))
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	release := mode == "release" || env == "production" || env == "prod"
	return ServiceKeyRollout{NamedKeys: named, LegacyKey: legacy, Release: release}
}

// CallerKeyRegistration descreve a chave que ESTE processo apresenta nas
// chamadas internas e se os receptores a reconhecem.
//
// Existe por causa de um defeito de configuração que o beta viveu em
// 2026-10-05: a linha de `INTERNAL_SERVICE_KEYS` no arquivo de ambiente ficou
// com o texto da linha seguinte colado no fim da última entrada
// (`...toca-signature:<chave>INTERNAL_CALLER_KEY_GATEWAY=<chave>`). O registro
// passou a ter 14 chaves "válidas" — nenhum alarme de boot disparou, porque a
// contagem continuava certa — mas a chave do toca-signature não batia com
// nenhuma entrada. Resultado: TODA chamada interna do toca-signature virou 401
// no receptor, o evento de envio para assinatura esgotou as retentativas e foi
// para a DLQ, e o contrato ficou "enviado" sem envelope e sem e-mail para
// ninguém. O sintoma (documento que nunca chega) apareceu longe da causa
// (uma linha torta no .env), e ninguém tinha como ver.
//
// Este retrato fecha essa distância: cada serviço compara a chave que ele
// apresenta com o registro que ele mesmo carrega e grita no boot quando ela
// não está lá.
type CallerKeyRegistration struct {
	// PresentedKey é a chave configurada para este processo apresentar
	// (INTERNAL_CALLER_KEY), vazia quando o serviço não tem a sua.
	PresentedKey string
	// Fingerprint identifica a chave apresentada sem expô-la (sha256 truncado).
	Fingerprint string
	// RegisteredKey indica que a chave apresentada está em
	// INTERNAL_SERVICE_KEYS — os receptores a reconhecem por nome.
	RegisteredKey bool
	// LegacySharedKey indica que a chave apresentada é a compartilhada
	// (INTERNAL_API_KEY), aceita como identidade "legacy".
	LegacySharedKey bool
	// NamedKeys é quantas chaves nomeadas este processo conhece.
	NamedKeys int
}

// Recognized informa que os receptores conseguem autenticar este chamador.
func (r CallerKeyRegistration) Recognized() bool {
	return r.RegisteredKey || r.LegacySharedKey
}

// CallerKeyRegistrationFromEnv monta o retrato do chamador a partir do
// ambiente do processo.
func CallerKeyRegistrationFromEnv() CallerKeyRegistration {
	presented := strings.TrimSpace(os.Getenv("INTERNAL_CALLER_KEY"))
	reg := CallerKeyRegistration{
		PresentedKey: presented,
		Fingerprint:  internalKeyFingerprint(presented),
		NamedKeys:    LoadServiceKeyRegistry().Len(),
	}
	if presented == "" {
		return reg
	}
	shared := strings.TrimSpace(os.Getenv("INTERNAL_API_KEY"))
	switch {
	case shared != "" && subtleCompare(shared, presented):
		reg.LegacySharedKey = true
	default:
		_, reg.RegisteredKey = LoadServiceKeyRegistry().Resolve(presented)
	}
	return reg
}

// LogServiceKeyRollout registra o estado da migração no boot. É chamado pelos
// serviços; em produção com apenas a chave compartilhada o log é ERROR (alarme)
// e traz o caminho da migração. Também verifica se a chave que este serviço
// apresenta é reconhecida pelo registro — ver CallerKeyRegistration.
func LogServiceKeyRollout() ServiceKeyRollout {
	status := ServiceKeyRolloutFromEnv()
	logCallerKeyRegistration(CallerKeyRegistrationFromEnv())
	switch {
	case status.NeedsMigration():
		logger.Error("auth.internal_service_keys_missing_in_release",
			"event", "auth.internal_service_keys_missing_in_release",
			"legacy_shared_key", true,
			"hint", "configure INTERNAL_SERVICE_KEYS (nome:chave[:tenants]) e dê a cada chamador a própria chave em INTERNAL_API_KEY",
		)
	case status.LegacyKey && status.NamedKeys == 0:
		logger.Warn("auth.internal_legacy_key_model",
			"event", "auth.internal_legacy_key_model",
			"hint", "migre para INTERNAL_SERVICE_KEYS para vincular tenant à credencial")
	default:
		logger.Info("auth.internal_service_keys",
			"event", "auth.internal_service_keys", "named_keys", status.NamedKeys)
	}
	return status
}

// logCallerKeyRegistration emite o alarme do chamador não reconhecido.
//
// ERROR (e não WARN) porque o efeito é silencioso e grave: as chamadas internas
// deste serviço passam a responder 401 no receptor, e quem depende delas
// (envelope de assinatura, PDF, financeiro) falha em fluxo assíncrono, longe do
// operador. Com INTERNAL_SERVICE_KEYS configurado e INTERNAL_CALLER_KEY
// preenchida, não existe caso legítimo de chave não reconhecida.
func logCallerKeyRegistration(reg CallerKeyRegistration) {
	if reg.PresentedKey == "" {
		return
	}
	if reg.Recognized() {
		if reg.LegacySharedKey && reg.NamedKeys > 0 {
			logger.Warn("auth.internal_legacy_key",
				"event", "auth.internal_legacy_key",
				"key_fingerprint", reg.Fingerprint,
				"named_keys", reg.NamedKeys,
				"hint", "este serviço apresenta a chave compartilhada; preencha INTERNAL_CALLER_KEY_<SERVICO> para identificá-lo",
			)
		}
		return
	}
	logger.Error("auth.internal_caller_key_unregistered",
		"event", "auth.internal_caller_key_unregistered",
		"key_fingerprint", reg.Fingerprint,
		"named_keys", reg.NamedKeys,
		"hint", "a chave em INTERNAL_CALLER_KEY não está em INTERNAL_SERVICE_KEYS: toda chamada interna deste serviço vai responder 401. Confira a linha INTERNAL_SERVICE_KEYS do arquivo de ambiente (entrada <serviço>:<mesma chave>) e rode scripts/check-internal-keys.sh",
	)
}

// ResolveServiceIdentity resolve a identidade do chamador interno a partir da
// chave apresentada, caindo para a chave compartilhada quando ela bate.
func ResolveServiceIdentity(providedKey string) (ServiceIdentity, bool) {
	if identity, ok := LoadServiceKeyRegistry().Resolve(providedKey); ok {
		return identity, true
	}
	if legacy := os.Getenv("INTERNAL_API_KEY"); legacy != "" && subtleCompare(legacy, providedKey) {
		return ServiceIdentity{Name: "legacy", Legacy: true}, true
	}
	return ServiceIdentity{}, false
}

// setServiceContext registra a identidade resolvida no gin context e no request
// context para que handlers e serviços possam decidir o escopo do tenant.
func setServiceContext(c *gin.Context, identity ServiceIdentity) {
	c.Set(CtxInternalAuthKind, InternalAuthService)
	c.Set(CtxInternalServiceName, identity.Name)
	c.Set(CtxInternalAllowedScope, identity.AllowedTenants)
	c.Set(CtxInternalLegacyKey, identity.Legacy)
}

// InternalOrUserMiddleware aceita **chave interna de serviço** OU **JWT de
// usuário verificado**, registrando qual dos dois autenticou.
//
// Substitui `RequireInternalKey()` nas rotas internas que também são chamadas em
// nome de um usuário (ex.: toca-deal → toca-signature durante a assinatura de um
// contrato). Com o JWT presente, o tenant passa a ser derivado do TOKEN
// (ResolveTenantForInternalRequest) em vez do `?tenant_id=` do chamador —
// fechando o achado A3 sem quebrar o caminho de worker/serviço.
func InternalOrUserMiddleware(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := c.GetHeader("X-Internal-Key")
		if provided == "" {
			// Sem chave interna: exige JWT de usuário.
			c.Set(CtxInternalAuthKind, InternalAuthUser)
			Middleware(cfg)(c)
			return
		}

		// Mesma proteção anti-brute-force do RequireInternalKey (60/min por IP
		// por padrão). Sem isto, trocar o middleware nas rotas internas tiraria
		// a única barreira de taxa da superfície service-to-service.
		if !getAdminLimiter().allow(c.ClientIP()) {
			logger.Warn("internal.access_rate_limited",
				"event", "internal.access_rate_limited", "ip", c.ClientIP(),
				"path", c.Request.URL.Path, "method", c.Request.Method)
			c.Header("Retry-After", "60")
			response.Forbidden(c, "rate limit exceeded for internal endpoint")
			c.Abort()
			return
		}

		identity, ok := ResolveServiceIdentity(provided)
		if !ok {
			logger.Warn("internal.access_denied", "event", "internal.access_denied", "ip", c.ClientIP(), "path", c.Request.URL.Path, "method", c.Request.Method)
			response.Unauthorized(c, "unauthorized internal access")
			c.Abort()
			return
		}

		setServiceContext(c, identity)
		if identity.Legacy {
			// Sinal para a migração: identidade compartilhada sem restrição.
			logger.Warn("auth.internal_legacy_key",
				"event", "auth.internal_legacy_key",
				"path", c.Request.URL.Path,
				"hint", "migre para INTERNAL_SERVICE_KEYS para vincular tenant à credencial")
		}
		c.Next()
	}
}

// ResolveTenantForInternalRequest decide o tenant EFETIVO de uma requisição em
// rota interna (achado A3).
//
//   - autenticado por JWT de usuário: vale o tenant do TOKEN. O valor pedido
//     (query/header) só é aceito se vazio ou idêntico — pedir outro é 403;
//   - autenticado pela chave interna: o valor pedido é obrigatório e, quando a
//     credencial declara uma allowlist de tenants, precisa estar nela.
//
// O handler passa o valor que recebeu (`c.Query("tenant_id")`) e usa o retorno.
func ResolveTenantForInternalRequest(c *gin.Context, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	switch c.GetString(CtxInternalAuthKind) {
	case InternalAuthUser:
		tokenTenant := strings.TrimSpace(c.GetString("tenant_id"))
		if tokenTenant == "" {
			return "", ErrInternalTenantRequired
		}
		if requested != "" && requested != tokenTenant {
			return "", ErrInternalCrossTenant
		}
		return tokenTenant, nil

	case InternalAuthService:
		if requested == "" {
			return "", ErrInternalTenantRequired
		}
		allowed := c.GetStringSlice(CtxInternalAllowedScope)
		if len(allowed) > 0 {
			for _, t := range allowed {
				if t == requested {
					return requested, nil
				}
			}
			return "", ErrInternalCrossTenant
		}
		return requested, nil

	default:
		return "", ErrInternalUnauthenticated
	}
}

func maskEntry(entry string) string {
	if len(entry) <= 4 {
		return "***"
	}
	return entry[:2] + "***"
}

// subtleCompare compara duas strings em tempo constante.
func subtleCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

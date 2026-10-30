// Package timeutil concentra a regra de FUSO HORÁRIO do produto: as datas dos
// fluxos (lançamentos, relatórios, agendamentos) são dias civis no fuso
// configurado, não em UTC.
//
// Por que existe: o banco e os contêineres rodam em UTC. Uma baixa feita às
// 22h em São Paulo é 01h do dia seguinte em UTC — o lançamento caía no dia
// errado e, na virada do mês, no MÊS errado (medido no beta em 01/10/2026
// 00:55 UTC: `date_trunc('day', now())` devolvia 01/10 enquanto o dia civil
// local ainda era 30/09). O mesmo vale para vencimento, competência de DIMOB e
// agrupamento de relatório.
//
// Configuração (a primeira que existir vence):
//
//	APP_TIMEZONE                 nome IANA, ex. "America/Sao_Paulo" (padrão)
//	FINANCIAL_TZ_OFFSET_MINUTES  deslocamento fixo em minutos, ex. "-180"
//
// O padrão é Brasília (UTC-3). A tzdata vai EMBARCADA no binário (`time/tzdata`)
// para que o nome IANA funcione em contêiner mínimo, sem depender de zoneinfo do
// sistema operacional.
package timeutil

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "time/tzdata"
)

// DefaultTimezone é o fuso padrão do produto (Brasília, UTC-3).
const DefaultTimezone = "America/Sao_Paulo"

// defaultOffsetMinutes acompanha DefaultTimezone e é o fallback quando nem o
// nome nem a tzdata estão disponíveis (evita cair em UTC e reintroduzir o bug).
const defaultOffsetMinutes = -3 * 60

var (
	locationOnce sync.Once
	locationVal  *time.Location
)

// Location devolve o fuso configurado para os dias civis do produto.
//
// A leitura é feita uma vez por processo (o valor vem do ambiente, resolvido no
// boot). Use ReloadLocation nos testes que trocam a configuração.
func Location() *time.Location {
	locationOnce.Do(func() { locationVal = resolveLocation() })
	return locationVal
}

// ReloadLocation relê o ambiente. Existe para os testes não dependerem da ordem
// de execução (o Once é do processo) e para um operador reavaliar a configuração
// sem reiniciar.
func ReloadLocation() *time.Location {
	locationVal = resolveLocation()
	return locationVal
}

// TimezoneName devolve o nome utilizado nas queries SQL (`AT TIME ZONE`).
//
// Prefere o nome IANA — o Postgres tem tzdata própria e resolve horário de verão
// corretamente. Quando a configuração é apenas um deslocamento, devolve o
// formato aceito pelo Postgres ("-03:00").
func TimezoneName() string {
	if name := strings.TrimSpace(os.Getenv("APP_TIMEZONE")); name != "" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
		// Nome inválido não pode virar UTC silenciosamente: usa o padrão.
		return DefaultTimezone
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCIAL_TZ_OFFSET_MINUTES")); raw != "" {
		if minutes, err := strconv.Atoi(raw); err == nil {
			return offsetZoneName(minutes)
		}
	}
	return DefaultTimezone
}

// Today devolve o dia civil de agora no fuso configurado (meia-noite local).
func Today() time.Time {
	return StartOfDay(time.Now())
}

// StartOfDay devolve a meia-noite do dia civil do instante, no fuso configurado.
func StartOfDay(t time.Time) time.Time {
	local := InLocation(t)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

// EndOfDay devolve o último instante do dia civil do instante.
func EndOfDay(t time.Time) time.Time {
	return StartOfDay(t).AddDate(0, 0, 1).Add(-time.Microsecond)
}

// DateOnly formata o dia civil no fuso configurado ("2006-01-02").
func DateOnly(t time.Time) string {
	return InLocation(t).Format("2006-01-02")
}

// ParseDateOnly interpreta "2006-01-02" como o início do dia civil no fuso
// configurado (e não à meia-noite UTC).
func ParseDateOnly(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(value), Location())
}

// InLocation move o instante para o fuso configurado (mesmo instante, outra
// parede de relógio).
func InLocation(t time.Time) time.Time {
	return t.In(Location())
}

// SameCivilDay informa se dois instantes pertencem ao mesmo dia civil local.
func SameCivilDay(a, b time.Time) bool {
	return DateOnly(a) == DateOnly(b)
}

func resolveLocation() *time.Location {
	if name := strings.TrimSpace(os.Getenv("APP_TIMEZONE")); name != "" {
		if loc, err := time.LoadLocation(name); err == nil && loc != nil {
			return loc
		}
		return time.FixedZone(DefaultTimezone, defaultOffsetMinutes*60)
	}
	if raw := strings.TrimSpace(os.Getenv("FINANCIAL_TZ_OFFSET_MINUTES")); raw != "" {
		if minutes, err := strconv.Atoi(raw); err == nil {
			return time.FixedZone("configured", minutes*60)
		}
	}
	if loc, err := time.LoadLocation(DefaultTimezone); err == nil && loc != nil {
		return loc
	}
	return time.FixedZone(DefaultTimezone, defaultOffsetMinutes*60)
}

// offsetZoneName converte minutos em um nome aceito pelo Postgres ("-03:00").
func offsetZoneName(minutes int) string {
	sign := "+"
	if minutes < 0 {
		sign = "-"
		minutes = -minutes
	}
	return sign + pad2(minutes/60) + ":" + pad2(minutes%60)
}

func pad2(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// ── Fuso por tenant ──────────────────────────────────────────────────────────
//
// O fuso configurado na imobiliária (Configurações → Geral → "Fuso horário")
// manda nos fluxos: cada serviço registra no boot um resolvedor que lê esse
// valor (tipicamente do toca-core) e o timeutil o aplica por requisição.
//
// Precedência, da mais forte para a mais fraca:
//
//  1. fuso já carregado no CONTEXTO (WithLocation) — quem resolveu antes não
//     paga outra consulta;
//  2. fuso do tenant, pelo resolvedor registrado (com cache);
//  3. fuso do ambiente (APP_TIMEZONE, padrão Brasília).
//
// Sem resolvedor registrado (testes, workers simples) cai no ambiente — nunca
// quebra, e o pior caso é o padrão Brasília, que já é o comportamento correto
// para a operação brasileira.

// TenantResolver resolve o fuso configurado de um tenant. O segundo retorno diz
// se o tenant tem configuração própria; `false` faz o timeutil usar o padrão do
// ambiente.
type TenantResolver func(ctx context.Context, tenantID string) (*time.Location, bool)

type ctxLocationKey struct{}

var (
	tenantResolverMu sync.RWMutex
	tenantResolver   TenantResolver

	tenantCacheMu sync.RWMutex
	tenantCache   = map[string]tenantCacheEntry{}
)

// tenantCacheTTL é a validade do fuso por tenant na memória do processo. O valor
// muda raramente (é configuração), então 5 minutos evitam uma consulta por query
// sem deixar a mudança do operador "presa" por muito tempo.
const tenantCacheTTL = 5 * time.Minute

type tenantCacheEntry struct {
	location *time.Location
	found    bool
	expires  time.Time
}

// SetTenantResolver registra o resolvedor por tenant (chamado no boot de cada
// serviço). Passar nil volta ao comportamento só-ambiente.
func SetTenantResolver(resolver TenantResolver) {
	tenantResolverMu.Lock()
	tenantResolver = resolver
	tenantResolverMu.Unlock()
	ResetTenantCache()
}

// ResetTenantCache limpa o cache de fusos por tenant (testes e troca de
// configuração).
func ResetTenantCache() {
	tenantCacheMu.Lock()
	tenantCache = map[string]tenantCacheEntry{}
	tenantCacheMu.Unlock()
}

// WithLocation carrega o fuso já resolvido no contexto.
func WithLocation(ctx context.Context, loc *time.Location) context.Context {
	if loc == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxLocationKey{}, loc)
}

// LocationFromCtx devolve o fuso carregado no contexto, se houver.
func LocationFromCtx(ctx context.Context) (*time.Location, bool) {
	if ctx == nil {
		return nil, false
	}
	loc, ok := ctx.Value(ctxLocationKey{}).(*time.Location)
	return loc, ok && loc != nil
}

// LocationForTenant resolve o fuso aplicável ao tenant (ver precedência no
// comentário do bloco acima).
func LocationForTenant(ctx context.Context, tenantID string) *time.Location {
	if loc, ok := LocationFromCtx(ctx); ok {
		return loc
	}
	if loc, ok := resolveTenantLocation(ctx, tenantID); ok {
		return loc
	}
	return Location()
}

// TimezoneNameForTenant devolve o nome IANA do fuso aplicável ao tenant, no
// formato aceito pelo `AT TIME ZONE` do Postgres.
func TimezoneNameForTenant(ctx context.Context, tenantID string) string {
	if loc, ok := LocationFromCtx(ctx); ok {
		return renderLocationName(loc)
	}
	if loc, ok := resolveTenantLocation(ctx, tenantID); ok {
		return renderLocationName(loc)
	}
	return TimezoneName()
}

func resolveTenantLocation(ctx context.Context, tenantID string) (*time.Location, bool) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, false
	}
	tenantResolverMu.RLock()
	resolver := tenantResolver
	tenantResolverMu.RUnlock()
	if resolver == nil {
		return nil, false
	}

	tenantCacheMu.RLock()
	entry, cached := tenantCache[tenantID]
	tenantCacheMu.RUnlock()
	if cached && time.Now().Before(entry.expires) {
		return entry.location, entry.found
	}

	loc, found := resolver(ctx, tenantID)
	if loc == nil {
		found = false
	}
	tenantCacheMu.Lock()
	tenantCache[tenantID] = tenantCacheEntry{location: loc, found: found, expires: time.Now().Add(tenantCacheTTL)}
	tenantCacheMu.Unlock()
	return loc, found
}

// renderLocationName devolve o nome a usar em SQL. Zonas fixas criadas por
// deslocamento (FixedZone) não têm nome IANA: nesse caso devolvemos o
// deslocamento no formato aceito pelo Postgres ("-03:00").
func renderLocationName(loc *time.Location) string {
	if loc == nil {
		return TimezoneName()
	}
	name := loc.String()
	if name == "UTC" || name == "Local" {
		return name
	}
	if strings.Contains(name, "/") || name == DefaultTimezone {
		return name
	}
	_, offset := time.Now().In(loc).Zone()
	return offsetZoneName(offset / 60)
}

// StartOfDayIn devolve a meia-noite do dia civil do instante NO fuso informado.
func StartOfDayIn(loc *time.Location, t time.Time) time.Time {
	if loc == nil {
		loc = Location()
	}
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// ParseDateOnlyIn interpreta "2006-01-02" no fuso informado.
func ParseDateOnlyIn(loc *time.Location, value string) (time.Time, error) {
	if loc == nil {
		loc = Location()
	}
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(value), loc)
}

package timeutil

import (
	"context"
	"testing"
	"time"
)

// O padrão do produto é Brasília (UTC-3): sem configuração alguma, um instante
// às 22h em São Paulo precisa continuar sendo o MESMO dia civil.
// InLocationForTest move o instante para o fuso informado (o pacote expõe
// InLocation, mas aqui o fuso vem de uma variável, não da configuração).
func InLocationForTest(t time.Time, loc *time.Location) time.Time { return t.In(loc) }

func TestDefaultLocationIsBrasilia(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	loc := ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	if loc == nil {
		t.Fatal("Location() devolveu nil")
	}
	// 2026-10-01T00:55Z é 2026-09-30T21:55 em Brasília.
	instant := time.Date(2026, 10, 1, 0, 55, 0, 0, time.UTC)
	if got := DateOnly(instant); got != "2026-09-30" {
		t.Errorf("DateOnly = %q, want 2026-09-30 (dia civil local, não UTC)", got)
	}
	if got := TimezoneName(); got != DefaultTimezone {
		t.Errorf("TimezoneName = %q, want %q", got, DefaultTimezone)
	}
}

// A virada do dia local é o que separa um lançamento do dia seguinte — e, na
// virada do mês, de outro mês.
func TestDayBoundariesUseConfiguredTimezone(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	lastMinute := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC) // 20:59 BRT
	firstMinute := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)   // 00:00 BRT

	if got := DateOnly(lastMinute); got != "2026-09-30" {
		t.Errorf("23:59 UTC = %q, want 2026-09-30 (ainda 30/09 em Brasília)", got)
	}
	if got := DateOnly(firstMinute); got != "2026-10-01" {
		t.Errorf("03:00 UTC = %q, want 2026-10-01 (00:00 em Brasília)", got)
	}
	if StartOfDay(firstMinute).UTC() != firstMinute {
		t.Errorf("StartOfDay(03:00Z) = %s, want o próprio instante (meia-noite local)", StartOfDay(firstMinute).UTC())
	}
	// Precisão de microssegundo: é a do timestamptz do Postgres, e é o último
	// instante que ainda pertence ao dia civil local.
	if end := EndOfDay(lastMinute); end.UTC() != firstMinute.Add(-time.Microsecond) {
		t.Errorf("EndOfDay = %s, want 1µs antes de %s", end.UTC(), firstMinute)
	}
	if !SameCivilDay(lastMinute, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)) {
		t.Error("20:59 BRT e 09:00 BRT do mesmo dia deveriam ser o mesmo dia civil")
	}
}

// APP_TIMEZONE tem precedência e respeita horário de verão (a tzdata vai
// embarcada no binário, então o nome IANA funciona em contêiner mínimo).
func TestAppTimezoneOverridesOffset(t *testing.T) {
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "0")
	t.Setenv("APP_TIMEZONE", "America/Manaus")
	ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	instant := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC) // 21:00 em Manaus (UTC-4)
	if got := DateOnly(instant); got != "2026-09-30" {
		t.Errorf("DateOnly = %q, want 2026-09-30 em America/Manaus", got)
	}
	if got := TimezoneName(); got != "America/Manaus" {
		t.Errorf("TimezoneName = %q, want America/Manaus", got)
	}
}

// O deslocamento fixo continua valendo para quem já usava a variável, e o nome
// devolvido para o SQL é aceito pelo Postgres.
func TestOffsetConfigurationStillWorks(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "-180")
	ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	if got := TimezoneName(); got != "-03:00" {
		t.Errorf("TimezoneName = %q, want -03:00", got)
	}
	instant := time.Date(2026, 10, 1, 0, 55, 0, 0, time.UTC)
	if got := DateOnly(instant); got != "2026-09-30" {
		t.Errorf("DateOnly = %q, want 2026-09-30 com offset -180", got)
	}
}

// Nome inválido não pode virar UTC em silêncio (seria o bug de volta).
func TestInvalidTimezoneFallsBackToDefault(t *testing.T) {
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	t.Setenv("APP_TIMEZONE", "Marte/Olympus")
	loc := ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	instant := time.Date(2026, 10, 1, 0, 55, 0, 0, time.UTC)
	if got := DateOnly(instant); got != "2026-09-30" {
		t.Errorf("DateOnly = %q, want 2026-09-30 (fallback Brasília, nunca UTC)", got)
	}
	if _, offset := instant.In(loc).Zone(); offset != defaultOffsetMinutes*60 {
		t.Errorf("offset = %d, want %d", offset, defaultOffsetMinutes*60)
	}
}

func TestParseDateOnlyUsesConfiguredTimezone(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	parsed, err := ParseDateOnly("2026-09-30")
	if err != nil {
		t.Fatalf("ParseDateOnly: %v", err)
	}
	want := time.Date(2026, 9, 30, 3, 0, 0, 0, time.UTC) // 00:00 BRT
	if parsed.UTC() != want {
		t.Errorf("ParseDateOnly = %s, want %s (meia-noite local, não UTC)", parsed.UTC(), want)
	}
	if _, err := ParseDateOnly("30/09/2026"); err == nil {
		t.Error("formato inválido deveria devolver erro")
	}
}

func TestTodayIsTheLocalCivilDay(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	t.Cleanup(func() { ReloadLocation() })

	today := Today()
	if today.Hour() != 0 || today.Minute() != 0 {
		t.Errorf("Today() = %s, want meia-noite local", today)
	}
	if today.Format("2006-01-02") != DateOnly(time.Now()) {
		t.Errorf("Today() = %s não é o dia civil de agora (%s)", DateOnly(today), DateOnly(time.Now()))
	}
}

// O fuso configurado na imobiliária (Configurações → "Fuso horário") manda nos
// fluxos; o do ambiente é o fallback.
func TestTenantTimezoneOverridesDefault(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	ResetTenantCache()
	t.Cleanup(func() {
		SetTenantResolver(nil)
		ReloadLocation()
	})

	manaus, err := time.LoadLocation("America/Manaus")
	if err != nil {
		t.Fatalf("carregar Manaus: %v", err)
	}
	calls := 0
	SetTenantResolver(func(_ context.Context, tenantID string) (*time.Location, bool) {
		calls++
		if tenantID == "tenant-manaus" {
			return manaus, true
		}
		return nil, false
	})

	ctx := context.Background()
	// 2026-10-01T01:00Z: 21:00 em Manaus (30/09) e 22:00 em Brasília (30/09).
	instant := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	if got := DateOnly(InLocationForTest(instant, LocationForTenant(ctx, "tenant-manaus"))); got != "2026-09-30" {
		t.Errorf("Manaus: %q, want 2026-09-30", got)
	}
	if got := TimezoneNameForTenant(ctx, "tenant-manaus"); got != "America/Manaus" {
		t.Errorf("TimezoneNameForTenant = %q, want America/Manaus", got)
	}

	// Tenant sem configuração usa o padrão (Brasília).
	if got := TimezoneNameForTenant(ctx, "tenant-sem-config"); got != DefaultTimezone {
		t.Errorf("fallback = %q, want %q", got, DefaultTimezone)
	}

	// Cache: a segunda consulta do mesmo tenant não chama o resolvedor de novo.
	before := calls
	_ = LocationForTenant(ctx, "tenant-manaus")
	if calls != before {
		t.Errorf("resolvedor chamado %d vezes; o cache do tenant não está valendo", calls-before)
	}
}

// Fuso já carregado no contexto tem precedência sobre o do tenant (evita
// resolver duas vezes na mesma requisição).
func TestContextLocationWinsOverTenant(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	ResetTenantCache()
	t.Cleanup(func() {
		SetTenantResolver(nil)
		ReloadLocation()
	})

	SetTenantResolver(func(context.Context, string) (*time.Location, bool) {
		return time.UTC, true
	})
	sp, err := time.LoadLocation(DefaultTimezone)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithLocation(context.Background(), sp)
	if got := TimezoneNameForTenant(ctx, "qualquer"); got != DefaultTimezone {
		t.Errorf("TimezoneNameForTenant = %q, want o fuso do contexto (%s)", got, DefaultTimezone)
	}
}

// Sem resolvedor (worker simples, testes) o comportamento volta ao ambiente.
func TestWithoutResolverFallsBackToEnvironment(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "")
	ReloadLocation()
	SetTenantResolver(nil)
	t.Cleanup(func() { ReloadLocation() })

	if got := TimezoneNameForTenant(context.Background(), "tenant-x"); got != DefaultTimezone {
		t.Errorf("TimezoneNameForTenant = %q, want %q", got, DefaultTimezone)
	}
}

// Zona fixa (offset) precisa virar um nome aceito pelo Postgres no SQL.
func TestOffsetLocationRendersPostgresName(t *testing.T) {
	fixed := time.FixedZone("configured", -4*60*60)
	if got := renderLocationName(fixed); got != "-04:00" {
		t.Errorf("renderLocationName = %q, want -04:00", got)
	}
}

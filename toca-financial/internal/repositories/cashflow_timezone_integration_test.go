package repositories

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/timeutil"
)

// Integração (roda com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE): o caixa diário
// precisa agrupar pelo DIA CIVIL no fuso configurado, não pelo dia UTC.
//
// Regressão medida no beta em 01/10/2026 00:55 UTC: com o banco em UTC e o
// operador em Brasília (UTC-3), `DATE_TRUNC('day', now())` devolvia 01/10
// enquanto o dia civil local ainda era 30/09. Um recebimento às 23h30 do dia
// 30/09 (02h30 UTC de 01/10) aparecia no dia — e no MÊS — seguinte nos
// relatórios.
func TestCashFlowByDayGroupsByConfiguredTimezone(t *testing.T) {
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	if appURI == "" || svcURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI, MaxPoolSize: 4, MinPoolSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID := uuid.NewString()
	txID := uuid.NewString()
	defer func() {
		_ = db.ExecService(ctx, `DELETE FROM financial.transactions WHERE tenant_id::text = $1::text`, tenantID)
	}()

	// 30/09/2026 23:30 em Brasília = 01/10/2026 02:30 UTC: o caso que separa os
	// dois agrupamentos.
	localEvening := time.Date(2026, 9, 30, 23, 30, 0, 0, timeutil.Location())
	if got := localEvening.UTC().Format(time.RFC3339); got != "2026-10-01T02:30:00Z" {
		t.Fatalf("premissa do teste quebrou: instante local = %s", got)
	}

	metadata := `{
		"id": "` + txID + `",
		"tenant_id": "` + tenantID + `",
		"type": "receita",
		"status": "PAID",
		"value": 100.00,
		"description": "Recebimento de teste (virada do dia)",
		"payment_date": "` + localEvening.Format(time.RFC3339) + `"
	}`
	if err := db.ExecService(ctx, `INSERT INTO financial.transactions (id, tenant_id, metadata) VALUES ($1, $2, $3)`,
		txID, tenantID, metadata); err != nil {
		t.Fatalf("inserir transação de teste: %v", err)
	}

	repo := NewFinancialReportPostgresRepository(db)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, timeutil.Location())
	to := time.Date(2026, 10, 31, 23, 59, 59, 0, timeutil.Location())
	// O repositório lê pelo pool de aplicação: sob RLS a consulta só enxerga o
	// tenant que está no contexto (as claims são injetadas no checkout).
	tenantCtx := pkgcontext.WithTenantID(ctx, tenantID)
	entries, err := repo.CashFlowByDay(tenantCtx, tenantID, from, to)
	if err != nil {
		t.Fatalf("CashFlowByDay: %v", err)
	}

	var day string
	for _, entry := range entries {
		if entry.InflowCents > 0 {
			day = entry.Period
		}
	}
	if day == "" {
		t.Fatalf("nenhum bucket com entrada para o tenant de teste: %+v", entries)
	}
	if day != "2026-09-30" {
		t.Errorf("bucket = %s, want 2026-09-30 (dia civil em %s; em UTC seria 2026-10-01)",
			day, timeutil.TimezoneName())
	}
}

// Prova ponta a ponta de que o fuso CONFIGURADO PELA IMOBILIÁRIA manda no fluxo:
// lê o valor de `core.company_settings` do tenant real (é o que o resolvedor do
// core devolve em produção) e verifica que o bucket do caixa muda com ele.
//
// Instante escolhido para discriminar: 01/10/2026 00:30 em Brasília é
// 30/09/2026 23:30 em Manaus (UTC-4) — o mesmo recebimento pertence a dias civis
// diferentes conforme o fuso da imobiliária.
func TestCashFlowUsesTenantConfiguredTimezone(t *testing.T) {
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	tenantID := os.Getenv("TENANT_TIMEZONE_PROBE_TENANT_ID")
	if appURI == "" || svcURI == "" || tenantID == "" {
		t.Skip("defina RLS_PROBE_URI_APP, RLS_PROBE_URI_SERVICE e TENANT_TIMEZONE_PROBE_TENANT_ID")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI, MaxPoolSize: 4, MinPoolSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 1) O que está configurado na imobiliária hoje.
	var timezone string
	if err := db.QueryServiceRow(ctx,
		`SELECT COALESCE(metadata->>'timezone', '') FROM core.company_settings WHERE tenant_id = $1`, tenantID).
		Scan(&timezone); err != nil {
		t.Fatalf("ler fuso do tenant: %v", err)
	}
	if timezone == "" {
		t.Fatal("tenant sem timezone configurado: o teste precisa do valor gravado em core.company_settings")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("carregar fuso %q: %v", timezone, err)
	}

	// 2) Registra o mesmo caminho do resolvedor de produção.
	timeutil.ResetTenantCache()
	timeutil.SetTenantResolver(func(context.Context, string) (*time.Location, bool) { return loc, true })
	t.Cleanup(func() {
		timeutil.SetTenantResolver(nil)
		timeutil.ResetTenantCache()
	})

	txID := uuid.NewString()
	defer func() {
		_ = db.ExecService(ctx, `DELETE FROM financial.transactions WHERE tenant_id::text = $1::text AND id = $2`, tenantID, txID)
	}()

	// 00:30 de 01/10/2026 em Brasília.
	instant := time.Date(2026, 10, 1, 0, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	metadata := `{
		"id": "` + txID + `",
		"tenant_id": "` + tenantID + `",
		"type": "receita",
		"status": "PAID",
		"value": 100.00,
		"description": "Recebimento de teste (fuso do tenant)",
		"payment_date": "` + instant.Format(time.RFC3339) + `"
	}`
	if err := db.ExecService(ctx, `INSERT INTO financial.transactions (id, tenant_id, metadata) VALUES ($1, $2, $3)`,
		txID, tenantID, metadata); err != nil {
		t.Fatalf("inserir transação de teste: %v", err)
	}

	repo := NewFinancialReportPostgresRepository(db)
	tenantCtx := pkgcontext.WithTenantID(ctx, tenantID)
	entries, err := repo.CashFlowByDay(tenantCtx, tenantID,
		time.Date(2026, 9, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 31, 23, 59, 59, 0, loc))
	if err != nil {
		t.Fatalf("CashFlowByDay: %v", err)
	}

	want := instant.In(loc).Format("2006-01-02")
	var got string
	for _, entry := range entries {
		if entry.InflowCents > 0 {
			got = entry.Period
		}
	}
	if got != want {
		t.Errorf("bucket = %q, want %q (fuso configurado do tenant: %s)", got, want, timezone)
	}
	t.Logf("tenant %s com fuso %s: recebimento em %s caiu no dia %s", tenantID, timezone, instant.Format(time.RFC3339), got)
}

package database

import (
	"context"
	"os"
	"testing"

	"github.com/oginoino/toca/pkg/config"
)

// Integração (roda só com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE definidos):
// o pool de serviço PRECISA bypassar a RLS por inteiro — inclusive dentro de
// `RunInServiceContext`. Medido no beta em 2026-09-30: `QueryServiceRow(s)`
// devolviam a linha e `RunInServiceContext` + `QueryRowContext` devolviam ZERO
// sem erro, escondendo o negócio do cliente no portal (a lista e as seções do
// negócio caíam como "vazio"/"não encontrado").
func TestServicePoolBypassesRLSInEveryPath(t *testing.T) {
	appURI, svcURI := os.Getenv("RLS_PROBE_URI_APP"), os.Getenv("RLS_PROBE_URI_SERVICE")
	if appURI == "" || svcURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	pg, err := ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI})
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	var svcDBUser string
	_ = pg.serviceDB.QueryRowContext(ctx, "SELECT current_user").Scan(&svcDBUser)
	t.Logf("serviceDB: current_user=%q serviceDB==DB:%v ServicePool==pool:%v serviceRoleBypassesRLS:%v",
		svcDBUser, pg.serviceDB == pg.DB, pg.ServicePool == pg.pool, pg.serviceRoleBypassesRLS)

	var txUser string
	if tx, txErr := pg.serviceDB.BeginTxx(ctx, nil); txErr != nil {
		t.Logf("BeginTxx(serviceDB) erro: %v", txErr)
	} else {
		_ = tx.QueryRowContext(ctx, "SELECT current_user").Scan(&txUser)
		_ = tx.Rollback()
	}
	t.Logf("tx aberta pelo serviceDB: current_user=%q", txUser)

	// Existe pelo menos uma linha visível apenas para quem bypassa a RLS?
	var total int
	if err := pg.QueryServiceRow(ctx, "SELECT count(*) FROM deal.deals").Scan(&total); err != nil {
		t.Fatal(err)
	}
	t.Logf("deal.deals via QueryServiceRow: %d", total)
	if total == 0 {
		t.Skip("base sem linhas para medir")
	}

	whoami := func(label string, scan func(any, any) error) {
		var user, rowSecurity string
		if err := scan(&user, &rowSecurity); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		var bypass bool
		if err := pg.QueryServiceRow(ctx, "SELECT rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&bypass); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		t.Logf("%s: current_user=%s row_security=%s bypassrls=%v", label, user, rowSecurity, bypass)
	}

	whoami("pool de servico", func(a, b any) error {
		return pg.QueryServiceRow(ctx, "SELECT current_user, current_setting('row_security')").Scan(a, b)
	})

	var inTx int
	errTx := pg.RunInServiceContext(ctx, func(txCtx context.Context) error {
		whoami("dentro de RunInServiceContext", func(a, b any) error {
			return pg.QueryRowContext(txCtx, "SELECT current_user, current_setting('row_security')").Scan(a, b)
		})
		return pg.QueryRowContext(txCtx, "SELECT count(*) FROM deal.deals").Scan(&inTx)
	})
	if errTx != nil {
		t.Fatalf("RunInServiceContext: %v", errTx)
	}
	t.Logf("deal.deals dentro de RunInServiceContext: %d", inTx)

	if inTx != total {
		t.Fatalf("transação de serviço NÃO bypassa a RLS: %d linhas dentro da transação contra %d no pool", inTx, total)
	}
}

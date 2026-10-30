// Comando de diagnóstico: reproduz a LEITURA do motor de inadimplência fora do
// serviço — descoberta de tenants (pool de serviço) e listagem por tenant (pool
// de aplicação com as claims do tenant, sob RLS).
//
// Por que existe: em produção o tick automático logava "0 overdue" enquanto o
// disparo manual encontrava a cobrança. Este comando separa as duas etapas e diz
// QUAL delas volta vazia, com o fuso resolvido por tenant, em vez de obrigar a
// instrumentar o serviço em produção.
//
// Uso (somente leitura):
//
//	DATABASE_URI=... DATABASE_URI_SERVICE=... \
//	  go run ./cmd/diag-delinquency-read
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/oginoino/toca/pkg/config"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cfg := config.DatabaseConfig{
		URI:        os.Getenv("DATABASE_URI"),
		ServiceURI: os.Getenv("DATABASE_URI_SERVICE"),
	}
	pgDB, err := database.ConnectPostgres(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "banco:", err)
		os.Exit(2)
	}
	defer pgDB.Close()

	fmt.Printf("pool de serviço presente: %v\n", pgDB.ServicePool != nil)
	fmt.Printf("fuso padrão do processo:  %s\n\n", timeutil.TimezoneName())

	repo := repositories.NewChargePostgresRepository(pgDB)

	// Etapa 1 — descoberta de tenants (é o que o motor faz primeiro).
	tenants, err := repo.(interface {
		ListDistinctTenantIDs(context.Context) ([]string, error)
	}).ListDistinctTenantIDs(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListDistinctTenantIDs: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("etapa 1 — tenants com cobrança: %d\n", len(tenants))
	for _, t := range tenants {
		fmt.Printf("           %s\n", t)
	}

	// Etapa 2 — listagem por tenant, com claims (igual ao motor).
	for _, tid := range tenants {
		tenantCtx := pkgctx.WithTenantID(ctx, tid)
		tz := timeutil.TimezoneNameForTenant(tenantCtx, tid)
		lista, err := repo.(interface {
			ListOverdue(context.Context, string) ([]*charge.Charge, error)
		}).ListOverdue(tenantCtx, tid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  tenant %s: ListOverdue erro: %v\n", tid, err)
			continue
		}
		fmt.Printf("\netapa 2 — tenant %s (fuso %s): %d cobrança(s) vencida(s)\n", tid, tz, len(lista))
		for _, c := range lista {
			fmt.Printf("           %s %s %s venc=%s atraso=%d dias total=%.2f\n",
				c.ID[:8], c.Type, c.Status, c.DueDate.Format("2006-01-02"),
				c.OverdueDays(time.Now(), timeutil.LocationForTenant(tenantCtx, tid)), c.TotalAmount.Float())
		}

		// Diagnóstico fino: quem é a conexão, quais claims ela vê e quantas linhas
		// cada recorte devolve — separa RLS de predicado.
		var usuario, claims string
		var total, soStatus, soPredicado int
		_ = pgDB.QueryRow(tenantCtx, "select current_user, coalesce(current_setting('request.jwt.claims', true), '')").Scan(&usuario, &claims)
		_ = pgDB.QueryRow(tenantCtx, "select count(*) from financial.charges where tenant_id = $1 and deleted_at is null", tid).Scan(&total)
		_ = pgDB.QueryRow(tenantCtx, "select count(*) from financial.charges where tenant_id = $1 and deleted_at is null and (metadata->>'status') = 'OVERDUE'", tid).Scan(&soStatus)
		_ = pgDB.QueryRow(tenantCtx, `select count(*) from financial.charges where tenant_id = $1 and deleted_at is null
			and ((metadata->>'status') = 'OVERDUE'
			     or ((metadata->>'status') in ('PENDING','PARTIALLY_PAID')
			         and ((metadata->>'due_date')::timestamptz at time zone 'UTC')::date < (now() at time zone $2)::date))`, tid, tz).Scan(&soPredicado)
		fmt.Printf("           conexao=%s claims=%s\n", usuario, claims)
		fmt.Printf("           linhas: total=%d | status=OVERDUE=%d | predicado completo=%d\n", total, soStatus, soPredicado)

		// Comparação: a mesma leitura SEM as claims (o que o pool de aplicação
		// enxerga sem contexto de tenant) — é o sintoma do "0 overdue".
		semClaims, err := repo.(interface {
			ListOverdue(context.Context, string) ([]*charge.Charge, error)
		}).ListOverdue(ctx, tid)
		if err == nil {
			fmt.Printf("           (mesma consulta sem claims de tenant: %d)\n", len(semClaims))
		}
	}
}

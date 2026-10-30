// Comando de diagnóstico: confere se a dívida declarada em cada caso de
// inadimplência bate com a soma das cobranças que ele agrupa.
//
// É o painel do item F0.5 do plano de inadimplência: quando o caso e as
// cobranças divergem, o número que a tela mostra não é o número que o financeiro
// cobra — e não havia como perceber isso sem abrir caso por caso.
//
// Uso (somente leitura):
//
//	DATABASE_URI=... DATABASE_URI_SERVICE=... \
//	  go run ./cmd/diag-collection-divergence [--tenant <uuid>] [--limite 200]
//
// Sai com código 1 quando encontra divergência acima da tolerância (1 centavo),
// para poder ser usado em verificação automatizada.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

const tolerancia = 0.01

func main() {
	tenantFlag := flag.String("tenant", "", "limita a conferência a um tenant (uuid)")
	limite := flag.Int("limite", 200, "quantos casos conferir por tenant")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pgDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:        os.Getenv("DATABASE_URI"),
		ServiceURI: os.Getenv("DATABASE_URI_SERVICE"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "banco:", err)
		os.Exit(2)
	}
	defer pgDB.Close()

	// A leitura é deliberadamente cross-tenant pelo pool de serviço: é um
	// comando de diagnóstico operado por quem já tem acesso ao banco, e a
	// alternativa (um tenant por vez) exigiria rodar N vezes.
	query := `
		SELECT c.tenant_id,
		       c.id,
		       COALESCE((c.metadata->>'total_debt')::numeric, 0) AS declarado,
		       COALESCE((
		           SELECT SUM(GREATEST(0,
		                      COALESCE((ch.metadata->>'total_amount')::numeric, 0)
		                      - COALESCE((ch.metadata->>'paid_amount')::numeric, 0)))
		           FROM financial.charges ch
		           WHERE ch.tenant_id = c.tenant_id
		             AND ch.deleted_at IS NULL
		             AND (ch.metadata->>'status') IN ('OVERDUE','PENDING','PARTIALLY_PAID')
		             AND ch.id::text IN (SELECT jsonb_array_elements_text(COALESCE(c.metadata->'charge_ids', '[]'::jsonb)))
		       ), 0) AS aberto,
		       jsonb_array_length(COALESCE(c.metadata->'charge_ids', '[]'::jsonb)) AS cobrancas,
		       c.metadata->>'status' AS status
		FROM financial.collections c
		WHERE c.deleted_at IS NULL
		  AND c.metadata->>'status' <> 'RESOLVED'
		  AND ($1 = '' OR c.tenant_id = $1::uuid)
		ORDER BY c.updated_at DESC
		LIMIT $2
	`

	rows, err := pgDB.QueryServiceRows(ctx, query, *tenantFlag, *limite)
	if err != nil {
		fmt.Fprintln(os.Stderr, "consulta:", err)
		os.Exit(2)
	}
	defer rows.Close()

	var conferidos, divergentes int
	for rows.Next() {
		var tenantID, casoID, status string
		var declarado, aberto float64
		var cobrancas int
		if err := rows.Scan(&tenantID, &casoID, &declarado, &aberto, &cobrancas, &status); err != nil {
			fmt.Fprintln(os.Stderr, "leitura:", err)
			os.Exit(2)
		}
		conferidos++
		diferenca := declarado - aberto
		if diferenca > tolerancia || diferenca < -tolerancia {
			divergentes++
			fmt.Printf("DIVERGE tenant=%s caso=%s status=%s declarado=%.2f aberto=%.2f dif=%.2f cobrancas=%d\n",
				tenantID, casoID, status, declarado, aberto, diferenca, cobrancas)
		}
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fmt.Fprintln(os.Stderr, "iteração:", err)
		os.Exit(2)
	}

	fmt.Printf("\ncasos conferidos: %d | divergentes: %d\n", conferidos, divergentes)
	if divergentes > 0 {
		fmt.Println("Ação: cancelar/estornar cobrança e reabrir o caso pelo motor (o próximo tick recalcula),")
		fmt.Println("ou corrigir a dívida declarada — o campo passou a ser derivado das cobranças.")
		os.Exit(1)
	}
	fmt.Println("Nenhuma divergência acima da tolerância de 1 centavo.")
}

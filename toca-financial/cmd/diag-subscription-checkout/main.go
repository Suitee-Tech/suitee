// Diagnóstico: executa o MESMO `Select` do checkout de assinatura (código real,
// mesmo provedor e mesmas credenciais) para um tenant SINTÉTICO — nenhum cliente
// é afetado — e imprime o erro exato que a tela devolve como 409/503.
//
// Uso:
//
//	DATABASE_URI_SERVICE=... PAYMENT_GATEWAY_API_KEY=... ABACATEPAY_DEV_MODE=false \
//	PUBLIC_FRONTEND_URL=https://app.example.com \
//	  go run ./cmd/diag-subscription-checkout <tier> <ciclo> [tenant-id]
//
// Sem `tenant-id` usa um tenant sintético (e apaga a linha ao final). Com um
// tenant real NADA é apagado: o objetivo é ver o erro exato que aquela conta
// recebe — e, se der certo, a linha fica com o checkout pendente que a tela
// reusa.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

func main() {
	tier := "pro"
	ciclo := "MONTHLY"
	tenantArg := ""
	if len(os.Args) > 1 {
		tier = os.Args[1]
	}
	if len(os.Args) > 2 {
		ciclo = os.Args[2]
	}
	if len(os.Args) > 3 {
		tenantArg = os.Args[3]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// O pool de aplicação aplica request.jwt.claims por conexão: sem o tenant no
	// contexto a RLS esconde a linha e o diagnóstico veria um estado vazio (foi
	// o que escondeu, por um tempo, o caminho real do serviço).
	if tenantArg != "" {
		ctx = appctx.WithTenantID(ctx, tenantArg)
	}

	// Espelha o app: pool de aplicação (RLS, DATABASE_URI) + pool de serviço.
	pgDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:        os.Getenv("DATABASE_URI"),
		ServiceURI: os.Getenv("DATABASE_URI_SERVICE"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "banco:", err)
		os.Exit(2)
	}
	defer pgDB.Close()

	dev := os.Getenv("ABACATEPAY_DEV_MODE") == "true"
	svc := &services.PlatformSubscriptionService{
		Repo:      repositories.NewPlatformSubscriptionRepository(pgDB),
		Provider:  providers.NewAbacateSubscriptionProvider(os.Getenv("PAYMENT_GATEWAY_API_KEY"), dev),
		ReturnURL: os.Getenv("PUBLIC_FRONTEND_URL") + "/assinatura",
		DevMode:   dev,
		Enabled:   true,
	}

	// Tenant sintético: existe só para o fluxo ter escopo. Nenhum dado de
	// cliente é tocado além da linha de diagnóstico criada aqui.
	sintetico := true
	tenant := uuid.NewString()
	if tenantArg != "" {
		tenant = tenantArg
		sintetico = false
	}
	origem := "tenant sintético"
	if !sintetico {
		origem = "tenant real (nada será apagado)"
	}
	fmt.Printf("%s: %s | oferta: %s/%s | devMode=%v\n\n", origem, tenant, tier, ciclo, dev)

	if !sintetico {
		if antes, errAntes := repositories.NewPlatformSubscriptionRepository(pgDB).Get(ctx, tenant); errAntes == nil && antes != nil {
			checkout := "—"
			if antes.Checkout != nil {
				checkout = antes.Checkout.ID + " " + antes.Checkout.Plan.Cycle
			}
			fmt.Printf("estado visível pelo serviço: status=%s | checkout=%s | trial=%v\n\n", antes.Status, checkout, antes.TrialGrantedAt != nil)
		}
	}

	state, err := svc.Select(ctx, tenant, tier, ciclo)
	switch {
	case err == nil:
		fmt.Printf("SUCESSO: checkout %s status=%s url=%s\n", state.Checkout.ID, state.Status, state.Checkout.URL)
	case errors.Is(err, ps.ErrConflict):
		fmt.Printf("409 CONFLITO: %v\n", err)
	case errors.Is(err, ps.ErrNotFound):
		fmt.Printf("409 NÃO ENCONTRADO: %v\n", err)
	case errors.Is(err, ps.ErrMethodUnavailable):
		fmt.Printf("409 CONTA SEM COBRANÇA RECORRENTE: %v\n", err)
		fmt.Println("  -> habilite CARD (ou PIX Automático) para a loja no painel do AbacatePay; repetir não resolve")
	case errors.Is(err, ps.ErrUnavailable):
		fmt.Printf("503 INDISPONÍVEL: %v\n", err)
	default:
		fmt.Printf("ERRO NÃO MAPEADO: %v\n", err)
	}

	// Limpeza: só para o tenant sintético. Num tenant real a linha é o estado
	// legítimo (checkout pendente que a tela reusa) e não pode ser apagada.
	if sintetico {
		if _, err := pgDB.Exec(ctx, "DELETE FROM financial.platform_subscriptions WHERE tenant_id=$1", tenant); err != nil {
			fmt.Fprintln(os.Stderr, "limpeza:", err)
		} else {
			fmt.Println("linha do tenant sintético removida")
		}
	} else {
		fmt.Println("tenant real: nenhuma linha foi alterada ou removida")
	}
}

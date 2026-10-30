// Comando de diagnóstico: confere, para cada oferta do catálogo, se o produto
// correspondente existe no AbacatePay com preço, ciclo, status, devMode e trial
// exatamente como a aplicação espera.
//
// É o MESMO código que o checkout usa (`EnsureProduct`), então um `CONFLITO`
// aqui é exatamente o 409 que o operador vê na tela de assinatura. Somente
// leitura quando o produto existe; cria somente o que estiver faltando.
//
//	PAYMENT_GATEWAY_API_KEY=... ABACATEPAY_DEV_MODE=false \
//	  go run ./cmd/diag-subscription-products
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

func main() {
	chave := os.Getenv("PAYMENT_GATEWAY_API_KEY")
	if chave == "" {
		fmt.Fprintln(os.Stderr, "PAYMENT_GATEWAY_API_KEY ausente")
		os.Exit(2)
	}
	dev := os.Getenv("ABACATEPAY_DEV_MODE") == "true"
	provider := providers.NewAbacateSubscriptionProvider(chave, dev)
	fmt.Printf("devMode esperado: %v\n\n", dev)

	falhas := 0
	for _, plan := range services.PlatformPlans() {
		if plan.ContactOnly || plan.Cycle == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		id, err := provider.EnsureProduct(ctx, plan)
		cancel()

		oferta := fmt.Sprintf("%-8s %-9s R$ %8.2f trial=%d", plan.Tier, plan.Cycle, float64(plan.AmountCents)/100, plan.TrialDays)
		switch {
		case err == nil:
			fmt.Printf("OK        %s -> produto %s\n", oferta, id)
		case errors.Is(err, ps.ErrConflict):
			falhas++
			fmt.Printf("CONFLITO  %s -> o produto cadastrado no provedor não bate com a oferta (preço, ciclo, status, devMode ou trial). É este o 409 da tela.\n", oferta)
		default:
			falhas++
			fmt.Printf("ERRO      %s -> %v\n", oferta, err)
		}
	}
	if falhas > 0 {
		fmt.Printf("\n%d oferta(s) com problema\n", falhas)
		os.Exit(1)
	}
	fmt.Println("\ntodas as ofertas do catálogo conferem com o provedor")
}

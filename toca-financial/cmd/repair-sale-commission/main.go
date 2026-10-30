// Command repair-sale-commission corrige o rateio de comissão de uma venda já
// materializada (plano congelado na assinatura, cobranças pagas e comissão
// criada) — o caso em que o negócio foi assinado com um rateio errado e a
// alteração posterior de `broker_participations` não recalcula o que existe.
//
// O comando NÃO inventa percentuais: resolve o plano a partir do rateio atual
// do negócio e das regras de comissão por papel do tenant — exatamente a mesma
// resolução usada pelo preview e pela assinatura —, então:
//
//	1. cancela a comissão errada (e o payout dela), revertendo o accrual;
//	2. materializa as fatias corretas com o mesmo lastro de caixa
//	   (lançamento no ledger + accrual contábil + obrigação + payout coberto).
//
// Roda em modo de leitura por padrão; escreve apenas com -apply.
//
// Uso:
//
//	DATABASE_URI_SERVICE=... go run ./cmd/repair-sale-commission \
//	  -tenant <id> -deal <id> -contract <id> -commission <id> [-payout-order <id>] -apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

func main() {
	tenantID := flag.String("tenant", "", "tenant_id")
	dealID := flag.String("deal", "", "deal_id")
	contractID := flag.String("contract", "", "contract_id")
	commissionID := flag.String("commission", "", "commission_id errada a cancelar")
	payoutOrderID := flag.String("payout-order", "", "payout_order_id da comissão errada (opcional)")
	reason := flag.String("reason", "", "motivo registrado no audit trail")
	actor := flag.String("actor", "repair-sale-commission", "quem executou o reparo")
	apply := flag.Bool("apply", false, "executa o reparo (sem isso, apenas mostra o plano)")
	fixAccrualOnly := flag.Bool("fix-accrual-only", false, "apenas estorna o accrual contábil de uma comissão já cancelada")
	flag.Parse()

	uri := strings.TrimSpace(os.Getenv("DATABASE_URI_SERVICE"))
	if uri == "" {
		uri = strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URI"))
	}
	if uri == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URI_SERVICE é obrigatório")
		os.Exit(2)
	}
	for name, value := range map[string]string{
		"tenant": *tenantID, "deal": *dealID, "contract": *contractID, "commission": *commissionID,
	} {
		if strings.TrimSpace(value) == "" {
			fmt.Fprintf(os.Stderr, "-%s é obrigatório\n", name)
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pgDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conectar no banco: %v\n", err)
		os.Exit(1)
	}
	defer pgDB.Close()

	if *fixAccrualOnly {
		commissionRepo := repositories.NewCommissionPostgresRepository(pgDB)
		journalRepo := repositories.NewJournalPostgresRepository(pgDB)
		err := pgDB.RunInServiceContext(ctx, func(txCtx context.Context) error {
			return services.ReverseCancelledCommissionAccrual(txCtx, services.RepairCommissionDistributionDeps{
				DB:          pgDB,
				Commissions: services.NewCommissionService(commissionRepo).WithDB(pgDB),
				Journal:     services.NewJournalService(journalRepo).WithDB(pgDB),
			}, strings.TrimSpace(*tenantID), strings.TrimSpace(*commissionID), strings.TrimSpace(*reason), strings.TrimSpace(*actor))
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "estorno do accrual falhou: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("accrual da comissão cancelada estornado.")
		return
	}

	plan, err := resolveCorrectedPlan(ctx, pgDB, *tenantID, *dealID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolver plano corrigido: %v\n", err)
		os.Exit(1)
	}

	commissionRepo := repositories.NewCommissionPostgresRepository(pgDB)
	payoutRepo := repositories.NewPayoutOrderPostgresRepository(pgDB)
	txRepo := repositories.NewTransactionPostgresRepository(pgDB)
	journalRepo := repositories.NewJournalPostgresRepository(pgDB)

	commissionService := services.NewCommissionService(commissionRepo).WithDB(pgDB)
	txService := services.NewTransactionService(txRepo).WithDB(pgDB)
	commissionService.WithManualTransactionService(txService)
	payoutService := services.NewPayoutOrderService(payoutRepo)
	// O cancelamento do payout cascateia para a comissão ligada; sem o serviço
	// de comissão o agregado recusa a transição (fail-closed).
	payoutService.SetCommissionService(commissionService)
	// WithDB: o estorno do journal grava a reversão e a entrada compensatória no
	// MESMO commit (constraint do razão exige exatamente uma compensação).
	journalService := services.NewJournalService(journalRepo).WithDB(pgDB)

	in := services.RepairCommissionDistributionInput{
		TenantID:            strings.TrimSpace(*tenantID),
		DealID:              strings.TrimSpace(*dealID),
		ContractID:          strings.TrimSpace(*contractID),
		CancelCommissionID:  strings.TrimSpace(*commissionID),
		CancelPayoutOrderID: strings.TrimSpace(*payoutOrderID),
		Reason:              strings.TrimSpace(*reason),
		Actor:               strings.TrimSpace(*actor),
		EventID:             fmt.Sprintf("commission.repair:%s:%s", strings.TrimSpace(*contractID), plan.PlanHash),
		PlanHash:            plan.PlanHash,
		Shares:              plan.Shares,
		DryRun:              !*apply,
	}

	result, err := services.RepairCommissionDistribution(ctx, services.RepairCommissionDistributionDeps{
		DB:           pgDB,
		Commissions:  commissionService,
		Payouts:      payoutService,
		Transactions: txService,
		Journal:      journalService,
	}, in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reparo falhou: %v\n", err)
		os.Exit(1)
	}

	printResult(plan, result, *apply)
}

// resolveCorrectedPlan lê o negócio e as regras do tenant e resolve o plano
// corrigido com a MESMA função do pipeline (ResolveCommissionPlan), para o
// reparo carregar o hash que o runtime produziria hoje para este negócio.
func resolveCorrectedPlan(ctx context.Context, db *database.PostgresDB, tenantID, dealID string) (services.ResolvedCommissionPlan, error) {
	var dealRow struct {
		DealType        string  `db:"deal_type"`
		Value           float64 `db:"value"`
		CommissionPct   float64 `db:"commission_pct"`
		Participations  []byte  `db:"participations"`
	}
	if err := db.DB.GetContext(ctx, &dealRow, `
		SELECT COALESCE(metadata->>'type','')            AS deal_type,
		       COALESCE((metadata->>'value')::numeric,0) AS value,
		       COALESCE((metadata->>'commission')::numeric,0) AS commission_pct,
		       COALESCE(metadata->'broker_participations','[]'::jsonb) AS participations
		  FROM deal.deals WHERE id = $1`, dealID); err != nil {
		return services.ResolvedCommissionPlan{}, err
	}
	var bps []contracts.BrokerParticipation
	if err := json.Unmarshal(dealRow.Participations, &bps); err != nil {
		return services.ResolvedCommissionPlan{}, fmt.Errorf("broker_participations inválido: %w", err)
	}

	settings := &tenant_financial_settings.Settings{}
	var settingsRow struct {
		Rent      float64 `db:"rent"`
		Sale      float64 `db:"sale"`
		AdminFee  float64 `db:"admin_fee"`
		DueDay    int     `db:"due_day"`
	}
	if err := db.DB.GetContext(ctx, &settingsRow, `
		SELECT COALESCE((metadata->>'default_commission_pct_rent')::numeric,0)  AS rent,
		       COALESCE((metadata->>'default_commission_pct_sale')::numeric,0)  AS sale,
		       COALESCE((metadata->>'admin_fee_pct')::numeric,0)                AS admin_fee,
		       COALESCE((metadata->>'default_due_day')::int,0)                  AS due_day
		  FROM financial.tenant_financial_settings WHERE tenant_id = $1`, tenantID); err != nil {
		return services.ResolvedCommissionPlan{}, err
	}
	settings.DefaultCommissionPctRent = settingsRow.Rent
	settings.DefaultCommissionPctSale = settingsRow.Sale
	settings.AdminFeePct = settingsRow.AdminFee
	settings.DefaultDueDay = settingsRow.DueDay

	rows, err := db.DB.QueryContext(ctx, `
		SELECT role, COALESCE((metadata->>'default_percentage')::numeric,0) AS pct
		  FROM financial.tenant_commission_rules WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return services.ResolvedCommissionPlan{}, err
	}
	defer rows.Close()
	var rules []commission.Rule
	for rows.Next() {
		var role string
		var pct float64
		if err := rows.Scan(&role, &pct); err != nil {
			return services.ResolvedCommissionPlan{}, err
		}
		rules = append(rules, commission.Rule{TenantID: tenantID, Role: commission.Role(role), DefaultPercentage: pct})
	}
	if err := rows.Err(); err != nil {
		return services.ResolvedCommissionPlan{}, err
	}

	commissionPct := services.ResolveCommissionPercentage(dealRow.DealType, dealRow.CommissionPct, settings)
	return services.ResolveCommissionPlan(services.CommissionPlanInput{
		DealType:             dealRow.DealType,
		DealValue:            dealRow.Value,
		CommissionPct:        commissionPct,
		BrokerParticipations: bps,
		Rules:                rules,
		TenantSettings:       settings,
	})
}

func printResult(plan services.ResolvedCommissionPlan, result *services.RepairCommissionDistributionResult, applied bool) {
	mode := "SIMULAÇÃO (-apply para executar)"
	if applied {
		mode = "APLICADO"
	}
	fmt.Printf("modo: %s\n", mode)
	fmt.Printf("plano corrigido: %s\n", plan.PlanHash)
	fmt.Printf("comissão total: R$ %.2f | retenção da imobiliária: R$ %.2f (%.2f%%)\n",
		float64(plan.TotalAmountCents)/100, float64(plan.RetainedAmountCents)/100, plan.RetainedPercentage)
	fmt.Printf("comissão cancelada: %s (%.2f%% / R$ %.2f)\n",
		result.CancelledCommission, result.OriginalPercentage, float64(result.OriginalAmountCents)/100)
	if result.CancelledPayoutOrder != "" {
		fmt.Printf("payout cancelado: %s\n", result.CancelledPayoutOrder)
	}
	var total int64
	for _, share := range result.Shares {
		total += share.AmountCents
		fmt.Printf("  - %-12s %6.2f%%  R$ %10.2f  commission=%s payout=%s lastro=%d entrada(s)/R$ %.2f\n",
			share.Role, share.Percentage, float64(share.AmountCents)/100,
			orDash(share.CommissionID), orDash(share.PayoutOrderID), share.FundingEntries,
			float64(share.FundedAmountCents)/100)
	}
	fmt.Printf("total pago a corretores: R$ %.2f | retido: R$ %.2f\n",
		float64(total)/100, float64(plan.RetainedAmountCents)/100)
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// Command repair-portal-accounts garante a conta de portal das partes e
// signatários que ficaram sem identidade — o caso em que a pessoa pede o código
// de acesso no portal, o toca-auth responde sucesso genérico (não confirma
// existência de conta) e nenhum e-mail é enviado: "código de acesso nunca
// recebido".
//
// O runtime passou a provisionar toda parte/signatário com e-mail na criação do
// negócio e em cada mutação que introduz contato (nova parte, novo signatário,
// edição de e-mail). Este comando repara o que já estava gravado: percorre os
// negócios ativos, chama o mesmo `ProvisionPortalAccounts` do runtime e persiste
// o agregado — idempotente (quem já tem conta não é tocado nem consultado).
//
// Roda em modo de leitura por padrão; escreve apenas com -apply.
//
// Uso (dentro do container do toca-deal, que tem AUTH_SERVICE_URL,
// INTERNAL_API_KEY e DATABASE_URI_SERVICE):
//
//	repair-portal-accounts [-tenant <id>] [-limit 500] -apply
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

func main() {
	tenantFlag := flag.String("tenant", "", "tenant_id (vazio = todos os tenants)")
	limitFlag := flag.Int("limit", 500, "máximo de negócios avaliados")
	applyFlag := flag.Bool("apply", false, "executa o reparo (sem isso, apenas lista)")
	flag.Parse()

	tenantID := strings.TrimSpace(*tenantFlag)
	limit := *limitFlag
	apply := *applyFlag

	uri := strings.TrimSpace(os.Getenv("DATABASE_URI_SERVICE"))
	if uri == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URI_SERVICE é obrigatório (o reparo escreve no pool de serviço)")
		os.Exit(2)
	}
	if limit <= 0 {
		fmt.Fprintln(os.Stderr, "-limit deve ser positivo")
		os.Exit(2)
	}
	if apply {
		if strings.TrimSpace(os.Getenv("AUTH_SERVICE_URL")) == "" || strings.TrimSpace(os.Getenv("INTERNAL_API_KEY")) == "" {
			fmt.Fprintln(os.Stderr, "AUTH_SERVICE_URL e INTERNAL_API_KEY são obrigatórios com -apply (é o serviço que cria a conta de portal)")
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	pgDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conectar no banco: %v\n", err)
		os.Exit(1)
	}
	defer pgDB.Close()

	dealRepo := repositories.NewDealPostgresRepository(pgDB)
	repairRepo, ok := dealRepo.(deal.PortalAccountRepairRepository)
	if !ok {
		fmt.Fprintln(os.Stderr, "repositório de negócios não implementa a leitura do reparo")
		os.Exit(1)
	}

	// Leitura pelo pool de serviço: sem claims de tenant a RLS do pool de
	// aplicação devolveria zero linhas em silêncio.
	var candidates []*deal.Deal
	err = pgDB.RunInServiceContext(ctx, func(txCtx context.Context) error {
		var listErr error
		candidates, listErr = repairRepo.ListForPortalAccountRepair(txCtx, tenantID, limit)
		return listErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "listar negócios com contato sem conta: %v\n", err)
		os.Exit(1)
	}

	mode := "SIMULAÇÃO (-apply para executar)"
	if apply {
		mode = "APLICADO"
	}
	fmt.Printf("modo: %s\n", mode)
	fmt.Printf("tenant: %s\n", orAll(tenantID))
	fmt.Printf("negócios com contato sem conta de portal: %d\n", len(candidates))
	if len(candidates) == 0 {
		fmt.Println("nada a reparar.")
		return
	}

	service := services.NewDealService(dealRepo, nil, nil, nil, nil)
	provisioned, failures := 0, 0
	for _, d := range candidates {
		contacts := services.PortalContactsWithoutAccount(d)
		emails := make([]string, 0, len(contacts))
		for _, c := range contacts {
			emails = append(emails, c.Email)
		}
		line := fmt.Sprintf("  - tenant=%s negócio=%s contatos=%s", d.TenantID, d.ID, strings.Join(emails, ", "))
		if !apply {
			if len(contacts) == 0 {
				// Nada a criar: o negócio só precisa espelhar entre parte e
				// signatário um account_id que já existe em um dos registros.
				fmt.Printf("%s -> espelharia a conta já existente\n", line)
				continue
			}
			fmt.Printf("%s -> criaria as contas\n", line)
			continue
		}

		changed := service.ProvisionPortalAccounts(ctx, d)
		if !changed {
			// Sem mudança e com contatos pendentes = o serviço de identidade
			// recusou todas as tentativas (WARN no log deste processo).
			if len(contacts) > 0 {
				failures++
				fmt.Printf("%s -> FALHOU (nenhuma conta criada; ver WARN acima)\n", line)
				continue
			}
			fmt.Printf("%s -> ignorado (nada a fazer)\n", line)
			continue
		}
		created, missing := splitContacts(d, contacts)
		err := pgDB.RunInServiceContext(ctx, func(txCtx context.Context) error {
			return dealRepo.Update(txCtx, d)
		})
		if err != nil {
			failures++
			fmt.Printf("%s -> FALHOU ao persistir: %v\n", line, err)
			continue
		}
		provisioned += len(created)
		switch {
		case len(created) == 0:
			// Só espelhamento de conta já existente entre os registros.
			fmt.Printf("%s -> conta existente espelhada em parte/signatário\n", line)
		case len(missing) > 0:
			failures++
			fmt.Printf("%s -> criadas: %s | pendentes: %s\n", line, strings.Join(created, ", "), strings.Join(missing, ", "))
		default:
			fmt.Printf("%s -> criadas: %s\n", line, strings.Join(created, ", "))
		}
	}
	if apply {
		fmt.Printf("contas criadas: %d | negócios com falha: %d\n", provisioned, failures)
	}
	if failures > 0 {
		os.Exit(1)
	}
}

// splitContacts separa os contatos que passaram a ter conta dos que continuaram
// sem (falha do serviço de identidade) para o relatório do operador.
func splitContacts(d *deal.Deal, contacts []services.PortalContact) (created, missing []string) {
	created, missing = []string{}, []string{}
	for _, c := range contacts {
		if hasAccount(d, c.Email) {
			created = append(created, c.Email)
			continue
		}
		missing = append(missing, c.Email)
	}
	return created, missing
}

func hasAccount(d *deal.Deal, email string) bool {
	for _, p := range d.Parties {
		if strings.EqualFold(strings.TrimSpace(p.Email), email) && strings.TrimSpace(p.AccountID) != "" {
			return true
		}
	}
	for _, s := range d.Signers {
		if strings.EqualFold(strings.TrimSpace(s.Email), email) && strings.TrimSpace(s.AccountID) != "" {
			return true
		}
	}
	return false
}

func orAll(value string) string {
	if value == "" {
		return "(todos)"
	}
	return value
}

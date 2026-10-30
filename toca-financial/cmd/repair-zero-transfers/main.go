// Command repair-zero-transfers encerra repasses ao proprietário que ficaram
// pendentes com R$ 0,00 a repassar — o caso em que a renda foi totalmente
// consumida pela taxa de administração, por deduções e/ou por contas
// administradas, mas a transição que zerou o saldo aconteceu antes de o sistema
// passar a encerrar esses repasses automaticamente (movimento de reserva,
// dedução ou adoção de composição por cobrança paga).
//
// Sem esse encerramento o repasse fica PENDING para sempre: não há valor a
// transferir, a conciliação o conta como "aguardando baixa" e nenhuma baixa
// manual funciona (não existe pagamento a registrar). O comando usa exatamente
// a mesma transição do runtime — `SettleDanglingWithoutPayableBalance`, que
// fecha sem lançamento de caixa e registra o encerramento no histórico e no
// audit — e é idempotente.
//
// Roda em modo de leitura por padrão; escreve apenas com -apply.
//
// Uso:
//
//	DATABASE_URI_SERVICE=... go run ./cmd/repair-zero-transfers [-tenant <id>] [-limit 500] -apply
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
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

type candidate struct {
	ID             string
	TenantID       string
	ContractID     string
	OwnerID        string
	ReferenceMonth string
	Status         transfer.Status
	GrossRent      float64
	ReservedCents  int64
	AvailableCents int64
	Source         string
}

func main() {
	tenantFlag := flag.String("tenant", "", "tenant_id (vazio = todos os tenants)")
	actorFlag := flag.String("actor", "repair-zero-transfers", "quem executou o reparo (audit + histórico)")
	limitFlag := flag.Int("limit", 500, "máximo de repasses avaliados")
	applyFlag := flag.Bool("apply", false, "executa o reparo (sem isso, apenas lista)")
	flag.Parse()

	tenantID := strings.TrimSpace(*tenantFlag)
	actor := strings.TrimSpace(*actorFlag)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pgDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conectar no banco: %v\n", err)
		os.Exit(1)
	}
	defer pgDB.Close()

	candidates, err := listCandidates(ctx, pgDB, tenantID, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listar repasses zerados: %v\n", err)
		os.Exit(1)
	}

	mode := "SIMULAÇÃO (-apply para executar)"
	if apply {
		mode = "APLICADO"
	}
	fmt.Printf("modo: %s\n", mode)
	fmt.Printf("tenant: %s\n", orAll(tenantID))
	fmt.Printf("repasses zerados pendentes encontrados: %d\n", len(candidates))
	if len(candidates) == 0 {
		fmt.Println("nada a reparar.")
		return
	}

	transferService := services.NewTransferService(repositories.NewTransferPostgresRepository(pgDB)).WithDB(pgDB)
	settled := 0
	failures := 0
	for _, item := range candidates {
		line := fmt.Sprintf("  - tenant=%s transfer=%s contrato=%s competência=%s bruto=R$ %.2f reservado=R$ %.2f disponível=R$ %.2f status=%s origem=%s",
			item.TenantID, item.ID, item.ContractID, item.ReferenceMonth,
			item.GrossRent, float64(item.ReservedCents)/100, float64(item.AvailableCents)/100, item.Status, orDash(item.Source))
		if !apply {
			fmt.Printf("%s -> encerraria sem saldo\n", line)
			continue
		}
		changed, err := settle(ctx, pgDB, transferService, item, actor)
		if err != nil {
			failures++
			fmt.Printf("%s -> FALHOU: %v\n", line, err)
			continue
		}
		if changed {
			settled++
			fmt.Printf("%s -> encerrado sem saldo\n", line)
			continue
		}
		fmt.Printf("%s -> ignorado (já encerrado ou passou a ter valor a repassar)\n", line)
	}
	if apply {
		fmt.Printf("encerrados: %d | falhas: %d\n", settled, failures)
	}
	if failures > 0 {
		os.Exit(1)
	}
}

func settle(ctx context.Context, pgDB *database.PostgresDB, transferService *services.TransferService, item candidate, actor string) (bool, error) {
	var changed bool
	err := pgDB.RunInServiceContext(ctx, func(txCtx context.Context) error {
		tenantCtx := pkgctx.WithTenantID(txCtx, item.TenantID)
		var err error
		changed, err = transferService.SettleDanglingWithoutPayableBalance(tenantCtx, item.TenantID, item.ID, actor, "Reparo de repasses zerados")
		return err
	})
	return changed, err
}

// listCandidates lê pelo pool de serviço: o reparo roda sem claims de tenant e
// a RLS do pool de aplicação devolveria zero linhas silenciosamente.
func listCandidates(ctx context.Context, pgDB *database.PostgresDB, tenantID string, limit int) ([]candidate, error) {
	query := `
		SELECT id::text, tenant_id::text, metadata
		  FROM financial.transfers
		 WHERE deleted_at IS NULL
		   AND COALESCE((metadata->>'net_amount')::numeric, 0) = 0
		   AND (metadata->>'status') IN ('PENDING', 'APPROVED')
	`
	args := []any{}
	if tenantID != "" {
		query += " AND tenant_id = $1"
		args = append(args, tenantID)
	}
	query += " ORDER BY created_at ASC, id ASC"
	query += fmt.Sprintf(" LIMIT %d", limit)

	out := make([]candidate, 0, limit)
	err := pgDB.RunInServiceContext(ctx, func(txCtx context.Context) error {
		rows, err := pgDB.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				id       string
				tenant   string
				metadata []byte
			)
			if err := rows.Scan(&id, &tenant, &metadata); err != nil {
				return err
			}
			var item transfer.OwnerTransfer
			if err := json.Unmarshal(metadata, &item); err != nil {
				return fmt.Errorf("metadata inválido do repasse %s: %w", id, err)
			}
			// A regra de "nada a repassar" é a do domínio — inclusive a exigência
			// de reserva zerada (conta administrada pendente mantém o repasse
			// aberto de propósito).
			if !item.HasNoPayableBalance() {
				continue
			}
			entry := candidate{
				ID: id, TenantID: tenant, ContractID: item.ContractID, OwnerID: item.OwnerID,
				ReferenceMonth: item.ReferenceMonth, Status: item.Status, GrossRent: item.GrossRent,
			}
			if item.OwnerBalance != nil {
				entry.ReservedCents = item.OwnerBalance.ReservedCents
				entry.AvailableCents = item.OwnerBalance.AvailableCents()
			}
			switch {
			case strings.TrimSpace(item.SourceChargeID) != "":
				entry.Source = "cobrança " + item.SourceChargeID
			case strings.TrimSpace(item.SourceTransactionID) != "":
				entry.Source = "recebimento " + item.SourceTransactionID
			default:
				entry.Source = "manual/legado"
			}
			out = append(out, entry)
		}
		return rows.Err()
	})
	return out, err
}

func orAll(value string) string {
	if strings.TrimSpace(value) == "" {
		return "todos"
	}
	return value
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

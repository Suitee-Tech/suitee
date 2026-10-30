package repositories

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

// Integração (roda só com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE definidos):
// prova que a ESCRITA no ledger funciona no caminho de requisição de usuário.
//
// Incidente de 30/09/2026 — "marcar cobrança como paga" devolvia 500:
//
//	failed to create commission journal entry for broker …:
//	ERROR: new row violates row-level security policy for table "journal_entries" (SQLSTATE 42501)
//
// A migration 20260822123000 tornou o ledger append-only: dropou as policies de
// escrita e revogou INSERT/UPDATE/DELETE de authenticated/anon, deixando só a
// policy de SELECT. O pool de aplicação, portanto, NÃO pode gravar — e o
// repositório gravava por ele (`r.db.QueryRow`/`r.db.Exec`). Consumidores de
// evento escapavam porque rodam em RunInServiceContext; a requisição do usuário
// (reconcile-manual → ensure sale commission snapshot) não, e estourava 42501.
//
// O teste usa exatamente a forma da requisição: tenant no contexto (claims
// injetadas no checkout) e SEM transação de serviço.
func TestJournalPrivilegedWriterWorksFromUserRequestContext(t *testing.T) {
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	if appURI == "" || svcURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI, MaxPoolSize: 4, MinPoolSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID := uuid.NewString()
	repo := NewJournalPostgresRepository(db)
	// A requisição de usuário carrega o tenant no contexto → claims no checkout.
	userCtx := pkgcontext.WithTenantID(ctx, tenantID)

	entry, err := journal.New(tenantID, uuid.NewString(), "Comissão - teste do escritor privilegiado", time.Now(),
		[]journal.Leg{
			{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: 1000},
			{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: 1000},
		})
	if err != nil {
		t.Fatalf("montar lançamento: %v", err)
	}
	defer func() {
		_ = db.ExecService(ctx, `DELETE FROM financial.journal_entries WHERE tenant_id = $1`, tenantID)
	}()

	if err := repo.Create(userCtx, entry); err != nil {
		t.Fatalf("Create pelo contexto de usuário falhou (era o SQLSTATE 42501 do incidente): %v", err)
	}

	// A linha existe (lida pelo pool privilegiado, já que a policy de SELECT
	// exige as claims do tenant — que aqui existem).
	got, err := repo.GetByID(userCtx, tenantID, entry.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("lançamento não foi gravado")
	}

	// A outra escrita da tabela é o UPDATE (transição de status). A invariante
	// de estorno exige o lançamento espelho no MESMO commit, então aqui o que se
	// prova é o roteamento: um id inexistente devolve (false, nil) — pelo pool de
	// aplicação seria SQLSTATE 42501 em vez de zero linhas.
	missing := *got
	missing.ID = uuid.NewString()
	ok, err := repo.UpdateStatus(userCtx, &missing, got.UpdatedAt)
	if err != nil {
		t.Fatalf("UpdateStatus pelo contexto de usuário falhou (escritor privilegiado não usado?): %v", err)
	}
	if ok {
		t.Error("UpdateStatus afetou uma linha inexistente")
	}

	// E o mesmo caminho continua funcionando dentro de transação de serviço
	// (consumidores de evento), sem quebrar a atomicidade deles.
	inTx := false
	if err := db.RunInServiceContext(ctx, func(txCtx context.Context) error {
		txEntry, txErr := journal.New(tenantID, uuid.NewString(), "Comissão - dentro da transação de serviço", time.Now(),
			[]journal.Leg{
				{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: 500},
				{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: 500},
			})
		if txErr != nil {
			return txErr
		}
		if txErr = repo.Create(txCtx, txEntry); txErr != nil {
			return txErr
		}
		inTx = true
		return errors.New("rollback proposital")
	}); err == nil {
		t.Fatal("RunInServiceContext deveria devolver o erro do callback")
	}
	if !inTx {
		t.Fatal("Create dentro da transação de serviço não rodou")
	}
	var count int
	if err := db.QueryServiceRow(ctx, `SELECT count(*) FROM financial.journal_entries WHERE tenant_id = $1`, tenantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("lançamentos = %d, want 1: a entrada criada dentro da transação de serviço commitou fora dela", count)
	}
}

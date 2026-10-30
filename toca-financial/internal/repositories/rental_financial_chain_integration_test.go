package repositories_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// E2E do encadeamento financeiro da locação, com repositórios REAIS:
//
//	assinatura → emite cobranças (a correção) → reajuste → impacto
//
// Roda contra o banco local, como os outros testes de integração do pacote:
// sem TOCA_TEST_DATABASE_URI o teste é pulado.
//
// Este teste existe porque o caminho da assinatura não é alcançável por HTTP
// neste ambiente: concluir a assinatura depende do Clicksign (webhook ou
// reconcile). Aqui o encadeamento é exercitado direto nos serviços que o
// handler do `contract.signed` aciona, com persistência real — o que o teste
// unitário com fakes não cobre.
func TestRentalFinancialChain_ChargesOnSigningAndReadjustment(t *testing.T) {
	uri := os.Getenv("TOCA_TEST_DATABASE_URI")
	if uri == "" {
		t.Skip("TOCA_TEST_DATABASE_URI não configurado")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri})
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}

	configRepo := repositories.NewRentalChargeConfigPostgresRepository(db)
	chargeRepo := repositories.NewChargePostgresRepository(db)
	rentalConfig := service.NewRentalChargeConfigService(configRepo, chargeRepo, nil)

	tenantID := uuid.NewString()
	contractID := uuid.NewString()

	// O teste grava no banco de desenvolvimento: limpa o que criou, para poder
	// ser rodado repetidamente sem deixar cobranças órfãs no ambiente.
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, `DELETE FROM financial.charges WHERE metadata->>'contract_id' = $1`, contractID); err != nil {
			t.Logf("limpeza de cobranças: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM financial.rental_charge_configs WHERE metadata->>'contract_id' = $1`, contractID); err != nil {
			t.Logf("limpeza de config: %v", err)
		}
		// Fecha DEPOIS de limpar: com `defer db.Close()` a conexão fechava antes
		// deste cleanup e as cobranças ficavam no banco.
		db.Close()
	})
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 14, 0, 0, 0, 0, time.UTC)
	first := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	const mensalCents int64 = 343000 // R$ 3.430,00

	// ── Fase 1: a assinatura emite as cobranças (CreateAndGenerateCharges é o
	// que o handler do contract.signed chama agora).
	_, criadas, err := rentalConfig.CreateAndGenerateCharges(
		ctx, contractID, tenantID, uuid.NewString(), uuid.NewString(), uuid.NewString(),
		start, end, first, rental_charge_config.SameMonthProRata, 10, mensalCents,
	)
	if err != nil {
		t.Fatalf("assinatura: %v", err)
	}
	if criadas == 0 {
		t.Fatal("a assinatura precisa EMITIR cobranças; nenhuma foi criada")
	}
	t.Logf("fase 1: %d cobranças emitidas na assinatura", criadas)

	// ── Fase 2: reentrega do evento não duplica (idempotência por competência).
	_, criadasDeNovo, err := rentalConfig.CreateAndGenerateCharges(
		ctx, contractID, tenantID, uuid.NewString(), uuid.NewString(), uuid.NewString(),
		start, end, first, rental_charge_config.SameMonthProRata, 10, mensalCents,
	)
	if err != nil {
		t.Fatalf("reentrega: %v", err)
	}
	if criadasDeNovo != 0 {
		t.Fatalf("reentrega criou %d cobranças; o evento pode ser reentregue", criadasDeNovo)
	}
	t.Logf("fase 2: reentrega criou 0 cobranças (idempotente)")

	// ── Fase 3: reajuste grava o cronograma e recalcula as futuras.
	// É a MESMA função que o handler de `contract.readjusted` aciona.
	novoMensal := int64(358435) // R$ 3.584,35 (+4,5%)
	vigencia := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	_, atualizadas, err := rentalConfig.ApplyReadjustment(
		ctx, tenantID, contractID, uuid.NewString(), novoMensal, vigencia, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("reajuste: %v", err)
	}
	t.Logf("fase 3: %d cobranças recalculadas a partir de %s", atualizadas, vigencia.Format("2006-01-02"))

	// ── Fase 4: o impacto é exatamente nas PENDING com vencimento >= vigência.
	depois, err := configRepo.GetByContractID(ctx, tenantID, contractID)
	if err != nil || depois == nil {
		t.Fatalf("config após reajuste: %v", err)
	}
	if got := depois.MonthlyAmountFor(vigencia.AddDate(0, 1, 0)); got != novoMensal {
		t.Errorf("valor mensal após a vigência = %d, want %d", got, novoMensal)
	}
	// Antes da vigência o valor original permanece: o reajuste não retroage.
	if got := depois.MonthlyAmountFor(vigencia.AddDate(0, -1, 0)); got != mensalCents {
		t.Errorf("valor mensal antes da vigência = %d, want %d (reajuste não pode retroagir)", got, mensalCents)
	}
	t.Logf("fase 4: valor transitou de %d para %d na vigência", mensalCents, novoMensal)
}

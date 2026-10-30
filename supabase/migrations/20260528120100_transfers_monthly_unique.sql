-- financial.transfers — unique partial index para garantir 1 OwnerTransfer
-- por (tenant, contract, reference_month).
--
-- Motivação: o hook TransferChargeHook.EnsureMonthlyForChargePaid é
-- disparado por ChargeService.MarkPaid sempre que uma charge RENT ou SALE
-- vira PAID. Sem este índice, marcar paga duas charges do mesmo
-- reference_month (cenário raro com correções manuais, ou race-condition
-- por charges duplicadas legacy) criaria múltiplos Transfers para o
-- mesmo período — quebrando o relatório "Meus Repasses" do portal do
-- locador (issue #457).
--
-- O hook usa INSERT ... ON CONFLICT DO NOTHING + re-busca via
-- GetByContractMonth quando colisão; UI avisa "Já existe repasse para
-- este mês". Quando há split SALE legítimo (várias parcelas em meses
-- diferentes), cada uma cai em reference_month distinto e gera Transfer
-- próprio — o índice não atrapalha.
--
-- Domain: toca-financial/internal/domain/transfer/transfer.go
-- Repository: toca-financial/internal/repository/transfer_postgres.go
-- Hook: toca-financial/internal/service/transfer_charge_hook.go

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_monthly
    ON financial.transfers (
        tenant_id,
        ((metadata->>'contract_id')),
        ((metadata->>'reference_month'))
    )
    WHERE deleted_at IS NULL
      AND (metadata->>'contract_id') IS NOT NULL
      AND (metadata->>'reference_month') IS NOT NULL;

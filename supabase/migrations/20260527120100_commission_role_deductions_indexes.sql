-- Phase 6 (end-to-end financial) — índices novos em financial.commissions.
-- A tabela já é JSONB-based (sem migration estrutural pesada); só precisamos
-- de índices em campos que passam a ser query-heavy depois da expansão do
-- domain commission.Commission com Role, Percentage, GrossAmount, Deductions,
-- NetAmount, ContractID, PayoutOrderID.
--
-- Casos de uso que justificam cada índice:
--   * role          — UI filtra commissions por papel do broker (captador,
--                     visitador, fechamento) em /financeiro/comissoes
--   * payout_order_id — settlement flow consulta "qual payout pagou esta
--                     commission" e vice-versa via GET /commissions/by-payout
--   * contract_id   — auditoria fiscal (DIMOB / Informe) e a tela de
--                     contrato listam todas as commissions atadas ao contrato

CREATE INDEX IF NOT EXISTS idx_finance_commissions_tenant_role
    ON financial.commissions (tenant_id, ((metadata->>'role')))
    WHERE deleted_at IS NULL AND (metadata->>'role') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_commissions_tenant_payout_order
    ON financial.commissions (tenant_id, ((metadata->>'payout_order_id')))
    WHERE deleted_at IS NULL AND (metadata->>'payout_order_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_commissions_tenant_contract
    ON financial.commissions (tenant_id, ((metadata->>'contract_id')))
    WHERE deleted_at IS NULL AND (metadata->>'contract_id') IS NOT NULL;

-- Índices de leitura do domínio de inadimplência (F1.4 do plano).
--
-- Motivo: os lookups do motor e do painel filtram JSONB sem índice —
-- `FindByChargeID` usa `metadata->'charge_ids' ? $2` (operador de contenção),
-- `GetStats` agrupa por `metadata->>'status'`, `FindByContract` por
-- `metadata->>'contract_id'` e `FindActiveByDebtor` por `metadata->>'debtor_id'`.
-- Sem índice, cada tick varre a tabela do tenant inteira e cada abertura de tela
-- repete a varredura.
--
-- Escopo: só índices. Nenhuma tabela nova, nenhuma coluna, nenhuma política —
-- por isso esta migration não mexe em RLS (as tabelas já têm, ver
-- 20260718150531_retrofit_tenant_rls_empty_tables.sql).

-- Busca do caso pela cobrança: GIN sobre o array jsonb, que é o operador usado
-- pelo repositório (`?`). Índice de expressão jsonb_path_ops também suportaria
-- `@>`, mas não o `?` — por isso o GIN padrão.
CREATE INDEX IF NOT EXISTS idx_financial_collections_charge_ids
  ON financial.collections USING gin ((metadata->'charge_ids'))
  WHERE deleted_at IS NULL;

-- Painel e métricas: contagem e dívida por status, sempre por tenant.
CREATE INDEX IF NOT EXISTS idx_financial_collections_tenant_status
  ON financial.collections (tenant_id, ((metadata->>'status')))
  WHERE deleted_at IS NULL;

-- Busca do caso pelo contrato (contrato pode ter mais de um caso ao longo do
-- tempo) e pelo devedor (reaproveitamento do caso em atraso recorrente).
CREATE INDEX IF NOT EXISTS idx_financial_collections_tenant_contract
  ON financial.collections (tenant_id, ((metadata->>'contract_id')))
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_financial_collections_tenant_debtor
  ON financial.collections (tenant_id, ((metadata->>'debtor_id')))
  WHERE deleted_at IS NULL;

-- Descoberta do motor: "cobrança não fechada, vencida, deste tenant". A migration
-- anterior só tinha índices isolados de status e due_date (sem tenant) e um
-- (tenant_id, created_at) — nenhum servia à consulta real do tick, que agora
-- roda de hora em hora sobre TODAS as cobranças vencidas (e não apenas as que
-- acabaram de vencer).
CREATE INDEX IF NOT EXISTS idx_financial_charges_tenant_status_due
  ON financial.charges (
    tenant_id,
    ((metadata->>'status')),
    ((metadata->>'due_date'))
  )
  WHERE deleted_at IS NULL;

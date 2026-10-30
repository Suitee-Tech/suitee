-- Wave 1 (1.3) — Quarentena para telefones inválidos.
--
-- A normalização E.164 (pkg/validation/phone.go) agora rejeita inputs
-- malformados na criação/atualização de lead. Para registros legados que
-- já existem no banco com formatação livre, um job de backfill (fora desta
-- migração) deve mover qualquer Phone que não case com a regex E.164 BR
-- para esta tabela, preservando lead_id + raw_value para investigação.
--
-- O índice único parcial em (tenant_id, phone) ainda NÃO é criado nesta
-- migração porque exige backfill completo antes; a Wave 2 promove o
-- campo para coluna dedicada e formaliza a constraint.
CREATE TABLE IF NOT EXISTS crm.lead_phone_quarantine (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id       UUID NOT NULL,
  tenant_id     UUID NOT NULL,
  raw_value     TEXT NOT NULL,
  reason        TEXT NOT NULL DEFAULT 'normalization_failed',
  quarantined_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_phone_quarantine_tenant
  ON crm.lead_phone_quarantine(tenant_id);
CREATE INDEX IF NOT EXISTS idx_lead_phone_quarantine_lead
  ON crm.lead_phone_quarantine(lead_id);

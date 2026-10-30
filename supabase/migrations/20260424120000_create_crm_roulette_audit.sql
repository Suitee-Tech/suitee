CREATE TABLE IF NOT EXISTS crm.roulette_audit (
  id          UUID PRIMARY KEY,
  tenant_id   UUID NOT NULL,
  actor_id    TEXT NOT NULL,
  actor_email TEXT,
  action      TEXT NOT NULL,
  target_id   TEXT,
  diff        JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crm_roulette_audit_tenant_created_at ON crm.roulette_audit (tenant_id, created_at DESC);


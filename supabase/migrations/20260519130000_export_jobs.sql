-- Issue #474: tabela de export jobs (async CSV/PDF). Schema novo
-- "exports" pra isolar dos domain schemas (deal, crm, financial etc.)
-- — exports é uma capability transversal.
--
-- Padrão JSONB-metadata pelo mesmo motivo dos outros domains: o blob
-- inteiro vai em metadata; campos top-level (status, entity, requested_by,
-- tenant_id) ficam fora para queries de fila + listagem.

CREATE SCHEMA IF NOT EXISTS exports;

CREATE TABLE IF NOT EXISTS exports.jobs (
  id            UUID PRIMARY KEY,
  tenant_id     UUID NOT NULL,
  requested_by  UUID NOT NULL,
  entity_type   TEXT NOT NULL,
  format        TEXT NOT NULL,
  status        TEXT NOT NULL,
  metadata      JSONB NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ NULL
);

-- Fila do worker: pega o mais antigo em QUEUED. Filtra por
-- status para o índice ser eficiente (não pega RUNNING/SUCCEEDED).
CREATE INDEX IF NOT EXISTS idx_export_jobs_queue
  ON exports.jobs (created_at ASC)
  WHERE status = 'QUEUED' AND deleted_at IS NULL;

-- Lookup "Meus exports" — usuário lista os próprios.
CREATE INDEX IF NOT EXISTS idx_export_jobs_requester
  ON exports.jobs (tenant_id, requested_by, created_at DESC)
  WHERE deleted_at IS NULL;

-- Lookup admin: listar exports do tenant para auditoria.
CREATE INDEX IF NOT EXISTS idx_export_jobs_tenant_status
  ON exports.jobs (tenant_id, status, created_at DESC)
  WHERE deleted_at IS NULL;

-- Trigger updated_at automático.
CREATE OR REPLACE FUNCTION exports.set_jobs_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at := NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_export_jobs_updated_at ON exports.jobs;
CREATE TRIGGER trg_export_jobs_updated_at
  BEFORE UPDATE ON exports.jobs
  FOR EACH ROW
  EXECUTE FUNCTION exports.set_jobs_updated_at();

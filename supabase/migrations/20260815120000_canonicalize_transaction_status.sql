-- Migration: canonicalize transaction status values in the JSONB pilot table.
--
-- FIN-E2E-25 reconciles legacy Portuguese status strings with the canonical
-- English lifecycle enum persisted by current writers (PENDING, PAID, OVERDUE,
-- REVERSED, CANCELLED). New rows are already written canonically; this only
-- backfills rows written before the enum was unified so that SQL filters such
-- as `metadata->>'status' = 'PAID'` stop missing legacy `Pago` rows.

UPDATE financial.transactions
SET metadata = jsonb_set(
    metadata,
    '{status}',
    CASE metadata->>'status'
        WHEN 'Pendente'  THEN '"PENDING"'::jsonb
        WHEN 'Pago'      THEN '"PAID"'::jsonb
        WHEN 'Em Atraso' THEN '"OVERDUE"'::jsonb
        WHEN 'Estornado' THEN '"REVERSED"'::jsonb
        WHEN 'Cancelado' THEN '"CANCELLED"'::jsonb
        ELSE to_jsonb(metadata->>'status')
    END,
    false
)
WHERE metadata ? 'status'
  AND metadata->>'status' IN ('Pendente', 'Pago', 'Em Atraso', 'Estornado', 'Cancelado');

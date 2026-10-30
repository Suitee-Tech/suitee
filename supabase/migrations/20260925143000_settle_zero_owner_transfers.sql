-- Repasses canônicos sem saldo disponível e sem obrigação reservada não
-- representam pagamento bancário. Eles permanecem no histórico econômico,
-- mas devem estar encerrados para não aparecer como pendência de R$ 0,00.

BEGIN;

WITH candidates AS (
    SELECT
        id,
        tenant_id,
        GREATEST(
            created_at,
            COALESCE(NULLIF(metadata->>'updated_at', '')::timestamptz, created_at)
        ) AS settled_at
    FROM financial.transfers
    WHERE deleted_at IS NULL
      AND UPPER(COALESCE(metadata->>'status', '')) IN ('PENDING', 'APPROVED')
      AND metadata ? 'net_amount'
      AND NULLIF(metadata->>'net_amount', '')::numeric = 0
      AND COALESCE(NULLIF(metadata#>>'{owner_balance,reserved_cents}', '')::bigint, 0) = 0
      AND (
          NULLIF(BTRIM(metadata->>'source_charge_id'), '') IS NOT NULL
          OR NULLIF(BTRIM(metadata->>'source_transaction_id'), '') IS NOT NULL
      )
), settled AS (
    SELECT
        candidate.*,
        jsonb_build_object(
            'at', candidate.settled_at,
            'action', 'processed',
            'actor', 'system',
            'actor_label', 'Sistema',
            'note', 'Liquidado automaticamente sem saldo a repassar.'
        ) AS history_entry
    FROM candidates AS candidate
)
UPDATE financial.transfers AS transfer
SET metadata = jsonb_set(
        transfer.metadata || jsonb_build_object(
            'status', 'PROCESSED',
            'processed_at', settled.settled_at,
            'processed_by', 'system',
            'updated_at', settled.settled_at
        ),
        '{history}',
        (CASE
            WHEN jsonb_typeof(transfer.metadata->'history') = 'array' THEN transfer.metadata->'history'
            ELSE '[]'::jsonb
        END) || jsonb_build_array(settled.history_entry),
        true
    ),
    updated_at = NOW()
FROM settled
WHERE transfer.id = settled.id
  AND transfer.tenant_id = settled.tenant_id;

COMMIT;

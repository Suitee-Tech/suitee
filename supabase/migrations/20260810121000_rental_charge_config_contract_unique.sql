BEGIN;

LOCK TABLE financial.rental_charge_configs IN SHARE ROW EXCLUSIVE MODE;

-- RentalChargeConfig used Go's default PascalCase JSON field names before the
-- domain gained explicit snake_case tags. Rename every known top-level key as
-- JSONB, without converting values to text, so UUIDs, money, booleans, arrays
-- and timestamps retain their original JSON representation. If a rolling
-- deployment already wrote both forms, the current snake_case value wins.
WITH key_map(old_key, new_key) AS (
    VALUES
        ('ID', 'id'),
        ('TenantID', 'tenant_id'),
        ('PropertyID', 'property_id'),
        ('ContractID', 'contract_id'),
        ('TenantPartyID', 'tenant_party_id'),
        ('OwnerID', 'owner_id'),
        ('StartDate', 'start_date'),
        ('EndDate', 'end_date'),
        ('FirstChargeDate', 'first_charge_date'),
        ('FirstChargeType', 'first_charge_type'),
        ('DefaultDueDay', 'default_due_day'),
        ('MonthlyAmount', 'monthly_amount'),
        ('BaseMonthlyAmount', 'base_monthly_amount'),
        ('Adjustments', 'adjustments'),
        ('FirstChargePaid', 'first_charge_paid'),
        ('MonthlyProductRevision', 'monthly_product_revision'),
        ('BundledItems', 'bundled_items'),
        ('CreatedAt', 'created_at'),
        ('UpdatedAt', 'updated_at')
),
rewritten AS (
    SELECT
        cfg.id,
        jsonb_object_agg(
            COALESCE(key_map.new_key, item.key),
            item.value
            ORDER BY item.key
        ) AS metadata
    FROM financial.rental_charge_configs AS cfg
    CROSS JOIN LATERAL jsonb_each(
        CASE
            WHEN jsonb_typeof(cfg.metadata) = 'object' THEN cfg.metadata
            ELSE '{}'::jsonb
        END
    ) AS item(key, value)
    LEFT JOIN key_map ON key_map.old_key = item.key
    WHERE EXISTS (
        SELECT 1
        FROM key_map AS legacy_key
        WHERE cfg.metadata ? legacy_key.old_key
    )
      AND (
          key_map.new_key IS NULL
          OR NOT (cfg.metadata ? key_map.new_key)
      )
    GROUP BY cfg.id
)
UPDATE financial.rental_charge_configs AS cfg
SET metadata = rewritten.metadata
FROM rewritten
WHERE cfg.id = rewritten.id;

-- One active rental charge configuration per contract and tenant. The legacy
-- decoder remains in the application for rolling deploys, while COALESCE also
-- keeps an unexpectedly old row visible instead of allowing a duplicate.
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY tenant_id, COALESCE(metadata->>'contract_id', metadata->>'ContractID')
               ORDER BY created_at DESC, id DESC
           ) AS position
      FROM financial.rental_charge_configs
     WHERE deleted_at IS NULL
       AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') IS NOT NULL
)
UPDATE financial.rental_charge_configs AS cfg
   SET deleted_at = NOW()
  FROM ranked
 WHERE cfg.id = ranked.id
   AND ranked.position > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_financial_rental_charge_configs_contract
    ON financial.rental_charge_configs (
        tenant_id,
        (COALESCE(metadata->>'contract_id', metadata->>'ContractID'))
    )
    WHERE deleted_at IS NULL
      AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') IS NOT NULL;

COMMIT;

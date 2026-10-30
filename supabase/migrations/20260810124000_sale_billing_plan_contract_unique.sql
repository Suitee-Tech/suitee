BEGIN;

-- Freeze both sides of the plan -> charge identity while choosing a canonical
-- row. The application creates the plan before its charges, so this lock order
-- matches the runtime transaction order and prevents a charge from appearing
-- between the safety check and the deduplication update.
LOCK TABLE financial.sale_billing_plan IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE financial.charges IN SHARE MODE;

-- Charges are identified as sale.<plan_id>.<cash-leg>. If two duplicate plans
-- already own any charge rows (including soft-deleted financial history), an
-- automatic merge cannot know which obligations are authoritative or whether
-- a paid leg may be discarded. Abort before changing data and require an
-- explicit financial reconciliation.
DO $$
DECLARE
    unsafe_groups TEXT;
BEGIN
    WITH duplicate_keys AS (
        SELECT
            tenant_id,
            NULLIF(metadata->>'contract_id', '') AS contract_id
        FROM financial.sale_billing_plan
        WHERE deleted_at IS NULL
          AND NULLIF(metadata->>'contract_id', '') IS NOT NULL
        GROUP BY tenant_id, NULLIF(metadata->>'contract_id', '')
        HAVING COUNT(*) > 1
    ),
    plan_charge_counts AS (
        SELECT
            plan.tenant_id,
            duplicate_keys.contract_id,
            plan.id,
            COUNT(charge.id) AS charge_count
        FROM duplicate_keys
        JOIN financial.sale_billing_plan AS plan
          ON plan.tenant_id = duplicate_keys.tenant_id
         AND NULLIF(plan.metadata->>'contract_id', '') = duplicate_keys.contract_id
         AND plan.deleted_at IS NULL
        LEFT JOIN financial.charges AS charge
          ON charge.tenant_id::text = plan.tenant_id
         AND LEFT(
                 charge.metadata->>'source_event_id',
                 LENGTH('sale.' || plan.id::text || '.')
             ) = 'sale.' || plan.id::text || '.'
        GROUP BY plan.tenant_id, duplicate_keys.contract_id, plan.id
    ),
    conflicts AS (
        SELECT
            tenant_id,
            contract_id,
            array_agg(id::text ORDER BY id) FILTER (WHERE charge_count > 0) AS charged_plan_ids
        FROM plan_charge_counts
        GROUP BY tenant_id, contract_id
        HAVING COUNT(*) FILTER (WHERE charge_count > 0) > 1
    )
    SELECT string_agg(
               format(
                   'tenant=%s contract=%s charged_plans=%s',
                   tenant_id,
                   contract_id,
                   array_to_string(charged_plan_ids, ',')
               ),
               '; '
               ORDER BY tenant_id, contract_id
           )
    INTO unsafe_groups
    FROM conflicts;

    IF unsafe_groups IS NOT NULL THEN
        RAISE EXCEPTION
            'unsafe SALE billing-plan deduplication: multiple plans already own charges: %',
            unsafe_groups
            USING ERRCODE = '23514',
                  HINT = 'Reconcile the duplicate SALE obligations explicitly, preserving paid/history rows, then retry this migration. No plan was changed.';
    END IF;
END
$$;

-- With the unsafe case excluded, a plan that already owns materialized
-- obligations is the canonical row. When no duplicate has charges, retain the
-- earliest first-writer. Therefore every soft-deleted loser is guaranteed to
-- have no associated charge rows and no obligation becomes orphaned.
WITH plan_materialization AS (
    SELECT
        plan.id,
        plan.tenant_id,
        NULLIF(plan.metadata->>'contract_id', '') AS contract_id,
        plan.created_at,
        EXISTS (
            SELECT 1
            FROM financial.charges AS charge
            WHERE charge.tenant_id::text = plan.tenant_id
              AND LEFT(
                      charge.metadata->>'source_event_id',
                      LENGTH('sale.' || plan.id::text || '.')
                  ) = 'sale.' || plan.id::text || '.'
        ) AS has_materialized_charges
    FROM financial.sale_billing_plan AS plan
    WHERE plan.deleted_at IS NULL
      AND NULLIF(plan.metadata->>'contract_id', '') IS NOT NULL
),
ranked AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY tenant_id, contract_id
            ORDER BY has_materialized_charges DESC, created_at ASC, id ASC
        ) AS position
    FROM plan_materialization
)
UPDATE financial.sale_billing_plan AS plan
SET deleted_at = NOW(),
    updated_at = NOW()
FROM ranked
WHERE plan.id = ranked.id
  AND ranked.position > 1;

DROP INDEX IF EXISTS financial.idx_finance_sale_billing_plan_contract;
DROP INDEX IF EXISTS financial.uq_finance_sale_billing_plan_tenant_contract;

CREATE UNIQUE INDEX uq_finance_sale_billing_plan_tenant_contract
    ON financial.sale_billing_plan (
        tenant_id,
        (NULLIF(metadata->>'contract_id', ''))
    )
    WHERE deleted_at IS NULL
      AND NULLIF(metadata->>'contract_id', '') IS NOT NULL;

COMMIT;

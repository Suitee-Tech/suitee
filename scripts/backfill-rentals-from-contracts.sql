-- Backfill financial.rentals for legacy signed locação contracts.
--
-- Why: the Rental aggregate (financial.rentals) is created by the
-- contract.signed financial pipeline — see
-- toca-financial/internal/services/charge_issuance_handler.go
-- (createRentalAggregate -> RentalService.CreateFromDeal). Contracts signed
-- before that pipeline existed have no rental row, so the admin "Ver locação"
-- page (/locacao/:id -> GET /rentals/:id) returns 404 for every one of them.
--
-- This script creates exactly one rental per *signed locação* contract that
-- doesn't have one yet. Participants and value come from deal.deals (the
-- contracts don't carry owner_id); the lease dates come from the contract.
-- Status is set to ACTIVE (the rental for a signed contract). The metadata
-- JSONB intentionally mirrors the field set of the Go rental.Rental struct so
-- the repository's json.Unmarshal reads it back correctly. Dates are copied
-- as the contract's RFC3339 strings (Go time.Time requires RFC3339).
--
-- Idempotent: a contract that already has a non-deleted rental is skipped, so
-- the script is safe to re-run. It deliberately does NOT emit rental.created
-- events (unlike the live pipeline) to avoid notifying owners about old
-- rentals.
--
-- Run (read-only preview first, then the INSERT):
--   psql "$DATABASE_URL" -f scripts/backfill-rentals-from-contracts.sql
-- or execute the INSERT statement via the Supabase SQL editor / MCP.

INSERT INTO financial.rentals (id, tenant_id, metadata, created_at, updated_at)
WITH src AS (
  SELECT
    gen_random_uuid()                                   AS rid,
    c.id::text                                          AS contract_id,
    c.deal_id::text                                     AS deal_id,
    c.tenant_id::text                                   AS tenant_id,
    COALESCE(NULLIF(d.metadata->>'property_id', ''),
             NULLIF(c.metadata->>'property_id', ''))    AS property_id,
    d.metadata->>'customer_id'                          AS tenant_lead_id,
    d.metadata->>'owner_id'                             AS owner_lead_id,
    c.metadata->>'start_date'                           AS start_date,
    c.metadata->>'end_date'                             AS end_date,
    round(COALESCE(NULLIF(d.metadata->>'value', ''),
                   NULLIF(c.metadata->>'value', ''))::numeric * 100)::bigint AS monthly_value_cents,
    COALESCE(c.metadata->>'created_at',
             to_char(c.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')) AS created_at
  FROM deal.contracts c
  JOIN deal.deals d ON d.id = c.deal_id
  WHERE c.metadata->>'type'   = 'locacao'
    AND c.metadata->>'status' = 'SIGNED'
    AND NOT EXISTS (
      SELECT 1 FROM financial.rentals r
      WHERE r.tenant_id = c.tenant_id::text
        AND r.deleted_at IS NULL
        AND r.metadata->>'contract_id' = c.id::text
    )
)
SELECT
  src.rid,
  src.tenant_id,
  jsonb_build_object(
    'id',                  src.rid::text,
    'tenant_id',           src.tenant_id,
    'deal_id',             src.deal_id,
    'contract_id',         src.contract_id,
    'property_id',         src.property_id,
    'tenant_lead_id',      src.tenant_lead_id,
    'owner_lead_id',       src.owner_lead_id,
    'broker_id',           '',
    'start_date',          src.start_date,
    'end_date',            src.end_date,
    'monthly_value_cents', src.monthly_value_cents,
    'status',              'ACTIVE',
    'created_at',          src.created_at,
    'updated_at',          src.created_at
  ),
  now(),
  now()
FROM src
WHERE src.owner_lead_id  IS NOT NULL AND src.owner_lead_id  <> ''
  AND src.tenant_lead_id IS NOT NULL AND src.tenant_lead_id <> ''
  AND src.property_id    IS NOT NULL AND src.property_id     <> ''
  AND src.monthly_value_cents > 0
  AND src.end_date::timestamptz >= src.start_date::timestamptz;

-- Verify after running:
--   SELECT count(*) FROM financial.rentals WHERE deleted_at IS NULL;          -- expect 17
--   SELECT metadata->>'contract_id', metadata->>'owner_lead_id',
--          metadata->>'monthly_value_cents', metadata->>'status'
--   FROM financial.rentals ORDER BY created_at;

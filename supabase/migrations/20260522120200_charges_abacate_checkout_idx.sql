-- Lets the Abacate Pay webhook resolve a charge from a checkout id quickly.
-- The btree index covers the common case (current checkout); the GIN index
-- covers payments made against a superseded checkout after a charge edit.

CREATE INDEX IF NOT EXISTS idx_finance_charges_abacate_checkout
    ON financial.charges ((metadata->>'abacate_checkout_id'))
    WHERE deleted_at IS NULL AND (metadata->>'abacate_checkout_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_charges_prior_checkouts
    ON financial.charges USING gin ((metadata->'prior_checkout_ids'))
    WHERE deleted_at IS NULL;

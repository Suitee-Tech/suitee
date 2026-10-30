-- Phase 2 (item 2.3) — index maintenance tickets by rental_id and
-- requester_lead_id so LeadDetails "Tickets de manutenção" and
-- RentalDetails "Manutenção" tabs run in O(log n). Domain has the
-- new fields persisted into JSONB metadata; no schema change needed.

CREATE INDEX IF NOT EXISTS idx_ops_maintenance_rental_id
    ON ops.maintenance (tenant_id, ((metadata->>'rental_id')))
    WHERE deleted_at IS NULL AND (metadata->>'rental_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_maintenance_requester_lead_id
    ON ops.maintenance (tenant_id, ((metadata->>'requester_lead_id')))
    WHERE deleted_at IS NULL AND (metadata->>'requester_lead_id') IS NOT NULL;

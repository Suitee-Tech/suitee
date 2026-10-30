-- Supports the conflict check held under the calendar's transaction-scoped
-- advisory lock. Adjacent intervals remain allowed; legacy data is retained.
CREATE INDEX IF NOT EXISTS idx_calendar_appointments_agent_date_active
  ON calendar.appointments (tenant_id, (metadata->>'agent_id'), (metadata->>'date'))
  WHERE deleted_at IS NULL AND COALESCE(metadata->>'status', '') <> 'Cancelado';

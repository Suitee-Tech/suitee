-- A listagem da agenda sempre recorta primeiro pelo responsável e, para a
-- visão gerencial, pela visibilidade. Os índices parciais acompanham o filtro
-- de soft delete para não crescerem com compromissos removidos.
CREATE INDEX IF NOT EXISTS idx_calendar_appointments_visibility_active
  ON calendar.appointments (tenant_id, (metadata->>'visibility'))
  WHERE deleted_at IS NULL;

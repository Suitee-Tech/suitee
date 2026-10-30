-- Both trigger functions already use fully-qualified application relations.
-- Pin an empty search_path so callers cannot influence name resolution.

ALTER FUNCTION crm.prevent_lead_delete_with_active_deal()
    SET search_path = '';

ALTER FUNCTION exports.set_jobs_updated_at()
    SET search_path = '';

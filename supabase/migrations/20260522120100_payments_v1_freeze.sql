-- Freeze pre-existing payments as Abacate Pay v1 ("billing") records. The new
-- v2 integration only syncs/cancels payments tagged api_version = '2'; legacy
-- v1 rows are treated as terminal (no status sync against the v2 API). All new
-- payments are written with api_version '2' by the application.

UPDATE financial.payments
SET metadata = jsonb_set(metadata, '{api_version}', '"1"'::jsonb)
WHERE metadata IS NOT NULL AND NOT (metadata ? 'api_version');

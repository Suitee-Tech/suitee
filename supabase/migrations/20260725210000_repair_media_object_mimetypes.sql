-- Repair media uploaded before MIME detection was propagated to Supabase.
-- Those objects were stored as `image/*`, which is not a concrete response
-- Content-Type and prevents reliable rendering in browsers and viewers.
DO $$
BEGIN
  IF to_regclass('storage.objects') IS NULL THEN
    RETURN;
  END IF;

  UPDATE storage.objects
  SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{mimetype}',
    to_jsonb(
      CASE
        WHEN lower(name) ~ '\.jpe?g$' THEN 'image/jpeg'
        WHEN lower(name) ~ '\.png$' THEN 'image/png'
        WHEN lower(name) ~ '\.webp$' THEN 'image/webp'
        WHEN lower(name) ~ '\.gif$' THEN 'image/gif'
        WHEN lower(name) ~ '\.svg$' THEN 'image/svg+xml'
      END
    ),
    true
  )
  WHERE bucket_id IN (
    'toca-avatars',
    'toca-logos',
    'toca-property-images',
    'toca-message-attachments',
    'toca-documents'
  )
    AND lower(name) ~ '\.(jpe?g|png|webp|gif|svg)$'
    AND (
      metadata ->> 'mimetype' IS NULL
      OR lower(metadata ->> 'mimetype') IN (
        '',
        'image/*',
        'application/octet-stream',
        'application/x-octet-stream',
        'binary/octet-stream'
      )
    );
EXCEPTION
  WHEN insufficient_privilege THEN
    RAISE NOTICE 'Skipping media MIME repair: current role cannot update storage.objects';
END $$;

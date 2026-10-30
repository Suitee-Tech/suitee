INSERT INTO storage.buckets (id, name, public)
VALUES
  ('toca-avatars', 'toca-avatars', true),
  ('toca-logos', 'toca-logos', true),
  ('toca-property-images', 'toca-property-images', true),
  ('toca-documents', 'toca-documents', false),
  ('toca-message-attachments', 'toca-message-attachments', false)
ON CONFLICT (id) DO NOTHING;

ALTER TABLE storage.objects ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS public_read_toca_avatars ON storage.objects;
CREATE POLICY public_read_toca_avatars ON storage.objects FOR SELECT USING (bucket_id = 'toca-avatars');
DROP POLICY IF EXISTS tenant_folder_toca_avatars ON storage.objects;
CREATE POLICY tenant_folder_toca_avatars ON storage.objects FOR ALL USING (bucket_id = 'toca-avatars' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-avatars' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS public_read_toca_logos ON storage.objects;
CREATE POLICY public_read_toca_logos ON storage.objects FOR SELECT USING (bucket_id = 'toca-logos');
DROP POLICY IF EXISTS tenant_folder_toca_logos ON storage.objects;
CREATE POLICY tenant_folder_toca_logos ON storage.objects FOR ALL USING (bucket_id = 'toca-logos' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-logos' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS public_read_toca_property_images ON storage.objects;
CREATE POLICY public_read_toca_property_images ON storage.objects FOR SELECT USING (bucket_id = 'toca-property-images');
DROP POLICY IF EXISTS tenant_folder_toca_property_images ON storage.objects;
CREATE POLICY tenant_folder_toca_property_images ON storage.objects FOR ALL USING (bucket_id = 'toca-property-images' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-property-images' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS tenant_folder_toca_documents ON storage.objects;
CREATE POLICY tenant_folder_toca_documents ON storage.objects FOR ALL USING (bucket_id = 'toca-documents' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-documents' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

DROP POLICY IF EXISTS tenant_folder_toca_message_attachments ON storage.objects;
CREATE POLICY tenant_folder_toca_message_attachments ON storage.objects FOR ALL USING (bucket_id = 'toca-message-attachments' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-message-attachments' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

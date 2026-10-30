# Supabase Storage Buckets

This document maps the business domains to their respective Supabase storage buckets.

## Public Buckets

These buckets contain assets that need to be publicly accessible (e.g. avatars, property images for portal).

| Domain | Bucket | Path Structure |
| --- | --- | --- |
| Accounts/Users | `toca-avatars` | `{tenant_id}/{user_id}/{filename}.{ext}` |
| Company/Tenant | `toca-logos` | `{tenant_id}/logo.{ext}` |
| Properties | `toca-property-images` | `{tenant_id}/{property_id}/{filename}.{ext}` |

## Private Buckets

These buckets contain sensitive files and are protected by Row Level Security (RLS). The first folder in the path MUST be the `tenant_id` to comply with the RLS policy.

| Domain | Bucket | Path Structure |
| --- | --- | --- |
| Deal/Contracts | `toca-documents` | `{tenant_id}/{deal_id}/{filename}.{ext}` |
| Communication | `toca-message-attachments` | `{tenant_id}/{channel_id}/{filename}.{ext}` |
| Financial (Proofs) | `financial-proofs` | `{tenant_id}/{kind}/{yyyy-mm}/{uuid}.{ext}` |

## Row Level Security (RLS)

All private buckets have the following RLS policy on `storage.objects`:

```sql
CREATE POLICY "Tenant Isolation on Private Buckets" ON storage.objects FOR ALL
USING (
  bucket_id IN ('toca-documents', 'toca-message-attachments', 'financial-proofs') AND 
  (storage.foldername(name))[1] = (auth.jwt()->'app_metadata'->>'tenant_id')
)
WITH CHECK (
  bucket_id IN ('toca-documents', 'toca-message-attachments', 'financial-proofs') AND 
  (storage.foldername(name))[1] = (auth.jwt()->'app_metadata'->>'tenant_id')
);
```

This ensures that a user can only access and upload files inside a folder matching their `tenant_id`.

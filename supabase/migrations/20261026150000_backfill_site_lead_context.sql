-- Backfill dos leads do site criados antes do snapshot de imóvel no CRM.
--
-- O toca-sites passou a enviar property_interest + recommended_property_snapshot
-- na criação do lead. Leads antigos ficaram sem esse contexto e o portal não
-- conseguia mostrar o imóvel em "Imóveis do atendimento". O backfill usa o
-- vínculo canônico sites.leads.crm_lead_id + a projeção sites.listings; nunca
-- sobrescreve uma recomendação já definida pelo corretor.
WITH site_lead_context AS (
    SELECT
        cl.id AS crm_lead_id,
        cl.tenant_id,
        l.property_id,
        l.title,
        l.property_type,
        l.listing_type,
        l.sale_price,
        l.rent_price,
        l.address,
        l.features,
        l.images,
        sl.message,
        CASE lower(l.property_type)
            WHEN 'apartment' THEN 'APARTMENT'
            WHEN 'house' THEN 'HOUSE'
            WHEN 'land' THEN 'LAND'
            WHEN 'commercial' THEN 'COMMERCIAL'
            ELSE 'OTHER'
        END AS crm_property_type,
        GREATEST(COALESCE(l.sale_price, 0), COALESCE(l.rent_price, 0)) AS price_max,
        COALESCE(
            (SELECT jsonb_agg(image ->> 'url')
             FROM jsonb_array_elements(COALESCE(l.images, '[]'::jsonb)) AS image
             WHERE COALESCE(image ->> 'url', '') <> ''),
            '[]'::jsonb
        ) AS media_images
    FROM sites.leads sl
    JOIN sites.listings l
      ON l.tenant_id = sl.tenant_id
     AND l.property_id = sl.property_id
    JOIN crm.leads cl
      ON cl.id::text = sl.crm_lead_id
     AND cl.tenant_id = sl.tenant_id
    WHERE sl.crm_lead_id IS NOT NULL
      AND sl.property_id IS NOT NULL
      AND cl.deleted_at IS NULL
      AND cl.metadata ->> 'recommended_property_id' IS NULL
)
UPDATE crm.leads cl
SET metadata = jsonb_set(
        jsonb_set(
            jsonb_set(
                jsonb_set(
                    COALESCE(cl.metadata, '{}'::jsonb),
                    '{recommended_property_id}',
                    to_jsonb(ctx.property_id::text),
                    true
                ),
                '{recommended_property_snapshot}',
                jsonb_build_object(
                    'id', ctx.property_id::text,
                    'title', ctx.title,
                    'neighborhood', ctx.address ->> 'neighborhood',
                    'city', ctx.address ->> 'city',
                    'state', ctx.address ->> 'state',
                    'sale_price', ctx.sale_price,
                    'rent_price', ctx.rent_price,
                    'listing_type', ctx.listing_type,
                    'thumbnail_url', ctx.media_images ->> 0,
                    'media_images', ctx.media_images
                ),
                true
            ),
            '{PropertyInterest}',
            jsonb_build_object(
                'type', ctx.crm_property_type,
                'preferred_location', concat_ws(', ',
                    NULLIF(ctx.address ->> 'neighborhood', ''),
                    NULLIF(ctx.address ->> 'city', ''),
                    NULLIF(ctx.address ->> 'state', '')
                ),
                'price_range', jsonb_build_object('min', 0, 'max', ctx.price_max),
                'features', jsonb_strip_nulls(jsonb_build_object(
                    'bedrooms', ctx.features -> 'bedrooms',
                    'bathrooms', ctx.features -> 'bathrooms',
                    'parking', ctx.features -> 'parking_spaces',
                    'area_min', ctx.features -> 'area_total'
                )),
                'notes', 'Imóvel do site: ' || COALESCE(ctx.title, '') || ' (' || ctx.property_id::text || ')'
            ),
            true
        ),
        '{Qualification}',
        CASE
            WHEN COALESCE(ctx.message, '') = '' THEN COALESCE(cl.metadata -> 'Qualification', '{}'::jsonb)
            ELSE jsonb_set(
                COALESCE(cl.metadata -> 'Qualification', '{}'::jsonb),
                '{site_message}',
                to_jsonb(ctx.message),
                true
            )
        END,
        true
    ),
    updated_at = NOW(),
    updated_seq = cl.updated_seq + 1
FROM site_lead_context ctx
WHERE cl.id = ctx.crm_lead_id
  AND cl.tenant_id = ctx.tenant_id;

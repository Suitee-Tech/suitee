-- Stable identities make commission deduction deletion retry-safe. The first
-- legacy occurrence of a logical deduction receives the exact UUID v5 emitted
-- by events.DeriveNotificationID("commission_deduction", tenant, commission,
-- kind, description, amount). Repeated identical legacy lines get a stable
-- ordinal suffix so they remain individually removable without changing the
-- retry identity of the first line.
WITH expanded AS (
    SELECT
        c.id,
        c.tenant_id,
        item.value,
        item.ordinality,
        NULLIF(BTRIM(item.value->>'id'), '') IS NULL AS missing_id,
        BTRIM(COALESCE(item.value->>'kind', '')) AS kind,
        BTRIM(COALESCE(item.value->>'description', '')) AS description,
        CASE
            WHEN POSITION('.' IN COALESCE(item.value->>'amount', '')) > 0
                THEN RTRIM(RTRIM(item.value->>'amount', '0'), '.')
            ELSE COALESCE(item.value->>'amount', '')
        END AS amount_text
    FROM financial.commissions AS c
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE
            WHEN jsonb_typeof(c.metadata->'deductions') = 'array' THEN c.metadata->'deductions'
            ELSE '[]'::jsonb
        END
    ) WITH ORDINALITY AS item(value, ordinality)
    WHERE c.deleted_at IS NULL
), identified AS (
    SELECT
        expanded.*,
        CONCAT_WS('|',
            'commission_deduction',
            NULLIF(BTRIM(tenant_id::text), ''),
            NULLIF(BTRIM(id::text), ''),
            NULLIF(kind, ''),
            NULLIF(description, ''),
            NULLIF(amount_text, '')
        ) AS identity_key,
        SUM(CASE WHEN missing_id THEN 1 ELSE 0 END) OVER (
            PARTITION BY id, kind, description, amount_text
            ORDER BY ordinality
            ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
        ) AS legacy_occurrence
    FROM expanded
), hashed AS (
    SELECT
        identified.*,
        digest(
            decode('d7b8e6e67e165d3b9c5b1f87a5e8cd6e', 'hex') ||
            convert_to(
                CASE
                    WHEN legacy_occurrence = 1 THEN identity_key
                    ELSE identity_key || '|legacy_occurrence|' || legacy_occurrence::text
                END,
                'UTF8'
            ),
            'sha1'
        ) AS sha1_bytes
    FROM identified
), versioned AS (
    SELECT
        hashed.*,
        set_byte(
            set_byte(sha1_bytes, 6, (get_byte(sha1_bytes, 6) & 15) | 80),
            8,
            (get_byte(sha1_bytes, 8) & 63) | 128
        ) AS uuid_bytes
    FROM hashed
), rewritten AS (
    SELECT
        id,
        tenant_id,
        jsonb_agg(
            CASE
                WHEN missing_id THEN jsonb_set(
                    value,
                    '{id}',
                    to_jsonb(
                        SUBSTRING(encode(uuid_bytes, 'hex'), 1, 8) || '-' ||
                        SUBSTRING(encode(uuid_bytes, 'hex'), 9, 4) || '-' ||
                        SUBSTRING(encode(uuid_bytes, 'hex'), 13, 4) || '-' ||
                        SUBSTRING(encode(uuid_bytes, 'hex'), 17, 4) || '-' ||
                        SUBSTRING(encode(uuid_bytes, 'hex'), 21, 12)
                    ),
                    true
                )
                ELSE value
            END
            ORDER BY ordinality
        ) AS deductions
    FROM versioned
    GROUP BY id, tenant_id
    HAVING bool_or(missing_id)
)
UPDATE financial.commissions AS c
SET metadata = jsonb_set(c.metadata, '{deductions}', rewritten.deductions, true),
    updated_at = NOW()
FROM rewritten
WHERE c.id = rewritten.id
  AND c.tenant_id = rewritten.tenant_id;

-- sites.* — vitrine pública multi-tenant das imobiliárias (ADR 0006).
--
-- O estoque de imóveis continua em core.properties. Este schema guarda apenas:
--   * o registro da vitrine (tenant, marca do host, branding, páginas);
--   * a projeção local das listings (URL canônica, status de publicação);
--   * pageviews e leads do site público (métricas + espelho do CRM).
--
-- Isolamento: todas as tabelas têm tenant_id UUID e RLS no mesmo arquivo. O
-- toca-sites usa o pool de serviço (BYPASSRLS) e filtra tenant_id
-- explicitamente em toda query; as políticas abaixo são o backstop para
-- qualquer leitura via Supabase REST.
--
-- Domain: toca-sites/internal/domain/site
-- Repo:   toca-sites/internal/repository/postgres
-- Host:   {site.slug}.* — ver toca-sites/internal/domain/site/slug.go

CREATE SCHEMA IF NOT EXISTS sites;

-- ───────────────────────────────────────────────────────────────────────────
-- sites.sites — uma vitrine por imobiliária
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sites.sites (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    -- `{marca}` do host comercial `{marca}.imoveis.suitee.com.br`. UNIQUE
    -- global: o host é o discriminador do tenant na zona pública.
    slug              text NOT NULL,
    name              text NOT NULL DEFAULT '',
    enabled           boolean NOT NULL DEFAULT FALSE,
    public_base_url   text NOT NULL DEFAULT '',
    branding          jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- snapshot de contato (Configurações da empresa) projetado no Payload:
    -- e-mail, telefone, WhatsApp, endereço/cidade/UF e CRECI.
    contact           jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- id do doc `tenants` no Payload. Fica nulo enquanto o Payload não é
    -- chamado (dev sem Payload / sync pendente).
    payload_tenant_id text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sites_slug_format CHECK (
        slug ~ '^[a-z0-9]([a-z0-9-]{1,46})[a-z0-9]$'
    ),
    CONSTRAINT sites_slug_not_reserved CHECK (
        slug NOT IN ('www','app','api','admin','portal','sites','mail','cdn','imoveis','suitee')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_sites_tenant ON sites.sites (tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_sites_slug ON sites.sites (slug);
CREATE INDEX IF NOT EXISTS idx_sites_sites_enabled ON sites.sites (slug) WHERE enabled;

DROP POLICY IF EXISTS tenant_isolation ON sites.sites;
CREATE POLICY tenant_isolation ON sites.sites
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    );

ALTER TABLE sites.sites ENABLE ROW LEVEL SECURITY;

-- ───────────────────────────────────────────────────────────────────────────
-- sites.pages — home/sobre/contato por vitrine (conteúdo editado no painel)
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sites.pages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    site_id         uuid NOT NULL REFERENCES sites.sites(id) ON DELETE CASCADE,
    page_key        text NOT NULL,
    title           text NOT NULL DEFAULT '',
    subtitle        text NOT NULL DEFAULT '',
    body            jsonb,
    seo_title       text,
    seo_description text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sites_pages_key CHECK (page_key IN ('home','about','contact'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_pages_site_key ON sites.pages (site_id, page_key);
CREATE INDEX IF NOT EXISTS idx_sites_pages_tenant ON sites.pages (tenant_id);

DROP POLICY IF EXISTS tenant_isolation ON sites.pages;
CREATE POLICY tenant_isolation ON sites.pages
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    );

ALTER TABLE sites.pages ENABLE ROW LEVEL SECURITY;

-- ───────────────────────────────────────────────────────────────────────────
-- sites.listings — projeção do imóvel publicada no Payload
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sites.listings (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    site_id           uuid NOT NULL REFERENCES sites.sites(id) ON DELETE CASCADE,
    property_id       uuid NOT NULL,
    seo_slug          text NOT NULL,
    status            text NOT NULL DEFAULT 'unpublished',
    title             text NOT NULL DEFAULT '',
    description       text NOT NULL DEFAULT '',
    listing_type      text NOT NULL DEFAULT '',
    property_type     text NOT NULL DEFAULT '',
    sale_price        double precision NOT NULL DEFAULT 0,
    rent_price        double precision NOT NULL DEFAULT 0,
    condo_fee         double precision NOT NULL DEFAULT 0,
    property_tax      double precision NOT NULL DEFAULT 0,
    address           jsonb NOT NULL DEFAULT '{}'::jsonb,
    features          jsonb NOT NULL DEFAULT '{}'::jsonb,
    amenities         jsonb NOT NULL DEFAULT '[]'::jsonb,
    images            jsonb NOT NULL DEFAULT '[]'::jsonb,
    source_updated_at timestamptz,
    published_at      timestamptz,
    payload_doc_id    text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sites_listings_status CHECK (status IN ('published','unpublished'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_listings_tenant_property
    ON sites.listings (tenant_id, property_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_listings_tenant_slug
    ON sites.listings (tenant_id, seo_slug);
-- Busca da vitrine: listings publicadas de um site.
CREATE INDEX IF NOT EXISTS idx_sites_listings_site_published
    ON sites.listings (site_id, published_at DESC) WHERE status = 'published';
CREATE INDEX IF NOT EXISTS idx_sites_listings_tenant_status
    ON sites.listings (tenant_id, status);

DROP POLICY IF EXISTS tenant_isolation ON sites.listings;
CREATE POLICY tenant_isolation ON sites.listings
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    );

ALTER TABLE sites.listings ENABLE ROW LEVEL SECURITY;

-- ───────────────────────────────────────────────────────────────────────────
-- sites.listing_views — pageviews (NÃO vão para o RabbitMQ)
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sites.listing_views (
    id           bigserial PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    site_id      uuid NOT NULL REFERENCES sites.sites(id) ON DELETE CASCADE,
    listing_id   uuid REFERENCES sites.listings(id) ON DELETE CASCADE,
    property_id  uuid NOT NULL,
    -- hash da sessão (nunca o id cru) para deduplicação grosseira
    session_key  text,
    path         text,
    referrer     text,
    utm_source   text,
    utm_medium   text,
    utm_campaign text,
    country      text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sites_views_tenant_property_created
    ON sites.listing_views (tenant_id, property_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sites_views_tenant_created
    ON sites.listing_views (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sites_views_site_created
    ON sites.listing_views (site_id, created_at DESC);

DROP POLICY IF EXISTS tenant_isolation ON sites.listing_views;
CREATE POLICY tenant_isolation ON sites.listing_views
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    );

ALTER TABLE sites.listing_views ENABLE ROW LEVEL SECURITY;

-- ───────────────────────────────────────────────────────────────────────────
-- sites.leads — espelho local do lead do site (CRM é a fonte de verdade)
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sites.leads (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    site_id      uuid NOT NULL REFERENCES sites.sites(id) ON DELETE CASCADE,
    listing_id   uuid REFERENCES sites.listings(id) ON DELETE SET NULL,
    property_id  uuid,
    name         text NOT NULL,
    email        text,
    phone        text,
    message      text,
    status       text NOT NULL DEFAULT 'received',
    crm_lead_id  text,
    origin_host  text NOT NULL DEFAULT '',
    utm_source   text,
    utm_medium   text,
    utm_campaign text,
    error        text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sites_leads_status CHECK (status IN ('received','processed','failed','duplicate'))
);

CREATE INDEX IF NOT EXISTS idx_sites_leads_tenant_created
    ON sites.leads (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sites_leads_tenant_property_created
    ON sites.leads (tenant_id, property_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sites_leads_tenant_email
    ON sites.leads (tenant_id, lower(email));
CREATE INDEX IF NOT EXISTS idx_sites_leads_crm
    ON sites.leads (crm_lead_id) WHERE crm_lead_id IS NOT NULL;

DROP POLICY IF EXISTS tenant_isolation ON sites.leads;
CREATE POLICY tenant_isolation ON sites.leads
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF((SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id', '')::uuid
    );

ALTER TABLE sites.leads ENABLE ROW LEVEL SECURITY;

-- Endurece o contrato de slugs da vitrine (ADR 0006).
--
-- O host `{marca}.imoveis...` precisa ser único e semanticamente legível; o
-- slug do anúncio precisa ser um path válido e único por imobiliária. A
-- aplicação valida antes, mas as constraints são a última linha de defesa
-- contra corrida, importação e escrita administrativa.

ALTER TABLE sites.sites DROP CONSTRAINT IF EXISTS sites_slug_not_reserved;
ALTER TABLE sites.sites ADD CONSTRAINT sites_slug_not_reserved CHECK (
    slug NOT IN (
        'www','app','api','admin','portal','sites','mail','cdn','imoveis','suitee',
        'blog','docs','status','suporte','atendimento','conta','login','painel',
        'sistema','checkout','assets','static','media','storage','webhook','webhooks',
        'root','null','undefined','test','teste','demo','dev','staging','beta',
        'imobiliaria','imovel'
    )
);

ALTER TABLE sites.sites DROP CONSTRAINT IF EXISTS sites_slug_semantic;
ALTER TABLE sites.sites ADD CONSTRAINT sites_slug_semantic CHECK (slug ~ '[a-z]');

ALTER TABLE sites.listings DROP CONSTRAINT IF EXISTS sites_listings_seo_slug_format;
ALTER TABLE sites.listings ADD CONSTRAINT sites_listings_seo_slug_format CHECK (
    seo_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'
    AND seo_slug <> ''
);

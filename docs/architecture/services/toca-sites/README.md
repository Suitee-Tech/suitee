# toca-sites — vitrine pública das imobiliárias

Microsserviço Go (`:8094`) que orquestra o site público multi-tenant. Decisão de
arquitetura completa em [ADR 0006](../../adrs/0006-sites-payload-projection.md)
e no plano `docs/architecture/plans/sites.md`.

**Resumo de responsabilidades**

| Responsabilidade | Onde fica |
| --- | --- |
| Estoque de imóveis (`status`, `publication_status`, preço, mídia) | **toca-core** (`core.properties`) |
| Registro da vitrine, slug do host, páginas, branding | **toca-sites** (`sites.*`) |
| Render público (SSR/SEO) | **toca-web** (Payload 3 + Next) |
| Lead (fonte de verdade) | **toca-crm** |
| Pageviews e agregados | **toca-sites** (`sites.listing_views`) |
| Eventos de transição | RabbitMQ via outbox (`site.*`) |

O Payload é uma **projeção**. Nunca é source of truth de imóvel, lead ou
pagamento. O admin do Payload **não** é o painel da imobiliária: agências usam o
Toka (`/imoveis`, `/site`).

## Host comercial

A zona pública é `{marca}.imoveis.suitee.com.br` (produção) e
`{marca}.imoveis.localhost` (local). `{marca}` é `sites.sites.slug`.

A identidade pública segue a ordem **imobiliária → Suitee**: o logo da
imobiliária (URL/mídia do Payload) tem prioridade; sem ele, o site usa
`/suitee-logo.png` (claro) ou `/suitee-logo-white.png` (rodapé/tema escuro).
Nome e tagline vazios também caem para `Suitee` e `Sua plataforma imobiliária`.
As configurações da empresa são a fonte do painel; a vitrine guarda um snapshot
no Payload e pode ser sincronizada por "Usar dados da empresa".

### Contrato de slug

**Vitrine (`sites.sites.slug`)**

- O slug é a marca do host e é **único globalmente** (`uq_sites_sites_slug`).
- Se o usuário informar um slug, ele é normalizado apenas para minúsculas no
  painel; o servidor valida o valor final. Se ficar vazio, o servidor deriva de
  `name` com `Slugify` (acentos viram ASCII, símbolos viram hífen).
- Formato: 3–48 caracteres, `[a-z0-9-]`, sem hífen nas pontas nem consecutivo,
  com pelo menos uma letra e fora da lista de marcas reservadas.
- Nome numérico/simbólico gera `imobiliaria-{tenant}` em vez de um host só de
  números. Conflito em slug derivado recebe sufixo `-2`, `-3`…; conflito em slug
  explícito retorna **409** para o usuário escolher outro.
- O banco repete as regras em `sites_slug_semantic` e `sites_slug_not_reserved`;
  a corrida entre dois saves é resolvida pela constraint única e vira 409.

**Imóvel (`sites.listings.seo_slug`)**

- É único por imobiliária (`uq_sites_listings_tenant_slug`) e forma o path
  `/imoveis/{seo_slug}`.
- Derivado do título com `Slugify`; títulos vazios, numéricos ou sem letras usam
  `imovel-{id-curto}` como fallback determinístico.
- Limite de 80 caracteres, incluindo o sufixo de colisão (`-2`, `-3`…).
- O slug é mantido estável quando o título muda para preservar o canonical.
- Se dois imóveis escolherem o mesmo slug entre o SELECT e o INSERT, o índice
  único vence e o sync recalcula o sufixo e regrava a projeção (até 5 tentativas).
- `sites_listings_seo_slug_format` garante no banco o formato
  `^[a-z0-9]+(-[a-z0-9]+)*$`.

Erros de slug inválido retornam 400; slug já usado retorna 409 com mensagem
acionável no painel.

A identidade visual também é validada antes da projeção: cores precisam ser
`#RRGGBB`, o tema é `light|dark` e a tagline tem até 160 caracteres. Campos de
cor omitidos preservam o valor publicado; um branding inválido retorna 400 com
`branding.primary_color`/`branding.accent_color`, em vez de propagar o 400 do
Payload como 500.

- Único, estável, derivado do nome comercial (não do UUID).
- `a-z0-9` + hífen; 3–48 caracteres; sem hífen nas pontas ou duplicado.
- Reservados: `www`, `app`, `api`, `admin`, `portal`, `sites`, `mail`, `cdn`,
  `imoveis`, `suitee`.
- **Toda** resolução pública de tenant vem do `Host` (ou de `X-Site-Host`
  encaminhado pelo Next). Nunca de querystring, body ou cookie.
- Host inválido, site inexistente ou site desligado → **404** indistinguível.
- `*.sites.suitee.com.br` não existe: `sites` é nome interno de código/schema.

`PUBLIC_SITE_HOST_SUFFIX` é configurado **uma vez** no servidor. O Go monta
`public_base_url` e o frontend apenas o consome — nada de concatenar sufixo no
browser.

## Regra de publicação

Um imóvel entra na vitrine **somente** quando:

```
site.enabled = true
AND property.status = 'published'
AND property.publication_status = 'published'
```

Qualquer outro caso despublica a projeção (o registro local permanece para
métricas e para o SEO não apontar para uma URL morta). A regra vive em
`site.PropertySnapshot.IsPublishableOnSite` e em `services.SyncService`.

Publicar/despublicar **imóvel** continua sendo `PATCH /api/v1/properties/:id`
(`properties:write`) no toca-core. O site reage ao evento `property.updated`;
não existe endpoint de "publicar no site".

## Eventos

Consome (filas duráveis e dedicadas):

| Fila | Evento | Ação |
| --- | --- | --- |
| `toca-sites.property.updated` | `property.updated` | relê o imóvel no core e reconcilia a projeção |
| `toca-sites.property.captured` | `property.captured` | idem (primeiro anúncio do imóvel captado) |

O handler é idempotente. `property.updated` **não** carrega preço/status
completos: o serviço relê `GET /api/v1/core/internal/properties/:id` com
`X-Internal-Key` + `tenant_id`.

**Backfill no primeiro enable**: eventos só refletem mudanças futuras. Quando a
vitrine é ligada, o Go pagina o estoque existente em
`GET /api/v1/core/internal/properties?tenant_id=&limit=&offset=` (rota interna
do `toca-core`, criada para isso) e reconcilia cada imóvel em background —
idempotente, com orçamento de 15 minutos. Desligar o site desliga também o
tenant no Payload (o Next resolve o site por lá); sem isso o host continuaria
no ar.

Publica (outbox `PublishReliable`, at-least-once; contratos em
`pkg/events/contracts/site_events.go`):

| Evento | Quando |
| --- | --- |
| `site.listing.published` | transição para publicado na vitrine |
| `site.listing.unpublished` | saída da vitrine (despublicado, site desligado, imóvel removido) |
| `site.lead.received` | lead do formulário gravado e criado no CRM |

O `toca-comm` consome `site.lead.received` e `site.listing.published`/`unpublished`
(filas `toca-comm.site.*`) para broadcast via WebSocket no painel. Sem essas
filas o exchange não tem rota e o outbox do toca-sites fica em `NO_ROUTE`.

Pageviews **não** vão para o barramento: são ruído. Ficam em
`sites.listing_views` e alimentam o card "Performance Web" e o painel `/site`.

## HTTP

Autenticado (`Authorization: Bearer`, JWT Supabase; `sites:read` /
`sites:write`; papel `cliente` rejeitado):

| Método | Rota | Permissão |
| --- | --- | --- |
| GET | `/api/v1/sites/me` | `sites:read` |
| PUT | `/api/v1/sites/me` | `sites:write` |
| GET | `/api/v1/sites/pages` | `sites:read` |
| PUT | `/api/v1/sites/pages/:key` (`home`/`about`/`contact`) | `sites:write` |
| GET | `/api/v1/sites/listings/:propertyId/url` | `properties:read` (URL pública do anúncio) |
| GET | `/api/v1/sites/listings/:propertyId/stats` | `sites:read` |
| POST | `/api/v1/sites/listings/:propertyId/resync` | `sites:write` |
| GET | `/api/v1/sites/stats` | `sites:read` |
| POST | `/api/v1/sites/me/reconcile` | `sites:write` (reprojeta o estoque em background) |
| GET | `/api/v1/sites/leads` | `sites:read` |

Portal do cliente (JWT; sem permissão de gestão):

| Método | Rota | Uso |
| --- | --- | --- |
| GET | `/api/v1/sites/portal/listing-url?property_id=...&tenant_id=...` | CTA "Ver anúncio no site" só quando publicado na própria vitrine |
| GET | `/api/v1/sites/portal/site?tenant_id=...` | Cartão "Site da imobiliária" no deal do portal (nome, URL e branding) |

Público (sem JWT; tenant pelo Host; rate-limit por Host+IP; body ≤ 64 KiB):

| Método | Rota | Corpo |
| --- | --- | --- |
| POST | `/api/v1/sites/public/views` | `{property_id, path?, referrer?, session_id?, utm_*}` |
| POST | `/api/v1/sites/public/leads` | `{name, email?, phone?, message?, property_id?, accepted_contact, website?}` |

`accepted_contact` é obrigatório (LGPD). `website` é honeypot: preenchido →
`202` e nada é gravado. Host efetivo: `X-Site-Host` (Next SSR) > `X-Forwarded-Host`
> `Host`.

O gateway expõe o subtree em `/api/v1/sites*` (`SITES_SERVICE_URL`) e aceita a
origem `https://*.imoveis.suitee.com.br` no CORS — o matcher só aceita
subdomínios do sufixo configurado, nunca um wildcard global.

## Persistência

Schema `sites.*` (migration `supabase/migrations/20261026120000_create_sites_schema.sql`):

- `sites.sites` — tenant, slug, nome, enabled, `public_base_url`, branding,
  `contact` (e-mail, telefone, WhatsApp, endereço, cidade/UF e CRECI),
  `payload_tenant_id`.
- `sites.pages` — 3 páginas por site, `body` editorial em JSONB.
- `sites.listings` — projeção do imóvel (`property_id`, slug SEO, status,
  preço, endereço, features, mídia, `payload_doc_id`).
- `sites.listing_views` — pageviews (hash de sessão, UTM, país).
- `sites.leads` — espelho do lead (`received` → `processed`/`failed`/`duplicate`).

Todas as tabelas têm `tenant_id uuid` + **RLS** `tenant_isolation` no mesmo
arquivo (defesa em profundidade para leituras via Supabase REST). O serviço usa
o pool de aplicação (BYPASSRLS) e filtra `tenant_id` explicitamente em toda
query; tenant vazio **falha fechado** antes de tocar no banco. O contrato é
coberto por `internal/repository/postgres/tenant_scope_test.go`.

## Configuração

| Variável | Default | Descrição |
| --- | --- | --- |
| `PORT` | `8094` | porta HTTP |
| `DATABASE_URI` / `DATABASE_URI_SERVICE` | — | Postgres (mesmo banco da plataforma) |
| `SUPABASE_URL` | — | necessário para JWKS |
| `INTERNAL_API_KEY` | — | chamadas internas core/CRM/Payload |
| `RABBITMQ_URI` | vazio | sem broker o serviço sobe, mas não sincroniza |
| `CORE_SERVICE_URL` | `http://toca-core:8082` | leitura interna do imóvel |
| `CRM_SERVICE_URL` | `http://toca-crm:8084` | criação de lead |
| `PAYLOAD_API_URL` | vazio | `http://toca-web:3000`; vazio = projeção no CMS desligada |
| `PUBLIC_SITE_HOST_SUFFIX` | `imoveis.suitee.com.br` | sufixo único do host |
| `SITE_PUBLIC_SCHEME` | `https` | `http` em local |
| `SITE_PUBLIC_PORT` | vazio | porta do Next nas URLs locais (ex.: `3002`); produção vazia |
| `SITES_PUBLIC_RATE_LIMIT_PER_MIN` | `120` | token bucket por Host+IP |
| `SITES_PUBLIC_CORS_ORIGIN` | — | origem extra aceita (ex.: app do painel) |

O lead criado no formulário carrega o snapshot do imóvel (tipo, localização,
preço, características e fotos) como `property_interest` + recomendação no CRM.
Assim o painel mostra o interesse e o portal do cliente exibe o imóvel em
"Sugestões do seu corretor" sem uma leitura cross-tenant no login.

O contato é um snapshot de Configurações da empresa no Toka. O painel envia
`contact` no `PUT /sites/me`; o toca-sites projeta em `tenants.contact` do
Payload. O site público lê esse grupo para rodapé, card da imobiliária, CRECI e
CTAs de WhatsApp — sem chamada cross-service no SSR.

## Rodando local

Atalho da stack completa (Toka + `toca-sites` + `toca-web`, com migrations):

```bash
make sites-check
make sites-up
make sites-smoke
make sites-url
```

Só o site público (sem painel/gateway):

```bash
make sites-public-up
```

Para rodar o binário diretamente (com banco/broker já de pé):

```bash
cd toca-sites
go test ./...
go run ./cmd/server
```

O `toca-web` depende de migrations versionadas do Payload: no Compose, o
serviço one-shot `toca-web-migrate` roda `payload migrate` antes do Next subir.
Sem `PAYLOAD_API_URL` o serviço funciona (cadastro de site, sync local, views e
leads); a renderização pública fica a cargo do Next quando o Payload estiver de
pé. O sync exige RabbitMQ para reagir a eventos; o painel tem
`POST .../resync` como recuperação manual.

## Observabilidade / operação

- `/health`, `/ready`, `/health/deep`, `/metrics` (Gin/OTel).
- Fila em DLQ após 5 tentativas (`events.dlq_events`); mensagens com id inválido
  são descartadas para não envenenar a fila.
- Reconcile manual: `POST /api/v1/sites/listings/:propertyId/resync`.
- Não há scheduler: a fonte de mudança é o evento. Se o barramento estiver
  indisponível por muito tempo, o resync em massa deve ser feito por script
  (replay do outbox/consulta ao core), não por polling silencioso.

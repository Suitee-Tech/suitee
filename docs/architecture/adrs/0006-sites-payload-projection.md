# ADR 0006 — Site público das imobiliárias: Payload como projeção, Go como orquestração

- **Status**: Accepted
- **Data**: 2026-10-26
- **Owners**: Plataforma
- **Plano**: docs/architecture/plans/sites.md
- **Serviço Go**: [docs/architecture/services/toca-sites/README.md](../services/toca-sites/README.md)

## Contexto

A imobiliária precisa de uma vitrine pública dos seus anúncios com endereço
próprio, SEO e identidade visual — sem que o comprador veja um host de produto
interno (`*.sites.suitee.com.br`) nem a marca “suitee”. Ao mesmo tempo:

- o **estoque** já vive em `toca-core` (`core.properties`), com `status`
  (`published`, `sold`, `prospecting`, …) e `publication_status`
  (`draft`/`published`) definindo o que é anúncio público;
- o **painel** e o **portal** já consomem APIs autenticadas por JWT, com
  isolamento por `tenant_id` e RBAC (`pkg/auth`);
- a **comunicação/CRM** já reage a eventos no RabbitMQ, e o toca-portal já tem o
  padrão “grava lead local → cria no CRM → publica evento”.

Duplicar o imóvel no CMS (source of truth paralelo) criaria divergência de
preço/status e dois lugares para despublicar. Servir o site por um CMS que
também é o painel da imobiliária repetiria o acoplamento que o RBAC atual
evita.

## Decisão

1. **Duas peças, responsabilidades separadas**
   - `toca-sites` (Go, `:8094`): dono do registro da vitrine, projeção local das
     listings, ingestão de views/leads, métricas e eventos.
   - `toca-web` (Payload 3 + Next.js App Router): render SSR/SEO do site
     público. **Nunca** é source of truth.
2. **Payload é projeção**. O Go lê o imóvel no `toca-core` a cada evento e
   escreve a listing no Payload (API interna). Admin do Payload fica restrito a
   super-admin/rede interna; a imobiliária opera no Toka (`/site`).
3. **Host comercial, não host de produto**: `{marca}.imoveis.suitee.com.br`
   (prod) e `{marca}.imoveis.localhost` (local). `{marca}` é
   `sites.sites.slug` — único, estável, derivado do nome comercial, validado e
   com reservados (`www`, `app`, `api`, `admin`, `portal`, `sites`, `mail`,
   `cdn`, `imoveis`, `suitee`). `sites` permanece apenas no código interno
   (`toca-sites`, schema `sites.*`, `/api/v1/sites`).
4. **Tenant resolvido só pelo Host** no site público — nunca querystring, body
   ou cookie. Host inválido, site desligado ou inexistente → 404
   indistinguível. Uma única variável (`PUBLIC_SITE_HOST_SUFFIX`) alimenta o Go,
   que devolve `public_base_url` montada; o frontend não concatena sufixo.
5. **Regra única de publicação**: `site.enabled AND status=published AND
   publication_status=published`. Qualquer outra combinação despublica a
   projeção. Publicar imóvel continua sendo `PATCH /properties/:id`
   (`properties:write`) — não há endpoint “publicar no site”.
   No **primeiro enable** o Go faz backfill do estoque existente pela rota
   interna paginada `GET /api/v1/core/internal/properties` (em background,
   idempotente): eventos cobrem mudanças futuras, não o portfólio já cadastrado.
6. **Contato projetado**: o `contact` do site é um snapshot de Configurações
   da empresa (e-mail, telefone, WhatsApp, endereço e CRECI), enviado pelo
   painel no save e projetado no grupo `tenants.contact` do Payload. O SSR não
   consulta o core para renderizar rodapé/CRECI.
7. **Eventos no outbox**: `property.updated`/`property.captured` são consumidos
   em filas dedicadas (`toca-sites.*`); `site.listing.published`,
   `site.listing.unpublished` e `site.lead.received` são publicados com
   `PublishReliable` e consumidos pelo `toca-comm` (filas `toca-comm.site.*`)
   para broadcast WebSocket no painel. Pageviews **não** entram no bus (ruído):
   ficam em `sites.listing_views`.
8. **Isolamento**: schema `sites.*` com `tenant_id uuid` e RLS
   `tenant_isolation` em todas as tabelas; o Go filtra `tenant_id`
   explicitamente e falha fechado sem tenant. Escrita no Payload usa
   `X-Internal-Key` + `X-Tenant-Id`. A única leitura cross-tenant é a resolução
   pública por slug (devolve apenas site habilitado) e o CTA do portal, que
   devolve **somente a URL pública de anúncio já publicado**.
9. **Permissões novas** `sites:read`/`sites:write` (catálogo Go gera o TS). O
   portal do cliente usa o papel `cliente` e não recebe permissão de gestão.

## Alternativas consideradas

1. **Payload como source of truth do imóvel**
   - Prós: painel/editor únicos.
   - Cons: dois estoques, divergência de preço/status, RLS/RBAC dobrados,
     despublicação em dois lugares. Rejeitada.
2. **Next.js lendo direto do `toca-core`**
   - Prós: menos uma peça.
   - Cons: daria ao Next a credencial interna do core, duplicaria a regra de
     publicação e misturaria render com orquestração; sem lugar natural para
     views/leads. Rejeitada.
3. **Domínio próprio da imobiliária (`www.imobiliariacariri.com.br` via CNAME)**
   - Prós: melhor URL comercial.
   - Cons: certificado/ACME por host arbitrário, verificação de domínio e
     operação de DNS por tenant. Fica para a fase 2; o host
     `*.imoveis.suitee.com.br` é o fallback de produto já aceitável.
4. **Admin do Payload como painel da imobiliária**
   - Prós: menos UI a construir.
   - Cons: expõe a ferramenta de CMS, ignora RBAC/permissões do Toka e cria
     segundo caminho de publicação. Rejeitada.

## Consequências

### Aceitas

- Uma única fonte de estoque; despublicar no core reflete no site por evento.
- Site público isolado do painel: o CMS não vira porta de autenticação da
  imobiliária.
- SEO canônico por host, com `seller`/`og:site_name` da imobiliária e sem a
  marca Suitee na página.
- Métricas de vitrine (views/leads 7d/30d) reais no card “Performance Web”.
- Reuso do desenho de lead do toca-portal (lead local sobrevive a CRM fora do
  ar).

### Pagas

- Duas peças novas para operar (Go + Node) e uma migration de schema.
- Consistência eventual: o site depende do evento; `POST .../resync` cobre
  recuperação manual.
- Next/Payload exige passo de migração próprio (`payload migrate`) além das
  migrations Supabase. O Compose de produção sobe o serviço one-shot
  `toca-web-migrate` (imagem `toca-web-migrator`, target `migrator` do
  Dockerfile) antes do `toca-web`; a imagem de runtime é Next standalone e não
  carrega o CLI/drizzle-kit. `PAYLOAD_DB_PUSH` fica **false** em produção —
  push é apenas bootstrap de dev e pode pedir confirmação interativa.
- TLS curinga `*.imoveis.suitee.com.br` exige ACME **DNS-01**; sem isso, o
  bootstrap é host literal por imobiliária. O roteador Traefik `HostRegexp` é o
  alvo final.

## Operação

- Filas: `toca-sites.property.updated` e `toca-sites.property.captured`,
  duráveis, com DLQ após 5 tentativas. Evento com id inválido é descartado
  (poison message).
- Reconcile: `POST /api/v1/sites/listings/:propertyId/resync` (`sites:write`).
- Backfill do estoque no primeiro enable (background, 15 min de orçamento); cada
  listing é publicada/despublicada segundo a regra única. Falha fica no log do
  serviço e pode ser refeita ligando/desligando ou pelo resync manual.
- Health: `/health`, `/ready`, `/health/deep`; métricas em `/metrics`.
- Revalidação ISR: o Go chama `POST /api/revalidate` no `toca-web` após
  publicar/despublicar/página (best-effort — o ISR expira sozinho).

## Fora de escopo (fase 2)

- Domínio próprio por CNAME e ACME por host arbitrário.
- Site builder/blog/blocos livres.
- Admin do Payload para a imobiliária.
- Indexação do Toka (`app.example.com`) — continua `noindex`; só a zona
  `imoveis` é indexável.

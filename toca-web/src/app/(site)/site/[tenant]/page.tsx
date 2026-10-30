import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { CollectionNav } from '@/components/site/CollectionNav'
import { CollectionSection } from '@/components/site/CollectionSection'
import { PlainText } from '@/components/site/PlainText'
import { RichText } from '@/components/site/RichText'
import { ArrowIcon, HandshakeIcon, HomeIcon, ShieldIcon } from '@/components/site/icons'
import { getCollectionSummaries, getPublishedPage } from '@/lib/data'
import { tenantCoverUrl, tenantDisplayName, tenantTagline } from '@/lib/format'
import { buildPageMetadata } from '@/lib/seo'
import { getSiteContext } from '@/lib/site'

export const dynamic = 'force-dynamic'

export async function generateMetadata(): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  const page = await getPublishedPage(context, 'home')
  const metadata = buildPageMetadata({
    context,
    page,
    pathname: '/',
    fallbackTitle: 'Imóveis',
    fallbackDescription: `Encontre casas, apartamentos e terrenos com ${tenantDisplayName(context.tenant)}.`,
  })
  const customTitle = page?.seo?.metaTitle?.trim()
  const pageTitle = customTitle || page?.title?.trim() || 'Imóveis'
  const absoluteTitle = customTitle
    ? customTitle
    : pageTitle === context.tenant.name
      ? `${pageTitle} | Imóveis`
      : `${pageTitle} | ${context.tenant.name}`

  // The home page shares the layout segment, so Next does not apply the
  // layout's title template to it. Compose the full title explicitly.
  return {
    ...metadata,
    title: { absolute: absoluteTitle },
  }
}

export default async function HomePage() {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const [page, summaries] = await Promise.all([
    getPublishedPage(context, 'home'),
    getCollectionSummaries(context),
  ])
  const availableCollections = summaries.filter((summary) => summary.totalDocs > 0)
  const cover = tenantCoverUrl(context.tenant, context.origin)
  const name = tenantDisplayName(context.tenant)
  const homePageTitle = page?.title?.trim()
  // Alguns tenants antigos salvaram o nome da imobiliária como título da home;
  // isso transforma o hero em uma repetição do header. Nesse caso voltamos ao
  // headline editorial padrão.
  const title =
    homePageTitle && homePageTitle !== name ? homePageTitle : 'Encontre o imóvel certo para você'
  const subtitle =
    page?.subtitle?.trim() ||
    `${name} reúne imóveis selecionados para comprar ou alugar com atendimento próximo do primeiro contato à assinatura.`

  return (
    <>
      <section className={cover ? 'home-hero home-hero--image' : 'home-hero'}>
        {cover ? (
          <div className="home-hero__media" aria-hidden="true">
            <img src={cover} alt="" fetchPriority="high" />
          </div>
        ) : null}
        <div className="site-container home-hero__content">
          <p className="eyebrow">{tenantTagline(context.tenant)}</p>
          <h1>{title}</h1>
          <p className="home-hero__lead">{subtitle}</p>
          {page?.hero ? (
            <RichText data={page.hero} className="home-hero__richtext" />
          ) : null}

          <form className="quick-search" action="/imoveis" method="get">
            <label>
              <span>Quero</span>
              <select name="listingType" defaultValue="">
                <option value="">Comprar ou alugar</option>
                <option value="sale">Comprar</option>
                <option value="rent">Alugar</option>
              </select>
            </label>
            <label>
              <span>Onde</span>
              <input type="text" name="city" placeholder="Cidade ou bairro" />
            </label>
            <button type="submit" className="button button--accent">
              Buscar imóveis
              <ArrowIcon width={18} height={18} />
            </button>
          </form>

          <div className="home-hero__links">
            <Link href="/imoveis">Ver todos os imóveis</Link>
            <Link href="/contato">Falar com a imobiliária</Link>
          </div>
        </div>
      </section>

      {availableCollections.length > 0 ? (
        <>
          <div className="site-container home-collection-nav">
            <CollectionNav summaries={summaries} />
          </div>
          {availableCollections.map((summary, index) => (
            <CollectionSection
              key={summary.collection.slug}
              summary={summary}
              index={index}
              tenant={context.slug}
            />
          ))}
        </>
      ) : (
        <section className="section">
          <div className="site-container">
            <div className="empty-state">
              <HomeIcon width={28} height={28} />
              <h3>Nenhum imóvel publicado ainda</h3>
              <p>
                Em breve novos imóveis estarão disponíveis. Fale com a imobiliária para saber das
                oportunidades antes de todo mundo.
              </p>
              <Link href="/contato" className="button button--primary">
                Falar com a imobiliária
              </Link>
            </div>
          </div>
        </section>
      )}

      <section className="section section--soft">
        <div className="site-container">
          <div className="values-grid">
            <article className="value-card">
              <span className="value-card__icon">
                <HomeIcon width={22} height={22} />
              </span>
              <h3>Curadoria de verdade</h3>
              <p>
                Cada anúncio é acompanhado por quem conhece a região, o condomínio e o imóvel de
                perto.
              </p>
            </article>
            <article className="value-card">
              <span className="value-card__icon">
                <HandshakeIcon width={22} height={22} />
              </span>
              <h3>Atendimento próximo</h3>
              <p>
                Converse direto com a equipe de {name}, sem intermediários e sem robôs.
              </p>
            </article>
            <article className="value-card">
              <span className="value-card__icon">
                <ShieldIcon width={22} height={22} />
              </span>
              <h3>Decisão com segurança</h3>
              <p>
                Informações claras sobre valores, condomínio e documentação para você negociar com
                tranquilidade.
              </p>
            </article>
          </div>
        </div>
      </section>

      {page?.body || page?.bodyRich ? (
        <section className="section">
          <div className="site-container home-story">
            <div className="home-story__heading">
              <p className="eyebrow">Sobre a imobiliária</p>
              <h2>Pessoas que conhecem cada endereço</h2>
              <Link href="/sobre" className="text-link">
                Conheça nossa história
                <ArrowIcon width={18} height={18} />
              </Link>
            </div>
            <div className="home-story__body">
              {page.body ? <PlainText text={page.body} /> : <RichText data={page.bodyRich} />}
            </div>
          </div>
        </section>
      ) : null}

      <section className="site-container">
        <div className="cta-band">
          <div>
            <p className="eyebrow">Vamos conversar</p>
            <h2>Encontre o seu próximo endereço com a {name}</h2>
            <p>Conte o que procura e receba uma seleção de imóveis feita para você.</p>
          </div>
          <Link href="/contato" className="button button--accent">
            Falar com a imobiliária
            <ArrowIcon width={18} height={18} />
          </Link>
        </div>
      </section>
    </>
  )
}

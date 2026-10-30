import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { PlainText } from '@/components/site/PlainText'
import { RichText } from '@/components/site/RichText'
import { ArrowIcon, HandshakeIcon, HomeIcon, ShieldIcon } from '@/components/site/icons'
import { getPublishedPage } from '@/lib/data'
import { tenantCoverUrl } from '@/lib/format'
import { buildPageMetadata } from '@/lib/seo'
import { getSiteContext } from '@/lib/site'

export const dynamic = 'force-dynamic'

export async function generateMetadata(): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  const page = await getPublishedPage(context, 'about')
  return buildPageMetadata({
    context,
    page,
    pathname: '/sobre',
    fallbackTitle: 'Sobre nós',
    fallbackDescription: `Conheça a história, os valores e a equipe de ${context.tenant.name}.`,
  })
}

export default async function AboutPage() {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const page = await getPublishedPage(context, 'about')
  const cover = tenantCoverUrl(context.tenant, context.origin)
  const title = page?.title?.trim() || `Sobre a ${context.tenant.name}`
  const subtitle =
    page?.subtitle?.trim() ||
    'Uma imobiliária próxima, feita de pessoas que conhecem a cidade e se importam com cada mudança.'
  const hasBody = Boolean(page?.body || page?.bodyRich)

  return (
    <>
      <section className={cover ? 'page-hero page-hero--image' : 'page-hero'}>
        {cover ? (
          <div className="page-hero__media" aria-hidden="true">
            <img src={cover} alt="" fetchPriority="high" />
          </div>
        ) : null}
        <div className="site-container page-hero__content">
          <nav className="breadcrumbs" aria-label="Navegação">
            <Link href="/">Início</Link>
            <span>/</span>
            <span aria-current="page">Sobre</span>
          </nav>
          <p className="eyebrow">Quem somos</p>
          <h1>{title}</h1>
          <p className="page-hero__lead">{subtitle}</p>
          {page?.hero ? <RichText data={page.hero} className="page-hero__richtext" /> : null}
        </div>
      </section>

      <section className="section">
        <div className="site-container about-layout">
          <aside className="about-aside">
            <p className="eyebrow">Nossa essência</p>
            <h2>Negócios imobiliários com gente de verdade.</h2>
            <p>
              Cada imóvel tem uma história, e cada cliente tem um plano. Nosso papel é aproximar
              os dois com transparência, escuta e conhecimento local.
            </p>
            <Link href="/imoveis" className="button button--primary">
              Ver imóveis disponíveis
              <ArrowIcon width={18} height={18} />
            </Link>
          </aside>

          <div className="about-body">
            {hasBody ? (
              page?.body ? (
                <PlainText text={page.body} className="about-body__text" />
              ) : (
                <RichText data={page?.bodyRich} />
              )
            ) : (
              <div className="about-body__text">
                <p>
                  A {context.tenant.name} trabalha para tornar a experiência de comprar, vender ou
                  alugar um imóvel mais simples e mais humana. Conhecemos ruas, bairros, condomínios
                  e as histórias que fazem cada lugar único.
                </p>
                <p>
                  Nosso atendimento é próximo do começo ao fim: entendemos o que você procura,
                  apresentamos opções coerentes e acompanhamos a negociação e a documentação com
                  clareza.
                </p>
              </div>
            )}

            <div className="about-values">
              <article>
                <HomeIcon width={22} height={22} />
                <div>
                  <h3>Conhecimento local</h3>
                  <p>Uma leitura honesta da cidade, dos bairros e das oportunidades de cada região.</p>
                </div>
              </article>
              <article>
                <HandshakeIcon width={22} height={22} />
                <div>
                  <h3>Relação de confiança</h3>
                  <p>Atendimento consultivo, sem pressa e sem pressão para fechar negócio.</p>
                </div>
              </article>
              <article>
                <ShieldIcon width={22} height={22} />
                <div>
                  <h3>Segurança na jornada</h3>
                  <p>Informação organizada e acompanhamento cuidadoso em cada etapa.</p>
                </div>
              </article>
            </div>

            <div className="inline-cta">
              <div>
                <h2>Quer conhecer nossos imóveis?</h2>
                <p>Veja a seleção atual ou fale diretamente com a nossa equipe.</p>
              </div>
              <div className="inline-cta__actions">
                <Link href="/imoveis" className="button button--primary">
                  Ver imóveis
                </Link>
                <Link href="/contato" className="button button--ghost">
                  Entrar em contato
                </Link>
              </div>
            </div>
          </div>
        </div>
      </section>
    </>
  )
}

import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { ActiveFilters } from '@/components/site/ActiveFilters'
import { CollectionNav } from '@/components/site/CollectionNav'
import { ListingCard } from '@/components/site/ListingCard'
import { Pagination } from '@/components/site/Pagination'
import { SearchFilters } from '@/components/site/SearchFilters'
import { HomeIcon, PinIcon } from '@/components/site/icons'
import { getCollectionSummaries, parseListingFilters, searchListings } from '@/lib/data'
import { buildPageMetadata } from '@/lib/seo'
import { getSiteContext } from '@/lib/site'

export const dynamic = 'force-dynamic'

type SearchParams = Record<string, string | string[] | undefined>

export async function generateMetadata(): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  return buildPageMetadata({
    context,
    page: null,
    pathname: '/imoveis',
    fallbackTitle: 'Imóveis',
    fallbackDescription: `Casas, apartamentos e terrenos para comprar ou alugar com ${context.tenant.name}.`,
  })
}

export default async function ListingsPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>
}) {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const rawSearchParams = await searchParams
  const filters = parseListingFilters(rawSearchParams)
  const [results, summaries] = await Promise.all([
    searchListings(context, filters),
    getCollectionSummaries(context),
  ])
  const hasFilters = Boolean(
    filters.listingType ||
      filters.propertyType ||
      filters.city ||
      filters.q ||
      filters.minPrice !== undefined ||
      filters.maxPrice !== undefined,
  )

  return (
    <>
      <section className="listings-hero">
        <div className="site-container listings-hero__inner">
          <nav className="breadcrumbs" aria-label="Navegação">
            <Link href="/">Início</Link>
            <span>/</span>
            <span aria-current="page">Imóveis</span>
          </nav>
          <div className="listings-hero__headline">
            <div>
              <p className="eyebrow">Vitrine de imóveis</p>
              <h1>Encontre o seu próximo imóvel</h1>
              <p>
                Filtre por tipo, cidade e faixa de preço. Todos os anúncios são acompanhados pela
                equipe de {context.tenant.name}.
              </p>
            </div>
            {context.tenant.contact?.city ? (
              <span className="listings-hero__city">
                <PinIcon width={16} height={16} />
                {context.tenant.contact.city}
              </span>
            ) : null}
          </div>
        </div>
      </section>

      <section className="section section--listings">
        <div className="site-container">
          <CollectionNav summaries={summaries} allActive />
          <SearchFilters filters={filters} />
          <ActiveFilters filters={filters} />

          <div className="results-heading">
            <h2>
              {results.totalDocs === 0
                ? 'Nenhum imóvel encontrado'
                : results.totalDocs === 1
                  ? '1 imóvel encontrado'
                  : `${results.totalDocs} imóveis encontrados`}
            </h2>

          </div>

          {results.docs.length > 0 ? (
            <>
              <div className="listing-grid">
                {results.docs.map((listing) => (
                  <ListingCard key={listing.id} listing={listing} tenant={context.slug} />
                ))}
              </div>
              <Pagination
                currentPage={results.page}
                totalPages={results.totalPages}
                filters={filters}
              />
            </>
          ) : (
            <div className="empty-state">
              <HomeIcon width={28} height={28} />
              <h3>
                {hasFilters
                  ? 'Nenhum imóvel combina com esses filtros'
                  : 'Ainda não há imóveis publicados'}
              </h3>
              <p>
                {hasFilters
                  ? 'Tente ampliar a faixa de preço, remover o tipo de imóvel ou buscar outra cidade.'
                  : 'Em breve novos imóveis estarão disponíveis. Fale com a imobiliária para receber novidades.'}
              </p>
              {hasFilters ? (
                <Link href="/imoveis" className="button button--primary">
                  Ver todos os imóveis
                </Link>
              ) : (
                <Link href="/contato" className="button button--primary">
                  Falar com a imobiliária
                </Link>
              )}
            </div>
          )}
        </div>
      </section>
    </>
  )
}

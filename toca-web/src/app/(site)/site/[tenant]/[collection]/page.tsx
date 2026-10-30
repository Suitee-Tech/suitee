import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { ActiveFilters } from '@/components/site/ActiveFilters'
import { CollectionNav } from '@/components/site/CollectionNav'
import { ListingCard } from '@/components/site/ListingCard'
import { Pagination } from '@/components/site/Pagination'
import { SearchFilters } from '@/components/site/SearchFilters'
import { HomeIcon, PinIcon } from '@/components/site/icons'
import { getListingCollection } from '@/lib/collections'
import {
  getCollectionSummaries,
  parseListingFilters,
  searchListings,
  type ListingFilters,
} from '@/lib/data'
import { tenantDisplayName } from '@/lib/format'
import { buildPageMetadata } from '@/lib/seo'
import { getSiteContext, tenantParamMatches } from '@/lib/site'

export const dynamic = 'force-dynamic'

type SearchParams = Record<string, string | string[] | undefined>
type PageProps = {
  params: Promise<{ tenant: string; collection: string }>
  searchParams: Promise<SearchParams>
}

async function resolveCollectionPage(params: PageProps['params']) {
  const context = await getSiteContext()
  const { tenant, collection: slug } = await params
  if (!context || !tenantParamMatches(context, tenant)) return null
  const collection = getListingCollection(slug)
  if (!collection) return null
  return { context, collection }
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const resolved = await resolveCollectionPage(params)
  if (!resolved) return { robots: { index: false, follow: false } }
  const { context, collection } = resolved
  const summaries = await getCollectionSummaries(context)
  const total = summaries.find((summary) => summary.collection.slug === collection.slug)?.totalDocs ?? 0
  const metadata = buildPageMetadata({
    context,
    page: null,
    pathname: `/${collection.slug}`,
    fallbackTitle: collection.title,
    fallbackDescription: `${collection.description} Atendimento com ${tenantDisplayName(context.tenant)}.`,
  })
  // Coleção sem estoque é uma página de aterrissagem vazia: mantém a navegação,
  // mas não a entrega ao índice como conteúdo fino.
  return total > 0 ? metadata : { ...metadata, robots: { index: false, follow: true } }
}

export default async function CollectionPage({ params, searchParams }: PageProps) {
  const resolved = await resolveCollectionPage(params)
  if (!resolved) notFound()
  const { context, collection } = resolved

  const rawSearchParams = await searchParams
  const parsed = parseListingFilters(rawSearchParams)
  const filters: ListingFilters = {
    ...parsed,
    listingType: collection.listingType ?? parsed.listingType,
    propertyType: collection.propertyType ?? parsed.propertyType,
  }

  const [results, summaries] = await Promise.all([
    searchListings(context, filters),
    getCollectionSummaries(context),
  ])

  const hasFilters = Boolean(
    filters.q ||
      filters.city ||
      filters.minPrice !== undefined ||
      filters.maxPrice !== undefined ||
      (collection.listingType ? filters.propertyType : filters.listingType),
  )
  const resultLabel =
    results.totalDocs === 0
      ? 'Nenhum imóvel encontrado'
      : results.totalDocs === 1
        ? '1 imóvel encontrado'
        : `${results.totalDocs} imóveis encontrados`

  return (
    <>
      <section className="listings-hero collection-hero">
        <div className="site-container listings-hero__inner">
          <nav className="breadcrumbs" aria-label="Navegação">
            <Link href="/">Início</Link>
            <span>/</span>
            <Link href="/imoveis">Imóveis</Link>
            <span>/</span>
            <span aria-current="page">{collection.label}</span>
          </nav>
          <div className="listings-hero__headline">
            <div>
              <p className="eyebrow">{collection.eyebrow}</p>
              <h1>{collection.title}</h1>
              <p>
                {collection.description} Todos os anúncios são acompanhados pela equipe de{' '}
                {tenantDisplayName(context.tenant)}.
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
          <CollectionNav summaries={summaries} currentSlug={collection.slug} />
          <SearchFilters
            filters={filters}
            basePath={`/${collection.slug}`}
            lockedListingType={Boolean(collection.listingType)}
            lockedPropertyType={Boolean(collection.propertyType)}
          />
          <ActiveFilters
            filters={filters}
            basePath={`/${collection.slug}`}
            lockedListingType={Boolean(collection.listingType)}
            lockedPropertyType={Boolean(collection.propertyType)}
          />

          <div className="results-heading">
            <h2>{resultLabel}</h2>
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
                basePath={`/${collection.slug}`}
              />
            </>
          ) : (
            <div className="empty-state">
              <HomeIcon width={28} height={28} />
              <h3>
                {hasFilters
                  ? 'Nenhum imóvel combina com esses filtros'
                  : 'Ainda não há imóveis nesta coleção'}
              </h3>
              <p>
                {hasFilters
                  ? 'Tente ampliar a faixa de preço, trocar a cidade ou remover os filtros.'
                  : 'Novas oportunidades desta coleção aparecem aqui assim que forem publicadas.'}
              </p>
              <Link href={hasFilters ? `/${collection.slug}` : '/imoveis'} className="button button--primary">
                {hasFilters ? 'Limpar filtros' : 'Ver todos os imóveis'}
              </Link>
            </div>
          )}
        </div>
      </section>
    </>
  )
}

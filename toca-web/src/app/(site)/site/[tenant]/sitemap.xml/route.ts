import { getCollectionSummaries, getSitemapListings } from '@/lib/data'
import { siteIndexingDisabled } from '@/lib/indexing'
import { listingPath } from '@/lib/site-host'
import { getSiteContext, tenantParamMatches } from '@/lib/site'

export const dynamic = 'force-dynamic'

type RouteContext = {
  params: Promise<{ tenant: string }>
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

function urlEntry(loc: string, lastmod?: string | null): string {
  return [
    '  <url>',
    `    <loc>${escapeXml(loc)}</loc>`,
    ...(lastmod ? [`    <lastmod>${escapeXml(lastmod)}</lastmod>`] : []),
    '  </url>',
  ].join('\n')
}

export async function GET(_request: Request, { params }: RouteContext): Promise<Response> {
  const context = await getSiteContext()
  const { tenant } = await params

  if (!context || !tenantParamMatches(context, tenant)) {
    return new Response('Not Found', { status: 404 })
  }

  // Beta não publica sitemap: o robots.txt já bloqueia todo o host.
  if (siteIndexingDisabled()) {
    return new Response('Not Found', { status: 404 })
  }

  const [listings, collections] = await Promise.all([
    getSitemapListings(context),
    getCollectionSummaries(context),
  ])
  const staticPaths = ['/', '/imoveis', '/sobre', '/contato'] as const
  const collectionPaths = collections
    .filter((summary) => summary.totalDocs > 0)
    .map((summary) => `/${summary.collection.slug}`)
  const entries = [
    ...staticPaths.map((path) => urlEntry(`${context.origin}${path}`)),
    ...collectionPaths.map((path) => urlEntry(`${context.origin}${path}`)),
    ...listings.map((listing) =>
      urlEntry(`${context.origin}${listingPath(listing.seoSlug)}`, listing.updatedAt),
    ),
  ]

  const xml = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ...entries,
    '</urlset>',
    '',
  ].join('\n')

  return new Response(xml, {
    status: 200,
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      'Cache-Control': 'public, max-age=0, s-maxage=300, stale-while-revalidate=600',
      Vary: 'Host',
    },
  })
}

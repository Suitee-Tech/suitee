import { siteIndexingDisabled } from '@/lib/indexing'
import { getSiteContext, tenantParamMatches } from '@/lib/site'

export const dynamic = 'force-dynamic'

type RouteContext = {
  params: Promise<{ tenant: string }>
}

export async function GET(_request: Request, { params }: RouteContext): Promise<Response> {
  const context = await getSiteContext()
  const { tenant } = await params

  if (!context || !tenantParamMatches(context, tenant)) {
    return new Response('Not Found', { status: 404 })
  }

  const body = siteIndexingDisabled()
    ? ['User-agent: *', 'Disallow: /', ''].join('\n')
    : [
        'User-agent: *',
        'Allow: /',
        'Disallow: /admin',
        'Disallow: /api/',
        'Disallow: /site/',
        '',
        `Sitemap: ${context.origin}/sitemap.xml`,
        '',
      ].join('\n')

  return new Response(body, {
    status: 200,
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
      'Cache-Control': 'public, max-age=0, s-maxage=300, stale-while-revalidate=600',
      Vary: 'Host',
    },
  })
}

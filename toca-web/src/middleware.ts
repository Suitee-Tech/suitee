import { NextResponse, type NextRequest } from 'next/server'
import { getSiteSlugFromHost, isInternalSitePath } from '@/lib/site-host'

/**
 * Tenant resolution for the public storefront.
 *
 * The visible URL never changes: `{marca}.imoveis.../imoveis/foo` is internally
 * rewritten to `/site/{marca}/imoveis/foo`. Host validation (shape, suffix,
 * reserved slug) happens before any database lookup. The tenant layout then
 * resolves the slug in Payload and returns 404 for unknown/disabled sites.
 *
 * Payload Admin, the Payload API, Next internals and uploads are excluded by
 * the matcher. They are not part of the tenant rewrite.
 */
function withNoindex(response: NextResponse): NextResponse {
  if (String(process.env.SITE_NOINDEX ?? '').trim().toLowerCase() === 'true') {
    response.headers.set('X-Robots-Tag', 'noindex, nofollow, noarchive')
  }
  return response
}

export function middleware(request: NextRequest): NextResponse {
  const { pathname } = request.nextUrl

  // Never expose the internal route shape as a public duplicate URL.
  if (isInternalSitePath(pathname)) {
    return withNoindex(new NextResponse('Not Found', { status: 404 }))
  }

  let slug: string | null = null
  try {
    slug = getSiteSlugFromHost(request.headers.get('host'))
  } catch {
    // A missing/invalid PUBLIC_SITE_HOST_SUFFIX is a deployment error. Do not
    // fall back to another domain or to a query parameter.
    return withNoindex(new NextResponse('Site unavailable', { status: 404 }))
  }

  // Apex/marketing hosts (and any non-tenant host) fall through to the root
  // catch-all, which renders a 404. This includes direct `localhost:3000`.
  if (!slug) {
    return withNoindex(NextResponse.next())
  }

  const url = request.nextUrl.clone()
  url.pathname = pathname === '/' ? `/site/${slug}` : `/site/${slug}${pathname}`

  return withNoindex(NextResponse.rewrite(url))
}

export const config = {
  matcher: [
    '/((?!api|admin|_next|media|favicon\\.ico|.*\\..*).*)',
    '/sitemap.xml',
    '/robots.txt',
  ],
}

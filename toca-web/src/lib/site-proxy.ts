import { getSitesInternalApiUrl } from './env'
import {
  getSiteSlugFromHost,
  hostnameFromHost,
  normalizeHostHeader,
  normalizeSiteHostSuffix,
} from './site-host'

type ProxyResource = 'views' | 'leads'

const MAX_BODY_BYTES = 32 * 1024
const ALLOWED_RESOURCES: ReadonlySet<ProxyResource> = new Set(['views', 'leads'])

function jsonResponse(status: number, body: Record<string, unknown>): Response {
  return Response.json(body, {
    status,
    headers: { 'Cache-Control': 'no-store' },
  })
}

/**
 * Same-origin fallback used only when NEXT_PUBLIC_SITES_API_URL is not
 * configured. It forwards exactly the two public Go endpoints and derives the
 * site host from the actual request Host, never from the JSON body.
 */
export async function proxyPublicSiteRequest(
  request: Request,
  resource: ProxyResource,
): Promise<Response> {
  if (!ALLOWED_RESOURCES.has(resource)) {
    return jsonResponse(404, { error: 'not_found' })
  }

  if (request.method !== 'POST') {
    return new Response(null, {
      status: 405,
      headers: { Allow: 'POST', 'Cache-Control': 'no-store' },
    })
  }

  const host = normalizeHostHeader(request.headers.get('host'))
  if (!host) {
    return jsonResponse(404, { error: 'not_found' })
  }

  try {
    const suffix = normalizeSiteHostSuffix(
      process.env.PUBLIC_SITE_HOST_SUFFIX || process.env.NEXT_PUBLIC_SITE_HOST_SUFFIX || '',
    )
    if (!getSiteSlugFromHost(host, suffix)) {
      return jsonResponse(404, { error: 'not_found' })
    }
  } catch {
    return jsonResponse(404, { error: 'not_found' })
  }

  const rawBody = await request.text()
  if (rawBody.length > MAX_BODY_BYTES) {
    return jsonResponse(413, { error: 'payload_too_large' })
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(rawBody)
  } catch {
    return jsonResponse(400, { error: 'invalid_json' })
  }

  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return jsonResponse(400, { error: 'invalid_body' })
  }

  const target = `${getSitesInternalApiUrl()}/api/v1/sites/public/${resource}`
  const headers = new Headers({
    'Content-Type': 'application/json',
    Accept: 'application/json',
    'X-Site-Host': hostnameFromHost(host),
  })
  const forwardedFor = request.headers.get('x-forwarded-for')
  if (forwardedFor) {
    headers.set('X-Forwarded-For', forwardedFor)
  }

  try {
    const upstream = await fetch(target, {
      method: 'POST',
      headers,
      body: JSON.stringify(parsed),
      cache: 'no-store',
      signal: AbortSignal.timeout(10_000),
    })

    const upstreamBody = await upstream.text()
    const contentType = upstream.headers.get('content-type') || 'application/json'
    return new Response(upstreamBody, {
      status: upstream.status,
      headers: {
        'Content-Type': contentType,
        'Cache-Control': 'no-store',
      },
    })
  } catch {
    return jsonResponse(502, { error: 'upstream_unavailable' })
  }
}

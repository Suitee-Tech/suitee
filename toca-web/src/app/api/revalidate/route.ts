import { revalidatePath, revalidateTag } from 'next/cache'
import { isValidCompanyTenantId, isValidInternalApiKey } from '@/lib/internal-key'
import { siteCacheTag } from '@/lib/cache'
import { isValidSiteSlug } from '@/lib/site-host'

export const dynamic = 'force-dynamic'

interface RevalidateBody {
  tenantSlug: string
  paths: string[]
}

function normalizePublicPath(value: unknown): string | null {
  if (typeof value !== 'string') {
    return null
  }

  const raw = value.trim()
  if (
    !raw.startsWith('/') ||
    raw.startsWith('//') ||
    raw.length > 300 ||
    raw.includes('..') ||
    raw.includes('\\') ||
    /[\u0000-\u001f]/.test(raw)
  ) {
    return null
  }

  try {
    const url = new URL(raw, 'https://site.invalid')
    return url.pathname || '/'
  } catch {
    return null
  }
}

function json(status: number, body: Record<string, unknown>): Response {
  return Response.json(body, {
    status,
    headers: { 'Cache-Control': 'no-store' },
  })
}

/**
 * On-demand ISR cache invalidation called by toca-sites after a Payload sync.
 *
 * Contract:
 *   POST /api/revalidate
 *   X-Internal-Key: <INTERNAL_API_KEY>
 *   { "tenantSlug": "...", "paths": ["/", "/imoveis", "/imoveis/<seoSlug>"] }
 *
 * 200 {"revalidated":true} | 400 invalid body | 401 invalid key
 */
export async function POST(request: Request): Promise<Response> {
  if (!isValidInternalApiKey(request.headers.get('x-internal-key'))) {
    return json(401, { error: 'unauthorized' })
  }

  let body: unknown
  try {
    body = await request.json()
  } catch {
    return json(400, { error: 'invalid_json' })
  }

  if (!body || typeof body !== 'object' || Array.isArray(body)) {
    return json(400, { error: 'invalid_body' })
  }

  const payload = body as Partial<RevalidateBody>
  const tenantSlug = typeof payload.tenantSlug === 'string' ? payload.tenantSlug : ''
  if (!isValidSiteSlug(tenantSlug)) {
    return json(400, { error: 'invalid_tenant_slug' })
  }

  // X-Tenant-Id is optional on this endpoint; when present it must be a valid
  // company UUID. The Go client intentionally sends only the key + body.
  const tenantId = request.headers.get('x-tenant-id')
  if (tenantId && !isValidCompanyTenantId(tenantId)) {
    return json(400, { error: 'invalid_tenant_id' })
  }

  if (!Array.isArray(payload.paths) || payload.paths.length === 0 || payload.paths.length > 100) {
    return json(400, { error: 'invalid_paths' })
  }

  const paths = new Set<string>()
  for (const rawPath of payload.paths) {
    const normalized = normalizePublicPath(rawPath)
    if (!normalized) {
      return json(400, { error: 'invalid_path' })
    }
    paths.add(normalized)
  }

  try {
    // Every tenant data cache entry carries this tag (tenant, pages and
    // listings are all nested under it), so one call invalidates the tenant's
    // entire public projection.
    revalidateTag(siteCacheTag(tenantSlug), { expire: 0 })

    for (const pathname of paths) {
      revalidatePath(pathname)
      // The public URL is a middleware rewrite; invalidate the internal route
      // as well so path-based caches have no stale duplicate.
      revalidatePath(`/site/${tenantSlug}${pathname === '/' ? '' : pathname}`)
    }
  } catch {
    return json(500, { error: 'revalidation_failed' })
  }

  return json(200, { revalidated: true })
}

import { unstable_cache } from 'next/cache'
import { cache } from 'react'
import { headers } from 'next/headers'
import type { Tenant } from '@/payload-types'
import {
  getSiteOriginFromHost,
  getSiteSlugFromHost,
  normalizeHostHeader,
} from './site-host'
import { SITE_CACHE_SECONDS, siteCacheTag } from './cache'
import { getPayloadClient } from './payload'

export interface SiteContext {
  tenant: Tenant
  slug: string
  host: string
  origin: string
}

async function queryTenantBySlug(slug: string): Promise<Tenant | null> {
  const payload = await getPayloadClient()
  const result = await payload.find({
    collection: 'tenants',
    depth: 0,
    limit: 1,
    overrideAccess: true,
    pagination: false,
    where: {
      and: [{ slug: { equals: slug } }, { enabled: { equals: true } }],
    },
  })

  return result.docs[0] ?? null
}

/**
 * Tenant lookup is cached across requests and always constrained by
 * slug AND enabled=true. Unknown/disabled tenants resolve to null and the
 * segment layout calls notFound(). The wrapper is constructed with a
 * slug-specific tag; unstable_cache keys include the supplied keyParts, so
 * entries remain tenant-isolated.
 */
export async function getTenantBySlug(slug: string): Promise<Tenant | null> {
  return unstable_cache(
    async () => queryTenantBySlug(slug),
    ['site-tenant-v1', slug],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [siteCacheTag(slug)],
    },
  )()
}

/**
 * Request-scoped dedupe for the current Host. This function must remain
 * dynamic: the storefront never resolves a tenant from query params or cookies.
 */
export const getSiteContext = cache(async (): Promise<SiteContext | null> => {
  const requestHeaders = await headers()
  const rawHost = requestHeaders.get('host')
  const host = normalizeHostHeader(rawHost)
  if (!host) {
    return null
  }

  let slug: string | null = null
  try {
    slug = getSiteSlugFromHost(host)
  } catch {
    return null
  }

  if (!slug) {
    return null
  }

  const tenant = await getTenantBySlug(slug)
  if (!tenant || tenant.enabled !== true) {
    return null
  }

  const origin = getSiteOriginFromHost(host)
  if (!origin) {
    return null
  }

  return { tenant, slug, host, origin }
})

export function tenantParamMatches(context: SiteContext, tenantParam: string): boolean {
  return context.slug === tenantParam
}

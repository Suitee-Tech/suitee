import type { Access, PayloadRequest, TypedUser, Where } from 'payload'
import { getSiteSlugFromHost, getPublicSiteSuffix } from '@/lib/site-host'
import { getInternalServiceUser } from './internal-auth'

export function isSuperAdminUser(user: unknown): boolean {
  if (!user || typeof user !== 'object') {
    return false
  }

  const candidate = user as Record<string, unknown>
  return candidate.collection === 'users' && candidate.role === 'super-admin'
}

/**
 * Passed to @payloadcms/plugin-multi-tenant. Internal service identities and
 * super admins may address every tenant; ordinary users are never created for
 * agencies in this app.
 */
export function userHasAccessToAllTenants(user: TypedUser): boolean {
  return Boolean(getInternalServiceUser(user)) || isSuperAdminUser(user)
}

function tenantWhere(tenantDocId: string | number, extra?: Where): Where {
  if (!extra) {
    return { tenant: { equals: tenantDocId } }
  }

  return {
    and: [{ tenant: { equals: tenantDocId } }, extra],
  }
}

function publicTenantSlug(req: PayloadRequest): string | null {
  const host = req.headers.get('host')
  if (!host) {
    return null
  }

  try {
    return getSiteSlugFromHost(host, getPublicSiteSuffix())
  } catch {
    return null
  }
}

/**
 * Resolve the Host tenant to its Payload doc id for public REST access.
 *
 * Payload's access-control layer cannot consume the dotted relationship path
 * (`tenant.slug`) that works in a user-supplied query, so the host constraint is
 * resolved to an id here. This is also the only place the public REST path
 * trusts a tenant, and that tenant comes exclusively from the validated Host.
 */
async function publicTenantDocId(req: PayloadRequest): Promise<number | string | null> {
  const slug = publicTenantSlug(req)
  if (!slug) {
    return null
  }

  try {
    const result = await req.payload.find({
      collection: 'tenants',
      depth: 0,
      limit: 1,
      overrideAccess: true,
      pagination: false,
      where: {
        and: [{ slug: { equals: slug } }, { enabled: { equals: true } }],
      },
    })

    return result.docs[0]?.id ?? null
  } catch {
    return null
  }
}

/**
 * Read access for pages/listings.
 *
 * - Internal service: any status, strictly scoped to X-Tenant-Id.
 * - Super admin: everything (Payload Admin).
 * - Anonymous/public: only published docs for the tenant derived exclusively
 *   from the request Host. Query params cannot widen this because Payload ANDs
 *   field access with the caller's where clause.
 */
export function publishedTenantReadAccess(field: 'pages' | 'listings'): Access {
  void field

  return async ({ req }) => {
    const internal = getInternalServiceUser(req.user)
    if (internal) {
      if (internal.internalTenantDocId === null) {
        return false
      }

      return tenantWhere(internal.internalTenantDocId)
    }

    if (isSuperAdminUser(req.user)) {
      return true
    }

    const tenantDocId = await publicTenantDocId(req)
    if (tenantDocId === null) {
      return false
    }

    return {
      and: [
        { tenant: { equals: tenantDocId } },
        { status: { equals: 'published' } },
      ],
    }
  }
}

/** Internal writes and super admins only. The tenant is forced in hooks. */
export const internalCreateAccess: Access = ({ req }) =>
  Boolean(getInternalServiceUser(req.user)) || isSuperAdminUser(req.user)

/** Updates are limited to the internal caller's tenant, or super admin. */
export const internalUpdateAccess: Access = ({ req }) => {
  const internal = getInternalServiceUser(req.user)
  if (internal) {
    return internal.internalTenantDocId === null
      ? false
      : tenantWhere(internal.internalTenantDocId)
  }

  return isSuperAdminUser(req.user)
}

export const superAdminAccess: Access = ({ req }) => isSuperAdminUser(req.user)

export const internalOrSuperAdminAccess: Access = ({ req }) =>
  Boolean(getInternalServiceUser(req.user)) || isSuperAdminUser(req.user)

/**
 * Tenants collection: machine callers can only see their own company UUID.
 * This supports the Go bootstrap flow GET by suiteeTenantId before the doc
 * exists (returns an empty docs array).
 */
export const tenantsReadAccess: Access = ({ req }) => {
  const internal = getInternalServiceUser(req.user)
  if (internal) {
    return { suiteeTenantId: { equals: internal.internalSuiteeTenantId } }
  }

  if (isSuperAdminUser(req.user)) {
    return true
  }

  // Anonymous REST callers on a tenant Host may read only that tenant's public
  // projection. This also makes relationship filters such as
  // `where[tenant.slug][equals]=...` valid for Payload query-path validation,
  // while the returned tenants read still ANDs the Host slug into the query.
  // Sensitive fields (for example suiteeTenantId) are hidden by field access.
  const slug = publicTenantSlug(req)
  if (!slug) {
    return false
  }

  const publicWhere: Where = {
    and: [{ slug: { equals: slug } }, { enabled: { equals: true } }],
  }
  return publicWhere
}

export const tenantsUpdateAccess: Access = ({ req }) => {
  const internal = getInternalServiceUser(req.user)
  if (internal) {
    return { suiteeTenantId: { equals: internal.internalSuiteeTenantId } }
  }

  return isSuperAdminUser(req.user)
}

export const tenantsCreateAccess: Access = internalOrSuperAdminAccess
export const tenantsDeleteAccess: Access = superAdminAccess

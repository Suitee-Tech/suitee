import type { AuthStrategy, Payload, TypedUser } from 'payload'
import { isValidCompanyTenantId, isValidInternalApiKey } from '@/lib/internal-key'

/**
 * Synthetic identity used for machine-to-machine Payload calls made by
 * toca-sites (X-Internal-Key + X-Tenant-Id). It is never persisted and is not
 * allowed into the admin UI or into the users collection.
 */
export interface InternalServiceUser {
  _strategy: 'internal-service'
  collection: 'users'
  id: string
  internalService: true
  internalSuiteeTenantId: string
  internalTenantDocId: string | number | null
  tenants: Array<{ tenant: string | number }>
}

export function getInternalServiceUser(user: unknown): InternalServiceUser | null {
  if (!user || typeof user !== 'object') {
    return null
  }

  const candidate = user as Record<string, unknown>
  if (candidate.internalService !== true || candidate._strategy !== 'internal-service') {
    return null
  }

  const suiteeTenantId = candidate.internalSuiteeTenantId
  const tenantDocId = candidate.internalTenantDocId

  if (typeof suiteeTenantId !== 'string') {
    return null
  }

  return {
    _strategy: 'internal-service',
    collection: 'users',
    id: typeof candidate.id === 'string' ? candidate.id : `internal-service:${suiteeTenantId}`,
    internalService: true,
    internalSuiteeTenantId: suiteeTenantId,
    internalTenantDocId:
      typeof tenantDocId === 'string' || typeof tenantDocId === 'number' ? tenantDocId : null,
    tenants: Array.isArray(candidate.tenants)
      ? (candidate.tenants as Array<{ tenant: string | number }>)
      : [],
  }
}

async function findTenantDocId(
  payload: Payload,
  suiteeTenantId: string,
): Promise<string | number | null> {
  const result = await payload.find({
    collection: 'tenants',
    depth: 0,
    limit: 1,
    overrideAccess: true,
    pagination: false,
    where: {
      suiteeTenantId: { equals: suiteeTenantId },
    },
  })

  return result.docs[0]?.id ?? null
}

/**
 * Payload auth strategy that turns the internal API headers into a synthetic
 * user. Registering it as a strategy (instead of only checking headers in
 * access functions) lets @payloadcms/plugin-multi-tenant see a legitimate
 * tenant-capable user while still keeping the internal service out of every
 * human-facing access path.
 */
export const internalServiceStrategy: AuthStrategy = {
  name: 'internal-service',
  authenticate: async ({ headers, payload }) => {
    const providedKey = headers.get('x-internal-key')
    const suiteeTenantId = headers.get('x-tenant-id')?.trim()

    if (!isValidInternalApiKey(providedKey) || !isValidCompanyTenantId(suiteeTenantId)) {
      return { user: null }
    }

    let tenantDocId: string | number | null = null
    try {
      tenantDocId = await findTenantDocId(payload, suiteeTenantId)
    } catch {
      // Tenant bootstrap: before the first tenant write there is no tenant doc
      // for this company id. The synthetic identity can still create/read the
      // tenant collection, while all tenant-owned collection writes fail closed.
      tenantDocId = null
    }

    const user: InternalServiceUser = {
      _strategy: 'internal-service',
      collection: 'users',
      id: `internal-service:${suiteeTenantId}`,
      internalService: true,
      internalSuiteeTenantId: suiteeTenantId,
      internalTenantDocId: tenantDocId,
      tenants: tenantDocId === null ? [] : [{ tenant: tenantDocId }],
    }

    return {
      user: user as unknown as TypedUser,
    }
  },
}

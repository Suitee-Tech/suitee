import { APIError, type CollectionBeforeValidateHook } from 'payload'
import { getInternalServiceUser } from '@/access/internal-auth'
import { isValidSiteSlug, MIN_SITE_SLUG_LENGTH, MAX_SITE_SLUG_LENGTH } from '@/lib/site-host'

type TenantOwnedData = Record<string, unknown>

function relationshipId(value: unknown): string | number | null {
  if (typeof value === 'string' || typeof value === 'number') {
    return value
  }

  if (value && typeof value === 'object' && 'id' in value) {
    const id = (value as { id?: unknown }).id
    if (typeof id === 'string' || typeof id === 'number') {
      return id
    }
  }

  return null
}

/**
 * Force the tenant relationship from the authenticated internal identity.
 *
 * This runs before the multi-tenant plugin's own beforeValidate hook, so REST
 * requests made by toca-sites always carry a tenant that the plugin can check.
 * A payload cannot smuggle a different tenant through the JSON body.
 */
export const forceTenantFromInternal: CollectionBeforeValidateHook = ({
  data,
  operation,
  originalDoc,
  req,
}) => {
  const internal = getInternalServiceUser(req.user)
  if (!internal) {
    return data
  }

  if (internal.internalTenantDocId === null) {
    throw new APIError(
      'The X-Tenant-Id must reference an existing tenant before writing scoped documents.',
      403,
    )
  }

  const document = (data ?? {}) as TenantOwnedData

  if (operation === 'update' && originalDoc) {
    const originalTenantId = relationshipId((originalDoc as TenantOwnedData).tenant)
    if (
      originalTenantId !== null &&
      String(originalTenantId) !== String(internal.internalTenantDocId)
    ) {
      throw new APIError('Cross-tenant updates are not allowed.', 403)
    }
  }

  document.tenant = internal.internalTenantDocId
  return document
}

/**
 * The company UUID is immutable for machine callers: X-Tenant-Id is the source
 * of truth, not the JSON body. Super admins editing in Payload Admin are not
 * affected.
 */
export const forceSuiteeTenantIdFromInternal: CollectionBeforeValidateHook = ({
  data,
  originalDoc,
  req,
}) => {
  const internal = getInternalServiceUser(req.user)
  if (!internal) {
    return data
  }

  const document = (data ?? {}) as TenantOwnedData

  if (originalDoc) {
    const originalSuiteeId = (originalDoc as TenantOwnedData).suiteeTenantId
    if (
      typeof originalSuiteeId === 'string' &&
      originalSuiteeId !== internal.internalSuiteeTenantId
    ) {
      throw new APIError('Cross-tenant updates are not allowed.', 403)
    }
  }

  document.suiteeTenantId = internal.internalSuiteeTenantId
  return document
}

/** Sets publishedAt the first time a projection becomes public. */
export const setListingPublishedAt: CollectionBeforeValidateHook = ({ data, originalDoc }) => {
  const document = (data ?? {}) as TenantOwnedData
  const previous = (originalDoc ?? {}) as TenantOwnedData

  if (document.status === 'published' && !document.publishedAt && !previous.publishedAt) {
    document.publishedAt = new Date().toISOString()
  }

  return document
}

export function validateSiteSlug(value: unknown): true | string {
  if (typeof value !== 'string') {
    return 'Informe a marca do site.'
  }

  if (!isValidSiteSlug(value)) {
    return `Use de ${MIN_SITE_SLUG_LENGTH} a ${MAX_SITE_SLUG_LENGTH} caracteres, apenas a-z, 0-9 e hífen, sem hífen nas pontas e sem palavras reservadas.`
  }

  return true
}

export function validateSeoSlug(value: unknown): true | string {
  if (typeof value !== 'string' || !/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(value)) {
    return 'Use apenas a-z, 0-9 e hífen, sem hífen nas pontas.'
  }

  if (value.length > 160) {
    return 'O slug deve ter no máximo 160 caracteres.'
  }

  return true
}

export function validateUuid(value: unknown): true | string {
  if (
    typeof value !== 'string' ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)
  ) {
    return 'Informe um UUID válido.'
  }

  return true
}

export function validateHexColor(value: unknown): true | string {
  if (value === undefined || value === null || value === '') {
    return true
  }

  if (typeof value !== 'string' || !/^#[0-9a-fA-F]{6}$/.test(value)) {
    return 'Use uma cor hexadecimal no formato #RRGGBB.'
  }

  return true
}

export function validatePublicDomain(value: unknown): true | string {
  if (value === undefined || value === null || value === '') {
    return true
  }

  if (
    typeof value !== 'string' ||
    value.length > 253 ||
    !/^(?=.{4,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i.test(value)
  ) {
    return 'Informe um domínio válido, sem protocolo ou caminho.'
  }

  return true
}

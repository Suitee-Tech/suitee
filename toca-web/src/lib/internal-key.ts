import { createHash, timingSafeEqual } from 'node:crypto'
import { getInternalApiKey } from './env'

/**
 * Constant-time comparison using fixed-length digests. The raw key lengths are
 * never compared directly and the configured key is never logged.
 */
export function isValidInternalApiKey(provided: string | null | undefined): boolean {
  const expected = getInternalApiKey()
  if (!expected || !provided) {
    return false
  }

  const providedDigest = createHash('sha256').update(provided, 'utf8').digest()
  const expectedDigest = createHash('sha256').update(expected, 'utf8').digest()
  return timingSafeEqual(providedDigest, expectedDigest)
}

export function isValidCompanyTenantId(value: string | null | undefined): value is string {
  if (!value) {
    return false
  }

  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)
}

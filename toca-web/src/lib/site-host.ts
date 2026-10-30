/**
 * Host and slug primitives shared by Edge middleware and server components.
 *
 * This module intentionally has no Node.js or Payload imports: the public host
 * is the only tenant source on the storefront and it must be validated before
 * any database lookup.
 */

export const RESERVED_SITE_SLUGS = [
  'www',
  'app',
  'api',
  'admin',
  'portal',
  'sites',
  'mail',
  'cdn',
  'imoveis',
  'suitee',
] as const

const RESERVED_SITE_SLUG_SET: ReadonlySet<string> = new Set(RESERVED_SITE_SLUGS)
const SITE_SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/

export const MIN_SITE_SLUG_LENGTH = 3
export const MAX_SITE_SLUG_LENGTH = 48

/**
 * Brand slug rules from the Sites architecture plan:
 * - 3..48 chars
 * - lowercase a-z, digits and hyphen
 * - cannot start or end with a hyphen
 * - cannot use any reserved product subdomain
 */
export function isValidSiteSlug(value: string): boolean {
  if (
    value.length < MIN_SITE_SLUG_LENGTH ||
    value.length > MAX_SITE_SLUG_LENGTH ||
    !SITE_SLUG_PATTERN.test(value)
  ) {
    return false
  }

  return !RESERVED_SITE_SLUG_SET.has(value)
}

export function normalizeSiteSlug(value: string): string {
  return value.trim().toLowerCase()
}

/** Normalizes a Host header while preserving an explicit dev port. */
export function normalizeHostHeader(rawHost: string | null | undefined): string | null {
  if (!rawHost) {
    return null
  }

  const value = rawHost.trim().toLowerCase()
  if (!value || value.length > 255) {
    return null
  }

  // Reject scheme/userinfo/path/whitespace injections in a Host header.
  if (
    value.includes('/') ||
    value.includes('\\') ||
    value.includes('@') ||
    value.includes(',') ||
    value.includes(' ') ||
    value.includes('\t')
  ) {
    return null
  }

  const withoutTrailingDot = value.endsWith('.') ? value.slice(0, -1) : value
  if (!withoutTrailingDot) {
    return null
  }

  // IPv6 literals can never be a tenant host on this product.
  if (withoutTrailingDot.startsWith('[') || (withoutTrailingDot.match(/:/g) ?? []).length > 1) {
    return null
  }

  const colonIndex = withoutTrailingDot.lastIndexOf(':')
  if (colonIndex !== -1) {
    const port = withoutTrailingDot.slice(colonIndex + 1)
    if (!/^\d{2,5}$/.test(port)) {
      return null
    }
  }

  return withoutTrailingDot
}

/** Returns the hostname portion (no port) of a normalized Host header. */
export function hostnameFromHost(normalizedHost: string): string {
  const colonIndex = normalizedHost.lastIndexOf(':')
  return colonIndex === -1 ? normalizedHost : normalizedHost.slice(0, colonIndex)
}

export function normalizeSiteHostSuffix(rawSuffix: string): string {
  const suffix = rawSuffix.trim().toLowerCase().replace(/^\.+/, '').replace(/\.+$/, '')

  if (
    !suffix ||
    suffix.includes('/') ||
    suffix.includes(':') ||
    suffix.includes(' ') ||
    suffix.startsWith('.') ||
    !/^[a-z0-9.-]+$/.test(suffix)
  ) {
    throw new Error('PUBLIC_SITE_HOST_SUFFIX must be a bare DNS suffix, for example imoveis.suitee.com.br')
  }

  return suffix
}

/**
 * Reads the deployment's public site suffix. It is required at runtime; there
 * is no production fallback to a different domain.
 */
export function getPublicSiteSuffix(): string {
  const raw = process.env.PUBLIC_SITE_HOST_SUFFIX || process.env.NEXT_PUBLIC_SITE_HOST_SUFFIX
  if (!raw) {
    throw new Error(
      'PUBLIC_SITE_HOST_SUFFIX (or NEXT_PUBLIC_SITE_HOST_SUFFIX) is required to resolve storefront tenants',
    )
  }

  return normalizeSiteHostSuffix(raw)
}

/**
 * Extracts and validates `{marca}` only when the request host is exactly
 * `{marca}.{suffix}`. Apex hosts, nested subdomains, unknown suffixes and
 * reserved slugs all return null.
 */
export function getSiteSlugFromHost(rawHost: string | null | undefined, suffix?: string): string | null {
  const normalizedHost = normalizeHostHeader(rawHost)
  if (!normalizedHost) {
    return null
  }

  const siteSuffix = suffix ? normalizeSiteHostSuffix(suffix) : getPublicSiteSuffix()
  const hostname = hostnameFromHost(normalizedHost)
  const suffixWithDot = `.${siteSuffix}`

  if (!hostname.endsWith(suffixWithDot)) {
    return null
  }

  const brand = hostname.slice(0, -suffixWithDot.length)
  if (!brand || brand.includes('.')) {
    return null
  }

  return isValidSiteSlug(brand) ? brand : null
}

export function isApexSiteHost(rawHost: string | null | undefined, suffix?: string): boolean {
  const normalizedHost = normalizeHostHeader(rawHost)
  if (!normalizedHost) {
    return false
  }

  const siteSuffix = suffix ? normalizeSiteHostSuffix(suffix) : getPublicSiteSuffix()
  return hostnameFromHost(normalizedHost) === siteSuffix
}

export function siteHostForSlug(slug: string, suffix?: string): string {
  if (!isValidSiteSlug(slug)) {
    throw new Error('Invalid site slug')
  }

  const siteSuffix = suffix ? normalizeSiteHostSuffix(suffix) : getPublicSiteSuffix()
  return `${slug}.${siteSuffix}`
}

function getSiteScheme(hostname: string): 'http' | 'https' {
  const configured = process.env.PUBLIC_SITE_SCHEME?.trim().toLowerCase()
  if (configured === 'http' || configured === 'https') {
    return configured
  }

  return hostname.endsWith('.localhost') || hostname === 'localhost' ? 'http' : 'https'
}

/**
 * Builds an absolute origin from a validated request Host, preserving the local
 * development port. Used for canonical URLs, Open Graph and sitemap entries.
 */
export function getSiteOriginFromHost(rawHost: string | null | undefined): string | null {
  const normalizedHost = normalizeHostHeader(rawHost)
  if (!normalizedHost) {
    return null
  }

  const hostname = hostnameFromHost(normalizedHost)
  return `${getSiteScheme(hostname)}://${normalizedHost}`
}

export function absoluteSiteUrl(origin: string, pathname: string): string {
  const cleanOrigin = origin.replace(/\/+$/, '')
  const cleanPath = pathname.startsWith('/') ? pathname : `/${pathname}`
  return `${cleanOrigin}${cleanPath}`
}

/** Same-origin path for a listing; used by canonical and internal links. */
export function listingPath(seoSlug: string): string {
  return `/imoveis/${encodeURIComponent(seoSlug)}`
}

/** The internal rewrite root. Never linked publicly. */
export function internalSitePath(slug: string, pathname = '/'): string {
  const base = `/site/${encodeURIComponent(slug)}`
  return pathname === '/' ? base : `${base}${pathname.startsWith('/') ? pathname : `/${pathname}`}`
}

export function isInternalSitePath(pathname: string): boolean {
  return pathname === '/site' || pathname.startsWith('/site/')
}

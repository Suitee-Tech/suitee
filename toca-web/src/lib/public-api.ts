export type PublicSiteApiResource = 'views' | 'leads'

/**
 * Browser calls go directly to toca-sites when NEXT_PUBLIC_SITES_API_URL is
 * configured. A same-origin proxy is the fallback so local/preview deployments
 * and environments without a public Go URL still work; the proxy uses
 * SITES_INTERNAL_API_URL server-side.
 */
export function publicSiteApiEndpoint(resource: PublicSiteApiResource): string {
  const base = process.env.NEXT_PUBLIC_SITES_API_URL?.trim()
  if (base) {
    return `${base.replace(/\/+$/, '')}/api/v1/sites/public/${resource}`
  }

  return `/api/site-proxy/${resource}`
}

export function currentPublicHost(): string {
  if (typeof window === 'undefined') {
    return ''
  }

  return window.location.hostname
}

export function currentUtmParams(): {
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
} {
  if (typeof window === 'undefined') {
    return {}
  }

  const params = new URLSearchParams(window.location.search)
  const result: {
    utm_source?: string
    utm_medium?: string
    utm_campaign?: string
  } = {}

  for (const key of ['utm_source', 'utm_medium', 'utm_campaign'] as const) {
    const value = params.get(key)?.trim().slice(0, 120)
    if (value) {
      result[key] = value
    }
  }

  return result
}

export function getAnonymousSessionId(): string {
  if (typeof window === 'undefined') {
    return ''
  }

  const storageKey = 'toca_site_session'
  try {
    const existing = window.sessionStorage.getItem(storageKey)
    if (existing) {
      return existing
    }

    const generated =
      typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
        ? crypto.randomUUID()
        : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`

    window.sessionStorage.setItem(storageKey, generated)
    return generated
  } catch {
    return ''
  }
}

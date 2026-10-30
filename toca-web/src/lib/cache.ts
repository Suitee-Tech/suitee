export const SITE_CACHE_SECONDS = 300


export function siteCacheTag(slug: string): string {
  return `site:${slug}`
}

export function sitePagesCacheTag(slug: string): string {
  return `site:${slug}:pages`
}

export function sitePageCacheTag(slug: string, pageKey: string): string {
  return `site:${slug}:page:${pageKey}`
}

export function siteListingsCacheTag(slug: string): string {
  return `site:${slug}:listings`
}

export function siteListingCacheTag(slug: string, seoSlug: string): string {
  return `site:${slug}:listing:${seoSlug}`
}

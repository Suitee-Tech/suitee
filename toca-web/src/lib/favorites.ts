export interface FavoriteListing {
  id: string
  slug: string
  title: string
  price: string
  location?: string
  imageUrl?: string
  listingType?: string
  savedAt: number
}

export const FAVORITES_EVENT = 'toca:favorites-changed'

export function favoritesStorageKey(tenantSlug: string): string {
  return `toca:favorites:${tenantSlug}:v1`
}

export function readFavorites(tenantSlug: string): FavoriteListing[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(favoritesStorageKey(tenantSlug))
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(
      (item): item is FavoriteListing =>
        Boolean(item) &&
        typeof item === 'object' &&
        typeof (item as FavoriteListing).id === 'string' &&
        typeof (item as FavoriteListing).slug === 'string',
    )
  } catch {
    return []
  }
}

function notify(tenantSlug: string) {
  window.dispatchEvent(new CustomEvent(FAVORITES_EVENT, { detail: { tenantSlug } }))
}

export function toggleFavorite(tenantSlug: string, listing: FavoriteListing): boolean {
  if (typeof window === 'undefined') return false
  const key = favoritesStorageKey(tenantSlug)
  const current = readFavorites(tenantSlug)
  const exists = current.some((item) => item.id === listing.id)
  const next = exists
    ? current.filter((item) => item.id !== listing.id)
    : [{ ...listing, savedAt: Date.now() }, ...current].slice(0, 60)
  try {
    window.localStorage.setItem(key, JSON.stringify(next))
  } catch {
    return false
  }
  notify(tenantSlug)
  return !exists
}

export function favoriteIds(tenantSlug: string): Set<string> {
  return new Set(readFavorites(tenantSlug).map((item) => item.id))
}

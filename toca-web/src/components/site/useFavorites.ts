'use client'

import { useEffect, useState } from 'react'
import {
  FAVORITES_EVENT,
  favoriteIds,
  readFavorites,
  toggleFavorite,
  type FavoriteListing,
} from '@/lib/favorites'

export function useFavorites(tenantSlug: string) {
  const [items, setItems] = useState<FavoriteListing[]>([])
  const [ready, setReady] = useState(false)

  useEffect(() => {
    const sync = () => {
      setItems(readFavorites(tenantSlug))
      setReady(true)
    }
    sync()
    const onChange = () => sync()
    const onStorage = (event: StorageEvent) => {
      if (event.key && event.key.includes(tenantSlug)) sync()
    }
    window.addEventListener(FAVORITES_EVENT, onChange)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener(FAVORITES_EVENT, onChange)
      window.removeEventListener('storage', onStorage)
    }
  }, [tenantSlug])

  const ids = new Set(items.map((item) => item.id))

  return {
    items,
    ready,
    isFavorite: (id: string) => ids.has(id),
    ids: favoriteIds(tenantSlug),
    toggle: (listing: FavoriteListing) => {
      const added = toggleFavorite(tenantSlug, listing)
      setItems(readFavorites(tenantSlug))
      return added
    },
    count: items.length,
  }
}

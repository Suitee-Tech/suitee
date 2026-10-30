'use client'

import { useFavorites } from './useFavorites'
import { HeartIcon } from './icons'
import type { FavoriteListing } from '@/lib/favorites'

interface FavoriteButtonProps {
  tenant: string
  listing: FavoriteListing
  variant?: 'card' | 'detail'
  label?: string
}

export function FavoriteButton({ tenant, listing, variant = 'card', label }: FavoriteButtonProps) {
  const { isFavorite, toggle, ready } = useFavorites(tenant)
  const active = ready && isFavorite(listing.id)
  const action = label ?? (active ? 'Remover dos favoritos' : 'Salvar nos favoritos')

  return (
    <button
      type="button"
      className={`favorite-button favorite-button--${variant}${active ? ' is-active' : ''}`}
      aria-pressed={active}
      aria-label={`${action}: ${listing.title}`}
      title={action}
      onClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        toggle({ ...listing, savedAt: Date.now() })
      }}
    >
      <HeartIcon width={18} height={18} filled={active} />
    </button>
  )
}

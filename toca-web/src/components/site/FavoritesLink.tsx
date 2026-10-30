'use client'

import Link from 'next/link'
import { useFavorites } from './useFavorites'
import { HeartIcon } from './icons'

export function FavoritesLink({ tenant, mobile = false }: { tenant: string; mobile?: boolean }) {
  const { count } = useFavorites(tenant)
  return (
    <Link
      href="/favoritos"
      className={mobile ? 'site-menu__account site-header__favorites' : 'site-header__favorites'}
      aria-label={`Favoritos${count ? ` (${count})` : ''}`}
    >
      <HeartIcon width={16} height={16} filled={count > 0} />
      <span>Favoritos</span>
      {count > 0 ? <span className="site-header__favorites-count">{count}</span> : null}
    </Link>
  )
}

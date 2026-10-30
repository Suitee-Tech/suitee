import Link from 'next/link'
import { collectionHref } from '@/lib/collections'
import type { CollectionSummary } from '@/lib/data'

interface CollectionNavProps {
  summaries: CollectionSummary[]
  currentSlug?: string
  /** A listagem geral marca "Todos"; a home não tem coleção ativa. */
  allActive?: boolean
}

export function CollectionNav({ summaries, currentSlug, allActive = false }: CollectionNavProps) {
  const available = summaries.filter((summary) => summary.totalDocs > 0)
  if (available.length === 0) return null

  return (
    <nav className="collection-nav" aria-label="Coleções de imóveis">
      <Link
        href="/imoveis"
        className={allActive ? 'collection-nav__item is-active' : 'collection-nav__item'}
      >
        Todos
      </Link>
      {available.map(({ collection, totalDocs }) => (
        <Link
          key={collection.slug}
          href={collectionHref(collection)}
          className={
            collection.slug === currentSlug
              ? 'collection-nav__item is-active'
              : 'collection-nav__item'
          }
        >
          {collection.label}
          <span className="collection-nav__count">{totalDocs}</span>
        </Link>
      ))}
    </nav>
  )
}

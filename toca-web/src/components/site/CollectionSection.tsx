import Link from 'next/link'
import type { CollectionSummary } from '@/lib/data'
import { collectionHref } from '@/lib/collections'
import { ArrowIcon } from './icons'
import { ListingCard } from './ListingCard'

interface CollectionSectionProps {
  summary: CollectionSummary
  index: number
  tenant: string
}

export function CollectionSection({ summary, index, tenant }: CollectionSectionProps) {
  const { collection, docs, totalDocs } = summary
  const label = totalDocs === 1 ? '1 imóvel disponível' : `${totalDocs} imóveis disponíveis`

  return (
    <section className={index % 2 === 0 ? 'section' : 'section section--soft'}>
      <div className="site-container">
        <div className="section-heading collection-section__heading">
          <div>
            <p className="eyebrow">{collection.eyebrow}</p>
            <h2>{collection.title}</h2>
            <p className="collection-section__description">
              <span className="collection-count">{label}</span>
              {collection.description}
            </p>
          </div>
          <Link href={collectionHref(collection)} className="text-link">
            Ver todos
            <ArrowIcon width={18} height={18} />
          </Link>
        </div>
        <div className="listing-grid">
          {docs.map((listing) => (
            <ListingCard key={listing.id} listing={listing} tenant={tenant} />
          ))}
        </div>
      </div>
    </section>
  )
}

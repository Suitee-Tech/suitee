import Link from 'next/link'
import type { Listing } from '@/payload-types'
import {
  formatCurrency,
  listingFeatureItems,
  listingLocation,
  listingPrimaryPrice,
  primaryListingImage,
  PROPERTY_TYPE_LABELS,
  LISTING_TYPE_LABELS,
} from '@/lib/format'
import { AreaIcon, BathIcon, BedIcon, CarIcon, PinIcon } from './icons'
import { FavoriteButton } from './FavoriteButton'

interface ListingCardProps {
  listing: Listing
  tenant: string
}

function FeatureIcon({ label }: { label: string }) {
  if (label === 'Quartos') return <BedIcon width={17} height={17} />
  if (label === 'Suítes') return <BedIcon width={17} height={17} />
  if (label === 'Banheiros') return <BathIcon width={17} height={17} />
  if (label === 'Vagas') return <CarIcon width={17} height={17} />
  return <AreaIcon width={17} height={17} />
}

export function ListingCard({ listing, tenant }: ListingCardProps) {
  const image = primaryListingImage(listing)
  const location = listingLocation(listing)
  const facts = listingFeatureItems(listing).slice(0, 3)
  const condo = formatCurrency(listing.condoFee)
  const favorite = {
    id: String(listing.id),
    slug: listing.seoSlug,
    title: listing.title,
    price: listingPrimaryPrice(listing),
    location: location || undefined,
    imageUrl: image || undefined,
    listingType: listing.listingType,
    savedAt: 0,
  }

  return (
    <article className="listing-card">
      <Link href={`/imoveis/${listing.seoSlug}`} className="listing-card__media">
        {image ? (
          <img
            src={image}
            alt={listing.images?.find((item) => item.url === image)?.alt || listing.title}
            loading="lazy"
            className="listing-card__image"
          />
        ) : (
          <div className="listing-card__placeholder" aria-hidden="true">
            <span>{PROPERTY_TYPE_LABELS[listing.propertyType]}</span>
          </div>
        )}
        <span className="listing-card__type">{LISTING_TYPE_LABELS[listing.listingType]}</span>
      </Link>
      <FavoriteButton tenant={tenant} listing={favorite} />

      <div className="listing-card__body">
        <p className="listing-card__category">{PROPERTY_TYPE_LABELS[listing.propertyType]}</p>
        <h3>
          <Link href={`/imoveis/${listing.seoSlug}`}>{listing.title}</Link>
        </h3>
        {location ? (
          <p className="listing-card__location">
            <PinIcon width={15} height={15} />
            {location}
          </p>
        ) : null}

        <div className="listing-card__facts">
          {facts.map((fact) => (
            <span key={fact.label} className="listing-card__fact" title={fact.label}>
              <FeatureIcon label={fact.label} />
              {fact.value}
            </span>
          ))}
        </div>

        <div className="listing-card__footer">
          <div>
            <p className="listing-card__price">{listingPrimaryPrice(listing)}</p>
            {condo ? <p className="listing-card__condo">Condomínio {condo}</p> : null}
          </div>
          <Link
            href={`/imoveis/${listing.seoSlug}`}
            className="listing-card__link"
            aria-label={`Ver detalhes de ${listing.title}`}
          >
            Ver detalhes
          </Link>
        </div>
      </div>
    </article>
  )
}

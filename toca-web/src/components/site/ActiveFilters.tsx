import Link from 'next/link'
import type { ListingFilters } from '@/lib/data'
import { formatCurrency, LISTING_TYPE_LABELS, PROPERTY_TYPE_LABELS } from '@/lib/format'

interface ActiveFiltersProps {
  filters: ListingFilters
  basePath?: string
  lockedListingType?: boolean
  lockedPropertyType?: boolean
}

type FilterKey = 'q' | 'listingType' | 'propertyType' | 'city' | 'minPrice' | 'maxPrice'

function hrefWithout(
  filters: ListingFilters,
  key: FilterKey,
  basePath: string,
): string {
  const params = new URLSearchParams()
  if (key !== 'q' && filters.q) params.set('q', filters.q)
  if (key !== 'listingType' && filters.listingType) params.set('listingType', filters.listingType)
  if (key !== 'propertyType' && filters.propertyType) {
    params.set('propertyType', filters.propertyType)
  }
  if (key !== 'city' && filters.city) params.set('city', filters.city)
  if (key !== 'minPrice' && filters.minPrice !== undefined) {
    params.set('minPrice', String(filters.minPrice))
  }
  if (key !== 'maxPrice' && filters.maxPrice !== undefined) {
    params.set('maxPrice', String(filters.maxPrice))
  }
  const query = params.toString()
  return query ? `${basePath}?${query}` : basePath
}

export function ActiveFilters({
  filters,
  basePath = '/imoveis',
  lockedListingType = false,
  lockedPropertyType = false,
}: ActiveFiltersProps) {
  const chips: { key: FilterKey; label: string }[] = []
  if (filters.q) chips.push({ key: 'q', label: `Busca: “${filters.q}”` })
  if (filters.listingType && !lockedListingType) {
    chips.push({ key: 'listingType', label: LISTING_TYPE_LABELS[filters.listingType] })
  }
  if (filters.propertyType && !lockedPropertyType) {
    chips.push({ key: 'propertyType', label: PROPERTY_TYPE_LABELS[filters.propertyType] })
  }
  if (filters.city) chips.push({ key: 'city', label: filters.city })
  if (filters.minPrice !== undefined) {
    chips.push({
      key: 'minPrice',
      label: `A partir de ${formatCurrency(filters.minPrice) ?? `R$ ${filters.minPrice}`}`,
    })
  }
  if (filters.maxPrice !== undefined) {
    chips.push({
      key: 'maxPrice',
      label: `Até ${formatCurrency(filters.maxPrice) ?? `R$ ${filters.maxPrice}`}`,
    })
  }

  if (chips.length === 0) return null

  return (
    <div className="active-filters" aria-label="Filtros ativos">
      <span className="active-filters__label">Filtros ativos</span>
      <ul className="active-filters__list">
        {chips.map((chip) => (
          <li key={chip.key}>
            <Link href={hrefWithout(filters, chip.key, basePath)} className="filter-chip">
              {chip.label}
              <span aria-hidden="true">×</span>
              <span className="sr-only">Remover filtro {chip.label}</span>
            </Link>
          </li>
        ))}
      </ul>
      <Link href={basePath} className="active-filters__clear">
        Limpar tudo
      </Link>
    </div>
  )
}

import Link from 'next/link'
import type { ListingFilters } from '@/lib/data'
import { LISTING_TYPE_LABELS, PROPERTY_TYPE_LABELS } from '@/lib/format'

interface SearchFiltersProps {
  filters: ListingFilters
  basePath?: string
  lockedListingType?: boolean
  lockedPropertyType?: boolean
}

export function SearchFilters({
  filters,
  basePath = '/imoveis',
  lockedListingType = false,
  lockedPropertyType = false,
}: SearchFiltersProps) {
  return (
    <form className="filters" action={basePath} method="get">
      <div className="filters__top">
        <label className="field field--search">
          <span className="field__label">Buscar</span>
          <input
            type="search"
            name="q"
            defaultValue={filters.q ?? ''}
            placeholder="Bairro, cidade ou palavra-chave"
          />
        </label>
        {lockedListingType && filters.listingType ? (
          <input type="hidden" name="listingType" value={filters.listingType} />
        ) : (
          <label className="field">
            <span className="field__label">Tipo de anúncio</span>
            <select name="listingType" defaultValue={filters.listingType ?? ''}>
              <option value="">Venda e locação</option>
              {Object.entries(LISTING_TYPE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        )}
        {lockedPropertyType && filters.propertyType ? (
          <input type="hidden" name="propertyType" value={filters.propertyType} />
        ) : (
          <label className="field">
            <span className="field__label">Tipo de imóvel</span>
            <select name="propertyType" defaultValue={filters.propertyType ?? ''}>
              <option value="">Todos</option>
              {Object.entries(PROPERTY_TYPE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="field">
          <span className="field__label">Cidade</span>
          <input type="text" name="city" defaultValue={filters.city ?? ''} placeholder="Ex.: Fortaleza" />
        </label>
        <label className="field field--price">
          <span className="field__label">Preço mín.</span>
          <input
            type="number"
            name="minPrice"
            min={0}
            step={1000}
            inputMode="numeric"
            defaultValue={filters.minPrice ?? ''}
            placeholder="R$"
          />
        </label>
        <label className="field field--price">
          <span className="field__label">Preço máx.</span>
          <input
            type="number"
            name="maxPrice"
            min={0}
            step={1000}
            inputMode="numeric"
            defaultValue={filters.maxPrice ?? ''}
            placeholder="R$"
          />
        </label>
        <div className="filters__actions">
          <button type="submit" className="button button--primary">
            Filtrar imóveis
          </button>
          <Link href={basePath} className="filters__reset">
            Limpar filtros
          </Link>
        </div>
      </div>
    </form>
  )
}

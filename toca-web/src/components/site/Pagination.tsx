import Link from 'next/link'
import type { ListingFilters } from '@/lib/data'

interface PaginationProps {
  currentPage: number
  totalPages: number
  filters: ListingFilters
  basePath?: string
}

function hrefForPage(filters: ListingFilters, page: number, basePath: string): string {
  const query = new URLSearchParams()
  if (filters.listingType) query.set('listingType', filters.listingType)
  if (filters.propertyType) query.set('propertyType', filters.propertyType)
  if (filters.city) query.set('city', filters.city)
  if (filters.q) query.set('q', filters.q)
  if (filters.minPrice !== undefined) query.set('minPrice', String(filters.minPrice))
  if (filters.maxPrice !== undefined) query.set('maxPrice', String(filters.maxPrice))
  if (page > 1) query.set('page', String(page))

  const queryString = query.toString()
  return queryString ? `${basePath}?${queryString}` : basePath
}

function pageWindow(currentPage: number, totalPages: number): Array<number | 'ellipsis'> {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index + 1)
  }

  const pages = new Set<number>([1, totalPages, currentPage])
  if (currentPage > 2) pages.add(currentPage - 1)
  if (currentPage < totalPages - 1) pages.add(currentPage + 1)

  const ordered = [...pages].sort((a, b) => a - b)
  const result: Array<number | 'ellipsis'> = []
  ordered.forEach((page, index) => {
    const previous = ordered[index - 1]
    if (index > 0 && previous !== undefined && page - previous > 1) {
      result.push('ellipsis')
    }
    result.push(page)
  })

  return result
}

export function Pagination({
  currentPage,
  totalPages,
  filters,
  basePath = '/imoveis',
}: PaginationProps) {
  if (totalPages <= 1) {
    return null
  }

  return (
    <nav className="pagination" aria-label="Paginação de imóveis">
      {currentPage > 1 ? (
        <Link className="pagination__step" href={hrefForPage(filters, currentPage - 1, basePath)}>
          Anterior
        </Link>
      ) : (
        <span className="pagination__step pagination__step--disabled">Anterior</span>
      )}

      <div className="pagination__pages">
        {pageWindow(currentPage, totalPages).map((page, index) =>
          page === 'ellipsis' ? (
            <span key={`ellipsis-${index}`} className="pagination__ellipsis">
              …
            </span>
          ) : (
            <Link
              key={page}
              href={hrefForPage(filters, page, basePath)}
              className={
                page === currentPage ? 'pagination__page pagination__page--active' : 'pagination__page'
              }
              aria-current={page === currentPage ? 'page' : undefined}
            >
              {page}
            </Link>
          ),
        )}
      </div>

      {currentPage < totalPages ? (
        <Link className="pagination__step" href={hrefForPage(filters, currentPage + 1, basePath)}>
          Próxima
        </Link>
      ) : (
        <span className="pagination__step pagination__step--disabled">Próxima</span>
      )}
    </nav>
  )
}

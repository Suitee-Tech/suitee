import { unstable_cache } from 'next/cache'
import type { Where } from 'payload'
import type { Listing, Page } from '@/payload-types'
import {
  SITE_CACHE_SECONDS,
  siteCacheTag,
  siteListingCacheTag,
  siteListingsCacheTag,
  sitePageCacheTag,
  sitePagesCacheTag,
} from './cache'
import { LISTING_COLLECTIONS, type ListingCollectionDefinition, collectionFilters } from './collections'
import { getPayloadClient } from './payload'
import type { SiteContext } from './site'

export const LISTINGS_PER_PAGE = 9
const SITEMAP_BATCH_SIZE = 500
const SITEMAP_MAX_LISTINGS = 20_000

export type ListingType = Listing['listingType']
export type PropertyType = Listing['propertyType']

export interface ListingFilters {
  page: number
  listingType?: ListingType
  propertyType?: PropertyType
  city?: string
  q?: string
  minPrice?: number
  maxPrice?: number
}

export interface ListingSearchResult {
  docs: Listing[]
  totalDocs: number
  totalPages: number
  page: number
  hasNextPage: boolean
  hasPrevPage: boolean
}

export interface SitemapListing {
  seoSlug: string
  updatedAt: string
}

const LISTING_TYPES: readonly ListingType[] = ['sale', 'rent']
const PROPERTY_TYPES: readonly PropertyType[] = ['apartment', 'house', 'land', 'commercial']

function single(value: string | string[] | undefined): string | undefined {
  if (Array.isArray(value)) {
    return value[0]
  }

  return value
}

function parsePositiveInteger(value: string | undefined, fallback: number): number {
  if (!value || !/^\d+$/.test(value)) {
    return fallback
  }

  const parsed = Number.parseInt(value, 10)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback
}

function parsePrice(value: string | undefined): number | undefined {
  if (!value) {
    return undefined
  }

  const normalized = value.replace(/\./g, '').replace(',', '.').replace(/[^\d.]/g, '')
  const parsed = Number.parseFloat(normalized)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : undefined
}

export function parseListingFilters(
  searchParams: Record<string, string | string[] | undefined>,
): ListingFilters {
  const rawType = single(searchParams.listingType)
  const rawPropertyType = single(searchParams.propertyType)
  const city = single(searchParams.city)?.trim().slice(0, 120)
  const q = single(searchParams.q)?.trim().slice(0, 120)
  const minPrice = parsePrice(single(searchParams.minPrice))
  const maxPrice = parsePrice(single(searchParams.maxPrice))

  return {
    page: parsePositiveInteger(single(searchParams.page), 1),
    listingType: LISTING_TYPES.find((value) => value === rawType),
    propertyType: PROPERTY_TYPES.find((value) => value === rawPropertyType),
    city: city || undefined,
    q: q || undefined,
    minPrice,
    maxPrice:
      minPrice !== undefined && maxPrice !== undefined && maxPrice < minPrice ? minPrice : maxPrice,
  }
}

function priceConstraint(filters: ListingFilters, fieldName: 'salePrice' | 'rentPrice'): Where {
  const conditions: Record<string, number> = {}
  if (filters.minPrice !== undefined) {
    conditions.greater_than_equal = filters.minPrice
  }
  if (filters.maxPrice !== undefined) {
    conditions.less_than_equal = filters.maxPrice
  }

  return { [fieldName]: conditions } as Where
}

function buildListingWhere(tenantDocId: number, filters: ListingFilters): Where {
  const clauses: Where[] = [
    { tenant: { equals: tenantDocId } },
    { status: { equals: 'published' } },
  ]

  if (filters.listingType) {
    clauses.push({ listingType: { equals: filters.listingType } })
  }

  if (filters.propertyType) {
    clauses.push({ propertyType: { equals: filters.propertyType } })
  }

  if (filters.city) {
    clauses.push({ 'address.city': { like: filters.city } })
  }

  if (filters.q) {
    clauses.push({
      or: [
        { title: { like: filters.q } },
        { description: { like: filters.q } },
        { 'address.city': { like: filters.q } },
        { 'address.neighborhood': { like: filters.q } },
      ],
    })
  }

  if (filters.minPrice !== undefined || filters.maxPrice !== undefined) {
    if (filters.listingType === 'sale') {
      clauses.push(priceConstraint(filters, 'salePrice'))
    } else if (filters.listingType === 'rent') {
      clauses.push(priceConstraint(filters, 'rentPrice'))
    } else {
      // Apply the range to the price field that matches each listing type.
      clauses.push({
        or: [
          {
            and: [
              { listingType: { equals: 'sale' } },
              priceConstraint(filters, 'salePrice'),
            ],
          },
          {
            and: [
              { listingType: { equals: 'rent' } },
              priceConstraint(filters, 'rentPrice'),
            ],
          },
        ],
      })
    }
  }

  return { and: clauses }
}

async function queryListingSearch(
  tenantDocId: number,
  filters: ListingFilters,
): Promise<ListingSearchResult> {
  const payload = await getPayloadClient()
  const result = await payload.find({
    collection: 'listings',
    depth: 0,
    limit: LISTINGS_PER_PAGE,
    overrideAccess: true,
    page: filters.page,
    sort: '-publishedAt',
    where: buildListingWhere(tenantDocId, filters),
  })

  return {
    docs: result.docs,
    totalDocs: result.totalDocs,
    totalPages: result.totalPages,
    page: result.page ?? filters.page,
    hasNextPage: result.hasNextPage,
    hasPrevPage: result.hasPrevPage,
  }
}

export async function searchListings(
  context: SiteContext,
  filters: ListingFilters,
): Promise<ListingSearchResult> {
  const cacheKey = JSON.stringify(filters)
  return unstable_cache(
    async () => queryListingSearch(context.tenant.id, filters),
    ['site-listings-search-v1', context.slug, cacheKey],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [siteCacheTag(context.slug), siteListingsCacheTag(context.slug)],
    },
  )()
}

async function queryLatestListings(tenantDocId: number, limit: number): Promise<Listing[]> {
  const payload = await getPayloadClient()
  const result = await payload.find({
    collection: 'listings',
    depth: 0,
    limit,
    overrideAccess: true,
    pagination: false,
    sort: '-publishedAt',
    where: {
      and: [{ tenant: { equals: tenantDocId } }, { status: { equals: 'published' } }],
    },
  })

  return result.docs
}

export async function getLatestListings(context: SiteContext, limit = 6): Promise<Listing[]> {
  const safeLimit = Math.min(Math.max(limit, 1), 24)
  return unstable_cache(
    async () => queryLatestListings(context.tenant.id, safeLimit),
    ['site-listings-latest-v1', context.slug, String(safeLimit)],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [siteCacheTag(context.slug), siteListingsCacheTag(context.slug)],
    },
  )()
}

export interface CollectionSummary {
  collection: ListingCollectionDefinition
  totalDocs: number
  docs: Listing[]
}

async function queryCollectionSummaries(tenantDocId: number): Promise<CollectionSummary[]> {
  const payload = await getPayloadClient()

  return Promise.all(
    LISTING_COLLECTIONS.map(async (collection) => {
      const result = await payload.find({
        collection: 'listings',
        depth: 0,
        limit: 3,
        overrideAccess: true,
        page: 1,
        sort: '-publishedAt',
        where: buildListingWhere(tenantDocId, {
          page: 1,
          ...collectionFilters(collection),
        }),
      })

      return { collection, totalDocs: result.totalDocs, docs: result.docs }
    }),
  )
}

/**
 * Totais + amostra de cada coleção. A home usa somente as coleções com
 * estoque; nenhuma seção é hardcoded quando a imobiliária não tem aquele tipo.
 */
export async function getCollectionSummaries(context: SiteContext): Promise<CollectionSummary[]> {
  return unstable_cache(
    async () => queryCollectionSummaries(context.tenant.id),
    ['site-collections-v1', context.slug],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [siteCacheTag(context.slug), siteListingsCacheTag(context.slug)],
    },
  )()
}

async function queryListingBySlug(
  tenantDocId: number,
  seoSlug: string,
): Promise<Listing | null> {
  const payload = await getPayloadClient()
  const result = await payload.find({
    collection: 'listings',
    depth: 0,
    limit: 1,
    overrideAccess: true,
    pagination: false,
    where: {
      and: [
        { tenant: { equals: tenantDocId } },
        { seoSlug: { equals: seoSlug } },
        { status: { equals: 'published' } },
      ],
    },
  })

  return result.docs[0] ?? null
}

export async function getPublishedListingBySlug(
  context: SiteContext,
  seoSlug: string,
): Promise<Listing | null> {
  return unstable_cache(
    async () => queryListingBySlug(context.tenant.id, seoSlug),
    ['site-listing-detail-v1', context.slug, seoSlug],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [
        siteCacheTag(context.slug),
        siteListingsCacheTag(context.slug),
        siteListingCacheTag(context.slug, seoSlug),
      ],
    },
  )()
}

async function queryPublishedPage(
  tenantDocId: number,
  pageKey: Page['pageKey'],
): Promise<Page | null> {
  const payload = await getPayloadClient()
  const result = await payload.find({
    collection: 'pages',
    depth: 1,
    limit: 1,
    overrideAccess: true,
    pagination: false,
    where: {
      and: [
        { tenant: { equals: tenantDocId } },
        { pageKey: { equals: pageKey } },
        { status: { equals: 'published' } },
      ],
    },
  })

  return result.docs[0] ?? null
}

export async function getPublishedPage(
  context: SiteContext,
  pageKey: Page['pageKey'],
): Promise<Page | null> {
  return unstable_cache(
    async () => queryPublishedPage(context.tenant.id, pageKey),
    ['site-page-v1', context.slug, pageKey],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [
        siteCacheTag(context.slug),
        sitePagesCacheTag(context.slug),
        sitePageCacheTag(context.slug, pageKey),
      ],
    },
  )()
}

async function querySitemapListings(tenantDocId: number): Promise<SitemapListing[]> {
  const payload = await getPayloadClient()
  const listings: SitemapListing[] = []
  let page = 1
  let hasNextPage = true

  while (hasNextPage && listings.length < SITEMAP_MAX_LISTINGS) {
    const result = await payload.find({
      collection: 'listings',
      depth: 0,
      limit: SITEMAP_BATCH_SIZE,
      overrideAccess: true,
      page,
      pagination: true,
      select: {
        seoSlug: true,
        updatedAt: true,
      },
      sort: '-updatedAt',
      where: {
        and: [{ tenant: { equals: tenantDocId } }, { status: { equals: 'published' } }],
      },
    })

    for (const doc of result.docs) {
      if (doc.seoSlug && doc.updatedAt) {
        listings.push({ seoSlug: doc.seoSlug, updatedAt: doc.updatedAt })
      }
    }

    hasNextPage = result.hasNextPage
    page += 1
  }

  return listings.slice(0, SITEMAP_MAX_LISTINGS)
}

export async function getSitemapListings(context: SiteContext): Promise<SitemapListing[]> {
  return unstable_cache(
    async () => querySitemapListings(context.tenant.id),
    ['site-sitemap-listings-v1', context.slug],
    {
      revalidate: SITE_CACHE_SECONDS,
      tags: [siteCacheTag(context.slug), siteListingsCacheTag(context.slug)],
    },
  )()
}

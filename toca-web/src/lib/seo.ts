import type { Metadata } from 'next'
import type { Listing, Page, Tenant } from '@/payload-types'
import {
  activeListingPrice,
  listingLocation,
  pageOgImageUrl,
  primaryListingImage,
  resolveSiteAssetUrl,
  tenantCoverUrl,
  tenantDisplayName,
  tenantLogoUrl,
  tenantTagline,
  truncateMeta,
} from './format'
import { listingPath } from './site-host'
import { SITE_NOINDEX_ROBOTS, siteIndexingDisabled } from './indexing'
import type { SiteContext } from './site'

function fullPageTitle(context: SiteContext, rawTitle: string): string {
  const name = tenantDisplayName(context.tenant)
  return rawTitle === name ? rawTitle : `${rawTitle} | ${name}`
}

function defaultDescription(tenant: Tenant): string {
  return `${tenantTagline(tenant)}. Imóveis selecionados e atendimento próximo com ${tenantDisplayName(tenant)}.`
}

export function buildTenantMetadata(context: SiteContext): Metadata {
  const { tenant, origin } = context
  const logo = tenantLogoUrl(tenant, origin)
  const cover = tenantCoverUrl(tenant, origin)
  const description = defaultDescription(tenant)
  const name = tenantDisplayName(tenant)

  return {
    metadataBase: new URL(origin),
    applicationName: name,
    title: {
      default: `${name} | Imóveis`,
      template: `%s | ${name}`,
    },
    description,
    openGraph: {
      type: 'website',
      locale: 'pt_BR',
      siteName: name,
      title: `${name} | Imóveis`,
      description,
      url: origin,
      ...(cover || logo ? { images: [{ url: cover || logo, alt: name }] } : {}),
    },
    robots: siteIndexingDisabled()
      ? SITE_NOINDEX_ROBOTS
      : {
          index: true,
          follow: true,
          googleBot: { index: true, follow: true },
        },
    twitter: {
      card: 'summary_large_image',
      title: `${name} | Imóveis`,
      description,
      ...(cover || logo ? { images: [cover || logo] } : {}),
    },
    icons: logo
      ? [
          { rel: 'icon', url: logo },
          { rel: 'apple-touch-icon', url: logo },
        ]
      : [{ rel: 'icon', type: 'image/svg+xml', url: '/favicon.svg' }],
    other: {
      'og:site_name': name,
    },
  }
}

interface PageMetadataArgs {
  context: SiteContext
  page: Page | null
  pathname: string
  fallbackTitle: string
  fallbackDescription?: string
}

export function buildPageMetadata({
  context,
  page,
  pathname,
  fallbackTitle,
  fallbackDescription,
}: PageMetadataArgs): Metadata {
  const customTitle = page?.seo?.metaTitle?.trim()
  const title = customTitle || page?.title?.trim() || fallbackTitle
  const description = truncateMeta(
    page?.seo?.metaDescription?.trim() ||
      page?.subtitle?.trim() ||
      fallbackDescription ||
      defaultDescription(context.tenant),
    170,
  )
  const canonical = `${context.origin}${pathname}`
  const image =
    pageOgImageUrl(page, context.tenant, context.origin) ??
    tenantLogoUrl(context.tenant, context.origin)
  const noindex = siteIndexingDisabled() || page?.seo?.noindex === true
  const openGraphTitle = fullPageTitle(context, title)

  return {
    // Um meta title explícito no CMS é o título final: não concatenamos a marca
    // de novo, senão 70 caracteres + nome da imobiliária estouram a SERP.
    title: customTitle ? { absolute: customTitle } : title,
    description,
    alternates: {
      canonical,
    },
    robots: noindex
      ? { index: false, follow: false, googleBot: { index: false, follow: false } }
      : { index: true, follow: true },
    openGraph: {
      type: 'website',
      locale: 'pt_BR',
      siteName: tenantDisplayName(context.tenant),
      title: openGraphTitle,
      description,
      url: canonical,
      ...(image ? { images: [{ url: image, alt: tenantDisplayName(context.tenant) }] } : {}),
    },
    twitter: {
      card: 'summary_large_image',
      title: openGraphTitle,
      description,
      ...(image ? { images: [image] } : {}),
    },
    other: {
      'og:site_name': tenantDisplayName(context.tenant),
    },
  }
}

export function buildListingMetadata(context: SiteContext, listing: Listing): Metadata {
  const title = `${listing.title} | ${tenantDisplayName(context.tenant)}`
  const description = truncateMeta(listing.description, 170)
  const pathname = listingPath(listing.seoSlug)
  const canonical = `${context.origin}${pathname}`
  const rawImage = primaryListingImage(listing)
  const image =
    resolveSiteAssetUrl(rawImage, context.origin) ??
    tenantLogoUrl(context.tenant, context.origin)
  const location = listingLocation(listing)

  return {
    title: { absolute: title },
    description,
    alternates: {
      canonical,
    },
    robots: siteIndexingDisabled()
      ? SITE_NOINDEX_ROBOTS
      : {
          index: true,
          follow: true,
          googleBot: { index: true, follow: true },
        },
    openGraph: {
      type: 'article',
      locale: 'pt_BR',
      siteName: tenantDisplayName(context.tenant),
      title,
      description: location ? `${description} — ${location}` : description,
      url: canonical,
      ...(image ? { images: [{ url: image, alt: listing.title }] } : {}),
    },
    twitter: {
      card: 'summary_large_image',
      title,
      description,
      ...(image ? { images: [image] } : {}),
    },
    other: {
      'og:site_name': tenantDisplayName(context.tenant),
    },
  }
}

export function buildListingJsonLd(
  context: SiteContext,
  listing: Listing,
): Record<string, unknown> {
  const { tenant, origin } = context
  const price = activeListingPrice(listing)
  const pathname = listingPath(listing.seoSlug)
  const canonical = `${origin}${pathname}`
  const images = (listing.images ?? [])
    .slice()
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
    .map((image) => resolveSiteAssetUrl(image.url, origin))
    .filter((url): url is string => Boolean(url))

  const seller: Record<string, unknown> = {
    '@type': 'RealEstateAgent',
    name: tenantDisplayName(tenant),
    url: origin,
  }
  if (tenant.contact?.phone) {
    seller.telephone = tenant.contact.phone
  }
  if (tenant.contact?.email) {
    seller.email = tenant.contact.email
  }
  const logo = tenantLogoUrl(tenant, origin)
  if (logo) {
    seller.logo = logo
  }

  const address = listing.address
  const addressLocality = address?.city || undefined
  const addressRegion = address?.state || undefined

  const jsonLd: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'RealEstateListing',
    name: listing.title,
    description: truncateMeta(listing.description, 5000),
    url: canonical,
    mainEntityOfPage: canonical,
    identifier: listing.propertyId,
    seller,
    ...(images.length > 0 ? { image: images } : {}),
    ...(listing.publishedAt ? { datePosted: listing.publishedAt } : {}),
    ...(address
      ? {
          address: {
            '@type': 'PostalAddress',
            addressCountry: 'BR',
            ...(listingLocation(listing) ? { name: listingLocation(listing) } : {}),
            ...(address.showExactAddress && address.street
              ? { streetAddress: [address.street, address.number].filter(Boolean).join(', ') }
              : {}),
            ...(addressLocality ? { addressLocality } : {}),
            ...(addressRegion ? { addressRegion } : {}),
            ...(address.postalCode ? { postalCode: address.postalCode } : {}),
          },
        }
      : {}),
    ...(listing.features?.bedrooms ? { numberOfRooms: listing.features.bedrooms } : {}),
    ...(listing.features?.bathrooms
      ? { numberOfBathroomsTotal: listing.features.bathrooms }
      : {}),
    ...(listing.features?.areaBuilt || listing.features?.areaTotal
      ? {
          floorSize: {
            '@type': 'QuantitativeValue',
            value: listing.features.areaBuilt || listing.features.areaTotal,
            unitCode: 'MTK',
          },
        }
      : {}),
  }

  if (price !== null) {
    jsonLd.offers = {
      '@type': 'Offer',
      price,
      priceCurrency: 'BRL',
      availability: 'https://schema.org/InStock',
      url: canonical,
      seller,
      ...(listing.listingType === 'rent'
        ? {
            priceSpecification: {
              '@type': 'UnitPriceSpecification',
              price,
              priceCurrency: 'BRL',
              unitCode: 'MON',
            },
          }
        : {}),
    }
  }

  if (listing.updatedAt) {
    jsonLd.dateModified = listing.updatedAt
  }

  return jsonLd
}

/** Dados estruturados globais da vitrine: agência + website + busca. */
export function buildSiteJsonLd(context: SiteContext): Record<string, unknown> {
  const { tenant, origin } = context
  const name = tenantDisplayName(tenant)
  const logo = tenantLogoUrl(tenant, origin)
  const cover = tenantCoverUrl(tenant, origin)
  const organizationId = `${origin}/#organization`
  const websiteId = `${origin}/#website`
  const contact = tenant.contact

  const organization: Record<string, unknown> = {
    '@type': 'RealEstateAgent',
    '@id': organizationId,
    name,
    url: origin,
    ...(logo ? { logo } : {}),
    ...(cover ? { image: cover } : {}),
    description: defaultDescription(tenant),
    ...(contact?.phone ? { telephone: contact.phone } : {}),
    ...(contact?.email ? { email: contact.email } : {}),
    ...(contact?.creci ? { identifier: { '@type': 'PropertyValue', name: 'CRECI', value: contact.creci } } : {}),
    ...(contact?.addressLine || contact?.city || contact?.state
      ? {
          address: {
            '@type': 'PostalAddress',
            addressCountry: 'BR',
            ...(contact?.addressLine ? { streetAddress: contact.addressLine } : {}),
            ...(contact?.city ? { addressLocality: contact.city } : {}),
            ...(contact?.state ? { addressRegion: contact.state } : {}),
          },
        }
      : {}),
  }

  const website: Record<string, unknown> = {
    '@type': 'WebSite',
    '@id': websiteId,
    url: origin,
    name,
    inLanguage: 'pt-BR',
    publisher: { '@id': organizationId },
    potentialAction: {
      '@type': 'SearchAction',
      target: {
        '@type': 'EntryPoint',
        urlTemplate: `${origin}/imoveis?q={search_term_string}`,
      },
      'query-input': 'required name=search_term_string',
    },
  }

  return { '@context': 'https://schema.org', '@graph': [organization, website] }
}

/** BreadcrumbList do anúncio: Início > Imóveis > título. */
export function buildListingBreadcrumbJsonLd(
  context: SiteContext,
  listing: Listing,
): Record<string, unknown> {
  const { origin } = context
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      { '@type': 'ListItem', position: 1, name: 'Início', item: `${origin}/` },
      { '@type': 'ListItem', position: 2, name: 'Imóveis', item: `${origin}/imoveis` },
      {
        '@type': 'ListItem',
        position: 3,
        name: listing.title,
        item: `${origin}${listingPath(listing.seoSlug)}`,
      },
    ],
  }
}

export function serializeJsonLd(value: Record<string, unknown>): string {
  return JSON.stringify(value).replace(/</g, '\\u003c')
}

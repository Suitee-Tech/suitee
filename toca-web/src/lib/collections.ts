import type { Listing } from '@/payload-types'

export type CollectionListingType = Extract<Listing['listingType'], 'sale' | 'rent'>
export type CollectionPropertyType = Extract<
  Listing['propertyType'],
  'apartment' | 'house' | 'land' | 'commercial'
>

export interface ListingCollectionDefinition {
  slug: string
  /** Rótulo curto usado em chips, breadcrumbs e navegação. */
  label: string
  eyebrow: string
  title: string
  description: string
  listingType?: CollectionListingType
  propertyType?: CollectionPropertyType
  kind: 'listing-type' | 'property-type'
}

/**
 * Fonte única das coleções dinâmicas da vitrine. A home só renderiza as
 * coleções com estoque; cada slug também existe como página filtrável.
 */
export const LISTING_COLLECTIONS: readonly ListingCollectionDefinition[] = [
  {
    slug: 'venda',
    label: 'Venda',
    eyebrow: 'Para comprar',
    title: 'Imóveis à venda',
    description:
      'Casas, apartamentos, terrenos e imóveis comerciais para comprar, com atendimento direto da imobiliária.',
    listingType: 'sale',
    kind: 'listing-type',
  },
  {
    slug: 'aluguel',
    label: 'Locação',
    eyebrow: 'Para alugar',
    title: 'Imóveis para alugar',
    description:
      'Opções de locação residencial e comercial para quem procura o próximo endereço.',
    listingType: 'rent',
    kind: 'listing-type',
  },
  {
    slug: 'apartamentos',
    label: 'Apartamentos',
    eyebrow: 'Para morar ou investir',
    title: 'Apartamentos',
    description:
      'Apartamentos para comprar ou alugar, com filtros por cidade, bairro e faixa de preço.',
    propertyType: 'apartment',
    kind: 'property-type',
  },
  {
    slug: 'casas',
    label: 'Casas',
    eyebrow: 'Espaço para a família',
    title: 'Casas',
    description:
      'Casas para comprar ou alugar, com filtros por cidade, bairro e faixa de preço.',
    propertyType: 'house',
    kind: 'property-type',
  },
  {
    slug: 'terrenos',
    label: 'Terrenos',
    eyebrow: 'Para construir',
    title: 'Terrenos',
    description:
      'Terrenos e áreas para construir ou investir, com filtros por cidade e faixa de preço.',
    propertyType: 'land',
    kind: 'property-type',
  },
  {
    slug: 'comerciais',
    label: 'Comerciais',
    eyebrow: 'Para o seu negócio',
    title: 'Imóveis comerciais',
    description:
      'Salas, lojas e imóveis comerciais para comprar ou alugar, com filtros por cidade e preço.',
    propertyType: 'commercial',
    kind: 'property-type',
  },
] as const

export function getListingCollection(slug: string): ListingCollectionDefinition | null {
  return LISTING_COLLECTIONS.find((collection) => collection.slug === slug) ?? null
}

export function collectionHref(collection: ListingCollectionDefinition): string {
  return `/${collection.slug}`
}

export function collectionFilters(
  collection: ListingCollectionDefinition,
): { listingType?: CollectionListingType; propertyType?: CollectionPropertyType } {
  return {
    ...(collection.listingType ? { listingType: collection.listingType } : {}),
    ...(collection.propertyType ? { propertyType: collection.propertyType } : {}),
  }
}

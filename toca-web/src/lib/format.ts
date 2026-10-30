import type { CSSProperties } from 'react'
import type { Listing, Media, Page, Tenant } from '@/payload-types'
import { absoluteSiteUrl } from './site-host'

const brlFormatter = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
  maximumFractionDigits: 0,
})

const numberFormatter = new Intl.NumberFormat('pt-BR')

export const LISTING_TYPE_LABELS: Record<Listing['listingType'], string> = {
  sale: 'Venda',
  rent: 'Locação',
}

export const PROPERTY_TYPE_LABELS: Record<Listing['propertyType'], string> = {
  apartment: 'Apartamento',
  house: 'Casa',
  land: 'Terreno',
  commercial: 'Comercial',
}

export const PAGE_LABELS: Record<Page['pageKey'], string> = {
  home: 'Início',
  about: 'Sobre',
  contact: 'Contato',
}

export function formatCurrency(value: number | null | undefined): string | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return null
  }

  return brlFormatter.format(value)
}

export function formatArea(value: number | null | undefined): string | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return null
  }

  return `${numberFormatter.format(value)} m²`
}

export function listingPrimaryPrice(listing: Listing): string {
  if (listing.listingType === 'rent') {
    const rent = formatCurrency(listing.rentPrice)
    return rent ? `${rent}/mês` : 'Consulte o valor'
  }

  return formatCurrency(listing.salePrice) ?? 'Consulte o valor'
}

export function listingSecondaryPrice(listing: Listing): string | null {
  if (listing.listingType === 'rent') {
    return formatCurrency(listing.salePrice) ? `Venda: ${formatCurrency(listing.salePrice)}` : null
  }

  const rent = formatCurrency(listing.rentPrice)
  return rent ? `Locação: ${rent}/mês` : null
}

export function activeListingPrice(listing: Listing): number | null {
  const value = listing.listingType === 'rent' ? listing.rentPrice : listing.salePrice
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

export function sortedImages(listing: Listing): NonNullable<Listing['images']> {
  return [...(listing.images ?? [])].sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
}

export function primaryListingImage(listing: Listing): string | null {
  return sortedImages(listing).find((image) => Boolean(image.url))?.url ?? null
}

export function listingLocation(listing: Listing, includeExact = false): string {
  const address = listing.address
  if (!address) {
    return ''
  }

  const city = [address.city, address.state].filter(Boolean).join(' - ')
  const base = [address.neighborhood, city].filter(Boolean).join(', ')

  if (includeExact && address.showExactAddress && address.street) {
    const street = [address.street, address.number].filter(Boolean).join(', ')
    return [street, address.neighborhood, city].filter(Boolean).join(' - ')
  }

  return base
}

export function listingFeatureItems(
  listing: Listing,
): Array<{ label: string; value: string }> {
  const features = listing.features
  if (!features) {
    return []
  }

  const items: Array<{ label: string; value: string }> = []
  if (features.bedrooms) items.push({ label: 'Quartos', value: String(features.bedrooms) })
  if (features.suites) items.push({ label: 'Suítes', value: String(features.suites) })
  if (features.bathrooms) items.push({ label: 'Banheiros', value: String(features.bathrooms) })
  if (features.parkingSpaces) items.push({ label: 'Vagas', value: String(features.parkingSpaces) })
  if (features.areaBuilt) items.push({ label: 'Área construída', value: formatArea(features.areaBuilt) ?? '' })
  else if (features.areaTotal) items.push({ label: 'Área total', value: formatArea(features.areaTotal) ?? '' })

  return items.filter((item) => item.value.length > 0)
}

export function paragraphList(text: string | null | undefined): string[] {
  if (!text) {
    return []
  }

  return text
    .split(/\n{2,}/)
    .map((paragraph) => paragraph.trim())
    .filter(Boolean)
}

export function truncateMeta(text: string | null | undefined, max = 160): string {
  if (!text) {
    return ''
  }

  const singleLine = text.replace(/\s+/g, ' ').trim()
  if (singleLine.length <= max) {
    return singleLine
  }

  return `${singleLine.slice(0, Math.max(0, max - 1)).trimEnd()}…`
}

function isAbsoluteUrl(value: string): boolean {
  return /^https?:\/\//i.test(value) || value.startsWith('data:')
}

export function resolveSiteAssetUrl(
  rawUrl: string | null | undefined,
  origin: string,
): string | null {
  if (!rawUrl) {
    return null
  }

  const value = rawUrl.trim()
  if (!value) {
    return null
  }

  if (isAbsoluteUrl(value)) {
    return value
  }

  return absoluteSiteUrl(origin, value.startsWith('/') ? value : `/${value}`)
}

function mediaObject(value: Media | number | null | undefined): Media | null {
  return value && typeof value === 'object' ? value : null
}

export const SUITEE_LOGO = '/suitee-logo.png'
export const SUITEE_LOGO_WHITE = '/suitee-logo-white.png'
export const SUITEE_FALLBACK_NAME = 'Suitee'
export const SUITEE_FALLBACK_TAGLINE = 'Sua plataforma imobiliária'

export function isSuiteeLogoUrl(url: string | null | undefined): boolean {
  return Boolean(url && /\/suitee-logo(?:-white)?\.png(?:\?.*)?$/.test(url))
}

export function tenantDisplayName(tenant: Tenant): string {
  return tenant.name?.trim() || SUITEE_FALLBACK_NAME
}

export function tenantTagline(tenant: Tenant): string {
  return tenant.branding?.tagline?.trim() || SUITEE_FALLBACK_TAGLINE
}

/**
 * Prioridade: logo da imobiliária (URL ou mídia) → logo da Suitee.
 * `variant` escolhe a versão clara/escura para superfícies escuras.
 */
export function tenantLogoUrl(
  tenant: Tenant,
  origin: string,
  variant: 'light' | 'dark' = 'light',
): string {
  const tenantLogo =
    resolveSiteAssetUrl(tenant.branding?.logoUrl, origin) ??
    resolveSiteAssetUrl(mediaObject(tenant.branding?.logo)?.url, origin)
  if (tenantLogo) return tenantLogo

  const fallback = variant === 'dark' ? SUITEE_LOGO_WHITE : SUITEE_LOGO
  return origin ? new URL(fallback, origin).toString() : fallback
}

export function tenantCoverUrl(tenant: Tenant, origin: string): string | null {
  return resolveSiteAssetUrl(tenant.branding?.coverUrl, origin) ??
    resolveSiteAssetUrl(mediaObject(tenant.branding?.cover)?.url, origin)
}

export function pageOgImageUrl(page: Page | null, tenant: Tenant, origin: string): string | null {
  return resolveSiteAssetUrl(mediaObject(page?.seo?.ogImage)?.url, origin) ??
    tenantCoverUrl(tenant, origin)
}

export function formatPhone(phone: string | null | undefined): string {
  if (!phone) {
    return ''
  }

  const digits = phone.replace(/\D/g, '')
  if (digits.length === 10) {
    return `(${digits.slice(0, 2)}) ${digits.slice(2, 6)}-${digits.slice(6)}`
  }
  if (digits.length === 11) {
    return `(${digits.slice(0, 2)}) ${digits.slice(2, 7)}-${digits.slice(7)}`
  }
  if (digits.length === 12 && digits.startsWith('55')) {
    return `+55 (${digits.slice(2, 4)}) ${digits.slice(4, 8)}-${digits.slice(8)}`
  }
  if (digits.length === 13 && digits.startsWith('55')) {
    return `+55 (${digits.slice(2, 4)}) ${digits.slice(4, 9)}-${digits.slice(9)}`
  }

  return phone.trim()
}

export function phoneHref(phone: string | null | undefined): string | null {
  if (!phone) {
    return null
  }

  const digits = phone.replace(/\D/g, '')
  return digits ? `tel:+${digits}` : null
}

export function whatsappHref(phone: string | null | undefined, message?: string): string | null {
  if (!phone) {
    return null
  }

  let digits = phone.replace(/\D/g, '')
  if (!digits) {
    return null
  }
  if (!digits.startsWith('55') && digits.length <= 11) {
    digits = `55${digits}`
  }

  const base = `https://wa.me/${digits}`
  return message?.trim() ? `${base}?text=${encodeURIComponent(message.trim())}` : base
}

export function safeHexColor(value: string | null | undefined, fallback: string): string {
  if (typeof value !== 'string') {
    return fallback
  }

  const trimmed = value.trim()
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) {
    return trimmed.toLowerCase()
  }
  if (/^#[0-9a-fA-F]{3}$/.test(trimmed)) {
    const [r, g, b] = trimmed.slice(1).split('')
    return `#${r}${r}${g}${g}${b}${b}`.toLowerCase()
  }

  return fallback
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  return {
    r: Number.parseInt(hex.slice(1, 3), 16),
    g: Number.parseInt(hex.slice(3, 5), 16),
    b: Number.parseInt(hex.slice(5, 7), 16),
  }
}

function relativeLuminance(hex: string): number {
  const { r, g, b } = hexToRgb(hex)
  const channels = [r, g, b].map((value) => {
    const channel = value / 255
    return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })

  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

function contrastInk(hex: string): '#ffffff' | '#0b1220' {
  const background = relativeLuminance(hex)
  const dark = 0.006 // #0b1220
  const whiteContrast = (1.05) / (background + 0.05)
  const darkContrast = (background + 0.05) / (dark + 0.05)
  return whiteContrast >= darkContrast ? '#ffffff' : '#0b1220'
}

export type BrandingStyle = CSSProperties & {
  '--brand-primary': string
  '--brand-primary-ink': string
  '--brand-primary-soft': string
  '--brand-primary-text': string
  '--brand-primary-deep': string
  '--brand-accent': string
  '--brand-accent-ink': string
  '--brand-accent-text': string
  '--brand-accent-on-dark': string
  '--brand-accent-on-primary': string
  '--brand-accent-soft': string
  '--brand-focus': string
}

function toHex(value: number): string {
  return Math.max(0, Math.min(255, Math.round(value))).toString(16).padStart(2, '0')
}

function mixHex(from: string, to: string, weight: number): string {
  const a = hexToRgb(from)
  const b = hexToRgb(to)
  const w = Math.max(0, Math.min(1, weight))
  return `#${toHex(a.r + (b.r - a.r) * w)}${toHex(a.g + (b.g - a.g) * w)}${toHex(a.b + (b.b - a.b) * w)}`
}

function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  const light = Math.max(la, lb)
  const dark = Math.min(la, lb)
  return (light + 0.05) / (dark + 0.05)
}

/**
 * Mantém a cor da marca como identidade, mas garante legibilidade sobre o
 * fundo real do site. Não troca o tom por preto/branco de imediato: mistura
 * apenas o necessário para chegar ao contraste mínimo, preservando a marca.
 */
function readableOn(foreground: string, background: string, minimum = 4.5): string {
  if (contrastRatio(foreground, background) >= minimum) return foreground
  const target = relativeLuminance(background) > 0.5 ? '#000000' : '#ffffff'
  for (let step = 5; step <= 100; step += 5) {
    const candidate = mixHex(foreground, target, step / 100)
    if (contrastRatio(candidate, background) >= minimum) return candidate
  }
  return target
}

export function brandingStyle(tenant: Tenant): {
  dataTheme: 'light' | 'dark'
  style: BrandingStyle
} {
  const primary = safeHexColor(tenant.branding?.primaryColor, '#0f172a')
  const accent = safeHexColor(tenant.branding?.accentColor, '#f59e0b')
  const dataTheme = tenant.branding?.theme === 'dark' ? 'dark' : 'light'
  const paper = dataTheme === 'dark' ? '#0d1511' : '#f7f4ee'
  const { r, g, b } = hexToRgb(primary)
  const accentRgb = hexToRgb(accent)

  return {
    dataTheme,
    style: {
      '--brand-primary': primary,
      '--brand-primary-ink': contrastInk(primary),
      '--brand-primary-soft': `rgba(${r}, ${g}, ${b}, 0.08)`,
      '--brand-primary-text': readableOn(primary, paper),
      '--brand-primary-deep': readableOn(primary, '#ffffff', 4.5),
      '--brand-accent': accent,
      '--brand-accent-ink': contrastInk(accent),
      '--brand-accent-text': readableOn(accent, paper),
      // O hero e o CTA têm fundo escuro próprio, independente do tema.
      '--brand-accent-on-dark': readableOn(accent, '#0b1511', 4.5),
      '--brand-accent-on-primary': readableOn(accent, primary, 4.5),
      '--brand-accent-soft': `rgba(${accentRgb.r}, ${accentRgb.g}, ${accentRgb.b}, 0.14)`,
      '--brand-focus': readableOn(accent, paper, 3),
    },
  }
}

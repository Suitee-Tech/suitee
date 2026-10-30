import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { LeadForm } from '@/components/site/LeadForm'
import { ListingGallery } from '@/components/site/ListingGallery'
import { ShareButton } from '@/components/site/ShareButton'
import { FavoriteButton } from '@/components/site/FavoriteButton'
import { ViewBeacon } from '@/components/site/ViewBeacon'
import {
  AreaIcon,
  ArrowLeftIcon,
  ArrowIcon,
  BathIcon,
  BedIcon,
  CarIcon,
  MailIcon,
  PhoneIcon,
  PinIcon,
  WhatsAppIcon,
} from '@/components/site/icons'
import { getPublishedListingBySlug } from '@/lib/data'
import {
  formatArea,
  formatCurrency,
  formatPhone,
  listingFeatureItems,
  listingLocation,
  LISTING_TYPE_LABELS,
  listingPrimaryPrice,
  listingSecondaryPrice,
  paragraphList,
  phoneHref,
  tenantDisplayName,
  tenantLogoUrl,
  PROPERTY_TYPE_LABELS,
  sortedImages,
  whatsappHref,
} from '@/lib/format'
import {
  buildListingBreadcrumbJsonLd,
  buildListingJsonLd,
  buildListingMetadata,
  serializeJsonLd,
} from '@/lib/seo'
import { getSiteContext, tenantParamMatches } from '@/lib/site'

export const dynamic = 'force-dynamic'

type DetailParams = {
  params: Promise<{ tenant: string; seoSlug: string }>
}

function factIcon(label: string) {
  if (label === 'Quartos' || label === 'Suítes') return <BedIcon width={20} height={20} />
  if (label === 'Banheiros') return <BathIcon width={20} height={20} />
  if (label === 'Vagas') return <CarIcon width={20} height={20} />
  return <AreaIcon width={20} height={20} />
}

export async function generateMetadata({ params }: DetailParams): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  const { tenant, seoSlug } = await params
  if (!tenantParamMatches(context, tenant)) {
    return { robots: { index: false, follow: false } }
  }

  const listing = await getPublishedListingBySlug(context, seoSlug)
  if (!listing) {
    return {
      title: 'Imóvel não encontrado',
      robots: { index: false, follow: false },
    }
  }

  return buildListingMetadata(context, listing)
}

export default async function ListingDetailPage({ params }: DetailParams) {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const { tenant, seoSlug } = await params
  if (!tenantParamMatches(context, tenant)) {
    notFound()
  }

  const listing = await getPublishedListingBySlug(context, seoSlug)
  if (!listing) {
    notFound()
  }

  const { tenant: agency } = context
  const agencyName = tenantDisplayName(agency)
  const agencyLogoUrl = tenantLogoUrl(agency, context.origin)
  const images = sortedImages(listing)
  const facts = listingFeatureItems(listing)
  const description = paragraphList(listing.description)
  const amenities = (listing.amenities ?? []).filter(Boolean)
  const location = listingLocation(listing, true)
  const secondaryPrice = listingSecondaryPrice(listing)
  const phone = agency.contact?.phone?.trim()
  const phoneLink = phoneHref(phone)
  const whatsappMessage = `Olá! Vi o imóvel "${listing.title}" no site de ${agency.name} e gostaria de mais informações: ${context.origin}/imoveis/${listing.seoSlug}`
  const whatsapp = whatsappHref(agency.contact?.whatsapp || phone, whatsappMessage)
  const email = agency.contact?.email?.trim()
  const creci = agency.contact?.creci?.trim()
  const condo = formatCurrency(listing.condoFee)
  const tax = formatCurrency(listing.propertyTax)
  const totalArea = formatArea(listing.features?.areaTotal)
  const builtArea = formatArea(listing.features?.areaBuilt)
  const jsonLd = buildListingJsonLd(context, listing)
  const breadcrumbJsonLd = buildListingBreadcrumbJsonLd(context, listing)

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }}
      />
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: serializeJsonLd(breadcrumbJsonLd) }}
      />
      <ViewBeacon propertyId={listing.propertyId} siteSlug={context.slug} />

      <div className="site-container detail-breadcrumbs">
        <nav className="breadcrumbs" aria-label="Navegação">
          <Link href="/">Início</Link>
          <span>/</span>
          <Link href="/imoveis">Imóveis</Link>
          <span>/</span>
          <span aria-current="page">{listing.title}</span>
        </nav>
        <div className="detail-breadcrumbs__actions">
          <Link href="/imoveis" className="text-link">
            <ArrowLeftIcon width={17} height={17} />
            Voltar para imóveis
          </Link>
          <FavoriteButton
            tenant={context.slug}
            variant="detail"
            listing={{
              id: String(listing.id),
              slug: listing.seoSlug,
              title: listing.title,
              price: listingPrimaryPrice(listing),
              location: location || undefined,
              imageUrl: images[0]?.url || undefined,
              listingType: listing.listingType,
              savedAt: 0,
            }}
          />
          <ShareButton title={listing.title} />
        </div>
      </div>

      <section className="site-container listing-detail">
        <div className="listing-detail__gallery">
          <ListingGallery
            title={listing.title}
            images={images.map((image) => ({
              url: image.url,
              alt: image.alt || listing.title,
              caption: image.caption,
            }))}
          />
        </div>

        <div className="listing-detail__layout">
          <div className="listing-detail__main">
            <div className="listing-detail__badges">
              <span className="badge badge--solid">
                {LISTING_TYPE_LABELS[listing.listingType]}
              </span>
              <span className="badge">{PROPERTY_TYPE_LABELS[listing.propertyType]}</span>
              {listing.status === 'published' ? <span className="badge">Disponível</span> : null}
            </div>

            <h1>{listing.title}</h1>
            {location ? (
              <p className="listing-detail__location">
                <PinIcon width={18} height={18} />
                {location}
              </p>
            ) : null}

            <div className="listing-detail__price-row">
              <div>
                <span className="listing-detail__price">{listingPrimaryPrice(listing)}</span>
                {secondaryPrice ? <span className="listing-detail__subprice">{secondaryPrice}</span> : null}
              </div>
              <div className="listing-detail__actions">
                {whatsapp ? (
                  <a
                    className="button button--accent"
                    href={whatsapp}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <WhatsAppIcon width={18} height={18} />
                    Falar no WhatsApp
                  </a>
                ) : (
                  <a className="button button--primary" href="#interesse">
                    Tenho interesse
                    <ArrowIcon width={18} height={18} />
                  </a>
                )}
                {whatsapp ? (
                  <a
                    className="button button--primary listing-detail__contact-cta"
                    href="#interesse"
                  >
                    <MailIcon width={18} height={18} />
                    Falar com a imobiliária
                  </a>
                ) : null}
              </div>
            </div>

            {facts.length > 0 ? (
              <div className="detail-facts">
                {facts.map((fact) => (
                  <div key={fact.label} className="detail-facts__item">
                    <span className="detail-facts__icon">{factIcon(fact.label)}</span>
                    <span>
                      <strong>{fact.value}</strong>
                      <span>{fact.label}</span>
                    </span>
                  </div>
                ))}
              </div>
            ) : null}

            <section className="detail-section">
              <p className="eyebrow">Sobre este imóvel</p>
              <h2>Detalhes que fazem a diferença</h2>
              {description.length > 0 ? (
                <div className="prose-plain">
                  {description.map((paragraph, index) => (
                    <p key={`${paragraph.slice(0, 24)}-${index}`}>{paragraph}</p>
                  ))}
                </div>
              ) : (
                <p className="muted">A descrição deste imóvel será publicada em breve.</p>
              )}
            </section>

            {amenities.length > 0 ? (
              <section className="detail-section">
                <p className="eyebrow">Comodidades</p>
                <h2>O que este imóvel oferece</h2>
                <ul className="amenities">
                  {amenities.map((amenity) => (
                    <li key={amenity}>{amenity}</li>
                  ))}
                </ul>
              </section>
            ) : null}

            <section className="detail-section">
              <p className="eyebrow">Localização</p>
              <h2>Onde fica este imóvel</h2>
              <div className="address-card">
                <span className="address-card__icon">
                  <PinIcon width={22} height={22} />
                </span>
                <div>
                  <strong>
                    {listing.address?.showExactAddress && listing.address?.street
                      ? listingLocation(listing, true)
                      : [listing.address?.neighborhood, listing.address?.city]
                          .filter(Boolean)
                          .join(', ') || 'Localização sob consulta'}
                  </strong>
                  <p>
                    {[listing.address?.city, listing.address?.state]
                      .filter(Boolean)
                      .join(' - ') || 'Fale com a imobiliária para mais detalhes.'}
                    {listing.address?.showExactAddress && listing.address?.postalCode
                      ? ` · CEP ${listing.address.postalCode}`
                      : ''}
                  </p>
                  {!listing.address?.showExactAddress ? (
                    <span className="address-card__note">
                      O endereço exato é informado no atendimento com a imobiliária.
                    </span>
                  ) : null}
                </div>
              </div>
            </section>

            <section className="detail-section">
              <p className="eyebrow">Valores e medidas</p>
              <h2>Resumo do anúncio</h2>
              <dl className="summary-table">
                <div>
                  <dt>Preço</dt>
                  <dd>{listingPrimaryPrice(listing)}</dd>
                </div>
                {condo ? (
                  <div>
                    <dt>Condomínio</dt>
                    <dd>{condo}/mês</dd>
                  </div>
                ) : null}
                {tax ? (
                  <div>
                    <dt>IPTU</dt>
                    <dd>{tax}</dd>
                  </div>
                ) : null}
                {totalArea ? (
                  <div>
                    <dt>Área total</dt>
                    <dd>{totalArea}</dd>
                  </div>
                ) : null}
                {builtArea ? (
                  <div>
                    <dt>Área construída</dt>
                    <dd>{builtArea}</dd>
                  </div>
                ) : null}
              </dl>
            </section>

          </div>

          <aside className="listing-detail__aside" id="interesse">
            <div className="lead-form-card">
              <LeadForm
                propertyId={listing.propertyId}
                siteSlug={context.slug}
                propertyTitle={listing.title}
              />
            </div>

            <div className="agent-card">
              <p className="eyebrow">Anunciado por</p>
              {agencyLogoUrl ? (
                <img className="agent-card__logo" src={agencyLogoUrl} alt={agencyName} />
              ) : null}
              <h2>{agencyName}</h2>
              <p>
                Fale direto com a imobiliária responsável por este imóvel. Atendimento sem
                intermediários.
              </p>
              {creci ? <p className="agent-card__creci">CRECI {creci}</p> : null}
              <div className="agent-card__links">
                {phoneLink ? (
                  <a href={phoneLink}>
                    <PhoneIcon width={17} height={17} />
                    {formatPhone(phone)}
                  </a>
                ) : null}
                {whatsapp ? (
                  <a href={whatsapp} target="_blank" rel="noreferrer">
                    <WhatsAppIcon width={17} height={17} />
                    WhatsApp
                  </a>
                ) : null}
                {email ? (
                  <a href={`mailto:${email}`}>
                    <MailIcon width={17} height={17} />
                    {email}
                  </a>
                ) : null}
              </div>
            </div>



            <p className="listing-detail__disclaimer">
              Os dados do imóvel são fornecidos pela imobiliária. Confirme valores e disponibilidade
              no atendimento.
            </p>
          </aside>
        </div>
      </section>
    </>
  )
}

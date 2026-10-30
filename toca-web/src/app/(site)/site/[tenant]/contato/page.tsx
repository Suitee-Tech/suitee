import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { PlainText } from '@/components/site/PlainText'
import { RichText } from '@/components/site/RichText'
import {
  ArrowIcon,
  MailIcon,
  PhoneIcon,
  PinIcon,
  WhatsAppIcon,
} from '@/components/site/icons'
import { getPublishedPage } from '@/lib/data'
import {
  formatPhone,
  phoneHref,
  tenantCoverUrl,
  tenantDisplayName,
  whatsappHref,
} from '@/lib/format'
import { buildPageMetadata } from '@/lib/seo'
import { getSiteContext } from '@/lib/site'

export const dynamic = 'force-dynamic'

export async function generateMetadata(): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  const page = await getPublishedPage(context, 'contact')
  return buildPageMetadata({
    context,
    page,
    pathname: '/contato',
    fallbackTitle: 'Contato',
    fallbackDescription: `Fale com a equipe de ${context.tenant.name}.`,
  })
}

export default async function ContactPage() {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const page = await getPublishedPage(context, 'contact')
  const cover = tenantCoverUrl(context.tenant, context.origin)
  const name = tenantDisplayName(context.tenant)
  const contact = context.tenant.contact
  const phone = contact?.phone?.trim()
  const phoneLink = phoneHref(phone)
  const email = contact?.email?.trim()
  const whatsapp = whatsappHref(contact?.whatsapp || phone)
  const address = [contact?.addressLine, contact?.city, contact?.state].filter(Boolean).join(' · ')
  const title = page?.title?.trim() || 'Vamos conversar'
  const subtitle =
    page?.subtitle?.trim() ||
    `Tire dúvidas, fale sobre um imóvel ou conte o que você procura. A equipe de ${name} está pronta para ajudar.`

  return (
    <>
      <section className={cover ? 'page-hero page-hero--compact page-hero--image' : 'page-hero page-hero--compact'}>
        {cover ? (
          <div className="page-hero__media" aria-hidden="true">
            <img src={cover} alt="" fetchPriority="high" />
          </div>
        ) : null}
        <div className="site-container page-hero__content">
          <nav className="breadcrumbs" aria-label="Navegação">
            <Link href="/">Início</Link>
            <span>/</span>
            <span aria-current="page">Contato</span>
          </nav>
          <p className="eyebrow">Fale com a gente</p>
          <h1>{title}</h1>
          <p className="page-hero__lead">{subtitle}</p>
        </div>
      </section>

      <section className="section">
        <div className="site-container contact-layout">
          <div className="contact-details">
            <p className="eyebrow">Canais de atendimento</p>
            <h2>Escolha o jeito mais fácil de falar com a imobiliária</h2>

            <div className="contact-list">
              {whatsapp ? (
                <a className="contact-card contact-card--accent" href={whatsapp} target="_blank" rel="noreferrer">
                  <span className="contact-card__icon">
                    <WhatsAppIcon width={22} height={22} />
                  </span>
                  <span>
                    <strong>WhatsApp</strong>
                    <span>{formatPhone(contact?.whatsapp || phone)}</span>
                  </span>
                  <ArrowIcon width={18} height={18} />
                </a>
              ) : null}

              {phoneLink ? (
                <a className="contact-card" href={phoneLink}>
                  <span className="contact-card__icon">
                    <PhoneIcon width={22} height={22} />
                  </span>
                  <span>
                    <strong>Telefone</strong>
                    <span>{formatPhone(phone)}</span>
                  </span>
                  <ArrowIcon width={18} height={18} />
                </a>
              ) : null}

              {email ? (
                <a className="contact-card" href={`mailto:${email}`}>
                  <span className="contact-card__icon">
                    <MailIcon width={22} height={22} />
                  </span>
                  <span>
                    <strong>E-mail</strong>
                    <span>{email}</span>
                  </span>
                  <ArrowIcon width={18} height={18} />
                </a>
              ) : null}

              {address ? (
                <div className="contact-card">
                  <span className="contact-card__icon">
                    <PinIcon width={22} height={22} />
                  </span>
                  <span>
                    <strong>Endereço</strong>
                    <span>{address}</span>
                  </span>
                </div>
              ) : null}
            </div>

            {!whatsapp && !phoneLink && !email && !address ? (
              <div className="empty-state empty-state--compact">
                <h3>Atendimento pelo site</h3>
                <p>
                  Escolha um imóvel e envie seu interesse no formulário do anúncio. A equipe
                  receberá seus dados e entrará em contato.
                </p>
                <Link href="/imoveis" className="button button--primary">
                  Ver imóveis
                </Link>
              </div>
            ) : null}

            {page?.body || page?.bodyRich ? (
              <div className="contact-body">
                {page.body ? <PlainText text={page.body} /> : <RichText data={page.bodyRich} />}
              </div>
            ) : null}
          </div>

          <aside className="contact-aside">
            <p className="eyebrow">Interesse em um imóvel?</p>
            <h2>Envie uma mensagem pelo anúncio</h2>
            <p>
              Cada página de imóvel tem um formulário direto para a imobiliária. Escolha o que você
              gostou e conte o que precisa saber.
            </p>
            <Link href="/imoveis" className="button button--primary">
              Encontrar meu imóvel
              <ArrowIcon width={18} height={18} />
            </Link>
            <div className="contact-aside__note">
              <strong>Atendimento consultivo</strong>
              <span>Sem robôs, sem pressão. Você fala com gente de verdade.</span>
            </div>
          </aside>
        </div>
      </section>
    </>
  )
}

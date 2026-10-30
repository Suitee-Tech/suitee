import Link from 'next/link'
import type { Tenant } from '@/payload-types'
import {
  formatPhone,
  phoneHref,
  tenantDisplayName,
  tenantLogoUrl,
  tenantTagline,
  whatsappHref,
} from '@/lib/format'
import { MailIcon, PhoneIcon, PinIcon, WhatsAppIcon } from './icons'
import { BrandMark } from './BrandMark'

interface SiteFooterProps {
  tenant: Tenant
  portalUrl?: string
}

export function SiteFooter({ tenant, portalUrl }: SiteFooterProps) {
  const logoUrl = tenantLogoUrl(tenant, '', 'dark')
  const name = tenantDisplayName(tenant)
  const email = tenant.contact?.email?.trim()
  const phone = tenant.contact?.phone?.trim()
  const phoneLink = phoneHref(phone)
  const whatsappLink = whatsappHref(tenant.contact?.whatsapp || phone)
  const address = [tenant.contact?.addressLine, tenant.contact?.city, tenant.contact?.state]
    .filter(Boolean)
    .join(' · ')
  const creci = tenant.contact?.creci?.trim()

  return (
    <footer className="site-footer">
      <div className="site-container site-footer__grid">
        <div className="site-footer__brand">
          <BrandMark
            tenant={tenant}
            logoUrl={logoUrl}
            fallbackLogoUrl="/suitee-logo-white.png"
            compact
          />
          <p>
            {tenantTagline(tenant)}
          </p>
          {whatsappLink ? (
            <a className="button button--accent" href={whatsappLink} target="_blank" rel="noreferrer">
              <WhatsAppIcon width={18} height={18} />
              Falar no WhatsApp
            </a>
          ) : null}
        </div>

        <div className="site-footer__column">
          <h2>Navegar</h2>
          <Link href="/">Início</Link>
          <Link href="/imoveis">Imóveis</Link>
          <Link href="/venda">Comprar</Link>
          <Link href="/aluguel">Alugar</Link>
          <Link href="/sobre">Sobre</Link>
          <Link href="/contato">Contato</Link>
          <Link href="/favoritos">Favoritos</Link>
          {portalUrl ? <a href={portalUrl}>Área do cliente</a> : null}
        </div>

        <div className="site-footer__column site-footer__contact">
          <h2>Atendimento</h2>
          {phoneLink ? (
            <a href={phoneLink}>
              <PhoneIcon width={16} height={16} />
              {formatPhone(phone)}
            </a>
          ) : null}
          {email ? (
            <a href={`mailto:${email}`}>
              <MailIcon width={16} height={16} />
              {email}
            </a>
          ) : null}
          {address ? (
            <span>
              <PinIcon width={16} height={16} />
              {address}
            </span>
          ) : null}
          {creci ? <span className="site-footer__creci">CRECI {creci}</span> : null}
          {!phoneLink && !email && !address ? (
            <p className="site-footer__muted">
              Fale com a nossa equipe pelo formulário de interesse em qualquer imóvel.
            </p>
          ) : null}
        </div>
      </div>

      <div className="site-container site-footer__bottom">
        <p>
          © {new Date().getFullYear()} {name}. Todos os direitos reservados.
        </p>
        <p>
          As informações dos imóveis são de responsabilidade da imobiliária.
          {creci ? ` · CRECI ${creci}` : ''}
        </p>
      </div>
    </footer>
  )
}

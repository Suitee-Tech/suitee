import Link from 'next/link'
import type { Tenant } from '@/payload-types'
import { tenantDisplayName, tenantLogoUrl } from '@/lib/format'
import { MenuIcon } from './icons'
import { FavoritesLink } from './FavoritesLink'
import { BrandMark } from './BrandMark'

interface SiteHeaderProps {
  tenant: Tenant
  portalUrl?: string
}

const navItems = [
  { href: '/', label: 'Início' },
  { href: '/imoveis', label: 'Imóveis' },
  { href: '/sobre', label: 'Sobre' },
  { href: '/contato', label: 'Contato' },
]

export function SiteHeader({ tenant, portalUrl }: SiteHeaderProps) {
  const logoUrl = tenantLogoUrl(tenant, '')

  return (
    <header className="site-header">
      <div className="site-container site-header__inner">
        <Link href="/" className="site-header__brand" aria-label={`${tenantDisplayName(tenant)} — início`}>
          <BrandMark
            tenant={tenant}
            logoUrl={logoUrl}
            fallbackLogoUrl="/suitee-logo.png"
          />
        </Link>

        <nav className="site-nav site-nav--desktop" aria-label="Navegação principal">
          {navItems.map((item) => (
            <Link key={item.href} href={item.href} className="site-nav__link">
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="site-header__actions">
          <FavoritesLink tenant={tenant.slug} />
          {portalUrl ? (
            <a className="site-header__account" href={portalUrl}>
              Área do cliente
            </a>
          ) : null}
          <Link href="/imoveis" className="button button--primary site-header__cta">
            Encontrar imóvel
          </Link>
        </div>

        <details className="site-menu">
          <summary className="site-menu__trigger" aria-label="Abrir menu">
            <MenuIcon width={22} height={22} />
            <span>Menu</span>
          </summary>
          <nav className="site-menu__panel" aria-label="Navegação mobile">
            {navItems.map((item) => (
              <Link key={item.href} href={item.href} className="site-menu__link">
                {item.label}
              </Link>
            ))}
            <FavoritesLink tenant={tenant.slug} mobile />
            {portalUrl ? (
              <a href={portalUrl} className="site-menu__account">
                Área do cliente
              </a>
            ) : null}
            <Link href="/contato" className="button button--primary">
              Falar com a imobiliária
            </Link>
          </nav>
        </details>
      </div>
    </header>
  )
}

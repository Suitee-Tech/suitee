import type { Metadata, Viewport } from 'next'
import type { ReactNode } from 'react'
import { notFound } from 'next/navigation'
import { SiteFooter } from '@/components/site/SiteFooter'
import { SiteHeader } from '@/components/site/SiteHeader'
import { WhatsAppIcon } from '@/components/site/icons'
import { brandingStyle, safeHexColor, whatsappHref } from '@/lib/format'
import { buildSiteJsonLd, buildTenantMetadata, serializeJsonLd } from '@/lib/seo'
import { getSiteContext, tenantParamMatches } from '@/lib/site'

export const dynamic = 'force-dynamic'

type LayoutProps = {
  children: ReactNode
  params: Promise<{ tenant: string }>
}

export async function generateMetadata({ params }: LayoutProps): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) {
    return { robots: { index: false, follow: false } }
  }

  const { tenant } = await params
  if (!tenantParamMatches(context, tenant)) {
    return { robots: { index: false, follow: false } }
  }

  return buildTenantMetadata(context)
}

export async function generateViewport({ params }: LayoutProps): Promise<Viewport> {
  const context = await getSiteContext()
  if (!context) {
    return { themeColor: '#0f172a' }
  }

  const { tenant } = await params
  if (!tenantParamMatches(context, tenant)) {
    return { themeColor: '#0f172a' }
  }

  return {
    themeColor: safeHexColor(context.tenant.branding?.primaryColor, '#0f172a'),
    colorScheme: context.tenant.branding?.theme === 'dark' ? 'dark' : 'light',
  }
}

export default async function TenantLayout({ children, params }: LayoutProps) {
  const context = await getSiteContext()
  if (!context) {
    notFound()
  }

  const { tenant } = await params
  if (!tenantParamMatches(context, tenant)) {
    notFound()
  }

  const { dataTheme, style } = brandingStyle(context.tenant)
  const whatsapp = whatsappHref(
    context.tenant.contact?.whatsapp || context.tenant.contact?.phone,
  )
  const portalUrl = (process.env.PORTAL_PUBLIC_URL ?? '').trim()
  const siteJsonLd = serializeJsonLd(buildSiteJsonLd(context))

  return (
    <div className="site-shell" data-theme={dataTheme} style={style}>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: siteJsonLd }}
      />
      <a className="skip-link" href="#conteudo">
        Pular para o conteúdo
      </a>
      <SiteHeader tenant={context.tenant} portalUrl={portalUrl} />
      <main id="conteudo" className="site-main">
        {children}
      </main>
      <SiteFooter tenant={context.tenant} portalUrl={portalUrl} />
      {whatsapp ? (
        <a
          className="whatsapp-fab"
          href={whatsapp}
          target="_blank"
          rel="noreferrer"
          aria-label={`Falar com ${context.tenant.name} no WhatsApp`}
        >
          <WhatsAppIcon width={24} height={24} />
        </a>
      ) : null}
    </div>
  )
}

import type { Metadata, Viewport } from 'next'
import type { ReactNode } from 'react'
import '@/styles/site.css'

export const metadata: Metadata = {
  title: 'Imóveis',
  description: 'Encontre imóveis para comprar ou alugar.',
  robots: {
    index: false,
    follow: false,
  },
  icons: [{ rel: 'icon', type: 'image/svg+xml', url: '/favicon.svg' }],
}

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  themeColor: '#0f172a',
  colorScheme: 'light',
}

export default function SiteRootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="pt-BR">
      <body>{children}</body>
    </html>
  )
}

/**
 * Indexação da vitrine. Beta liga `SITE_NOINDEX=true` no compose; produção
 * mantém `false`. A checagem é feita em runtime (não é uma env pública/baked)
 * para que a mesma imagem possa ir de beta para produção no deploy.
 */
export function siteIndexingDisabled(): boolean {
  return String(process.env.SITE_NOINDEX ?? '').trim().toLowerCase() === 'true'
}

export const SITE_NOINDEX_ROBOTS = {
  index: false,
  follow: false,
  googleBot: { index: false, follow: false, noarchive: true },
} as const

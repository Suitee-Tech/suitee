import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { FavoritesList } from '@/components/site/FavoritesList'
import { getSiteContext } from '@/lib/site'

export const dynamic = 'force-dynamic'

export async function generateMetadata(): Promise<Metadata> {
  const context = await getSiteContext()
  if (!context) return { robots: { index: false, follow: false } }
  return {
    title: 'Favoritos',
    description: `Imóveis salvos no site de ${context.tenant.name}.`,
    robots: { index: false, follow: false },
  }
}

export default async function FavoritesPage() {
  const context = await getSiteContext()
  if (!context) notFound()
  return <FavoritesList tenant={context.slug} />
}

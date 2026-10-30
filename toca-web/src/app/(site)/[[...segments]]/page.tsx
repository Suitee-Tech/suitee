import { notFound } from 'next/navigation'

export const dynamic = 'force-dynamic'

/**
 * Apex/marketing hosts (and any unmatched path that reached Next without a
 * tenant rewrite) intentionally resolve to a 404 page. The public storefront
 * never falls back to a default tenant.
 */
export default function UnknownHostPage() {
  notFound()
}

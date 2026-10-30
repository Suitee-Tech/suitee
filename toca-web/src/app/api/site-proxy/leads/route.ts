import { proxyPublicSiteRequest } from '@/lib/site-proxy'

export const dynamic = 'force-dynamic'

export function POST(request: Request): Promise<Response> {
  return proxyPublicSiteRequest(request, 'leads')
}

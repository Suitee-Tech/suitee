import { useQuery } from "@tanstack/react-query";
import { ExternalLink } from "lucide-react";

import { portalService } from "../services/portal.service";

/**
 * CTA "Ver anúncio no site" do portal do cliente.
 *
 * Só renderiza quando o imóvel está publicado na vitrine da imobiliária do
 * DEAL. O JWT do cliente do portal não carrega o tenant da imobiliária (ele é
 * a conta do próprio cliente), então o tenant é passado explicitamente a
 * partir do negócio — o backend devolve somente a URL pública de um anúncio já
 * publicado, nunca rascunho, métrica ou dado de outro tenant.
 */
export function PortalSiteListingLink({
  propertyId,
  tenantId,
}: {
  propertyId?: string;
  tenantId?: string;
}) {
  const { data } = useQuery({
    queryKey: ["portal", "site-listing-url", tenantId, propertyId],
    queryFn: () => portalService.getSiteListingUrl(propertyId!, tenantId),
    enabled: Boolean(propertyId),
    staleTime: 5 * 60_000,
    retry: false,
  });

  if (!data?.published || !data.public_url) return null;

  return (
    <a
      href={data.public_url}
      target="_blank"
      rel="noreferrer"
      className="mt-3 inline-flex h-10 items-center justify-center gap-1.5 rounded-xl border border-border bg-card px-4 text-sm font-semibold text-foreground transition-colors hover:bg-muted"
    >
      <ExternalLink className="h-4 w-4" aria-hidden="true" />
      Ver anúncio no site
    </a>
  );
}

import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Globe2 } from "lucide-react";

import { portalService } from "../services/portal.service";

/**
 * Cartão da imobiliária do deal no portal do cliente.
 *
 * O cliente já conversa com a imobiliária pelo chat; este cartão fecha o
 * circuito com a vitrine pública (imóveis, institucional e contato). A query
 * usa o tenant do deal, nunca o tenant da conta do cliente.
 */
export function PortalAgencySiteCard({ tenantId }: { tenantId?: string }) {
  const { data } = useQuery({
    queryKey: ["portal", "agency-site", tenantId],
    queryFn: () => portalService.getSite(tenantId),
    enabled: Boolean(tenantId),
    staleTime: 5 * 60_000,
    retry: false,
  });

  if (!data?.public_url) return null;

  return (
    <section
      className="rounded-3xl border bg-card p-5 shadow-sm"
      style={data.branding?.primary_color ? { borderColor: data.branding.primary_color } : undefined}
    >
      <div className="flex items-center gap-2 text-caption font-bold uppercase tracking-wide text-muted-foreground">
        <Globe2 className="h-4 w-4" aria-hidden="true" />
        Site da imobiliária
      </div>
      <h2 className="mt-2 text-lg font-bold text-foreground">{data.name}</h2>
      <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
        Veja o catálogo completo, conheça a equipe e fale direto com a imobiliária.
      </p>
      <a
        href={data.public_url}
        target="_blank"
        rel="noreferrer"
        className="mt-4 inline-flex h-10 items-center justify-center gap-1.5 rounded-xl border border-border bg-card px-4 text-sm font-semibold text-foreground transition-colors hover:bg-muted"
      >
        Visitar site
        <ExternalLink className="h-4 w-4" aria-hidden="true" />
      </a>
    </section>
  );
}

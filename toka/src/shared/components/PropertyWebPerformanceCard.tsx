import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Eye, Globe2, Loader2, Users } from "lucide-react";
import { Link } from "react-router";

import { usePermissionsContext } from "../context/PermissionsContext";
import { siteService } from "../services/site.service";
import { Button } from "../ui/button";
import { Card, CardContent } from "../ui/card";

/**
 * Link canônico do anúncio no host comercial (`{marca}.imoveis...`).
 *
 * Só aparece quando o imóvel está de fato publicado na vitrine. Um link para
 * uma URL que responde 404 é pior que nenhum link: o corretor manda para o
 * comprador e o comprador vê "não encontrado".
 */
export function PropertySiteLink({ propertyId }: { propertyId: string }) {
  const { can } = usePermissionsContext();
  // A URL é pública e o link mora no detalhe do imóvel: a leitura acompanha
  // `properties:read`, não a gestão da vitrine.
  const canReadProperty = can("properties:read");

  const { data } = useQuery({
    queryKey: ["site", "listing-url", propertyId],
    queryFn: () => siteService.listingUrl(propertyId),
    enabled: canReadProperty && Boolean(propertyId),
    staleTime: 60_000,
    retry: false,
  });

  if (!data?.published || !data.public_url) return null;

  return (
    <Button variant="outline" asChild>
      <a href={data.public_url} target="_blank" rel="noreferrer">
        <ExternalLink className="size-4" aria-hidden="true" />
        Ver no site
      </a>
    </Button>
  );
}

/**
 * Card "Performance Web" do detalhe do imóvel.
 *
 * Pageviews não passam pelo barramento: são lidos sob demanda do toca-sites
 * (7/30 dias) e limitados ao tenant do JWT. Sem `sites:read` o card não é
 * renderizado — não há métrica de vitrine para quem não administra a vitrine.
 */
export function PropertyWebPerformanceCard({ propertyId }: { propertyId: string }) {
  const { can } = usePermissionsContext();
  const canReadSite = can("sites:read");

  const { data, isLoading } = useQuery({
    queryKey: ["site", "listing-stats", propertyId],
    queryFn: () => siteService.listingStats(propertyId),
    enabled: canReadSite && Boolean(propertyId),
    staleTime: 60_000,
    retry: false,
  });

  if (!canReadSite) return null;

  const published = Boolean(data?.public_url);
  const statusLabel = published ? "Ativo" : "Rascunho";
  const statusClass = published
    ? "bg-status-ok-soft text-status-ok border-status-ok/30"
    : "bg-status-warning-soft text-status-warning border-status-warning/30";

  return (
    <Card className="overflow-hidden border-0 bg-gradient-to-br from-primary-900 to-primary-700 text-white">
      <CardContent className="space-y-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="flex items-center gap-2 text-caption font-bold uppercase tracking-wider text-white/80">
              <Globe2 className="size-4" aria-hidden="true" />
              Performance Web
            </p>
            <p className="mt-1 text-sm text-white/80">
              {published
                ? "Este anúncio está no site de imóveis da sua imobiliária."
                : "Este anúncio ainda não está publicado no site de imóveis."}
            </p>
          </div>
          <span className={`rounded-full border px-2.5 py-1 text-caption font-bold uppercase tracking-wider ${statusClass}`}>
            {statusLabel}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-3 sm:max-w-md">
          <div className="rounded-xl border border-white/15 bg-white/10 px-4 py-3">
            <p className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wider text-white/80">
              <Eye className="size-3.5" aria-hidden="true" />
              Visualizações (7d)
            </p>
            <p className="mt-1 text-2xl font-bold text-white">
              {isLoading ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : (data?.views_7d ?? 0)}
            </p>
          </div>
          <div className="rounded-xl border border-white/15 bg-white/10 px-4 py-3">
            <p className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wider text-white/80">
              <Users className="size-3.5" aria-hidden="true" />
              Leads gerados (7d)
            </p>
            <p className="mt-1 text-2xl font-bold text-white">
              {isLoading ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : (data?.leads_7d ?? 0)}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {data?.public_url ? (
            <Button variant="secondary" size="sm" asChild>
              <a href={data.public_url} target="_blank" rel="noreferrer">
                <ExternalLink className="size-4" aria-hidden="true" />
                Abrir anúncio no site
              </a>
            </Button>
          ) : null}
          <Button variant="ghost" size="sm" className="text-white hover:bg-white/10 hover:text-white" asChild>
            <Link to="/site">Configurar site de imóveis</Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

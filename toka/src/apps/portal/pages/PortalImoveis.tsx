import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ArrowLeft, Building2, DollarSign, Loader2 } from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../../../shared/ui/button";
import { PortalEmpty, PortalError } from "../components/PortalState";
import { portalService, type PortalRentalDTO } from "../services/portal.service";
import { PortalSiteListingLink } from "../components/PortalSiteListingLink";
import { usePortalDeals } from "../context/PortalDealsContext";
import { formatCurrencyBRL0 } from "../../../shared/utils/format";
import { describeListingPrice } from "../../../shared/utils/listingPrice";
import { collectLeadProperties } from "./portalPropertyPresentation";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { usePortalPersona } from "../hooks/usePortalPersona";
import { AgencyIdentity } from "../components/AgencyIdentity";
import { leadStatusLabel } from "../lib/portalRelationship";

export function PortalImoveis() {
  const navigate = useNavigate();
  // "Meus Imóveis" só descreve o que a tela mostra para quem anuncia. Para quem
  // procura, os imóveis listados são os do atendimento (sugestões/recomendações).
  const { copy } = usePortalPersona();
  const { activeDeal } = usePortalDeals();
  const [items, setItems] = useState<PortalRentalDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [rentalsLoading, setRentalsLoading] = useState(false);
  const [rentalsError, setRentalsError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const [overviewProps, setOverviewProps] = useState<any[]>([]);
  const [overviewLeads, setOverviewLeads] = useState<import("../services/portal.service").PortalOverviewLead[]>([]);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setRentalsError(null);
    setItems([]);
    setOverviewProps([]);
    setOverviewLeads([]);

    const load = async () => {
      try {
        // O overview é a fonte dos imóveis do atendimento (lead do site,
        // recomendações e matches). Ele NÃO pode depender das locações: uma
        // falha do financeiro não pode esconder o imóvel que gerou o lead.
        const ov = await portalService.getOverview();
        if (cancelled) return;
        setOverviewLeads(ov ?? []);
        const props = collectLeadProperties(ov ?? []).map((entry) => ({
          property: {
            id: entry.property.id,
            title: entry.property.title,
            neighborhood: entry.property.neighborhood,
            city: entry.property.city,
            state: entry.property.state,
            sale_price: entry.property.listingType === "sale" ? entry.property.price : undefined,
            rent_price: entry.property.listingType === "rent" ? entry.property.price : undefined,
            listing_type: entry.property.listingType,
            media_images: entry.property.images.map((image) => image.url),
            thumbnail_url: entry.property.imageUrl,
            images: entry.property.images,
            image_url: entry.property.imageUrl,
          },
          score: entry.property.matchScore,
          leadId: entry.leadId,
          tenantId: entry.tenantId,
          recommended: entry.recommended,
        }));
        setOverviewProps(props);

        // "Locações" é vocabulário de proprietário. Para comprador/locatário
        // do lead do site, não chamamos o financeiro (que pode estar lento).
        const ownerFromLead = (ov ?? []).some((lead) => lead.role === "owner");
        const ownerFromDeal =
          activeDeal?.my_role === "owner";
        if (ownerFromLead || ownerFromDeal) {
          setRentalsLoading(true);
          portalService
            .getMyRentals()
            .then((data) => {
              if (!cancelled) {
                setItems((data ?? []).filter((d) => d.my_role === "owner" || d.my_role === "both"));
              }
            })
            .catch(() => {
              if (!cancelled) {
                setRentalsError(
                  "Não foi possível carregar suas locações agora. Seus imóveis do atendimento continuam disponíveis.",
                );
              }
            })
            .finally(() => {
              if (!cancelled) setRentalsLoading(false);
            });
        }
      } catch {
        if (!cancelled) {
          setError("Não foi possível carregar seus imóveis do atendimento.");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [activeDeal?.id, activeDeal?.my_role, reloadKey]);

  const hasSuggestedProperties = copy.recommendationsTitle !== null && overviewProps.length > 0;
  const relationGroups = (() => {
    const byLead = new Map<string, typeof overviewProps>();
    for (const entry of overviewProps) {
      const key = entry.leadId || "sem-atendimento";
      byLead.set(key, [...(byLead.get(key) ?? []), entry]);
    }
    return Array.from(byLead.entries()).map(([leadId, entries]) => ({
      leadId,
      lead: overviewLeads.find((lead) => lead.id === leadId) ?? null,
      entries,
    }));
  })();

  return (
    <PortalPage className="overflow-x-hidden">
      <PortalPageHeader title={copy.propertiesTitle} />

      <div className="px-5 pt-4 space-y-4">
        {loading && (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin mr-2" /> Carregando…
          </div>
        )}

        {!loading && error && (
          <div className="space-y-3">
            <PortalError description={error} />
            <Button variant="outline" size="sm" onClick={() => setReloadKey((key) => key + 1)}>
              Tentar novamente
            </Button>
          </div>
        )}

        {!loading && !error && !hasSuggestedProperties && items.length === 0 && !rentalsLoading && !rentalsError && (
          <PortalEmpty
            icon={Building2}
            title={copy.propertiesEmptyTitle}
            description={copy.propertiesEmptyBody}
            action={
              <Button asChild>
                <Link to="/portal/buscar">Explorar imóveis</Link>
              </Button>
            }
          />
        )}

        {!loading && !error && hasSuggestedProperties && (
          <div className="space-y-6">
            {relationGroups.map(({ leadId, lead, entries }) => (
              <section key={leadId} className="space-y-3">
                <div className="rounded-2xl border border-border bg-card p-4 shadow-sm">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <AgencyIdentity tenantId={lead?.tenant_id} showSiteLink />
                    {lead ? (
                      <span className="rounded-full border border-border bg-muted px-2.5 py-1 text-[11px] font-bold text-muted-foreground">
                        {leadStatusLabel(lead.status)}
                      </span>
                    ) : null}
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className="font-semibold text-foreground">
                      {lead?.broker_name ? `Atendimento com ${lead.broker_name}` : "Atendimento imobiliário"}
                    </span>
                    <span>· {entries.length} {entries.length === 1 ? "imóvel" : "imóveis"}</span>
                    {lead?.role ? <span>· {lead.role === "owner" ? "Proprietário" : lead.role === "tenant" ? "Locatário" : lead.role === "buyer" ? "Comprador" : "Cliente"}</span> : null}
                  </div>
                </div>
                {entries.map(({ property: p, score, recommended, tenantId }, i) => {
                  const valor = describeListingPrice(
                    (p as any).listing_type === "rent" ? (p as any).rent_price : (p as any).sale_price,
                    (p as any).listing_type,
                  );
                  return (
                    <div key={(p as any).id || i} className="space-y-1">
                      <Link
                        to={`/portal/imoveis/${(p as any).id}`}
                        state={{ property: p, tenantId, leadId }}
                        className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 hover:shadow-md transition-all"
                      >
                        <div className="flex items-start gap-3">
                          <div className="w-20 h-16 sm:w-24 sm:h-20 rounded-xl overflow-hidden shrink-0 bg-muted flex items-center justify-center text-muted-foreground">
                            {(p as any).media_images?.length > 0 ? (
                              <img src={(p as any).media_images[0]} alt={(p as any).title || "Imóvel"} className="w-full h-full object-cover" />
                            ) : (p as any).thumbnail_url ? (
                              <img src={(p as any).thumbnail_url} alt={(p as any).title || "Imóvel"} className="w-full h-full object-cover" />
                            ) : (
                              <Building2 className="w-5 h-5" />
                            )}
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:justify-between sm:gap-2">
                              <p className="min-w-0 text-sm font-bold leading-snug text-foreground line-clamp-2">{(p as any).title || "Imóvel"}</p>
                              {recommended && <span className="shrink-0 whitespace-nowrap rounded-full bg-primary-500/10 px-2 py-0.5 text-caption font-bold text-primary-600">Sugestão do corretor</span>}
                            </div>
                            {((p as any).neighborhood || (p as any).city) && (
                              <p className="text-xs text-muted-foreground mt-0.5 truncate">{[ (p as any).neighborhood, (p as any).city ].filter(Boolean).join(", ")}</p>
                            )}
                            <div className="mt-2 flex flex-wrap items-center justify-between gap-1.5">
                              <span className="text-sm font-bold text-foreground tabular-nums">
                                {valor.label && <span className="mr-1 text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>}
                                {valor.amount}
                                {valor.period && <span className="text-caption font-normal text-muted-foreground">{valor.period}</span>}
                              </span>
                              {typeof score === "number" && <span className="text-caption font-bold text-status-ok">{(score).toFixed(0)}% compatível</span>}
                            </div>
                          </div>
                        </div>
                      </Link>
                      {tenantId ? (
                        <PortalSiteListingLink propertyId={(p as any).id} tenantId={tenantId} />
                      ) : null}
                    </div>
                  );
                })}
              </section>
            ))}
          </div>
        )}

        {rentalsLoading && (
          <div className="flex items-center justify-center py-6 text-sm text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin mr-2" /> Carregando locações…
          </div>
        )}

        {rentalsError && (
          <div className="space-y-2 rounded-2xl border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">{rentalsError}</p>
            <Button variant="outline" size="sm" onClick={() => setReloadKey((key) => key + 1)}>
              Tentar novamente
            </Button>
          </div>
        )}

        {items.length > 0 && (
          <p className="pt-2 text-caption font-bold uppercase tracking-wide text-muted-foreground">
            Suas locações
          </p>
        )}

        {items.map(({ rental: r }) => (
          <Link
            key={r.id}
            to={`/portal/locacao/${r.id}`}
            className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 hover:shadow-md transition-all"
          >
            <div className="flex items-center gap-3 mb-3">
              <div className="w-12 h-12 rounded-xl bg-primary-50 flex items-center justify-center shrink-0">
                <Building2 className="w-6 h-6 text-primary-600" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-base font-bold text-foreground truncate">Imóvel #{r.property_id.slice(0, 8)}</p>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Locação ativa desde {r.start_date ? new Date(r.start_date).toLocaleDateString("pt-BR") : "—"}
                </p>
              </div>
              <span
                className={clsx(
                  "px-2.5 py-1 text-xs font-bold rounded-full shrink-0",
                  r.status === "ACTIVE"
                    ? "bg-status-ok-soft text-status-ok"
                    : r.status === "ENDED"
                      ? "bg-muted text-muted-foreground"
                      : "bg-status-critical-soft text-status-critical",
                )}
              >
                {r.status === "ACTIVE" ? "Locado" : r.status === "ENDED" ? "Encerrado" : "Rescindido"}
              </span>
            </div>
            <div className="bg-primary-50 rounded-xl px-3 py-2 flex items-center gap-2 text-sm">
              <DollarSign className="w-4 h-4 text-primary-600" />
              <span className="text-xs text-muted-foreground">Aluguel mensal:</span>
              <span className="font-bold text-foreground ml-auto">{formatCurrencyBRL0((r.monthly_value_cents || 0) / 100)}</span>
            </div>
          </Link>
        ))}
      </div>
    </PortalPage>
  );
}

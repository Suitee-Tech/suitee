import { usePortalContact } from "../hooks/usePortalContact";
import { useMemo, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import { Bath, BedDouble, CalendarDays, Car, Eye, Heart, Loader2, MapPin, MessageSquareText, Ruler, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../../../shared/ui/button";
import { portalService } from "../services/portal.service";
import { PropertyGallery } from "../components/PropertyGallery";
import { PortalError } from "../components/PortalState";
import { format as formatDate } from "date-fns";
import { AppointmentPickerDialog, type AppointmentPickerValues } from "../components/AppointmentPickerDialog";
import { usePortalOverview } from "../hooks/usePortalOverview";
import { usePortalDeals } from "../context/PortalDealsContext";
import { usePortalPersona } from "../hooks/usePortalPersona";
import { useQuery } from "@tanstack/react-query";
import { portalKeys } from "../hooks/queryKeys";
import { isVisitClosed, visitPickerDefaults, visitStatusLabel } from "./portalVisitPresentation";
import {
  matchedToPropertyView,
  summaryToPropertyView,
} from "./portalPropertyPresentation";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { PortalSiteListingLink } from "../components/PortalSiteListingLink";
import { notifyApiError } from "../../../shared/services/errorToast";

function formatBRDate(iso?: string): string {
  if (!iso) return "";
  const [y, m, d] = iso.split("-");
  if (!y || !m || !d) return iso;
  return `${d}/${m}/${y}`;
}
function matchLabel(score?: number) {
  if (typeof score !== "number") return "Selecionado para você";
  if (score >= 80) return "Excelente combinação";
  if (score >= 60) return "Boa combinação";
  return "Combinação possível";
}
function matchReason(property: any) {
  const reasons = property?.match_reasons;
  if (!reasons) return "Escolhido com base no seu interesse";
  const matches = [
    { value: reasons.business ?? 0, weight: reasons.used_weights?.business ?? 0, label: "tipo de negócio" },
    { value: reasons.price ?? 0, weight: reasons.used_weights?.price ?? 0, label: "orçamento" },
    { value: reasons.location ?? 0, weight: reasons.used_weights?.location ?? 0, label: "região" },
    { value: reasons.bedrooms ?? 0, weight: reasons.used_weights?.bedrooms ?? 0, label: "quartos" },
  ].filter((i) => i.weight > 0).sort((a, b) => b.value / b.weight - a.value / a.weight);
  const best = matches[0];
  if (!best) return "Escolhido com base no seu interesse";
  return `Combina com seu interesse por ${best.label}`;
}

/** Rótulo humano para a comodidade cadastrada (ex.: "sala_de_estar"). */
/** Metros quadrados sem zeros inúteis (300 em vez de 300.00). */
function formatArea(value?: number): string {
  if (typeof value !== "number" || !Number.isFinite(value)) return "";
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.?0+$/, "");
}

function amenityLabel(value: string): string {
  const raw = String(value ?? "").replace(/[_-]+/g, " ").trim();
  if (!raw) return "";
  return raw.charAt(0).toUpperCase() + raw.slice(1);
}

/**
 * Visão do imóvel que esta tela renderiza: a união das três fontes
 * (estado da navegação, imóvel do negócio e sugestões/recomendações) mais os
 * detalhes que a projeção do portal expõe.
 */
type PortalPropertyView = {
  id?: string;
  title?: string;
  address?: string;
  neighborhood?: string;
  city?: string;
  state?: string;
  price?: number;
  listing_type?: string;
  image_url?: string;
  bedrooms?: number;
  suites?: number;
  bathrooms?: number;
  parking_spaces?: number;
  total_area_m2?: number;
  usable_area_m2?: number;
  amenities?: string[];
  description?: string;
  images?: { url: string; description?: string }[];
};

type PortalImovelDetailState = {
  property?: { id?: string } & Record<string, any>;
  leadId?: string;
  dealId?: string;
  tenantId?: string;
} | null;

export function PortalImovelDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const overviewQuery = usePortalOverview();
  const { activeDeal } = usePortalDeals();

  // O imóvel chega por TRÊS caminhos, e o detalhe precisa aceitar todos:
  //
  //  1. estado da navegação — quem lista (Busca, Meus Imóveis, Dashboard) já
  //     tem o objeto em mãos e o entrega no `state`;
  //  2. busca do negócio (`/client-portal/deals/:id/properties`) — cobre o
  //     RECARREGAMENTO da página, quando o estado da navegação se perde;
  //  3. overview do portal — recomendação/match do lead, para quem chega por
  //     link direto sem negócio ativo.
  //
  // Antes só o caminho 3 existia: abrir pelo Buscar e recarregar (ou abrir um
  // imóvel que não é o recomendado/match do lead) mostrava "Imóvel não
  // encontrado." mesmo com o imóvel existindo e tendo acabado de ser listado.
  const navigationProperty = (() => {
    const state = location.state as PortalImovelDetailState;
    const property = state?.property;
    if (!property || (property.id && property.id !== id)) return null;
    // Dashboard/Buscar entregam o objeto cru (recomendação com thumbnail_url /
    // media_images). O detalhe precisa do mesmo formato da galeria para não
    // cair em "Fotos em preparação" depois de clicar no card.
    if (property.images?.length || property.image_url) return property;
    if (property.media?.images?.length) {
      return summaryToPropertyView(property as Parameters<typeof summaryToPropertyView>[0]);
    }
    if (property.media_images?.length || property.thumbnail_url) {
      return matchedToPropertyView(property as Parameters<typeof matchedToPropertyView>[0], property.score);
    }
    return property;
  })();
  const dealId = (location.state as PortalImovelDetailState)?.dealId || activeDeal?.id || null;
  const stateLeadId = (location.state as PortalImovelDetailState)?.leadId;
  const stateTenantId = (location.state as PortalImovelDetailState)?.tenantId;
  // Sem negócio, o tenant da imobiliária vem do lead do site no overview —
  // necessário para o CTA "Ver anúncio no site" no detalhe do imóvel.
  const overviewTenantId = overviewQuery.data?.find((lead) => {
    if (lead.recommended_property?.id === id) return true;
    return (lead.matched_properties ?? []).some((match) => match.property?.id === id);
  })?.tenant_id;
  const siteTenantId =
    (activeDeal as { tenant_id?: string } | null)?.tenant_id ||
    stateTenantId ||
    overviewTenantId;

  const dealPropertyQuery = useQuery({
    queryKey: portalKeys.detailProperty(id ?? ""),
    enabled: Boolean(dealId && id && !navigationProperty),
    queryFn: async () => {
      // 1) O imóvel DO NEGÓCIO tem endpoint dedicado. A busca do negócio
      //    (`/deals/:id/properties`) devolve as SUGESTÕES feitas ao cliente e
      //    pode não conter o imóvel que está sendo negociado — era isso que
      //    fazia o detalhe do imóvel da negociação cair em "Imóvel não
      //    encontrado" mesmo com a API respondendo 200.
      if (dealId) {
        try {
          const dealProperty = await portalService.getDealProperty(dealId);
          if (dealProperty && (dealProperty as { id?: string }).id === id) return dealProperty;
        } catch {
          // Negócio sem imóvel vinculado (ou indisponível no momento): segue
          // para a busca, que cobre as sugestões listadas no contexto do negócio.
        }
      }
      // 2) Sugestões do negócio (imóvel aberto a partir da lista de sugestões).
      const properties = await portalService.searchProperties(dealId!, {});
      return properties.find((property) => property.id === id) ?? null;
    },
  });

  const lead = overviewQuery.data?.find((item) => item.recommended_property_id === id || item.matched_properties?.some((match) => match.property.id === id)) ?? null;
  const [visitOpen, setVisitOpen] = useState(false);
  const { openContact, opening: chatLoading } = usePortalContact();

  // Vocabulário do cliente: quem procura (comprador/locatário) e quem anuncia
  // (vendedor/locador) não veem os mesmos CTAs.
  const { copy } = usePortalPersona({ leadRole: lead?.role ?? null });
  const recMatch = useMemo(() => (lead?.matched_properties || []).find((m) => m.property.id === id), [lead, id]);
  // Normaliza todas as origens para o mesmo formato de galeria. O overview
  // entrega `thumbnail_url`/`media_images`; a tela espera `image_url`/`images`.
  // Sem isso o card do lead exibia a foto, mas o detalhe caía em "Fotos em
  // preparação".
  const recommendedView = lead?.recommended_property_id === id && lead?.recommended_property
    ? matchedToPropertyView(lead.recommended_property, lead.recommended_property.score ?? 100)
    : undefined;
  const matchedView = recMatch?.property
    ? matchedToPropertyView(recMatch.property, recMatch.score)
    : undefined;
  const dealView = dealPropertyQuery.data ?? undefined;
  const rec = navigationProperty || dealView || recommendedView || matchedView;
  const property = rec as PortalPropertyView | undefined;
  // A tela NÃO anuncia valor de venda/aluguel: a negociação é conduzida pelo
  // corretor e o valor que vale é o do negócio (tela de acompanhamento), não o
  // preço de anúncio do cadastro. Exibir o preço aqui abria divergência entre
  // o que o cliente via e o que estava sendo negociado.
  const leadForActions = lead ?? (stateLeadId ? ({ id: stateLeadId } as any) : null);
  const currentVisit = lead?.visit_request;
  const activeVisit = currentVisit?.property_id === id && !isVisitClosed(currentVisit)
    ? currentVisit
    : undefined;

  // Consulta DESABILITADA fica `isPending` para sempre no React Query: sem
  // negócio ativo, esperar a busca do negócio travaria a tela no spinner.
  const awaitingDealSearch = Boolean(dealId && id) && !navigationProperty && dealPropertyQuery.isPending;
  const loading = !navigationProperty && !rec && (overviewQuery.isPending || awaitingDealSearch);
  // Falha de leitura NÃO pode se disfarçar de "não encontrado": quando o
  // imóvel existe e a fonte falhou (sessão, rede, 5xx), a tela mostra erro com
  // "Tentar novamente" — antes o operador via "não faz parte das sugestões".
  const loadFailed = overviewQuery.isError || dealPropertyQuery.isError;
  const error = !rec && loadFailed && !awaitingDealSearch
    ? "Não foi possível carregar este imóvel."
    : null;

  const handleScheduleVisit = async (values: AppointmentPickerValues) => {
    if (!leadForActions) throw new Error("Oportunidade não encontrada.");
    try {
      const request = { date: formatDate(values.date, "yyyy-MM-dd"), time: values.startTime, end_time: values.endTime, note: values.note?.trim() };
      if (activeVisit) {
        await portalService.updateLeadVisit(leadForActions.id, request);
      } else {
        await portalService.requestLeadVisit(leadForActions.id, { property_id: id, ...request });
      }
      toast.success(activeVisit ? "Novo horário solicitado. Seu corretor fará a confirmação." : "Visita solicitada! Seu corretor confirmará o horário.");
      void overviewQuery.refetch();
    } catch (err: any) {
      notifyApiError(err, { context: "Solicitar visita" });
      throw err;
    }
  };

  // Características que qualificam a decisão, na ordem em que o cliente
  // costuma comparar imóveis. Só entra o que existe no cadastro.
  const specs = [
    property?.bedrooms ? { icon: BedDouble, value: String(property.bedrooms), label: property.bedrooms === 1 ? "quarto" : "quartos" } : null,
    property?.suites ? { icon: Sparkles, value: String(property.suites), label: property.suites === 1 ? "suíte" : "suítes" } : null,
    property?.bathrooms ? { icon: Bath, value: String(property.bathrooms), label: property.bathrooms === 1 ? "banheiro" : "banheiros" } : null,
    property?.parking_spaces ? { icon: Car, value: String(property.parking_spaces), label: property.parking_spaces === 1 ? "vaga" : "vagas" } : null,
    property?.total_area_m2 ? { icon: Ruler, value: `${formatArea(property.total_area_m2)} m²`, label: "área total" } : null,
    property?.usable_area_m2 && property.usable_area_m2 !== property.total_area_m2
      ? { icon: Ruler, value: `${formatArea(property.usable_area_m2)} m²`, label: "área útil" }
      : null,
  ].filter((spec): spec is { icon: typeof BedDouble; value: string; label: string } => spec !== null);

  const handleOpenChat = () => openContact({ leadId: leadForActions?.id });

  if (loading) return <PortalPage variant="full" className="items-center justify-center"><Loader2 className="w-5 h-5 animate-spin text-muted-foreground" /></PortalPage>;
  if (error) return (
    <PortalPage variant="full" className="px-5 pt-8">
      <PortalError
        description={error}
        onRetry={() => {
          void overviewQuery.refetch();
          void dealPropertyQuery.refetch();
        }}
      />
    </PortalPage>
  );
  if (!rec) return (
    <PortalPage variant="full" className="items-center justify-center gap-3 px-8 text-center">
      <p className="text-sm text-muted-foreground">Imóvel não encontrado.</p>
      <p className="text-xs text-muted-foreground">
        Ele pode ter saído da carteira ou não faz parte das sugestões do seu atendimento.
      </p>
      <Button variant="outline" size="sm" onClick={() => navigate("/portal/buscar")}>
        Ver imóveis disponíveis
      </Button>
    </PortalPage>
  );

  return (
    <PortalPage variant="full">
      <PortalPageHeader title={rec?.title || "Imóvel"} backTo="/portal/imoveis" />

      <div className="flex-1 min-h-0 overflow-y-auto">
        {/* Galeria: foto grande com arraste, contador, miniaturas e tela cheia. */}
        <div className="relative">
          <PropertyGallery
            images={property?.images}
            fallbackUrl={property?.image_url}
            alt={rec.title}
            className="w-full h-56 sm:h-72"
          />
        </div>

        <div className="p-5 space-y-4">
          {/* Localização por bairro/cidade/UF. A rua e o número só chegam
              quando a visita está marcada (o backend mascara o endereço nas
              leituras de vitrine); sem ele, a tela explica como liberar. */}
          <div>
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <MapPin className="h-4 w-4 text-primary-600" aria-hidden="true" />
              {[property?.neighborhood, property?.city, property?.state].filter(Boolean).join(", ") || "Localização sob consulta"}
            </p>
            {property?.address ? (
              <p className="mt-1 text-xs text-muted-foreground">{property.address}</p>
            ) : (
              <p className="mt-1 text-xs text-muted-foreground">{copy.propertyAddressHint}</p>
            )}
          </div>

          <p className="text-xs text-muted-foreground flex items-center gap-1.5">
            <Sparkles className="w-3.5 h-3.5 text-primary-600" aria-hidden="true" />
            {typeof recMatch?.score === "number" ? `${matchLabel(recMatch.score)} · ` : ""}{matchReason(rec)}
          </p>

          {/* Características do imóvel: o que o cliente usa para decidir. */}
          {specs.length > 0 && (
            <div className="grid grid-cols-3 gap-2">
              {specs.map((spec) => (
                <div key={spec.label} className="rounded-xl border border-border bg-muted/30 px-3 py-2">
                  <spec.icon className="h-4 w-4 text-primary-600" aria-hidden="true" />
                  <p className="mt-1 text-sm font-bold text-foreground tabular-nums">{spec.value}</p>
                  <p className="text-caption text-muted-foreground">{spec.label}</p>
                </div>
              ))}
            </div>
          )}

          {property?.amenities && property.amenities.length > 0 && (
            <div>
              <h2 className="text-sm font-bold text-foreground">Características e lazer</h2>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {property.amenities.map((amenity) => (
                  <span key={amenity} className="rounded-full border border-border bg-card px-2.5 py-1 text-caption font-semibold text-muted-foreground">
                    {amenityLabel(amenity)}
                  </span>
                ))}
              </div>
            </div>
          )}

          {property?.description && (
            <div>
              <h2 className="text-sm font-bold text-foreground">Sobre o imóvel</h2>
              <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{property.description}</p>
            </div>
          )}

          {lead?.broker_name && (
            <div className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
              {lead.broker_avatar ? (
                <img src={lead.broker_avatar} alt={lead.broker_name} className="w-7 h-7 rounded-full object-cover" />
              ) : (
                <div className="w-7 h-7 rounded-full bg-primary/15 text-primary text-caption font-bold flex items-center justify-center">C</div>
              )}
              Corretor: <strong className="text-foreground">{lead.broker_name}</strong>
            </div>
          )}

          {lead?.visit_request?.status && lead.visit_request.property_id === id && (
            <p className="text-xs font-bold text-status-ok mt-4">
              {`${visitStatusLabel(lead.visit_request)} · ${formatBRDate(lead.visit_request.date)} ${lead.visit_request.time}${lead.visit_request.end_time ? `–${lead.visit_request.end_time}` : ""}`}
            </p>
          )}

          {copy.propertyVisitOwnerHint && (
            <p className="mt-4 rounded-xl border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
              {copy.propertyVisitOwnerHint}
            </p>
          )}

          <div className={copy.propertyVisitCta ? "grid grid-cols-2 gap-2 mt-4" : "grid grid-cols-1 gap-2 mt-4"}>
            <button onClick={handleOpenChat} disabled={chatLoading} className="h-11 rounded-xl bg-primary hover:bg-primary-600 text-primary-foreground text-sm font-semibold inline-flex items-center justify-center gap-1.5 disabled:opacity-60">
              {chatLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <MessageSquareText className="w-4 h-4" />} {copy.propertyContactCta}
            </button>
            {copy.propertyVisitCta && (
              <button onClick={() => setVisitOpen(true)} className="h-11 rounded-xl bg-status-ok hover:bg-status-ok/90 text-white text-sm font-semibold inline-flex items-center justify-center gap-1.5">
                <CalendarDays className="w-4 h-4" /> {activeVisit ? copy.propertyVisitReschedule : copy.propertyVisitCta}
              </button>
            )}
          </div>

          <PortalSiteListingLink propertyId={property?.id} tenantId={siteTenantId} />
        </div>
      </div>

      {copy.propertyVisitCta && (
        <AppointmentPickerDialog
        open={visitOpen} onOpenChange={setVisitOpen}
        propertyLabel={rec.title} onSubmit={handleScheduleVisit}
        mode={activeVisit ? "reschedule" : "create"}
        defaultValues={visitPickerDefaults(activeVisit)}
        />
      )}
    </PortalPage>
  );
}

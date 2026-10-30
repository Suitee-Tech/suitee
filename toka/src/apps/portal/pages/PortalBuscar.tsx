import { useQueryClient } from "@tanstack/react-query";
import { usePortalOverview } from "../hooks/usePortalOverview";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { clsx } from "clsx";
import {
  Bath,
  BedDouble,
  Building2,
  CalendarDays,
  CarFront,
  Heart,
  Home,
  MapPin,
  MessageSquareText,
  Ruler,
  Search as SearchIcon,
  Sparkles,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { format as formatDateFns } from "date-fns";
import { useQuery } from "@tanstack/react-query";
import { portalService, type PortalOverviewLead, type PortalPropertySummary } from "../services/portal.service";
import { PropertyMediaCarousel } from "../components/PropertyMediaCarousel";
import { AppointmentPickerDialog } from "../components/AppointmentPickerDialog";
import { Skeleton } from "../../../shared/ui/skeleton";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { Button } from "../../../shared/ui/button";
import { DealSwitcher } from "../components/DealSwitcher";
import { useCreateDealAppointment, useToggleDealFavorite } from "../hooks/useDealMutations";
import { usePortalDealFavorites } from "../hooks/useDealResources";
import { usePortalDeals } from "../context/PortalDealsContext";
import { portalKeys } from "../hooks/queryKeys";
import { PortalError } from "../components/PortalState";
import { useDebouncedValue } from "../../../shared/hooks/useDebouncedValue";
import { formatCurrencyBRL0 } from "../../../shared/utils/format";
import { describeListingPrice } from "../../../shared/utils/listingPrice";
import {
  collectLeadProperties,
  filterLeadProperties,
  propertyLocation,
  propertyMatchLabel,
  summaryToPropertyView,
  type LeadPropertyEntry,
  type PortalPropertyView,
} from "./portalPropertyPresentation";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { notifyApiError } from "../../../shared/services/errorToast";

type SearchPropertyEntry = {
  property: PortalPropertyView;
  leadId?: string;
  recommended?: boolean;
};

function PropertyFacts({ property }: { property: PortalPropertyView }) {
  const facts = [
    property.bedrooms ? { icon: BedDouble, label: `${property.bedrooms} ${property.bedrooms === 1 ? "quarto" : "quartos"}` } : null,
    property.bathrooms ? { icon: Bath, label: `${property.bathrooms} ${property.bathrooms === 1 ? "banheiro" : "banheiros"}` } : null,
    property.parkingSpaces ? { icon: CarFront, label: `${property.parkingSpaces} ${property.parkingSpaces === 1 ? "vaga" : "vagas"}` } : null,
    property.totalAreaM2 ? { icon: Ruler, label: `${Math.round(property.totalAreaM2)} m²` } : null,
  ].filter((fact): fact is { icon: typeof BedDouble; label: string } => fact != null);

  if (facts.length === 0) return null;

  return (
    <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground" aria-label="Características do imóvel">
      {facts.map(({ icon: Icon, label }) => (
        <li key={label} className="inline-flex items-center gap-1.5">
          <Icon className="size-3.5" aria-hidden="true" />
          {label}
        </li>
      ))}
    </ul>
  );
}

export function PortalBuscar() {
  const { deals, activeDeal, activeDealId, isLoading, setActiveDealId } = usePortalDeals();
  const dealId = activeDeal?.id;
  const createAppointment = useCreateDealAppointment(dealId);
  const toggleFavoriteMutation = useToggleDealFavorite(dealId);
  const [listingType, setListingType] = useState<"rent" | "sale">("rent");
  // Finalidade do NEGÓCIO do cliente — o padrão da tela vem daqui (um negócio
  // de venda não abre a busca em locação) e é o que permite avisar quando o
  // cliente está olhando o outro mercado.
  const dealListingType: "rent" | "sale" | null =
    activeDeal?.type === "RENT" ? "rent" : activeDeal?.type === "SALE" ? "sale" : null;
  const [query, setQuery] = useState("");
  const [propertyType, setPropertyType] = useState("");
  const [cityFilter, setCityFilter] = useState("");
  const [maxPrice, setMaxPrice] = useState("");
  const [isScheduling, setIsScheduling] = useState(false);
  const [scheduleModalOpen, setScheduleModalOpen] = useState(false);
  const [selectedProperty, setSelectedProperty] = useState<SearchPropertyEntry | null>(null);
  const normalizedQuery = useDebouncedValue(query.trim(), 300);
  const leadOverview = usePortalOverview(!isLoading && !dealId).data;

  useEffect(() => {
    if (activeDeal?.type) {
      setListingType(activeDeal.type === "SALE" ? "sale" : "rent");
      return;
    }
    // Sem negócio, a finalidade vem do PAPEL do cliente no atendimento:
    // comprador abre em venda, locatário em locação. Antes a tela abria sempre
    // em "Para alugar", mesmo para quem procurava imóvel para comprar.
    const role = leadOverview?.[0]?.role;
    if (role === "buyer") setListingType("sale");
    else if (role === "tenant") setListingType("rent");
  }, [activeDeal?.id, activeDeal?.type, leadOverview]);

  const searchQuery = useQuery<PortalPropertySummary[]>({
    queryKey: dealId
      ? portalKeys.dealPropertySearch(dealId, listingType, normalizedQuery)
      : [...portalKeys.all(), "deal", "", "properties", listingType, normalizedQuery],
    queryFn: async () => {
      // Com negócio: busca do negócio (deal-scoped). Sem negócio: vitrine das
      // imobiliárias com quem o cliente se relaciona — antes esta consulta
      // ficava desabilitada e quem só tinha lead via apenas a recomendação.
      const properties = dealId
        ? await portalService.searchProperties(dealId, {
            q: normalizedQuery || undefined,
            listing_type: listingType,
            limit: 30,
          })
        : await portalService.listAvailableProperties({ listing_type: listingType, limit: 30 });
      return Array.isArray(properties) ? properties : [];
    },
    enabled: !isLoading,
  });

  const queryClient = useQueryClient();
  const leadOverviewQuery = usePortalOverview(!isLoading && !dealId);

  const favoritesQuery = usePortalDealFavorites(dealId);
  const favoriteIds = useMemo(() => new Set(favoritesQuery.data ?? []), [favoritesQuery.data]);
  const isTogglingFavorite = toggleFavoriteMutation.isPending
    ? toggleFavoriteMutation.variables?.propertyId ?? null
    : null;

  const results = useMemo<SearchPropertyEntry[]>(() => {
    if (dealId) {
      return (Array.isArray(searchQuery.data) ? searchQuery.data : []).map((property) => ({
        property: summaryToPropertyView(property),
      }));
    }

    // Sem negócio: vitrine + sugestões do lead, sem duplicar imóvel. A vitrine
    // vem do servidor (portfólio disponível); as sugestões carregam o vínculo
    // com o lead, necessário para agendar visita.
    const vitrine: SearchPropertyEntry[] = (Array.isArray(searchQuery.data) ? searchQuery.data : []).map(
      (property) => ({ property: summaryToPropertyView(property) }),
    );
    const leadEntries: LeadPropertyEntry[] = collectLeadProperties(leadOverviewQuery.data ?? []);
    const seen = new Set(vitrine.map((entry) => entry.property.id));
    const sugestoes = leadEntries.filter((entry) => !seen.has(entry.property.id));
    // Sugestões primeiro (têm vínculo com o lead); a vitrine entra pelo filtro
    // de texto quando há busca, e completa a lista quando não há.
    const merged: LeadPropertyEntry[] = [
      ...sugestoes,
      ...vitrine.map((entry) => ({ property: entry.property, leadId: "", recommended: false })),
    ];
    return normalizedQuery ? filterLeadProperties(merged, query) : merged;
  }, [dealId, leadOverviewQuery.data, normalizedQuery, query, searchQuery.data]);

  const filteredResults = useMemo(() => {
    const city = cityFilter.trim().toLocaleLowerCase("pt-BR");
    const ceiling = Number(maxPrice);
    return results.filter(({ property }) => {
      if (propertyType && property.propertyType !== propertyType) return false;
      if (city && ![property.city, property.neighborhood, property.address].filter(Boolean).some((value) => value!.toLocaleLowerCase("pt-BR").includes(city))) return false;
      if (Number.isFinite(ceiling) && ceiling > 0 && property.price > ceiling) return false;
      return true;
    });
  }, [cityFilter, maxPrice, propertyType, results]);
  const hasLocalFilters = Boolean(propertyType || cityFilter.trim() || maxPrice);
  const clearLocalFilters = () => { setPropertyType(""); setCityFilter(""); setMaxPrice(""); };
  const isResultsLoading = dealId
    ? searchQuery.isPending
    : searchQuery.isPending || leadOverviewQuery.isPending;
  const isResultsFetching = searchQuery.isFetching || leadOverviewQuery.isFetching;
  const isResultsError = dealId ? searchQuery.isError : searchQuery.isError && leadOverviewQuery.isError;

  const retryResults = () => {
    void searchQuery.refetch();
    if (!dealId) void leadOverviewQuery.refetch();
  };

  // Com negócio, a visita é criada no próprio negócio; sem negócio, depende do
  // atendimento (lead) do imóvel. Item de vitrine não tem esse vínculo.
  const canSchedule = (entry: SearchPropertyEntry) => Boolean(dealId || entry.leadId);

  const handleScheduleClick = (entry: SearchPropertyEntry) => {
    setSelectedProperty(entry);
    setScheduleModalOpen(true);
  };

  const handleScheduleSubmit = async (data: { date: Date; startTime: string; endTime: string; note?: string }) => {
    if (!selectedProperty) return;

    setIsScheduling(true);
    try {
      const date = formatDateFns(data.date, "yyyy-MM-dd");
      if (dealId) {
        const context = `Imóvel: ${selectedProperty.property.title} (${selectedProperty.property.id})`;
        await createAppointment.mutateAsync({
          date,
          time: data.startTime,
          end_time: data.endTime,
          type: "VISIT",
          property_id: selectedProperty.property.id,
          title: `Visita · ${selectedProperty.property.title}`,
          description: [context, data.note?.trim()].filter(Boolean).join("\n"),
        });
      } else if (selectedProperty.leadId) {
        await portalService.requestLeadVisit(selectedProperty.leadId, {
          property_id: selectedProperty.property.id,
          date,
          time: data.startTime,
          end_time: data.endTime,
          note: data.note?.trim(),
        });
        void queryClient.invalidateQueries({ queryKey: portalKeys.overview() });
      } else {
        throw new Error("Não foi possível identificar a oportunidade desta visita.");
      }

      toast.success("Visita solicitada. A imobiliária confirmará o horário com você.");
    } catch (error: any) {
      notifyApiError(error, { context: "Solicitar visita" });
      throw error;
    } finally {
      setIsScheduling(false);
    }
  };

  const toggleFavorite = async (propertyId: string) => {
    if (!dealId || toggleFavoriteMutation.isPending) return;
    const isFavorite = favoriteIds.has(propertyId);
    try {
      await toggleFavoriteMutation.mutateAsync({ propertyId, favorite: !isFavorite });
      toast.success(isFavorite ? "Imóvel removido dos favoritos." : "Imóvel salvo nos favoritos.");
    } catch {
      toast.error("Não foi possível atualizar seus favoritos.");
    }
  };

  if (isLoading) {
    return (
      <PortalPage width="wide" className="bg-muted px-5 py-6 overflow-x-hidden" aria-busy="true" aria-label="Carregando busca de imóveis">
        <Skeleton className="h-20 w-full rounded-2xl" />
        <Skeleton className="mt-5 h-12 w-full rounded-2xl" />
        <div className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-96 rounded-3xl" />)}
        </div>
      </PortalPage>
    );
  }

  return (
    <PortalPage width="wide" className="overflow-x-hidden">
      <PortalPageHeader title="Imóveis" />
      <section className="px-5 pt-6 sm:px-6 sm:pt-8">
        {deals.length > 1 && (
          <div className="mb-5 max-w-xl">
            <DealSwitcher
              deals={deals}
              activeDealId={activeDealId}
              onSelect={setActiveDealId}
              title="Escolha a negociação para pesquisar"
            />
          </div>
        )}

        <div className="max-w-2xl">
          <span className="inline-flex items-center gap-1.5 text-xs font-bold uppercase tracking-wider text-primary-600">
            {dealId ? <Building2 className="size-3.5" aria-hidden="true" /> : <Sparkles className="size-3.5" aria-hidden="true" />}
            {dealId ? "Sua busca" : "Seleções para você"}
          </span>
          <h1 className="mt-2 text-2xl font-bold tracking-tight text-foreground sm:text-3xl">Encontre seu próximo imóvel</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground sm:text-base">
            {dealId
              ? "Explore as opções da imobiliária, salve seus favoritos e solicite uma visita."
              : "Veja os imóveis selecionados a partir das suas preferências e avance com seu corretor."}
          </p>
        </div>

        <div className="mt-5 max-w-2xl">
          <label htmlFor="portal-property-search" className="sr-only">Buscar imóveis</label>
          <div className="relative">
            <SearchIcon className="absolute left-4 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <input
              id="portal-property-search"
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={dealId ? "Buscar pelo nome do imóvel" : "Buscar por imóvel, bairro ou cidade"}
              autoComplete="off"
              className="h-12 w-full rounded-2xl border border-border bg-card pl-11 pr-11 text-sm text-foreground shadow-sm outline-none transition focus:border-primary-500 focus:ring-4 focus:ring-primary-500/10"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                className="absolute right-2 top-1/2 inline-flex size-8 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                aria-label="Limpar busca"
              >
                <X className="size-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>

        <div className="mt-3 flex max-w-3xl flex-wrap items-end gap-2">
          <label className="sr-only" htmlFor="portal-filter-type">Tipo de imóvel</label>
          <select
            id="portal-filter-type"
            value={propertyType}
            onChange={(event) => setPropertyType(event.target.value)}
            className="h-10 rounded-xl border border-border bg-card px-3 text-xs font-semibold text-foreground shadow-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          >
            <option value="">Todos os tipos</option>
            <option value="apartment">Apartamento</option>
            <option value="house">Casa</option>
            <option value="land">Terreno</option>
            <option value="commercial">Comercial</option>
          </select>
          <label className="sr-only" htmlFor="portal-filter-city">Cidade ou bairro</label>
          <input
            id="portal-filter-city"
            value={cityFilter}
            onChange={(event) => setCityFilter(event.target.value)}
            placeholder="Cidade ou bairro"
            className="h-10 min-w-[150px] rounded-xl border border-border bg-card px-3 text-xs text-foreground shadow-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          />
          <label className="sr-only" htmlFor="portal-filter-max">Preço máximo</label>
          <input
            id="portal-filter-max"
            value={maxPrice}
            onChange={(event) => setMaxPrice(event.target.value.replace(/[^\d]/g, ""))}
            inputMode="numeric"
            placeholder="Preço máximo"
            className="h-10 w-32 rounded-xl border border-border bg-card px-3 text-xs text-foreground shadow-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          />
          {hasLocalFilters ? (
            <button type="button" onClick={clearLocalFilters} className="h-10 rounded-xl px-3 text-xs font-semibold text-muted-foreground underline underline-offset-4 hover:text-foreground">
              Limpar filtros
            </button>
          ) : null}
        </div>

        {dealId && (
          <div className="mt-4 inline-flex rounded-full border border-border bg-card p-1 shadow-sm" role="group" aria-label="Finalidade do imóvel">
            {([{"value":"rent","label":"Para alugar"},{"value":"sale","label":"Para comprar"}] as const).map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => setListingType(option.value)}
                aria-pressed={listingType === option.value}
                className={clsx(
                  "rounded-full px-4 py-2 text-sm font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500",
                  listingType === option.value
                    ? "bg-foreground text-background shadow-sm"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                {option.label}
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="px-5 pt-6 sm:px-6" aria-labelledby="property-results-heading" aria-busy={isResultsLoading || isResultsFetching}>
        <div className="mb-4 flex min-h-6 items-center justify-between gap-3">
          <h2 id="property-results-heading" className="text-sm font-bold text-foreground">
            {isResultsLoading ? "Buscando imóveis…" : `${filteredResults.length} ${filteredResults.length === 1 ? "imóvel encontrado" : "imóveis encontrados"}`}
          </h2>
          {isResultsFetching && !isResultsLoading && <span className="text-xs text-muted-foreground" role="status">Atualizando…</span>}
        </div>

        {isResultsLoading && (
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3" aria-hidden="true">
            {Array.from({ length: 3 }).map((_, index) => (
              <div key={index} className="overflow-hidden rounded-3xl border border-border bg-card">
                <Skeleton className="h-48 w-full rounded-none" />
                <div className="space-y-3 p-5">
                  <Skeleton className="h-3 w-32" />
                  <Skeleton className="h-5 w-4/5" />
                  <Skeleton className="h-6 w-28" />
                  <Skeleton className="h-11 w-full rounded-xl" />
                </div>
              </div>
            ))}
          </div>
        )}

        {isResultsError && (
          <PortalError
            title="Não foi possível carregar os imóveis"
            description="Verifique sua conexão e tente novamente. Sua busca foi preservada."
            onRetry={retryResults}
          />
        )}

        {!isResultsLoading && !isResultsError && filteredResults.length > 0 && (
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {filteredResults.map((entry) => {
              const { property } = entry;
              const isFavorite = favoriteIds.has(property.id);
              const detailState = { property, leadId: entry.leadId, dealId };

              return (
                <article key={property.id} className="group overflow-hidden rounded-3xl border border-border bg-card shadow-sm transition hover:-translate-y-0.5 hover:border-primary-300 hover:shadow-md">
                  <div className="relative h-48 overflow-hidden bg-muted">
                    <Link to={`/portal/imoveis/${property.id}`} state={detailState} className="block h-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" aria-label={`Ver detalhes de ${property.title}`}>
                      {property.images.length > 0 || property.imageUrl ? (
                        <PropertyMediaCarousel
                          images={property.images}
                          fallbackUrl={property.imageUrl}
                          alt={property.title}
                          className="h-full w-full"
                          imgClassName="h-full w-full object-cover transition duration-300 group-hover:scale-[1.02]"
                        />
                      ) : (
                        <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-slate-100 to-slate-200 text-muted-foreground">
                          <Home className="size-8" aria-hidden="true" />
                          <span className="sr-only">Imagem indisponível</span>
                        </div>
                      )}
                    </Link>

                    {entry.recommended && (
                      <span className="absolute left-3 top-3 inline-flex items-center gap-1 rounded-full bg-primary/95 px-2.5 py-1 text-[11px] font-bold text-primary-foreground shadow-sm">
                        <Sparkles className="size-3" aria-hidden="true" /> Sugestão do corretor
                      </span>
                    )}

                    {dealId && (
                      <button
                        type="button"
                        onClick={() => void toggleFavorite(property.id)}
                        disabled={isTogglingFavorite === property.id}
                        aria-label={isFavorite ? `Remover ${property.title} dos favoritos` : `Salvar ${property.title} nos favoritos`}
                        aria-pressed={isFavorite}
                        className="absolute right-3 top-3 inline-flex size-10 items-center justify-center rounded-full bg-card/95 text-muted-foreground shadow-sm backdrop-blur transition hover:scale-105 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:opacity-60"
                      >
                        <Heart className={clsx("size-4", isFavorite && "fill-primary-600 text-primary-600")} aria-hidden="true" />
                      </button>
                    )}
                  </div>

                  <div className="p-5">
                    {propertyLocation(property) && (
                      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <MapPin className="size-3.5 shrink-0" aria-hidden="true" />
                        <span className="truncate">{propertyLocation(property)}</span>
                      </p>
                    )}
                    <h3 className="mt-1.5 text-base font-bold leading-6 text-foreground">
                      <Link to={`/portal/imoveis/${property.id}`} state={detailState} className="rounded-sm hover:text-primary-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500">
                        {property.title}
                      </Link>
                    </h3>
                    {(() => {
                      const valor = describeListingPrice(property.price, property.listingType);
                      return (
                        <p className="mt-2 flex items-baseline gap-1.5 text-lg font-bold tabular-nums text-foreground">
                          {valor.label && (
                            <span className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>
                          )}
                          <span>{valor.amount}</span>
                          {valor.period && <span className="text-xs font-normal text-muted-foreground">{valor.period}</span>}
                        </p>
                      );
                    })()}

                    {/* Fora do negócio do cliente, o resultado é do outro mercado:
                        dizer isso evita ler um aluguel como se fosse preço de compra. */}
                    {dealListingType && listingType !== dealListingType && (
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        Fora do seu negócio de {dealListingType === "rent" ? "locação" : "compra"} — mostrando{" "}
                        {listingType === "rent" ? "locação" : "compra"}.
                      </p>
                    )}

                    <PropertyFacts property={property} />

                    {typeof property.matchScore === "number" && (
                      <p className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-status-ok-soft px-2.5 py-1 text-xs font-bold text-status-ok">
                        <Sparkles className="size-3" aria-hidden="true" />
                        {Math.round(property.matchScore)}% · {propertyMatchLabel(property.matchScore)}
                      </p>
                    )}

                    <div className="mt-5 grid grid-cols-2 gap-2">
                      {/* Visita é por LEAD: item de vitrine (sem negócio e sem
                          atendimento vinculado) não tem oportunidade para
                          agendar — o caminho é falar com a imobiliária pelo
                          chat, que os Detalhes oferecem. */}
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => handleScheduleClick(entry)}
                        disabled={!canSchedule(entry) || (isScheduling && selectedProperty?.property.id === property.id)}
                        title={canSchedule(entry) ? undefined : "Fale com a imobiliária pelo chat para agendar uma visita"}
                        className="h-11 rounded-xl px-3 text-sm font-semibold"
                      >
                        <CalendarDays className="mr-1.5 size-4" aria-hidden="true" />
                        Agendar
                      </Button>
                      <Button asChild className="h-11 rounded-xl px-3 text-sm font-semibold">
                        <Link to={`/portal/imoveis/${property.id}`} state={detailState}>Ver detalhes</Link>
                      </Button>
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        )}

        {!isResultsLoading && !isResultsError && filteredResults.length === 0 && (
          <div className="rounded-3xl border border-dashed border-border bg-card px-5 py-10">
            <EmptyState
              icon={query ? SearchIcon : Sparkles}
              title={query || hasLocalFilters ? "Nenhum imóvel para esta busca" : dealId ? "Nenhum imóvel disponível agora" : "Novas sugestões a caminho"}
              message={query || hasLocalFilters
                ? "Tente outro nome ou limpe os filtros para ver todas as opções."
                : dealId
                  ? "A imobiliária ainda não publicou opções para este perfil. Tente novamente mais tarde."
                  : "Seu corretor ainda está preparando uma seleção com base nas suas preferências."}
              action={query || hasLocalFilters ? (
                <Button type="button" variant="outline" onClick={() => { setQuery(""); clearLocalFilters(); }}>
                  Limpar busca
                </Button>
              ) : undefined}
            />
            {!query && !dealId && (
              <div className="mt-4 flex justify-center">
                <Button asChild variant="outline" className="rounded-xl">
                  <Link to="/portal/mensagens?contact=1"><MessageSquareText className="mr-2 size-4" aria-hidden="true" /> Falar com meu corretor</Link>
                </Button>
              </div>
            )}
          </div>
        )}
      </section>

      <AppointmentPickerDialog
        open={scheduleModalOpen}
        onOpenChange={(open) => {
          setScheduleModalOpen(open);
          if (!open && !isScheduling) setSelectedProperty(null);
        }}
        onSubmit={handleScheduleSubmit}
        propertyLabel={selectedProperty?.property.title}
      />
    </PortalPage>
  );
}

import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  Bath,
  Bed,
  BedDouble,
  Car,
  Check,
  Building2,
  ChevronLeft,
  ChevronRight,
  Image as ImageIcon,
  LoaderCircle,
  MapPin,
  Pencil,
  Share2,
  Square,
} from "lucide-react";
import { clsx } from "clsx";
import { Property, PropertyStatus, formatPropertyAddress } from "../data";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useAuth } from "../context/AuthContext";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { propertyService, type PropertyDTO } from "../services/property.service";
import { unwrapApiData, toGatewayUrl } from "../services/api";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";
import { getImgproxyUrl } from "../services/imgproxy";
import { SharedBadge } from "../components/SharedBadge";
import { Button } from "../ui/button";
import { Dialog, DialogContent } from "../ui/dialog";
import { EmptyState } from "../ui/EmptyState";
import { LoadError } from "../ui/load-error";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Switch } from "../ui/switch";
import { PropertyModal } from "../components/PropertyModal";
import { PropertyCommissionCard } from "../components/PropertyCommissionCard";
import { PropertySiteLink, PropertyWebPerformanceCard } from "../components/PropertyWebPerformanceCard";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs";
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from "../ui/carousel";
import { Can } from "../../lib/permissions/Can";
import { toast } from "sonner";
import { formatCurrencyBRL0 } from "../utils/format";
import { propertyFallbackQueryKey } from "../properties/queries";
import {
  propertyNeedsHydration,
  resolvePropertyDetailSnapshot,
} from "../lib/properties/detailState";
import { Skeleton } from "../ui/skeleton";

const PROPERTY_STATUSES: PropertyStatus[] = [
  "Em Captação",
  "Em Portfólio",
  "Anunciado",
  "Em Atendimento",
  "Em Negociação",
  "Em Proposta",
  "Em Análise",
  "Em Contrato",
  "Alugado",
  "Vendido",
  "Em Administração",
  "Em Rescisão",
  "Disponível",
  "Inativo",
];

const getStatusPill = (status: PropertyStatus) => {
  switch (status) {
    case "Em Captação":
      return "bg-stage-contract/10 text-stage-contract border-stage-contract/20";
    case "Em Portfólio":
    case "Anunciado":
      return "bg-primary-50 text-primary-700 border-primary-200";
    case "Em Negociação":
    case "Em Proposta":
      return "bg-status-warning-soft text-status-warning border-status-warning/30";
    case "Vendido":
    case "Em Rescisão":
      return "bg-status-critical-soft text-status-critical border-status-critical/30";
    case "Alugado":
    case "Em Contrato":
    case "Disponível":
      return "bg-status-ok-soft text-status-ok border-status-ok/30";
    default:
      return "bg-muted text-muted-foreground border-border";
  }
};

function PropertyDetailsSkeleton() {
  return (
    <div className="flex flex-col h-full bg-muted overflow-hidden" aria-label="Carregando imóvel">
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <Skeleton className="size-10 rounded-xl" />
            <div className="space-y-2">
              <Skeleton className="h-3 w-48" />
              <Skeleton className="h-6 w-64" />
            </div>
          </div>
          <div className="flex gap-2">
            <Skeleton className="h-9 w-24 rounded-xl" />
            <Skeleton className="h-9 w-28 rounded-xl" />
          </div>
        </div>
      </div>
      <div className="flex-1 p-6 lg:p-8">
        <div className="max-w-6xl mx-auto space-y-6">
          <div className="bg-card rounded-3xl border border-border overflow-hidden grid grid-cols-1 lg:grid-cols-5">
            <Skeleton className="lg:col-span-3 h-[280px] sm:h-[360px] lg:h-[440px] rounded-none" />
            <div className="lg:col-span-2 p-6 lg:p-8 space-y-5">
              <Skeleton className="h-12 w-full rounded-xl" />
              <Skeleton className="h-28 w-full rounded-2xl" />
              <div className="grid grid-cols-5 gap-3">
                {Array.from({ length: 5 }).map((_, index) => (
                  <Skeleton key={index} className="h-28 rounded-2xl" />
                ))}
              </div>
            </div>
          </div>
          <Skeleton className="h-11 w-full rounded-xl" />
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Skeleton className="h-36 rounded-2xl" />
            <Skeleton className="h-36 rounded-2xl" />
          </div>
        </div>
      </div>
    </div>
  );
}

function PropertyEditSkeleton() {
  return (
    <div className="flex flex-col h-full bg-card" aria-label="Carregando formulário do imóvel">
      <div className="flex-1 overflow-hidden p-6 space-y-8">
        {Array.from({ length: 3 }).map((_, section) => (
          <div key={section} className="space-y-4">
            <Skeleton className="h-5 w-52" />
            <div className="grid grid-cols-2 gap-4">
              <Skeleton className="col-span-2 h-12 rounded-xl" />
              <Skeleton className="h-12 rounded-xl" />
              <Skeleton className="h-12 rounded-xl" />
              <Skeleton className="col-span-2 h-24 rounded-xl" />
            </div>
          </div>
        ))}
      </div>
      <div className="flex-none px-6 py-4 border-t border-border flex justify-end gap-3">
        <Skeleton className="h-10 w-24 rounded-xl" />
        <Skeleton className="h-10 w-40 rounded-xl" />
      </div>
    </div>
  );
}

function PropertyEditError({
  onRetry,
  isRetrying,
}: {
  onRetry: () => void;
  isRetrying: boolean;
}) {
  return (
    <div className="flex h-full items-center justify-center bg-card p-8">
      <EmptyState
        icon={Building2}
        title="Não foi possível carregar o formulário"
        message="Os dados completos do imóvel são necessários para editar com segurança."
        className="max-w-md border-none bg-transparent"
        action={
          <Button onClick={onRetry} loading={isRetrying} loadingText="Tentando novamente…">
            Tentar novamente
          </Button>
        }
      />
    </div>
  );
}

export function PropertyDetails() {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { properties, updateProperty, updatePropertyStatus } = usePropertiesContext();
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  const { can } = usePermissionsContext();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();
  // `properties:create` nunca existiu no catálogo (ver Properties.tsx).
  const canWrite = can("properties:write") || can("properties:capture");

  // Owners are stored on the property as bare lead references and capturing
  // brokers as bare agent references — resolve their ids to display names so
  // the Histórico tab shows people, not UUIDs (mirrors PropertyModal pickers).
  const ownerNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const l of leads) m.set(l.id, l.name);
    return m;
  }, [leads]);
  const brokerNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agents) m.set(a.id, a.name);
    return m;
  }, [agents]);

  const fromContext = useMemo(
    () => (id ? properties.find((p) => p.id === id) : null),
    [properties, id],
  );

  const needsHydration = propertyNeedsHydration(fromContext);

  const fallbackQuery = useQuery({
    queryKey: propertyFallbackQueryKey(id ?? "", tenantId),
    queryFn: async () => {
      if (!id) return null;
      const res = await propertyService.getById(id);
      const dto = unwrapApiData<PropertyDTO>(res);
      const mapped = mapPropertyDtoToProperty(dto);
      return {
        ...mapped,
        owners: mapped.owners ?? [],
        capturingBrokers: mapped.capturingBrokers ?? [],
      };
    },
    enabled: Boolean(id && tenantId && needsHydration),
    staleTime: 30_000,
  });

  const property = resolvePropertyDetailSnapshot(fromContext, fallbackQuery.data);
  const hydratedEditProperty =
    !needsHydration
      ? property
      : fallbackQuery.data && !propertyNeedsHydration(fallbackQuery.data)
        ? resolvePropertyDetailSnapshot(fromContext, fallbackQuery.data)
        : null;

  const [current, setCurrent] = useState(0);
  const [editOpen, setEditOpen] = useState(false);
  const [statusOpen, setStatusOpen] = useState(false);
  const [statusSaving, setStatusSaving] = useState<PropertyStatus | null>(null);
  const [publicationSaving, setPublicationSaving] = useState(false);
  const [photosOpen, setPhotosOpen] = useState(false);
  const [photoIndex, setPhotoIndex] = useState(0);
  const [activeTab, setActiveTab] = useState("details");

  useEffect(() => {
    const edit = searchParams.get("edit");
    setEditOpen(edit === "1");
  }, [searchParams]);

  if (!property && fallbackQuery.isLoading) {
    return <PropertyDetailsSkeleton />;
  }

  if (!property && fallbackQuery.isError) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-muted p-8">
        <LoadError
          description="Não foi possível carregar este imóvel. A falha de leitura não foi tratada como imóvel inexistente."
          onRetry={() => fallbackQuery.refetch()}
          retrying={fallbackQuery.isFetching}
          className="border-none bg-transparent"
        />
      </div>
    );
  }

  if (!property) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-muted p-8">
        <EmptyState
          icon={Building2}
          title="Imóvel não encontrado"
          message="Não foi possível localizar este imóvel."
          className="border-none bg-transparent"
          action={<Button onClick={() => navigate(-1)}>Voltar</Button>}
        />
      </div>
    );
  }

  const gallery = (property.images?.length ? property.images : property.image ? [{ url: property.image }] : []).filter(
    (img) => img && typeof img.url === "string" && img.url.trim() !== "",
  );
  const hasNav = gallery.length > 1;

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      <ExpandableSidePanel
        open={editOpen && canWrite}
        onClose={() => {
          setEditOpen(false);
          const next = new URLSearchParams(searchParams);
          next.delete("edit");
          setSearchParams(next, { replace: true });
        }}
        defaultSize="4xl"
        expandedSize="full"
      >
        {needsHydration && fallbackQuery.isFetching && !hydratedEditProperty ? (
          <PropertyEditSkeleton />
        ) : !hydratedEditProperty ? (
          <PropertyEditError
            onRetry={() => {
              void fallbackQuery.refetch();
            }}
            isRetrying={fallbackQuery.isFetching}
          />
        ) : (
          <PropertyModal
            isOpen={editOpen}
            property={hydratedEditProperty}
            onClose={() => {
              setEditOpen(false);
              const next = new URLSearchParams(searchParams);
              next.delete("edit");
              setSearchParams(next, { replace: true });
            }}
          />
        )}
      </ExpandableSidePanel>
      <Dialog open={photosOpen} onOpenChange={setPhotosOpen}>
        <DialogContent className="max-w-5xl p-0 bg-black border-none">
          <div className="relative">
            <img
              src={getImgproxyUrl(toGatewayUrl(gallery[photoIndex]?.url || ""), { width: 1800, height: 1200 })}
              alt={property.title}
              className="w-full max-h-[80vh] object-contain bg-black"
            />
            {gallery.length > 1 ? (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => setPhotoIndex((i) => (i - 1 + gallery.length) % gallery.length)}
                  className="absolute left-3 top-1/2 -translate-y-1/2 text-white bg-black/40 hover:bg-black/60"
                  title="Anterior"
                >
                  <ChevronLeft className="w-5 h-5" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => setPhotoIndex((i) => (i + 1) % gallery.length)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-white bg-black/40 hover:bg-black/60"
                  title="Próxima"
                >
                  <ChevronRight className="w-5 h-5" />
                </Button>
              </>
            ) : null}
            <div className="absolute bottom-3 left-1/2 -translate-x-1/2 px-3 py-1 rounded-full bg-black/60 text-white text-xs font-bold">
              {gallery.length > 0 ? `${photoIndex + 1} / ${gallery.length}` : "—"}
            </div>
          </div>
        </DialogContent>
      </Dialog>
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-20">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <Button variant="ghost" size="icon" onClick={() => navigate(-1)}>
              <ArrowLeft className="w-5 h-5" />
            </Button>
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground font-medium truncate">
                REF: {property.id}
              </p>
              <h1 className="text-lg font-bold text-foreground truncate">{property.title}</h1>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            {canWrite ? (
              <Popover open={statusOpen} onOpenChange={setStatusOpen}>
                <PopoverTrigger asChild>
                  <button
                    type="button"
                    disabled={statusSaving !== null}
                    aria-busy={statusSaving !== null}
                    className={clsx(
                      "px-2.5 py-1 rounded-md border text-caption font-bold uppercase tracking-wider",
                      "hover:opacity-90 transition-opacity",
                      getStatusPill(property.status),
                    )}
                  >
                    {property.status}
                  </button>
                </PopoverTrigger>
                <PopoverContent align="end" className="w-72 p-2">
                  <div className="grid gap-1">
                    {PROPERTY_STATUSES.map((s) => (
                      <button
                        key={s}
                        type="button"
                        disabled={statusSaving !== null}
                        onClick={async () => {
                          setStatusSaving(s);
                          try {
                            const updated = await updatePropertyStatus(property.id, s);
                            if (!updated) return;
                            setStatusOpen(false);
                            toast.success(
                              s === "Anunciado"
                                ? "Status atualizado e imóvel publicado no site"
                                : "Status atualizado",
                            );
                          } finally {
                            setStatusSaving(null);
                          }
                        }}
                        className={clsx(
                          "w-full flex items-center justify-between gap-2 px-2.5 py-2 rounded-md text-xs font-bold transition-colors",
                          s === property.status
                            ? "bg-primary-50 text-primary-700"
                            : "hover:bg-muted text-foreground",
                        )}
                      >
                        <span className="truncate">{s}</span>
                        {statusSaving === s ? (
                          <LoaderCircle className="w-4 h-4 animate-spin" />
                        ) : s === property.status ? (
                          <Check className="w-4 h-4" />
                        ) : null}
                      </button>
                    ))}
                  </div>
                  <div className="mt-2 border-t border-border pt-2">
                    <div className="flex items-center justify-between gap-3 px-1 py-1">
                      <div className="min-w-0">
                        <p className="text-xs font-bold text-foreground">Publicar no site</p>
                        <p className="text-[11px] text-muted-foreground">
                          {property.status === "Anunciado"
                            ? "Visível na vitrine quando o site está ativo."
                            : "Mude o status para Anunciado para liberar."}
                        </p>
                      </div>
                      <Switch
                        checked={property.publicationStatus === "published"}
                        disabled={
                          !canWrite ||
                          property.status !== "Anunciado" ||
                          publicationSaving
                        }
                        aria-label="Publicar imóvel no site"
                        onCheckedChange={async (checked) => {
                          setPublicationSaving(true);
                          try {
                            const updated = await updateProperty(property.id, {
                              publicationStatus: checked ? "published" : "draft",
                            });
                            if (!updated) return;
                            toast.success(
                              checked ? "Imóvel publicado no site" : "Imóvel removido do site",
                            );
                          } finally {
                            setPublicationSaving(false);
                          }
                        }}
                      />
                    </div>
                  </div>
                </PopoverContent>
              </Popover>
            ) : (
              <span
                className={clsx(
                  "px-2.5 py-1 rounded-md border text-caption font-bold uppercase tracking-wider",
                  getStatusPill(property.status),
                )}
              >
                {property.status}
              </span>
            )}
            {canWrite ? (
              <Button variant="outline" onClick={() => navigate(`/imoveis/${property.id}?edit=1`)}>
                <Pencil className="w-4 h-4" /> Editar
              </Button>
            ) : null}
            <PropertySiteLink propertyId={property.id} />
            <Button
              variant="ghost"
              size="icon"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(window.location.href);
                  toast.success("Link copiado");
                } catch {
                  toast.error("Não foi possível copiar o link");
                }
              }}
              className="text-muted-foreground"
              title="Copiar link"
            >
              <Share2 className="w-4 h-4" />
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        <div className="max-w-6xl mx-auto space-y-6">
          <div className="bg-card rounded-3xl border border-border shadow-sm overflow-hidden grid grid-cols-1 lg:grid-cols-5">
            <div className="lg:col-span-3 bg-muted">
              {gallery.length > 0 ? (
                <Carousel
                  opts={{ loop: true }}
                  className="h-[280px] sm:h-[360px] lg:h-[440px]"
                  setApi={(api) => {
                    if (!api) return;
                    setCurrent(api.selectedScrollSnap());
                    api.on("select", () => setCurrent(api.selectedScrollSnap()));
                  }}
                >
                  <CarouselContent className="h-full">
                    {gallery.map((img, idx) => (
                      <CarouselItem key={`${img.url}-${idx}`} className="h-full">
                        <img
                          src={getImgproxyUrl(toGatewayUrl(img.url), { width: 1400, height: 900 })}
                          alt={property.title}
                          className="w-full h-full object-cover"
                        />
                      </CarouselItem>
                    ))}
                  </CarouselContent>
                  {hasNav ? (
                    <>
                      <CarouselPrevious className="left-4 top-1/2 -translate-y-1/2" />
                      <CarouselNext className="right-4 top-1/2 -translate-y-1/2" />
                    </>
                  ) : null}
                </Carousel>
              ) : (
                <div className="h-[280px] sm:h-[360px] lg:h-[440px] flex items-center justify-center text-muted-foreground">
                  <Building2 className="w-10 h-10" />
                </div>
              )}
              {gallery.length > 0 ? (
                <div className="px-5 py-3 text-xs text-muted-foreground bg-card border-t border-border flex items-center justify-between gap-3">
                  <span>{`${current + 1} / ${gallery.length}`}</span>
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="truncate">{property.type}</span>
                    {gallery.length > 1 ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setPhotoIndex(current);
                          setPhotosOpen(true);
                        }}
                        className="text-xs font-bold text-muted-foreground"
                      >
                        <ImageIcon className="w-4 h-4" /> Ver fotos ({gallery.length})
                      </Button>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </div>

            <div className="lg:col-span-2 p-6 lg:p-8 space-y-5">
              <div className="space-y-2">
                {property.sharedFrom?.companyName ? (
                  <SharedBadge companyName={property.sharedFrom.companyName} size="md" />
                ) : null}
                <div className="flex items-start gap-3 text-muted-foreground">
                  <MapPin className="w-5 h-5 text-primary-500 shrink-0 mt-0.5" />
                  <span className="text-sm font-medium leading-snug">
                    {formatPropertyAddress(property.address)}
                  </span>
                </div>
              </div>

              <div className="rounded-2xl border border-border bg-card p-4">
                <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
                  Valor
                </p>
                <p className="text-2xl font-bold text-foreground">{formatCurrencyBRL0(property.price || 0)}</p>
              </div>

              <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                <div className="p-3 rounded-2xl border border-border bg-muted/60 flex flex-col items-center">
                  <Bed className="w-5 h-5 text-muted-foreground mb-1" />
                  <span className="text-lg font-bold text-foreground">{property.beds}</span>
                  <span className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
                    Quartos
                  </span>
                </div>
                <div className="p-3 rounded-2xl border border-border bg-muted/60 flex flex-col items-center">
                  <BedDouble className="w-5 h-5 text-muted-foreground mb-1" />
                  <span className="text-lg font-bold text-foreground">{property.suites || 0}</span>
                  <span className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
                    Suítes
                  </span>
                </div>
                <div className="p-3 rounded-2xl border border-border bg-muted/60 flex flex-col items-center">
                  <Bath className="w-5 h-5 text-muted-foreground mb-1" />
                  <span className="text-lg font-bold text-foreground">{property.baths}</span>
                  <span className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
                    Banheiros
                  </span>
                </div>
                <div className="p-3 rounded-2xl border border-border bg-muted/60 flex flex-col items-center">
                  <Car className="w-5 h-5 text-muted-foreground mb-1" />
                  <span className="text-lg font-bold text-foreground">{property.parkingSpaces || 0}</span>
                  <span className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
                    Vagas
                  </span>
                </div>
                <div className="p-3 rounded-2xl border border-border bg-muted/60 flex flex-col items-center">
                  <Square className="w-5 h-5 text-muted-foreground mb-1" />
                  <span className="text-lg font-bold text-foreground">{property.area}</span>
                  <span className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
                    m²
                  </span>
                </div>
              </div>
            </div>
          </div>

          <PropertyWebPerformanceCard propertyId={property.id} />

          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList className="grid grid-cols-2 lg:grid-cols-4 w-full">
              <TabsTrigger value="details">Detalhes & Características</TabsTrigger>
              <TabsTrigger value="finance">Financeiro & Avaliação</TabsTrigger>
              <TabsTrigger value="docs">Documentação</TabsTrigger>
              <TabsTrigger value="history">Histórico & CRM</TabsTrigger>
            </TabsList>

            <TabsContent value="details" className="mt-6">
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">
                    Descrição
                  </p>
                  <p className="text-sm text-muted-foreground whitespace-pre-wrap">
                    {property.description || "Nenhuma descrição cadastrada."}
                  </p>
                </div>
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">
                    Comodidades
                  </p>
                  {property.amenities && property.amenities.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {property.amenities.map((a) => (
                        <span
                          key={a}
                          className="px-2.5 py-1 rounded-full border border-border bg-muted text-xs font-bold text-foreground"
                        >
                          {a}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">Nenhuma comodidade cadastrada.</p>
                  )}
                </div>
              </div>
            </TabsContent>

            <TabsContent value="finance" className="mt-6">
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
                    Condomínio
                  </p>
                  <p className="text-xl font-bold text-foreground">{formatCurrencyBRL0(property.condominiumFee || 0)}/mês</p>
                </div>
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">IPTU</p>
                  <p className="text-xl font-bold text-foreground">{formatCurrencyBRL0(property.propertyTax || 0)}/ano</p>
                </div>
                <PropertyCommissionCard
                  listingType={property.listingType}
                  availableFor={property.availableFor}
                />
              </div>
            </TabsContent>

            <TabsContent value="docs" className="mt-6">
              <div className="rounded-2xl border border-border bg-card p-6 flex items-start justify-between gap-6">
                <div className="min-w-0">
                  <p className="text-sm font-bold text-foreground">Documentos do Imóvel</p>
                  <p className="text-sm text-muted-foreground mt-1">
                    A listagem de documentos por imóvel ainda não está integrada nesta tela.
                  </p>
                </div>
                <Button variant="outline" onClick={() => navigate("/documentos")}>
                  Abrir documentos
                </Button>
              </div>
            </TabsContent>

            <TabsContent value="history" className="mt-6">
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">
                    Corretores captadores
                  </p>
                  {property.capturingBrokers && property.capturingBrokers.length > 0 ? (
                    <div className="space-y-2">
                      {property.capturingBrokers.map((b) => (
                        <div
                          key={`${b.id}-${b.role}`}
                          className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 px-3 py-2"
                        >
                          <span className="text-sm font-bold text-foreground truncate">{brokerNameById.get(b.id) || b.id}</span>
                          <span className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                            {b.role}
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">Nenhum captador vinculado.</p>
                  )}
                </div>
                <div className="rounded-2xl border border-border bg-card p-5">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">
                    Proprietários
                  </p>
                  {property.owners && property.owners.length > 0 ? (
                    <div className="space-y-2">
                      {property.owners.map((o) => (
                        <div
                          key={o.id}
                          className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 px-3 py-2"
                        >
                          <span className="text-sm font-bold text-foreground truncate">{ownerNameById.get(o.id) || o.id}</span>
                          <span className="text-xs font-bold text-muted-foreground">
                            {o.percentage !== undefined ? `${o.percentage}%` : "—"}
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">Nenhum proprietário vinculado.</p>
                  )}
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </div>
  );
}

import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import {
  Building2,
  LayoutGrid,
  List,
  Map as MapIcon,
  MoreVertical,
  Plus,
  RefreshCw,
  Search,
} from "lucide-react";
import { clsx } from "clsx";
import { Property, formatPropertyAddress, PropertyStatus } from "../data";
import { formatMoneyBR } from "../utils/format";

const BUSINESS_TYPE_LABEL: Record<string, string> = { sale: "Venda", venda: "Venda", rent: "Locação", locacao: "Locação", exchange: "Permuta", permuta: "Permuta" };
const BUSINESS_TYPE_TONE: Record<string, string> = {
  sale: "bg-status-info-soft text-status-info border-status-info/30",
  venda: "bg-status-info-soft text-status-info border-status-info/30",
  rent: "bg-status-ok-soft text-status-ok border-status-ok/30",
  locacao: "bg-status-ok-soft text-status-ok border-status-ok/30",
  exchange: "bg-status-warning-soft text-status-warning border-status-warning/30",
  permuta: "bg-status-warning-soft text-status-warning border-status-warning/30",
};
import { usePropertiesContext } from "../context/PropertiesContext";
import { usePermissionsContext } from "../context/PermissionsContext";
import { PropertyMap } from "../components/PropertyMap";
import { PropertyModal } from "../components/PropertyModal";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { SharedBadge } from "../components/SharedBadge";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { PageHeader } from "../ui/page-header";
import { toast } from "sonner";
import { Can } from "../../lib/permissions/Can";
import { Tabs, TabsList, TabsTrigger } from "../ui/tabs";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/dropdown-menu";
import { propertyService, type PropertyDTO } from "../services/property.service";
import { toGatewayUrl, unwrapApiData } from "../services/api";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";

type ViewMode = "list" | "map";

type StatusTab = "all" | "cap_port" | "anunciado" | "negociacao" | "fechados";

const STATUS_TABS: { value: StatusTab; label: string; statuses?: PropertyStatus[] }[] = [
  { value: "all", label: "Todos" },
  { value: "cap_port", label: "Captação & Portfólio", statuses: ["Em Captação", "Em Portfólio"] },
  { value: "anunciado", label: "Anunciado", statuses: ["Anunciado", "Disponível"] },
  {
    value: "negociacao",
    label: "Em Negociação",
    statuses: ["Em Atendimento", "Em Negociação", "Em Proposta", "Em Análise", "Em Contrato"],
  },
  {
    value: "fechados",
    label: "Fechados",
    statuses: ["Alugado", "Vendido", "Em Administração", "Em Rescisão", "Inativo"],
  },
];

export function Properties() {
  const navigate = useNavigate();
  const { properties, fetchProperties } = usePropertiesContext();
  const { can } = usePermissionsContext();
  // `properties:create` nunca existiu no catálogo: a criação de imóvel é a
  // captação, gated por `properties:capture` (toca-core/router.go:93).
  const canWrite = can("properties:write") || can("properties:capture");

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [statusTab, setStatusTab] = useState<StatusTab>("all");
  const [viewMode, setViewMode] = useState<ViewMode>("list");
  const [businessType, setBusinessType] = useState<"todos" | "sale" | "rent">("todos");
  const [createOpen, setCreateOpen] = useState(false);
  const [editProperty, setEditProperty] = useState<Property | null>(null);
  const [editLoadingId, setEditLoadingId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchProperties()
      .catch(() => {
        if (!cancelled) setError("Não foi possível carregar os imóveis.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [fetchProperties]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const tab = STATUS_TABS.find((t) => t.value === statusTab);
    const allowed = tab?.statuses ? new Set<PropertyStatus>(tab.statuses) : null;
    return properties.filter((p) => {
      if (allowed && !allowed.has(p.status)) return false;
      if (businessType !== "todos") {
        const types = p.availableFor?.length ? p.availableFor : (p.listingType ? [p.listingType] : []);
        if (!types.includes(businessType)) return false;
      }
      if (!q) return true;
      const hay = `${p.id} ${p.displayId || ""} ${p.title} ${formatPropertyAddress(p.address)} ${p.type}`.toLowerCase();
      return hay.includes(q);
    });
  }, [properties, search, statusTab, businessType]);

  const handleReload = async () => {
    setLoading(true);
    setError(null);
    try {
      await fetchProperties();
    } catch {
      setError("Não foi possível carregar os imóveis.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-10">
        <div className="flex flex-col gap-4">
          <PageHeader
            title="Estoque de Imóveis"
            description="Busque e gerencie os imóveis do portfólio"
          >
            <div className="flex shrink-0 items-center gap-2">
              <Can anyOf={["properties:write", "properties:capture"]}>
                <Button onClick={() => setCreateOpen(true)} className="w-full sm:w-auto">
                  <Plus className="w-4 h-4" />
                  <span className="hidden sm:inline">Novo Cadastro</span>
                </Button>
              </Can>
            </div>
          </PageHeader>

          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full sm:w-72">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Buscar por ID, bairro ou título…"
                className="w-full pl-9 pr-4 py-2 bg-muted border-transparent rounded-lg text-sm focus:bg-card focus:border-primary-500 focus:ring-2 focus:ring-primary-200 outline-none transition-all"
              />
            </div>

            <div className="hidden sm:flex items-center rounded-lg border border-border overflow-hidden bg-muted">
              {(["todos", "sale", "rent"] as const).map((bt) => (
                <button
                  key={bt}
                  type="button"
                  onClick={() => setBusinessType(bt)}
                  className={clsx(
                    "px-3 py-2 text-sm font-medium",
                    businessType === bt ? "bg-card text-foreground" : "text-muted-foreground hover:text-foreground",
                    bt !== "todos" && "border-l border-border",
                  )}
                >
                  {bt === "todos" ? "Todos" : bt === "sale" ? "Venda" : "Locação"}
                </button>
              ))}
            </div>

            <div className="hidden sm:flex items-center rounded-lg border border-border overflow-hidden bg-muted">
              <button
                type="button"
                onClick={() => setViewMode("list")}
                className={clsx(
                  "px-3 py-2 text-sm font-medium flex items-center gap-2",
                  viewMode === "list"
                    ? "bg-card text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                <List className="w-4 h-4" /> Lista
              </button>
              <button
                type="button"
                onClick={() => setViewMode("map")}
                className={clsx(
                  "px-3 py-2 text-sm font-medium flex items-center gap-2 border-l border-border",
                  viewMode === "map"
                    ? "bg-card text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                <MapIcon className="w-4 h-4" /> Mapa
              </button>
            </div>

            <Button variant="outline" onClick={handleReload} disabled={loading}>
              <RefreshCw className="w-4 h-4" />
              <span className="hidden sm:inline">Recarregar</span>
            </Button>
          </div>
        </div>

        <div className="mt-4">
          <Tabs value={statusTab} onValueChange={(v) => setStatusTab(v as StatusTab)}>
            {/* Mobile: 2 colunas com o rótulo podendo quebrar linha. O
                `whitespace-nowrap` + altura fixa (h-9) do TabsTrigger base
                fazia os rótulos longos ("Captação & Portfólio", "Em
                Negociação") transbordarem a célula e estourarem a largura da
                tela. De `sm` para cima volta à linha única de 5 colunas. */}
            <TabsList className="grid h-auto w-full grid-cols-2 items-stretch gap-1 sm:h-9 sm:grid-cols-5 sm:gap-0">
              {STATUS_TABS.map((t) => (
                <TabsTrigger
                  key={t.value}
                  value={t.value}
                  className="h-auto min-h-8 whitespace-normal px-2 py-1.5 text-center text-xs leading-tight sm:h-[calc(100%-1px)] sm:whitespace-nowrap sm:text-sm"
                >
                  {t.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        {loading ? (
          <div className="text-center py-10 text-muted-foreground">Carregando imóveis...</div>
        ) : error ? (
          <EmptyState
            icon={Building2}
            title="Não foi possível carregar"
            message={error}
            className="border-none bg-transparent"
            action={
              <Button onClick={handleReload}>
                <RefreshCw className="w-4 h-4" /> Tentar novamente
              </Button>
            }
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={LayoutGrid}
            title="Nenhum imóvel encontrado"
            message="Tente ajustar os filtros ou criar um novo imóvel."
            className="border-none bg-transparent"
          />
        ) : viewMode === "map" ? (
          <PropertyMap
            properties={filtered}
            height={520}
            onSelectProperty={(p) => navigate(`/imoveis/${p.id}`)}
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
            {filtered.map((p) => (
              <div
                key={p.id}
                className="text-left bg-card rounded-2xl border border-border shadow-sm overflow-hidden hover:shadow-md transition-shadow group relative"
              >
                <div className="aspect-[16/10] bg-muted overflow-hidden">
                  {p.image ? (
                    <img src={toGatewayUrl(p.image)} alt={p.title} className="w-full h-full object-cover" />
                  ) : (
                    <div className="w-full h-full flex items-center justify-center text-muted-foreground/60">
                      <Building2 className="w-8 h-8" />
                    </div>
                  )}
                </div>
                <div className="p-5 space-y-2">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <button
                        type="button"
                        onClick={() => navigate(`/imoveis/${p.id}`)}
                        title={p.title}
                        className="text-base font-bold text-foreground truncate group-hover:text-primary-600 transition-colors"
                      >
                        {p.title}
                      </button>
                      <p className="text-xs text-muted-foreground truncate">
                        {formatPropertyAddress(p.address)}
                      </p>
                    </div>
                    {p.sharedFrom?.companyName ? (
                      <SharedBadge companyName={p.sharedFrom.companyName} />
                    ) : null}
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5 pt-1">
                    {(p.availableFor && p.availableFor.length > 0 ? p.availableFor : p.listingType ? [p.listingType] : []).map((t) => {
                      const tone = BUSINESS_TYPE_TONE[String(t).toLowerCase()] || "status-info";
                      return (
                        <span
                          key={t}
                          className={"px-2 py-0.5 rounded-md text-[11px] font-bold uppercase tracking-wide " + (BUSINESS_TYPE_TONE[String(t).toLowerCase()] || "bg-status-info-soft text-status-info border-status-info/30")}
                        >
                          {BUSINESS_TYPE_LABEL[String(t).toLowerCase()] || t}
                        </span>
                      );
                    })}
                  </div>
                  {(p.salePrice ?? 0) > 0 && (
                    <p className="text-sm font-bold text-foreground">Venda: {formatMoneyBR(p.salePrice)}</p>
                  )}
                  {(p.rentPrice ?? 0) > 0 && (
                    <p className="text-sm font-bold text-foreground">Locação: {formatMoneyBR(p.rentPrice)}<span className="text-muted-foreground font-medium">/mês</span></p>
                  )}
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-xs text-muted-foreground">{p.type}</span>
                    <span className="text-xs font-bold text-foreground">{p.status}</span>
                  </div>
                </div>
                <div className="absolute top-3 right-3">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="bg-black/30 hover:bg-black/40 text-white">
                        <MoreVertical className="w-4 h-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => navigate(`/imoveis/${p.id}`)}>
                        Abrir detalhes
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={async () => {
                          if (!canWrite) return;
                          try {
                            setEditLoadingId(p.id);
                            const res = await propertyService.getById(p.id);
                            const dto = unwrapApiData<PropertyDTO>(res);
                            setEditProperty(mapPropertyDtoToProperty(dto));
                          } catch {
                            toast.error("Não foi possível carregar os detalhes do imóvel.");
                          } finally {
                            setEditLoadingId(null);
                          }
                        }}
                        disabled={!canWrite || editLoadingId === p.id}
                      >
                        Editar
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <ExpandableSidePanel open={createOpen} onClose={() => setCreateOpen(false)} defaultSize="4xl" expandedSize="full">
        <PropertyModal isOpen={createOpen} onClose={() => setCreateOpen(false)} />
      </ExpandableSidePanel>
      <ExpandableSidePanel open={!!editProperty} onClose={() => setEditProperty(null)} defaultSize="4xl" expandedSize="full">
        {editProperty ? (
          <PropertyModal
            isOpen={!!editProperty}
            property={editProperty}
            onClose={() => setEditProperty(null)}
          />
        ) : null}
      </ExpandableSidePanel>
    </div>
  );
}

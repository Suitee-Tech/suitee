import { useMemo, useState } from "react";
import { Link } from "react-router";
import {
  Briefcase,
  Building2,
  LayoutGrid,
  List as ListIcon,
  Plus,
  Search,
  User,
} from "lucide-react";
import { clsx } from "clsx";

import { usePropertiesContext } from "../context/PropertiesContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { CaptacaoWizard } from "../components/CaptacaoWizard";
import { Can } from "../../lib/permissions/Can";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";

const CAPTURE_STAGES = [
  {
    id: "Em Captação",
    label: "Em Captação",
    colorBg: "bg-stage-contract/10",
    colorText: "text-stage-contract",
    colorBorder: "border-stage-contract/30",
  },
  {
    id: "Em Portfólio",
    label: "Em Portfólio",
    colorBg: "bg-primary-50",
    colorText: "text-primary-700",
    colorBorder: "border-primary-200",
  },
  {
    id: "Anunciado",
    label: "Anunciado",
    colorBg: "bg-status-info-soft",
    colorText: "text-status-info",
    colorBorder: "border-status-info/30",
  },
] as const;

type CaptureStageId = (typeof CAPTURE_STAGES)[number]["id"];

export function CaptacoesPage() {
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();
  const [view, setView] = useState<"list" | "kanban">("list");
  const [stageFilter, setStageFilter] = useState<CaptureStageId | "all">("all");
  const [search, setSearch] = useState("");
  const [wizardOpen, setWizardOpen] = useState(false);

  const captacaoStatuses: CaptureStageId[] = useMemo(
    () => CAPTURE_STAGES.map((s) => s.id) as CaptureStageId[],
    [],
  );

  const filtered = useMemo(() => {
    return properties.filter((p) => {
      if (!captacaoStatuses.includes(p.status as CaptureStageId)) return false;
      if (stageFilter !== "all" && p.status !== stageFilter) return false;
      if (search.trim() !== "") {
        const q = search.trim().toLowerCase();
        const haystack = `${p.title || ""} ${p.id || ""}`.toLowerCase();
        if (!haystack.includes(q)) return false;
      }
      return true;
    });
  }, [properties, captacaoStatuses, stageFilter, search]);

  const ownerOf = (propertyId: string) => {
    const prop = properties.find((p) => p.id === propertyId);
    if (!prop?.owners?.length) return null;
    return leads.find((l) => l.id === prop.owners![0].id);
  };

  const captadorOf = (propertyId: string) => {
    const prop = properties.find((p) => p.id === propertyId);
    if (!prop?.capturingBrokers?.length) return null;
    const cap =
      prop.capturingBrokers.find((cb) => cb.role === "captador") ||
      prop.capturingBrokers[0];
    return agents.find((a) => a.id === cap.id);
  };

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-20">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h1 className="text-xl font-bold text-foreground flex items-center gap-2">
              <Briefcase className="w-5 h-5 text-primary-600" /> Captações
            </h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Imóveis no portfólio antes de virarem negócio
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2 w-full sm:w-auto">
            <div className="relative flex-1 sm:w-64">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Buscar por título ou ID…"
                className="w-full pl-10 pr-4 py-2 rounded-lg border border-border bg-card text-sm focus:ring-2 focus:ring-primary/30 focus:outline-none"
              />
            </div>
            <div className="flex border border-border rounded-lg overflow-hidden">
              <button
                type="button"
                onClick={() => setView("list")}
                className={clsx(
                  "p-2 transition-colors",
                  view === "list"
                    ? "bg-primary-50 text-primary-700"
                    : "text-muted-foreground hover:bg-muted",
                )}
                title="Lista"
              >
                <ListIcon className="w-4 h-4" />
              </button>
              <button
                type="button"
                onClick={() => setView("kanban")}
                className={clsx(
                  "p-2 transition-colors border-l border-border",
                  view === "kanban"
                    ? "bg-primary-50 text-primary-700"
                    : "text-muted-foreground hover:bg-muted",
                )}
                title="Kanban"
              >
                <LayoutGrid className="w-4 h-4" />
              </button>
            </div>
            <Can permission="properties:capture">
              <Button onClick={() => setWizardOpen(true)}>
                <Plus className="w-4 h-4" /> Iniciar Captação
              </Button>
            </Can>
          </div>
        </div>

        <div className="flex items-center gap-2 mt-4 overflow-x-auto pb-1">
          <button
            type="button"
            onClick={() => setStageFilter("all")}
            className={clsx(
              "px-3 py-1.5 text-xs font-bold rounded-full whitespace-nowrap transition-colors",
              stageFilter === "all"
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground",
            )}
          >
            Todas ({filtered.length})
          </button>
          {CAPTURE_STAGES.map((s) => {
            const count = properties.filter((p) => p.status === s.id).length;
            return (
              <button
                key={s.id}
                type="button"
                onClick={() => setStageFilter(s.id)}
                className={clsx(
                  "px-3 py-1.5 text-xs font-bold rounded-full whitespace-nowrap transition-colors",
                  stageFilter === s.id
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:text-foreground",
                )}
              >
                {s.label} ({count})
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        {filtered.length === 0 ? (
          <EmptyState
            icon={Briefcase}
            message="Nenhuma captação no momento. Use o botão Iniciar Captação para começar."
          />
        ) : view === "list" ? (
          <div className="bg-card rounded-2xl border border-border overflow-hidden">
            <table className="w-full">
              <thead className="bg-muted/50 text-xs font-bold text-muted-foreground uppercase tracking-wider">
                <tr>
                  <th className="px-6 py-3 text-left">Imóvel</th>
                  <th className="px-6 py-3 text-left">Proprietário</th>
                  <th className="px-6 py-3 text-left">Captador</th>
                  <th className="px-6 py-3 text-left">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {filtered.map((p) => {
                  const owner = ownerOf(p.id);
                  const captador = captadorOf(p.id);
                  const stage = CAPTURE_STAGES.find((s) => s.id === p.status);
                  return (
                    <tr
                      key={p.id}
                      className="hover:bg-muted/30 transition-colors"
                    >
                      <td className="px-6 py-4">
                        <Link to={`/imoveis/${p.id}`} className="block">
                          <p className="text-sm font-bold text-foreground hover:text-primary-700 transition-colors truncate max-w-[280px]">
                            {p.title}
                          </p>
                          <p className="text-xs text-muted-foreground mt-0.5">
                            #{p.id.slice(0, 8)}
                          </p>
                        </Link>
                      </td>
                      <td className="px-6 py-4">
                        {owner ? (
                          <Link
                            to={`/leads/${owner.id}`}
                            className="text-sm text-foreground hover:text-primary-700 inline-flex items-center gap-1.5"
                          >
                            <User className="w-3.5 h-3.5 text-muted-foreground" />
                            {owner.name}
                          </Link>
                        ) : (
                          <span className="text-xs text-muted-foreground italic">
                            —
                          </span>
                        )}
                      </td>
                      <td className="px-6 py-4">
                        {captador ? (
                          <span className="text-sm text-foreground">
                            {captador.name}
                          </span>
                        ) : (
                          <span className="text-xs text-muted-foreground italic">
                            —
                          </span>
                        )}
                      </td>
                      <td className="px-6 py-4">
                        <span
                          className={clsx(
                            "px-2.5 py-1 text-xs font-bold rounded-full border",
                            stage?.colorBg,
                            stage?.colorText,
                            stage?.colorBorder,
                          )}
                        >
                          {stage?.label || p.status}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {CAPTURE_STAGES.map((stage) => {
              const items = filtered.filter((p) => p.status === stage.id);
              return (
                <div
                  key={stage.id}
                  className={clsx(
                    "rounded-2xl border-2 p-4",
                    stage.colorBorder,
                    stage.colorBg,
                  )}
                >
                  <h3
                    className={clsx(
                      "text-sm font-bold uppercase tracking-wide mb-3 flex items-center justify-between",
                      stage.colorText,
                    )}
                  >
                    <span>{stage.label}</span>
                    <span className="text-xs">{items.length}</span>
                  </h3>
                  <div className="space-y-2">
                    {items.length === 0 && (
                      <p className="text-xs text-muted-foreground italic text-center py-4">
                        Vazio
                      </p>
                    )}
                    {items.map((p) => {
                      const owner = ownerOf(p.id);
                      const captador = captadorOf(p.id);
                      return (
                        <Link
                          key={p.id}
                          to={`/imoveis/${p.id}`}
                          className="block bg-card rounded-xl border border-border p-3 hover:shadow-md hover:border-primary-300 transition-all"
                        >
                          <p className="text-sm font-bold text-foreground line-clamp-2">
                            {p.title}
                          </p>
                          {owner && (
                            <p className="text-xs text-muted-foreground mt-1.5 flex items-center gap-1">
                              <User className="w-3 h-3" /> {owner.name}
                            </p>
                          )}
                          {captador && (
                            <p className="text-xs text-muted-foreground mt-0.5 flex items-center gap-1">
                              <Building2 className="w-3 h-3" />{" "}
                              {captador.name}
                            </p>
                          )}
                        </Link>
                      );
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <CaptacaoWizard open={wizardOpen} onClose={() => setWizardOpen(false)} />
    </div>
  );
}

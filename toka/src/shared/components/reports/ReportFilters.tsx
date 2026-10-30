import { useState, useRef, useEffect } from "react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { Filter, X, User, Building2, CalendarRange, ChevronDown, RotateCcw } from "lucide-react";
import { Button } from "../../ui/button";

export interface ReportFilterState {
  agentIds: string[];
  propertyTypes: string[];
  dateFrom: string;
  dateTo: string;
}

interface ReportFiltersProps {
  agents: { id: string; name: string }[];
  propertyTypes: string[];
  filters: ReportFilterState;
  onChange: (filters: ReportFilterState) => void;
  onReset: () => void;
}

export const EMPTY_FILTERS: ReportFilterState = {
  agentIds: [],
  propertyTypes: [],
  dateFrom: "",
  dateTo: "",
};

export function hasActiveFilters(f: ReportFilterState): boolean {
  return f.agentIds.length > 0 || f.propertyTypes.length > 0 || f.dateFrom !== "" || f.dateTo !== "";
}

export function ReportFilters({ agents, propertyTypes, filters, onChange, onReset }: ReportFiltersProps) {
  const [expanded, setExpanded] = useState(false);
  const [agentOpen, setAgentOpen] = useState(false);
  const [typeOpen, setTypeOpen] = useState(false);
  const agentRef = useRef<HTMLDivElement>(null);
  const typeRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (agentRef.current && !agentRef.current.contains(e.target as Node)) setAgentOpen(false);
      if (typeRef.current && !typeRef.current.contains(e.target as Node)) setTypeOpen(false);
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const activeCount =
    (filters.agentIds.length > 0 ? 1 : 0) + (filters.propertyTypes.length > 0 ? 1 : 0) + (filters.dateFrom || filters.dateTo ? 1 : 0);

  const toggleAgent = (id: string) => {
    const next = filters.agentIds.includes(id) ? filters.agentIds.filter((a) => a !== id) : [...filters.agentIds, id];
    onChange({ ...filters, agentIds: next });
  };

  const toggleType = (t: string) => {
    const next = filters.propertyTypes.includes(t)
      ? filters.propertyTypes.filter((x) => x !== t)
      : [...filters.propertyTypes, t];
    onChange({ ...filters, propertyTypes: next });
  };

  return (
    <div className={clsx("rounded-xl border shadow-sm", "bg-card border-border")}>
      <button
        onClick={() => setExpanded((e) => !e)}
        className={clsx("w-full flex items-center justify-between px-4 py-3 transition-colors rounded-xl", "hover:bg-muted/50")}
      >
        <div className={clsx("flex items-center gap-2 text-sm font-bold", "text-foreground")}>
          <Filter className="w-4 h-4 text-muted-foreground" />
          Filtros Interativos
          {activeCount > 0 && (
            <span className="px-2 py-0.5 bg-primary-100 text-primary-700 text-micro font-bold rounded-full">
              {activeCount} ativo{activeCount > 1 ? "s" : ""}
            </span>
          )}
        </div>
        <ChevronDown className={clsx("w-4 h-4 text-muted-foreground transition-transform", expanded && "rotate-180")} />
      </button>

      <AnimatePresence>
        {expanded && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            // Sem `overflow-hidden`: ele recortava as listas de Agente e Tipo de
            // Imóvel, que abrem para baixo além da altura do painel. O z-50 das
            // listas não resolvia porque o corte é do ancestral, não ordem de
            // empilhamento. Durante a animação de entrada o conteúdo fica oculto
            // pela própria opacidade.
            style={{ willChange: "opacity" }}
          >
            <div className={clsx("px-4 pb-4 pt-1 flex flex-wrap items-end gap-4 border-t", "border-border")}>
              <div className="relative min-w-[180px]" ref={agentRef}>
                <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1.5">Agente</label>
                <button
                  onClick={() => {
                    setAgentOpen((o) => !o);
                    setTypeOpen(false);
                  }}
                  className={clsx(
                    "w-full flex items-center justify-between gap-2 px-3 py-2 border rounded-lg text-sm transition-colors",
                    filters.agentIds.length > 0
                      ? "border-primary-300 bg-primary-50/50 text-primary-700"
                      : "border-border bg-card text-foreground hover:border-border",
                  )}
                >
                  <span className="flex items-center gap-1.5 truncate">
                    <User className="w-3.5 h-3.5 flex-shrink-0" />
                    {filters.agentIds.length === 0
                      ? "Todos"
                      : `${filters.agentIds.length} selecionado${filters.agentIds.length > 1 ? "s" : ""}`}
                  </span>
                  <ChevronDown className="w-3.5 h-3.5 flex-shrink-0" />
                </button>
                <AnimatePresence>
                  {agentOpen && (
                    <motion.div
                      initial={{ opacity: 0, y: -4 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0, y: -4 }}
                      className={clsx(
                        "absolute left-0 top-full mt-1 border rounded-xl shadow-lg py-1 min-w-[200px] z-50 max-h-[220px] overflow-y-auto",
                        "bg-card border-border",
                      )}
                    >
                      {agents.map((agent) => (
                        <button
                          key={agent.id}
                          onClick={() => toggleAgent(agent.id)}
                          className={clsx(
                            "w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-muted transition-colors",
                            filters.agentIds.includes(agent.id) && "bg-primary-50/50 text-primary-700 font-bold",
                          )}
                        >
                          <div
                            className={clsx(
                              "w-4 h-4 rounded border flex items-center justify-center transition-colors",
                              filters.agentIds.includes(agent.id) ? "bg-primary border-primary text-primary-foreground" : "border-border",
                            )}
                          >
                            {filters.agentIds.includes(agent.id) && (
                              <svg className="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                              </svg>
                            )}
                          </div>
                          {agent.name}
                        </button>
                      ))}
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>

              <div className="relative min-w-[180px]" ref={typeRef}>
                <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1.5">
                  Tipo de Imóvel
                </label>
                <button
                  onClick={() => {
                    setTypeOpen((o) => !o);
                    setAgentOpen(false);
                  }}
                  className={clsx(
                    "w-full flex items-center justify-between gap-2 px-3 py-2 border rounded-lg text-sm transition-colors",
                    filters.propertyTypes.length > 0
                      ? "border-primary-300 bg-primary-50/50 text-primary-700"
                      : "border-border bg-card text-foreground hover:border-border",
                  )}
                >
                  <span className="flex items-center gap-1.5 truncate">
                    <Building2 className="w-3.5 h-3.5 flex-shrink-0" />
                    {filters.propertyTypes.length === 0
                      ? "Todos"
                      : `${filters.propertyTypes.length} selecionado${filters.propertyTypes.length > 1 ? "s" : ""}`}
                  </span>
                  <ChevronDown className="w-3.5 h-3.5 flex-shrink-0" />
                </button>
                <AnimatePresence>
                  {typeOpen && (
                    <motion.div
                      initial={{ opacity: 0, y: -4 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0, y: -4 }}
                      className={clsx(
                        "absolute left-0 top-full mt-1 border rounded-xl shadow-lg py-1 min-w-[200px] z-50 max-h-[220px] overflow-y-auto",
                        "bg-card border-border",
                      )}
                    >
                      {propertyTypes.map((t) => (
                        <button
                          key={t}
                          onClick={() => toggleType(t)}
                          className={clsx(
                            "w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-muted transition-colors",
                            filters.propertyTypes.includes(t) && "bg-primary-50/50 text-primary-700 font-bold",
                          )}
                        >
                          <div
                            className={clsx(
                              "w-4 h-4 rounded border flex items-center justify-center transition-colors",
                              filters.propertyTypes.includes(t) ? "bg-primary border-primary text-primary-foreground" : "border-border",
                            )}
                          >
                            {filters.propertyTypes.includes(t) && (
                              <svg className="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                              </svg>
                            )}
                          </div>
                          {t}
                        </button>
                      ))}
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>

              <div className="flex items-end gap-2">
                <div>
                  <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1.5">De</label>
                  <div className="relative">
                    <CalendarRange className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground pointer-events-none" />
                    <input
                      type="date"
                      value={filters.dateFrom}
                      onChange={(e) => onChange({ ...filters, dateFrom: e.target.value })}
                      className={clsx(
                        "pl-8 pr-3 py-2 border rounded-lg text-sm w-[150px] transition-colors",
                        filters.dateFrom ? "border-primary-300 bg-primary-50/50 text-primary-700" : "border-border text-foreground hover:border-border",
                      )}
                    />
                  </div>
                </div>
                <div>
                  <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1.5">Ate</label>
                  <div className="relative">
                    <CalendarRange className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground pointer-events-none" />
                    <input
                      type="date"
                      value={filters.dateTo}
                      onChange={(e) => onChange({ ...filters, dateTo: e.target.value })}
                      className={clsx(
                        "pl-8 pr-3 py-2 border rounded-lg text-sm w-[150px] transition-colors",
                        filters.dateTo ? "border-primary-300 bg-primary-50/50 text-primary-700" : "border-border text-foreground hover:border-border",
                      )}
                    />
                  </div>
                </div>
              </div>

              {activeCount > 0 && (
                <Button
                  variant="ghost"
                  onClick={onReset}
                  className="mb-px text-sm font-bold text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
                >
                  <RotateCcw className="w-3.5 h-3.5" />
                  Limpar
                </Button>
              )}

              {activeCount > 0 && (
                <div className={clsx("w-full flex flex-wrap gap-1.5 pt-2 border-t mt-1", "border-border")}>
                  {filters.agentIds.map((id) => {
                    const agent = agents.find((a) => a.id === id);
                    return (
                      <span
                        key={id}
                        className="inline-flex items-center gap-1 px-2 py-1 bg-primary-50 text-primary-700 text-caption font-bold rounded-lg border border-primary-100"
                      >
                        <User className="w-3 h-3" />
                        {agent?.name.split(" ")[0] || id}
                        <button onClick={() => toggleAgent(id)} className="ml-0.5 hover:text-status-critical transition-colors">
                          <X className="w-3 h-3" />
                        </button>
                      </span>
                    );
                  })}
                  {filters.propertyTypes.map((t) => (
                    <span
                      key={t}
                      className="inline-flex items-center gap-1 px-2 py-1 bg-status-ok-soft text-status-ok text-caption font-bold rounded-lg border border-status-ok/20"
                    >
                      <Building2 className="w-3 h-3" />
                      {t}
                      <button onClick={() => toggleType(t)} className="ml-0.5 hover:text-status-critical transition-colors">
                        <X className="w-3 h-3" />
                      </button>
                    </span>
                  ))}
                  {(filters.dateFrom || filters.dateTo) && (
                    <span className="inline-flex items-center gap-1 px-2 py-1 bg-status-warning-soft text-status-warning text-caption font-bold rounded-lg border border-status-warning/20">
                      <CalendarRange className="w-3 h-3" />
                      {filters.dateFrom || "..."} - {filters.dateTo || "..."}
                      <button
                        onClick={() => onChange({ ...filters, dateFrom: "", dateTo: "" })}
                        className="ml-0.5 hover:text-status-critical transition-colors"
                      >
                        <X className="w-3 h-3" />
                      </button>
                    </span>
                  )}
                </div>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

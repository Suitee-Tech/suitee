import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  CheckCircle,
  ChevronDown,
  Clock3,
  FileCode2,
  PanelLeftClose,
  RefreshCw,
  Search,
  Star,
  X,
} from "lucide-react";

import { CONTRACT_VARIABLE_CATEGORIES, CONTRACT_VARIABLES, type ContractVariable } from "../../contracts/contractVariables";
import { VariableChip } from "./VariableChip";
import { AdvancedDateVariableBuilder } from "./AdvancedDateVariableBuilder";
import type { VariableDetectionState } from "../../hooks/useVariableDetection";
import { cn } from "../../ui/utils";
import { ScrollArea } from "../../ui/scroll-area";
import { Button } from "../../ui/button";

type Tab = "sistema" | "documento";

interface VariableSidebarProps {
  detection: VariableDetectionState;
  onTriggerScan: () => void;
  onInsertVariable: (key: string) => void;
  onMapVariable: (key: string) => void;
  onRemapVariable: (key: string) => void;
  onLocateVariable?: (key: string) => void;
  /** Recolhe o painel no desktop; no mobile a gaveta tem seu próprio gatilho. */
  onCollapse?: () => void;
}

const RECENT_KEY = "contract-vars-recent";
const PINNED_KEY = "contract-vars-pinned";

const readStoredList = (key: string): string[] => {
  if (typeof window === "undefined") return [];
  try {
    const parsed = JSON.parse(window.localStorage.getItem(key) || "[]");
    return Array.isArray(parsed) ? parsed.filter((value): value is string => typeof value === "string") : [];
  } catch {
    return [];
  }
};

const writeStoredList = (key: string, values: string[]) => {
  try {
    window.localStorage.setItem(key, JSON.stringify(values));
  } catch {
    // localStorage pode estar bloqueado; o catálogo continua funcionando.
  }
};

const plural = (count: number, singular: string, pluralForm: string) =>
  `${count} ${count === 1 ? singular : pluralForm}`;

export function VariableSidebar({
  detection,
  onTriggerScan,
  onInsertVariable,
  onMapVariable,
  onRemapVariable,
  onLocateVariable,
  onCollapse,
}: VariableSidebarProps) {
  const [activeTab, setActiveTab] = useState<Tab>("sistema");
  const [openCategories, setOpenCategories] = useState<Set<string>>(() => new Set(["Pessoas"]));
  const [search, setSearch] = useState("");
  const [recentKeys, setRecentKeys] = useState<string[]>(() => readStoredList(RECENT_KEY));
  const [pinnedKeys, setPinnedKeys] = useState<string[]>(() => readStoredList(PINNED_KEY));
  const baseId = useId();
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onGlobalKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const editable = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable;
      if (event.key === "/" && !editable) {
        event.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onGlobalKeyDown);
    return () => window.removeEventListener("keydown", onGlobalKeyDown);
  }, []);

  const searchLower = search.trim().toLowerCase();
  const hasSearch = searchLower.length > 0;

  const filteredSystemVars = useMemo(() => {
    if (!hasSearch) return CONTRACT_VARIABLES;
    return CONTRACT_VARIABLES.filter(
      (v) =>
        v.key.toLowerCase().includes(searchLower) ||
        v.label.toLowerCase().includes(searchLower) ||
        v.description?.toLowerCase().includes(searchLower),
    );
  }, [hasSearch, searchLower]);

  const documentIdentifiedKeys = new Set(detection.identifiedVars.map((v) => v.key));

  const filteredUnidentified = useMemo(() => {
    if (!hasSearch) return detection.unidentifiedKeys;
    return detection.unidentifiedKeys.filter((k) => k.toLowerCase().includes(searchLower));
  }, [hasSearch, searchLower, detection.unidentifiedKeys]);

  const filteredIdentified = useMemo(() => {
    if (!hasSearch) return detection.identifiedVars;
    return detection.identifiedVars.filter(
      (v) =>
        v.key.toLowerCase().includes(searchLower) ||
        v.label.toLowerCase().includes(searchLower) ||
        v.description?.toLowerCase().includes(searchLower),
    );
  }, [hasSearch, searchLower, detection.identifiedVars]);

  const copyVariable = useCallback(
    (key: string) => {
      setRecentKeys((current) => {
        const next = [key, ...current.filter((item) => item !== key)].slice(0, 6);
        writeStoredList(RECENT_KEY, next);
        return next;
      });
      onInsertVariable(key);
    },
    [onInsertVariable],
  );

  const togglePin = useCallback((key: string) => {
    setPinnedKeys((current) => {
      const next = current.includes(key) ? current.filter((item) => item !== key) : [key, ...current];
      writeStoredList(PINNED_KEY, next);
      return next;
    });
  }, []);

  const byKey = useMemo(() => new Map(CONTRACT_VARIABLES.map((variable) => [variable.key, variable])), []);
  const pinnedVariables = useMemo(
    () => pinnedKeys.map((key) => byKey.get(key)).filter((variable): variable is ContractVariable => Boolean(variable)),
    [byKey, pinnedKeys],
  );
  const recentVariables = useMemo(
    () =>
      recentKeys
        .filter((key) => !pinnedKeys.includes(key))
        .map((key) => byKey.get(key))
        .filter((variable): variable is ContractVariable => Boolean(variable))
        .slice(0, 6),
    [byKey, pinnedKeys, recentKeys],
  );

  // A ferramenta de data não pertence ao resultado de uma busca por "propr".
  const dateSearchMatches =
    !hasSearch ||
    (searchLower.length >= 3 &&
      (["data", "vencimento", "prazo", "calcular", "date"].some((term) => term.includes(searchLower)) ||
        "current_date_plus_30_days".includes(searchLower)));

  const identifiedCount = detection.identifiedVars.length;
  const pendingCount = detection.unidentifiedKeys.length;
  const resultCount =
    activeTab === "sistema"
      ? filteredSystemVars.length
      : filteredIdentified.length + filteredUnidentified.length;

  const renderQuickSection = (title: string, icon: React.ReactNode, variables: ContractVariable[]) => {
    if (variables.length === 0) return null;
    return (
      <div className="space-y-2">
        <h5 className="flex items-center gap-2 text-caption font-bold uppercase tracking-wider text-muted-foreground">
          {icon}
          {title}
          <span className="font-medium normal-case tracking-normal">({variables.length})</span>
          <div className="h-px flex-1 bg-muted" />
        </h5>
        <div className="flex flex-wrap gap-2">
          {variables.map((variable) => (
            <VariableChip
              key={`${title}-${variable.key}`}
              variableKey={variable.key}
              label={variable.label}
              description={variable.description}
              status="system"
              category={variable.category}
              variable={variable}
              inDocument={documentIdentifiedKeys.has(variable.key)}
              onInsert={copyVariable}
              searchQuery={hasSearch ? searchLower : undefined}
              pinned={pinnedKeys.includes(variable.key)}
              onTogglePin={togglePin}
            />
          ))}
        </div>
      </div>
    );
  };

  const selectTab = (tab: Tab) => {
    setActiveTab(tab);
    window.requestAnimationFrame(() => document.getElementById(`${baseId}-${tab}-tab`)?.focus());
  };

  return (
    <aside aria-label="Variáveis do modelo" className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="relative z-10 border-b border-border bg-card px-4 pb-3 pt-4 shadow-sm">
        <div className="mb-3 flex items-center justify-between">
          <h4 className="text-sm font-bold text-foreground">Variáveis</h4>
          <div className="flex items-center gap-1">
            {onCollapse && (
              <Button
                variant="ghost"
                size="icon"
                onClick={onCollapse}
                className="hidden size-8 text-muted-foreground md:inline-flex"
                title="Recolher painel de variáveis"
                aria-label="Recolher painel de variáveis"
              >
                <PanelLeftClose className="size-4" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              onClick={onTriggerScan}
              className={cn("size-8 text-muted-foreground", detection.isScanning && "animate-spin text-primary-600")}
              title="Reexaminar variáveis do documento"
              aria-label="Reexaminar variáveis do documento"
            >
              <RefreshCw className="size-4" />
            </Button>
          </div>
        </div>

        <div role="status" aria-live="polite" className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-caption">
          {identifiedCount > 0 ? (
            <span className="inline-flex items-center gap-1.5 font-semibold text-status-ok">
              <CheckCircle className="size-3.5" />
              {plural(identifiedCount, "variável identificada", "variáveis identificadas")}
            </span>
          ) : (
            <span className="text-muted-foreground">Nenhuma variável detectada</span>
          )}
          {pendingCount > 0 && (
            <span className="inline-flex items-center gap-1.5 font-semibold text-status-warning">
              <AlertTriangle className="size-3.5" />
              {plural(pendingCount, "pendente", "pendentes")}
            </span>
          )}
        </div>

        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            ref={searchRef}
            type="text"
            placeholder="Buscar variável..."
            aria-label="Buscar variável"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape" && hasSearch) {
                e.preventDefault();
                setSearch("");
              }
            }}
            className="w-full rounded-lg border border-border bg-muted py-2 pl-9 pr-9 text-sm text-foreground transition-all placeholder:text-muted-foreground hover:border-border focus:border-primary-300 focus:outline-none focus:ring-2 focus:ring-primary-100"
          />
          {hasSearch && (
            <button
              type="button"
              onClick={() => setSearch("")}
              aria-label="Limpar busca"
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-card hover:text-foreground"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
        {hasSearch && (
          <p className="mt-1.5 text-caption text-muted-foreground" aria-live="polite">
            {plural(resultCount, "resultado", "resultados")} nesta aba
          </p>
        )}

        <div role="tablist" aria-label="Origem das variáveis" className="mt-3 flex gap-1 rounded-lg border border-border/50 bg-muted/80 p-1">
          <button
            id={`${baseId}-sistema-tab`}
            role="tab"
            type="button"
            aria-selected={activeTab === "sistema"}
            aria-controls={`${baseId}-panel`}
            tabIndex={activeTab === "sistema" ? 0 : -1}
            onClick={() => setActiveTab("sistema")}
            onKeyDown={(event) => {
              if (event.key === "ArrowRight" || event.key === "ArrowLeft") {
                event.preventDefault();
                selectTab("documento");
              }
            }}
            className={cn(
              "flex-1 whitespace-nowrap rounded-md py-1.5 text-micro font-semibold tracking-normal transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-300",
              activeTab === "sistema"
                ? "bg-card text-primary-700 shadow-sm ring-1 ring-black/5"
                : "text-muted-foreground hover:bg-muted/50 hover:text-foreground",
            )}
          >
            Sistema
          </button>
          <button
            id={`${baseId}-documento-tab`}
            role="tab"
            type="button"
            aria-selected={activeTab === "documento"}
            aria-controls={`${baseId}-panel`}
            tabIndex={activeTab === "documento" ? 0 : -1}
            onClick={() => setActiveTab("documento")}
            onKeyDown={(event) => {
              if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
                event.preventDefault();
                selectTab("sistema");
              }
            }}
            className={cn(
              "inline-flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap rounded-md py-1.5 text-micro font-semibold tracking-normal transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-300",
              activeTab === "documento"
                ? "bg-card text-primary-700 shadow-sm ring-1 ring-black/5"
                : "text-muted-foreground hover:bg-muted/50 hover:text-foreground",
            )}
          >
            No documento
            {detection.allFoundKeys.length > 0 && (
              <span
                className={cn(
                  "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full text-micro font-bold transition-colors",
                  activeTab === "documento" ? "bg-primary-100 text-primary-700" : "bg-muted text-muted-foreground",
                )}
              >
                {detection.allFoundKeys.length}
              </span>
            )}
          </button>
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div
          id={`${baseId}-panel`}
          role="tabpanel"
          aria-labelledby={`${baseId}-${activeTab}-tab`}
          className="space-y-5 px-5 py-4"
        >
          {activeTab === "sistema" && (
            <>
              <details className="rounded-lg border border-border bg-muted/40">
                <summary className="cursor-pointer px-3 py-2 text-xs font-semibold text-foreground">Como usar as variáveis</summary>
                <p className="px-3 pb-3 text-xs leading-relaxed text-muted-foreground">
                  Arraste uma variável para o documento ou clique para copiar o código completo. Use a busca para encontrar pelo nome em inglês ou pela descrição.
                </p>
              </details>

              {dateSearchMatches && (
                <details className="rounded-lg border border-primary-200 bg-primary-50/30">
                  <summary className="cursor-pointer px-3 py-2.5 text-sm font-semibold text-foreground">
                    <span className="flex items-center justify-between gap-2">
                      <span>Calcular data ou vencimento</span>
                      <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
                    </span>
                    <code className="mt-1 block break-all text-caption font-normal text-primary-700">{"{{current_date_plus_30_days}}"}</code>
                  </summary>
                  <div className="px-2 pb-2">
                    <AdvancedDateVariableBuilder onSelect={copyVariable} />
                  </div>
                </details>
              )}

              {!hasSearch && renderQuickSection("Fixadas", <Star className="size-3.5 fill-current" />, pinnedVariables)}
              {!hasSearch && renderQuickSection("Recentes", <Clock3 className="size-3.5" />, recentVariables)}

              <div className="space-y-4">
                {filteredSystemVars.length === 0 && (
                  <p className="text-xs text-muted-foreground">Nenhuma variável encontrada. Busque pelo código em inglês ou pela descrição.</p>
                )}
                {CONTRACT_VARIABLE_CATEGORIES.map((cat) => {
                  const vars = filteredSystemVars.filter((v) => v.category === cat);
                  if (vars.length === 0) return null;
                  return (
                    <div key={cat} className="space-y-2">
                      <button
                        type="button"
                        aria-expanded={hasSearch || openCategories.has(cat)}
                        onClick={() =>
                          setOpenCategories((current) => {
                            const next = new Set(current);
                            if (next.has(cat)) next.delete(cat);
                            else next.add(cat);
                            return next;
                          })
                        }
                        className="sticky top-0 z-10 flex w-full items-center gap-2 bg-card/95 py-1 text-left text-caption font-bold uppercase tracking-wider text-muted-foreground backdrop-blur-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-300"
                      >
                        {cat}
                        <span className="font-medium normal-case tracking-normal">({vars.length})</span>
                        <div className="h-px flex-1 bg-muted" />
                        <ChevronDown
                          className={cn(
                            "size-3.5 transition-transform",
                            (hasSearch || openCategories.has(cat)) && "rotate-180",
                          )}
                        />
                      </button>
                      {(hasSearch || openCategories.has(cat)) && (
                        <div className="flex flex-wrap gap-2">
                          {vars.map((v) => (
                            <VariableChip
                              key={v.key}
                              variableKey={v.key}
                              label={v.label}
                              description={v.description}
                              status="system"
                              category={v.category}
                              variable={v}
                              inDocument={documentIdentifiedKeys.has(v.key)}
                              onInsert={copyVariable}
                              searchQuery={hasSearch ? searchLower : undefined}
                              pinned={pinnedKeys.includes(v.key)}
                              onTogglePin={togglePin}
                            />
                          ))}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </>
          )}

          {activeTab === "documento" && (
            <div className="space-y-5">
              {filteredIdentified.length > 0 && (
                <div className="space-y-2">
                  <h5 className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wider text-status-ok">
                    <CheckCircle className="size-3.5" />
                    Identificadas ({filteredIdentified.length})
                    <div className="ml-1 h-px flex-1 bg-status-ok-soft" />
                  </h5>
                  <div className="flex flex-wrap gap-2">
                    {filteredIdentified.map((v) => (
                      <VariableChip
                        key={v.key}
                        variableKey={v.key}
                        label={v.label}
                        description={v.description}
                        status="identified"
                        category={v.category}
                        variable={v}
                        onRemap={onRemapVariable}
                        onLocate={onLocateVariable}
                        searchQuery={hasSearch ? searchLower : undefined}
                      />
                    ))}
                  </div>
                </div>
              )}

              {filteredUnidentified.length > 0 && (
                <div className="space-y-2">
                  <h5 className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wider text-status-warning">
                    <AlertTriangle className="size-3.5" />
                    Não identificadas ({filteredUnidentified.length})
                    <div className="ml-1 h-px flex-1 bg-status-warning-soft" />
                  </h5>
                  <div className="flex flex-wrap gap-2">
                    {filteredUnidentified.map((key) => (
                      <VariableChip
                        key={key}
                        variableKey={key}
                        label={key}
                        status="unidentified"
                        onMap={onMapVariable}
                        searchQuery={hasSearch ? searchLower : undefined}
                      />
                    ))}
                  </div>
                </div>
              )}

              {hasSearch && resultCount === 0 && detection.allFoundKeys.length > 0 && (
                <p className="text-xs text-muted-foreground">Nenhuma variável deste documento corresponde à busca.</p>
              )}

              {hasSearch && activeTab === "documento" && resultCount === 0 && filteredSystemVars.length > 0 && (
                <button
                  type="button"
                  onClick={() => setActiveTab("sistema")}
                  className="rounded-md text-left text-xs font-semibold text-primary-700 underline-offset-2 hover:underline"
                >
                  {plural(filteredSystemVars.length, "resultado", "resultados")} no catálogo do Sistema
                </button>
              )}

              {detection.allFoundKeys.length === 0 && (
                <div className="flex flex-col items-center justify-center py-10 text-center">
                  <div className="mb-4 flex size-12 items-center justify-center rounded-full border border-border bg-muted">
                    <FileCode2 className="size-6 text-muted-foreground/60" />
                  </div>
                  <p className="text-sm font-medium text-muted-foreground">Nenhuma variável detectada</p>
                  <p className="mt-1.5 max-w-[200px] text-xs leading-relaxed text-muted-foreground">
                    Use o formato{" "}
                    <code className="rounded bg-muted px-1 py-0.5 font-mono text-muted-foreground">{"{{variable_name}}"}</code>{" "}
                    no texto do documento.
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      </ScrollArea>
    </aside>
  );
}

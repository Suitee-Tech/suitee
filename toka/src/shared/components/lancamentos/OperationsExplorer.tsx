import { useMemo } from "react";
import { AlertTriangle, Columns3, LayoutList } from "lucide-react";
import { clsx } from "clsx";

import { formatMoneyBR } from "../../utils/format";
import type { Lancamento, LancamentoStatus } from "../../lib/lancamentos/types";
import type { EconomicOperation } from "../../lib/lancamentos/presentation";
import {
  collectMonthOptions,
  countQuickFilters,
  currentMonthKey,
  groupOperations,
  monthLabel,
  OPERATION_GROUPS,
  OPERATION_QUICK_FILTERS,
  OPERATION_SORTS,
  summarizeOperations,
  type OperationGroupId,
  type OperationQuickFilterId,
  type OperationSortId,
} from "../../lib/lancamentos/operationsView";
import type { LancamentosFilters } from "../../queries/lancamentos";
import { EconomicOperationsTable } from "./EconomicOperationsTable";
import { OperationSpreadsheet } from "./OperationSpreadsheet";

export type OperationViewMode = "cards" | "sheet";

const OPEN_STATUSES: LancamentoStatus[] = ["pendente", "aprovado", "parcial", "vencido"];

interface OperationsExplorerProps {
  /** Operações já filtradas — é o que a lista mostra. */
  operations: EconomicOperation[];
  /** Operações sem os atalhos de estado, usadas para contar e listar meses. */
  facetSource: EconomicOperation[];
  filters: LancamentosFilters;
  onFiltersChange: (next: LancamentosFilters) => void;
  view: OperationViewMode;
  onViewChange: (view: OperationViewMode) => void;
  groupBy: OperationGroupId;
  onGroupChange: (group: OperationGroupId) => void;
  sort: OperationSortId;
  onSortChange: (sort: OperationSortId) => void;
  onSelect: (lancamento: Lancamento) => void;
  onOpenRentalPlan?: (contractId: string) => void;
  isLoading?: boolean;
}

/**
 * Aba de Operações: leitura, recorte e agrupamento da carteira.
 *
 * A barra de controles fica sempre visível (não escondida atrás de "Filtros")
 * porque é ela que impede a lista grande de virar uma pilha: primeiro o mês
 * que interessa, depois os atalhos de urgência, depois o agrupamento.
 */
export function OperationsExplorer({
  operations,
  facetSource,
  filters,
  onFiltersChange,
  view,
  onViewChange,
  groupBy,
  onGroupChange,
  sort,
  onSortChange,
  onSelect,
  onOpenRentalPlan,
  isLoading,
}: OperationsExplorerProps) {
  const counts = useMemo(() => countQuickFilters(facetSource), [facetSource]);
  const monthOptions = useMemo(() => collectMonthOptions(facetSource), [facetSource]);
  const totals = useMemo(() => summarizeOperations(operations), [operations]);
  const sections = useMemo(() => groupOperations(operations, groupBy, sort), [operations, groupBy, sort]);

  const currentMonth = currentMonthKey();
  const patch = (change: Partial<LancamentosFilters>) => onFiltersChange({ ...filters, ...change });

  const activeQuickFilters = useMemo(() => {
    const active = new Set<OperationQuickFilterId>();
    if (filters.overdueOnly) active.add("overdue");
    if (filters.flow === "receber") active.add("receivable");
    if (filters.flow === "pagar") active.add("payable");
    if (filters.divergentOnly) active.add("divergent");
    return active;
  }, [filters.overdueOnly, filters.flow, filters.divergentOnly]);

  const toggleQuickFilter = (id: OperationQuickFilterId) => {
    if (id === "overdue") {
      patch({ overdueOnly: filters.overdueOnly ? undefined : true });
      return;
    }
    if (id === "divergent") {
      patch({ divergentOnly: filters.divergentOnly ? undefined : true });
      return;
    }
    const flow = id === "receivable" ? "receber" : "pagar";
    if (filters.flow === flow) {
      // Desligar o atalho também solta os status que ele impôs.
      patch({ flow: undefined, statuses: [] });
      return;
    }
    patch({ flow, statuses: [...OPEN_STATUSES] });
  };

  const competenceOptions = monthOptions.competence.filter((month) => month !== currentMonth);
  const dueOptions = monthOptions.due.filter((month) => month !== currentMonth);

  return (
    <div className="space-y-3">
      <section aria-label="Recorte e agrupamento das operações" className="rounded-xl border border-border bg-card shadow-sm">
        {/* Quatro dimensões em grade: 2×2 no mobile, linha única no desktop,
            com a alternância de visão ancorada à direita. */}
        <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-4 lg:grid-cols-[repeat(4,minmax(9rem,1fr))] lg:items-end xl:grid-cols-[repeat(4,minmax(9rem,1fr))_auto]">
          <Field label="Competência" htmlFor="ops-competence">
            <select
              id="ops-competence"
              value={filters.competence ?? ""}
              onChange={(event) => patch({ competence: event.target.value || undefined })}
              className="h-9 w-full rounded-lg border border-border bg-background px-2.5 text-label text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <option value="">Todas</option>
              <option value={currentMonth}>{monthLabel(currentMonth)} · atual</option>
              {competenceOptions.map((month) => (
                <option key={month} value={month}>
                  {monthLabel(month)}
                </option>
              ))}
              <option value="none">Sem competência</option>
            </select>
          </Field>

          <Field label="Vencimento" htmlFor="ops-due">
            <select
              id="ops-due"
              value={filters.dueMonth ?? ""}
              onChange={(event) => patch({ dueMonth: event.target.value || undefined })}
              className="h-9 w-full rounded-lg border border-border bg-background px-2.5 text-label text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <option value="">Todos</option>
              <option value={currentMonth}>{monthLabel(currentMonth)} · vence</option>
              {dueOptions.map((month) => (
                <option key={month} value={month}>
                  {monthLabel(month)}
                </option>
              ))}
              <option value="none">Sem vencimento</option>
            </select>
          </Field>

          <Field label="Agrupar por" htmlFor="ops-group">
            <select
              id="ops-group"
              value={groupBy}
              onChange={(event) => onGroupChange(event.target.value as OperationGroupId)}
              className="h-9 w-full rounded-lg border border-border bg-background px-2.5 text-label text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              {OPERATION_GROUPS.map((group) => (
                <option key={group.id} value={group.id}>
                  {group.label}
                </option>
              ))}
            </select>
          </Field>

          <Field label="Ordenar" htmlFor="ops-sort">
            <select
              id="ops-sort"
              value={sort}
              onChange={(event) => onSortChange(event.target.value as OperationSortId)}
              title={OPERATION_SORTS.find((option) => option.id === sort)?.hint}
              className="h-9 w-full rounded-lg border border-border bg-background px-2.5 text-label text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              {OPERATION_SORTS.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.label}
                </option>
              ))}
            </select>
          </Field>

          <div
            role="group"
            aria-label="Visualização"
            className="col-span-2 flex items-center gap-1 rounded-lg border border-border bg-muted p-0.5 sm:col-span-4 sm:w-fit sm:justify-self-end lg:col-span-4 xl:col-span-1"
          >
            <ViewButton active={view === "cards"} onClick={() => onViewChange("cards")} icon={<LayoutList className="size-3.5" />} label="Cartões" />
            <ViewButton active={view === "sheet"} onClick={() => onViewChange("sheet")} icon={<Columns3 className="size-3.5" />} label="Planilha" />
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2.5">
          <span className="text-micro font-bold uppercase tracking-wide text-muted-foreground">Atalhos</span>
          {OPERATION_QUICK_FILTERS.map((quick) => {
            const active = activeQuickFilters.has(quick.id);
            const count = counts[quick.id];
            return (
              <button
                key={quick.id}
                type="button"
                onClick={() => toggleQuickFilter(quick.id)}
                aria-pressed={active}
                title={quick.hint}
                className={clsx(
                  "inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-1 text-caption font-bold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                  active
                    ? "border-primary bg-primary text-primary-foreground"
                    : "border-border bg-background text-foreground hover:border-primary",
                  !active && count === 0 && "opacity-60",
                )}
              >
                {quick.id === "overdue" && <AlertTriangle className="size-3.5" aria-hidden="true" />}
                {quick.label}
                <span
                  className={clsx(
                    "rounded-full px-1.5 text-micro tabular-nums",
                    active ? "bg-primary-foreground/20 text-primary-foreground" : "bg-muted text-muted-foreground",
                  )}
                >
                  {count}
                </span>
              </button>
            );
          })}

          <span className="flex w-full flex-wrap items-center gap-x-2 gap-y-0.5 text-caption text-muted-foreground sm:w-auto lg:ml-auto">
            <span className="font-bold text-foreground">
              {totals.count} {totals.count === 1 ? "operação" : "operações"}
            </span>
            <span aria-hidden="true">·</span>
            <span>{totals.lines} linhas</span>
            {totals.receivable > 0 && (
              <>
                <span aria-hidden="true">·</span>
                <span className="font-mono font-bold text-status-info tabular-nums">
                  a receber {formatMoneyBR(totals.receivable, { withSymbol: true })}
                </span>
              </>
            )}
            {totals.payable > 0 && (
              <>
                <span aria-hidden="true">·</span>
                <span className="font-mono font-bold text-status-warning tabular-nums">
                  a pagar {formatMoneyBR(totals.payable, { withSymbol: true })}
                </span>
              </>
            )}
            {totals.overdueCount > 0 && (
              <>
                <span aria-hidden="true">·</span>
                <span className="font-bold text-status-critical">{totals.overdueCount} em atraso</span>
              </>
            )}
          </span>
        </div>
      </section>

      {operations.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border bg-card p-12 text-center">
          <p className="text-sm font-bold text-foreground">Nenhuma operação neste recorte</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Ajuste a competência, o vencimento ou limpe os atalhos para ver mais operações.
          </p>
          {(filters.competence || filters.dueMonth || filters.overdueOnly || filters.flow || filters.divergentOnly) && (
            <button
              type="button"
              onClick={() =>
                patch({ competence: undefined, dueMonth: undefined, overdueOnly: undefined, flow: undefined, divergentOnly: undefined, statuses: [] })
              }
              className="mt-3 text-xs font-bold text-primary-600 hover:underline"
            >
              Limpar recorte das operações
            </button>
          )}
        </div>
      ) : view === "sheet" ? (
        <OperationSpreadsheet sections={sections} onSelect={onSelect} isLoading={isLoading} />
      ) : (
        <div className="space-y-5">
          {sections.map((section) => (
            <section key={section.key} className="space-y-2">
              {!(sections.length === 1 && section.key === "all") && (
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 px-1">
                  <h2 className="text-title font-bold text-foreground">{section.label}</h2>
                  <span className="text-caption text-muted-foreground">
                    {section.count} {section.count === 1 ? "operação" : "operações"}
                  </span>
                  {section.overdueCount > 0 && (
                    <span className="inline-flex items-center gap-1 rounded-full border border-status-critical/30 bg-status-critical-soft px-2 py-0.5 text-micro font-bold text-status-critical">
                      <AlertTriangle className="size-3" aria-hidden="true" /> {section.overdueCount} em atraso
                    </span>
                  )}
                  <span className="ml-auto flex items-center gap-3 font-mono text-caption font-bold tabular-nums">
                    {section.receivable > 0 && (
                      <span className="text-status-info">a receber {formatMoneyBR(section.receivable, { withSymbol: true })}</span>
                    )}
                    {section.payable > 0 && (
                      <span className="text-status-warning">a pagar {formatMoneyBR(section.payable, { withSymbol: true })}</span>
                    )}
                  </span>
                </div>
              )}
              <EconomicOperationsTable
                operations={section.operations}
                isLoading={isLoading}
                onSelect={onSelect}
                onOpenRentalPlan={onOpenRentalPlan}
                showSummary={false}
              />
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

function Field({ label, htmlFor, children }: { label: string; htmlFor: string; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <label htmlFor={htmlFor} className="truncate text-micro font-bold uppercase tracking-wide text-muted-foreground">
        {label}
      </label>
      {children}
    </div>
  );
}

function ViewButton({
  active,
  onClick,
  icon,
  label,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={clsx(
        "inline-flex flex-1 items-center justify-center gap-1.5 rounded-md px-2.5 py-1.5 text-caption font-bold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:flex-none",
        active ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground hover:text-foreground",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

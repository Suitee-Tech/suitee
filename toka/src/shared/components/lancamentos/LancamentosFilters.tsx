import { useId, useState } from "react";
import { AlertOctagon, ChevronDown, Search, SlidersHorizontal, X } from "lucide-react";
import { clsx } from "clsx";

import { Button } from "../../ui/button";
import { formatCompetencia } from "../../utils/format";
import { statusLabel } from "../../lib/lancamentos/status";
import type { LancamentoStatus } from "../../lib/lancamentos/types";
import type { LancamentosFilters as FiltersState } from "../../queries/lancamentos";

interface LancamentosFiltersProps {
  value: FiltersState;
  onChange: (next: FiltersState) => void;
  availableStatuses?: LancamentoStatus[];
}

const DEFAULT_STATUSES: LancamentoStatus[] = ["pendente", "aprovado", "pago", "processado", "vencido", "parcial", "cancelado"];

export function LancamentosFilters({ value, onChange, availableStatuses = DEFAULT_STATUSES }: LancamentosFiltersProps) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const searchId = useId();
  const fromId = useId();
  const toId = useId();
  const statusId = useId();
  const dateBasisId = useId();

  const advancedCount =
    (value.statuses?.length ?? 0) +
    Number(Boolean(value.from)) +
    Number(Boolean(value.to)) +
    Number(Boolean(value.dateBasis && value.dateBasis !== "operational")) +
    Number(Boolean(value.divergentOnly)) +
    Number(Boolean(value.flow)) +
    Number(Boolean(value.competence)) +
    Number(Boolean(value.dueMonth)) +
    Number(Boolean(value.overdueOnly));
  const hasAny = Boolean(value.search) || advancedCount > 0;

  const update = (patch: Partial<FiltersState>) => {
    onChange({ ...value, ...patch });
  };

  const toggleStatus = (status: LancamentoStatus) => {
    const current = value.statuses ?? [];
    const next = current.includes(status) ? current.filter((item) => item !== status) : [...current, status];
    update({ statuses: next });
  };

  const clearFilters = () => {
    onChange({
      search: "",
      from: undefined,
      to: undefined,
      dateBasis: "operational",
      statuses: [],
      flow: undefined,
      divergentOnly: undefined,
      competence: undefined,
      dueMonth: undefined,
      overdueOnly: undefined,
      kinds: value.kinds,
      counterparty: value.counterparty,
    });
  };

  return (
    <div className="sticky top-0 z-10 rounded-xl border border-border bg-card p-3 shadow-sm">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(240px,1fr)_auto] md:items-end">
        <div>
          <label htmlFor={searchId} className="sr-only">Buscar lançamentos</label>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              id={searchId}
              type="search"
              value={value.search ?? ""}
              onChange={(event) => update({ search: event.target.value })}
              placeholder="Buscar por ID, contraparte ou origem…"
              className="h-10 w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setShowAdvanced((current) => !current)}
            aria-expanded={showAdvanced}
          >
            <SlidersHorizontal className="h-4 w-4" />
            Filtros
            {advancedCount > 0 && (
              <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1.5 text-[11px] font-bold text-primary-foreground">
                {advancedCount}
              </span>
            )}
            <ChevronDown className={clsx("h-4 w-4 transition-transform", showAdvanced && "rotate-180")} />
          </Button>
          {hasAny && (
            <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
              <X className="h-4 w-4" /> Limpar
            </Button>
          )}
        </div>
      </div>

      {!showAdvanced && advancedCount > 0 && (
        <p className="mt-2 flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
          {[
            value.flow === "receber" ? "Fluxo: a receber" : value.flow === "pagar" ? "Fluxo: a pagar" : "",
            value.overdueOnly ? "Somente em atraso" : "",
            value.competence ? `Competência: ${value.competence === "none" ? "sem competência" : formatCompetencia(value.competence)}` : "",
            value.dueMonth ? `Vencimento: ${value.dueMonth === "none" ? "sem vencimento" : formatCompetencia(value.dueMonth)}` : "",
            value.statuses?.length ? `${value.statuses.length} status` : "",
            value.from || value.to
              ? `Período por ${value.dateBasis === "settled" ? "liquidação" : value.dateBasis === "created" ? "criação" : "vencimento/criação"}`
              : "",
            value.divergentOnly ? "Somente divergências" : "",
          ]
            .filter(Boolean)
            .map((chunk, index) => (
              <span key={chunk} className="inline-flex items-center gap-1">
                {index > 0 && <span aria-hidden="true">·</span>}
                {chunk}
              </span>
            ))}
        </p>
      )}

      {showAdvanced && (
        <div className="mt-3 space-y-3 border-t border-border pt-3">
          <div className="grid grid-cols-1 gap-3 lg:grid-cols-[auto_auto_minmax(180px,240px)_1fr] lg:items-end">
            <div>
              <label htmlFor={fromId} className="text-xs font-bold uppercase tracking-wide text-muted-foreground">De</label>
              <input
                id={fromId}
                type="date"
                value={value.from ?? ""}
                onChange={(event) => update({ from: event.target.value || undefined })}
                className="mt-1 block h-9 rounded-lg border border-border bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              />
            </div>
            <div>
              <label htmlFor={toId} className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Até</label>
              <input
                id={toId}
                type="date"
                value={value.to ?? ""}
                onChange={(event) => update({ to: event.target.value || undefined })}
                className="mt-1 block h-9 rounded-lg border border-border bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              />
            </div>
            <div>
              <label htmlFor={dateBasisId} className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Data considerada</label>
              <select
                id={dateBasisId}
                value={value.dateBasis ?? "operational"}
                onChange={(event) => update({ dateBasis: event.target.value as FiltersState["dateBasis"] })}
                className="mt-1 block h-9 w-full rounded-lg border border-border bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <option value="operational">Vencimento (ou criação)</option>
                <option value="settled">Liquidação / pagamento</option>
                <option value="created">Criação do registro</option>
              </select>
            </div>
            <div>
              <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Fluxo</span>
              <div className="mt-1 flex items-center gap-2">
                {([{"id":"receber","label":"A receber"},{"id":"pagar","label":"A pagar"}] as const).map((option) => (
                  <button
                    key={option.id}
                    type="button"
                    onClick={() => update({ flow: value.flow === option.id ? undefined : option.id })}
                    aria-pressed={value.flow === option.id}
                    className={clsx(
                      "h-9 rounded-lg border px-3 text-xs font-semibold",
                      value.flow === option.id
                        ? "border-primary bg-primary text-primary-foreground"
                        : "border-border bg-background text-foreground hover:border-primary",
                    )}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2" aria-label="Filtros de status" id={statusId}>
            <span className="mr-1 text-xs font-bold uppercase tracking-wide text-muted-foreground">Status</span>
            {availableStatuses.map((status) => {
              const active = value.statuses?.includes(status) ?? false;
              return (
                <button
                  key={status}
                  type="button"
                  onClick={() => toggleStatus(status)}
                  aria-pressed={active}
                  className={clsx(
                    "inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-semibold",
                    active
                      ? "border-primary bg-primary text-primary-foreground"
                      : "border-border bg-background text-foreground hover:border-primary",
                  )}
                >
                  {statusLabel(status)}
                </button>
              );
            })}
            <span className="mx-1 h-4 w-px bg-border" aria-hidden="true" />
            <button
              type="button"
              onClick={() => update({ divergentOnly: !value.divergentOnly })}
              aria-pressed={Boolean(value.divergentOnly)}
              className={clsx(
                "inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-semibold",
                value.divergentOnly
                  ? "border-status-warning bg-status-warning text-status-warning-foreground"
                  : "border-status-warning/40 bg-background text-status-warning hover:border-status-warning",
              )}
              title="Mostrar apenas lançamentos que precisam de conciliação"
            >
              <AlertOctagon className="h-3.5 w-3.5" /> Em divergência
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

import { Fragment, useMemo, useState } from "react";
import { AlertTriangle, ChevronDown, ChevronRight } from "lucide-react";
import { clsx } from "clsx";

import { formatISODateBR, formatMoneyBR } from "../../utils/format";
import { lancamentoSituationLabel, lancamentoSituationVariant } from "../../lib/lancamentos/chargeDue";
import { getOwnerTransferPresentation } from "../../lib/finance/ownerTransferFlow";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { Lancamento } from "../../lib/lancamentos/types";
import type { EconomicOperation } from "../../lib/lancamentos/presentation";
import {
  monthLabel,
  summarizeOperation,
  type OperationSection,
  type OperationSummary,
} from "../../lib/lancamentos/operationsView";
import { useIsDesktop, useMediaQuery } from "../../hooks/useMediaQuery";

interface OperationSpreadsheetProps {
  sections: OperationSection[];
  onSelect: (lancamento: Lancamento) => void;
  isLoading?: boolean;
}

const KIND_LABEL: Record<Lancamento["kind"], string> = {
  charge: "Cobrança",
  commission: "Comissão",
  transfer: "Repasse",
  royalty: "Royalty",
  transaction: "Caixa",
};

const formatDate = (iso?: string) => (iso ? formatISODateBR(iso) || "—" : "—");
const formatMonth = (month?: string) => (!month || month === "sem-data" ? "—" : monthLabel(month));

const isSaida = (l: Lancamento): boolean => {
  if (l.kind === "commission" || l.kind === "transfer" || l.kind === "royalty") return true;
  if (l.kind === "transaction") return l.kind_data.direction === "saida";
  return false;
};

/**
 * Visão condensada das operações, em formato de planilha.
 *
 * Uma linha por operação, subtotal por grupo e as linhas auditáveis
 * disponíveis ao expandir. Segue o padrão de tabela do projeto
 * (`ui/responsive-table`): tabela densa no desktop e cartões abaixo disso —
 * são 9 colunas, e espremê-las em um tablet só produziria truncamento.
 */
export function OperationSpreadsheet({ sections, onSelect, isLoading }: OperationSpreadsheetProps) {
  const isDesktop = useIsDesktop();
  // Colunas opcionais são decididas em JS, não por `hidden`: esconder células
  // deixa buracos nas linhas com `colSpan` e o navegador cria colunas
  // fantasma que roubam a largura de "Operação".
  const showOrigin = useMediaQuery("(min-width: 1280px)");
  const showLines = useMediaQuery("(min-width: 1536px)");
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  const summaries = useMemo(() => {
    const map = new Map<string, OperationSummary>();
    for (const section of sections) {
      for (const operation of section.operations) map.set(operation.key, summarizeOperation(operation));
    }
    return map;
  }, [sections]);

  const totals = useMemo(() => {
    let receivable = 0;
    let payable = 0;
    let overdue = 0;
    let lines = 0;
    for (const section of sections) {
      receivable += section.receivable;
      payable += section.payable;
      overdue += section.overdueCount;
      for (const operation of section.operations) lines += operation.items.length;
    }
    return { receivable, payable, overdue, lines };
  }, [sections]);

  const toggle = (key: string) =>
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  if (isLoading && sections.length === 0) {
    return <div className="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">Carregando operações…</div>;
  }

  const showGroupRows = !(sections.length === 1 && sections[0].key === "all");

  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      {isDesktop ? (
        <div className="max-h-[70vh] overflow-auto">
          <table className="w-full min-w-[42rem] table-fixed border-collapse text-xs">
            <caption className="sr-only">Operações econômicas em visão condensada</caption>
            <thead className="sticky top-0 z-10 bg-muted text-muted-foreground">
              <tr className="border-b border-border text-left">
                <th scope="col" className="w-7 px-1.5 py-2">
                  <span className="sr-only">Expandir</span>
                </th>
                <th scope="col" className="w-[5.5rem] px-2 py-2 text-micro font-bold uppercase tracking-wide">Competência</th>
                <th scope="col" className="w-[5.5rem] px-2 py-2 text-micro font-bold uppercase tracking-wide">Vencimento</th>
                <th scope="col" className="px-2 py-2 text-micro font-bold uppercase tracking-wide">Operação</th>
                {showOrigin && (
                  <th scope="col" className="w-[20%] px-2 py-2 text-micro font-bold uppercase tracking-wide">Origem</th>
                )}
                {showLines && (
                  <th scope="col" className="w-12 px-2 py-2 text-right text-micro font-bold uppercase tracking-wide">Linhas</th>
                )}
                <th scope="col" className="w-[6.5rem] px-2 py-2 text-right text-micro font-bold uppercase tracking-wide">A receber</th>
                <th scope="col" className="w-[6.5rem] px-2 py-2 text-right text-micro font-bold uppercase tracking-wide">A pagar</th>
                <th scope="col" className="w-[6.5rem] px-2 py-2 text-micro font-bold uppercase tracking-wide">Situação</th>
              </tr>
            </thead>
            <tbody>
              {sections.map((section) => (
                <Fragment key={section.key}>
                  {showGroupRows && (
                    <tr className="border-b border-border bg-muted/60">
                      <td aria-hidden="true" />
                      <td colSpan={3 + (showOrigin ? 1 : 0)} className="px-2 py-1.5">
                        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-micro font-bold uppercase tracking-wide text-foreground">
                          <span>{section.label}</span>
                          <span className="font-medium normal-case tracking-normal text-muted-foreground">
                            {section.count} {section.count === 1 ? "operação" : "operações"}
                          </span>
                          {section.overdueCount > 0 && <OverdueChip count={section.overdueCount} />}
                        </div>
                      </td>
                      {/* O `colSpan` do rótulo já cobre a coluna Origem. */}
                      {showLines && <td />}
                      <td className="truncate px-2 py-1.5 text-right font-mono text-status-info tabular-nums">
                        {section.receivable > 0 ? formatMoneyBR(section.receivable, { withSymbol: true }) : ""}
                      </td>
                      <td className="truncate px-2 py-1.5 text-right font-mono text-status-warning tabular-nums">
                        {section.payable > 0 ? formatMoneyBR(section.payable, { withSymbol: true }) : ""}
                      </td>
                      <td />
                    </tr>
                  )}

                  {section.operations.map((operation, index) => {
                    const summary = summaries.get(operation.key) ?? summarizeOperation(operation);
                    const isOpen = expanded.has(operation.key);
                    const originHint = operation.subtitle || operation.primary.origin.label || "—";
                    return (
                      <Fragment key={operation.key}>
                        <tr
                          onClick={() => (operation.items.length === 1 ? onSelect(operation.primary) : toggle(operation.key))}
                          className={clsx(
                            "cursor-pointer border-b border-border transition-colors last:border-0 hover:bg-muted/40",
                            index % 2 === 1 && "bg-muted/20",
                          )}
                        >
                          <td className="px-1.5 py-1.5 align-middle">
                            {operation.items.length > 1 ? (
                              <button
                                type="button"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  toggle(operation.key);
                                }}
                                aria-expanded={isOpen}
                                aria-label={isOpen ? `Recolher ${operation.title}` : `Expandir ${operation.title}`}
                                className="rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                              >
                                {isOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                              </button>
                            ) : (
                              <ChevronRight className="size-3.5 text-muted-foreground/50" aria-hidden="true" />
                            )}
                          </td>
                          <td className="truncate px-2 py-1.5 tabular-nums text-muted-foreground">{formatMonth(summary.competence)}</td>
                          <td
                            className={clsx(
                              "truncate px-2 py-1.5 tabular-nums",
                              summary.overdue ? "font-bold text-status-critical" : "text-muted-foreground",
                            )}
                          >
                            {formatDate(summary.dueAt)}
                          </td>
                          <td className="truncate px-2 py-1.5 font-bold text-foreground" title={operation.title}>
                            {operation.title}
                          </td>
                          {showOrigin && (
                            <td className="truncate px-2 py-1.5 text-muted-foreground" title={originHint}>
                              {operation.primary.presentation?.property_title || originHint}
                            </td>
                          )}
                          {showLines && (
                            <td className="px-2 py-1.5 text-right tabular-nums text-muted-foreground">{operation.items.length}</td>
                          )}
                          <td className="truncate px-2 py-1.5 text-right font-mono font-bold tabular-nums text-status-info">
                            {summary.receivable > 0 ? (
                              formatMoneyBR(summary.receivable, { withSymbol: true })
                            ) : (
                              <span className="text-muted-foreground/60">—</span>
                            )}
                          </td>
                          <td className="truncate px-2 py-1.5 text-right font-mono font-bold tabular-nums text-status-warning">
                            {summary.payable > 0 ? (
                              formatMoneyBR(summary.payable, { withSymbol: true })
                            ) : (
                              <span className="text-muted-foreground/60">—</span>
                            )}
                          </td>
                          <td className="px-2 py-1.5">
                            <OperationSituation summary={summary} />
                          </td>
                        </tr>

                        {isOpen &&
                          operation.items.map((item) => (
                            <tr
                              key={`${operation.key}:${item.kind}:${item.id}`}
                              onClick={() => onSelect(item)}
                              className="cursor-pointer border-b border-border/60 bg-background transition-colors last:border-0 hover:bg-muted/40"
                            >
                              <td />
                              <td className="truncate px-2 py-1 pl-6 text-caption tabular-nums text-muted-foreground">
                                {item.presentation?.reference_month || formatDate(item.created_at)}
                              </td>
                              <td className="truncate px-2 py-1 text-caption tabular-nums text-muted-foreground">{formatDate(item.due_at)}</td>
                              <td className="truncate px-2 py-1">
                                <span className="flex items-center gap-1.5">
                                  <KindTag kind={item.kind} />
                                  <span className="truncate text-caption text-foreground" title={item.counterparty.label}>
                                    {item.counterparty.label}
                                  </span>
                                </span>
                              </td>
                              {showOrigin && (
                                <td className="truncate px-2 py-1 text-caption text-muted-foreground">{item.origin.label ?? "—"}</td>
                              )}
                              {showLines && <td />}
                              <td className="truncate px-2 py-1 text-right font-mono text-caption tabular-nums text-muted-foreground">
                                {isSaida(item) ? "—" : formatMoneyBR(item.amount, { withSymbol: true })}
                              </td>
                              <td className="truncate px-2 py-1 text-right font-mono text-caption tabular-nums text-muted-foreground">
                                {isSaida(item) ? formatMoneyBR(item.amount, { withSymbol: true }) : "—"}
                              </td>
                              <td className="px-2 py-1">
                                <StatusChip lancamento={item} />
                              </td>
                            </tr>
                          ))}
                      </Fragment>
                    );
                  })}
                </Fragment>
              ))}
            </tbody>
            <tfoot className="sticky bottom-0 bg-muted">
              <tr className="border-t border-border">
                {/* O rodapé fecha exatamente as mesmas colunas do cabeçalho. */}
                <td colSpan={4 + (showOrigin ? 1 : 0)} className="px-2 py-2">
                  <span className="flex flex-wrap items-center gap-x-2 text-micro font-bold uppercase tracking-wide text-foreground">
                    Total no recorte
                    <span className="font-medium normal-case tracking-normal text-muted-foreground">
                      {totals.lines} {totals.lines === 1 ? "linha auditável" : "linhas auditáveis"}
                    </span>
                    {totals.overdue > 0 && (
                      <span className="font-medium normal-case tracking-normal text-status-critical">{totals.overdue} em atraso</span>
                    )}
                  </span>
                </td>
                {showLines && <td />}
                <td className="px-2 py-2 text-right font-mono font-bold tabular-nums text-status-info">
                  {formatMoneyBR(totals.receivable, { withSymbol: true })}
                </td>
                <td className="px-2 py-2 text-right font-mono font-bold tabular-nums text-status-warning">
                  {formatMoneyBR(totals.payable, { withSymbol: true })}
                </td>
                <td />
              </tr>
            </tfoot>
          </table>
        </div>
      ) : (
        <ul className="divide-y divide-border">
          {sections.map((section) => (
            <li key={section.key}>
              {showGroupRows && (
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 bg-muted/60 px-3 py-2">
                  <span className="text-micro font-bold uppercase tracking-wide text-foreground">{section.label}</span>
                  <span className="text-caption text-muted-foreground">
                    {section.count} {section.count === 1 ? "operação" : "operações"}
                  </span>
                  {section.overdueCount > 0 && <OverdueChip count={section.overdueCount} />}
                  <span className="ml-auto flex flex-wrap items-center gap-x-2 font-mono text-caption font-bold tabular-nums">
                    {section.receivable > 0 && (
                      <span className="text-status-info">a receber {formatMoneyBR(section.receivable, { withSymbol: true })}</span>
                    )}
                    {section.payable > 0 && (
                      <span className="text-status-warning">a pagar {formatMoneyBR(section.payable, { withSymbol: true })}</span>
                    )}
                  </span>
                </div>
              )}
              <ul className="divide-y divide-border">
                {section.operations.map((operation) => {
                  const summary = summaries.get(operation.key) ?? summarizeOperation(operation);
                  return (
                    <li key={operation.key}>
                      <OperationCard
                        operation={operation}
                        summary={summary}
                        expanded={expanded.has(operation.key)}
                        onToggle={() => toggle(operation.key)}
                        onSelect={onSelect}
                      />
                    </li>
                  );
                })}
              </ul>
            </li>
          ))}
        </ul>
      )}

      <p className="border-t border-border bg-muted/30 px-3 py-2 text-caption text-muted-foreground">
        Entradas e saídas ficam em colunas separadas: a mesma obrigação aparece em mais de uma linha do evento econômico e não deve ser somada.
      </p>
    </div>
  );
}

/**
 * Cartão da operação no mobile, no mesmo desenho do `ui/responsive-table`:
 * título como cabeçalho e o restante em lista de definição de duas colunas.
 */
function OperationCard({
  operation,
  summary,
  expanded,
  onToggle,
  onSelect,
}: {
  operation: EconomicOperation;
  summary: OperationSummary;
  expanded: boolean;
  onToggle: () => void;
  onSelect: (lancamento: Lancamento) => void;
}) {
  const origin = operation.primary.presentation?.property_title || operation.subtitle || operation.primary.origin.label || "—";
  const facts: { label: string; value: string; tone?: string }[] = [
    { label: "Competência", value: formatMonth(summary.competence) },
    { label: "Vencimento", value: formatDate(summary.dueAt), tone: summary.overdue ? "text-status-critical" : undefined },
    {
      label: "A receber",
      value: summary.receivable > 0 ? formatMoneyBR(summary.receivable, { withSymbol: true }) : "—",
      tone: summary.receivable > 0 ? "text-status-info" : undefined,
    },
    {
      label: "A pagar",
      value: summary.payable > 0 ? formatMoneyBR(summary.payable, { withSymbol: true }) : "—",
      tone: summary.payable > 0 ? "text-status-warning" : undefined,
    },
    { label: "Linhas", value: String(operation.items.length) },
    { label: "Origem", value: origin },
  ];

  return (
    <article className="bg-card p-3">
      <button
        type="button"
        onClick={() => (operation.items.length === 1 ? onSelect(operation.primary) : onToggle())}
        aria-expanded={operation.items.length > 1 ? expanded : undefined}
        className="flex w-full flex-wrap items-start justify-between gap-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        <span className="min-w-[10rem] flex-1">
          {/* Duas linhas: o nome da operação é o identificador principal e
              truncá-lo em uma linha esconde justamente o que distingue. */}
          <span className="block text-title font-bold text-foreground line-clamp-2">{operation.title}</span>
          <span className="mt-0.5 block truncate text-caption text-muted-foreground">{origin}</span>
        </span>
        <span className="flex shrink-0 items-center gap-1.5">
          <OperationSituation summary={summary} />
          {operation.items.length > 1 && (
            <ChevronDown className={clsx("size-4 text-muted-foreground transition-transform", expanded && "rotate-180")} aria-hidden="true" />
          )}
        </span>
      </button>

      <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-2">
        {facts.map((fact) => (
          <div key={fact.label} className="min-w-0">
            <dt className="truncate text-micro font-bold uppercase tracking-wide text-muted-foreground">{fact.label}</dt>
            <dd className={clsx("truncate text-caption font-semibold tabular-nums", fact.tone ?? "text-foreground/90")}>{fact.value}</dd>
          </div>
        ))}
      </dl>

      {expanded && operation.items.length > 1 && (
        <ul className="mt-3 space-y-1.5 border-t border-border pt-3">
          {operation.items.map((item) => (
            <li key={`${item.kind}:${item.id}`}>
              <button
                type="button"
                onClick={() => onSelect(item)}
                className="flex w-full items-center justify-between gap-2 rounded-lg px-1.5 py-1 text-left transition-colors hover:bg-muted/40"
              >
                <span className="flex min-w-0 items-center gap-1.5">
                  <KindTag kind={item.kind} />
                  <span className="truncate text-caption text-foreground">{item.counterparty.label}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <span className="font-mono text-caption font-bold tabular-nums text-foreground">
                    {formatMoneyBR(item.amount, { withSymbol: true })}
                  </span>
                  <StatusChip lancamento={item} />
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </article>
  );
}

function OverdueChip({ count }: { count: number }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-status-critical/30 bg-status-critical-soft px-1.5 py-0.5 text-micro font-bold normal-case tracking-normal text-status-critical">
      <AlertTriangle className="size-3" aria-hidden="true" /> {count} em atraso
    </span>
  );
}

function OperationSituation({ summary }: { summary: OperationSummary }) {
  if (summary.overdue) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-status-critical/30 bg-status-critical-soft px-2 py-0.5 text-micro font-bold text-status-critical">
        <AlertTriangle className="size-3" aria-hidden="true" /> Em atraso
      </span>
    );
  }
  if (summary.closed) {
    return (
      <span className="inline-flex items-center rounded-full border border-status-ok/30 bg-status-ok-soft px-2 py-0.5 text-micro font-bold text-status-ok">
        Concluída
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-full border border-status-info/30 bg-status-info-soft px-2 py-0.5 text-micro font-bold text-status-info">
      {summary.openLines} em aberto
    </span>
  );
}

function KindTag({ kind }: { kind: Lancamento["kind"] }) {
  return (
    <span className="shrink-0 rounded border border-border bg-muted/60 px-1.5 py-0.5 text-micro font-bold uppercase tracking-wide text-muted-foreground">
      {KIND_LABEL[kind]}
    </span>
  );
}

function StatusChip({ lancamento }: { lancamento: Lancamento }) {
  const variant = lancamentoSituationVariant(lancamento);
  // O repasse é a única linha com estado de apresentação próprio ("Sem saldo",
  // "Liquidado sem saldo"): o status canônico do lançamento continua sendo a
  // base, mas não sabe expressar "nada havia a repassar". A cobrança segue a
  // régua de vencimento do backend ("Vence hoje" não é atraso) e as demais
  // linhas leem o rótulo canônico.
  const label = lancamento.kind === "transfer"
    ? getOwnerTransferPresentation(lancamento.source as OwnerTransfer).status
    : lancamentoSituationLabel(lancamento);
  return (
    <span className={clsx("inline-flex shrink-0 rounded-md border px-1.5 py-0.5 text-micro font-semibold", variant.bg, variant.text, variant.border)}>
      {label}
    </span>
  );
}

import { Fragment, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Briefcase, ChevronLeft, ChevronRight, Crown, FileText, Home, Wallet } from "lucide-react";
import { clsx } from "clsx";

import { formatMoneyBR, formatISODateBR } from "../../utils/format";
import { groupFinanceList, groupHeaderCell } from "../../pages/Finance/listGroups";
import { computeDivergence, describeDivergence, divergenceReasonLabel } from "../../lib/lancamentos/divergence";
import { lancamentoSituationLabel, lancamentoSituationVariant } from "../../lib/lancamentos/chargeDue";
import type { Lancamento, LancamentoKind } from "../../lib/lancamentos/types";
import { TooltipText } from "../../ui/TooltipText";

interface LancamentosTableProps {
  lancamentos: Lancamento[];
  onSelect: (l: Lancamento) => void;
  isLoading?: boolean;
  extensionColumn?: {
    header: string;
    render: (l: Lancamento) => React.ReactNode;
  };
}

const KIND_META: Record<LancamentoKind, { label: string; icon: typeof FileText; bg: string; text: string }> = {
  charge: { label: "Cobrança", icon: FileText, bg: "bg-status-info-soft", text: "text-status-info" },
  commission: { label: "Comissão", icon: Briefcase, bg: "bg-primary-50", text: "text-primary-700" },
  transfer: { label: "Repasse", icon: Home, bg: "bg-status-warning-soft", text: "text-status-warning" },
  royalty: { label: "Parceria", icon: Crown, bg: "bg-accent", text: "text-accent-foreground" },
  transaction: { label: "Caixa", icon: Wallet, bg: "bg-muted", text: "text-muted-foreground" },
};

const formatDate = (iso?: string) => {
  if (!iso) return "—";
  return formatISODateBR(iso) || "—";
};

export function LancamentosTable({ lancamentos, onSelect, isLoading, extensionColumn }: LancamentosTableProps) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const totalPages = Math.max(1, Math.ceil(lancamentos.length / pageSize));
  const pageItems = useMemo(
    () => lancamentos.slice((page - 1) * pageSize, page * pageSize),
    [lancamentos, page, pageSize],
  );
  const groupedPage = useMemo(() => groupFinanceList(pageItems, (l) => l.status), [pageItems]);

  useEffect(() => {
    setPage(1);
  }, [lancamentos, pageSize]);

  if (isLoading && lancamentos.length === 0) {
    return <div className="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">Carregando lançamentos…</div>;
  }

  if (lancamentos.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border bg-card p-12 text-center">
        <p className="text-sm font-bold text-foreground">Nenhum lançamento encontrado</p>
        <p className="mt-1 text-xs text-muted-foreground">Ajuste os filtros ou aguarde a próxima sincronização.</p>
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="hidden md:block overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-muted-foreground">
            <tr className="text-left">
              <Th>Tipo</Th>
              <Th>Contraparte</Th>
              <Th>Origem</Th>
              <Th className="text-right">Valor</Th>
              <Th>Data-chave</Th>
              <Th>Situação</Th>
              {extensionColumn && <Th>{extensionColumn.header}</Th>}
              <Th className="w-8" srOnly>
                Ações
              </Th>
            </tr>
          </thead>
          <tbody>
            {groupedPage.map((group) => (
              <Fragment key={group.key}>
                <tr className="bg-muted/40">{groupHeaderCell(group.label, group.items.length, 7 + (extensionColumn ? 1 : 0))}</tr>
                {group.items.map((l) => (
                  <Row key={`${l.kind}-${l.id}`} l={l} onSelect={onSelect} extension={extensionColumn?.render(l)} />
                ))}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>

      <ul className="md:hidden divide-y divide-border">
        {groupedPage.map((group) => (
          <Fragment key={group.key}>
            <li className="bg-muted/50 px-4 py-2 text-xs font-bold uppercase tracking-wider flex items-center gap-2">
              <span className="size-1.5 rounded-full bg-current" />{group.label}
              <span className="text-muted-foreground font-medium normal-case tracking-normal">({group.items.length})</span>
            </li>
            {group.items.map((l) => {
              const kind = KIND_META[l.kind];
              const variant = lancamentoSituationVariant(l);
              const situation = lancamentoSituationLabel(l);
          return (
            <li key={`${l.kind}-${l.id}`}>
              <button type="button" onClick={() => onSelect(l)} className="w-full text-left p-4 hover:bg-muted/30 transition-colors">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <KindBadge kind={l.kind} />
                    <p className="mt-1 text-sm font-bold text-foreground truncate">{l.counterparty.label}</p>
                    <p className="text-xs text-muted-foreground truncate">{l.origin.label ?? kind.label}</p>
                  </div>
                  <div className="text-right shrink-0">
                    <p className="text-sm font-bold tabular-nums text-foreground">{formatMoneyBR(l.amount, { withSymbol: true })}</p>
                    <p className="text-xs text-muted-foreground">{formatDate(relevantDate(l).date)}</p>
                  </div>
                </div>
                <div className="mt-2 flex items-center gap-1.5 flex-wrap">
                  <span className={clsx("inline-flex items-center px-2 py-0.5 rounded-md text-xs font-semibold border", variant.bg, variant.text, variant.border)}>
                    {situation}
                  </span>
                  <DivergenceBadge lancamento={l} />
                </div>
              </button>
            </li>
          );
            })}
          </Fragment>
        ))}
      </ul>

      <div className="flex flex-col sm:flex-row items-center justify-between gap-3 border-t border-border bg-muted/20 px-4 py-3 text-xs text-muted-foreground">
        <span>
          {((page - 1) * pageSize) + 1}–{Math.min(page * pageSize, lancamentos.length)} de {lancamentos.length} registros
        </span>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2">
            Por página
            <select
              value={pageSize}
              onChange={(event) => setPageSize(Number(event.target.value))}
              className="h-8 rounded-lg border border-border bg-background px-2 text-xs text-foreground"
            >
              <option value={25}>25</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </label>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={page === 1}
              className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border bg-background text-foreground disabled:opacity-40"
              aria-label="Página anterior"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span className="min-w-16 text-center font-semibold text-foreground">
              {page} de {totalPages}
            </span>
            <button
              type="button"
              onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
              disabled={page === totalPages}
              className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border bg-background text-foreground disabled:opacity-40"
              aria-label="Próxima página"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function Th({ children, className, srOnly }: { children: React.ReactNode; className?: string; srOnly?: boolean }) {
  return (
    <th className={clsx("px-4 py-2 text-xs font-bold uppercase tracking-wide", className)}>
      {srOnly ? <span className="sr-only">{children}</span> : children}
    </th>
  );
}

function Row({ l, onSelect, extension }: { l: Lancamento; onSelect: (l: Lancamento) => void; extension?: React.ReactNode }) {
  const variant = lancamentoSituationVariant(l);
  return (
    <tr onClick={() => onSelect(l)} className="border-t border-border cursor-pointer hover:bg-muted/30 transition-colors">
      <td className="px-4 py-3">
        <KindBadge kind={l.kind} />
      </td>
      <td className="px-4 py-3 max-w-[220px]">
        {/* Contraparte e origem são truncadas na célula: o texto completo
            precisa continuar alcançável por teclado e toque, não só por
            hover (o `title` nativo não aparece no foco). */}
        <TooltipText text={l.counterparty.label} className="block font-bold text-foreground truncate">
          {l.counterparty.label}
        </TooltipText>
      </td>
      <td className="px-4 py-3 max-w-[220px]">
        <TooltipText text={l.origin.label ?? ""} className="block text-muted-foreground truncate">
          {l.origin.label ?? "—"}
        </TooltipText>
      </td>
      <td className="px-4 py-3 text-right font-bold tabular-nums text-foreground">{formatMoneyBR(l.amount, { withSymbol: true })}</td>
      <td className="px-4 py-3 text-muted-foreground tabular-nums">
        <span className="block text-foreground">{formatDate(relevantDate(l).date)}</span>
        <span className="block text-[11px]">{relevantDate(l).label}</span>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className={clsx("inline-flex items-center px-2 py-0.5 rounded-md text-xs font-semibold border", variant.bg, variant.text, variant.border)}>
            {lancamentoSituationLabel(l)}
          </span>
          <DivergenceBadge lancamento={l} />
        </div>
      </td>
      {extension !== undefined && <td className="px-4 py-3 text-sm">{extension}</td>}
      <td className="px-4 py-3 text-right">
        <ChevronRight className="w-4 h-4 text-muted-foreground inline" aria-hidden="true" />
      </td>
    </tr>
  );
}

function relevantDate(lancamento: Lancamento): { date?: string; label: string } {
  if (lancamento.due_at && lancamento.status !== "pago" && lancamento.status !== "processado") {
    return { date: lancamento.due_at, label: "Vencimento" };
  }
  if (lancamento.settled_at) return { date: lancamento.settled_at, label: "Liquidação" };
  return { date: lancamento.created_at, label: "Criação" };
}

function KindBadge({ kind }: { kind: LancamentoKind }) {
  const meta = KIND_META[kind];
  const Icon = meta.icon;
  return (
    <span className={clsx("inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-bold", meta.bg, meta.text)}>
      <Icon className="w-3 h-3" aria-hidden="true" />
      {meta.label}
    </span>
  );
}

function DivergenceBadge({ lancamento }: { lancamento: Lancamento }) {
  const info = computeDivergence(lancamento);
  if (!info) return null;
  const description = describeDivergence(info);
  return (
    <span
      className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-status-warning/30 bg-status-warning-soft px-2 py-0.5 text-xs font-bold text-status-warning"
      title={`${description}. Abra o lançamento para resolver.`}
    >
      <AlertTriangle className="w-3 h-3" aria-hidden="true" />
      {divergenceReasonLabel(info)}
    </span>
  );
}

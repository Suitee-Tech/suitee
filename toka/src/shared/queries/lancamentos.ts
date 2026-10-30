import { listAllCharges, listAllCommissions, listAllTransfers, listAllPayouts, listAllTransactions } from "../services/financialLists";
import { useQueries } from "@tanstack/react-query";
import { useMemo } from "react";


import { mapLancamentos } from "../lib/lancamentos/mappers";
import { sortLancamentosForOperations, lancamentoMonth } from "../lib/lancamentos/presentation";
import { monthKeyOf } from "../lib/lancamentos/operationsView";

/** Valor dos seletores de competência/vencimento para "sem data". */
export const NONE_KEY = "none";
import type { CounterpartyKind, Lancamento, LancamentoKind, LancamentoStatus } from "../lib/lancamentos/types";

const STALE_TIME = 30 * 1000;

export const lancamentosKeys = {
  all: () => ["lancamentos"] as const,
  charges: () => ["lancamentos", "charges"] as const,
  commissions: () => ["lancamentos", "commissions"] as const,
  transfers: () => ["lancamentos", "transfers"] as const,
  payouts: () => ["lancamentos", "payouts", "royalty"] as const,
  transactions: () => ["lancamentos", "transactions"] as const,
};

export interface LancamentosFilters {
  search?: string;
  kinds?: LancamentoKind[];
  statuses?: LancamentoStatus[];
  flow?: "receber" | "pagar";
  from?: string;
  to?: string;
  dateBasis?: "operational" | "settled" | "created";
  counterparty?: {
    kind?: CounterpartyKind;
    id: string;
  };
  divergentOnly?: boolean;
  /** Competência (AAAA-MM) ou `none` para lançamentos sem competência. */
  competence?: string;
  /** Mês de vencimento (AAAA-MM) ou `none` para lançamentos sem vencimento. */
  dueMonth?: string;
  /** Atalho de "Em atraso": mantém apenas linhas vencidas. */
  overdueOnly?: boolean;
}

export interface UseLancamentosListResult {
  data: Lancamento[];
  isLoading: boolean;
  isError: boolean;
  errors: Array<unknown>;
  partialFailures: LancamentoKind[];
  refetchAll: () => void;
}

export function useLancamentosList(filters: LancamentosFilters = {}): UseLancamentosListResult {
  const queries = useQueries({
    queries: [
      // O `signal` do React Query é repassado para a coleta: sem ele, uma
      // varredura de páginas em andamento continuaria buscando a carteira
      // inteira depois que a tela saiu ou o filtro mudou.
      {
        queryKey: lancamentosKeys.charges(),
        queryFn: ({ signal }) => listAllCharges({ signal }),
        staleTime: STALE_TIME,
      },
      {
        queryKey: lancamentosKeys.commissions(),
        queryFn: ({ signal }) => listAllCommissions({ signal }),
        staleTime: STALE_TIME,
      },
      {
        queryKey: lancamentosKeys.transfers(),
        queryFn: ({ signal }) => listAllTransfers({ signal }),
        staleTime: STALE_TIME,
      },
      {
        queryKey: lancamentosKeys.payouts(),
        queryFn: ({ signal }) => listAllPayouts({ source_type: "royalty_settlement" }, { signal }),
        staleTime: STALE_TIME,
      },
      {
        queryKey: lancamentosKeys.transactions(),
        queryFn: ({ signal }) => listAllTransactions({ signal }),
        staleTime: STALE_TIME,
      },
    ],
  });

  const [chargesQ, commissionsQ, transfersQ, payoutsQ, transactionsQ] = queries;

  const lancamentos = useMemo(() => {
    const all = mapLancamentos({
      charges: chargesQ.data,
      commissions: commissionsQ.data,
      transfers: transfersQ.data,
      payouts: payoutsQ.data,
      transactions: transactionsQ.data,
    });
    return applyFilters(all, filters);
  }, [chargesQ.data, commissionsQ.data, transfersQ.data, payoutsQ.data, transactionsQ.data, filters]);

  const partialFailures: LancamentoKind[] = [];
  if (chargesQ.isError) partialFailures.push("charge");
  if (commissionsQ.isError) partialFailures.push("commission");
  if (transfersQ.isError) partialFailures.push("transfer");
  if (payoutsQ.isError) partialFailures.push("royalty");
  if (transactionsQ.isError) partialFailures.push("transaction");

  return {
    data: lancamentos,
    isLoading: queries.some((q) => q.isLoading || q.isFetching),
    isError: queries.every((q) => q.isError),
    errors: queries.map((q) => q.error).filter(Boolean),
    partialFailures,
    refetchAll: () => {
      queries.forEach((q) => q.refetch());
    },
  };
}

export function applyFilters(lancamentos: Lancamento[], filters: LancamentosFilters): Lancamento[] {
  const {
    search,
    kinds,
    statuses,
    flow,
    from,
    to,
    dateBasis = "operational",
    counterparty,
    divergentOnly,
    competence,
    dueMonth,
    overdueOnly,
  } = filters;
  const needle = search?.trim().toLowerCase();
  const kindSet = kinds && kinds.length > 0 ? new Set(kinds) : null;
  const statusSet = statuses && statuses.length > 0 ? new Set(statuses) : null;
  const fromTime = from ? Date.parse(from.includes("T") ? from : `${from}T00:00:00.000`) : null;
  const toTime = to ? Date.parse(to.includes("T") ? to : `${to}T23:59:59.999`) : null;

  const filtered = lancamentos.filter((l) => {
    if (kindSet && !kindSet.has(l.kind)) return false;
    if (statusSet && !statusSet.has(l.status)) return false;
    if (flow) {
      const matchesFlow =
        l.kind === "transaction"
          ? l.kind_data.direction === (flow === "receber" ? "entrada" : "saida")
          : flow === "receber"
            ? l.kind === "charge"
            : l.kind === "commission" || l.kind === "transfer" || l.kind === "royalty";
      if (!matchesFlow) return false;
    }
    if (divergentOnly && !l.timeline_summary.divergence_flag) return false;
    if (overdueOnly && l.status !== "vencido") return false;
    if (competence) {
      const month = lancamentoMonth(l);
      const matches = competence === NONE_KEY ? month === "sem-data" : month === competence;
      if (!matches) return false;
    }
    if (dueMonth) {
      const month = l.due_at ? monthKeyOf(l.due_at) : undefined;
      const matches = dueMonth === NONE_KEY ? !month : month === dueMonth;
      if (!matches) return false;
    }
    if (counterparty?.id) {
      if (l.counterparty.id !== counterparty.id) return false;
      if (counterparty.kind && l.counterparty.kind !== counterparty.kind) return false;
    }
    if (fromTime || toTime) {
      const refDate = dateBasis === "settled" ? l.settled_at : dateBasis === "created" ? l.created_at : l.due_at || l.created_at;
      const refTime = refDate ? Date.parse(refDate) : NaN;
      if (!Number.isFinite(refTime)) return false;
      if (fromTime && refTime < fromTime) return false;
      if (toTime && refTime > toTime) return false;
    }
    if (needle) {
      const source = l.source as { description?: string; category?: string; reference_month?: string; month?: string };
      const hay = [
        l.id,
        l.counterparty.label,
        l.origin.label,
        l.origin.entity_id,
        l.presentation?.operation_title,
        l.presentation?.operation_subtitle,
        source.description,
        source.category,
        source.reference_month,
        source.month,
      ].filter(Boolean).join(" ").toLowerCase();
      if (!hay.includes(needle)) return false;
    }
    return true;
  });
  return sortLancamentosForOperations(filtered);
}

export function groupByKind(lancamentos: Lancamento[]): Record<LancamentoKind, Lancamento[]> {
  const out: Record<LancamentoKind, Lancamento[]> = {
    charge: [],
    commission: [],
    transfer: [],
    royalty: [],
    transaction: [],
  };
  for (const l of lancamentos) out[l.kind].push(l);
  return out;
}

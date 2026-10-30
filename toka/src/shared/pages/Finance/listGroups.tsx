import type { ReactNode } from "react";

export type FinanceListBucketKey = "overdue" | "upcoming" | "done";

export interface FinanceListGroup<T> {
  key: FinanceListBucketKey;
  label: string;
  items: T[];
}

/** Rótulos dos grupos exibidos como cabeçalhos nas listas do financeiro. */
export const FINANCE_GROUP_LABELS: Record<FinanceListBucketKey, string> = {
  overdue: "Em atraso",
  upcoming: "A vencer",
  done: "Concluídas",
};

/**
 * Classifica um status exibido em um dos três baldes de urgência:
 * Em atraso (crítico) → A vencer (aberto) → Concluídas (encerrado).
 */
export function financeListBucket(status: string | null | undefined): FinanceListBucketKey {
  const s = (status || "").toLowerCase();
  if (["em atraso", "vencido", "atrasado", "overdue"].includes(s)) return "overdue";
  if (
    ["pago", "processado", "concluído", "concluido", "cancelado", "cancelada",
     "revertido", "reversado", "reversada", "sem saldo", "liquidado sem saldo"].includes(s)
  ) return "done";
  // pendente, parcial, aprovado e demais estados abertos
  return "upcoming";
}

/**
 * Agrupa uma lista ordenada em baldes de urgência, mantendo a ordem interna
 * de cada grupo e retornando apenas os grupos com itens, na ordem
 * Em atraso → A vencer → Concluídas.
 */
export function groupFinanceList<T>(items: T[], statusOf: (item: T) => string): FinanceListGroup<T>[] {
  const buckets: Record<FinanceListBucketKey, T[]> = { overdue: [], upcoming: [], done: [] };
  for (const item of items) buckets[financeListBucket(statusOf(item))].push(item);
  return ([
    { key: "overdue", label: FINANCE_GROUP_LABELS.overdue, items: buckets.overdue },
    { key: "upcoming", label: FINANCE_GROUP_LABELS.upcoming, items: buckets.upcoming },
    { key: "done", label: FINANCE_GROUP_LABELS.done, items: buckets.done },
  ] as FinanceListGroup<T>[]).filter((g) => g.items.length > 0);
}

/** Renderiza uma célula de cabeçalho de grupo (linha full-width). */
export function groupHeaderCell(label: string, count: number, colSpan: number, className?: string): ReactNode {
  return (
    <td colSpan={colSpan} className={className || "group-header-cell px-5 py-2.5"}>
      <span className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider">
        <span className="size-1.5 rounded-full bg-current" />
        {label}
        <span className="font-medium text-muted-foreground normal-case tracking-normal">({count})</span>
      </span>
    </td>
  );
}

import { formatCompetencia } from "../../utils/format";
import type { EconomicOperation } from "./presentation";
import type { Lancamento, LancamentoKind, LancamentoStatus } from "./types";

/**
 * Leitura operacional da lista de operações: competência, vencimento,
 * prioridade e agrupamento.
 *
 * A lista de lançamentos cresce todo mês e, sem eixo de leitura, vira uma
 * pilha. Aqui ficam as regras — puras e testáveis — que respondem "o que é
 * deste mês?", "o que vence este mês?", "o que está atrasado?" e "como
 * agrupar isso de um jeito que faça sentido".
 */

const OPEN_STATUSES = new Set<LancamentoStatus>(["pendente", "aprovado", "parcial", "vencido"]);

export const NO_COMPETENCE_KEY = "sem-data";

/** Chave de mês (AAAA-MM) a partir de uma data ISO. */
export function monthKeyOf(iso?: string | null): string | undefined {
  const value = (iso ?? "").trim();
  if (value.length < 7) return undefined;
  const match = /^(\d{4})-(\d{2})/.exec(value);
  return match ? `${match[1]}-${match[2]}` : undefined;
}

/** Competência do mês corrente — o "agora" que a lista deve priorizar. */
export function currentMonthKey(now: Date = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
}

export function monthLabel(month?: string | null): string {
  if (!month) return "Sem competência";
  if (month === NO_COMPETENCE_KEY) return "Sem competência";
  return formatCompetencia(month);
}

const isSaida = (l: Lancamento): boolean => {
  if (l.kind === "commission" || l.kind === "transfer" || l.kind === "royalty") return true;
  if (l.kind === "transaction") return l.kind_data.direction === "saida";
  return false;
};

function competenceOfItem(l: Lancamento): string {
  const reference = (l.presentation?.reference_month ?? "").trim();
  if (reference.length >= 7) return reference.slice(0, 7);
  return monthKeyOf(l.due_at) ?? monthKeyOf(l.settled_at) ?? monthKeyOf(l.created_at) ?? NO_COMPETENCE_KEY;
}

function safeTime(value?: string): number {
  const parsed = value ? Date.parse(value) : NaN;
  return Number.isFinite(parsed) ? parsed : 0;
}

export interface OperationSummary {
  /** Competência predominante entre as linhas da operação. */
  competence: string;
  /** Vencimento mais próximo entre as linhas em aberto. */
  dueAt?: string;
  dueMonth?: string;
  totalLines: number;
  openLines: number;
  /** Em aberto a receber (cobranças e entradas). */
  receivable: number;
  /** Em aberto a pagar (comissões, repasses, royalties e saídas). */
  payable: number;
  kinds: LancamentoKind[];
  overdue: boolean;
  /** Nenhuma linha em aberto. */
  closed: boolean;
}

/**
 * Resumo de uma operação para planilha e agrupamentos. Não soma linhas
 * vinculadas em um único "valor": entradas e saídas ficam separadas porque a
 * mesma obrigação aparece em mais de uma linha do mesmo evento econômico.
 */
export function summarizeOperation(operation: EconomicOperation): OperationSummary {
  const competenceVotes = new Map<string, { count: number; earliest: string }>();
  let dueAt: string | undefined;
  let receivable = 0;
  let payable = 0;
  let openLines = 0;

  for (const item of operation.items) {
    const competence = competenceOfItem(item);
    const vote = competenceVotes.get(competence);
    if (vote) vote.count += 1;
    else competenceVotes.set(competence, { count: 1, earliest: competence });

    if (!OPEN_STATUSES.has(item.status)) continue;
    openLines += 1;
    const value = Math.abs(item.net_amount ?? item.amount ?? 0);
    if (isSaida(item)) payable += value;
    else receivable += value;

    if (item.due_at && (!dueAt || safeTime(item.due_at) < safeTime(dueAt))) dueAt = item.due_at;
  }

  // Competência predominante; empate resolve pela mais antiga, que é a
  // competência de origem do evento econômico.
  let competence = NO_COMPETENCE_KEY;
  let best = -1;
  for (const [key, vote] of competenceVotes) {
    if (vote.count > best) {
      best = vote.count;
      competence = key;
    }
  }

  const kinds = Array.from(new Set(operation.items.map((item) => item.kind)));
  const overdue = operation.items.some((item) => item.status === "vencido");

  return {
    competence,
    dueAt,
    dueMonth: dueAt ? monthKeyOf(dueAt) : undefined,
    totalLines: operation.items.length,
    openLines,
    receivable,
    payable,
    kinds,
    overdue,
    closed: openLines === 0,
  };
}

export type OperationGroupId = "competence" | "due" | "origin" | "kind" | "status" | "counterparty" | "none";

export const OPERATION_GROUPS: readonly { id: OperationGroupId; label: string }[] = [
  { id: "competence", label: "Competência" },
  { id: "due", label: "Vencimento" },
  { id: "origin", label: "Origem" },
  { id: "kind", label: "Tipo" },
  { id: "status", label: "Situação" },
  { id: "counterparty", label: "Contraparte" },
  { id: "none", label: "Sem agrupamento" },
];

export type OperationSortId = "focus" | "due" | "recent" | "amount";

export const OPERATION_SORTS: readonly { id: OperationSortId; label: string; hint: string }[] = [
  { id: "focus", label: "Mês atual primeiro", hint: "Atrasados e o mês corrente no topo" },
  { id: "due", label: "Vencimento", hint: "Do vencimento mais próximo ao mais distante" },
  { id: "recent", label: "Mais recentes", hint: "Última movimentação primeiro" },
  { id: "amount", label: "Maior valor", hint: "Soma em aberto, do maior ao menor" },
];

const KIND_LABEL: Record<LancamentoKind, string> = {
  charge: "Cobrança",
  commission: "Comissão",
  transfer: "Repasse",
  royalty: "Parcerias",
  transaction: "Caixa",
};

/**
 * Ranque de foco: atraso → mês corrente → competência passada ainda em aberto
 * → o que ainda vai vencer → encerradas → sem data. O passado em aberto vem
 * antes do futuro de propósito: é dinheiro que ficou para trás.
 */
function focusRank(summary: OperationSummary, now: Date): number {
  if (summary.overdue) return 0;
  const month = currentMonthKey(now);
  if (summary.competence === month || summary.dueMonth === month) return 1;
  if (summary.closed) return 5;
  if (summary.competence !== NO_COMPETENCE_KEY && summary.competence < month) return 2;
  if (summary.competence !== NO_COMPETENCE_KEY || summary.dueMonth) return 3;
  return 4;
}

/**
 * Ordena as operações. O padrão (`focus`) coloca o que está atrasado e o que
 * é do mês corrente no topo — é o que evita ter de varrer a lista inteira
 * para achar o mês que o operador está trabalhando.
 */
export function sortOperations(
  operations: readonly EconomicOperation[],
  sort: OperationSortId = "focus",
  now: Date = new Date(),
): EconomicOperation[] {
  const withSummary = operations.map((operation) => ({ operation, summary: summarizeOperation(operation) }));

  withSummary.sort((a, b) => {
    if (sort === "focus") {
      const rank = focusRank(a.summary, now) - focusRank(b.summary, now);
      if (rank !== 0) return rank;
    }
    if (sort === "amount") {
      const totalA = a.summary.receivable + a.summary.payable;
      const totalB = b.summary.receivable + b.summary.payable;
      if (totalB !== totalA) return totalB - totalA;
    }
    if (sort === "recent") {
      const recent = safeTime(b.operation.latestAt) - safeTime(a.operation.latestAt);
      if (recent !== 0) return recent;
    } else {
      // `due` e o desempate do `focus`: vencimento mais próximo primeiro;
      // operações sem vencimento (ou já encerradas) vão para o fim.
      const dueA = a.summary.dueAt ? safeTime(a.summary.dueAt) : Number.POSITIVE_INFINITY;
      const dueB = b.summary.dueAt ? safeTime(b.summary.dueAt) : Number.POSITIVE_INFINITY;
      if (dueA !== dueB) return dueA - dueB;
    }
    return safeTime(b.operation.latestAt) - safeTime(a.operation.latestAt) || a.operation.key.localeCompare(b.operation.key);
  });

  return withSummary.map((entry) => entry.operation);
}

export interface OperationSection {
  key: string;
  label: string;
  hint?: string;
  count: number;
  receivable: number;
  payable: number;
  overdueCount: number;
  operations: EconomicOperation[];
}

function operationOriginLabel(operation: EconomicOperation): { key: string; label: string; hint?: string } {
  const presentation = operation.primary.presentation;
  if (presentation?.contract_id) {
    return {
      key: `contract:${presentation.contract_id}`,
      label: presentation.property_title || operation.title,
      hint: operation.subtitle,
    };
  }
  const label = operation.primary.origin.label;
  if (label) return { key: `origin:${label}`, label };
  return { key: "origin:sem-origem", label: "Sem origem identificada" };
}

function operationStatusLabel(summary: OperationSummary): { key: string; label: string } {
  if (summary.overdue) return { key: "overdue", label: "Em atraso" };
  if (summary.closed) return { key: "done", label: "Concluídas" };
  return { key: "upcoming", label: "A vencer" };
}

/**
 * Agrupa operações em seções com subtotais. A ordem dos grupos também é
 * operacional: competência e vencimento colocam o mês corrente primeiro e o
 * que não tem data no fim.
 */
export function groupOperations(
  operations: readonly EconomicOperation[],
  groupId: OperationGroupId,
  sort: OperationSortId = "focus",
  now: Date = new Date(),
): OperationSection[] {
  const sorted = sortOperations(operations, sort, now);
  if (groupId === "none") {
    return [buildSection({ key: "all", label: "Todas as operações" }, sorted)];
  }

  const buckets = new Map<string, { label: string; hint?: string; operations: EconomicOperation[] }>();
  for (const operation of sorted) {
    const descriptor = describeBucket(operation, groupId, now);
    const bucket = buckets.get(descriptor.key);
    if (bucket) bucket.operations.push(operation);
    else buckets.set(descriptor.key, { label: descriptor.label, hint: descriptor.hint, operations: [operation] });
  }

  const sections = Array.from(buckets, ([key, bucket]) =>
    buildSection({ key, label: bucket.label, hint: bucket.hint }, bucket.operations),
  );

  return orderSections(sections, groupId, now);
}

function buildSection(
  meta: { key: string; label: string; hint?: string },
  operations: EconomicOperation[],
): OperationSection {
  let receivable = 0;
  let payable = 0;
  let overdueCount = 0;
  for (const operation of operations) {
    const summary = summarizeOperation(operation);
    receivable += summary.receivable;
    payable += summary.payable;
    if (summary.overdue) overdueCount += 1;
  }
  return { ...meta, count: operations.length, receivable, payable, overdueCount, operations };
}

function describeBucket(
  operation: EconomicOperation,
  groupId: OperationGroupId,
  now: Date,
): { key: string; label: string; hint?: string } {
  const summary = summarizeOperation(operation);
  switch (groupId) {
    case "competence":
      return {
        key: summary.competence,
        label: monthLabel(summary.competence),
        hint: summary.competence === currentMonthKey(now) ? "Mês corrente" : undefined,
      };
    case "due": {
      if (!summary.dueMonth) return { key: "due:none", label: "Sem vencimento em aberto" };
      return {
        key: summary.dueMonth,
        label: monthLabel(summary.dueMonth),
        hint: summary.dueMonth === currentMonthKey(now) ? "Vence este mês" : undefined,
      };
    }
    case "origin":
      return operationOriginLabel(operation);
    case "kind": {
      const [first] = summary.kinds;
      if (summary.kinds.length > 1) return { key: "kind:misto", label: "Operações mistas" };
      return { key: `kind:${first}`, label: KIND_LABEL[first] ?? "Outros" };
    }
    case "status":
      return operationStatusLabel(summary);
    case "counterparty": {
      const label = operation.primary.counterparty.label || "Sem contraparte";
      return { key: `cp:${operation.primary.counterparty.id ?? label}`, label };
    }
    default:
      return { key: "all", label: "Todas as operações" };
  }
}

function orderSections(sections: OperationSection[], groupId: OperationGroupId, now: Date): OperationSection[] {
  const reference = currentMonthKey(now);
  const monthRank = (month: string): number => {
    if (month === NO_COMPETENCE_KEY || month === "due:none") return 3;
    if (month === reference) return 0;
    return month < reference ? 1 : 2;
  };

  if (groupId === "competence" || groupId === "due") {
    return [...sections].sort((a, b) => {
      const rank = monthRank(a.key) - monthRank(b.key);
      if (rank !== 0) return rank;
      // Mesmo ranque: passado do mais recente ao mais antigo, futuro crescente.
      return a.key > reference && b.key > reference ? a.key.localeCompare(b.key) : b.key.localeCompare(a.key);
    });
  }

  if (groupId === "status") {
    const order: Record<string, number> = { overdue: 0, upcoming: 1, done: 2 };
    return [...sections].sort((a, b) => (order[a.key] ?? 9) - (order[b.key] ?? 9));
  }

  return [...sections].sort(
    (a, b) => b.overdueCount - a.overdueCount || b.count - a.count || a.label.localeCompare(b.label, "pt-BR"),
  );
}

/** Atalhos contextuais exibidos acima da lista, com contagem por recorte. */
export type OperationQuickFilterId = "overdue" | "receivable" | "payable" | "divergent";

export const OPERATION_QUICK_FILTERS: readonly { id: OperationQuickFilterId; label: string; hint: string }[] = [
  { id: "overdue", label: "Em atraso", hint: "Operações com alguma linha vencida" },
  { id: "receivable", label: "A receber", hint: "Cobranças e entradas em aberto" },
  { id: "payable", label: "A pagar", hint: "Comissões, repasses e royalties em aberto" },
  { id: "divergent", label: "Divergências", hint: "Linhas que precisam de conciliação" },
];

export function countQuickFilters(operations: readonly EconomicOperation[]): Record<OperationQuickFilterId, number> {
  const counts: Record<OperationQuickFilterId, number> = { overdue: 0, receivable: 0, payable: 0, divergent: 0 };
  for (const operation of operations) {
    const summary = summarizeOperation(operation);
    if (summary.overdue) counts.overdue += 1;
    if (summary.receivable > 0) counts.receivable += 1;
    if (summary.payable > 0) counts.payable += 1;
    if (operation.divergenceCount > 0) counts.divergent += 1;
  }
  return counts;
}

export interface OperationTotals {
  count: number;
  lines: number;
  receivable: number;
  payable: number;
  overdueCount: number;
}

export function summarizeOperations(operations: readonly EconomicOperation[]): OperationTotals {
  let lines = 0;
  let receivable = 0;
  let payable = 0;
  let overdueCount = 0;
  for (const operation of operations) {
    const summary = summarizeOperation(operation);
    lines += summary.totalLines;
    receivable += summary.receivable;
    payable += summary.payable;
    if (summary.overdue) overdueCount += 1;
  }
  return { count: operations.length, lines, receivable, payable, overdueCount };
}

/** Meses disponíveis para os seletores de competência e vencimento. */
export function collectMonthOptions(operations: readonly EconomicOperation[]): {
  competence: string[];
  due: string[];
} {
  const competence = new Set<string>();
  const due = new Set<string>();
  for (const operation of operations) {
    const summary = summarizeOperation(operation);
    if (summary.competence !== NO_COMPETENCE_KEY) competence.add(summary.competence);
    if (summary.dueMonth) due.add(summary.dueMonth);
  }
  const desc = (a: string, b: string) => b.localeCompare(a);
  return { competence: Array.from(competence).sort(desc), due: Array.from(due).sort(desc) };
}

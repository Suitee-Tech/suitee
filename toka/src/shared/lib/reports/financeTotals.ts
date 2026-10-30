import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";

/**
 * Semântica financeira do relatório da imobiliária.
 *
 * Princípio (do diagnóstico de 10/09): não converter recursos de terceiros em
 * receita própria. Numa imobiliária, o valor cobrado do inquilino/comprador é
 * ARRECADADO, não faturado pela agência:
 *
 * - a parcela do proprietário é dinheiro dele, em trânsito;
 * - a parcela do corretor é comissão devida a ele;
 * - só a taxa de administração/intermediação/retida é RECEITA da imobiliária.
 *
 * O backend já computa essa fronteira: a DRE soma as alocações econômicas
 * (allocation_type = "revenue") das cobranças pagas, e as charges carregam
 * line_items com amount_cents por titular. Aqui reproduzimos a mesma fronteira
 * no cliente para o que precisa de granularidade local (mês, status, drill-down);
 * totais absolutos seguem os endpoints do servidor.
 */

/** Total em centavos de uma cobrança, na unidade em que o backend a guarda. */
export const chargeTotalCents = (charge: Charge): number => {
  if (typeof charge.total_amount === "number" && charge.total_amount > 0) {
    return Math.round(charge.total_amount * 100);
  }
  return Math.round((charge.amount || 0) * 100);
};

/** Parcela da cobrança que pertence economicamente à agência, em centavos. */
export const chargeAgencyCents = (charge: Charge): number => {
  const items = charge.line_items;
  if (Array.isArray(items) && items.length > 0) {
    const revenue = items
      .filter((item) => item.allocation_type === "revenue")
      .reduce((sum, item) => sum + (item.amount_cents || 0), 0);
    if (revenue > 0) return revenue;
  }
  // Sem composição econômica, trata a cobrança inteira como da agência —
  // coerente com o comportamento do backend para fluxos legados.
  return chargeTotalCents(charge);
};

/** Saldo aberto de uma cobrança (total menos pago), em centavos. */
export const chargeOpenCents = (charge: Charge): number => {
  const paid = typeof charge.paid_amount === "number" ? Math.round(charge.paid_amount * 100) : 0;
  return Math.max(0, chargeTotalCents(charge) - paid);
};

export type ChargeReceivableStatus = "pago" | "pendente" | "vencido";

/** Normaliza o status de uma cobrança para as três categorias do relatório. */
export const chargeReceivableStatus = (charge: Charge): ChargeReceivableStatus => {
  const raw = String(charge.status ?? "").toUpperCase();
  if (raw === "PAID" || raw === "PAGA" || raw === "PAGO" || raw === "PARTIALLY_PAID" || raw === "PARCIAL") {
    return "pago";
  }
  if (raw === "OVERDUE" || raw === "VENCIDA" || raw === "VENCIDO" || raw === "EM ATRASO") {
    return "vencido";
  }
  return "pendente";
};

/** Data econômica de uma cobrança: pagamento quando existe, vencimento senão. */
export const chargeEconomicDate = (charge: Charge): string =>
  charge.paid_at || charge.due_date || charge.created_at;

/** Data econômica de um repasse: processamento quando existe, aprovação senão. */
export const transferEconomicDate = (transfer: OwnerTransfer): string =>
  transfer.processed_at || transfer.approved_at || transfer.created_at;

/** Data econômica de uma comissão: pagamento quando existe, criação senão. */
export const commissionEconomicDate = (commission: Commission): string =>
  commission.paid_at || commission.created_at;

/** Valor efetivo pago de uma comissão, em reais — líquido quando existe. */
export const commissionPaidValue = (commission: Commission): number => {
  if (typeof commission.net_amount === "number") return commission.net_amount;
  if (typeof commission.gross_amount === "number") return commission.gross_amount;
  return commission.amount;
};

export interface FinancePeriodTotals {
  /** Receita própria da agência no período (fronteira econômica), em reais. */
  revenue: number;
  /** Comissões pagas no período, em reais (saída efetiva de caixa). */
  commissionExpense: number;
  /** Repasses processados no período, em reais (saída a proprietários). */
  transfers: number;
  /** Receita própria recebida (cobranças pagas), em reais. */
  received: number;
  /** A receber: saldo aberto da carteira (fotografia, não do período). */
  pending: number;
  /** Em atraso: saldo aberto vencido da carteira (fotografia). */
  overdue: number;
  /** Arrecadado bruto da carteira (fotografia). */
  grossReceived: number;
}

/**
 * Posição da carteira de cobranças — uma FOTOGRAFIA, não um recorte de período.
 *
 * "A receber" e "em atraso" listam o que está em aberto hoje. Amarrá-los à
 * janela do relatório escondia a carteira inteira quando as parcelas venciam
 * fora do período (uma locação de 30 meses, por exemplo): o painel mostrava
 * R$ 0 a receber com 29 cobranças em aberto.
 */
export interface ChargePortfolio {
  /** Saldo aberto de cobranças pendentes, em reais. */
  pending: number;
  /** Saldo aberto de cobranças vencidas, em reais. */
  overdue: number;
  /** Valor bruto das cobranças já pagas, em reais — independente de período. */
  grossReceived: number;
}

export function computeChargePortfolio(charges: Charge[]): ChargePortfolio {
  let pendingCents = 0;
  let overdueCents = 0;
  let grossReceivedCents = 0;

  for (const charge of charges) {
    const status = chargeReceivableStatus(charge);
    if (status === "pago") {
      grossReceivedCents += chargeTotalCents(charge);
    } else if (status === "vencido") {
      overdueCents += chargeOpenCents(charge);
    } else {
      pendingCents += chargeOpenCents(charge);
    }
  }

  return {
    pending: centsToReais(pendingCents),
    overdue: centsToReais(overdueCents),
    grossReceived: centsToReais(grossReceivedCents),
  };
}

export interface FinanceMonthlyBucket {
  /** Chave "YYYY-MM" do mês econômico. */
  month: string;
  revenue: number;
  expense: number;
}

const MONTHS_PT = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];

export const monthLabelOf = (key: string): string => {
  const [, month] = key.split("-");
  const index = Number(month) - 1;
  return MONTHS_PT[index] ?? key;
};

export const monthKeyOf = (iso: string): string => {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
};

const centsToReais = (cents: number): number => cents / 100;

interface PeriodWindow {
  from: Date;
  to: Date;
}

/**
 * Totais financeiros de um período, a partir das listas completas que o
 * contexto já carrega. As listas são as mesmas que a Conferência usa, então a
 * fronteira econômica é idêntica à dela.
 */
export function computeFinanceTotals(
  charges: Charge[],
  transfers: OwnerTransfer[],
  commissions: Commission[],
  window: PeriodWindow,
): FinancePeriodTotals {
  const inWindow = (iso: string | undefined): boolean => {
    if (!iso) return false;
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return false;
    return date >= window.from && date <= window.to;
  };

  let revenueCents = 0;
  let commissionExpenseCents = 0;
  let transfersCents = 0;
  let receivedCents = 0;

  // Receita recebida é do PERÍODO (cobranças pagas na janela). A carteira
  // aberta vem de computeChargePortfolio, que é fotografia.
  for (const charge of charges) {
    if (!inWindow(chargeEconomicDate(charge))) continue;
    if (chargeReceivableStatus(charge) !== "pago") continue;
    revenueCents += chargeAgencyCents(charge);
    receivedCents += chargeAgencyCents(charge);
  }

  for (const transfer of transfers) {
    const status = String(transfer.status ?? "").toUpperCase();
    if (status !== "PROCESSED" && status !== "PROCESSADA" && status !== "CONCLUIDO") continue;
    if (!inWindow(transferEconomicDate(transfer))) continue;
    transfersCents += Math.round((transfer.net_amount || 0) * 100);
  }

  for (const commission of commissions) {
    const status = String(commission.status ?? "").toUpperCase();
    if (status !== "PAID" && status !== "PAGA" && status !== "PAGO") continue;
    if (!inWindow(commissionEconomicDate(commission))) continue;
    commissionExpenseCents += Math.round(commissionPaidValue(commission) * 100);
  }

  const portfolio = computeChargePortfolio(charges);

  return {
    revenue: centsToReais(revenueCents),
    commissionExpense: centsToReais(commissionExpenseCents),
    transfers: centsToReais(transfersCents),
    received: centsToReais(receivedCents),
    pending: portfolio.pending,
    overdue: portfolio.overdue,
    grossReceived: portfolio.grossReceived,
  };
}

/** Série mensal de receita própria e despesas (comissões pagas), em reais. */
export function computeFinanceMonthly(
  charges: Charge[],
  commissions: Commission[],
  window: PeriodWindow,
): FinanceMonthlyBucket[] {
  const buckets = new Map<string, FinanceMonthlyBucket>();
  const bucketFor = (month: string): FinanceMonthlyBucket => {
    let bucket = buckets.get(month);
    if (!bucket) {
      bucket = { month, revenue: 0, expense: 0 };
      buckets.set(month, bucket);
    }
    return bucket;
  };
  const inWindow = (iso: string | undefined): boolean => {
    if (!iso) return false;
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return false;
    return date >= window.from && date <= window.to;
  };

  for (const charge of charges) {
    const date = chargeEconomicDate(charge);
    if (!inWindow(date)) continue;
    const status = chargeReceivableStatus(charge);
    if (status !== "pago") continue;
    bucketFor(monthKeyOf(date)).revenue += centsToReais(chargeAgencyCents(charge));
  }
  for (const commission of commissions) {
    const status = String(commission.status ?? "").toUpperCase();
    if (status !== "PAID" && status !== "PAGA" && status !== "PAGO") continue;
    if (!inWindow(commissionEconomicDate(commission))) continue;
    bucketFor(monthKeyOf(commissionEconomicDate(commission))).expense += commissionPaidValue(commission);
  }

  // Completa os doze meses do ano corrente.
  //
  // Sem isto o gráfico "Receita Mensal" desenhava só os meses COM movimento: com
  // uma única competência lançada ele exibia uma barra isolada, sem o resto da
  // linha do tempo. Quem olha não consegue situar o mês no ano — e um painel
  // anual que mostra um mês só parece dado faltando, não ausência de movimento.
  // A série dos lançamentos (buildMonthlySummary) já preenche os doze.
  const year = window.to.getFullYear();
  return MONTHS_PT.map((_, index) => {
    const month = `${year}-${String(index + 1).padStart(2, "0")}`;
    return buckets.get(month) ?? { month, revenue: 0, expense: 0 };
  });
}

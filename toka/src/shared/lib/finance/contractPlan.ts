import type { Charge } from "../../services/charge.service";
import type { Invoice } from "../../finance/types";
import { formatCompetencia } from "../../utils/format";
import { normalizeChargeStatus } from "../../utils/charge";
import { chargeReceivableAmounts } from "./economicRecords";

/**
 * Plano financeiro de um contrato, na tela de detalhes.
 *
 * Os dois fluxos cobram por fontes diferentes e isso já causou um painel
 * vazio: locação é cobrada por `charges` vinculadas ao **contrato**
 * (competência + vencimento), enquanto venda usa `invoices` do **negócio**.
 * Ler a fonte errada devolve lista vazia mesmo com cobranças emitidas e pagas.
 */
export type ContractPlanSource = "rent" | "sale";

export interface ContractPlanRow {
  id: string;
  /** Competência na locação; "n/N" nas parcelas de venda. */
  label: string;
  dueDate?: string;
  amount: number;
  /** Status no vocabulário de cobrança (PENDING, PAID, PARTIALLY_PAID...). */
  status: string;
}

export interface ContractPlanTotals {
  /** Cobranças ativas (canceladas e reversas ficam fora). */
  count: number;
  emitted: number;
  received: number;
  open: number;
  overdue: number;
}

export interface ContractPlanInput {
  source: ContractPlanSource;
  charges?: readonly Charge[];
  invoices?: readonly Invoice[];
}

export function buildContractPlanRows({ source, charges = [], invoices = [] }: ContractPlanInput): ContractPlanRow[] {
  if (source === "rent") {
    return charges
      .map((charge) => ({
        id: charge.id,
        label: charge.reference_month
          ? formatCompetencia(charge.reference_month)
          : charge.description || "Cobrança",
        dueDate: charge.due_date,
        amount: charge.total_amount ?? charge.amount ?? 0,
        status: normalizeChargeStatus(charge.status),
      }))
      // O plano é lido em ordem cronológica de vencimento.
      .sort((a, b) => (a.dueDate ?? "").localeCompare(b.dueDate ?? ""));
  }

  return invoices.map((invoice, index) => ({
    id: invoice.id,
    label: `${index + 1}/${invoices.length}`,
    dueDate: invoice.due_date,
    amount: invoice.amount,
    status: normalizeChargeStatus(invoice.status),
  }));
}

/**
 * Totais do plano emitido. Reaproveita `chargeReceivableAmounts` para que o
 * valor recebido de uma baixa parcial não seja perdido e cancelamentos não
 * inflem os indicadores.
 */
export function summarizeContractPlan(charges: readonly Charge[] = []): ContractPlanTotals {
  const totals: ContractPlanTotals = { count: 0, emitted: 0, received: 0, open: 0, overdue: 0 };

  for (const charge of charges) {
    const rawStatus = String(charge.status ?? "").trim().toUpperCase();
    if (rawStatus === "CANCELLED" || rawStatus === "REVERSED") continue;

    const { paid, pending, overdue } = chargeReceivableAmounts(charge);
    totals.count += 1;
    totals.received += paid;
    totals.open += pending + overdue;
    totals.overdue += overdue;
  }

  totals.emitted = totals.received + totals.open;
  return totals;
}

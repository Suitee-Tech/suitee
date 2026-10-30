import { formatMoneyBR, formatCurrencyBRL } from "../../utils/format";
import type { Lancamento, TransferLancamento, TransactionLancamento } from "./types";

/**
 * Ações de liquidação/baixa manual confirmáveis na esteira financeira.
 * "settle"  -> baixa de caixa (sale transaction / cobrança já paga sem repasse).
 * "approve" -> repasse Pendente -> Aprovado (não move dinheiro).
 * "process" -> repasse Aprovado -> Processado (o valor sai de fato).
 */
export type SettlementConfirmAction = "settle" | "approve" | "process";

export interface SettlementEffectRow {
  label: string;
  value: string;
  accent?: "ok" | "critical" | "primary" | "muted";
}

export interface SettlementEffectsData {
  /** Frase que descreve o que está sendo feito. */
  title: string;
  /** Linha de contexto curta (contraparte / valor / origem). */
  context?: string;
  /** Destinos financeiros do valor movimentado. */
  effects: SettlementEffectRow[];
  /** Linhas de efeito contábil (partidas / lançamentos). */
  accounting: string[];
  /** Se a ação é desconstrutiva (cancela/estorna). */
  destructive?: boolean;
  /** Nota explicativa opcional sobre impacto/ordem. */
  impactNote?: string;
}

const money = (value: number) => formatMoneyBR(value, { withSymbol: true });
const moneyCents = (cents: number) => formatMoneyBR(cents / 100, { withSymbol: true });

const transferEffects = (l: TransferLancamento): SettlementEffectRow[] => {
  const bd = l.kind_data.economic_breakdown;
  const rows: SettlementEffectRow[] = [];
  if (bd) {
    const isSale = bd.flow === "sale" || l.kind_data.settlement_type === "sale";
    if (isSale) {
      rows.push({ label: "Valor recebido", value: moneyCents(bd.gross_cents) });
      if (bd.discount_cents > 0) rows.push({ label: "Descontos aplicados", value: `-${moneyCents(bd.discount_cents)}`, accent: "critical" });
      rows.push({ label: "Obrigação ao proprietário", value: moneyCents(bd.owner_payable_cents), accent: "ok" });
      if (bd.commission_payable_cents > 0) rows.push({ label: "Comissões a terceiros", value: moneyCents(bd.commission_payable_cents) });
      if (bd.partner_payable_cents > 0) rows.push({ label: "Obrigações a parceiros", value: moneyCents(bd.partner_payable_cents) });
      rows.push({ label: "Disponível para repasse", value: moneyCents(bd.available_cents), accent: "ok" });
    } else {
      rows.push({ label: "Aluguel recebido", value: moneyCents(bd.gross_cents) });
      if (bd.admin_fee_cents && bd.admin_fee_cents > 0) rows.push({ label: `Receita de administração (${l.kind_data.admin_fee_percent}%)`, value: moneyCents(bd.admin_fee_cents), accent: "ok" });
      if (bd.intermediation_revenue_cents && bd.intermediation_revenue_cents > 0) rows.push({ label: "Intermediação apropriada", value: moneyCents(bd.intermediation_revenue_cents) });
      if (bd.commission_funded_cents && bd.commission_funded_cents > 0) rows.push({ label: "Comissão coberta neste aluguel", value: moneyCents(bd.commission_funded_cents) });
      if (bd.reserved_cents > 0) rows.push({ label: "Reservado para encargos", value: moneyCents(bd.reserved_cents) });
      if (bd.available_cents > 0) rows.push({ label: "Repasse líquido ao proprietário", value: moneyCents(bd.available_cents), accent: "ok" });
    }
  } else {
    rows.push({ label: "Valor do repasse", value: money(l.amount) });
  }
  return rows;
};

const transactionEffects = (l: TransactionLancamento): SettlementEffectRow[] => {
  const bd = l.kind_data.economic_breakdown;
  if (bd && bd.flow === "sale") {
    return [
      { label: "Valor recebido", value: moneyCents(bd.gross_cents) },
      ...(bd.discount_cents > 0 ? [{ label: "Descontos aplicados", value: `-${moneyCents(bd.discount_cents)}`, accent: "critical" } as const] : []),
      { label: "Comissões a corretores", value: moneyCents(bd.commission_payable_cents) },
      ...(bd.partner_payable_cents > 0 ? [{ label: "Parceiros e entidades", value: moneyCents(bd.partner_payable_cents) } as const] : []),
      { label: "Obrigação ao proprietário", value: moneyCents(bd.owner_payable_cents), accent: "ok" },
      { label: "Receita bruta de intermediação", value: moneyCents(bd.agency_retained_cents + bd.commission_payable_cents) },
      { label: "Saldo retido pela imobiliária", value: moneyCents(bd.agency_retained_cents), accent: "ok" },
    ];
  }
  const direction = l.kind_data.direction;
  return [{ label: direction === "entrada" ? "Entrada de caixa" : "Saída de caixa", value: money(l.amount), accent: direction === "entrada" ? "ok" : "critical" }];
};

/**
 * Deriva o texto que descreve a ação de baixa/liquidação manual + os efeitos
 * financeiros (destinos) e contábeis (partidas/lançamentos) para a confirmação.
 * Função pura, sem dependência de UI, para ser testável e reutilizável.
 */
export function deriveSettlementEffects(action: SettlementConfirmAction, lancamento: Lancamento): SettlementEffectsData {
  switch (action) {
    case "approve": {
      const l = lancamento as TransferLancamento;
      const effects = transferEffects(l);
      return {
        title: "Aprovar repasse",
        context: `${l.counterparty.label} · ${money(l.amount)}`,
        effects,
        accounting: [
          "Nenhum valor é movimentado nesta etapa.",
          "A ordem passa de Pendente para Aprovada e fica liberada para processamento.",
        ],
        impactNote: "A aprovação não afeta o caixa. Confirme apenas para liberar a ordem ao fluxo de processamento.",
      };
    }
    case "process": {
      const l = lancamento as TransferLancamento;
      const bd = l.kind_data.economic_breakdown;
      const effects = transferEffects(l);
      const isSale = bd?.flow === "sale" || l.kind_data.settlement_type === "sale";
      return {
        title: "Processar repasse",
        context: `${l.counterparty.label} · ${money(l.amount)}`,
        effects,
        accounting: [
          isSale ? "Débito: Obrigação ao proprietário (liquidação da venda)." : "Débito: Obrigação ao proprietário (repasse líquido).",
          "Crédito: Caixa (saída de recursos).",
        ],
        impactNote: "Este é o passo em que o dinheiro sai. Confirme apenas quando o repasse tiver sido efetivado.",
      };
    }
    case "settle":
    default: {
      const l = lancamento as TransactionLancamento;
      const bd = l.kind_data.economic_breakdown;
      const effects = transactionEffects(l);
      const isSale = bd?.flow === "sale";
      const direction = l.kind_data.direction;
      return {
        title: "Dar baixa manual",
        context: `${l.counterparty.label} · ${money(l.amount)} · ${l.kind_data.transaction_type}`,
        effects,
        accounting: isSale
          ? [
              "Débito: Caixa (valor recebido).",
              "Crédito: Obrigação ao proprietário e Parceiros.",
              "Crédito: Receita bruta de intermediação.",
              "Débito: Despesa de comissão; crédito: Comissões a pagar.",
            ]
          : direction === "entrada"
            ? ["Débito: Caixa.", "Crédito: Receita / Conta a receber."]
            : ["Débito: Despesa / Obrigação.", "Crédito: Caixa."],
        impactNote: isSale
          ? "A obrigação do proprietário é criada no mesmo evento e registrada no repasse vinculado (baixa + repasse atômicos)."
          : undefined,
      };
    }
  }
}

/** Rótulo de contraparte + valor para usar como contexto curto em qualquer ação. */
export function settlementContextLabel(lancamento: Lancamento): string {
  return `${lancamento.counterparty.label} · ${formatCurrencyBRL(lancamento.amount)}`;
}

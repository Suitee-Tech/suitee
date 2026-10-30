import type { PayoutOrder } from "../services/payoutOrder.service";

export type PayoutOriginKind =
  | "sale_commission"
  | "rental_commission"
  | "royalty"
  | "manual"
  | "unknown";

export interface PayoutCoverageCopy {
  sectionTitle: string;
  title: string;
  description: string;
  emptyDescription: string;
  emptyBadge: string;
  emptyHint: string;
  emptyOutstandingHint: string;
  completedDescription: string;
  blockedPrefix: string;
  blockedSuffix: string;
  valueLabel: string;
  coveredLabel: string;
  outstandingLabel: string;
  ariaLabel: string;
}

const ORIGIN_LABEL: Record<PayoutOriginKind, string> = {
  sale_commission: "Comissão de venda",
  rental_commission: "Comissão de locação",
  royalty: "Liquidação de royalty",
  manual: "Ordem manual",
  unknown: "Origem não informada",
};

const DRAFT_LABEL: Record<PayoutOriginKind, string> = {
  sale_commission: "Aguardando recebimento da venda",
  rental_commission: "Aguardando aluguéis",
  royalty: "Aguardando liquidação do royalty",
  manual: "Aguardando cobertura",
  unknown: "Aguardando cobertura",
};

/**
 * A comissão de venda é liberada pelo recebimento da venda. O backend pode
 * cobri-la com outro caixa, mas a copy não pode chamar isso de "aluguéis".
 *
 * O detalhe ainda não recebe um `funding_basis` explícito. O fallback usa os
 * sinais disponíveis e cai em "unknown" quando não há certeza — nesse caso a
 * linguagem fica neutra em vez de inventar uma origem.
 */
export function payoutOriginKind(order: PayoutOrder): PayoutOriginKind {
  if (order.source_type === "royalty_settlement") return "royalty";

  const sourceRef = order.source_ref ?? {};
  const hasDeal = typeof sourceRef.deal_id === "string" && sourceRef.deal_id.trim().length > 0;
  const hasReferenceMonth =
    typeof sourceRef.reference_month === "string" && sourceRef.reference_month.trim().length > 0;

  if (hasDeal && !hasReferenceMonth) return "sale_commission";
  if (hasReferenceMonth || order.source_type === "charge") return "rental_commission";
  if (order.source_type === "manual") return "manual";
  return "unknown";
}

export function payoutOriginLabel(order: PayoutOrder): string {
  return ORIGIN_LABEL[payoutOriginKind(order)];
}

export function payoutStatusLabel(order: PayoutOrder): string {
  if (order.status === "DRAFT") return DRAFT_LABEL[payoutOriginKind(order)];
  return {
    PENDING_REVIEW: "Aguardando revisão",
    APPROVED: "Aprovado",
    PAID: "Pago",
    FAILED: "Falhou",
    CANCELLED: "Cancelado",
  }[order.status];
}

export function payoutCoverageCopy(order: PayoutOrder): PayoutCoverageCopy {
  const kind = payoutOriginKind(order);

  if (kind === "sale_commission") {
    return {
      sectionTitle: "Cobertura da comissão de venda",
      title: "Cobertura pelo recebimento da venda",
      description:
        "Recebimento da venda que cobre esta ordem. Se necessário, outro caixa pode complementar a cobertura.",
      emptyDescription:
        "Nenhum recebimento da venda registrado até agora. A comissão será liberada quando a venda for recebida; outro caixa pode complementar a cobertura.",
      emptyBadge: "Aguardando venda",
      emptyHint: "Nenhum recebimento da venda",
      emptyOutstandingHint: "Quando a venda for recebida",
      completedDescription: "Cobertura concluída pelo recebimento da venda. A obrigação pode seguir para revisão e pagamento.",
      blockedPrefix: "Aprovação ainda bloqueada. Faltam ",
      blockedSuffix:
        " de cobertura. A ordem será liberada quando o recebimento da venda entrar; outro caixa pode complementar a cobertura.",
      valueLabel: "Valor da comissão",
      coveredLabel: "Coberta por recebimentos",
      outstandingLabel: "Saldo a receber",
      ariaLabel: "Cobertura da comissão de venda pelo recebimento da venda",
    };
  }

  if (kind === "rental_commission") {
    return {
      sectionTitle: "Cobertura dos aluguéis",
      title: "Cobertura pelos aluguéis recebidos",
      description: "Caixa já recebido que cobre o valor desta ordem.",
      emptyDescription:
        "Nenhum aluguel recebido até agora. A comissão é liberada conforme os aluguéis entram, por isso a cobertura está em R$ 0,00 — não é ausência de valor contratado.",
      emptyBadge: "Nada a pagar ainda",
      emptyHint: "Nenhum aluguel recebido",
      emptyOutstandingHint: "Liberado a partir do 1º aluguel",
      completedDescription: "Cobertura concluída. A obrigação pode seguir para revisão e pagamento.",
      blockedPrefix: "Aprovação ainda bloqueada. Faltam ",
      blockedSuffix:
        " de caixa recebido. O sistema liberará esta ordem automaticamente para revisão quando os próximos aluguéis completarem a cobertura.",
      valueLabel: "Comissão contratual",
      coveredLabel: "Coberta por recebimentos",
      outstandingLabel: "Saldo para próximos meses",
      ariaLabel: "Cobertura da comissão pelos aluguéis",
    };
  }

  return {
    sectionTitle: "Cobertura",
    title: "Cobertura por caixa recebido",
    description: "Caixa já recebido que cobre o valor desta ordem.",
    emptyDescription:
      "Nenhum recebimento registrado até agora. A ordem será liberada quando o caixa vinculado à origem entrar.",
    emptyBadge: "Aguardando cobertura",
    emptyHint: "Nenhum recebimento",
    emptyOutstandingHint: "Quando o caixa entrar",
    completedDescription: "Cobertura concluída. A obrigação pode seguir para revisão e pagamento.",
    blockedPrefix: "Aprovação ainda bloqueada. Faltam ",
    blockedSuffix: " de cobertura. A ordem será liberada quando o caixa vinculado à origem entrar.",
    valueLabel: "Valor da ordem",
    coveredLabel: "Coberta por recebimentos",
    outstandingLabel: "Saldo a cobrir",
    ariaLabel: "Cobertura por caixa recebido",
  };
}

export function payoutCoveragePendingLabel(order: PayoutOrder, outstandingCents: number): string {
  if (outstandingCents <= 0) return "Ordem encerrada ou indisponível para ação em lote";
  const kind = payoutOriginKind(order);
  if (kind === "sale_commission") return "Ordem aguardando recebimento da venda";
  if (kind === "rental_commission") return "Ordem aguardando cobertura por aluguéis";
  return "Ordem aguardando cobertura";
}

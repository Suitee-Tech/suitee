import type { OwnerTransfer } from "../../services/transfer.service";
import { normalizeStatus } from "../lancamentos/status";

export type OwnerTransferDisplayStatus =
  | "Pendente"
  | "Aprovado"
  | "Processado"
  | "Cancelado"
  | "A apurar"
  /** Repasse sem valor a transferir, aguardando a conclusão do operador. */
  | "Sem saldo"
  /** Repasse encerrado sem movimentação de caixa (nada havia a repassar). */
  | "Liquidado sem saldo";

export type OwnerTransferAction = "approve" | "process" | "settle" | null;

/** Centavos inteiros do líquido do repasse (o backend compara em centavos). */
function netAmountCents(transfer: Pick<OwnerTransfer, "net_amount">): number {
  return Math.round(Number(transfer.net_amount) * 100);
}

/**
 * "Sem saldo a repassar": o repasse não tem valor a transferir e nenhuma
 * reserva do proprietário pode consumir o que sobrou. A reserva é o desempate
 * que importa — reserva positiva significa contas administradas ainda em
 * aberto, que podem consumir ou liberar o valor, então o repasse segue o fluxo
 * normal (A apurar / aguardando). Repasse legado ou manual sem snapshot de
 * saldo conta como sem saldo.
 */
export function hasNoPayableBalance(
  transfer: Pick<OwnerTransfer, "net_amount" | "owner_balance">,
): boolean {
  const expectedCents = netAmountCents(transfer);
  if (!Number.isFinite(expectedCents) || expectedCents !== 0) return false;
  return (transfer.owner_balance?.reserved_cents ?? 0) === 0;
}

/**
 * Repasse encerrado sem nada a repassar: o marcador persistido pelo backend ou,
 * em payloads antigos (deploy gradual), o próprio líquido zerado.
 */
export function hasSettledWithoutBalance(
  transfer: Pick<OwnerTransfer, "net_amount" | "settled_without_balance">,
): boolean {
  if (transfer.settled_without_balance === true) return true;
  const expectedCents = netAmountCents(transfer);
  return Number.isFinite(expectedCents) && expectedCents === 0;
}

export function getOwnerTransferOrigin(
  transfer: Pick<OwnerTransfer, "settlement_type" | "economic_breakdown" | "source_charge_id" | "source_transaction_id">,
  contractType?: string,
): { flow: "sale" | "rent" | null; label: string; source: string; automatic: boolean } {
  const candidates = [transfer.settlement_type, transfer.economic_breakdown?.flow, contractType];
  const flow = candidates.map((value) => value?.toLowerCase()).find((value) => value === "sale" || value === "rent");
  const chargeId = transfer.source_charge_id?.trim();
  const transactionId = transfer.source_transaction_id?.trim();
  return {
    flow: flow === "sale" || flow === "rent" ? flow : null,
    label: flow === "sale" ? "Venda" : flow === "rent" ? "Locação" : "Modalidade não informada",
    source: chargeId ? `Cobrança ${chargeId}` : transactionId ? `Recebimento ${transactionId}` : "Origem a apurar",
    automatic: Boolean(chargeId || transactionId),
  };
}

/**
 * O backend só liquida um repasse quando a obrigação possui origem canônica e
 * o saldo disponível do proprietário cobre exatamente o valor líquido. Um
 * repasse manual antigo pode existir na API, mas não deve parecer processável
 * na interface se essa composição não estiver presente.
 */
export function hasOwnerTransferSettlementCoverage(
  transfer: Pick<
    OwnerTransfer,
    "source_charge_id" | "source_transaction_id" | "owner_balance" | "net_amount" | "economic_breakdown"
  >,
): boolean {
  const hasCanonicalSource = Boolean(
    transfer.source_charge_id?.trim() || transfer.source_transaction_id?.trim(),
  );
  const expectedCents = Math.round(Number(transfer.net_amount) * 100);
  if (!Number.isFinite(expectedCents) || expectedCents <= 0) return false;

  // available_cents do owner_balance é derivado no backend (total − reservado
  // − bloqueado). Defensivo: se o payload veio sem ele (registros antigos),
  // calcula dos buckets; e, por fim, usa o available do breakdown econômico.
  const buckets = transfer.owner_balance;
  let availableCents: number | undefined;
  if (typeof buckets?.available_cents === "number") {
    availableCents = buckets.available_cents;
  } else if (typeof buckets?.total_cents === "number") {
    availableCents = Math.max(
      buckets.total_cents - (buckets.reserved_cents ?? 0) - (buckets.blocked_cents ?? 0),
      0,
    );
  } else {
    availableCents = transfer.economic_breakdown?.available_cents;
  }

  return (
    hasCanonicalSource &&
    typeof availableCents === "number" &&
    availableCents === expectedCents
  );
}

export function getOwnerTransferPresentation(
  transfer: Pick<
    OwnerTransfer,
    | "status"
    | "source_charge_id"
    | "source_transaction_id"
    | "owner_balance"
    | "net_amount"
    | "economic_breakdown"
    | "settled_without_balance"
  >,
): {
  status: OwnerTransferDisplayStatus;
  action: OwnerTransferAction;
  settlementReady: boolean;
} {
  const canonicalStatus = normalizeStatus("transfer", transfer.status);
  const settlementReady = hasOwnerTransferSettlementCoverage(transfer);

  if (canonicalStatus === "processado") {
    if (hasSettledWithoutBalance(transfer)) {
      return { status: "Liquidado sem saldo", action: null, settlementReady };
    }
    return { status: "Processado", action: null, settlementReady };
  }
  if (canonicalStatus === "cancelado" || canonicalStatus === "revertido") {
    return { status: "Cancelado", action: null, settlementReady };
  }
  // O backend aceita a conclusão sem saldo em PENDING e APPROVED sem exigir
  // comprovante; fora desses estados a ação não é oferecida.
  if (
    (canonicalStatus === "pendente" || canonicalStatus === "aprovado") &&
    hasNoPayableBalance(transfer)
  ) {
    return { status: "Sem saldo", action: "settle", settlementReady: false };
  }
  if (!settlementReady || !canonicalStatus) {
    return { status: "A apurar", action: null, settlementReady: false };
  }
  if (canonicalStatus === "pendente") {
    return { status: "Pendente", action: "approve", settlementReady: true };
  }
  if (canonicalStatus === "aprovado") {
    return { status: "Aprovado", action: "process", settlementReady: true };
  }

  return { status: "A apurar", action: null, settlementReady: false };
}

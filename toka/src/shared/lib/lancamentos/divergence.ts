import type { CommissionLancamento, Lancamento, RoyaltyLancamento, TransferLancamento } from "./types";

const DIVERGENCE_TOLERANCE_BRL = 0.01;

export type DivergenceReason = "amount_mismatch" | "missing_proof" | "missing_timestamp";

export interface DivergenceInfo {
  reason: DivergenceReason;
  delta?: number;
  expected?: number;
  received?: number;
}

export function hasDivergence(lancamento: Lancamento): boolean {
  return computeDivergence(lancamento) !== null;
}

export function computeDivergence(lancamento: Lancamento): DivergenceInfo | null {
  switch (lancamento.kind) {
    case "charge":
      return computeChargeDivergence(lancamento);
    case "commission":
      return computeCommissionDivergence(lancamento);
    case "transfer":
      return computeTransferDivergence(lancamento);
    case "royalty":
      return computeRoyaltyDivergence(lancamento);
    case "transaction":
      // Manual journal entries have no derived expectation to reconcile.
      return null;
  }
}

function computeChargeDivergence(lancamento: Extract<Lancamento, { kind: "charge" }>): DivergenceInfo | null {
  if (lancamento.status !== "pago") return null;
  const paid = lancamento.kind_data.paid_amount;
  if (paid === undefined || paid === null) return null;
  const expected = lancamento.amount;
  const delta = paid - expected;
  if (Math.abs(delta) <= DIVERGENCE_TOLERANCE_BRL) return null;
  return { reason: "amount_mismatch", delta, expected, received: paid };
}

function computeCommissionDivergence(lancamento: CommissionLancamento): DivergenceInfo | null {
  const { gross_amount, deductions_total, proof_url } = lancamento.kind_data;
  if (gross_amount !== undefined && lancamento.net_amount !== undefined && deductions_total !== undefined) {
    const expected = gross_amount - deductions_total;
    const delta = lancamento.net_amount - expected;
    if (Math.abs(delta) > DIVERGENCE_TOLERANCE_BRL) {
      return { reason: "amount_mismatch", delta, expected, received: lancamento.net_amount };
    }
  }

  if (lancamento.status === "pago" && !proof_url) {
    return { reason: "missing_proof" };
  }

  return null;
}

function computeTransferDivergence(lancamento: TransferLancamento): DivergenceInfo | null {
  const {
    gross_rent,
    admin_fee_amount,
    maintenance_deductions,
    other_deductions,
    processed_at,
    economic_breakdown,
  } = lancamento.kind_data;
  if (lancamento.net_amount !== undefined) {
    // Allocation-aware transfers already carry the authoritative amount that
    // may leave the owner's subledger. In SALE this excludes the complete
    // commission pool; in RENT it also excludes reserved managed bills. The
    // legacy arithmetic remains only for rows without an economic allocation.
    const expected = economic_breakdown
      ? economic_breakdown.available_cents / 100
      : gross_rent - admin_fee_amount - maintenance_deductions - other_deductions;
    const delta = lancamento.net_amount - expected;
    if (Math.abs(delta) > DIVERGENCE_TOLERANCE_BRL) {
      return { reason: "amount_mismatch", delta, expected, received: lancamento.net_amount };
    }
  }

  if (lancamento.status === "processado" && !processed_at) {
    return { reason: "missing_timestamp" };
  }

  return null;
}

function computeRoyaltyDivergence(lancamento: RoyaltyLancamento): DivergenceInfo | null {
  if (lancamento.status === "pago" && !lancamento.kind_data.proof_url) {
    return { reason: "missing_proof" };
  }
  return null;
}

export function describeDivergence(info: DivergenceInfo): string {
  switch (info.reason) {
    case "amount_mismatch": {
      const delta = info.delta ?? 0;
      const sign = delta > 0 ? "+" : "−";
      const formatter = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
      return `Divergência ${sign}${formatter.format(Math.abs(delta))}`;
    }
    case "missing_proof":
      return "Pago sem comprovante anexado";
    case "missing_timestamp":
      return "Status terminal sem timestamp";
  }
}

export function divergenceReasonLabel(info: DivergenceInfo): string {
  switch (info.reason) {
    case "amount_mismatch": {
      const delta = info.delta ?? 0;
      const sign = delta > 0 ? "+" : "−";
      const formatter = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
      return `Diferença ${sign}${formatter.format(Math.abs(delta))}`;
    }
    case "missing_proof":
      return "Sem comprovante";
    case "missing_timestamp":
      return "Sem data de liquidação";
  }
}

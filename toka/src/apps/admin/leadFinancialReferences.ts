import type { Contract } from "../../shared/contracts/types";
import type { EsteiraDeal } from "../../shared/deals/types";
import type { Charge } from "../../shared/services/charge.service";
import type { Commission } from "../../shared/services/commission.service";

const positiveFiniteNumber = (value: number | null | undefined): number | null =>
  typeof value === "number" && Number.isFinite(value) && value > 0 ? value : null;

/** Valor bruto canônico da cobrança, em reais, como exposto pela API financeira. */
export function resolveLeadChargeAmount(
  charge: Pick<Charge, "total_amount" | "amount">,
): number | null {
  return positiveFiniteNumber(charge.total_amount) ?? positiveFiniteNumber(charge.amount);
}

/**
 * Resolve o percentual efetivo da comissão pelo negócio exato do contrato.
 * O contrato guarda apenas um snapshot e registros antigos podem tê-lo vazio.
 */
export function resolveContractCommissionPct(
  contract: Pick<Contract, "id" | "deal_id" | "commission_pct" | "value">,
  deals: Array<Pick<EsteiraDeal, "id" | "commission">>,
  commissions: Array<Pick<Commission, "id" | "deal_id" | "contract_id" | "amount" | "gross_amount" | "status">> = [],
): number | null {
  const linkedDeal = contract.deal_id
    ? deals.find((deal) => deal.id === contract.deal_id)
    : undefined;

  const configuredPct = positiveFiniteNumber(linkedDeal?.commission)
    ?? positiveFiniteNumber(contract.commission_pct);
  if (configuredPct !== null) return configuredPct;

  const contractValue = positiveFiniteNumber(contract.value);
  if (contractValue === null) return null;

  const seen = new Set<string>();
  const generatedAmount = commissions.reduce((total, commission) => {
    const isLinked = commission.contract_id === contract.id
      || (contract.deal_id && commission.deal_id === contract.deal_id);
    const isActive = commission.status !== "CANCELLED" && commission.status !== "REVERSED";
    if (!isLinked || !isActive || seen.has(commission.id)) return total;
    seen.add(commission.id);
    return total + (positiveFiniteNumber(commission.gross_amount)
      ?? positiveFiniteNumber(commission.amount)
      ?? 0);
  }, 0);

  if (generatedAmount === 0) return null;
  return Math.round((generatedAmount / contractValue) * 10_000) / 100;
}

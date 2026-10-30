import type { Charge } from "../services/charge.service";

export interface ChargeCompositionItem {
  key: string;
  description: string;
  amountCents: number;
}

export interface RentalAdministrationSplit {
  grossRentCents: number;
  administrationCents: number;
  ownerRentCents: number;
  ownerItemIndex: number;
  administrationItemIndex: number;
}

// Recurring rental charges store economic destinations: the rental_base is
// already net of the generated revenue/admin_fee. Explicit bundled admin fees
// are additional charges and lack that revenue marker. Do not infer a deduction
// from a description or from the presence of an administration fee alone.
export function getChargeComposition(charge: Charge): {
  items: ChargeCompositionItem[];
  rentalSplit?: RentalAdministrationSplit;
} {
  const lineItems = charge.line_items ?? [];
  const items = lineItems.map((item, index) => ({
    key: item.id || `${item.source_id || "item"}-${index}`,
    description: item.description,
    amountCents: item.amount_cents,
  }));

  if (charge.type !== "RENT" || !charge.source_event_id?.startsWith("rental.") ||
      !charge.tenant_id || !charge.owner_id || charge.tenant_id === charge.owner_id) {
    return { items };
  }

  const ownerIndexes = lineItems.flatMap((item, index) =>
    item.source_type === "rental_base" && item.economic_owner_id === charge.owner_id &&
    item.settlement_mode === "arrecadacao" && item.amount_cents >= 0 ? [index] : []);
  const adminIndexes = lineItems.flatMap((item, index) =>
    item.source_type === "rental_bundled_item" && item.source_id === "admin_fee" &&
    item.allocation_type === "revenue" && item.economic_owner_id === charge.tenant_id &&
    item.settlement_mode === "arrecadacao" && item.amount_cents > 0 ? [index] : []);
  if (ownerIndexes.length !== 1 || adminIndexes.length !== 1) return { items };

  const ownerItemIndex = ownerIndexes[0];
  const administrationItemIndex = adminIndexes[0];
  const owner = lineItems[ownerItemIndex];
  const administration = lineItems[administrationItemIndex];
  if (!owner.rule_version_id || owner.rule_version_id !== administration.rule_version_id) return { items };

  const rentalSplit: RentalAdministrationSplit = {
    grossRentCents: owner.amount_cents + administration.amount_cents,
    administrationCents: administration.amount_cents,
    ownerRentCents: owner.amount_cents,
    ownerItemIndex,
    administrationItemIndex,
  };
  return {
    items: items.flatMap((item, index) => index === administrationItemIndex ? [] : [{
      ...item,
      ...(index === ownerItemIndex ? { description: "Aluguel", amountCents: rentalSplit.grossRentCents } : {}),
    }]),
    rentalSplit,
  };
}

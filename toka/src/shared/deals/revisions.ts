import type { DealDTO } from "../services/deal.service";

function revision(value: string): [number, number] | null {
  const millis = Date.parse(value);
  if (!Number.isFinite(millis)) return null;
  // Go timestamps retain nanoseconds; Date.parse alone truncates them.
  const fraction = value.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/)?.[1] ?? "";
  return [millis, Number(fraction.padEnd(9, "0").slice(0, 9)) % 1_000_000];
}

export function newestDeal(previous: DealDTO | undefined, next: DealDTO): DealDTO {
  if (!previous || previous.id !== next.id || previous.tenant_id !== next.tenant_id) return next;
  const before = revision(previous.updated_at);
  const after = revision(next.updated_at);
  if (!before || !after) return next;
  return before[0] > after[0] || (before[0] === after[0] && before[1] > after[1]) ? previous : next;
}

function isDeal(value: unknown): value is DealDTO {
  return typeof value === "object" && value !== null && "id" in value && typeof value.id === "string"
    && "updated_at" in value && typeof value.updated_at === "string"
    && "tenant_id" in value && typeof value.tenant_id === "string";
}

export function shareDealRevision(previous: unknown, next: unknown): unknown {
  return isDeal(previous) && isDeal(next) ? newestDeal(previous, next) : next;
}

export function shareDealsRevisions(previous: unknown, next: unknown): unknown {
  if (!Array.isArray(previous) || !Array.isArray(next)) return next;
  const byID = new Map(previous.filter(isDeal).map((deal) => [deal.id, deal]));
  // Preserve the server's membership/order; a missing item may have been deleted.
  return next.map((deal: unknown) => isDeal(deal) ? newestDeal(byID.get(deal.id), deal) : deal);
}

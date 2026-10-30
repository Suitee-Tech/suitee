import type { Lead } from "../../data";

/**
 * Merge the (possibly stale) in-memory owner leads with a fresh server fetch so
 * the most recently created captação leads always surface. Deduped by id with
 * the fetched copy winning on collisions, ordered most-recent-first by creation
 * date (leads without a parseable date sink to the bottom).
 */
export function mergeOwnerOptions(ownerLeads: Lead[], fetchedOwners: Lead[]): Lead[] {
  const byId = new Map<string, Lead>();
  for (const l of ownerLeads) byId.set(l.id, l);
  for (const l of fetchedOwners) byId.set(l.id, l);
  return Array.from(byId.values()).sort((a, b) => {
    const da = a.date ? Date.parse(a.date) : 0;
    const db = b.date ? Date.parse(b.date) : 0;
    return (Number.isNaN(db) ? 0 : db) - (Number.isNaN(da) ? 0 : da);
  });
}

/**
 * Guarantee the currently-selected owner is present in the option list so the
 * picker trigger never blanks on a preset/initial value that fell outside the
 * fetched page. Falls back to the full leads list to resolve it.
 */
export function ensureSelectedOwner(options: Lead[], selectedId: string | undefined, allLeads: Lead[]): Lead[] {
  if (!selectedId || options.some((l) => l.id === selectedId)) return options;
  const fromAll = allLeads.find((l) => l.id === selectedId);
  return fromAll ? [fromAll, ...options] : options;
}

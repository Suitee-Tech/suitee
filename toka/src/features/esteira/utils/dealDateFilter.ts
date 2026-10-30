export type DealDateFilter = "all" | "today" | "week";

/** Uses the browser's civil days, matching the displayed update date.
 * Last seven days includes today and the preceding six calendar days.
 */
export function matchesDealDate(updatedAt: string | undefined, filter: DealDateFilter, now = new Date()): boolean {
  if (filter === "all") return true;
  if (!updatedAt) return false;
  const date = new Date(updatedAt);
  if (!Number.isFinite(date.getTime())) return false;
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const end = new Date(start);
  end.setDate(end.getDate() + 1);
  if (filter === "week") start.setDate(start.getDate() - 6);
  return date >= start && date < end;
}

import { describe, it, expect } from "vitest";
import type { Lead } from "../../data";
import { mergeOwnerOptions, ensureSelectedOwner } from "./ownerOptions";

const lead = (id: string, over: Partial<Lead> = {}): Lead => ({
  id,
  name: `Lead ${id}`,
  email: `${id}@example.com`,
  phone: "",
  property: "",
  budget: 0,
  status: "Novo",
  date: "2026-01-01T00:00:00Z",
  role: "owner",
  leadType: "captacao",
  ...over,
});

describe("mergeOwnerOptions", () => {
  it("dedupes by id with the fetched copy winning on collisions", () => {
    const stale = [lead("a", { name: "Stale A", agentId: undefined })];
    const fresh = [lead("a", { name: "Fresh A", agentId: "broker-1" })];

    const result = mergeOwnerOptions(stale, fresh);

    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("Fresh A");
    expect(result[0].agentId).toBe("broker-1");
  });

  it("orders the newest captação leads first", () => {
    const ownerLeads = [
      lead("old", { date: "2026-01-01T00:00:00Z" }),
      lead("newest", { date: "2026-06-05T12:00:00Z" }),
    ];
    const fetched = [lead("mid", { date: "2026-03-15T00:00:00Z" })];

    const result = mergeOwnerOptions(ownerLeads, fetched);

    expect(result.map((l) => l.id)).toEqual(["newest", "mid", "old"]);
  });

  it("surfaces a freshly fetched owner absent from the stale context list", () => {
    const stale = [lead("known", { date: "2026-01-01T00:00:00Z" })];
    const fetched = [lead("brand-new", { date: "2026-06-05T09:00:00Z" })];

    const result = mergeOwnerOptions(stale, fetched);

    expect(result.map((l) => l.id)).toContain("brand-new");
    expect(result[0].id).toBe("brand-new"); // newest first
  });

  it("sinks leads without a parseable date to the bottom", () => {
    const result = mergeOwnerOptions(
      [lead("dated", { date: "2026-02-02T00:00:00Z" })],
      [lead("undated", { date: "" })],
    );

    expect(result.map((l) => l.id)).toEqual(["dated", "undated"]);
  });

  it("returns an empty list when there is nothing to merge", () => {
    expect(mergeOwnerOptions([], [])).toEqual([]);
  });
});

describe("ensureSelectedOwner", () => {
  const options = [lead("a"), lead("b")];

  it("returns the options unchanged when no owner is selected", () => {
    expect(ensureSelectedOwner(options, undefined, [])).toBe(options);
    expect(ensureSelectedOwner(options, "", [])).toBe(options);
  });

  it("returns the options unchanged when the selected owner is already present", () => {
    expect(ensureSelectedOwner(options, "b", [])).toBe(options);
  });

  it("prepends the selected owner resolved from the full leads list when missing", () => {
    const preset = lead("preset", { name: "Preset Owner" });
    const result = ensureSelectedOwner(options, "preset", [preset, ...options]);

    expect(result.map((l) => l.id)).toEqual(["preset", "a", "b"]);
    expect(result[0].name).toBe("Preset Owner");
  });

  it("leaves the options unchanged when the selected owner cannot be resolved anywhere", () => {
    expect(ensureSelectedOwner(options, "ghost", [])).toBe(options);
  });
});

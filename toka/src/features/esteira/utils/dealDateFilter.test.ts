import { describe, expect, it } from "vitest";
import { matchesDealDate } from "./dealDateFilter";

const now = new Date(2026, 8, 17, 12);
const stamp = (day: number, hour = 0, minute = 0) => new Date(2026, 8, day, hour, minute).toISOString();
describe("deal update date filters", () => {
  it("includes today's civil-day boundaries without yesterday or tomorrow", () => {
    expect(matchesDealDate(stamp(17), "today", now)).toBe(true);
    expect(matchesDealDate(stamp(17, 23, 59), "today", now)).toBe(true);
    expect(matchesDealDate(stamp(16, 23, 59), "today", now)).toBe(false);
    expect(matchesDealDate(stamp(18), "today", now)).toBe(false);
  });
  it("includes exactly seven civil days, including today", () => {
    expect(matchesDealDate(stamp(11), "week", now)).toBe(true);
    expect(matchesDealDate(stamp(17,23,59), "week", now)).toBe(true);
    expect(matchesDealDate(stamp(10,23,59), "week", now)).toBe(false);
    expect(matchesDealDate(stamp(18), "week", now)).toBe(false);
  });
  it("keeps missing/invalid timestamps out of date filters", () => {
    for (const value of [undefined, "invalid", "17/09/2026"]) {
      expect(matchesDealDate(value,"today",now)).toBe(false);
      expect(matchesDealDate(value,"week",now)).toBe(false);
      expect(matchesDealDate(value,"all",now)).toBe(true);
    }
  });
});

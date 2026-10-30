import { describe, expect, it } from "vitest";
import { formatBRL, formatLeadDate } from "./format";

describe("lib/leads/format", () => {
  describe("formatBRL", () => {
    it("formats integer values", () => {
      expect(formatBRL(1234567)).toMatch(/^R\$\s?1\.234\.567/);
    });

    it("treats null/undefined/NaN as zero", () => {
      expect(formatBRL(null)).toMatch(/^R\$\s?0/);
      expect(formatBRL(undefined)).toMatch(/^R\$\s?0/);
      expect(formatBRL(Number.NaN)).toMatch(/^R\$\s?0/);
    });
  });

  describe("formatLeadDate", () => {
    it("formats ISO date in numeric pt-BR by default", () => {
      // toLocaleDateString in pt-BR -> DD/MM/YYYY
      expect(formatLeadDate("2026-05-17T12:00:00Z")).toMatch(/\d{2}\/\d{2}\/2026/);
    });

    it("formats short variant with abbreviated month", () => {
      const out = formatLeadDate("2026-05-17T12:00:00Z", "short");
      expect(out).toMatch(/2026/);
    });

    it("returns em-dash for null/undefined", () => {
      expect(formatLeadDate(null)).toBe("—");
      expect(formatLeadDate(undefined)).toBe("—");
    });

    it("returns raw string for unparseable input", () => {
      expect(formatLeadDate("not-a-date")).toBe("not-a-date");
    });
  });
});

import { describe, expect, it } from "vitest";
import { getLeadTypeDetails, leadStatusLabel } from "./labels";

describe("lib/leads/labels", () => {
  describe("getLeadTypeDetails", () => {
    it("returns canonical entries for known types", () => {
      expect(getLeadTypeDetails("captacao").label).toBe("Captação");
      expect(getLeadTypeDetails("compra_venda").label).toBe("Compra e Venda");
      expect(getLeadTypeDetails("locacao").label).toBe("Locação");
    });

    it("falls back to Geral for unknown or empty", () => {
      expect(getLeadTypeDetails(undefined).label).toBe("Geral");
      expect(getLeadTypeDetails(null).label).toBe("Geral");
      expect(getLeadTypeDetails("bogus").label).toBe("Geral");
    });
  });

  describe("leadStatusLabel", () => {
    it("translates api enum to pt-BR display", () => {
      expect(leadStatusLabel("NEW")).toBe("Novo");
      expect(leadStatusLabel("CONVERTED")).toBe("Convertido");
      expect(leadStatusLabel("LOST")).toBe("Perdido");
    });

    it("returns em-dash for empty input", () => {
      expect(leadStatusLabel(undefined)).toBe("—");
      expect(leadStatusLabel(null)).toBe("—");
    });

    it("returns raw value for unknown status", () => {
      expect(leadStatusLabel("MYSTERY")).toBe("MYSTERY");
    });
  });
});

import { describe, expect, it } from "vitest";
import type { CommissionRule } from "../services/commissionRules.service";
import {
  commissionBasisForListing,
  formatCommissionPercentage,
  formatCommissionSplit,
  resolvePropertyCommissionSummary,
} from "./propertyCommission";

const settings = {
  default_commission_pct_sale: 6,
  default_commission_pct_rent: 100,
};

const rules = (
  values: Partial<Record<CommissionRule["role"], number>>,
): CommissionRule[] =>
  Object.entries(values).map(([role, defaultPercentage]) => ({
    tenant_id: "tenant-1",
    role: role as CommissionRule["role"],
    default_percentage: defaultPercentage ?? 0,
  }));

describe("property commission summary", () => {
  it("uses the tenant's sale default and role rules for a sale listing", () => {
    const summary = resolvePropertyCommissionSummary({
      listingType: "sale",
      settings: { ...settings, default_commission_pct_sale: 5 },
      rules: rules({ captador: 30, fechamento: 40, imobiliaria: 30 }),
    });

    expect(summary).toMatchObject({
      basis: "sale",
      totalPercentage: 5,
      agencyPercentage: 30,
      brokerPercentage: 70,
      hasDistribution: true,
      overallocated: false,
    });
  });

  it("uses the tenant's rent default (first rent) for a rent listing", () => {
    const summary = resolvePropertyCommissionSummary({
      listingType: "rent",
      settings: { ...settings, default_commission_pct_rent: 50 },
      rules: rules({ captador: 100 }),
    });

    expect(summary.basis).toBe("rent");
    expect(summary.totalPercentage).toBe(50);
    expect(summary.brokerPercentage).toBe(100);
    expect(summary.agencyPercentage).toBe(0);
  });

  it("treats an unknown listing type as sale", () => {
    expect(commissionBasisForListing(undefined)).toBe("sale");
    expect(commissionBasisForListing("exchange")).toBe("sale");
    expect(commissionBasisForListing("rent")).toBe("rent");
  });

  it("uses 'Disponível para' when only rent is offered, even if listing_type says sale", () => {
    // Regressão: imóvel marcado só como locação exibia "Padrão ... para venda".
    const summary = resolvePropertyCommissionSummary({
      listingType: "sale",
      availableFor: ["rent"],
      settings,
      rules: rules({ captador: 30, visitador: 20, fechamento: 50 }),
    });

    expect(summary.basis).toBe("rent");
    expect(summary.totalPercentage).toBe(100);
  });

  it("uses 'Disponível para' when only sale is offered, even if listing_type says rent", () => {
    expect(
      resolvePropertyCommissionSummary({
        listingType: "rent",
        availableFor: ["sale"],
        settings,
      }).basis,
    ).toBe("sale");
  });

  it("lets the main listing type decide when both sale and rent are offered", () => {
    expect(
      commissionBasisForListing("rent", ["sale", "rent", "exchange"]),
    ).toBe("rent");
    expect(commissionBasisForListing("sale", ["sale", "rent"])).toBe("sale");
    expect(commissionBasisForListing(undefined, ["sale", "rent"])).toBe("sale");
  });

  it("falls back to listing_type when 'Disponível para' is empty", () => {
    expect(commissionBasisForListing("rent", [])).toBe("rent");
    expect(commissionBasisForListing("rent", undefined)).toBe("rent");
    expect(commissionBasisForListing("sale", [])).toBe("sale");
  });

  it("has no distribution when the agency kept every role at zero", () => {
    const summary = resolvePropertyCommissionSummary({
      listingType: "sale",
      settings,
      rules: rules({ captador: 0, visitador: 0, fechamento: 0, imobiliaria: 0 }),
    });

    expect(summary.hasDistribution).toBe(false);
    expect(summary.totalPercentage).toBe(6);
    expect(formatCommissionSplit(summary)).toBeNull();
  });

  it("flags an overallocated distribution instead of hiding it", () => {
    const summary = resolvePropertyCommissionSummary({
      listingType: "sale",
      settings,
      rules: rules({ captador: 60, fechamento: 50 }),
    });

    expect(summary.overallocated).toBe(true);
    expect(summary.brokerPercentage).toBe(110);
    expect(formatCommissionSplit(summary)).toBeNull();
  });

  it("falls back to zero when settings are missing", () => {
    const summary = resolvePropertyCommissionSummary({ listingType: "sale" });

    expect(summary.totalPercentage).toBe(0);
    expect(summary.hasDistribution).toBe(false);
  });
});

describe("commission formatting", () => {
  it("formats percentages in pt-BR without noisy decimals", () => {
    expect(formatCommissionPercentage(40)).toBe("40");
    expect(formatCommissionPercentage(5.5)).toBe("5,5");
    expect(formatCommissionPercentage(3.3333)).toBe("3,33");
    expect(formatCommissionPercentage(Number.NaN)).toBe("0");
  });

  it("builds the split label and omits zeroed sides", () => {
    const base = { hasDistribution: true, overallocated: false };
    expect(
      formatCommissionSplit({ ...base, agencyPercentage: 40, brokerPercentage: 60 }),
    ).toBe("40% imobiliária / 60% corretor");
    expect(
      formatCommissionSplit({ ...base, agencyPercentage: 0, brokerPercentage: 100 }),
    ).toBe("100% corretor");
    expect(
      formatCommissionSplit({ ...base, agencyPercentage: 100, brokerPercentage: 0 }),
    ).toBe("100% imobiliária");
  });
});

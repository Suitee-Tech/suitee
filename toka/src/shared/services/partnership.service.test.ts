import { describe, it, expect } from "vitest";
import {
  grantFrom,
  canViewPortfolio,
  effectiveFinancial,
  type Partnership,
} from "./partnership.service";
const p: Partnership = {
  id: "agreement",
  version: 1,
  type: "partner",
  created_at: "2026-09-27T12:00:00Z",
  updated_at: "2026-09-27T12:00:00Z",
  financial_config: { royalty_percentage: 0, admin_fee: 0, transaction_fee: 0 },
  requester_id: "a",
  target_id: "b",
  status: "active",
  permissions: [{ scope: "shared_portfolio", access_level: "view" }],
  sharing_config: {
    share_owner_contact: true,
    allow_co_brokering: false,
    shared_property_types: [],
    shared_regions: [],
  },
};
describe("directional partnership access", () => {
  it("never falls back to legacy permissions when directional grants exist", () => {
    const directional = {
      ...p,
      grants: {
        a: {
          sharing_config: {
            share_owner_contact: true,
            allow_co_brokering: false,
            shared_property_types: [],
            shared_regions: [],
          },
          permissions: p.permissions,
        },
      },
    } as Partnership;
    expect(canViewPortfolio(directional, "a")).toBe(false);
    expect(canViewPortfolio(directional, "b")).toBe(true);
    expect(canViewPortfolio(directional, "third")).toBe(false);
  });
  it("normalizes empty backend grants and does not infer lead contact consent", () => {
    expect(
      grantFrom(
        {
          ...p,
          grants: { a: { sharing_config: {}, permissions: null } },
        } as unknown as Partnership,
        "a",
      ).permissions,
    ).toEqual([]);
    expect(grantFrom(p, "a").sharing_config.share_lead_contact).toBe(false);
  });
  it("revokes portfolio access for every non-active status", () => {
    for (const status of ["pending", "suspended", "inactive"] as const)
      expect(canViewPortfolio({ ...p, status }, "b")).toBe(false);
  });
  it("preserves legacy negotiated splits and supplies explicit defaults", () => {
    expect(
      effectiveFinancial({
        royalty_percentage: 0,
        admin_fee: 0,
        transaction_fee: 0,
        commission_split: "60/40",
        commission_model: { owner_company_pct: 0, closing_company_pct: 0 },
      }),
    ).toMatchObject({
      commission_model: { owner_company_pct: 60, closing_company_pct: 40 },
      royalty_base: "gross_commission",
    });
  });
});

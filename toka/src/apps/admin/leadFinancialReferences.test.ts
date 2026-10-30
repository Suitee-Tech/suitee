import { describe, expect, it } from "vitest";

import {
  resolveContractCommissionPct,
  resolveLeadChargeAmount,
} from "./leadFinancialReferences";

describe("lead financial references", () => {
  it("uses the charge total in reais without treating it as a money object or cents", () => {
    expect(resolveLeadChargeAmount({ total_amount: 4_128.5, amount: 4_000 })).toBe(4_128.5);
  });

  it("falls back to the base amount when a legacy charge has no usable total", () => {
    expect(resolveLeadChargeAmount({ total_amount: 0, amount: 3_200 })).toBe(3_200);
  });

  it("uses the exact linked deal commission before the stale contract snapshot", () => {
    expect(resolveContractCommissionPct(
      { id: "contract-1", deal_id: "deal-2", commission_pct: 0, value: 100_000 },
      [
        { id: "deal-1", commission: 5 },
        { id: "deal-2", commission: 8 },
      ],
    )).toBe(8);
  });

  it("returns an unknown reference when neither linked source has a commission", () => {
    expect(resolveContractCommissionPct(
      { id: "contract-1", deal_id: "deal-1", commission_pct: 0, value: 3_200 },
      [{ id: "deal-1", commission: 0 }],
    )).toBeNull();
  });

  it("derives the effective percentage from generated commission records", () => {
    expect(resolveContractCommissionPct(
      { id: "contract-1", deal_id: "deal-1", commission_pct: 0, value: 3_200 },
      [{ id: "deal-1", commission: 0 }],
      [{
        id: "commission-1",
        deal_id: "deal-1",
        contract_id: "contract-1",
        amount: 3_200,
        status: "PENDING",
      }],
    )).toBe(100);
  });

  it("sums active splits and ignores reversed commission records", () => {
    expect(resolveContractCommissionPct(
      { id: "contract-1", deal_id: "deal-1", commission_pct: 0, value: 100_000 },
      [{ id: "deal-1", commission: 0 }],
      [
        { id: "commission-1", deal_id: "deal-1", amount: 3_000, status: "APPROVED" },
        { id: "commission-2", deal_id: "deal-1", amount: 3_000, status: "PAID" },
        { id: "commission-3", deal_id: "deal-1", amount: 6_000, status: "REVERSED" },
      ],
    )).toBe(6);
  });
});

import { describe, expect, it } from "vitest";
import type { CommissionRule } from "../services/commissionRules.service";
import {
  buildCommissionRulesPayload,
  calculateCommissionDistribution,
  COMMISSION_ROLES,
} from "./commissionRules";

const rules = (
  values: Partial<Record<CommissionRule["role"], number>>,
): CommissionRule[] =>
  Object.entries(values).map(([role, defaultPercentage]) => ({
    tenant_id: "tenant-1",
    role: role as CommissionRule["role"],
    default_percentage: defaultPercentage ?? 0,
  }));

describe("commission rules form helpers", () => {
  it("calculates the agency remainder from the editable roles", () => {
    const distribution = calculateCommissionDistribution(
      rules({ captador: 30, fechamento: 50, imobiliaria: 99 }),
    );

    expect(distribution).toEqual({
      nonAgencyPercentage: 80,
      agencyPercentage: 20,
      totalPercentage: 100,
      overallocated: false,
    });
  });

  it("keeps a zero configuration consolidated without agency distribution", () => {
    expect(calculateCommissionDistribution([])).toEqual({
      nonAgencyPercentage: 0,
      agencyPercentage: 0,
      totalPercentage: 0,
      overallocated: false,
    });
  });

  it("marks an overallocated distribution without hiding the excess", () => {
    expect(
      calculateCommissionDistribution(rules({ captador: 60, fechamento: 50 })),
    ).toEqual({
      nonAgencyPercentage: 110,
      agencyPercentage: 0,
      totalPercentage: 110,
      overallocated: true,
    });
  });

  it("builds a complete replace-all payload with the calculated remainder", () => {
    const payload = buildCommissionRulesPayload(
      rules({ captador: 30, fechamento: 50 }),
    );

    expect(payload.map((rule) => rule.role)).toEqual(COMMISSION_ROLES);
    expect(payload).toContainEqual({ role: "imobiliaria", default_percentage: 20 });
    expect(payload).toContainEqual({ role: "visitador", default_percentage: 0 });
  });
});

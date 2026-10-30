import type { CommissionRole, CommissionRule } from "../services/commissionRules.service";

export const EDITABLE_COMMISSION_ROLES: CommissionRole[] = [
  "captador",
  "co_captador",
  "visitador",
  "atendente",
  "fechamento",
];

export const COMMISSION_ROLES: CommissionRole[] = [
  ...EDITABLE_COMMISSION_ROLES,
  "imobiliaria",
];

export interface CommissionDistribution {
  nonAgencyPercentage: number;
  agencyPercentage: number;
  totalPercentage: number;
  overallocated: boolean;
}

const roundPercentage = (value: number) => Math.round(value * 10_000) / 10_000;

export function commissionRulePercentage(
  rules: CommissionRule[],
  role: CommissionRole,
): number {
  return rules.find((rule) => rule.role === role)?.default_percentage ?? 0;
}

export function calculateCommissionDistribution(
  rules: CommissionRule[],
): CommissionDistribution {
  const nonAgencyPercentage = roundPercentage(
    EDITABLE_COMMISSION_ROLES.reduce(
      (sum, role) => sum + commissionRulePercentage(rules, role),
      0,
    ),
  );
  const overallocated = nonAgencyPercentage > 100;
  const agencyPercentage =
    nonAgencyPercentage > 0 && !overallocated
      ? roundPercentage(100 - nonAgencyPercentage)
      : 0;

  return {
    nonAgencyPercentage,
    agencyPercentage,
    totalPercentage: roundPercentage(nonAgencyPercentage + agencyPercentage),
    overallocated,
  };
}

export function buildCommissionRulesPayload(rules: CommissionRule[]) {
  const distribution = calculateCommissionDistribution(rules);
  return COMMISSION_ROLES.map((role) => ({
    role,
    default_percentage:
      role === "imobiliaria"
        ? distribution.agencyPercentage
        : commissionRulePercentage(rules, role),
  }));
}

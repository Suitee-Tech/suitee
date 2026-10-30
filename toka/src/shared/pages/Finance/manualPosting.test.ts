import { beforeEach, describe, expect, it } from "vitest";
import type { CreateTransactionBatchItem } from "../../services/transaction.service";
import {
  allocateCentsByPercentage,
  allocateManualCommissionCents,
  calculateManualCommissionDistribution,
  clearManualPostingAttempt,
  findManualPostingAttempt,
  isAgencyCommissionRole,
  isPayableBrokerCommissionRole,
  localPostingDate,
  manualPostingAttemptStorageKey,
  manualPostingIntentFingerprint,
  payableBrokerCommissionEntries,
  persistManualPostingAttempt,
  type ManualPostingAttempt,
} from "./manualPosting";

const items = (date: string): CreateTransactionBatchItem[] => [{
  contract_id: "contract-1",
  type: "receita",
  description: "Receita",
  value: 100.01,
  date,
}];

describe("manual posting durability", () => {
  beforeEach(() => localStorage.clear());

  it("distribui centavos com resto determinístico e conservação exata", () => {
    expect(allocateCentsByPercentage(10_001, [50, 50])).toEqual([5_001, 5_000]);
    expect(allocateCentsByPercentage(10_001, [33.33, 33.33, 33.34])).toEqual([3_333, 3_333, 3_335]);
    expect(allocateCentsByPercentage(10_001, [33.33, 33.33, 33.34]).reduce((a, b) => a + b, 0)).toBe(10_001);
  });

  it("completa abaixo de 100% com o restante implícito da imobiliária", () => {
    const payablePreviews = payableBrokerCommissionEntries([
      { role: "captador", broker_id: "broker-1", percentage: 30 },
      { role: "fechamento", broker_id: "broker-2", percentage: 50 },
      { role: "imobiliaria", broker_id: "", percentage: 20, retained: true },
    ]);
    expect(payablePreviews.map((preview) => preview.role)).toEqual([
      "captador",
      "fechamento",
    ]);

    expect(calculateManualCommissionDistribution(
      payablePreviews.map((preview) => preview.percentage),
    )).toEqual({
      brokerPercentage: 80,
      agencyPercentage: 20,
      totalPercentage: 100,
      invalid: false,
      overallocated: false,
    });

    const allocation = allocateManualCommissionCents(10_001, [30, 50]);
    expect(allocation).toEqual({
      brokerPoolCents: 8_001,
      brokerCents: [3_000, 5_001],
      agencyCents: 2_000,
    });
    expect(
      allocation.brokerCents.reduce((sum, cents) => sum + cents, 0) +
        allocation.agencyCents,
    ).toBe(10_001);

    expect(allocateManualCommissionCents(1, [80])).toEqual({
      brokerPoolCents: 1,
      brokerCents: [1],
      agencyCents: 0,
    });

    // Parity contract shared with the backend: the sum is rounded to four
    // decimal places before deriving the broker pool, while the internal
    // largest-remainder split still uses the original percentages.
    expect(allocateManualCommissionCents(3_000_000, [33.33336, 33.33336])).toEqual({
      brokerPoolCents: 2_000_001,
      brokerCents: [1_000_001, 1_000_000],
      agencyCents: 999_999,
    });
  });

  it("não trata a imobiliária como contraparte de corretor", () => {
    expect(isAgencyCommissionRole("imobiliaria")).toBe(true);
    expect(isAgencyCommissionRole("Imobiliária")).toBe(true);
    expect(isAgencyCommissionRole("captador")).toBe(false);
    expect(isPayableBrokerCommissionRole("captador")).toBe(true);
    expect(isPayableBrokerCommissionRole("Total 6.0%")).toBe(false);
    expect(isPayableBrokerCommissionRole("imobiliaria")).toBe(false);
  });

  it("mantém 100% dos corretores e rejeita sobrealocação", () => {
    expect(allocateManualCommissionCents(10_001, [50, 50])).toEqual({
      brokerPoolCents: 10_001,
      brokerCents: [5_001, 5_000],
      agencyCents: 0,
    });
    expect(calculateManualCommissionDistribution([60, 50])).toMatchObject({
      brokerPercentage: 110,
      agencyPercentage: 0,
      overallocated: true,
    });
    expect(() => allocateManualCommissionCents(10_001, [60, 50])).toThrow(
      "broker percentages cannot exceed 100",
    );
    expect(allocateManualCommissionCents(10_001, [50.005, 50])).toMatchObject({
      brokerPoolCents: 10_001,
      agencyCents: 0,
    });
    expect(calculateManualCommissionDistribution([49.995, 50])).toMatchObject({
      brokerPercentage: 99.995,
      agencyPercentage: 0.005,
      totalPercentage: 100,
    });
    expect(allocateManualCommissionCents(10_001, [49.995, 50])).toEqual({
      brokerPoolCents: 10_000,
      brokerCents: [5_000, 5_000],
      agencyCents: 1,
    });
  });

  it("mantém toda a comissão na imobiliária quando não há corretor pagável", () => {
    expect(allocateManualCommissionCents(5_880_000, [])).toEqual({
      brokerPoolCents: 0,
      brokerCents: [],
      agencyCents: 5_880_000,
    });
  });

  it("mantém fingerprint econômico e payload/data exatos através de reload", () => {
    const intentFingerprint = manualPostingIntentFingerprint(items("2026-08-13"), []);
    expect(manualPostingIntentFingerprint(items("2026-08-14"), [])).toBe(intentFingerprint);
    const attempt: ManualPostingAttempt = {
      intentFingerprint,
      operationID: "manual-posting:stable",
      createdAt: 1,
      items: items("2026-08-13"),
      commissions: [],
    };
    persistManualPostingAttempt("tenant-a", attempt);

    expect(findManualPostingAttempt("tenant-a", intentFingerprint)).toEqual(attempt);
    expect(findManualPostingAttempt("tenant-b", intentFingerprint)).toBeNull();
    expect(localStorage.getItem(manualPostingAttemptStorageKey("tenant-b"))).toBeNull();
  });

  it("inclui a retenção da imobiliária no fingerprint e no retry persistido", () => {
    const commissions = [{
      deal_id: "deal-1",
      contract_id: "contract-1",
      broker_id: "broker-1",
      role: "captador",
      percentage: 80,
      amount: 80,
    }];
    const retention = { percentage: 20, amount: 0 };
    const intentFingerprint = manualPostingIntentFingerprint(
      items("2026-08-13"),
      commissions,
      retention,
    );
    expect(manualPostingIntentFingerprint(items("2026-08-13"), commissions))
      .not.toBe(intentFingerprint);

    const attempt: ManualPostingAttempt = {
      intentFingerprint,
      operationID: "manual-posting:retained",
      createdAt: 1,
      items: items("2026-08-13"),
      commissions,
      commissionRetention: retention,
    };
    persistManualPostingAttempt("tenant-a", attempt);
    expect(findManualPostingAttempt("tenant-a", intentFingerprint)).toEqual(attempt);
  });

  it("vincula o fingerprint econômico ao negócio que origina o repasse", () => {
    const saleItems: CreateTransactionBatchItem[] = [{
      ...items("2026-08-13")[0],
      description: "Recebimento - Venda",
      flow: "sale",
      deal_id: "deal-1",
    }];
    const retention = { percentage: 100, amount: 58_800 };
    const first = manualPostingIntentFingerprint(saleItems, [], retention);

    expect(manualPostingIntentFingerprint(
      [{ ...saleItems[0], deal_id: "deal-2" }],
      [],
      retention,
    )).not.toBe(first);
  });

  it("inclui vencimento e plano de pagamentos no fingerprint do lançamento", () => {
    const base = items("2026-08-29")[0];
    const withProvenance = manualPostingIntentFingerprint(
      [{ ...base, due_date: "2026-09-07", payment_plan: "Entrada + 11x de R$ 8.333,33" }],
      [],
    );
    const withoutProvenance = manualPostingIntentFingerprint([base], []);

    expect(withProvenance).not.toBe(withoutProvenance);
    // Mesma proveniência → mesmo fingerprint (retry idempotente no cliente).
    expect(manualPostingIntentFingerprint(
      [{ ...base, due_date: "2026-09-07", payment_plan: "Entrada + 11x de R$ 8.333,33" }],
      [],
    )).toBe(withProvenance);
  });

  it("limpa somente a tentativa confirmada e preserva outras pendentes", () => {
    const firstFingerprint = manualPostingIntentFingerprint(items("2026-08-13"), []);
    const secondItems = [{ ...items("2026-08-13")[0], value: 200 }];
    const secondFingerprint = manualPostingIntentFingerprint(secondItems, []);
    persistManualPostingAttempt("tenant-a", {
      intentFingerprint: firstFingerprint,
      operationID: "manual-posting:first",
      createdAt: 1,
      items: items("2026-08-13"),
      commissions: [],
    });
    persistManualPostingAttempt("tenant-a", {
      intentFingerprint: secondFingerprint,
      operationID: "manual-posting:second",
      createdAt: 2,
      items: secondItems,
      commissions: [],
    });

    clearManualPostingAttempt("tenant-a", firstFingerprint);

    expect(findManualPostingAttempt("tenant-a", firstFingerprint)).toBeNull();
    expect(findManualPostingAttempt("tenant-a", secondFingerprint)?.operationID).toBe("manual-posting:second");
  });

  it("usa a data civil local, sem avançar pelo UTC", () => {
    expect(localPostingDate(new Date(2026, 7, 13, 23, 30))).toBe("2026-08-13");
  });

  it("falha fechado quando a tentativa persistida está corrompida", () => {
    localStorage.setItem(manualPostingAttemptStorageKey("tenant-a"), "{invalid");
    expect(() => findManualPostingAttempt("tenant-a", "intent")).toThrow(
      "stored manual posting attempts are invalid",
    );
    expect(localStorage.getItem(manualPostingAttemptStorageKey("tenant-a"))).toBe("{invalid");
  });
});

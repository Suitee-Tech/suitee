import { describe, expect, it } from "vitest";
import type { OwnerTransfer } from "../../services/transfer.service";
import {
  getOwnerTransferPresentation,
  hasNoPayableBalance,
  hasOwnerTransferSettlementCoverage,
  hasSettledWithoutBalance,
} from "./ownerTransferFlow";

const transfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer => ({
  id: "transfer-1",
  tenant_id: "tenant-1",
  source_charge_id: "charge-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-09",
  gross_rent: 1_000,
  admin_fee_percent: 10,
  admin_fee_amount: 100,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 900,
  status: "PENDING",
  owner_balance: {
    total_cents: 90_000,
    reserved_cents: 0,
    blocked_cents: 0,
    available_cents: 90_000,
  },
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-01T12:00:00Z",
  ...overrides,
});

describe("owner transfer financial flow", () => {
  it("separa aprovação e processamento para um repasse com cobertura", () => {
    expect(getOwnerTransferPresentation(transfer())).toMatchObject({
      status: "Pendente",
      action: "approve",
      settlementReady: true,
    });
    expect(getOwnerTransferPresentation(transfer({ status: "APPROVED" }))).toMatchObject({
      status: "Aprovado",
      action: "process",
      settlementReady: true,
    });
  });

  it("não oferece ação operacional a repasse manual sem composição canônica", () => {
    const manual = transfer({ source_charge_id: undefined, owner_balance: undefined });
    expect(hasOwnerTransferSettlementCoverage(manual)).toBe(false);
    expect(getOwnerTransferPresentation(manual)).toEqual({
      status: "A apurar",
      action: null,
      settlementReady: false,
    });
  });

  it("não aprova quando o saldo disponível diverge do valor líquido", () => {
    const divergent = transfer({
      owner_balance: {
        total_cents: 90_000,
        reserved_cents: 0,
        blocked_cents: 0,
        available_cents: 89_999,
      },
    });
    expect(getOwnerTransferPresentation(divergent).status).toBe("A apurar");
    expect(getOwnerTransferPresentation(divergent).action).toBeNull();
  });

  it("mantém estados terminais somente leitura", () => {
    expect(getOwnerTransferPresentation(transfer({ status: "PROCESSED" }))).toMatchObject({
      status: "Processado",
      action: null,
    });
    expect(getOwnerTransferPresentation(transfer({ status: "CANCELLED" }))).toMatchObject({
      status: "Cancelado",
      action: null,
    });
  });
});

describe("repasse sem saldo a repassar", () => {
  const zeroTransfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer =>
    transfer({
      net_amount: 0,
      owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
      ...overrides,
    });

  it("pendente zerado vira 'Sem saldo' com conclusão disponível", () => {
    const zero = zeroTransfer();
    expect(hasNoPayableBalance(zero)).toBe(true);
    expect(getOwnerTransferPresentation(zero)).toEqual({
      status: "Sem saldo",
      action: "settle",
      settlementReady: false,
    });
  });

  it("aprovado zerado também aceita a conclusão sem saldo", () => {
    expect(getOwnerTransferPresentation(zeroTransfer({ status: "APPROVED" }))).toEqual({
      status: "Sem saldo",
      action: "settle",
      settlementReady: false,
    });
  });

  it("zerado concluído aparece como liquidado sem saldo e sem ação", () => {
    const settled = zeroTransfer({ status: "PROCESSED", settled_without_balance: true });
    expect(hasSettledWithoutBalance(settled)).toBe(true);
    expect(getOwnerTransferPresentation(settled)).toEqual({
      status: "Liquidado sem saldo",
      action: null,
      settlementReady: false,
    });
  });

  it("payload antigo sem o marcador cai na heurística de líquido zerado", () => {
    const legacySettled = zeroTransfer({ status: "PROCESSED", processed_by: "system" });
    expect(hasSettledWithoutBalance(legacySettled)).toBe(true);
    expect(getOwnerTransferPresentation(legacySettled).status).toBe("Liquidado sem saldo");
    expect(getOwnerTransferPresentation(legacySettled).action).toBeNull();
  });

  it("não conclui sem saldo quando ainda há reserva de contas administradas", () => {
    const reserved = zeroTransfer({
      owner_balance: { total_cents: 0, reserved_cents: 5_000, blocked_cents: 0, available_cents: 0 },
    });
    expect(hasNoPayableBalance(reserved)).toBe(false);
    expect(getOwnerTransferPresentation(reserved)).toEqual({
      status: "A apurar",
      action: null,
      settlementReady: false,
    });
  });

  it("repasse legado/manual zerado sem snapshot de saldo conta como sem saldo", () => {
    const manual = transfer({ net_amount: 0, source_charge_id: undefined, owner_balance: undefined });
    expect(hasNoPayableBalance(manual)).toBe(true);
    expect(getOwnerTransferPresentation(manual)).toMatchObject({ status: "Sem saldo", action: "settle" });
  });

  it("estado terminal continua cancelado, mesmo zerado", () => {
    expect(getOwnerTransferPresentation(zeroTransfer({ status: "CANCELLED" }))).toMatchObject({
      status: "Cancelado",
      action: null,
    });
  });

  it("repasse com líquido a repassar mantém aprovação e processamento", () => {
    const normal = transfer();
    expect(hasNoPayableBalance(normal)).toBe(false);
    expect(hasSettledWithoutBalance(normal)).toBe(false);
    expect(getOwnerTransferPresentation(normal)).toMatchObject({ status: "Pendente", action: "approve" });
    expect(getOwnerTransferPresentation(transfer({ status: "APPROVED" }))).toMatchObject({
      status: "Aprovado",
      action: "process",
    });
  });

  it("compara o líquido em centavos", () => {
    expect(hasNoPayableBalance(transfer({ net_amount: 0.01, owner_balance: undefined }))).toBe(false);
    expect(hasNoPayableBalance(transfer({ net_amount: 0.001, owner_balance: undefined }))).toBe(true);
  });

  it("respeita o marcador explícito do backend mesmo com líquido positivo", () => {
    expect(hasSettledWithoutBalance(transfer({ settled_without_balance: true }))).toBe(true);
    expect(getOwnerTransferPresentation(transfer({ status: "PROCESSED", settled_without_balance: true })).status).toBe(
      "Liquidado sem saldo",
    );
  });
});

describe("owner transfer settlement coverage (payloads legados)", () => {
  it("calcula available a partir dos buckets quando available_cents não veio", () => {
    const legacy = transfer({
      owner_balance: { total_cents: 90_000, reserved_cents: 10_000, blocked_cents: 0 } as OwnerTransfer["owner_balance"],
    });
    // 90.000 − 10.000 = 80.000 ≠ net 900*100 → ainda A apurar (cobertura
    // exata exige igualdade).
    expect(hasOwnerTransferSettlementCoverage(legacy)).toBe(false);
    const covered = transfer({
      net_amount: 800,
      owner_balance: { total_cents: 90_000, reserved_cents: 10_000, blocked_cents: 0 } as OwnerTransfer["owner_balance"],
    });
    expect(getOwnerTransferPresentation(covered).status).toBe("Pendente");
  });

  it("usa o available do breakdown econômico como último fallback", () => {
    const fallback = transfer({
      owner_balance: undefined,
      economic_breakdown: {
        flow: "sale",
        gross_cents: 100_000,
        owner_payable_cents: 90_000,
        reserved_cents: 0,
        available_cents: 90_000,
        agency_revenue_cents: 10_000,
        commission_payable_cents: 0,
        partner_payable_cents: 0,
        agency_retained_cents: 10_000,
        discount_cents: 0,
        deductions_cents: 0,
      },
    });
    expect(getOwnerTransferPresentation(fallback).status).toBe("Pendente");
  });
});

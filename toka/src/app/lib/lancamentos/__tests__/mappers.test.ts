import { describe, expect, it } from "vitest";

import type { Charge } from "../../../services/charge.service";
import type { Commission } from "../../../services/commission.service";
import type { OwnerTransfer } from "../../../services/transfer.service";
import type { PayoutOrder } from "../../../services/payoutOrder.service";
import type { TransactionDTO } from "../../../services/transaction.service";

import {
  chargeToLancamento,
  commissionToLancamento,
  mapLancamentos,
  payoutOrderToRoyaltyLancamento,
  transactionToLancamento,
  transferToLancamento,
} from "../mappers";

const baseCharge: Charge = {
  id: "ch-1",
  tenant_id: "t-1",
  contract_id: "co-1",
  tenant_party_id: "p-1",
  owner_id: "o-1",
  property_id: "pr-1",
  type: "RENT",
  description: "Aluguel Maio/2026",
  reference_month: "2026-05",
  amount: 2000,
  late_fee: 0,
  interest: 0,
  discount: 0,
  total_amount: 2000,
  due_date: "2026-05-10",
  status: "PENDING",
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-02T00:00:00Z",
};

describe("chargeToLancamento", () => {
  it("maps PENDING into a pendente lancamento with common fields", () => {
    const l = chargeToLancamento(baseCharge);
    expect(l.kind).toBe("charge");
    expect(l.status).toBe("pendente");
    expect(l.amount).toBe(2000);
    expect(l.currency).toBe("BRL");
    expect(l.due_at).toBe("2026-05-10");
    expect(l.origin.entity_kind).toBe("contract");
    expect(l.origin.entity_id).toBe("co-1");
    expect(l.counterparty.kind).toBe("tenant");
    expect(l.counterparty.id).toBe("p-1");
  });

  it("prefers total_amount but falls back to amount", () => {
    const noTotal = { ...baseCharge, total_amount: undefined as unknown as number };
    expect(chargeToLancamento(noTotal).amount).toBe(2000);
  });

  it("preserves the raw charge in source for narrow-and-edit flows", () => {
    const l = chargeToLancamento(baseCharge);
    expect(l.source).toBe(baseCharge);
  });

  it("counts reminders sent", () => {
    const c: Charge = {
      ...baseCharge,
      reminders_sent: [
        { sent_at: "x", channels: ["email"] },
        { sent_at: "y", channels: ["whatsapp"] },
      ],
    };
    expect(chargeToLancamento(c).kind_data.reminders_count).toBe(2);
  });

  it("flags counterparty as desconhecido when tenant_party_id is missing", () => {
    const c: Charge = {
      ...baseCharge,
      tenant_party_id: "" as unknown as string,
    };
    expect(chargeToLancamento(c).counterparty.kind).toBe("desconhecido");
  });

  it("does not flag divergence on a pending charge", () => {
    expect(
      chargeToLancamento(baseCharge).timeline_summary.divergence_flag,
    ).toBe(false);
  });

  it("flags divergence when a paid charge has paid_amount != total_amount", () => {
    const overpaid: Charge = {
      ...baseCharge,
      status: "PAID",
      paid_amount: 2050,
      paid_at: "2026-05-12T00:00:00Z",
    };
    const l = chargeToLancamento(overpaid);
    expect(l.status).toBe("pago");
    expect(l.timeline_summary.divergence_flag).toBe(true);
  });

  it("does not flag divergence on an exact match", () => {
    const exact: Charge = {
      ...baseCharge,
      status: "PAID",
      paid_amount: 2000,
      paid_at: "2026-05-12T00:00:00Z",
    };
    expect(
      chargeToLancamento(exact).timeline_summary.divergence_flag,
    ).toBe(false);
  });
});

const baseCommission: Commission = {
  id: "co-1",
  deal_id: "d-1",
  broker_id: "b-1",
  role: "fechamento",
  percentage: 50,
  amount: 500,
  gross_amount: 600,
  net_amount: 480,
  currency: "BRL",
  status: "APPROVED",
  tenant_id: "t-1",
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-03T00:00:00Z",
  deductions: [
    { kind: "IRRF", amount: 75 },
    { kind: "INSS", amount: 45 },
  ],
};

describe("commissionToLancamento", () => {
  it("maps APPROVED to aprovado and sums deductions", () => {
    const l = commissionToLancamento(baseCommission);
    expect(l.status).toBe("aprovado");
    expect(l.kind_data.deductions_total).toBe(120);
    expect(l.kind_data.gross_amount).toBe(600);
    expect(l.net_amount).toBe(480);
    expect(l.counterparty.kind).toBe("broker");
    expect(l.counterparty.id).toBe("b-1");
    expect(l.origin.entity_kind).toBe("deal");
  });

  it("defaults split_origin to desconhecido in Phase 1", () => {
    const l = commissionToLancamento(baseCommission);
    expect(l.kind_data.split_origin).toBe("desconhecido");
  });

  it("zeros deductions_total when array is missing", () => {
    const c = { ...baseCommission, deductions: undefined };
    expect(commissionToLancamento(c).kind_data.deductions_total).toBe(0);
  });

  it("exposes rental funding progress without changing the contractual amount", () => {
    const l = commissionToLancamento({
      ...baseCommission,
      status: "PENDING",
      amount: 5000,
      funding_required_cents: 500000,
      funded_amount_cents: 225000,
    });
    expect(l.amount).toBe(5000);
    expect(l.kind_data.funding_required_cents).toBe(500000);
    expect(l.kind_data.funded_amount_cents).toBe(225000);
    expect(l.kind_data.funding_outstanding_cents).toBe(275000);
  });
});

const baseTransfer: OwnerTransfer = {
  id: "tr-1",
  tenant_id: "t-1",
  contract_id: "co-1",
  owner_id: "o-1",
  property_id: "pr-1",
  reference_month: "2026-05",
  gross_rent: 3000,
  admin_fee_percent: 10,
  admin_fee_amount: 300,
  maintenance_deductions: 150,
  other_deductions: 50,
  net_amount: 2500,
  status: "APPROVED",
  approved_at: "2026-05-10T12:00:00Z",
  created_at: "2026-05-09T00:00:00Z",
  updated_at: "2026-05-10T12:00:00Z",
  history: [
    { at: "2026-05-09T00:00:00Z", action: "created" },
    { at: "2026-05-10T12:00:00Z", action: "approved" },
  ],
};

describe("transferToLancamento", () => {
  it("maps APPROVED and preserves deductions in kind_data", () => {
    const l = transferToLancamento(baseTransfer);
    expect(l.status).toBe("aprovado");
    expect(l.amount).toBe(2500);
    expect(l.net_amount).toBe(2500);
    expect(l.kind_data.admin_fee_amount).toBe(300);
    expect(l.kind_data.maintenance_deductions).toBe(150);
    expect(l.kind_data.other_deductions).toBe(50);
    expect(l.kind_data.history_count).toBe(2);
    expect(l.counterparty.kind).toBe("owner");
  });

  it("labels last event from history (Aprovado)", () => {
    expect(transferToLancamento(baseTransfer).timeline_summary.last_event_label).toBe(
      "Aprovado",
    );
  });

  it("identifies a sale settlement and preserves its economic waterfall", () => {
    const sale = transferToLancamento({
      ...baseTransfer,
      id: "sale-transfer",
      settlement_type: "sale",
      gross_rent: 980000,
      net_amount: 921200,
      economic_breakdown: {
        flow: "sale",
        gross_cents: 98000000,
        owner_payable_cents: 92120000,
        reserved_cents: 0,
        available_cents: 92120000,
        agency_revenue_cents: 2352000,
        commission_payable_cents: 3528000,
        partner_payable_cents: 0,
        agency_retained_cents: 2352000,
        discount_cents: 0,
        deductions_cents: 0,
      },
    });

    expect(sale.origin.label).toBe("Venda · 2026-05");
    expect(sale.amount).toBe(921200);
    expect(sale.net_amount).toBe(921200);
    expect(sale.kind_data.settlement_type).toBe("sale");
    expect(sale.kind_data.economic_breakdown?.owner_payable_cents).toBe(92120000);
    expect(sale.kind_data.economic_breakdown?.agency_retained_cents).toBe(2352000);
  });
});

const basePayoutOrder: PayoutOrder = {
  id: "po-1",
  tenant_id: "t-1",
  source_type: "royalty_settlement",
  source_id: "ps-1",
  recipient_type: "royalty_payee",
  recipient_id: "rp-1",
  recipient_label: "Franqueador X",
  amount_cents: 75000,
  currency: "BRL",
  status: "APPROVED",
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-04T00:00:00Z",
};

describe("payoutOrderToRoyaltyLancamento", () => {
 it.each([["admin_fee", "Taxa administrativa da parceria"], ["transaction_fee", "Taxa por negócio da parceria"]])("identifica %s separadamente de royalties", (kind,label) => {
 const l=payoutOrderToRoyaltyLancamento({...basePayoutOrder, source_ref:{obligation_kind:kind,deal_id:"deal-origin"}, contract_id:"contract"});
 expect(l?.origin.label).toBe(label);
 expect(l?.kind_data.source_deal_id).toBe("deal-origin");
 expect(l?.kind_data.obligation_kind).toBe(kind);
 });
  it("maps royalty_settlement payouts to royalty lancamentos", () => {
    const l = payoutOrderToRoyaltyLancamento(basePayoutOrder);
    expect(l).not.toBeNull();
    expect(l!.kind).toBe("royalty");
    expect(l!.status).toBe("aprovado");
    expect(l!.amount).toBe(750);
    expect(l!.counterparty.kind).toBe("partner");
    expect(l!.counterparty.label).toBe("Franqueador X");
    expect(l!.permissions_required).toContain("partnerships:read");
  });

  it("returns null for non-royalty payouts (broker/manual)", () => {
    const broker: PayoutOrder = { ...basePayoutOrder, source_type: "manual" };
    expect(payoutOrderToRoyaltyLancamento(broker)).toBeNull();
  });
});

describe("transactionToLancamento", () => {
  it("preserves the sale waterfall that explains the receipt destination", () => {
    const transaction: TransactionDTO = {
      id: "receipt-sale",
      tenant_id: "tenant-sale",
      contract_id: "contract-sale",
      deal_id: "deal-sale",
      type: "receita",
      description: "Recebimento - Venda",
      value: 980000,
      date: "2026-08-29T00:00:00Z",
      status: "PENDING",
      created_at: "2026-08-29T00:00:00Z",
      updated_at: "2026-08-29T00:00:00Z",
      economic_breakdown: {
        flow: "sale",
        gross_cents: 98000000,
        commission_pool_cents: 5880000,
        commission_payable_cents: 3528000,
        partner_payable_cents: 0,
        agency_retained_cents: 2352000,
        owner_payable_cents: 92120000,
        discount_cents: 0,
      },
    };

    const lancamento = transactionToLancamento(transaction);
    expect(lancamento.kind_data.deal_id).toBe("deal-sale");
    expect(lancamento.kind_data.economic_breakdown?.gross_cents).toBe(98000000);
    expect(lancamento.kind_data.economic_breakdown?.commission_payable_cents).toBe(3528000);
    expect(lancamento.kind_data.economic_breakdown?.agency_retained_cents).toBe(2352000);
    expect(lancamento.kind_data.economic_breakdown?.owner_payable_cents).toBe(92120000);
  });
});

describe("mapLancamentos", () => {
  it("merges all four kinds and filters non-royalty payouts", () => {
    const out = mapLancamentos({
      charges: [baseCharge],
      commissions: [baseCommission],
      transfers: [baseTransfer],
      payouts: [basePayoutOrder, { ...basePayoutOrder, id: "po-2", source_type: "manual" }],
    });
    expect(out).toHaveLength(4);
    const kinds = out.map((l) => l.kind).sort();
    expect(kinds).toEqual(["charge", "commission", "royalty", "transfer"]);
  });
});

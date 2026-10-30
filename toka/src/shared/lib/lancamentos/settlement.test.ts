import { describe, expect, it } from "vitest";

import type { TransferLancamento, TransactionLancamento } from "./types";
import { deriveSettlementEffects } from "./settlement";

const transfer = {
  id: "tr1",
  kind: "transfer",
  status: "aprovado",
  amount: 5000,
  currency: "BRL",
  counterparty: { kind: "owner", id: "o1", label: "Proprietária" },
  origin: { entity_kind: "contract", entity_id: "c1", label: "Contrato" },
  timeline_summary: {},
  presentation: {},
  permissions_required: [],
  created_at: "",
  updated_at: "",
  kind_data: {
    admin_fee_amount: 400,
    admin_fee_percent: 8,
    maintenance_deductions: 0,
    other_deductions: 0,
    gross_rent: 5000,
    history_count: 0,
    economic_breakdown: {
      flow: "rent",
      gross_cents: 500000,
      owner_payable_cents: 460000,
      reserved_cents: 100000,
      available_cents: 360000,
      agency_revenue_cents: 40000,
      admin_fee_cents: 40000,
      intermediation_revenue_cents: 0,
      commission_funded_cents: 0,
      commission_payable_cents: 0,
      partner_payable_cents: 0,
      agency_retained_cents: 40000,
      discount_cents: 0,
      deductions_cents: 0,
    },
  },
  source: {},
} as unknown as TransferLancamento;

const transaction = {
  id: "tx1",
  kind: "transaction",
  status: "pendente",
  amount: 5000,
  currency: "BRL",
  counterparty: { kind: "tenant", id: "t1", label: "Locatário" },
  origin: { entity_kind: "deal", entity_id: "d1", label: "Negócio" },
  timeline_summary: {},
  presentation: {},
  permissions_required: [],
  created_at: "",
  updated_at: "",
  kind_data: {
    transaction_type: "receita",
    direction: "entrada",
    manual: false,
    economic_breakdown: {
      flow: "sale",
      gross_cents: 500000,
      commission_pool_cents: 100000,
      commission_payable_cents: 30000,
      partner_payable_cents: 0,
      agency_retained_cents: 40000,
      owner_payable_cents: 430000,
      discount_cents: 0,
    },
  },
  source: {},
} as unknown as TransactionLancamento;

describe("deriveSettlementEffects", () => {
  it("approve de repasse não movimenta valor e explica a etapa", () => {
    const data = deriveSettlementEffects("approve", transfer);
    expect(data.title).toBe("Aprovar repasse");
    expect(data.impactNote).toContain("não afeta o caixa");
    expect(data.accounting[0]).toContain("Nenhum valor é movimentado");
    expect(data.effects.some((e) => e.label === "Repasse líquido ao proprietário" && e.value.includes("3.600,00"))).toBe(true);
  });

  it("processar repasse descreve a saída de caixa", () => {
    const data = deriveSettlementEffects("process", transfer);
    expect(data.title).toBe("Processar repasse");
    expect(data.accounting.some((l) => l.includes("Débito: Obrigação ao proprietário"))).toBe(true);
    expect(data.accounting.some((l) => l.includes("Crédito: Caixa"))).toBe(true);
    expect(data.impactNote).toContain("dinheiro sai");
  });

  it("baixa de transação de venda mostra destinos + efeito contábil", () => {
    const data = deriveSettlementEffects("settle", transaction);
    expect(data.title).toBe("Dar baixa manual");
    expect(data.effects.some((e) => e.label === "Valor recebido" && e.value.includes("5.000,00"))).toBe(true);
    expect(data.effects.some((e) => e.label === "Obrigação ao proprietário" && e.value.includes("4.300,00"))).toBe(true);
    expect(data.effects.some((e) => e.label === "Receita bruta de intermediação" && e.value.includes("700,00"))).toBe(true);
    expect(data.effects.some((e) => e.label === "Saldo retido pela imobiliária" && e.value.includes("400,00"))).toBe(true);
    expect(data.accounting.some((l) => l.includes("Débito: Despesa de comissão"))).toBe(true);
    expect(data.accounting.some((l) => l.includes("Débito: Caixa"))).toBe(true);
    expect(data.impactNote).toContain("baixa + repasse atômicos");
  });
});

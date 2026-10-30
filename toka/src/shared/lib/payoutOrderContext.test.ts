import { describe, expect, it } from "vitest";

import type { PayoutOrder } from "../services/payoutOrder.service";
import {
  payoutCoverageCopy,
  payoutCoveragePendingLabel,
  payoutOriginKind,
  payoutOriginLabel,
  payoutStatusLabel,
} from "./payoutOrderContext";

const order = (overrides: Partial<PayoutOrder> = {}): PayoutOrder => ({
  id: "payout-1",
  tenant_id: "tenant-1",
  source_type: "manual",
  recipient_type: "broker",
  recipient_label: "Corretora Alfa",
  amount_cents: 120_000_00,
  funding_required_cents: 120_000_00,
  funded_amount_cents: 0,
  currency: "BRL",
  status: "DRAFT",
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  ...overrides,
});

describe("contexto da ordem de pagamento", () => {
  it("reconhece comissão de venda pelo negócio sem competência de aluguel", () => {
    const sale = order({ source_ref: { deal_id: "deal-1", split_pct: 50 } });
    expect(payoutOriginKind(sale)).toBe("sale_commission");
    expect(payoutOriginLabel(sale)).toBe("Comissão de venda");
    expect(payoutStatusLabel(sale)).toBe("Aguardando recebimento da venda");
    expect(payoutCoverageCopy(sale).title).toBe("Cobertura pelo recebimento da venda");
    expect(payoutCoverageCopy(sale).blockedSuffix).toMatch(/recebimento da venda/);
  });

  it("mantém a copy de aluguéis para comissão de locação", () => {
    const rental = order({ source_type: "charge", source_ref: { reference_month: "2026-10" } });
    expect(payoutOriginKind(rental)).toBe("rental_commission");
    expect(payoutStatusLabel(rental)).toBe("Aguardando aluguéis");
    expect(payoutCoverageCopy(rental).title).toBe("Cobertura pelos aluguéis recebidos");
    expect(payoutCoverageCopy(rental).blockedSuffix).toMatch(/aluguéis completarem/);
  });

  it("não chama de aluguel uma origem ambígua", () => {
    const unknown = order();
    expect(payoutOriginKind(unknown)).toBe("manual");
    expect(payoutStatusLabel(unknown)).toBe("Aguardando cobertura");
    expect(payoutCoverageCopy(unknown).blockedSuffix).not.toMatch(/aluguéis/);
  });

  it("distingue royalty e usa a pendência certa no checkbox", () => {
    const royalty = order({ source_type: "royalty_settlement" });
    expect(payoutOriginKind(royalty)).toBe("royalty");
    expect(payoutStatusLabel(royalty)).toBe("Aguardando liquidação do royalty");
    expect(payoutCoveragePendingLabel(royalty, 100)).toBe("Ordem aguardando cobertura");
    expect(payoutCoveragePendingLabel(order({ source_ref: { deal_id: "deal-1" } }), 100)).toBe(
      "Ordem aguardando recebimento da venda",
    );
    expect(payoutCoveragePendingLabel(order({ source_ref: { deal_id: "deal-1" } }), 0)).toBe(
      "Ordem encerrada ou indisponível para ação em lote",
    );
  });
});

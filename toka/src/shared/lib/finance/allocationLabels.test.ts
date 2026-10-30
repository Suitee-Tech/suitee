import { describe, expect, it } from "vitest";
import { ALLOCATION_TYPE_LABEL, allocationTypeLabel } from "./allocationLabels";

describe("allocationTypeLabel", () => {
  it("traduz os códigos de classificação econômica para pt-BR", () => {
    expect(ALLOCATION_TYPE_LABEL.accounts_receivable).toBe("Contas a receber");
    expect(ALLOCATION_TYPE_LABEL.owner_payable).toBe("Repasse ao proprietário");
    expect(ALLOCATION_TYPE_LABEL.commission_payable).toBe("Comissão a pagar");
    expect(ALLOCATION_TYPE_LABEL.revenue).toBe("Receita da imobiliária");
    expect(ALLOCATION_TYPE_LABEL.partner_payable).toBe("Repasse ao parceiro");
    expect(ALLOCATION_TYPE_LABEL.royalty_payable).toBe("Royalty a pagar");
    expect(ALLOCATION_TYPE_LABEL.tax_withheld).toBe("Tributo retido");
  });

  it("allocationTypeLabel devolve o rótulo traduzido", () => {
    expect(allocationTypeLabel("accounts_receivable")).toBe("Contas a receber");
    expect(allocationTypeLabel("revenue")).toBe("Receita da imobiliária");
  });

  it("allocationTypeLabel faz fallback para o próprio código quando desconhecido", () => {
    expect(allocationTypeLabel("codigo_desconhecido")).toBe("codigo_desconhecido");
  });
});

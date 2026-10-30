import { describe, expect, it } from "vitest";
import { formatCurrencyBRL0 } from "./format";
import { describeListingPrice, listingBusinessLabel } from "./listingPrice";

// Revisão de acesso a imóveis (2026-09-19): cada tela descrevia o valor do seu
// jeito — uma usava `sale_price`, outra `sale_price || rent_price`, uma
// formatava com centavos e outra sem, e nenhuma dizia se o número era venda ou
// aluguel. Este contrato é o que passou a valer em todas elas.
describe("describeListingPrice", () => {
  it("descreve venda sem periodicidade e sem centavos", () => {
    const valor = describeListingPrice(450000, "sale");
    expect(valor.hasPrice).toBe(true);
    expect(valor.label).toBe("Venda");
    expect(valor.period).toBe("");
    // Mesmo formatador de todas as telas: sem centavos na vitrine.
    expect(valor.amount).toBe(formatCurrencyBRL0(450000));
    expect(valor.amount).not.toContain(",00");
  });

  it("descreve aluguel como mensal", () => {
    const valor = describeListingPrice(2500, "rent");
    expect(valor.hasPrice).toBe(true);
    expect(valor.label).toBe("Aluguel");
    expect(valor.period).toBe("/mês");
    expect(valor.amount).toBe(formatCurrencyBRL0(2500));
  });

  it("valor ausente ou zero é 'Preço sob consulta', nunca R$ 0 nem o valor da outra finalidade", () => {
    for (const price of [0, null, undefined, Number.NaN]) {
      expect(describeListingPrice(price as number, "sale")).toEqual({
        hasPrice: false,
        amount: "Preço sob consulta",
        label: "",
        period: "",
      });
    }
  });

  it("rótulo da finalidade não depende da tela", () => {
    expect(listingBusinessLabel("rent")).toBe("Aluguel");
    expect(listingBusinessLabel("RENT")).toBe("Aluguel");
    expect(listingBusinessLabel("sale")).toBe("Venda");
    expect(listingBusinessLabel(undefined)).toBe("Venda");
  });
});

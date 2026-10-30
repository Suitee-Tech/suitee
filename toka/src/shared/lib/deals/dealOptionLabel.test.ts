import { describe, expect, it } from "vitest";

import { dealOptionLabel, dealStatusLabel, dealTypeLabel } from "./dealOptionLabel";

/** O formatador de moeda usa espaço não separável; a comparação normaliza. */
const text = (value: string) => value.replace(/\u00a0/g, " ");

// O dropdown de negócio mostrava `display_id || id` — sem display_id (o caso
// comum) o operador via um UUID. Estes testes travam o rótulo semântico.
describe("dealOptionLabel", () => {
  it("identifica o negócio pelo imóvel, cliente, tipo, valor e etapa", () => {
    const label = dealOptionLabel({
      deal: {
        id: "3fb8df23-d5b7-4c5c-a0ac-d0fe3459de87",
        type: "SALE",
        status: "CONTRACT",
        value: 1_400_000,
      },
      propertyTitle: "Casa Vitória Régia",
      customerName: "João Neto",
    });

    expect(label.title).toBe("Casa Vitória Régia");
    expect(text(label.detail)).toBe("Venda · João Neto · R$ 1.400.000 · Contrato");
    expect(text(label.full)).toBe("Casa Vitória Régia · Venda · João Neto · R$ 1.400.000 · Contrato");
    expect(label.full).not.toMatch(/3fb8df23/);
  });

  it("usa as partes do próprio negócio quando o painel não passa os nomes", () => {
    const label = dealOptionLabel({
      deal: {
        id: "deal-1",
        type: "RENT",
        status: "NEGOTIATION",
        value: 4_200,
        parties: [
          { role: "owner", name: "Karina Motta" },
          { role: "tenant", name: "Vanessa Melo" },
        ],
      },
    });

    expect(label.title).toBe("Vanessa Melo");
    expect(text(label.detail)).toBe("Locação · R$ 4.200 · Em Negociação");
  });

  it("busca por imóvel, cliente e valor (sem acentos)", () => {
    const label = dealOptionLabel({
      deal: { id: "deal-1", type: "SALE", status: "PROPOSAL", value: 900_000 },
      propertyTitle: "Casa Vitória Régia",
      customerName: "João Neto",
    });

    expect(label.searchText).toContain("casa vitoria regia");
    expect(label.searchText).toContain("joao neto");
    expect(label.searchText).toContain("r$ 900.000");
  });

  it("cai num código curto legível quando não há nada semântico", () => {
    const label = dealOptionLabel({
      deal: { id: "3fb8df23-d5b7-4c5c-a0ac-d0fe3459de87", type: null, status: null },
    });

    expect(label.title).toBe("#3fb8df23");
    expect(label.detail).toBe("Negócio");
    expect(label.full).toBe("#3fb8df23 · Negócio");
  });

  it("prefere o display_id quando existir e não repete o cliente já usado no título", () => {
    const label = dealOptionLabel({
      deal: { id: "deal-2", display_id: "NEG-0042", type: "SALE", status: "CONVERTED", value: 0 },
      customerName: "Cliente Único",
    });

    expect(text(label.full)).toBe("Cliente Único · Venda · Concluído");
    expect(label.searchText).toContain("neg-0042");
  });
});

describe("rótulos auxiliares", () => {
  it("traduz tipo e etapa", () => {
    expect(dealTypeLabel("SALE")).toBe("Venda");
    expect(dealTypeLabel("RENT")).toBe("Locação");
    expect(dealTypeLabel("")).toBe("Negócio");
    expect(dealStatusLabel("CONTRACT")).toBe("Contrato");
    expect(dealStatusLabel("NEGOTIATION")).toBe("Em Negociação");
    expect(dealStatusLabel("DESCONHECIDO")).toBe("DESCONHECIDO");
  });
});

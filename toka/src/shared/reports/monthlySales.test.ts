import { describe, expect, it } from "vitest";
import { buildMonthlySalesData, parseDealDate, resolveDealDate } from "./monthlySales";

// Regressão: os gráficos "Vendas & Locações Mensais" e "Valor por Tipo de
// Transação" apareciam VAZIOS em /relatorios → Vendas & Locações. O `date` da
// esteira é a data exibida no formato pt-BR ("30/09/2026") e o gráfico fazia
// `new Date(deal.date)`: o parser do JS não entende dd/mm/aaaa, devolve
// Invalid Date, `getMonth()` vira NaN e todo negócio caía num bucket que não
// existe na série (meses Jan..Dez) — zero em todas as barras.
describe("parseDealDate", () => {
  it("interpreta a data pt-BR exibida na esteira", () => {
    const parsed = parseDealDate("30/09/2026");
    expect(parsed).not.toBeNull();
    expect(parsed!.getMonth()).toBe(8); // Setembro
    expect(parsed!.getDate()).toBe(30);
    expect(parsed!.getFullYear()).toBe(2026);
  });

  it("interpreta ISO e aaaa-mm-dd", () => {
    expect(parseDealDate("2026-09-30T12:00:00Z")!.getTime()).toBe(Date.parse("2026-09-30T12:00:00Z"));
    const dateOnly = parseDealDate("2026-09-30");
    expect(dateOnly!.getMonth()).toBe(8);
    expect(dateOnly!.getDate()).toBe(30);
  });

  it("devolve null para entrada inútil (sem virar mês inexistente)", () => {
    for (const value of [null, undefined, "", "   ", "sem data", {}, 42]) {
      expect(parseDealDate(value as unknown)).toBeNull();
    }
  });

  it("prefere a data confiável (updated_at) à de exibição", () => {
    expect(resolveDealDate({ updated_at: "2026-03-05T12:00:00Z", date: "30/09/2026" })!.getMonth()).toBe(2);
    expect(resolveDealDate({ date: "30/09/2026" })!.getMonth()).toBe(8);
  });
});

describe("buildMonthlySalesData", () => {
  it("agrupa por mês e separa vendas de locações", () => {
    const data = buildMonthlySalesData([
      { type: "Venda", value: 1_400_000, date: "30/09/2026" },
      { type: "Locação", value: 2_500, date: "05/09/2026" },
      { type: "Locação", value: 3_000, date: "10/01/2026" },
    ]);

    expect(data).toHaveLength(12);
    const setembro = data[8];
    expect(setembro.month).toBe("Set");
    expect(setembro.vendas).toBe(1);
    expect(setembro.valorVendas).toBe(1_400_000);
    expect(setembro.locacoes).toBe(1);
    expect(setembro.valorLocacoes).toBe(2_500);

    const janeiro = data[0];
    expect(janeiro.locacoes).toBe(1);
    expect(janeiro.valorLocacoes).toBe(3_000);
    expect(janeiro.vendas).toBe(0);
  });

  it("aceita o tipo cru da API (SALE/RENT) e valores ausentes", () => {
    // Meio-dia UTC de propósito: o agrupamento é por mês LOCAL (o mesmo mês que
    // a data exibida na esteira), então um timestamp à meia-noite UTC pode cair
    // no mês anterior em fuso negativo.
    const data = buildMonthlySalesData([
      { type: "SALE", updated_at: "2026-02-15T12:00:00Z" },
      { type: "RENT", value: 1_000, updated_at: "2026-02-16T12:00:00Z" },
    ]);
    expect(data[1].vendas).toBe(1);
    expect(data[1].valorVendas).toBe(0);
    expect(data[1].locacoes).toBe(1);
    expect(data[1].valorLocacoes).toBe(1_000);
  });

  it("ignora negócio sem data interpretável sem contaminar os totais", () => {
    const data = buildMonthlySalesData([
      { type: "Venda", value: 10, date: "sem data" },
      { type: "Venda", value: 20, date: "30/09/2026" },
    ]);
    expect(data.reduce((acc, point) => acc + point.vendas, 0)).toBe(1);
    expect(data.reduce((acc, point) => acc + point.valorVendas, 0)).toBe(20);
  });

  it("devolve os 12 meses mesmo sem negócio nenhum", () => {
    const data = buildMonthlySalesData([]);
    expect(data.map((point) => point.month)).toEqual([
      "Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez",
    ]);
    expect(data.every((point) => point.vendas === 0 && point.locacoes === 0)).toBe(true);
  });

  it("tolera lista nula", () => {
    expect(buildMonthlySalesData(null)).toHaveLength(12);
  });
});

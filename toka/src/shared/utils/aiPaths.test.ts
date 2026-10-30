import { describe, expect, it } from "vitest";

import { normalizeAiPath } from "./aiPaths";

describe("normalizeAiPath", () => {
  it("mapeia rotas legadas das ferramentas para as rotas canônicas", () => {
    expect(normalizeAiPath("/properties/5b0f05c5-1b50-4ddd-871a-d882d8f6c3a1")).toBe(
      "/imoveis/5b0f05c5-1b50-4ddd-871a-d882d8f6c3a1",
    );
    expect(normalizeAiPath("/transfers/abc?tab=1")).toBe("/financeiro/repasses?tab=1");
    expect(normalizeAiPath("/deals/abc")).toBe("/esteira/deals/abc");
    expect(normalizeAiPath("/contracts/abc")).toBe("/contratos/abc");
    expect(normalizeAiPath("/finance/charges")).toBe("/financeiro/cobrancas");
    expect(normalizeAiPath("/messages")).toBe("/mensagens");
  });

  it("normaliza URLs internas da Suitee e preserva links externos", () => {
    expect(normalizeAiPath("https://app.suitee.com.br/properties/abc")).toBe("/imoveis/abc");
    expect(normalizeAiPath("https://app.suitee.com.br/transfers/abc")).toBe("/financeiro/repasses");
    expect(normalizeAiPath("https://example.com/properties/abc")).toBe("https://example.com/properties/abc");
  });
});

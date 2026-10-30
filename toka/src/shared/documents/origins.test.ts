import { describe, expect, it } from "vitest";
import { DOCUMENT_ORIGINS, getDocumentOriginLabel, originEntityKey, resolveDocumentOrigin } from "./origins";

describe("resolveDocumentOrigin", () => {
  it("mapeia cada entity_type para a origem operacional correspondente", () => {
    expect(resolveDocumentOrigin({ entityType: "contract" })).toBe("contract");
    expect(resolveDocumentOrigin({ entityType: "deal" })).toBe("deal");
    expect(resolveDocumentOrigin({ entityType: "property" })).toBe("property");
    expect(resolveDocumentOrigin({ entityType: "lead" })).toBe("client");
    expect(resolveDocumentOrigin({ entityType: "inspection" })).toBe("inspection");
    expect(resolveDocumentOrigin({ entityType: "checklist" })).toBe("checklist");
    expect(resolveDocumentOrigin({ entityType: "analysis" })).toBe("analysis");
    expect(resolveDocumentOrigin({ entityType: "fiscal" })).toBe("fiscal");
    expect(resolveDocumentOrigin({ entityType: "maintenance" })).toBe("maintenance");
  });

  it("tolera variações de caixa e plural gravadas por outros serviços", () => {
    expect(resolveDocumentOrigin({ entityType: "Contract" })).toBe("contract");
    expect(resolveDocumentOrigin({ entityType: " DEAL " })).toBe("deal");
    expect(resolveDocumentOrigin({ entityType: "ticket" })).toBe("maintenance");
  });

  it("usa o contrato quando o vínculo genérico não diz nada", () => {
    expect(resolveDocumentOrigin({ entityType: "general", contractId: "c-1" })).toBe("contract");
    expect(resolveDocumentOrigin({ contractId: "c-1" })).toBe("contract");
  });

  it("cai para o serviço de origem quando não há vínculo", () => {
    expect(resolveDocumentOrigin({ source: "toca-financial" })).toBe("financial");
    expect(resolveDocumentOrigin({ source: "toca-ops" })).toBe("maintenance");
    expect(resolveDocumentOrigin({ source: "portal" })).toBe("client");
    expect(resolveDocumentOrigin({ source: "toca-deal" })).toBe("deal");
  });

  it("nunca inventa vínculo: sem evidência o documento fica sem vínculo", () => {
    expect(resolveDocumentOrigin({})).toBe("unassigned");
    expect(resolveDocumentOrigin({ entityType: "general", source: "manual" })).toBe("unassigned");
    expect(resolveDocumentOrigin({ entityType: "tipo-desconhecido" })).toBe("unassigned");
  });

  it("o vínculo explícito tem prioridade sobre o serviço de origem", () => {
    expect(resolveDocumentOrigin({ entityType: "inspection", source: "toca-ops" })).toBe("inspection");
    expect(resolveDocumentOrigin({ entityType: "analysis", contractId: "c-9", source: "toca-deal" })).toBe("analysis");
  });
});

describe("catálogo de origens", () => {
  it("tem rótulo e descrição em pt-BR para toda origem", () => {
    for (const origin of DOCUMENT_ORIGINS) {
      expect(origin.label).not.toMatch(/^documents\./);
      expect(origin.label.length).toBeGreaterThan(2);
      expect(origin.description).not.toMatch(/^documents\./);
    }
  });

  it("rotula a origem sem vínculo de forma explícita", () => {
    expect(getDocumentOriginLabel("unassigned")).toBe("Sem vínculo");
  });

  it("gera chave estável por registro e uma chave dedicada para ausência de vínculo", () => {
    expect(originEntityKey("contract", "abc")).toBe("contract:abc");
    expect(originEntityKey("contract", "")).toBe("contract:__none__");
    expect(originEntityKey("contract", null)).toBe("contract:__none__");
  });
});

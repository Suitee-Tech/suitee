import { describe, expect, it } from "vitest";
import type { EsteiraDeal } from "../../../shared/deals/types";
import { asciiKeyPart, buildCreateIntentKey, buildEsteiraDealFormDefaults } from "./EsteiraDealModal";

describe("buildEsteiraDealFormDefaults", () => {
  it("rehydrates an edit form from the latest deal snapshot", () => {
    const deal: EsteiraDeal = {
      id: "deal-1",
      client: "Cliente antigo",
      clientId: "lead-1",
      property: "Imóvel antigo",
      propertyId: "property-1",
      type: "Locação",
      value: 4200,
      stage: "Contrato",
      date: "Hoje",
      avatarUrl: "",
    };

    expect(buildEsteiraDealFormDefaults({ dealToEdit: deal })).toEqual({
      client: "lead-1",
      property: "property-1",
      type: "Locação",
      value: 4200,
      stage: "Contrato",
      avatarUrl: "",
    });
  });

  it("uses resolved ids when a create form is reopened", () => {
    expect(
      buildEsteiraDealFormDefaults({
        initialLeadId: "lead-2",
        resolvedInitialPropertyId: "property-2",
      }),
    ).toMatchObject({
      client: "lead-2",
      property: "property-2",
      type: "Venda",
      stage: "Em Negociação",
    });
  });
});

describe("buildCreateIntentKey", () => {
  const data = {
    client: "João da Criação",
    property: "Apartamento Vista Criação",
    type: "Venda" as const,
    value: 350000.5,
    stage: "Em Negociação" as const,
    avatarUrl: "",
  };

  it("gera chave ASCII determinística a partir de strings de exibição", () => {
    const key = buildCreateIntentKey("tenant-1", data);
    expect(key).toBe("deal-create:tenant-1:ApartamentoVistaCriacao:JoaodaCriacao:Venda:350000.50");
    // Determinístico para o mesmo formulário (idempotência de intenção).
    expect(buildCreateIntentKey("tenant-1", data)).toBe(key);
  });

  it("asciiKeyPart remove diacríticos e símbolos", () => {
    expect(asciiKeyPart("Criação & Cia.")).toBe("CriacaoCia.");
    expect(asciiKeyPart("")).toBe("");
    expect(asciiKeyPart("a-b_c:d")).toBe("a-b_c:d");
  });
});

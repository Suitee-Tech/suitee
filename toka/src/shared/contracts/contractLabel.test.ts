import { describe, expect, it } from "vitest";
import { contractLabel } from "./contractLabel";
describe("contractLabel", () => {
  it("mostra referência, cliente e imóvel", () => {
    expect(contractLabel({ display_id: "CTR-42", client: "Ana", property_title: "Casa Centro", start_date: "2026-10-01" })).toBe("CTR-42 · Ana · Casa Centro");
  });
  it("usa data legível quando não há referência e resolve nomes do cadastro", () => {
    expect(contractLabel({ start_date: "2026-10-01T00:00:00Z" }, "Bia", "Apartamento 301")).toBe("Contrato de 01/10/2026 · Bia · Apartamento 301");
  });
});

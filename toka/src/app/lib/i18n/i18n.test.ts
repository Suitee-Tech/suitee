import { describe, expect, it, vi } from "vitest";
import { t, DICT_PT_BR } from "./index";

describe("t()", () => {
  it("returns the dictionary value for a known key", () => {
    expect(t("documents.status.assinado")).toBe("Assinado");
    expect(t("documents.category.identidade")).toBe("Documento pessoal");
  });

  it("returns the key itself for an unknown key (caller stays alive in prod)", () => {
    const spy = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      expect(t("definitely.not.in.dict")).toBe("definitely.not.in.dict");
    } finally {
      spy.mockRestore();
    }
  });

  it("interpolates {var} placeholders", () => {
    // Add a temporary key by reusing an existing one with placeholder syntax —
    // the production dictionary doesn't ship vars-using strings yet (Phase
    // 4/C7 scope) but the helper has to support them so callers wiring
    // dynamic copy don't hit a wall.
    DICT_PT_BR["test.greeting"] = "Olá, {name}!";
    try {
      expect(t("test.greeting", { name: "Maria" })).toBe("Olá, Maria!");
    } finally {
      delete DICT_PT_BR["test.greeting"];
    }
  });

  it("leaves placeholders intact when the variable is missing", () => {
    DICT_PT_BR["test.greeting"] = "Olá, {name}!";
    try {
      expect(t("test.greeting", {})).toBe("Olá, {name}!");
    } finally {
      delete DICT_PT_BR["test.greeting"];
    }
  });
});

describe("DICT_PT_BR", () => {
  it("has labels for every canonical document status and category", () => {
    const statuses = [
      "rascunho",
      "pendente",
      "pendente_assinatura",
      "parcialmente_assinado",
      "assinado",
      "expirado",
      "cancelado",
    ];
    for (const s of statuses) {
      expect(DICT_PT_BR[`documents.status.${s}`]).toBeTruthy();
    }
    const categories = [
      "contrato",
      "aditivo",
      "laudo",
      "vistoria",
      "procuracao",
      "recibo",
      "notificacao",
      "financeiro",
      "juridico",
      "fiscal",
      "identidade",
      "comprovante",
      "foto",
      "outros",
    ];
    for (const c of categories) {
      expect(DICT_PT_BR[`documents.category.${c}`]).toBeTruthy();
    }
  });
});

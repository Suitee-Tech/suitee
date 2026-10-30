import { describe, expect, it } from "vitest";
import { MAX_TAG_ITEMS, MAX_TAG_RUNES, normalizeTag, normalizeTags, splitTagDraft, tagListError } from "./tags";

// Espelho de `pkg/validation/tags.go`: se um lado mudar sem o outro, o
// formulário passa a aceitar o que a API recusa.
describe("normalizeTag", () => {
  it("limpa espaços e caracteres de controle", () => {
    expect(normalizeTag("  Piscina  ")).toBe("Piscina");
    expect(normalizeTag("Salão   de   Festas")).toBe("Salão de Festas");
    expect(normalizeTag("Playground\ncoberto")).toBe("Playground coberto");
    expect(normalizeTag("   ")).toBe("");
  });
});

describe("normalizeTags", () => {
  it("remove duplicatas sem diferenciar maiúsculas e preserva a ordem", () => {
    expect(normalizeTags(["Piscina", " piscina ", "", "Academia", "ACADEMIA"])).toEqual(["Piscina", "Academia"]);
  });
});

describe("splitTagDraft", () => {
  it("quebra por vírgula, ponto e vírgula e quebra de linha", () => {
    expect(splitTagDraft("Piscina, Academia")).toEqual(["Piscina", "Academia"]);
    expect(splitTagDraft("Piscina;Academia")).toEqual(["Piscina", "Academia"]);
    expect(splitTagDraft("Piscina\nAcademia")).toEqual(["Piscina", "Academia"]);
    expect(splitTagDraft("Piscina,")).toEqual(["Piscina"]);
    expect(splitTagDraft("  ,  , ")).toEqual([]);
  });
});

describe("tagListError", () => {
  it("aceita lista normal", () => {
    expect(tagListError(["Piscina", "Academia"])).toBeNull();
  });

  it("recusa acima do teto de itens, contando distintos", () => {
    const tags = Array.from({ length: MAX_TAG_ITEMS + 1 }, (_, i) => `Item ${i}`);
    expect(tagListError(tags)).toContain("máximo de");
    expect(tagListError(new Array(MAX_TAG_ITEMS).fill("Piscina"))).toBeNull();
  });

  it("recusa item acima do teto de caracteres", () => {
    expect(tagListError(["a".repeat(MAX_TAG_RUNES + 1)])).toContain("até");
  });
});

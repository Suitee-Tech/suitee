import { describe, expect, it } from "vitest";

import { isHexColor, normalizeHexColor, readableTextColor } from "./hexColor";

describe("normalizeHexColor", () => {
  it("normaliza caixa e espaços para o formato do seletor de cores", () => {
    expect(normalizeHexColor("  #1C905B ")).toBe("#1c905b");
    expect(normalizeHexColor("1c905b")).toBe("#1c905b");
  });

  it("expande a forma curta", () => {
    expect(normalizeHexColor("#abc")).toBe("#aabbcc");
  });

  it("recusa o que o tema não conseguiria aplicar", () => {
    for (const invalid of ["", "#", "#12", "#12345", "#1234567", "rgb(1,2,3)", "#gggggg", "verde", null, undefined]) {
      expect(normalizeHexColor(invalid as string)).toBeNull();
      expect(isHexColor(invalid as string)).toBe(false);
    }
  });

  it("aceita o que é cor de verdade", () => {
    expect(isHexColor("#FFF")).toBe(true);
    expect(isHexColor("#1C905B")).toBe(true);
  });
});

describe("readableTextColor", () => {
  it("usa texto escuro sobre cor clara e branco sobre cor escura", () => {
    expect(readableTextColor("#D1FAE5")).toBe("#1E293B");
    expect(readableTextColor("#FFFFFF")).toBe("#1E293B");
    expect(readableTextColor("#0d1b2a")).toBe("#FFFFFF");
    expect(readableTextColor("#064E3B")).toBe("#FFFFFF");
  });

  it("normaliza a forma curta antes de decidir", () => {
    expect(readableTextColor("#fff")).toBe("#1E293B");
    expect(readableTextColor("#000")).toBe("#FFFFFF");
  });

  it("cai no escuro quando o valor não é cor", () => {
    expect(readableTextColor("")).toBe("#1E293B");
    expect(readableTextColor("#12")).toBe("#1E293B");
  });
});

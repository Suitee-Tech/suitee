import { describe, expect, it } from "vitest";

import {
  AA_TEXT,
  DARK_SURFACE,
  LIGHT_SURFACE,
  contrastRatio,
  hexToHSL,
  pickPrimaryTokens,
  readableForeground,
} from "./themeContrast";

// A escolha de cor do white-label era feita por `lightness > 60%`, que não mede
// contraste: a marca padrão (#1C905B) produzia botão branco-sobre-verde em
// 3.99:1 (abaixo de AA) e texto de destaque em 3.82–4.27:1 no modo noturno.
//
// As marcas abaixo cobrem os extremos: a padrão da plataforma, a escura que a
// Imobiliária Exemplo usa de verdade, uma clara (amarelo) e o cinza médio — o
// pior caso, em que NENHUM dos dois textos passa sobre a cor original.

const BRANDS = {
  "padrão da plataforma": "#1C905B",
  "escura (tenant real)": "#013529",
  "clara (amarelo)": "#FFD700",
  "cinza médio (pior caso)": "#7A7A7A",
  azul: "#2563EB",
  roxo: "#7C3AED",
  vermelho: "#DC2626",
};

describe("contrastRatio", () => {
  it("mede os extremos conhecidos", () => {
    expect(Math.round(contrastRatio("#ffffff", "#000000"))).toBe(21);
    expect(contrastRatio("#ffffff", "#ffffff")).toBeCloseTo(1, 5);
  });

  it("bate com o caso que originou a correção", () => {
    // Branco sobre a marca padrão: 3.99:1 — reprovado em AA.
    expect(contrastRatio("#ffffff", "#1C905B")).toBeGreaterThan(3.9);
    expect(contrastRatio("#ffffff", "#1C905B")).toBeLessThan(4.5);
  });
});

describe("readableForeground", () => {
  it("escolhe o lado de maior contraste, não o limiar de lightness", () => {
    // Sobre a marca padrão o quase preto (4.21) é melhor que o branco (3.99),
    // embora a lightness da cor (34%) esteja abaixo do limiar antigo de 60%.
    const onBrand = readableForeground("#1C905B");
    expect(onBrand.color).toBe("0 0% 11%");
    expect(onBrand.ratio).toBeGreaterThan(contrastRatio("#ffffff", "#1C905B"));
  });

  it("prefere branco em fundo escuro", () => {
    expect(readableForeground("#013529").color).toBe("0 0% 100%");
  });
});

describe("pickPrimaryTokens", () => {
  describe.each(Object.entries(BRANDS))("marca %s", (_name, brand) => {
    it("o preenchimento comporta o texto do botão em AA", () => {
      for (const mode of ["light", "dark"] as const) {
        const { fill, fillForeground } = pickPrimaryTokens(brand, mode);
        const foreground = fillForeground === "0 0% 100%" ? "#ffffff" : "#1c1c1c";
        expect(
          contrastRatio(foreground, fill),
          `${brand} (${mode}): ${fillForeground} sobre ${fill}`,
        ).toBeGreaterThanOrEqual(AA_TEXT);
      }
    });

    it("o texto de destaque passa em AA sobre a superfície do modo", () => {
      for (const mode of ["light", "dark"] as const) {
        const { accentText } = pickPrimaryTokens(brand, mode);
        const surface = mode === "dark" ? DARK_SURFACE : LIGHT_SURFACE;
        expect(
          contrastRatio(accentText, surface),
          `${brand} (${mode}): ${accentText} sobre ${surface}`,
        ).toBeGreaterThanOrEqual(AA_TEXT);
      }
    });
  });

  // A marca padrão fica no tom original quando ele já serve de texto (mantém o
  // texto de destaque com a MESMA cor do botão), e o preenchimento desce um
  // degrau para comportar o texto branco.
  it("mantém marca e preenchimento coerentes quando possível", () => {
    const dark = pickPrimaryTokens("#1C905B", "dark");
    expect(contrastRatio(dark.accentText, DARK_SURFACE)).toBeGreaterThanOrEqual(AA_TEXT);
    expect(dark.fill).not.toBe("#1C905B"); // desceu na escala para o branco passar
    expect(dark.fillForeground).toBe("0 0% 100%");

    // Já a marca escura serve de preenchimento como está (branco com folga) e só
    // o texto de destaque precisa subir na escala.
    const escura = pickPrimaryTokens("#013529", "dark");
    expect(escura.fill).toBe("#013529");
    expect(escura.fillForeground).toBe("0 0% 100%");
    expect(escura.accentText).not.toBe("#013529");
    expect(contrastRatio(escura.accentText, DARK_SURFACE)).toBeGreaterThanOrEqual(AA_TEXT);
  });

  it("no tema claro mantém o comportamento quando a marca já passa", () => {
    // #013529 (tenant real) com branco passa em AA: nada muda no tema claro.
    const light = pickPrimaryTokens("#013529", "light");
    expect(light.fill).toBe("#013529");
    expect(light.fillForeground).toBe("0 0% 100%");
    expect(light.accentText).toBe("#013529");
  });

  it("não recicla o heurístico de lightness", () => {
    // #7A7A7A passa de lightness 48% e mesmo assim nenhum texto chega a 4.5
    // sobre ele: o preenchimento TEM de sair da cor original.
    const { fill, fillForeground } = pickPrimaryTokens("#7A7A7A", "dark");
    const foreground = fillForeground === "0 0% 100%" ? "#ffffff" : "#1c1c1c";
    expect(contrastRatio(foreground, "#7A7A7A")).toBeLessThan(AA_TEXT);
    expect(contrastRatio(foreground, fill)).toBeGreaterThanOrEqual(AA_TEXT);
  });
});

describe("hexToHSL", () => {
  it("preserva o tom da marca na escala gerada", () => {
    const { h, s, l } = hexToHSL("#1C905B");
    expect(h).toBeGreaterThan(140);
    expect(h).toBeLessThan(170);
    expect(s).toBeGreaterThan(50);
    expect(l).toBeGreaterThan(20);
  });
});

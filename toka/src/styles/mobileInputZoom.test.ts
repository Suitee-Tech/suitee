import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// O Safari do iOS amplia a página quando um <input>/<select>/<textarea> com
// `font-size` menor que 16px recebe foco. No chat o compositor usa `text-sm`
// (14px), então a tela "pulava" a cada foco no iPhone.
//
// O invariante é lido do CSS porque o defeito é exatamente "a regra saiu do
// arquivo": nenhum teste de componente enxerga o zoom do navegador.

const css = readFileSync(join(__dirname, "mobile.css"), "utf8");

describe("zoom automático do iOS ao focar controles", () => {
  const media = css.match(/@media\s*\(max-width:\s*(\d+)px\)\s*\{([\s\S]*)\}/);

  it("declara uma media query para viewports pequenas", () => {
    expect(media, "media query de mobile não encontrada em mobile.css").not.toBeNull();
    expect(Number(media![1])).toBeGreaterThanOrEqual(640);
  });

  it("aplica 16px (limite do iOS) a input, select e textarea", () => {
    const block = media![2];

    expect(block).toContain("input,");
    expect(block).toContain("select,");
    expect(block).toContain("textarea {");

    const size = Number(block.match(/font-size:\s*(\d+)px/)?.[1] ?? 0);
    expect(size).toBeGreaterThanOrEqual(16);
  });
});

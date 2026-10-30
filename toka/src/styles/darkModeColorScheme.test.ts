import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Modo noturno dos controles NATIVOS (select, date picker, scrollbar).
//
// O app pinta os campos com tokens (`bg-card`, `text-foreground`), mas o POPUP
// de um `<select>` nativo é desenhado pelo navegador com o `color-scheme` da
// página. Sem a declaração, o tema escuro do app abria a lista de opções clara
// sobre texto claro — ilegível — e o modo claro herdava o escuro do sistema
// operacional de quem usa o SO em dark mode. Os selects de permissões e o
// "Baseado em" do editor de perfil dependem disso.
//
// O invariante é lido do CSS de tokens porque o defeito é exatamente "a
// declaração sumiu do bloco": nenhum teste de componente enxerga isso.

const css = readFileSync(join(__dirname, "tailwind.css"), "utf8");

function ruleBody(selector: string): string {
  const start = css.indexOf(`${selector} {`);
  expect(start, `bloco ${selector} não encontrado em tailwind.css`).toBeGreaterThan(-1);
  const end = css.indexOf("}", start);
  return css.slice(start, end);
}

describe("color-scheme dos controles nativos", () => {
  it("declara dark no tema escuro", () => {
    expect(ruleBody(".dark")).toMatch(/color-scheme:\s*dark/);
  });

  it("declara light na raiz (o tema claro não herda o escuro do sistema)", () => {
    expect(ruleBody(":root")).toMatch(/color-scheme:\s*light/);
  });
});

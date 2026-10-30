import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Regressão de mobile: no painel de Novo Lead (ExpandableSidePanel) o conteúdo
// era `<div className="flex flex-col h-full">` com as ações ("Cancelar" /
// "Criar Lead") no FIM da área rolável. Como o painel tem um header como irmão
// e é `position: fixed` com `overflow-hidden`:
//
//   1. `h-full` no conteúdo mede 100% do PAINEL inteiro — header + 100% passa
//      da altura e o excesso é recortado, então o fim do conteúdo (com as
//      ações) ficava fora da área clicável;
//   2. `height: 100%` num elemento `fixed` mede a viewport GRANDE do mobile
//      (a faixa atrás da barra do navegador entra na conta), o que empurra a
//      última linha do painel para fora da tela visível.
//
// Com o conteúdo recortado e sem scroll que o alcançasse, "Criar Lead" e
// "Cancelar" ficavam inacessíveis no celular. A correção: altura de viewport
// dinâmica (`100dvh`) e conteúdo em contêiner flexível limitado (`min-h-0`),
// com as ações promovidas a rodapé fixo do painel (fora da área rolável).
//
// O invariante é lido do arquivo porque nenhum teste de componente enxerga
// recorte de overflow nem a altura real da viewport no navegador.

const leads = readFileSync(join(__dirname, "Leads.tsx"), "utf8");
const panel = readFileSync(join(__dirname, "../../shared/components/ExpandableSidePanel.tsx"), "utf8");

function lineWith(source: string, marker: string): string {
  const line = source.split("\n").find((candidate) => candidate.includes(marker));
  if (!line) throw new Error(`marcador ausente no fonte: ${marker}`);
  return line;
}

function indentOf(line: string): number {
  return line.length - line.trimStart().length;
}

function indexOf(source: string, marker: string): number {
  const index = source.indexOf(marker);
  if (index < 0) throw new Error(`marcador ausente no fonte: ${marker}`);
  return index;
}

describe("painel lateral no mobile", () => {
  it("o painel mede a viewport visível (dvh), não a viewport grande (h-full)", () => {
    expect(
      panel.includes("h-[100dvh]"),
      "o painel voltou a usar altura de viewport grande: a última linha cai atrás da barra do navegador",
    ).toBe(true);
    expect(panel).not.toMatch(/w-full h-full shadow-2xl flex flex-col overflow-hidden/);
  });

  it("o conteúdo é envolvido por um contêiner flexível de altura limitada", () => {
    // Sem este contêiner, um filho com `h-full` mede 100% do painel e estoura a
    // altura por causa do header — o excesso é recortado pelo overflow-hidden.
    expect(panel.includes('<div className="flex min-h-0 flex-1 flex-col">{children}</div>')).toBe(true);
  });
});

describe("ações do painel de Novo Lead", () => {
  it("o conteúdo do formulário usa flex-1 min-h-0 em vez de h-full", () => {
    expect(leads.includes('<div className="flex min-h-0 flex-1 flex-col">')).toBe(true);
    expect(
      leads.includes('<div className="flex flex-col h-full">'),
      "o conteúdo do painel voltou a medir 100% do painel inteiro (header + conteúdo estoura a altura)",
    ).toBe(false);
  });

  it("a área rolável pode encolher abaixo do conteúdo (min-h-0)", () => {
    expect(leads.includes("min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-6")).toBe(true);
  });

  it("as ações ficam em rodapé fixo do painel, irmão da área rolável", () => {
    const scroll = lineWith(leads, "min-h-0 flex-1 overflow-y-auto overscroll-contain");
    const footer = lineWith(leads, "shrink-0 items-center justify-between gap-3 border-t border-border bg-card");

    // Mesmo nível de indentação = irmãos; o rodapé depois da área rolável.
    expect(indentOf(footer)).toBe(indentOf(scroll));
    expect(indexOf(leads, "shrink-0 items-center justify-between gap-3 border-t border-border bg-card")).toBeGreaterThan(
      indexOf(leads, "min-h-0 flex-1 overflow-y-auto overscroll-contain"),
    );
    // As ações estão DENTRO do rodapé (depois dele no arquivo), por isso não
    // dependem do scroll do formulário.
    const footerIndex = indexOf(leads, "shrink-0 items-center justify-between gap-3 border-t border-border bg-card");
    expect(indexOf(leads, '{selectedExistingLead ? "Salvar" : "Criar Lead"}')).toBeGreaterThan(footerIndex);
    expect(leads.slice(footerIndex, footerIndex + 900)).toContain("Cancelar");
    expect(
      leads.includes("pt-4 border-t border-border flex items-center justify-between"),
      "as ações voltaram para dentro do conteúdo rolável (ficam inalcançáveis no mobile)",
    ).toBe(false);
  });

  it("o rodapé respeita a área segura do aparelho", () => {
    expect(leads.includes("pb-[max(1rem,env(safe-area-inset-bottom))]")).toBe(true);
  });
});

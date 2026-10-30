import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Regressão de mobile: as duas linhas do cabeçalho de Leads agrupam
// (`flex`) o toggle de visualização + botões de ação e o toggle de origem +
// três selects. Sem `flex-wrap`, a largura mínima da linha (os selects são
// `w-full` no mobile e os botões têm rótulos longos) ultrapassava a viewport
// e empurrava o `<main className="overflow-auto">` do layout: a página
// inteira ganhava scroll horizontal — inclusive o título e a lista — embora
// o kanban já role dentro do próprio board.
//
// O invariante é lido do arquivo porque nenhum teste de componente enxerga a
// largura mínima de uma linha flex no layout do navegador.

const source = readFileSync(join(__dirname, "Leads.tsx"), "utf8");

describe("cabeçalho de Leads no mobile", () => {
  it("a linha de ações (toggle + botões) quebra em vez de vazar", () => {
    expect(
      source.includes("flex flex-wrap items-center gap-3 w-full sm:w-auto"),
      "a linha de ações perdeu o flex-wrap (volta o scroll horizontal do body no mobile)",
    ).toBe(true);
  });

  it("a linha de filtros (abas + selects) quebra em vez de vazar", () => {
    expect(
      source.includes("flex flex-wrap items-center gap-2 w-full sm:w-auto"),
      "a linha de filtros perdeu o flex-wrap (volta o scroll horizontal do body no mobile)",
    ).toBe(true);
  });
});

// O painel de busca/filtros ocupava ~40% da tela do celular antes de qualquer
// conteúdo. Ele passa a ficar recolhido atrás de dois ícones no mobile e segue
// sempre visível em ≥sm (onde há largura de sobra).
describe("busca e filtros recolhidos no mobile", () => {
  it("o painel é controlado pelo estado mobilePanel e tem id para o aria-controls", () => {
    expect(source.includes('id="leads-search-filters"')).toBe(true);
    expect(
      source.includes('mobilePanel ? "flex flex-col" : "hidden"'),
      "o painel de busca/filtros deixou de ser recolhível no mobile",
    ).toBe(true);
  });

  it("os dois botões de ícone existem, só no mobile, e apontam para o painel", () => {
    const toggles = source.match(/aria-controls="leads-search-filters"/g) ?? [];
    expect(toggles.length).toBe(2);
    expect(source.includes('prev === "search" ? null : "search"')).toBe(true);
    expect(source.includes('prev === "filters" ? null : "filters"')).toBe(true);
  });

  it("o estado ativo continua visível com o painel fechado (ponto e contador)", () => {
    // Busca ativa → ponto no ícone; filtros ativos → contador com o total.
    expect(source.includes("activeFilterCount > 0 && (")).toBe(true);
    expect(source.includes("activeFilterCount > 0 ? `Filtrar leads")).toBe(true);
    expect(source.includes("{searchTerm && (")).toBe(true);
  });
});

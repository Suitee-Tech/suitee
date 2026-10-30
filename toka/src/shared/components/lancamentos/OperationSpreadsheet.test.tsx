import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { EconomicOperation } from "../../lib/lancamentos/presentation";
import type { Lancamento, LancamentoKind } from "../../lib/lancamentos/types";
import { groupOperations } from "../../lib/lancamentos/operationsView";
import { OperationSpreadsheet } from "./OperationSpreadsheet";

const NOW = new Date("2026-09-18T12:00:00");

function makeItem(overrides: Partial<Lancamento> & { id: string; kind: LancamentoKind }): Lancamento {
  const { id, kind, ...rest } = overrides;
  return {
    id,
    kind,
    status: "pendente",
    amount: 1000,
    currency: "BRL",
    counterparty: { kind: "tenant", label: "Inquilino" },
    origin: { entity_kind: "contract", entity_id: "ct-1", label: "Contrato CTR-1" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "2026-09-05T10:00:00Z",
    updated_at: "2026-09-05T10:00:00Z",
    source: {},
    kind_data: { direction: "entrada" },
    ...rest,
  } as unknown as Lancamento;
}

function makeOperation(key: string, title: string, items: Lancamento[]): EconomicOperation {
  return {
    key,
    title,
    items,
    primary: items[0],
    latestAt: items[0].updated_at,
    amount: items.reduce((sum, item) => sum + item.amount, 0),
    openCount: items.length,
    divergenceCount: 0,
  };
}

const charge = makeItem({
  id: "charge-1",
  kind: "charge",
  amount: 2500,
  status: "vencido",
  due_at: "2026-09-10T00:00:00Z",
  counterparty: { kind: "tenant", label: "Vanessa Rodrigues Melo" },
  presentation: {
    operation_key: "contract:ct-1:rent:2026-09",
    operation_title: "Locação · Apartamento dos Ipês",
    contract_id: "ct-1",
    property_title: "Apartamento dos Ipês",
    reference_month: "2026-09",
    flow: "rent",
  },
});

const commission = makeItem({
  id: "commission-1",
  kind: "commission",
  amount: 800,
  counterparty: { kind: "broker", label: "Tiago Santini" },
  presentation: {
    operation_key: "contract:ct-1:rent:2026-09",
    operation_title: "Locação · Apartamento dos Ipês",
    contract_id: "ct-1",
    property_title: "Apartamento dos Ipês",
    reference_month: "2026-09",
    flow: "rent",
  },
});

const octoberCharge = makeItem({
  id: "charge-2",
  kind: "charge",
  amount: 2600,
  due_at: "2026-10-10T00:00:00Z",
  presentation: {
    operation_key: "contract:ct-1:rent:2026-10",
    operation_title: "Locação · Outubro",
    contract_id: "ct-1",
    reference_month: "2026-10",
    flow: "rent",
  },
});

const operations = [
  makeOperation("contract:ct-1:rent:2026-09", "Locação · Apartamento dos Ipês", [charge, commission]),
  makeOperation("contract:ct-1:rent:2026-10", "Locação · Outubro", [octoberCharge]),
];

const sections = groupOperations(operations, "competence", "focus", NOW);

/**
 * O layout da planilha é decidido por media query (tabela só no desktop).
 * O stub resolve a consulta contra uma largura real, para exercitar cada
 * combinação de colunas visíveis.
 */
function useViewport(width: number) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => {
      const min = /min-width:\s*(\d+)px/.exec(query);
      const max = /max-width:\s*(\d+)px/.exec(query);
      const matches = min ? width >= Number(min[1]) : max ? width <= Number(max[1]) : false;
      return {
        matches,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      };
    }),
  );
}

/** Tabela no desktop, com Origem visível (≥1280) e Linhas oculta. */
beforeEach(() => useViewport(1440));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("OperationSpreadsheet", () => {
  it("mostra uma linha condensada por operação com competência, vencimento e valores em aberto", () => {
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

    const row = screen.getByText("Locação · Apartamento dos Ipês").closest("tr") as HTMLElement;
    expect(within(row).getByText("Set/2026")).toBeInTheDocument();
    expect(within(row).getByText("10/09/2026")).toBeInTheDocument();
    expect(within(row).getByText("R$ 2.500,00")).toBeInTheDocument(); // a receber
    expect(within(row).getByText("R$ 800,00")).toBeInTheDocument(); // a pagar
    expect(within(row).getByText("Em atraso")).toBeInTheDocument();
  });

  it("traz linha de grupo com contagem e subtotais alinhados às colunas de valor", () => {
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

    const groupRow = screen.getByText("Set/2026", { selector: "span" }).closest("tr") as HTMLElement;
    expect(within(groupRow).getByText("1 operação")).toBeInTheDocument();
    expect(within(groupRow).getByText("1 em atraso")).toBeInTheDocument();

    // Os subtotais caem exatamente nas colunas de valor (as duas células à
    // esquerda da coluna de situação), como em uma planilha.
    const cells = Array.from(groupRow.children) as HTMLElement[];
    const texts = cells.map((cell) => (cell.textContent ?? "").replace(/\u00a0/g, " "));
    expect(texts[texts.length - 3]).toBe("R$ 2.500,00");
    expect(texts[texts.length - 2]).toBe("R$ 800,00");
  });

  it("abre as linhas auditáveis da operação ao expandir", () => {
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

    expect(screen.queryByText("Tiago Santini")).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Expandir Locação · Apartamento dos Ipês"));

    expect(screen.getByText("Tiago Santini")).toBeInTheDocument();
    expect(screen.getByText("Vanessa Rodrigues Melo")).toBeInTheDocument();
    expect(screen.getByText("Cobrança")).toBeInTheDocument();
    expect(screen.getByText("Comissão")).toBeInTheDocument();
  });

  it("seleciona a operação de uma linha só e a linha filha quando expandida", () => {
    const onSelect = vi.fn();
    render(<OperationSpreadsheet sections={sections} onSelect={onSelect} />);

    fireEvent.click(screen.getByText("Locação · Outubro"));
    expect(onSelect).toHaveBeenCalledWith(octoberCharge);

    onSelect.mockClear();
    fireEvent.click(screen.getByLabelText("Expandir Locação · Apartamento dos Ipês"));
    fireEvent.click(screen.getByText("Tiago Santini"));
    expect(onSelect).toHaveBeenCalledWith(commission);
  });

  it("fecha o recorte com o total de linhas auditáveis e o alerta de não somar", () => {
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

    expect(screen.getByText(/3 linhas auditáveis/)).toBeInTheDocument();
    expect(screen.getByText(/não deve ser somada/)).toBeInTheDocument();
  });

  it("revela Origem e Linhas conforme a largura disponível", () => {
    useViewport(1024);
    const { unmount } = render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);
    expect(screen.queryByRole("columnheader", { name: "Origem" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Linhas" })).not.toBeInTheDocument();
    unmount();

    useViewport(1600);
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);
    expect(screen.getByRole("columnheader", { name: "Origem" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Linhas" })).toBeInTheDocument();
  });

  it.each([1024, 1280, 1440, 1600])(
    "mantém todas as linhas com o mesmo número de colunas em %ipx (sem coluna fantasma)",
    (width) => {
      useViewport(width);
      const { unmount } = render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

      const table = screen.getByRole("table");
      const spans = Array.from(table.querySelectorAll("tr")).map((row) =>
        Array.from(row.children).reduce((total, cell) => total + (cell as HTMLTableCellElement).colSpan, 0),
      );

      expect(new Set(spans).size).toBe(1);
      unmount();
    },
  );

  it("não repete cabeçalho de grupo quando não há agrupamento", () => {
    const flat = groupOperations(operations, "none", "focus", NOW);
    render(<OperationSpreadsheet sections={flat} onSelect={vi.fn()} />);

    expect(screen.queryByText("Todas as operações")).not.toBeInTheDocument();
    expect(screen.getByText("Locação · Apartamento dos Ipês")).toBeInTheDocument();
  });

  it("troca a tabela por cartões no mobile mantendo os mesmos dados", () => {
    useViewport(390);
    render(<OperationSpreadsheet sections={sections} onSelect={vi.fn()} />);

    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.getByText("Locação · Apartamento dos Ipês")).toBeInTheDocument();

    // Um cartão por operação, com os mesmos fatos da linha da planilha.
    const card = screen.getByText("Locação · Apartamento dos Ipês").closest("article") as HTMLElement;
    expect(within(card).getByText("Competência")).toBeInTheDocument();
    expect(within(card).getByText("Set/2026")).toBeInTheDocument();
    expect(within(card).getByText("R$ 2.500,00")).toBeInTheDocument();
    expect(within(card).getByText("R$ 800,00")).toBeInTheDocument();
    expect(within(card).getByText("Em atraso")).toBeInTheDocument();
  });

  it("expande as linhas auditáveis também no cartão do mobile", () => {
    useViewport(390);
    const onSelect = vi.fn();
    render(<OperationSpreadsheet sections={sections} onSelect={onSelect} />);

    fireEvent.click(screen.getByText("Locação · Apartamento dos Ipês"));
    expect(screen.getByText("Tiago Santini")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Tiago Santini"));
    expect(onSelect).toHaveBeenCalledWith(commission);
  });
});

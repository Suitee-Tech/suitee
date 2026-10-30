import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Lancamento, LancamentoKind } from "../../lib/lancamentos/types";
import type { EconomicOperation } from "../../lib/lancamentos/presentation";
import type { LancamentosFilters } from "../../queries/lancamentos";
import { OperationsExplorer, type OperationViewMode } from "./OperationsExplorer";
import type { OperationGroupId, OperationSortId } from "../../lib/lancamentos/operationsView";

const NOW_MONTH = new Date().toISOString().slice(0, 7);

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

const overdueCharge = makeItem({
  id: "charge-1",
  kind: "charge",
  amount: 2500,
  status: "vencido",
  due_at: `${NOW_MONTH}-10T00:00:00Z`,
  presentation: { operation_key: "op-1", operation_title: "Locação · Ipês", reference_month: NOW_MONTH, flow: "rent" },
});

const pendingCommission = makeItem({
  id: "commission-1",
  kind: "commission",
  amount: 800,
  counterparty: { kind: "broker", label: "Tiago Santini" },
  presentation: { operation_key: "op-2", operation_title: "Comissão · Venda", reference_month: NOW_MONTH, flow: "sale" },
});

const operations = [
  makeOperation("op-1", "Locação · Ipês", [overdueCharge]),
  makeOperation("op-2", "Comissão · Venda", [pendingCommission]),
];

function Harness({ onFilters }: { onFilters?: (filters: LancamentosFilters) => void } = {}) {
  const [filters, setFilters] = useState<LancamentosFilters>({});
  const [view, setView] = useState<OperationViewMode>("cards");
  const [groupBy, setGroupBy] = useState<OperationGroupId>("competence");
  const [sort, setSort] = useState<OperationSortId>("focus");

  return (
    <OperationsExplorer
      operations={operations}
      facetSource={operations}
      filters={filters}
      onFiltersChange={(next) => {
        setFilters(next);
        onFilters?.(next);
      }}
      view={view}
      onViewChange={setView}
      groupBy={groupBy}
      onGroupChange={setGroupBy}
      sort={sort}
      onSortChange={setSort}
      onSelect={vi.fn()}
    />
  );
}

/**
 * O layout da planilha é decidido por media query (tabela só no desktop).
 * O stub resolve a consulta contra uma largura real.
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

beforeEach(() => useViewport(1440));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("OperationsExplorer", () => {
  it("deixa os atalhos e os controles de leitura sempre à vista", () => {
    render(<Harness />);

    expect(screen.getByLabelText("Competência")).toBeInTheDocument();
    expect(screen.getByLabelText("Vencimento")).toBeInTheDocument();
    expect(screen.getByLabelText("Agrupar por")).toBeInTheDocument();
    expect(screen.getByLabelText("Ordenar")).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Visualização" })).toBeInTheDocument();
    for (const shortcut of ["Em atraso", "A receber", "A pagar", "Divergências"]) {
      expect(screen.getByRole("button", { name: new RegExp(shortcut) })).toBeInTheDocument();
    }
  });

  it("mostra a contagem de cada atalho pelo estado real das operações", () => {
    render(<Harness />);

    const overdue = screen.getByRole("button", { name: /Em atraso/ });
    expect(within(overdue).getByText("1")).toBeInTheDocument();

    const receivable = screen.getByRole("button", { name: /A receber/ });
    expect(within(receivable).getByText("1")).toBeInTheDocument();

    const payable = screen.getByRole("button", { name: /A pagar/ });
    expect(within(payable).getByText("1")).toBeInTheDocument();
  });

  it("liga e desliga o atalho de atraso", () => {
    const onFilters = vi.fn();
    render(<Harness onFilters={onFilters} />);

    const overdue = screen.getByRole("button", { name: /Em atraso/ });
    fireEvent.click(overdue);
    expect(onFilters).toHaveBeenLastCalledWith(expect.objectContaining({ overdueOnly: true }));

    fireEvent.click(overdue);
    expect(onFilters).toHaveBeenLastCalledWith(expect.objectContaining({ overdueOnly: undefined }));
  });

  it("o atalho de fluxo restringe aos lançamentos em aberto e solta ao desligar", () => {
    const onFilters = vi.fn();
    render(<Harness onFilters={onFilters} />);

    const receivable = screen.getByRole("button", { name: /A receber/ });
    fireEvent.click(receivable);
    expect(onFilters).toHaveBeenLastCalledWith(
      expect.objectContaining({ flow: "receber", statuses: ["pendente", "aprovado", "parcial", "vencido"] }),
    );

    fireEvent.click(receivable);
    expect(onFilters).toHaveBeenLastCalledWith(expect.objectContaining({ flow: undefined, statuses: [] }));
  });

  it("prioriza o mês atual como opção de competência e de vencimento", () => {
    const onFilters = vi.fn();
    render(<Harness onFilters={onFilters} />);

    fireEvent.change(screen.getByLabelText("Competência"), { target: { value: NOW_MONTH } });
    expect(onFilters).toHaveBeenLastCalledWith(expect.objectContaining({ competence: NOW_MONTH }));

    fireEvent.change(screen.getByLabelText("Vencimento"), { target: { value: NOW_MONTH } });
    expect(onFilters).toHaveBeenLastCalledWith(expect.objectContaining({ dueMonth: NOW_MONTH }));
  });

  it("troca para a planilha condensada e mantém o agrupamento escolhido", () => {
    render(<Harness />);

    fireEvent.click(screen.getByRole("button", { name: /Planilha/ }));
    expect(screen.getByRole("table", { name: "Operações econômicas em visão condensada" })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Agrupar por"), { target: { value: "status" } });
    expect(screen.getByRole("button", { name: /Planilha/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("resume o recorte com operações, linhas e valores em aberto", () => {
    render(<Harness />);

    const summary = screen.getByLabelText("Recorte e agrupamento das operações");
    expect(within(summary).getByText("2 operações")).toBeInTheDocument();
    expect(within(summary).getByText("2 linhas")).toBeInTheDocument();
    expect(within(summary).getByText(/a receber R\$ 2\.500,00/)).toBeInTheDocument();
    expect(within(summary).getByText(/a pagar R\$ 800,00/)).toBeInTheDocument();
    expect(within(summary).getByText("1 em atraso")).toBeInTheDocument();
  });

  it("agrupa os cartões por competência com cabeçalho e subtotal", () => {
    render(<Harness />);

    const section = screen.getByRole("heading", { name: /Set|Out|Nov|Dez|Jan|Fev|Mar|Abr|Mai|Jun|Jul|Ago/ }).closest("section") as HTMLElement;
    expect(within(section).getByText("2 operações")).toBeInTheDocument();
    expect(within(section).getByText(/a receber R\$ 2\.500,00/)).toBeInTheDocument();
  });
});

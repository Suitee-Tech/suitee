import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { parseOperationView } from "./lancamentosView";
import type { OwnerTransfer } from "../services/transfer.service";
import { transferToLancamento } from "../lib/lancamentos/mappers";

// Relato: /financeiro/lancamentos deve abrir na PLANILHA; o cartão é o modo
// explícito (`?view=cards`). Antes o padrão era cartão e a planilha exigia
// `?view=sheet` na URL.
describe("visualização padrão de lançamentos", () => {
  it("abre na planilha quando a URL não diz nada", () => {
    expect(parseOperationView(null)).toBe("sheet");
    expect(parseOperationView("")).toBe("sheet");
  });
  it("respeita a escolha explícita do operador", () => {
    expect(parseOperationView("cards")).toBe("cards");
    expect(parseOperationView("sheet")).toBe("sheet");
  });
  it("ignora valor desconhecido caindo no padrão (planilha)", () => {
    expect(parseOperationView("tabela")).toBe("sheet");
  });
});

const harness = vi.hoisted(() => ({
  transfers: [] as unknown[],
  refetchAll: vi.fn(),
  process: vi.fn(),
}));

vi.mock("../queries/lancamentos", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../queries/lancamentos")>();
  return {
    ...actual,
    useLancamentosList: () => ({
      data: harness.transfers,
      isLoading: false,
      partialFailures: [],
      refetchAll: harness.refetchAll,
    }),
  };
});

vi.mock("../context/ContractsContext", () => ({ useContractsContext: () => ({ contracts: [] }) }));
vi.mock("../context/PropertiesContext", () => ({ usePropertiesContext: () => ({ properties: [] }) }));
vi.mock("../context/LeadsContext", () => ({ useLeadsContext: () => ({ leads: [] }) }));
vi.mock("../context/AgentsContext", () => ({ useAgentsContext: () => ({ agents: [] }) }));
vi.mock("../context/PermissionsContext", () => ({ usePermissionsContext: () => ({ can: () => true }) }));

vi.mock("../services/transfer.service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../services/transfer.service")>();
  return {
    ...actual,
    transferService: {
      ...actual.transferService,
      approve: vi.fn().mockResolvedValue({ data: {} }),
      process: harness.process,
    },
  };
});

import { LancamentosInbox } from "./LancamentosInbox";

const transfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer => ({
  id: "transfer-zero",
  tenant_id: "tenant-1",
  source_charge_id: "charge-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-09",
  settlement_type: "rent",
  gross_rent: 1_000,
  admin_fee_percent: 100,
  admin_fee_amount: 1_000,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 0,
  status: "PENDING",
  owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-01T12:00:00Z",
  ...overrides,
});

function useDesktopViewport() {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => {
      const min = /min-width:\s*(\d+)px/.exec(query);
      const max = /max-width:\s*(\d+)px/.exec(query);
      return {
        matches: min ? 1440 >= Number(min[1]) : max ? 1440 <= Number(max[1]) : false,
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

function renderInbox(transfers: OwnerTransfer[]) {
  harness.transfers = transfers.map(transferToLancamento);
  return render(
    <MemoryRouter initialEntries={["/financeiro/lancamentos"]}>
      <LancamentosInbox />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useDesktopViewport();
  harness.transfers = [];
  // O mock do serviço é fixado no módulo: limpar preserva a mesma referência.
  harness.process.mockReset();
  harness.process.mockResolvedValue({ data: {} });
  harness.refetchAll.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LancamentosInbox — repasse sem saldo a repassar", () => {
  it("explica o repasse zerado no detalhe e conclui sem comprovante", async () => {
    const user = userEvent.setup();
    renderInbox([transfer()]);

    // A planilha abre pelo repasse: um clique na operação seleciona a linha.
    await user.click(await screen.findByRole("row", { name: /Locação/i }));

    expect(await screen.findByRole("heading", { level: 2, name: "Detalhes do Repasse" })).toBeInTheDocument();
    expect(
      screen.getByText(/Sem saldo a repassar nesta competência: conclua o repasse para encerrá-lo/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/não possui origem econômica e saldo do proprietário conciliados/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Concluir sem saldo/i }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText("Concluir repasse sem saldo")).toBeInTheDocument();
    expect(within(dialog).getByText(/nenhuma movimentação de caixa será registrada/i)).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: /Concluir sem saldo/i }));

    // Conclusão sem comprovante: o backend encerra o repasse zerado.
    await waitFor(() => expect(harness.process).toHaveBeenCalledWith("transfer-zero", {}));
    expect(harness.refetchAll).toHaveBeenCalled();
  });

  it("mostra o repasse zerado já liquidado como Liquidado sem saldo na planilha", async () => {
    const user = userEvent.setup();
    renderInbox([
      transfer({ status: "PROCESSED", settled_without_balance: true, processed_by: "operador-1" }),
      transfer({
        id: "transfer-2",
        status: "PROCESSED",
        net_amount: 900,
        admin_fee_amount: 100,
        admin_fee_percent: 10,
        owner_balance: { total_cents: 90_000, reserved_cents: 0, blocked_cents: 0, available_cents: 90_000 },
      }),
    ]);

    // Duas linhas na mesma competência: a operação abre e expõe o status de cada
    // linha auditável, onde o repasse zerado precisa aparecer liquidado.
    await user.click(await screen.findByRole("button", { name: /Expandir/i }));

    expect(await screen.findByText("Liquidado sem saldo")).toBeInTheDocument();
    expect(screen.queryByText("A apurar")).not.toBeInTheDocument();
  });

  it("não oferece conclusão para repasse com saldo a repassar", async () => {
    const user = userEvent.setup();
    renderInbox([
      transfer({
        net_amount: 900,
        admin_fee_amount: 100,
        admin_fee_percent: 10,
        owner_balance: { total_cents: 90_000, reserved_cents: 0, blocked_cents: 0, available_cents: 90_000 },
      }),
    ]);

    await user.click(await screen.findByRole("row", { name: /Locação/i }));

    expect(await screen.findByRole("heading", { level: 2, name: "Detalhes do Repasse" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Aprovar/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Concluir sem saldo/i })).not.toBeInTheDocument();
  });
});

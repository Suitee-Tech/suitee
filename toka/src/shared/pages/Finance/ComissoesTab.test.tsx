import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import { listAllCommissions } from "../../services/financialLists";
import type { Commission } from "../../services/commission.service";
import { ComissoesTab } from "./ComissoesTab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

// Arrays ESTÁVEIS: o tab deriva `brokerName`/`dealLabel` deles e um array novo a
// cada render faria os efeitos de derivação rodarem para sempre.
const contextData = vi.hoisted(() => ({ dealDTOs: [] as unknown[], agents: [] as unknown[] }));
vi.mock("../../context/DealsContext", () => ({ useDealsContext: () => ({ dealDTOs: contextData.dealDTOs }) }));
vi.mock("../../context/AgentsContext", () => ({ useAgentsContext: () => ({ agents: contextData.agents }) }));

vi.mock("../../services/financialLists", () => ({ listAllCommissions: vi.fn() }));

const listAll = vi.mocked(listAllCommissions);

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog usa.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  proto.hasPointerCapture ??= vi.fn(() => false);
  proto.setPointerCapture ??= vi.fn();
  proto.releasePointerCapture ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

const pendingCommission: Commission = {
  id: "commission-pending",
  tenant_id: "tenant-1",
  deal_id: "deal-1",
  broker_id: "broker-1",
  role: "captador",
  amount: 4000,
  gross_amount: 4000,
  net_amount: 4000,
  currency: "BRL",
  status: "PENDING",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-02T10:00:00Z",
};

const approvedCommission: Commission = {
  ...pendingCommission,
  id: "commission-approved",
  deal_id: "deal-2",
  status: "APPROVED",
  amount: 4000,
  gross_amount: 4000,
  deductions: [{ id: "deduction-1", kind: "IRRF", amount: 800 }],
  net_amount: 3200,
};

beforeEach(() => {
  vi.restoreAllMocks();
  listAll.mockReset();
  listAll.mockResolvedValue([pendingCommission, approvedCommission]);
});

const renderTab = () =>
  render(
    <MemoryRouter>
      <ComissoesTab />
    </MemoryRouter>,
  );

const rowFor = (dealLabel: string) => {
  const table = screen.getByRole("table");
  return within(table).getByText(dealLabel).closest("tr") as HTMLElement;
};

describe("ComissoesTab · composição Bruto → Descontos → Líquido", () => {
  it("mostra o desconto que separa o bruto do líquido em cada comissão", async () => {
    renderTab();
    await screen.findByText("deal-1");

    const withDeduction = rowFor("deal-2");
    expect(within(withDeduction).getByText("R$ 4.000,00")).toBeInTheDocument();
    expect(within(withDeduction).getByText("−R$ 800,00")).toBeInTheDocument();
    expect(within(withDeduction).getByText("R$ 3.200,00")).toBeInTheDocument();
    expect(within(withDeduction).getByText("1 lançamento")).toBeInTheDocument();

    // Sem desconto o valor é zero, e não vazio: a coluna é a mesma para todos.
    const withoutDeduction = rowFor("deal-1");
    expect(within(withoutDeduction).getByText("R$ 0,00")).toBeInTheDocument();
  });

  it("explica que o desconto vira tributo retido e só entra com a comissão pendente", async () => {
    renderTab();
    await screen.findByText("deal-1");

    expect(screen.getByText(/só é aceito enquanto a comissão está Pendente/i)).toBeInTheDocument();
    expect(screen.getByText(/Impostos retidos/i)).toBeInTheDocument();
  });
});

describe("ComissoesTab · caminho para lançar o desconto", () => {
  it("abre o mesmo drawer de detalhe pela ação 'Lançar desconto' da linha", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await screen.findByText("deal-1");

    const row = rowFor("deal-1");
    await user.click(within(row).getByRole("button", { name: /Lançar desconto na comissão commission-pending/i }));

    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByRole("heading", { name: /Detalhes da Comissão/i })).toBeInTheDocument();
    // O editor está liberado e diz até quando dá para lançar.
    expect(within(drawer).getByText(/Adicionar desconto/i)).toBeInTheDocument();
    expect(within(drawer).getByText(/antes da aprovação da ordem de pagamento/i)).toBeInTheDocument();
    expect(within(drawer).queryByText(/Lançamento de desconto indisponível/i)).not.toBeInTheDocument();
  });

  it("abre os descontos da comissão já lançada pela ação 'Descontos'", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await screen.findByText("deal-1");

    const row = rowFor("deal-2");
    await user.click(within(row).getByRole("button", { name: /Ver descontos da comissão commission-approved/i }));

    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByText("IRRF")).toBeInTheDocument();
  });
});

describe("ComissoesTab · bloqueio do desconto explicado", () => {
  it("explica por que a comissão aprovada não aceita mais desconto", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await screen.findByText("deal-1");

    const row = rowFor("deal-2");
    await user.click(within(row).getByRole("button", { name: /Ver descontos da comissão commission-approved/i }));

    const drawer = await screen.findByRole("dialog");
    // O formulário não aparece, mas o motivo (e o caminho) sim.
    expect(await within(drawer).findByText(/Lançamento de desconto indisponível/i)).toBeInTheDocument();
    expect(within(drawer).getByText(/já foi aprovada e o valor da ordem de pagamento está comprometido/i)).toBeInTheDocument();
    expect(within(drawer).queryByText(/^Adicionar desconto$/i)).not.toBeInTheDocument();
  });

  it("não deixa a composição ambígua: bruto, desconto e líquido sempre visíveis", async () => {
    renderTab();
    await screen.findByText("deal-1");

    await waitFor(() => expect(listAll).toHaveBeenCalledTimes(1));
    const table = screen.getByRole("table");
    for (const header of ["Bruto", "Descontos", "Líquido"]) {
      expect(within(table).getByRole("columnheader", { name: header })).toBeInTheDocument();
    }
  });
});

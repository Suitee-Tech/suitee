import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import { brokerAdvanceService, type BrokerAdvance, type BrokerBalance } from "../../services/brokerAdvance.service";
import { AdiantamentosTab } from "./AdiantamentosTab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

// Os helpers de saldo reais são usados pela tela; só o cliente HTTP do serviço
// é dublado, para o teste travar o que a tela envia e como lê a resposta.
vi.mock("../../services/brokerAdvance.service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../services/brokerAdvance.service")>();
  return {
    ...actual,
    brokerAdvanceService: { list: vi.fn(), create: vi.fn(), cancel: vi.fn(), outstandingForBroker: vi.fn() },
  };
});

// O nome do corretor vem do contexto compartilhado (mesma fonte da aba de
// comissões): aqui ele é dublado para o teste não depender do provider.
vi.mock("../../context/AgentsContext", () => ({
  useAgentsContext: () => ({
    agents: [
      { id: "broker-1", name: "Ana Souza" },
      { id: "broker-2", name: "Bruno Lima" },
      { id: "broker-3", name: "Carla Dias" },
    ],
  }),
}));

const list = vi.mocked(brokerAdvanceService.list);
const create = vi.mocked(brokerAdvanceService.create);
const cancel = vi.mocked(brokerAdvanceService.cancel);

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

const openAdvance: BrokerAdvance = {
  id: "adv-1",
  tenant_id: "tenant-1",
  broker_id: "broker-1",
  amount_cents: 200000,
  settled_cents: 0,
  status: "OPEN",
  description: "pedido em setembro",
  granted_at: "2026-09-10T00:00:00Z",
  created_at: "2026-09-10T10:00:00Z",
  updated_at: "2026-09-10T10:00:00Z",
};

/** Em aberto, mas já com abatimento: não pode ser cancelado. */
const partiallySettled: BrokerAdvance = {
  id: "adv-2",
  tenant_id: "tenant-1",
  broker_id: "broker-2",
  amount_cents: 300000,
  settled_cents: 100000,
  status: "OPEN",
  granted_at: "2026-08-01T00:00:00Z",
  created_at: "2026-08-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const settledAdvance: BrokerAdvance = {
  id: "adv-3",
  tenant_id: "tenant-1",
  broker_id: "broker-3",
  amount_cents: 100000,
  settled_cents: 100000,
  status: "SETTLED",
  granted_at: "2026-07-05T00:00:00Z",
  created_at: "2026-07-05T10:00:00Z",
  updated_at: "2026-08-05T10:00:00Z",
};

const cancelledAdvance: BrokerAdvance = {
  id: "adv-4",
  tenant_id: "tenant-1",
  broker_id: "broker-3",
  amount_cents: 50000,
  settled_cents: 0,
  status: "CANCELLED",
  granted_at: "2026-06-01T00:00:00Z",
  created_at: "2026-06-01T10:00:00Z",
  updated_at: "2026-06-02T10:00:00Z",
};

const balances: BrokerBalance[] = [
  { broker_id: "broker-1", open_count: 1, outstanding_cents: 200000, settled_cents: 100000 },
  { broker_id: "broker-2", open_count: 1, outstanding_cents: 200000, settled_cents: 100000 },
];

beforeEach(() => {
  vi.restoreAllMocks();
  list.mockReset();
  create.mockReset();
  cancel.mockReset();
  list.mockResolvedValue({
    items: [openAdvance, partiallySettled, settledAdvance, cancelledAdvance],
    balances,
  });
});

const renderTab = () =>
  render(
    <MemoryRouter>
      <AdiantamentosTab />
    </MemoryRouter>,
  );

describe("AdiantamentosTab · lista e totais", () => {
  it("lista corretor, valor, abatido, saldo, data e status de cada adiantamento", async () => {
    renderTab();

    // O nome do corretor também aparece no filtro: a linha é procurada na tabela.
    const table = await screen.findByRole("table");
    await within(table).findByText("Ana Souza");

    const row = within(table).getByText("Ana Souza").closest("tr") as HTMLElement;
    // Valor e saldo coincidem aqui: o adiantamento ainda não foi abatido.
    expect(within(row).getAllByText("R$ 2.000,00")).toHaveLength(2);
    expect(within(row).getByText("R$ 0,00")).toBeInTheDocument(); // abatido
    expect(within(row).getByText("10/09/2026")).toBeInTheDocument();
    expect(within(row).getByText("Em aberto")).toBeInTheDocument();

    const partialRow = within(table).getByText("Bruno Lima").closest("tr") as HTMLElement;
    expect(within(partialRow).getByText("R$ 3.000,00")).toBeInTheDocument(); // valor
    expect(within(partialRow).getByText("R$ 1.000,00")).toBeInTheDocument(); // abatido
    // Saldo em aberto = valor - abatido.
    expect(within(partialRow).getByText("R$ 2.000,00")).toBeInTheDocument();
    expect(within(table).getByText("Quitado")).toBeInTheDocument();
    expect(within(table).getByText("Cancelado")).toBeInTheDocument();
  });

  it("mostra o saldo a recuperar e o já abatido calculados no servidor (meta.balances)", async () => {
    renderTab();

    const outstandingCard = (await screen.findByText("2 adiantamentos em aberto")).closest(
      "div.rounded-xl",
    ) as HTMLElement;
    expect(within(outstandingCard).getByText("Saldo a recuperar")).toBeInTheDocument();
    expect(within(outstandingCard).getByText("R$ 4.000,00")).toBeInTheDocument();

    const settledCard = screen.getByText("recuperado em descontos nas comissões").closest("div.rounded-xl") as HTMLElement;
    expect(within(settledCard).getByText("Já abatido")).toBeInTheDocument();
    expect(within(settledCard).getByText("R$ 2.000,00")).toBeInTheDocument();
  });

  it("oferece cancelar apenas para adiantamento aberto e sem abatimento", async () => {
    renderTab();

    const table = await screen.findByRole("table");
    await within(table).findByText("Ana Souza");
    const cancelButton = within(table).getByRole("button", { name: /Cancelar adiantamento de/ });
    expect(cancelButton).toBeInTheDocument();

    const partialRow = within(table).getByText("Bruno Lima").closest("tr") as HTMLElement;
    expect(within(partialRow).queryByRole("button", { name: /Cancelar adiantamento de/ })).not.toBeInTheDocument();
    // A linha aberta COM abatimento explica por que não há ação.
    expect(within(partialRow).getByText("já tem abatimento")).toBeInTheDocument();
  });

  it("filtra por corretor e por status recarregando com os parâmetros escolhidos", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await within(await screen.findByRole("table")).findByText("Ana Souza");

    await user.selectOptions(screen.getByLabelText("Corretor"), "broker-2");
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ broker_id: "broker-2" })));

    await user.selectOptions(screen.getByLabelText("Status"), "SETTLED");
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ status: "SETTLED" })));

    await user.click(screen.getByRole("button", { name: "Limpar filtros" }));
    await waitFor(() =>
      expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ broker_id: undefined, status: undefined })),
    );
  });
});

describe("AdiantamentosTab · novo adiantamento", () => {
  it("registra o adiantamento pelo painel lateral e recarrega a lista", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    create.mockResolvedValue(openAdvance);
    renderTab();
    await screen.findByRole("table");

    await user.click(screen.getByRole("button", { name: /Novo adiantamento/ }));
    const panel = await screen.findByRole("dialog", { name: "Novo adiantamento" });

    await user.selectOptions(within(panel).getByLabelText("Corretor"), "broker-2");
    await user.type(within(panel).getByLabelText("Valor do adiantamento"), "150000");
    await user.type(within(panel).getByLabelText("Descrição do adiantamento"), "pedido do corretor");
    await user.click(within(panel).getByRole("button", { name: /Registrar adiantamento/ }));

    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(
        expect.objectContaining({
          broker_id: "broker-2",
          amount: 1500,
          description: "pedido do corretor",
        }),
      ),
    );
    // A lista recarrega para o saldo a recuperar acompanhar.
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(1));
  });

  it("não envia sem corretor e sem valor, explicando o que falta", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await screen.findByRole("table");

    await user.click(screen.getByRole("button", { name: /Novo adiantamento/ }));
    const panel = await screen.findByRole("dialog", { name: "Novo adiantamento" });
    await user.click(within(panel).getByRole("button", { name: /Registrar adiantamento/ }));

    expect(create).not.toHaveBeenCalled();
    expect(await within(panel).findByText(/Selecione o corretor que recebeu o adiantamento/)).toBeInTheDocument();
  });
});

describe("AdiantamentosTab · cancelamento", () => {
  it("cancela com confirmação no painel, enviando o motivo", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    cancel.mockResolvedValue({ ...openAdvance, status: "CANCELLED" });
    renderTab();

    await user.click(await screen.findByRole("button", { name: /Cancelar adiantamento de/ }));

    const panel = await screen.findByRole("dialog", { name: "Cancelar adiantamento" });
    // O painel de confirmação repete o que será cancelado antes de confirmar.
    expect(within(panel).getByText("Ana Souza")).toBeInTheDocument();
    // Valor e saldo: nada foi abatido ainda.
    expect(within(panel).getAllByText("R$ 2.000,00")).toHaveLength(2);

    await user.type(within(panel).getByLabelText("Motivo do cancelamento"), "lançado em duplicidade");
    await user.click(within(panel).getByRole("button", { name: /^Cancelar adiantamento$/ }));

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("adv-1", "lançado em duplicidade"));
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(1));
  });

  it("mostra a recusa do servidor quando o adiantamento já tem abatimento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    cancel.mockRejectedValue({
      response: { data: { message: "adiantamento já tem abatimentos em comissões: desfaça os descontos antes de cancelar" } },
    });
    renderTab();

    await user.click(await screen.findByRole("button", { name: /Cancelar adiantamento de/ }));
    const panel = await screen.findByRole("dialog", { name: "Cancelar adiantamento" });
    await user.click(within(panel).getByRole("button", { name: /^Cancelar adiantamento$/ }));

    expect(
      await within(panel).findByText(/adiantamento já tem abatimentos em comissões/i),
    ).toBeInTheDocument();
  });
});

describe("AdiantamentosTab · estado vazio", () => {
  it("explica que o saldo é recuperado lançando o desconto de Adiantamento na comissão", async () => {
    list.mockResolvedValue({ items: [], balances: [] });
    renderTab();

    expect(await screen.findByText(/Nenhum adiantamento registrado/i)).toBeInTheDocument();
    expect(screen.getByText(/desconto do tipo/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Abrir comissões" });
    expect(link).toHaveAttribute("href", "/financeiro/comissoes");
    expect(screen.getAllByRole("button", { name: /Novo adiantamento/ }).length).toBeGreaterThan(0);
  });

  it("distingue o filtro sem resultado do tenant sem adiantamento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    list.mockResolvedValue({ items: [], balances: [] });
    renderTab();

    await screen.findByText(/Nenhum adiantamento registrado/i);
    await user.selectOptions(screen.getByLabelText("Status"), "CANCELLED");

    expect(await screen.findByText(/Nenhum adiantamento neste filtro/i)).toBeInTheDocument();
  });
});

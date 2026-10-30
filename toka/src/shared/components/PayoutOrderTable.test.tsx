import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { payoutOrderService, type PayoutOrder, type PayoutStatus } from "../services/payoutOrder.service";
import { PayoutOrderTable } from "./PayoutOrderTable";
import { PermissionsContext } from "../context/PermissionsContext";

vi.mock("../services/payoutOrder.service", () => ({
  payoutOrderService: {
    list: vi.fn(),
    group: vi.fn(),
    approve: vi.fn(),
  },
}));

const order = (id: string, status: PayoutStatus): PayoutOrder => ({
  id,
  tenant_id: "tenant-1",
  source_type: "manual",
  source_id: `source-${id}`,
  recipient_type: "external",
  recipient_id: `recipient-${id}`,
  recipient_label: `Pessoa ${id}`,
  amount_cents: 10_000,
  currency: "BRL",
  status,
  funding_required_cents: 0,
  funded_amount_cents: 0,
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
});

const items = [
  order("pendente", "PENDING_REVIEW"),
  order("aprovada", "APPROVED"),
  order("paga", "PAID"),
  order("rascunho", "DRAFT"),
];

const renderWithPermissions = () => render(
  <PermissionsContext.Provider value={{
    functionalRole: "financeiro",
    baseRole: "",
    hasTenantWideScope: false,
    profile: "member",
    permissions: new Set(["finance:read", "finance:write", "finance:approve_transfer"]),
    can: () => true,
    isLoading: false,
    refresh: vi.fn(),
  }}>
    <PayoutOrderTable />
  </PermissionsContext.Provider>,
);

describe("PayoutOrderTable", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(payoutOrderService.list).mockResolvedValue({ items, total: items.length });
    vi.mocked(payoutOrderService.group).mockResolvedValue({ group_id: "group-12345678", updated: items.slice(0, 2) });
    vi.mocked(payoutOrderService.approve).mockResolvedValue(order("pendente", "APPROVED"));
  });

  it("só permite selecionar ordens abertas e cobertas", async () => {
    renderWithPermissions();

    expect(await screen.findByLabelText("Selecionar ordem de Pessoa pendente")).toBeEnabled();
    expect(screen.getByLabelText("Selecionar ordem de Pessoa aprovada")).toBeEnabled();
    expect(screen.getAllByLabelText("Ordem encerrada ou indisponível para ação em lote")).toHaveLength(2);
    screen.getAllByLabelText("Ordem encerrada ou indisponível para ação em lote").forEach((checkbox) => {
      expect(checkbox).toBeDisabled();
    });
  });

  it("confirma o agrupamento e deixa explícito que os registros serão preservados", async () => {
    const user = userEvent.setup();
    renderWithPermissions();

    await user.click(await screen.findByLabelText("Selecionar ordem de Pessoa pendente"));
    await user.click(screen.getByLabelText("Selecionar ordem de Pessoa aprovada"));
    await user.click(screen.getByRole("button", { name: /Agrupar selecionadas/i }));

    expect(screen.getByRole("heading", { name: "Agrupar ordens selecionadas" })).toBeInTheDocument();
    expect(screen.getByText(/registros individuais continuarão preservados/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirmar agrupamento" }));

    await waitFor(() => expect(payoutOrderService.group).toHaveBeenCalledWith(["pendente", "aprovada"]));
  });
});

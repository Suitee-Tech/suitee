import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ContractStatus } from "../../contracts/types";

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("../../services/transaction.service", () => ({
  transactionService: { create: mocks.create },
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.success, error: mocks.error },
}));

import { NovaTransacaoModal } from "./NovaTransacaoModal";

beforeAll(() => {
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

describe("NovaTransacaoModal — relações financeiras", () => {
  beforeEach(() => {
    mocks.create.mockReset().mockResolvedValue({ id: "tx-1" });
    mocks.success.mockReset();
    mocks.error.mockReset();
  });

  it("sugere cliente e contrato e envia a categoria orientada pela direção", async () => {
    const onClose = vi.fn();
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(
      <NovaTransacaoModal
        direction="entrada"
        onClose={onClose}
        onCreated={vi.fn()}
        contracts={[{
          id: "c1",
          client: "Maria",
          customer_id: "t1",
          property_id: "p1",
          property_title: "Apt 101",
          status: ContractStatus.ACTIVE,
          content: "",
          signatories: [],
          start_date: "2026-01-01",
          created_at: "2026-01-01",
          updated_at: "2026-01-01",
          tenant_id: "company-1",
        }]}
        properties={[{
          id: "p1",
          title: "Apt 101",
          address: {} as never,
          price: 0,
          type: "Apartamento",
          status: "Em Contrato",
          beds: 0,
          baths: 0,
          area: 0,
          image: "",
        }]}
      />,
    );

    const textboxes = screen.getAllByRole("textbox");
    fireEvent.change(textboxes[0], { target: { value: "Aluguel recebido" } });
    fireEvent.change(textboxes[1], { target: { value: "150000" } });

    await user.click(screen.getByRole("combobox", { name: "Contrato" }));
    await user.click(screen.getByRole("option", { name: /Apt 101 — Maria/i }));

    await user.click(screen.getByRole("combobox", { name: "Tipo de conta / categoria" }));
    await user.click(screen.getByRole("option", { name: "Receita de aluguel" }));

    expect(screen.getByRole("combobox", { name: "Entidade relacionada" })).toHaveTextContent(/Cliente \/ inquilino · Maria/);
    fireEvent.click(screen.getByRole("button", { name: /registrar entrada/i }));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledTimes(1));
    expect(mocks.create.mock.calls[0][0]).toMatchObject({
      contract_id: "c1",
      category: "Receita de aluguel",
      payer_name: "Maria",
      payer_kind: "tenant",
      value: 1500,
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

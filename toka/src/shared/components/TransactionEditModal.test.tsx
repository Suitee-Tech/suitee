import { describe, it, expect, vi, beforeAll, beforeEach } from "vitest";
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { Transaction } from "../types/transaction";
import { TransactionEditModal } from "./TransactionEditModal";

const { updateTransactionMock, toastSuccessMock } = vi.hoisted(() => ({
  updateTransactionMock: vi.fn(),
  toastSuccessMock: vi.fn(),
}));
vi.mock("../context/FinanceContext", () => ({
  useFinanceContext: () => ({ updateTransaction: updateTransactionMock }),
}));
vi.mock("../context/ContractsContext", () => ({
  useContractsContext: () => ({ contracts: [] }),
}));
vi.mock("sonner", () => ({
  toast: {
    success: toastSuccessMock,
    error: vi.fn(),
  },
}));

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

beforeEach(() => {
  updateTransactionMock.mockReset();
  updateTransactionMock.mockResolvedValue(true);
  toastSuccessMock.mockReset();
});

const tx = (over: Partial<Transaction>) =>
  ({ id: "tx", contractId: "c1", value: 0, status: "Pendente", description: "", date: "", ...over }) as unknown as Transaction;

describe("TransactionEditModal", () => {
  it("cobranca: salva o valor mascarado e zera pagamento quando não está Pago", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(
      <TransactionEditModal
        mode="cobranca"
        transaction={tx({ id: "tx1", value: 1500, status: "Pendente", description: "Aluguel", category: "Aluguel" })}
        onClose={vi.fn()}
      />,
    );

    expect((screen.getByLabelText(/valor/i) as HTMLInputElement).value).toMatch(/1\.500,00/);
    await user.click(screen.getByRole("button", { name: /salvar altera/i }));

    await waitFor(() => expect(updateTransactionMock).toHaveBeenCalledTimes(1));
    const [id, updates] = updateTransactionMock.mock.calls[0];
    expect(id).toBe("tx1");
    expect(updates.value).toBe(1500);
    expect(updates.status).toBe("Pendente");
    expect(updates.paymentMethod).toBeNull();
  });

  it("repasse: valor líquido = bruto − deduções", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(
      <TransactionEditModal
        mode="repasse"
        transaction={tx({ id: "tx2", value: 1000, grossValue: 2000, description: "Repasse" })}
        onClose={vi.fn()}
      />,
    );

    const money = screen.getAllByPlaceholderText("R$ 0,00") as HTMLInputElement[];
    expect(money).toHaveLength(3); // taxa admin, manutenção, consumo
    fireEvent.change(money[0], { target: { value: "20000" } }); // R$ 200,00
    fireEvent.change(money[1], { target: { value: "5000" } }); // R$ 50,00

    await user.click(screen.getByRole("button", { name: /salvar altera/i }));

    await waitFor(() => expect(updateTransactionMock).toHaveBeenCalledTimes(1));
    const [, updates] = updateTransactionMock.mock.calls[0];
    expect(updates.grossValue).toBe(2000);
    expect(updates.adminFee).toBe(200);
    expect(updates.value).toBe(1750); // 2000 − 200 − 50 − 0
  });

  it("aguarda a atualização terminar antes de fechar e sinalizar sucesso", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onClose = vi.fn();
    let resolveUpdate!: (applied: boolean) => void;
    updateTransactionMock.mockImplementationOnce(
      () =>
        new Promise<boolean>((resolve) => {
          resolveUpdate = resolve;
        }),
    );

    render(
      <TransactionEditModal
        mode="cobranca"
        transaction={tx({ id: "tx3", value: 900, description: "Aluguel" })}
        onClose={onClose}
      />,
    );

    await user.click(screen.getByRole("button", { name: /salvar altera/i }));

    await waitFor(() => expect(updateTransactionMock).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("button", { name: /salvando/i })).toBeDisabled();

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 350));
    });

    expect(onClose).not.toHaveBeenCalled();
    expect(toastSuccessMock).not.toHaveBeenCalled();

    await act(async () => {
      resolveUpdate(true);
    });

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(toastSuccessMock).toHaveBeenCalledTimes(1);
  });

  it("permanece aberto e não sinaliza sucesso quando a atualização falha", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onClose = vi.fn();
    let rejectUpdate!: (error: Error) => void;
    updateTransactionMock.mockImplementationOnce(
      () =>
        new Promise<boolean>((_resolve, reject) => {
          rejectUpdate = reject;
        }),
    );

    render(
      <TransactionEditModal
        mode="cobranca"
        transaction={tx({ id: "tx4", value: 750, description: "IPTU" })}
        onClose={onClose}
      />,
    );

    await user.click(screen.getByRole("button", { name: /salvar altera/i }));
    await waitFor(() => expect(updateTransactionMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      rejectUpdate(new Error("falha ao atualizar"));
    });

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /salvar altera/i })).toBeEnabled(),
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(toastSuccessMock).not.toHaveBeenCalled();
  });

  it("não fecha quando a operação pertence a um tenant que deixou de ser ativo", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onClose = vi.fn();
    updateTransactionMock.mockResolvedValueOnce(false);

    render(
      <TransactionEditModal
        mode="cobranca"
        transaction={tx({ id: "tx5", value: 500, description: "Condomínio" })}
        onClose={onClose}
      />,
    );

    await user.click(screen.getByRole("button", { name: /salvar altera/i }));
    await waitFor(() => expect(updateTransactionMock).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /salvar altera/i })).toBeEnabled(),
    );

    expect(onClose).not.toHaveBeenCalled();
    expect(toastSuccessMock).not.toHaveBeenCalled();
  });
});

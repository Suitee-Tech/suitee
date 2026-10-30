import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mocks = vi.hoisted(() => ({
  createBatch: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("../../hooks/useEsteiraDeals", () => ({
  useEsteiraDeals: () => ({ deals: [] }),
}));

vi.mock("../../context/ContractsContext", () => ({
  useContractsContext: () => ({ contracts: [] }),
}));

vi.mock("../../context/AgentsContext", () => ({
  useAgentsContext: () => ({ agents: [] }),
}));

vi.mock("../../services/commission.service", () => ({
  commissionService: { createBatch: mocks.createBatch },
}));

vi.mock("sonner", () => ({
  toast: { error: mocks.toastError, success: mocks.toastSuccess },
}));

import { NovaComissaoModal } from "./NovaComissaoModal";

beforeAll(() => {
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  proto.hasPointerCapture ??= vi.fn(() => false);
  proto.setPointerCapture ??= vi.fn();
  proto.releasePointerCapture ??= vi.fn();
});

const fillValidForm = (amountDigits = "100000") => {
  fireEvent.change(screen.getByPlaceholderText("ID do negócio (deal_id)"), {
    target: { value: "deal-1" },
  });
  fireEvent.change(screen.getByPlaceholderText("ID do corretor (broker_id)"), {
    target: { value: "broker-1" },
  });
  fireEvent.change(screen.getByPlaceholderText("0,00"), {
    target: { value: amountDigits },
  });
};

const submit = async () => {
  const user = userEvent.setup({ pointerEventsCheck: 0 });
  await user.click(screen.getByRole("button", { name: "Criar Comissão" }));
};

describe("NovaComissaoModal — retry idempotente", () => {
  beforeEach(() => {
    mocks.createBatch.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("reutiliza a mesma chave ao reenviar o mesmo payload após erro", async () => {
    mocks.createBatch
      .mockRejectedValueOnce(new Error("timeout"))
      .mockResolvedValueOnce([{ id: "commission-1" }]);
    const onCreated = vi.fn();
    const onClose = vi.fn();
    render(<NovaComissaoModal onCreated={onCreated} onClose={onClose} />);
    fillValidForm();

    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByRole("button", { name: "Criar Comissão" })).toBeEnabled());
    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(2));

    const firstKey = mocks.createBatch.mock.calls[0][1];
    const retryKey = mocks.createBatch.mock.calls[1][1];
    expect(firstKey).toMatch(/^manual-commission-batch:/);
    expect(retryKey).toBe(firstKey);
    expect(onCreated).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("gera outra chave quando o payload muda depois do erro", async () => {
    mocks.createBatch
      .mockRejectedValueOnce(new Error("timeout"))
      .mockResolvedValueOnce([{ id: "commission-1" }]);
    render(<NovaComissaoModal onCreated={vi.fn()} onClose={vi.fn()} />);
    fillValidForm();

    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByRole("button", { name: "Criar Comissão" })).toBeEnabled());
    fireEvent.change(screen.getByPlaceholderText("0,00"), { target: { value: "200000" } });
    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(2));

    expect(mocks.createBatch.mock.calls[1][0].splits[0].amount).toBe(2_000);
    expect(mocks.createBatch.mock.calls[1][1]).not.toBe(mocks.createBatch.mock.calls[0][1]);
  });

  it("descarta a chave depois do sucesso", async () => {
    mocks.createBatch.mockResolvedValue([{ id: "commission-1" }]);
    render(<NovaComissaoModal onCreated={vi.fn()} onClose={vi.fn()} />);
    fillValidForm();

    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByRole("button", { name: "Criar Comissão" })).toBeEnabled());
    await submit();
    await waitFor(() => expect(mocks.createBatch).toHaveBeenCalledTimes(2));

    expect(mocks.createBatch.mock.calls[1][1]).not.toBe(mocks.createBatch.mock.calls[0][1]);
  });

  it("impede percentual acima de 100% antes de criar o lote", async () => {
    render(<NovaComissaoModal onCreated={vi.fn()} onClose={vi.fn()} />);
    fillValidForm();
    fireEvent.change(screen.getByPlaceholderText("0"), { target: { value: "120" } });

    await submit();

    expect(mocks.createBatch).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith(expect.stringMatching(/menor ou igual a 100%/i));
  });
});

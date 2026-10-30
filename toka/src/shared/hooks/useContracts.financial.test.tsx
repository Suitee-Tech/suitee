import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { chargeService } from "../services/charge.service";
import { transferService } from "../services/transfer.service";
import { useMarkChargePaidMutation, useProcessTransferMutation } from "./useContracts";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), message: vi.fn() },
}));

vi.mock("../services/charge.service", () => ({
  chargeService: {
    markPaid: vi.fn(),
    create: vi.fn(),
  },
}));

vi.mock("../services/transfer.service", () => ({
  transferService: {
    create: vi.fn(),
    approve: vi.fn(),
    process: vi.fn(),
  },
}));

const setup = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, invalidate, wrapper };
};

describe("financial contract mutations", () => {
  beforeEach(() => vi.clearAllMocks());

  it("encaminha comprovante e observação ao processar o repasse", async () => {
    vi.mocked(transferService.process).mockResolvedValue({ data: { id: "transfer-1" } } as never);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useProcessTransferMutation("contract-1"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        transferId: "transfer-1",
        evidence: { proof_url: "https://proof.example/transfer.pdf", note: "PIX conferido" },
      });
    });

    expect(transferService.process).toHaveBeenCalledWith("transfer-1", {
      proof_url: "https://proof.example/transfer.pdf",
      note: "PIX conferido",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["contracts", "contract-1", "transfers"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["lancamentos", "transfers"] });
  });

  it("atualiza cobranças, repasses e obrigações após registrar recebimento", async () => {
    vi.mocked(chargeService.markPaid).mockResolvedValue({ data: { id: "charge-1" } } as never);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useMarkChargePaidMutation("contract-1"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        chargeId: "charge-1",
        data: { payment_method: "pix", paid_amount: 900, paid_at: "2026-09-07T12:00:00.000Z" },
      });
    });

    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["contracts", "contract-1", "charges"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["contracts", "contract-1", "transfers"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["lancamentos", "charges"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["lancamentos", "transfers"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["lancamentos", "commissions"] });
  });
});

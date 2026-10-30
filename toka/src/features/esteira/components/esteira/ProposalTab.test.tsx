import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { EsteiraDeal } from "../../../../shared/deals/types";
import type { DealDTO } from "../../../../shared/services/deal.service";
import { ProposalTab } from "./ProposalTab";

const mocks = vi.hoisted(() => ({
  addProposal: vi.fn(),
  acceptCounterOffer: vi.fn(),
  notifyApiError: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("../../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    addProposal: mocks.addProposal,
    acceptCounterOffer: mocks.acceptCounterOffer,
  }),
}));

vi.mock("../../../../shared/services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

vi.mock("../../../../shared/ws/useWsSubscription", () => ({
  useWsSubscription: vi.fn(),
}));

vi.mock("../../../../lib/permissions/PermissionsContext", () => ({
  usePermissionsContext: () => ({ can: () => true }),
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function makeContract(id = "deal-1"): EsteiraDeal {
  return {
    id,
    client: "Fernando Pessoa",
    property: "Casa Vila Mariana",
    type: "Locação",
    value: 3_200,
    stage: "Proposta",
    date: "2026-07-28",
    avatarUrl: "",
  };
}

function makeDeal({
  id = "deal-1",
  counterOffer = false,
}: {
  id?: string;
  counterOffer?: boolean;
} = {}): DealDTO {
  return {
    id,
    tenant_id: "tenant-1",
    type: "RENT",
    status: "PROPOSAL",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "customer-1",
    broker_id: "broker-1",
    value: 3_200,
    commission: 0,
    created_at: "2026-07-28T12:00:00.000Z",
    updated_at: "2026-07-28T12:00:00.000Z",
    proposals: counterOffer
      ? [
          {
            id: "proposal-1",
            value: 3_200,
            counter_value: 3_100,
            payment_method: "À Vista",
            status: "COUNTER_OFFER",
            created_at: "2026-07-28T12:00:00.000Z",
            created_by: "user-1",
          },
        ]
      : [],
    parties: [],
    signers: [],
  };
}

describe("ProposalTab async state", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("blocks duplicate proposal submissions and relies on the canonical mutation result", async () => {
    const user = userEvent.setup();
    const request = deferred<DealDTO | null>();
    const onRefresh = vi.fn();
    mocks.addProposal.mockReturnValueOnce(request.promise);

    render(
      <ProposalTab
        contract={makeContract()}
        dealDetails={makeDeal()}
        onRefresh={onRefresh}
        onGoToTab={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Nova Proposta/i }));
    await user.click(screen.getByRole("button", { name: /Salvar Proposta/i }));

    expect(mocks.addProposal).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Salvando…" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Salvando…" }));
    expect(mocks.addProposal).toHaveBeenCalledTimes(1);

    await act(async () => {
      request.resolve(makeDeal());
      await request.promise;
    });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Salvando…" })).not.toBeInTheDocument();
    });
    expect(screen.queryByText("Forma de pagamento")).not.toBeInTheDocument();
    expect(onRefresh).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Proposta adicionada com sucesso!");
  }, 10_000);

  it("keeps the proposal form open and reports a failed save", async () => {
    const user = userEvent.setup();
    const error = new Error("network error");
    mocks.addProposal.mockRejectedValueOnce(error);

    render(
      <ProposalTab
        contract={makeContract()}
        dealDetails={makeDeal()}
        onRefresh={vi.fn()}
        onGoToTab={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Nova Proposta/i }));
    await user.click(screen.getByRole("button", { name: /Salvar Proposta/i }));

    await waitFor(() => {
      expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
        context: "Adicionar proposta",
      });
    });
    expect(screen.getByText("Forma de pagamento")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Salvar Proposta/i })).toBeEnabled();
  });

  it("writes an accepted counteroffer through DealsContext without a second refresh", async () => {
    const user = userEvent.setup();
    const request = deferred<DealDTO | null>();
    const onRefresh = vi.fn();
    mocks.acceptCounterOffer.mockReturnValueOnce(request.promise);

    render(
      <ProposalTab
        contract={makeContract()}
        dealDetails={makeDeal({ counterOffer: true })}
        onRefresh={onRefresh}
        onGoToTab={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Aceitar Contraproposta/i }));
    expect(mocks.acceptCounterOffer).toHaveBeenCalledWith("deal-1", "proposal-1");
    expect(screen.getByRole("button", { name: "Aceitando…" })).toBeDisabled();

    await act(async () => {
      request.resolve(makeDeal());
      await request.promise;
    });

    await waitFor(() => {
      expect(mocks.toastSuccess).toHaveBeenCalledWith(
        "Contraproposta aceita. Negociação atualizada.",
      );
    });
    expect(onRefresh).not.toHaveBeenCalled();
  });

  it("ignores completion from a proposal request after switching deals", async () => {
    const user = userEvent.setup();
    const request = deferred<DealDTO | null>();
    mocks.addProposal.mockReturnValueOnce(request.promise);
    const view = render(
      <ProposalTab
        contract={makeContract("deal-1")}
        dealDetails={makeDeal({ id: "deal-1" })}
        onRefresh={vi.fn()}
        onGoToTab={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Nova Proposta/i }));
    await user.click(screen.getByRole("button", { name: /Salvar Proposta/i }));

    view.rerender(
      <ProposalTab
        contract={makeContract("deal-2")}
        dealDetails={makeDeal({ id: "deal-2" })}
        onRefresh={vi.fn()}
        onGoToTab={vi.fn()}
      />,
    );

    await act(async () => {
      request.resolve(makeDeal({ id: "deal-1" }));
      await request.promise;
    });

    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(screen.queryByText("Forma de pagamento")).not.toBeInTheDocument();
  });
});

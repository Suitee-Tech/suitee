import { act, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type { DealDTO, PartyDTO } from "../../../shared/services/deal.service";
import { PartiesPanel } from "./PartiesPanel";

const mocks = vi.hoisted(() => ({
  updateParty: vi.fn(),
  notifyApiError: vi.fn(),
  addParty: vi.fn(),
  removeParty: vi.fn(),
  setPartyPrimary: vi.fn(),
  updatePartyRole: vi.fn(),
  inviteSigner: vi.fn(),
  addChecklistItem: vi.fn(),
}));

vi.mock("../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    updateParty: mocks.updateParty,
    addParty: mocks.addParty,
    removeParty: mocks.removeParty,
    setPartyPrimary: mocks.setPartyPrimary,
    updatePartyRole: mocks.updatePartyRole,
    inviteSigner: mocks.inviteSigner,
    addChecklistItem: mocks.addChecklistItem,
  }),
}));

vi.mock("../../../shared/context/LeadsContext", () => ({
  useLeadsContext: () => ({ leads: [] }),
}));

vi.mock("../../../shared/services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

vi.mock("../../../shared/components/AddPartyDialog", () => ({
  AddPartyDialog: () => null,
}));

vi.mock("../../../shared/components/EditPartyDialog", () => ({
  EditPartyDialog: () => null,
}));

vi.mock("../../../shared/components/PartyRepresentationPanel", () => ({
  PartyRepresentationPanel: () => null,
}));

vi.mock("../../../shared/components/ConfirmDialog", () => ({
  ConfirmDialog: () => null,
}));

vi.mock("../../../shared/ui/FieldWithProvenance", () => ({
  FieldProvenanceIndicator: () => null,
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

const baseParty: PartyDTO = {
  id: "party-1",
  name: "Fernando Pessoa",
  email: "fernando@example.com",
  role: "tenant",
  is_primary: true,
  created_at: "2026-07-28T12:00:00.000Z",
  updated_at: "2026-07-28T12:00:00.000Z",
};

function dealWithParty(party: PartyDTO = baseParty, dealId = "deal-1"): DealDTO {
  return {
    id: dealId,
    type: "RENT",
    status: "PROPOSAL",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "lead-1",
    broker_id: "broker-1",
    value: 3200,
    commission: 0,
    tenant_id: "tenant-1",
    created_at: "2026-07-28T12:00:00.000Z",
    updated_at: "2026-07-28T12:00:00.000Z",
    parties: [party],
    signers: [],
  };
}

async function selectNoSpouseAction() {
  const user = userEvent.setup({ pointerEventsCheck: 0 });
  await user.click(screen.getByRole("button", { name: "Mais ações" }));
  await user.click(await screen.findByText("Marcar sem cônjuge"));
}

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

describe("PartiesPanel spouse declaration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("mostra loading em voo e só concretiza após a confirmação (commit-on-confirm)", async () => {
    const request = deferred<DealDTO>();
    mocks.updateParty.mockReturnValueOnce(request.promise);
    const onRefresh = vi.fn();
    const view = render(<MemoryRouter><PartiesPanel deal={dealWithParty()} onRefresh={onRefresh} /></MemoryRouter>);

    await selectNoSpouseAction();

    expect(mocks.updateParty).toHaveBeenCalledTimes(1);
    expect(mocks.updateParty).toHaveBeenCalledWith("deal-1", "party-1", {
      no_spouse_declared: true,
    });
    // Em voo: loading/disabled e o estado definitivo NÃO muda.
    expect(screen.getByText("Atualizando cônjuge...")).toBeInTheDocument();
    expect(screen.queryByText("Sem cônjuge declarado")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mais ações" })).toBeDisabled();

    await act(async () => {
      request.resolve(
        dealWithParty({
          ...baseParty,
          no_spouse_declared: true,
          updated_at: "2026-07-28T12:01:00.000Z",
        }),
      );
      await request.promise;
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Mais ações" })).toBeEnabled();
    });

    // No app real o painel re-renderiza com o deal autoritativo (cache
    // atualizado pela mutação) — aqui simulamos com o valor persistido.
    view.rerender(
      <MemoryRouter>
        <PartiesPanel
          deal={dealWithParty({
            ...baseParty,
            no_spouse_declared: true,
          })}
          onRefresh={onRefresh}
        />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("Sem cônjuge declarado")).toBeInTheDocument();
    });
    expect(mocks.updateParty).toHaveBeenCalledTimes(1);
    expect(onRefresh).not.toHaveBeenCalled();
  }, 10_000);

  it("rolls the optimistic state back and reports the error when persistence fails", async () => {
    const request = deferred<DealDTO>();
    mocks.updateParty.mockReturnValueOnce(request.promise);
    render(<MemoryRouter><PartiesPanel deal={dealWithParty()} onRefresh={vi.fn()} /></MemoryRouter>);

    await selectNoSpouseAction();
    expect(screen.getByText("Atualizando cônjuge...")).toBeInTheDocument();

    const error = new Error("network error");
    await act(async () => {
      request.reject(error);
      await expect(request.promise).rejects.toThrow("network error");
    });

    await waitFor(() => {
      expect(screen.getByText("Cônjuge pendente")).toBeInTheDocument();
    });
    expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
      context: "Atualizar declaração de cônjuge",
    });
    expect(screen.getByRole("button", { name: "Mais ações" })).toBeEnabled();
  });

  it("reflete o deal autoritativo quando o cache é atualizado com o valor persistido", async () => {
    const request = deferred<DealDTO>();
    mocks.updateParty.mockReturnValueOnce(request.promise);
    const view = render(<MemoryRouter><PartiesPanel deal={dealWithParty()} onRefresh={vi.fn()} /></MemoryRouter>);

    await selectNoSpouseAction();
    // Em voo: loading substitui o valor definitivo (que ainda não mudou).
    expect(screen.getByText("Atualizando cônjuge...")).toBeInTheDocument();
    expect(screen.queryByText("Cônjuge pendente")).not.toBeInTheDocument();

    await act(async () => {
      request.resolve(
        dealWithParty({
          ...baseParty,
          no_spouse_declared: true,
          updated_at: "2026-07-28T12:01:00.000Z",
        }),
      );
      await request.promise;
    });

    // Cache atualizado com o valor persistido → o painel re-renderiza com ele.
    view.rerender(
      <MemoryRouter>
        <PartiesPanel
          deal={dealWithParty({
            ...baseParty,
            no_spouse_declared: true,
          })}
          onRefresh={vi.fn()}
        />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("Sem cônjuge declarado")).toBeInTheDocument();
    });

    view.rerender(
      <MemoryRouter>
        <PartiesPanel
          deal={dealWithParty({
            ...baseParty,
            no_spouse_declared: false,
          })}
          onRefresh={vi.fn()}
        />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("Cônjuge pendente")).toBeInTheDocument();
    });
  });

  it("ignores completion from the previous deal without clearing the new deal request", async () => {
    const previousDealRequest = deferred<DealDTO>();
    const currentDealRequest = deferred<DealDTO>();
    mocks.updateParty
      .mockReturnValueOnce(previousDealRequest.promise)
      .mockReturnValueOnce(currentDealRequest.promise);

    const view = render(<MemoryRouter><PartiesPanel deal={dealWithParty()} onRefresh={vi.fn()} /></MemoryRouter>);
    await selectNoSpouseAction();
    expect(screen.getByText("Atualizando cônjuge...")).toBeInTheDocument();

    view.rerender(
      <MemoryRouter>
        <PartiesPanel
          deal={dealWithParty(baseParty, "deal-2")}
          onRefresh={vi.fn()}
        />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("Cônjuge pendente")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Mais ações" })).toBeEnabled();
    });

    await selectNoSpouseAction();
    expect(mocks.updateParty).toHaveBeenNthCalledWith(2, "deal-2", "party-1", {
      no_spouse_declared: true,
    });

    const staleError = new Error("stale request failed");
    await act(async () => {
      previousDealRequest.reject(staleError);
      await expect(previousDealRequest.promise).rejects.toThrow("stale request failed");
    });

    expect(mocks.notifyApiError).not.toHaveBeenCalled();
    expect(screen.getByText("Atualizando cônjuge...")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mais ações" })).toBeDisabled();

    await act(async () => {
      currentDealRequest.resolve(
        dealWithParty(
          {
            ...baseParty,
            no_spouse_declared: true,
            updated_at: "2026-07-28T12:03:00.000Z",
          },
          "deal-2",
        ),
      );
      await currentDealRequest.promise;
    });

    // Cache atualizado com o valor persistido → re-renderiza com o deal-2.
    view.rerender(
      <MemoryRouter>
        <PartiesPanel
          deal={dealWithParty(
            {
              ...baseParty,
              no_spouse_declared: true,
              updated_at: "2026-07-28T12:03:00.000Z",
            },
            "deal-2",
          )}
          onRefresh={vi.fn()}
        />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getByText("Sem cônjuge declarado")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Mais ações" })).toBeEnabled();
    });
  }, 10_000);
});

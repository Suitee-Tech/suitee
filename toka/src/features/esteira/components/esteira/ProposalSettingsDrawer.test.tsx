import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { DealDTO } from "../../../../shared/services/deal.service";
import { ProposalSettingsDrawer } from "./ProposalSettingsDrawer";

const mocks = vi.hoisted(() => ({
  updateParty: vi.fn(),
  updateSigner: vi.fn(),
  notifyApiError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("../../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    updateParty: mocks.updateParty,
    updateSigner: mocks.updateSigner,
  }),
}));

vi.mock("../../../../shared/services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
  },
}));

vi.mock("../../../../shared/components/ExpandableSidePanel", () => ({
  ExpandableSidePanel: ({
    open,
    children,
    renderHeader,
  }: {
    open: boolean;
    children: ReactNode;
    renderHeader?: (props: {
      closeButton: ReactNode;
      expandButton: ReactNode;
      isExpanded: boolean;
      onClose: () => void;
      onToggleExpand: () => void;
    }) => ReactNode;
  }) => {
    if (!open) return null;
    return (
      <div>
        {renderHeader?.({
          closeButton: <button type="button">Fechar</button>,
          expandButton: null,
          isExpanded: false,
          onClose: vi.fn(),
          onToggleExpand: vi.fn(),
        })}
        {children}
      </div>
    );
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

function makeDeal({
  dealId = "deal-1",
  partyRequired = false,
  signerRequired = true,
}: {
  dealId?: string;
  partyRequired?: boolean;
  signerRequired?: boolean;
} = {}): DealDTO {
  return {
    id: dealId,
    tenant_id: "tenant-1",
    type: "RENT",
    status: "PROPOSAL",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "customer-1",
    broker_id: "broker-1",
    value: 3200,
    commission: 0,
    created_at: "2026-07-28T12:00:00.000Z",
    updated_at: "2026-07-28T12:00:00.000Z",
    parties: [
      {
        id: "party-1",
        name: "Fernando Pessoa",
        email: "fernando@example.com",
        role: "tenant",
        is_primary: true,
        signature_required: partyRequired,
        created_at: "2026-07-28T12:00:00.000Z",
        updated_at: "2026-07-28T12:00:00.000Z",
      },
    ],
    signers: [
      {
        id: "signer-1",
        name: "Maria do Carmo",
        email: "maria@example.com",
        role: "locador",
        status: "PENDING",
        is_required: signerRequired,
      },
    ],
  };
}

describe("ProposalSettingsDrawer requirement toggles (commit-on-confirm)", () => {
  beforeEach(() => {
    mocks.updateParty.mockReset();
    mocks.updateSigner.mockReset();
    mocks.notifyApiError.mockReset();
    mocks.toastSuccess.mockReset();
  });

  it("só muda o aceite quando o valor é confirmado (nunca otimista)", async () => {
    const user = userEvent.setup();
    const pending = deferred<DealDTO | null>();
    mocks.updateParty.mockReturnValueOnce(pending.promise);
    const staleDeal = makeDeal({ partyRequired: false });

    const { rerender } = render(
      <ProposalSettingsDrawer open onClose={vi.fn()} deal={staleDeal} />,
    );

    const partyToggle = screen.getByRole("switch", {
      name: "Aceite obrigatório para Fernando Pessoa",
    });

    expect(partyToggle).toHaveAttribute("aria-checked", "false");
    await user.click(partyToggle);

    expect(mocks.updateParty).toHaveBeenCalledWith(
      "deal-1",
      "party-1",
      { signature_required: true },
    );
    // Em voo: loading/disabled e O ESTADO DEFINITIVO NÃO MUDA.
    expect(partyToggle).toHaveAttribute("aria-checked", "false");
    expect(partyToggle).toBeDisabled();
    expect(partyToggle).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("Salvando")).toBeInTheDocument();

    // Mesmo com o cache/prop ainda entregando o deal anterior, o valor permanece.
    rerender(<ProposalSettingsDrawer open onClose={vi.fn()} deal={staleDeal} />);
    expect(partyToggle).toHaveAttribute("aria-checked", "false");

    await act(async () => {
      pending.resolve(makeDeal({ partyRequired: true }));
      await pending.promise;
    });

    await waitFor(() => expect(partyToggle).not.toBeDisabled());
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Aceite marcado como obrigatório.");

    // Só quando o deal autoritativo carrega o valor persistido o toggle expõe.
    rerender(
      <ProposalSettingsDrawer
        open
        onClose={vi.fn()}
        deal={makeDeal({ partyRequired: true })}
      />,
    );
    await waitFor(() => expect(partyToggle).toHaveAttribute("aria-checked", "true"));
  });

  it("mantém o valor anterior e libera o toggle quando a gravação falha", async () => {
    const user = userEvent.setup();
    const pending = deferred<DealDTO | null>();
    const error = new Error("falha ao salvar");
    mocks.updateParty.mockReturnValueOnce(pending.promise);

    render(<ProposalSettingsDrawer open onClose={vi.fn()} deal={makeDeal()} />);

    const partyToggle = screen.getByRole("switch", {
      name: "Aceite obrigatório para Fernando Pessoa",
    });
    await user.click(partyToggle);
    // Em voo: ainda no valor anterior, mas em loading.
    expect(partyToggle).toHaveAttribute("aria-checked", "false");
    expect(partyToggle).toBeDisabled();

    await act(async () => {
      pending.reject(error);
      await pending.promise.catch(() => undefined);
    });

    await waitFor(() => expect(partyToggle).not.toBeDisabled());
    expect(partyToggle).toHaveAttribute("aria-checked", "false");
    expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
      context: "Atualizar parte",
    });
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });

  it("só muda a assinatura quando o valor é confirmado", async () => {
    const user = userEvent.setup();
    const pending = deferred<DealDTO | null>();
    mocks.updateSigner.mockReturnValueOnce(pending.promise);
    const staleDeal = makeDeal({ signerRequired: true });

    const { rerender } = render(
      <ProposalSettingsDrawer open onClose={vi.fn()} deal={staleDeal} />,
    );

    const signerToggle = screen.getByRole("switch", {
      name: "Assinatura obrigatória para Maria do Carmo",
    });
    await user.click(signerToggle);

    expect(mocks.updateSigner).toHaveBeenCalledWith(
      "deal-1",
      "signer-1",
      { is_required: false },
    );
    // Em voo: ainda no valor anterior (obrigatório), loading/disabled.
    expect(signerToggle).toHaveAttribute("aria-checked", "true");
    expect(signerToggle).toBeDisabled();

    await act(async () => {
      pending.resolve(makeDeal({ signerRequired: false }));
      await pending.promise;
    });

    await waitFor(() => expect(signerToggle).not.toBeDisabled());
    expect(signerToggle).toHaveAttribute("aria-checked", "true"); // prop ainda antigo
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Assinatura marcada como opcional.");

    rerender(
      <ProposalSettingsDrawer
        open
        onClose={vi.fn()}
        deal={makeDeal({ signerRequired: false })}
      />,
    );
    await waitFor(() => expect(signerToggle).toHaveAttribute("aria-checked", "false"));
  });

  it("reverte a assinatura quando a API rejeita a alteração", async () => {
    const user = userEvent.setup();
    const pending = deferred<DealDTO | null>();
    const error = new Error("assinatura não atualizada");
    mocks.updateSigner.mockReturnValueOnce(pending.promise);

    render(
      <ProposalSettingsDrawer
        open
        onClose={vi.fn()}
        deal={makeDeal({ signerRequired: true })}
      />,
    );

    const signerToggle = screen.getByRole("switch", {
      name: "Assinatura obrigatória para Maria do Carmo",
    });
    await user.click(signerToggle);
    expect(signerToggle).toHaveAttribute("aria-checked", "true");

    await act(async () => {
      pending.reject(error);
      await pending.promise.catch(() => undefined);
    });

    await waitFor(() => expect(signerToggle).not.toBeDisabled());
    expect(signerToggle).toHaveAttribute("aria-checked", "true");
    expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
      context: "Atualizar assinatura",
    });
  });

  it("ignora conclusões do negócio anterior após trocar o deal do drawer", async () => {
    const user = userEvent.setup();
    const pendingParty = deferred<DealDTO | null>();
    mocks.updateParty.mockReturnValueOnce(pendingParty.promise);

    const { rerender } = render(
      <ProposalSettingsDrawer
        open
        onClose={vi.fn()}
        deal={makeDeal({ dealId: "deal-anterior" })}
      />,
    );

    const partyToggle = screen.getByRole("switch", {
      name: "Aceite obrigatório para Fernando Pessoa",
    });

    await user.click(partyToggle);
    expect(partyToggle).toBeDisabled();

    rerender(
      <ProposalSettingsDrawer
        open
        onClose={vi.fn()}
        deal={makeDeal({ dealId: "deal-atual" })}
      />,
    );

    await waitFor(() => {
      expect(partyToggle).not.toBeDisabled();
    });
    expect(partyToggle).toHaveAttribute("aria-checked", "false");

    await act(async () => {
      pendingParty.resolve(
        makeDeal({ dealId: "deal-anterior", partyRequired: true }),
      );
      await Promise.allSettled([pendingParty.promise]);
    });

    expect(partyToggle).toHaveAttribute("aria-checked", "false");
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(mocks.notifyApiError).not.toHaveBeenCalled();
  });
});

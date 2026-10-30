import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

import { proofService } from "../services/proof.service";
import { MarkPaidCommissionModal } from "./MarkPaidCommissionModal";

// A política de comprovante vem do backend via React Query; aqui é fixada para
// o teste não precisar de QueryClientProvider e para deixar explícito o cenário
// (exigir comprovante) que a maioria dos casos deste arquivo assume.
vi.mock("../hooks/useProofPolicy", () => ({
  useProofPolicy: () => ({ proofRequired: true, loading: false }),
}));

vi.mock("../services/proof.service", () => ({
  proofService: {
    upload: vi.fn(),
  },
}));

/**
 * Captura o contrato de fechamento do painel. Escape, clique fora e o X passam
 * todos pelo mesmo `onClose` do `ExpandableSidePanel` (o `FinancialActionPanel`
 * repassa para lá o guard do fluxo); interceptá-lo exercita o guard exatamente
 * onde ele é decidido, sem depender de foco sintético que o jsdom não reproduz.
 */
const panelCapture: { onClose?: () => void } = {};

vi.mock("./ExpandableSidePanel", async () => {
  const actual = await vi.importActual<typeof import("./ExpandableSidePanel")>("./ExpandableSidePanel");
  return {
    ...actual,
    // Mantém o painel real (o Content depende do provider do Radix) e apenas
    // registra o onClose para exercitar o guard de fechamento.
    ExpandableSidePanel: (props: { onClose: () => void } & Record<string, unknown>) => {
      panelCapture.onClose = props.onClose;
      const RealPanel = actual.ExpandableSidePanel as unknown as (p: Record<string, unknown>) => ReactNode;
      return RealPanel(props);
    },
  };
});

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
  vi.clearAllMocks();
});

describe("MarkPaidCommissionModal", () => {
  it("requires evidence when resolving a paid commission without proof", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);

    render(
      <MarkPaidCommissionModal
        mode="attach-proof"
        commissionId="commission-1"
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    expect(await screen.findByText(/anexe um arquivo ou informe a URL/i)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("uploads a proof file and saves its URL", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    vi.mocked(proofService.upload).mockResolvedValue({
      url: "https://files.example/commission-proof.pdf",
      filename: "commission-proof.pdf",
      size_bytes: 1024,
      expires_at: "2027-08-29T00:00:00Z",
    });

    render(
      <MarkPaidCommissionModal
        mode="attach-proof"
        commissionId="commission-2"
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    const file = new File(["proof"], "commission-proof.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    await waitFor(() => {
      expect(proofService.upload).toHaveBeenCalledWith(file, "commission");
      expect(onSave).toHaveBeenCalledWith("commission-2", {
        proof_url: "https://files.example/commission-proof.pdf",
        note: undefined,
      });
    });
  });

  it("keeps the selected file and allows retry when storage is unavailable", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    vi.mocked(proofService.upload)
      .mockRejectedValueOnce({ response: { status: 503 } })
      .mockResolvedValueOnce({
        url: "https://files.example/commission-proof.pdf",
        filename: "commission-proof.pdf",
        size_bytes: 1024,
        expires_at: "2027-08-29T00:00:00Z",
      });

    render(
      <MarkPaidCommissionModal
        mode="attach-proof"
        commissionId="commission-retry"
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    const file = new File(["%PDF-1.4\n%%EOF"], "proof.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/temporariamente indisponível/i);
    expect(screen.getByText("proof.pdf")).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    await waitFor(() => {
      expect(proofService.upload).toHaveBeenCalledTimes(2);
      expect(onSave).toHaveBeenCalledWith("commission-retry", {
        proof_url: "https://files.example/commission-proof.pdf",
        note: undefined,
      });
    });
  });

  // ── FIN-05/N-04: a confirmação precisa mostrar os centavos ──
  it("mostra o líquido da comissão com duas casas decimais", () => {
    render(
      <MarkPaidCommissionModal
        commissionId="commission-cents"
        brokerLabel="Bruno"
        netAmount={100.49}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    // R$ 100,49 não pode virar "R$ 100" na linha de líquido.
    expect(screen.getByText(/R\$ 100,49/)).toBeInTheDocument();
  });

  // ── FIN-08: falha da gravação não é falha do armazenamento ──
  it("atribui a falha da gravação à etapa financeira, não ao armazenamento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockRejectedValue({ response: { status: 500 } });
    vi.mocked(proofService.upload).mockResolvedValue({
      url: "https://files.example/commission-proof.pdf",
      filename: "commission-proof.pdf",
      size_bytes: 1024,
      expires_at: "2027-08-29T00:00:00Z",
    });

    render(
      <MarkPaidCommissionModal
        mode="attach-proof"
        commissionId="commission-mutation"
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    const file = new File(["proof"], "commission-proof.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    // O upload concluiu: um 500 da mutação não é indisponibilidade do storage.
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/registro financeiro não pôde ser concluído/i);
    expect(alert).not.toHaveTextContent(/armazenamento de comprovantes/i);
  });

  // ── FIN-06: não descartar o modal durante o envio ──
  it("não fecha pelo painel enquanto o envio do comprovante está em curso", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    let resolveUpload:
      | ((value: { url: string; filename: string; size_bytes: number; expires_at: string }) => void)
      | undefined;
    vi.mocked(proofService.upload).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveUpload = resolve;
        }),
    );

    render(
      <MarkPaidCommissionModal
        mode="attach-proof"
        commissionId="commission-guard"
        onClose={onClose}
        onSave={onSave}
      />,
    );

    const file = new File(["proof"], "commission-proof.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /anexar e resolver/i }));

    // Envio em andamento: o guard tem de cobrir a etapa de upload, não só a
    // gravação — senão Escape/clique fora/X descartam a operação em curso.
    await waitFor(() => expect(proofService.upload).toHaveBeenCalledTimes(1));
    // Sem o ponto de fechamento capturado o teste passaria em falso: o guard só
    // é exercitado se o painel realmente tiver sido montado.
    expect(panelCapture.onClose).toBeTypeOf("function");
    act(() => { panelCapture.onClose?.(); });
    expect(onClose).not.toHaveBeenCalled();
    // O painel continua aberto com o próprio título (nome acessível do diálogo).
    expect(screen.getByRole("dialog", { name: /resolver: comprovante ausente/i })).toBeInTheDocument();

    // Conclui o envio para não deixar promessa pendente ao fim do teste.
    await act(async () => {
      resolveUpload?.({
        url: "https://files.example/commission-proof.pdf",
        filename: "commission-proof.pdf",
        size_bytes: 1024,
        expires_at: "2027-08-29T00:00:00Z",
      });
    });
  });
});

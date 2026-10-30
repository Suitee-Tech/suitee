import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ProcessRepasseModal } from "./ProcessRepasseModal";
import type { OwnerTransfer } from "../services/transfer.service";

// A política de comprovante vem do backend via React Query; aqui é fixada para
// o teste não precisar de QueryClientProvider e para deixar explícito o cenário
// (exigir comprovante) que a maioria dos casos deste arquivo assume.
vi.mock("../hooks/useProofPolicy", () => ({
  useProofPolicy: () => ({ proofRequired: true, loading: false }),
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

const baseTransfer: OwnerTransfer = {
  id: "t1",
  tenant_id: "tenant-1",
  contract_id: "c1",
  owner_id: "owner-1",
  property_id: "p1",
  reference_month: "2026-09",
  gross_rent: 3750,
  net_amount: 3450,
  status: "APPROVED",
} as OwnerTransfer;

function renderModal(overrides: Partial<OwnerTransfer> = {}, props: { onProcess?: () => Promise<void>; onClose?: () => void } = {}) {
  const onClose = props.onClose ?? vi.fn();
  const onProcess = (props.onProcess ?? vi.fn().mockResolvedValue(undefined)) as unknown as (
    transferId: string,
    evidence: { proof_url?: string; note?: string },
  ) => Promise<void>;
  const result = render(
    <ProcessRepasseModal transferId="t1" transfer={{ ...baseTransfer, ...overrides }} onClose={onClose} onProcess={onProcess} />,
  );
  return { ...result, onClose };
}

/**
 * O painel expõe o título duas vezes no DOM: o `h2` sr-only que o Radix usa para
 * nomear o diálogo e o `h2` visível do cabeçalho. O contrato do shell é "painel
 * aberto, com esse nome acessível, e cabeçalho visível".
 */
const visibleTitle = (name: string) =>
  screen.getAllByRole("heading", { name }).find((node) => !node.className.includes("sr-only"));
const panel = () => screen.getByRole("dialog", { name: "Processar Repasse" });

/**
 * O fechamento do painel (X, Escape e clique fora) converge no mesmo pedido de
 * fechamento passado ao shell. O X é o ponto determinístico no jsdom — o Escape
 * do Radix depende de foco real, que o jsdom não reproduz — então é por ele que
 * o guard é exercitado.
 */
const closePanel = () => fireEvent.click(screen.getByRole("button", { name: /Fechar painel/i }));

describe("ProcessRepasseModal", () => {
  it("renderiza o painel com upload de comprovante e observação", () => {
    renderModal();
    expect(panel()).toBeInTheDocument();
    expect(visibleTitle("Processar Repasse")).toBeTruthy();
    expect(screen.getByText(/Arquivo do comprovante/i)).toBeInTheDocument();
    expect(screen.getByText(/URL do comprovante/i)).toBeInTheDocument();
    expect(screen.getAllByText(/Observação/i).length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: /Confirmar Repasse/i })).toBeInTheDocument();
  });

  it("submete o comprovante/observação ao processar o repasse", async () => {
    const onProcess = vi.fn().mockResolvedValue(undefined);
    render(
      <ProcessRepasseModal transferId="t1" transfer={baseTransfer} onClose={vi.fn()} onProcess={onProcess} />,
    );

    fireEvent.change(screen.getByLabelText(/Observação/i), { target: { value: "TED enviado 14:30" } });
    fireEvent.change(screen.getByLabelText(/URL do comprovante/i), { target: { value: "https://s3.example/recibo.pdf" } });

    fireEvent.click(screen.getByRole("button", { name: /Confirmar Repasse/i }));

    await waitFor(() => expect(onProcess).toHaveBeenCalledTimes(1));
    expect(onProcess).toHaveBeenCalledWith("t1", {
      proof_url: "https://s3.example/recibo.pdf",
      note: "TED enviado 14:30",
    });
  });

  it("permite anexar um arquivo pelo input de comprovante", () => {
    renderModal();
    // O input é o da seção compartilhada de comprovante (ProofAttachmentFields).
    const fileInput = screen.getByLabelText(/Selecionar PDF, JPG ou PNG/i) as HTMLInputElement;
    expect(fileInput).toBeTruthy();
    expect(fileInput.getAttribute("accept")).toBe(".pdf,.jpg,.jpeg,.png");
  });

  it("não conclui o repasse sem comprovante", async () => {
    const onProcess = vi.fn();
    render(
      <ProcessRepasseModal transferId="t1" transfer={baseTransfer} onClose={vi.fn()} onProcess={onProcess} />,
    );

    fireEvent.click(screen.getByRole("button", { name: /Confirmar Repasse/i }));

    expect(await screen.findByText(/Anexe um comprovante ou informe a URL/i)).toBeInTheDocument();
    expect(onProcess).not.toHaveBeenCalled();
  });

  // ── FIN-05: a confirmação precisa mostrar centavos ──
  it("mostra o líquido com duas casas decimais", () => {
    renderModal({ net_amount: 100.49 });

    // R$ 100,49 não pode virar "R$ 100".
    expect(screen.getByText("R$ 100,49")).toBeInTheDocument();
    expect(screen.queryByText("R$ 100")).not.toBeInTheDocument();
  });

  it("distingue 100,49 de 100,50 na confirmação", () => {
    const { unmount } = renderModal({ net_amount: 100.49 });
    expect(screen.getByText("R$ 100,49")).toBeInTheDocument();
    unmount();

    renderModal({ net_amount: 100.5 });
    expect(screen.getByText("R$ 100,50")).toBeInTheDocument();
  });

  // ── FIN-06: não descartar o painel durante o envio ──
  it("não fecha o painel enquanto o processamento está em curso", async () => {
    const onClose = vi.fn();
    renderModal({}, { onProcess: () => new Promise<void>(() => {}), onClose });

    fireEvent.change(screen.getByLabelText(/URL do comprovante/i), { target: { value: "https://s3.example/recibo.pdf" } });
    fireEvent.click(screen.getByRole("button", { name: /Confirmar Repasse/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /Fechar painel/i })).toBeInTheDocument());

    // Pedido de fechamento (X / clique fora / Escape) durante a operação.
    closePanel();
    expect(onClose).not.toHaveBeenCalled();
    expect(panel()).toBeInTheDocument();
    expect(visibleTitle("Processar Repasse")).toBeTruthy();
  });

  it("repassa o pedido de fechamento quando nada está em curso", () => {
    const onClose = vi.fn();
    renderModal({}, { onClose });

    closePanel();
    expect(onClose).toHaveBeenCalled();
  });

  it("desabilita Cancelar durante o processamento", async () => {
    let resolveProcess: (() => void) | undefined;
    const onProcess = vi.fn(
      () => new Promise<void>((resolve) => { resolveProcess = resolve; }),
    );
    render(
      <ProcessRepasseModal transferId="t1" transfer={baseTransfer} onClose={vi.fn()} onProcess={onProcess} />,
    );

    fireEvent.change(screen.getByLabelText(/URL do comprovante/i), { target: { value: "https://s3.example/recibo.pdf" } });
    fireEvent.click(screen.getByRole("button", { name: /Confirmar Repasse/i }));

    await waitFor(() => expect(onProcess).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: /Cancelar/i })).toBeDisabled();

    resolveProcess?.();
  });

  // ── FIN-08: falha da gravação não é falha do armazenamento ──
  it("atribui a falha da gravação financeira à etapa correta", async () => {
    renderModal({}, { onProcess: () => Promise.reject({ response: { status: 500 } }) });

    fireEvent.change(screen.getByLabelText(/URL do comprovante/i), { target: { value: "https://s3.example/recibo.pdf" } });
    fireEvent.click(screen.getByRole("button", { name: /Confirmar Repasse/i }));

    // Um 500 na mutação não é indisponibilidade do armazenamento.
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/registro financeiro não pôde ser concluído/i);
    expect(alert).not.toHaveTextContent(/armazenamento de comprovantes/i);
  });
});

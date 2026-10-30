import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { toast } from "sonner";
import { proofService } from "../../services/proof.service";
import { transactionService } from "../../services/transaction.service";
import { SettleLancamentoModal, type SettleLancamentoTarget } from "./SettleLancamentoModal";

// A política de comprovante vem do backend via React Query. Aqui é fixada para o
// teste não precisar de QueryClientProvider e é configurável por caso: os testes
// de payload rodam com a exigência desligada (para exercitar os opcionais) e o
// teste de política a liga explicitamente.
const proofPolicy = { proofRequired: true };
vi.mock("../../hooks/useProofPolicy", () => ({
  useProofPolicy: () => ({ proofRequired: proofPolicy.proofRequired, loading: false }),
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog usa.
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

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
  // Default do arquivo: exigir comprovante. Casos que precisam do contrário
  // (campos opcionais) desligam dentro do próprio teste.
  proofPolicy.proofRequired = true;
});

const baseLancamento: SettleLancamentoTarget = {
  id: "lanc-1",
  type: "receita",
  description: "Aluguel 09/2026",
  value: 100.49,
};

function renderModal(lancamento: SettleLancamentoTarget = baseLancamento) {
  const onClose = vi.fn();
  const onSaved = vi.fn();
  render(<SettleLancamentoModal lancamento={lancamento} onClose={onClose} onSaved={onSaved} />);
  return { onClose, onSaved };
}

describe("SettleLancamentoModal", () => {
  it("usa o shell padrão do painel financeiro, com seções nomeadas e rodapé fixo", () => {
    renderModal();

    // Mesmo painel lateral dos demais fluxos: nome acessível = título do cabeçalho.
    expect(screen.getByRole("dialog", { name: "Baixar Entrada" })).toBeInTheDocument();
    expect(screen.getByText("Registra o pagamento e o movimento no caixa")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Lançamento" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Dados da baixa" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Comprovante" })).toBeInTheDocument();

    // Rodapé fixo: Cancelar + ação primária.
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Confirmar baixa/ })).toBeInTheDocument();

    // Seção padrão de comprovante (arquivo, link e observação).
    expect(screen.getByText(/Arquivo do comprovante/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/URL do comprovante/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Observação/i)).toBeInTheDocument();
  });

  it("mostra o alvo da baixa com duas casas decimais", () => {
    renderModal();

    expect(screen.getByText("Aluguel 09/2026")).toBeInTheDocument();
    // R$ 100,49 não pode virar "R$ 100" no resumo do que será baixado.
    expect(screen.getByText("R$ 100,49")).toBeInTheDocument();
    expect(screen.queryByText("R$ 100")).not.toBeInTheDocument();
  });

  it("preserva o payload da baixa (data ISO, método, opcionais undefined)", async () => {
    // Cenário com a exigência de comprovante desligada: o objetivo é provar que
    // os campos opcionais não são inventados quando não informados.
    proofPolicy.proofRequired = false;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const settle = vi.spyOn(transactionService, "settle").mockResolvedValue({} as never);
    const { onClose, onSaved } = renderModal();

    await user.click(screen.getByRole("button", { name: /Confirmar baixa/i }));

    await waitFor(() => expect(settle).toHaveBeenCalledTimes(1));
    expect(settle).toHaveBeenCalledWith("lanc-1", {
      payment_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T/),
      payment_method: "pix",
      proof_url: undefined,
      note: undefined,
    });
    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("Entrada baixada.");
  });

  it("antecipa a exigência de comprovante do tenant antes de chamar o serviço", async () => {
    proofPolicy.proofRequired = true;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const settle = vi.spyOn(transactionService, "settle").mockResolvedValue({} as never);
    renderModal();

    await user.click(screen.getByRole("button", { name: /Confirmar baixa/i }));

    // O backend recusaria com 400; a interface antecipa a exigência.
    expect(await screen.findByRole("alert")).toHaveTextContent(/Anexe um comprovante ou informe a URL/i);
    expect(settle).not.toHaveBeenCalled();
  });

  it("bloqueia a baixa com URL de comprovante inválida", async () => {
    proofPolicy.proofRequired = false;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const settle = vi.spyOn(transactionService, "settle").mockResolvedValue({} as never);
    renderModal();

    await user.type(screen.getByLabelText(/URL do comprovante/i), "not-a-url");
    await user.click(screen.getByRole("button", { name: /Confirmar baixa/i }));

    expect(await screen.findByText(/URL deve começar com http/i)).toBeInTheDocument();
    expect(settle).not.toHaveBeenCalled();
  });

  it("reenvia a baixa sem subir o comprovante de novo e separa a falha da gravação da do armazenamento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const settle = vi.spyOn(transactionService, "settle").mockRejectedValue({ response: { status: 500 } });
    const upload = vi.spyOn(proofService, "upload").mockResolvedValue({
      url: "https://files.example/recibo.pdf",
      filename: "recibo.pdf",
      size_bytes: 1024,
      expires_at: "2027-01-01T00:00:00Z",
    });
    renderModal();

    const file = new File(["proof"], "recibo.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/Selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /Confirmar baixa/i }));

    await waitFor(() => expect(settle).toHaveBeenCalledTimes(1));
    expect(upload).toHaveBeenCalledTimes(1);
    expect(settle.mock.calls[0][1]?.proof_url).toBe("https://files.example/recibo.pdf");

    // FIN-08: um 500 da baixa não é indisponibilidade do armazenamento.
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/registro financeiro não pôde ser concluído/i);
    expect(alert).not.toHaveTextContent(/armazenamento de comprovantes/i);

    // Nova tentativa: o arquivo já está guardado e não sobe de novo (N-06).
    await user.click(screen.getByRole("button", { name: /Confirmar baixa/i }));
    await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
    expect(upload).toHaveBeenCalledTimes(1);
    expect(settle.mock.calls[1][1]?.proof_url).toBe("https://files.example/recibo.pdf");
  });

  it("fecha pelo rodapé sem salvar", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const settle = vi.spyOn(transactionService, "settle").mockResolvedValue({} as never);
    const { onClose, onSaved } = renderModal();

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onSaved).not.toHaveBeenCalled();
    expect(settle).not.toHaveBeenCalled();
  });
});

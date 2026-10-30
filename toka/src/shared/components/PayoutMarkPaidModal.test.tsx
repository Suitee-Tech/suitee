import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { proofService } from "../services/proof.service";
import { PayoutMarkPaidModal } from "./PayoutMarkPaidModal";

// A política de comprovante vem do backend via React Query. Aqui é fixada para o
// teste não precisar de QueryClientProvider e é configurável por caso.
const proofPolicy = { proofRequired: true };
vi.mock("../hooks/useProofPolicy", () => ({
  useProofPolicy: () => ({ proofRequired: proofPolicy.proofRequired, loading: false }),
}));

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
  proofPolicy.proofRequired = true;
});

const uploadedProof = {
  url: "https://files.example/payout-proof.pdf",
  filename: "payout-proof.pdf",
  size_bytes: 1024,
  expires_at: "2027-08-29T00:00:00Z",
};

const uploadProof = () => vi.spyOn(proofService, "upload").mockResolvedValue(uploadedProof);

const renderModal = (overrides: Partial<Parameters<typeof PayoutMarkPaidModal>[0]> = {}) => {
  const props: Parameters<typeof PayoutMarkPaidModal>[0] = {
    open: true,
    payoutId: "payout-1",
    payoutLabel: "Corretor Ana",
    onClose: vi.fn(),
    onConfirm: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return {
    ...render(<PayoutMarkPaidModal {...props} />),
    onClose: vi.mocked(props.onClose),
    onConfirm: vi.mocked(props.onConfirm),
  };
};

const attachFile = async (user: ReturnType<typeof userEvent.setup>) => {
  const file = new File(["proof"], "payout-proof.pdf", { type: "application/pdf" });
  await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
};

describe("PayoutMarkPaidModal", () => {
  it("usa o painel padrão e identifica a ordem no cabeçalho", () => {
    renderModal();

    expect(screen.getByRole("dialog", { name: "Marcar ordem como paga" })).toBeInTheDocument();
    // O título visível é o `h2` do cabeçalho padrão (o nome acessível do painel
    // vem do título sr-only exigido pelo Radix).
    const headings = screen.getAllByRole("heading", { name: "Marcar ordem como paga" });
    expect(
      headings.some((node) => node.tagName === "H2" && !node.className.includes("sr-only")),
    ).toBe(true);
    expect(screen.getByText("Ordem para Corretor Ana")).toBeInTheDocument();
    // O comprovante vem da seção compartilhada (arquivo + link + observação).
    expect(screen.getByLabelText(/url do comprovante/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/observação/i)).toBeInTheDocument();
  });

  it("não renderiza nada quando open é false (painel é sempre-aberto por design)", () => {
    const { container } = renderModal({ open: false });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(container).toBeEmptyDOMElement();
  });

  it("envia o comprovante anexado no payload de confirmação", async () => {
    proofPolicy.proofRequired = false;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const upload = uploadProof();
    const { onClose, onConfirm } = renderModal();

    await attachFile(user);
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    // O mesmo upload do serviço de comprovantes, no purpose do fluxo de payout.
    expect(upload).toHaveBeenCalledWith(expect.any(File), "payout");
    expect(onConfirm).toHaveBeenCalledWith({
      method: "MANUAL_PIX",
      proofUrl: uploadedProof.url,
      proofFilename: uploadedProof.filename,
      note: "",
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("mantém o mesmo payload de observação digitada e limpa o formulário após confirmar", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    uploadProof();
    const { onClose, onConfirm } = renderModal();

    await attachFile(user);
    await user.type(screen.getByLabelText(/observação/i), "PIX enviado ao Itaú 0001/12345-6");
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm.mock.calls[0][0].note).toBe("PIX enviado ao Itaú 0001/12345-6");

    // Confirmado: a próxima abertura começa sem comprovante nem observação.
    await waitFor(() =>
      expect(screen.getByLabelText(/observação/i)).toHaveValue(""),
    );
    expect(screen.getByText(/selecionar PDF/i)).toBeInTheDocument();
  });

  it("impede a baixa sem comprovante e mostra o erro no painel", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onClose, onConfirm } = renderModal();

    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/anexe o comprovante antes de confirmar/i);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("bloqueia o submit e valida URL de comprovante inválida", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onClose, onConfirm } = renderModal();

    await user.type(screen.getByLabelText(/url do comprovante/i), "not-a-url");
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    expect(await screen.findByText(/URL deve começar com http/i)).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("aceita o link do comprovante quando não há arquivo anexado", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const upload = uploadProof();
    const { onClose, onConfirm } = renderModal();

    await user.type(
      screen.getByLabelText(/url do comprovante/i),
      "https://drive.google.com/comprovante.pdf",
    );
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    // Sem arquivo, o link digitado é a evidência: nada é enviado ao storage.
    expect(upload).not.toHaveBeenCalled();
    expect(onConfirm.mock.calls[0][0]).toMatchObject({
      proofUrl: "https://drive.google.com/comprovante.pdf",
      proofFilename: "",
    });
  });

  // ── FIN-08: falha da gravação não é falha do armazenamento ──
  it("atribui a falha da gravação à etapa financeira, não ao armazenamento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    uploadProof();
    const rejectConfirm = vi.fn().mockRejectedValue({ response: { status: 500 } });
    const { onClose, onConfirm } = renderModal({ onConfirm: rejectConfirm });

    await attachFile(user);
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/registro financeiro não pôde ser concluído/i);
    expect(alert).not.toHaveTextContent(/armazenamento de comprovantes/i);
    // A ordem continua aberta para nova tentativa.
    expect(onClose).not.toHaveBeenCalled();
  });

  // ── FIN-08 (lado do storage): o arquivo permanece para nova tentativa ──
  it("atribui a falha do envio ao armazenamento e mantém o arquivo selecionado", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    vi.spyOn(proofService, "upload").mockRejectedValue({ response: { status: 503 } });
    const { onClose, onConfirm } = renderModal();

    await attachFile(user);
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/armazenamento de comprovantes/i);
    expect(onConfirm).not.toHaveBeenCalled();
    // O arquivo anexado não é descartado por uma falha de armazenamento.
    expect(screen.getByText("payout-proof.pdf")).toBeInTheDocument();
  });

  it("fecha pelo Cancelar sem confirmar nada", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onClose, onConfirm } = renderModal();

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("não fecha o painel no meio do envio (X, overlay e Esc ficam inertes)", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    let release: (value: typeof uploadedProof) => void = () => {};
    vi.spyOn(proofService, "upload").mockImplementation(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    const { onClose, onConfirm } = renderModal();

    await attachFile(user);
    await user.click(screen.getByRole("button", { name: /confirmar pagamento/i }));

    // Envio em curso: fechar descartaria a operação inteira.
    await user.click(screen.getByRole("button", { name: "Fechar painel" }));
    expect(onClose).not.toHaveBeenCalled();

    release(uploadedProof);
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalled();
  });
});

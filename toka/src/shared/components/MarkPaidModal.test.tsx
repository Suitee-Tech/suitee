import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { proofService } from "../services/proof.service";
import type { Charge } from "../services/charge.service";
import { MarkPaidModal } from "./MarkPaidModal";

// A política de comprovante vem do backend via React Query. Aqui é fixada para
// o teste não precisar de QueryClientProvider e é configurável por caso: os
// testes de payload rodam com a exigência desligada (para exercitar os campos
// opcionais) e o teste de política a liga explicitamente.
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
  // Default do arquivo: exigir comprovante. Casos que precisam do contrário
  // (campos opcionais) desligam dentro do próprio teste.
  proofPolicy.proofRequired = true;
});

/** Cobrança mínima: sem pagamentos nem itens, o resumo só tem "Valor desta baixa". */
const baseCharge = {
  id: "charge-1",
  tenant_id: "tenant-1",
  contract_id: "contract-1",
  tenant_party_id: "tenant-party-1",
  owner_id: "owner-1",
  property_id: "property-1",
  type: "rent",
  description: "Aluguel 09/2026",
  reference_month: "2026-09",
  amount: 100.49,
  late_fee: 0,
  interest: 0,
  discount: 0,
  total_amount: 100.49,
  due_date: "2026-09-10",
  status: "PENDING",
} as Charge;

describe("MarkPaidModal", () => {
  it("preserva o payload no submit (valor mascarado, data ISO, opcionais undefined)", async () => {
    // Cenário com a exigência de comprovante desligada: o objetivo é provar que
    // os campos opcionais não são inventados quando não informados.
    proofPolicy.proofRequired = false;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    render(<MarkPaidModal chargeId="charge-1" defaultAmount={150} onClose={onClose} onSave={onSave} />);

    // valor pré-preenchido com a máscara de moeda
    expect((screen.getByLabelText(/valor recebido/i) as HTMLInputElement).value).toMatch(/150,00/);

    await user.click(screen.getByRole("button", { name: /confirmar recebimento/i }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [id, payload] = onSave.mock.calls[0];
    expect(id).toBe("charge-1");
    expect(payload.payment_method).toBe("pix");
    expect(payload.paid_amount).toBe(150);
    expect(payload.paid_at).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    expect(payload.proof_url).toBeUndefined();
    expect(payload.note).toBeUndefined();
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("impede a baixa sem comprovante quando a política do tenant exige", async () => {
    proofPolicy.proofRequired = true;
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<MarkPaidModal chargeId="charge-1" defaultAmount={150} onClose={vi.fn()} onSave={onSave} />);

    await user.click(screen.getByRole("button", { name: /confirmar recebimento/i }));

    // O backend recusaria com 400; a interface antecipa a exigência.
    expect(await screen.findByText(/Anexe um comprovante ou informe a URL/i)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("bloqueia o submit e valida URL de comprovante inválida", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<MarkPaidModal chargeId="c2" defaultAmount={100} onClose={vi.fn()} onSave={onSave} />);

    await user.type(screen.getByLabelText(/url do comprovante/i), "not-a-url");
    await user.click(screen.getByRole("button", { name: /confirmar recebimento/i }));

    expect(await screen.findByText(/URL deve começar com http/i)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  // ── N-04: os destinos do valor confirmado precisam mostrar centavos ──
  it("mostra os destinos do valor com duas casas decimais", () => {
    render(
      <MarkPaidModal chargeId="charge-cents" defaultAmount={100.49} charge={baseCharge} onClose={vi.fn()} onSave={vi.fn()} />,
    );

    // R$ 100,49 não pode ser arredondado para "R$ 100" na divisão do dinheiro.
    expect(screen.getByText("R$ 100,49")).toBeInTheDocument();
    expect(screen.queryByText("R$ 100")).not.toBeInTheDocument();
  });

  it("acompanha o valor digitado no campo, não a prop defaultAmount", async () => {
    render(
      <MarkPaidModal chargeId="charge-live" defaultAmount={100} charge={baseCharge} onClose={vi.fn()} onSave={vi.fn()} />,
    );

    // O operador corrige o valor recebido no próprio formulário.
    fireEvent.change(screen.getByLabelText(/valor recebido/i), { target: { value: "250,00" } });

    // O resumo é a prévia do que será gravado: não pode continuar exibindo a prop.
    await waitFor(() => expect(screen.getByText("R$ 250,00")).toBeInTheDocument());
    expect(screen.queryByText("R$ 100")).not.toBeInTheDocument();
  });

  // ── FIN-08: falha da gravação não é falha do armazenamento ──
  it("atribui a falha da gravação à etapa financeira, não ao armazenamento", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockRejectedValue({ response: { status: 500 } });
    vi.spyOn(proofService, "upload").mockResolvedValue({
      url: "https://files.example/charge-proof.pdf",
      filename: "charge-proof.pdf",
      size_bytes: 1024,
      expires_at: "2027-08-29T00:00:00Z",
    });

    render(<MarkPaidModal chargeId="charge-mutation" defaultAmount={150} onClose={vi.fn()} onSave={onSave} />);

    const file = new File(["proof"], "charge-proof.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText(/selecionar PDF/i), file);
    await user.click(screen.getByRole("button", { name: /confirmar recebimento/i }));

    // O upload concluiu: um 500 da baixa não é indisponibilidade do storage.
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/registro financeiro não pôde ser concluído/i);
    expect(alert).not.toHaveTextContent(/armazenamento de comprovantes/i);
  });
});

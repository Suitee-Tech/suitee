import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";

import { TransactionDetailSlideover } from "./TransactionDetailSlideover";

const baseTransaction = {
  id: "tx-1",
  status: "Pendente",
  valor: 1_000,
  desc: "Recebimento manual",
  contractId: "contract-1",
  data: "06/09/2026",
};

const renderSlideover = (transaction: Record<string, unknown>, overrides: Record<string, unknown> = {}) => {
  const onRegisterPayment = vi.fn();
  render(
    <TransactionDetailSlideover
      transaction={transaction}
      isExpanded={false}
      onToggleExpand={vi.fn()}
      onClose={vi.fn()}
      onRegisterPayment={onRegisterPayment}
      {...overrides}
    />,
  );
  return onRegisterPayment;
};

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

describe("TransactionDetailSlideover", () => {
  it("não oferece baixa para documento cujo vínculo está a apurar", () => {
    renderSlideover({ ...baseTransaction, _includedInTotals: false });

    expect(screen.queryByRole("button", { name: /Registrar pagamento/i })).not.toBeInTheDocument();
  });

  it("oferece baixa para recebível independente em aberto", () => {
    const onRegisterPayment = renderSlideover(baseTransaction);

    fireEvent.click(screen.getByRole("button", { name: /Registrar pagamento/i }));
    expect(onRegisterPayment).toHaveBeenCalledWith(baseTransaction);
  });

  it("permite tornar o detalhe canônico de repasse explicitamente somente leitura", () => {
    renderSlideover({
      ...baseTransaction,
      type: "repasse",
      _beneficiaryLabel: "Marisa Almeida",
      _beneficiaryKind: "Proprietária",
      _expectedDueAt: "2026-09-08",
      _settledAt: "2026-09-09T14:30:00Z",
    }, {
      title: "Detalhes do repasse",
      canRegisterPayment: false,
      operationalNotice: "A apurar: vínculo econômico ainda não conciliado.",
    });

    expect(screen.getByRole("heading", { name: "Detalhes do repasse" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/A apurar/);
    expect(screen.getByText("Beneficiário")).toBeInTheDocument();
    expect(screen.getByText("Marisa Almeida")).toBeInTheDocument();
    expect(screen.getByText("Vencimento esperado")).toBeInTheDocument();
    expect(screen.getByText("08/09/2026")).toBeInTheDocument();
    expect(screen.getByText("Liquidação")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Registrar pagamento/i })).not.toBeInTheDocument();
  });

  // FIN-07: o slide-over montava overlay artesanal com motion.div, sem papel de
  // diálogo, sem Escape e sem gestão de foco.
  it("expõe contrato de diálogo com nome acessível", async () => {
    renderSlideover(baseTransaction);
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAttribute("aria-labelledby");
    expect(screen.getByRole("heading", { name: /Detalhes da movimentação/i })).toBeInTheDocument();
    // O código do documento é descrição do diálogo, não texto solto.
    expect(dialog).toHaveAttribute("aria-describedby");
  });

  it("fecha com Escape", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onClose = vi.fn();
    renderSlideover(baseTransaction, { onClose });
    await screen.findByRole("dialog");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("não renderiza diálogo sem movimentação selecionada", () => {
    render(
      <TransactionDetailSlideover
        transaction={null}
        isExpanded={false}
        onToggleExpand={vi.fn()}
        onClose={vi.fn()}
        onRegisterPayment={vi.fn()}
      />,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("anuncia o estado do controle de expansão", () => {
    renderSlideover(baseTransaction);
    const expand = screen.getByRole("button", { name: /Expandir painel/i });
    expect(expand).toHaveAttribute("aria-expanded", "false");
  });
});

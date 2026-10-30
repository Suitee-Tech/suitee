import { describe, expect, it, vi, beforeAll, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import { CommissionDetailDrawer } from "./CommissionDetailDrawer";
import type { Commission } from "../services/commission.service";

const commission: Commission = {
  id: "commission-1",
  tenant_id: "tenant-1",
  contract_id: "contract-1",
  broker_id: "broker-1",
  role: "captador",
  amount: 4000,
  net_amount: 3200,
  status: "APPROVED",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-02T10:00:00Z",
} as Commission;

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
});

function renderDrawer(props: Partial<Parameters<typeof CommissionDetailDrawer>[0]> = {}) {
  const onClose = vi.fn();
  render(
    <MemoryRouter>
      <CommissionDetailDrawer
        commission={commission}
        onClose={onClose}
        onAction={vi.fn()}
        {...props}
      />
    </MemoryRouter>,
  );
  return { onClose };
}

/**
 * FIN-07: os drawers de detalhe financeiro eram overlays artesanais com
 * motion.div, sem papel de diálogo, aria-modal, Escape ou gestão de foco — o
 * teclado não conseguia nem fechar o painel. A migração para a primitiva
 * compartilhada (Radix via Sheet) é o que estes testes travam.
 */
describe("CommissionDetailDrawer · contrato de diálogo", () => {
  it("expõe papel de diálogo com nome acessível", async () => {
    renderDrawer();
    const dialog = await screen.findByRole("dialog");
    // Radix liga o diálogo ao título e à descrição; o foco fica contido no
    // painel e o restante da árvore recebe aria-hidden.
    expect(dialog).toHaveAttribute("aria-labelledby");
    expect(dialog).toHaveAttribute("aria-describedby");
    expect(screen.getByRole("heading", { name: /Detalhes da Comissão/i })).toBeInTheDocument();
  });

  it("fecha com Escape", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onClose } = renderDrawer();
    await screen.findByRole("dialog");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("não renderiza diálogo quando não há comissão selecionada", () => {
    render(
      <MemoryRouter>
        <CommissionDetailDrawer commission={null} onClose={vi.fn()} onAction={vi.fn()} />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("mostra a composição da comissão", async () => {
    renderDrawer({ brokerLabel: "Cauã de Souza" });
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Beneficiário")).toBeInTheDocument();
    expect(within(dialog).getByText("Cauã de Souza")).toBeInTheDocument();
    expect(within(dialog).getByText("Corretor · Captador")).toBeInTheDocument();
    expect(within(dialog).getByText("Vencimento esperado")).toBeInTheDocument();
    expect(within(dialog).getByText("Não definido")).toBeInTheDocument();
    expect(within(dialog).getByText("Liquidação")).toBeInTheDocument();
    expect(within(dialog).getByText("Pendente")).toBeInTheDocument();
    expect(screen.getByText(/Aprovada/)).toBeInTheDocument();
  });
});

/**
 * O desconto de IRRF/INSS/ISS saiu do "só o líquido aparece" para uma
 * composição explícita (Bruto → Descontos → Líquido) e o bloqueio passou a ser
 * EXPLICADO: o backend só aceita o lançamento com a comissão em `PENDING`, e o
 * formulário simplesmente sumir não dizia ao operador se era status, permissão
 * ou defeito da tela.
 */
describe("CommissionDetailDrawer · descontos", () => {
  it("mostra Bruto, Descontos e Líquido com o desconto que separa os dois", async () => {
    renderDrawer({
      commission: {
        ...commission,
        gross_amount: 4000,
        net_amount: 3200,
        deductions: [{ id: "deduction-1", kind: "IRRF", amount: 800 }],
      } as Commission,
    });
    const dialog = await screen.findByRole("dialog");

    expect(within(dialog).getByText("Bruto")).toBeInTheDocument();
    expect(within(dialog).getByText("R$ 4.000,00")).toBeInTheDocument();
    expect(within(dialog).getByText("Descontos (1)")).toBeInTheDocument();
    expect(within(dialog).getByText("−R$ 800,00")).toBeInTheDocument();
    expect(within(dialog).getByText("Líquido")).toBeInTheDocument();
    expect(within(dialog).getByText("R$ 3.200,00")).toBeInTheDocument();
  });

  it("explica por que a comissão aprovada não aceita mais desconto", async () => {
    renderDrawer();
    const dialog = await screen.findByRole("dialog");

    expect(await within(dialog).findByText("Lançamento de desconto indisponível")).toBeInTheDocument();
    expect(
      within(dialog).getByText(/já foi aprovada e o valor da ordem de pagamento está comprometido/i),
    ).toBeInTheDocument();
    expect(within(dialog).queryByText("Adicionar desconto")).not.toBeInTheDocument();
  });

  it("explica o bloqueio na comissão paga apontando o estorno como caminho", async () => {
    renderDrawer({
      commission: {
        ...commission,
        id: "commission-paid",
        status: "PAID",
        paid_at: "2026-09-10T10:00:00Z",
      } as Commission,
    });
    const dialog = await screen.findByRole("dialog");

    expect(await within(dialog).findByText("Lançamento de desconto indisponível")).toBeInTheDocument();
    expect(within(dialog).getByText(/já foi paga: o líquido saiu/i)).toBeInTheDocument();
  });

  it("libera o formulário e diz até quando dá para lançar com a comissão pendente", async () => {
    renderDrawer({
      commission: { ...commission, id: "commission-pending", status: "PENDING", net_amount: 4000 } as Commission,
    });
    const dialog = await screen.findByRole("dialog");

    expect(within(dialog).getByText("Adicionar desconto")).toBeInTheDocument();
    expect(within(dialog).getByText(/antes da aprovação da ordem de pagamento/i)).toBeInTheDocument();
    expect(within(dialog).queryByText("Lançamento de desconto indisponível")).not.toBeInTheDocument();
  });
});

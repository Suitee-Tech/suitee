import { describe, it, expect, vi, beforeAll } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// O wizard importa serviços que constroem um cliente HTTP no import. Em modo
// SALE/commit nenhum deles é chamado no mount, mas stubamos os módulos para
// isolar a UI do cliente real (e de qualquer leitura de env).
vi.mock("../services/rentalChargeConfig.service", () => ({
  rentalChargeConfigService: {
    getByContractId: vi.fn().mockResolvedValue(null),
    updateBundledItems: vi.fn(),
  },
}));
vi.mock("../services/saleBillingPlan.service", () => ({
  saleBillingPlanService: {
    getDraft: vi.fn().mockResolvedValue(null),
    create: vi.fn(),
    upsertDraft: vi.fn(),
  },
}));

import { BillingPlanWizard } from "./BillingPlanWizard";
import { saleBillingPlanService } from "../services/saleBillingPlan.service";

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog (Sheet) usa.
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

const renderSale = () =>
  render(
    <BillingPlanWizard contractId="contract-1" contractType="SALE" onClose={vi.fn()} onSaved={vi.fn()} />,
  );

describe("BillingPlanWizard — máscara de moeda nos valores da venda", () => {
  it("mascara cada valor digitado como BRL (e não despeja o evento como '[object Object]')", () => {
    renderSale();
    // Ordem no DOM (modo À vista): valor total, entrada, dação, troca, saldo.
    const [total, entrada, dacao] = screen.getAllByPlaceholderText("R$ 0,00") as HTMLInputElement[];

    fireEvent.change(total, { target: { value: "150000" } });
    fireEvent.change(entrada, { target: { value: "50000" } });
    fireEvent.change(dacao, { target: { value: "12345" } });

    expect(total.value).toMatch(/1\.500,00/);
    expect(entrada.value).toMatch(/500,00/);
    expect(dacao.value).toMatch(/123,45/);
    for (const field of [total, entrada, dacao]) {
      expect(field.value).not.toContain("object Object");
    }
  });

  it("propaga o valor mascarado para o resumo da composição", () => {
    renderSale();
    const [total] = screen.getAllByPlaceholderText("R$ 0,00") as HTMLInputElement[];

    fireEvent.change(total, { target: { value: "150000" } });

    // O resumo usa formatMoneyBR; só renderiza certo se o round-trip
    // string→parseMoneyBR→número funcionar (o bug anterior gravava o evento).
    expect(screen.getAllByText(/1\.500,00/).length).toBeGreaterThan(0);
  });

  it("associa label, obrigatoriedade e estado do modo de recebimento", () => {
    renderSale();

    expect(screen.getByLabelText(/Valor total da venda/i)).toHaveAttribute("aria-required", "true");
    expect(screen.getByRole("group", { name: "Estrutura do recebimento" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "À vista" })).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByRole("button", { name: "Parcelado" }));
    expect(screen.getByRole("button", { name: "Parcelado" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText(/Número de parcelas/i)).toHaveAttribute("aria-required", "true");
  });

  it("mascara o valor por parcela no modo Parcelado", async () => {
    // Radix Dialog marca o body com pointer-events:none; desliga a checagem do userEvent.
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderSale();

    await user.click(screen.getByRole("button", { name: /parcelado/i }));
    const parcela = screen.getByPlaceholderText("Deixe em branco para calcular") as HTMLInputElement;
    fireEvent.change(parcela, { target: { value: "25000" } });

    expect(parcela.value).toMatch(/250,00/);
    expect(parcela.value).not.toContain("object Object");
  });

  it("calcula o recebimento à vista como a parcela única e salva apenas o rascunho", async () => {
    const onSaved = vi.fn();
    const onClose = vi.fn();
    render(
      <BillingPlanWizard
        contractId=""
        contractType="SALE"
        defaultTotalAmount={980_000}
        mode="draft"
        dealId="deal-1"
        onClose={onClose}
        onSaved={onSaved}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /salvar plano financeiro/i }));

    await waitFor(() => {
      expect(saleBillingPlanService.upsertDraft).toHaveBeenCalledWith(
        "deal-1",
        expect.objectContaining({
          mode: "SINGLE",
          total_amount: 980_000,
          installment_count: 1,
          installment_amount: 980_000,
        }),
      );
    });
    expect(saleBillingPlanService.create).not.toHaveBeenCalled();
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

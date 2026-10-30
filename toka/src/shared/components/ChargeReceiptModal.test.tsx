import { useState } from "react";
import userEvent from "@testing-library/user-event";
import { render, screen, within, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Charge } from "../services/charge.service";
import { getChargeComposition } from "../utils/chargeComposition";
import { ChargeReceiptModal } from "./ChargeReceiptModal";
import * as printElementModule from "../utils/printDocument";
import { chargeService } from "../services/charge.service";

const rentalCharge: Charge = {
  id: "charge-real-rent", tenant_id: "agency", owner_id: "owner", tenant_party_id: "payer",
  contract_id: "contract", property_id: "property", type: "RENT", description: "Aluguel + Encargos",
  source_event_id: "rental.cfg.month.2026-10.r1", reference_month: "2026-10",
  amount: 4128, total_amount: 4128, late_fee: 0, interest: 0, discount: 0,
  status: "PENDING", due_date: "2026-10-10", created_at: "2026-09-18", updated_at: "2026-09-18",
  line_items: [
    { description: "Aluguel", amount_cents: 288000, source_type: "rental_base", source_id: "cfg", economic_owner_id: "owner", settlement_mode: "arrecadacao", rule_version_id: "rule" },
    { description: "Taxa de administração", amount_cents: 32000, source_type: "rental_bundled_item", source_id: "admin_fee", economic_owner_id: "agency", settlement_mode: "arrecadacao", allocation_type: "revenue", rule_version_id: "rule" },
    { description: "Condomínio", amount_cents: 65000, source_type: "rental_bundled_item", source_id: "condominio" },
    { description: "IPTU", amount_cents: 18000, source_type: "rental_bundled_item", source_id: "iptu" },
    { description: "Seguro Fiança", amount_cents: 9800, source_type: "rental_bundled_item", source_id: "seguro_fianca" },
  ],
};

describe("composição da cobrança de aluguel", () => {
  it("exibe aluguel bruto e mantém administração incluída e líquido separados do total cobrado", () => {
    render(<ChargeReceiptModal charge={rentalCharge} onClose={vi.fn()} />);
    const receipt = screen.getByRole("document");
    expect(within(within(receipt).getByText("Aluguel", { selector: "span.flex-1" }).parentElement!).getByText("R$ 3.200,00")).toBeInTheDocument();
    expect(within(within(receipt).getByText("Taxa de administração").parentElement!).getByText("R$ 320,00")).toBeInTheDocument();
    expect(within(within(receipt).getByText("Aluguel líquido do proprietário").parentElement!).getByText("R$ 2.880,00")).toBeInTheDocument();
    expect(within(receipt).getByText(/Administração deduzida do aluguel, incluída/)).toBeInTheDocument();
    expect(within(within(receipt).getByText("Total").parentElement!).getByText("R$ 4.128,00")).toBeInTheDocument();
  });

  it("preserva centavos, total e itens econômicos originais na projeção proporcional", () => {
    const proportional = { ...rentalCharge, amount: 1376, line_items: rentalCharge.line_items!.map((item, i) => ({ ...item, amount_cents: [96001, 10667, 21666, 6000, 3266][i] })) };
    const original = structuredClone(proportional);
    const composition = getChargeComposition(proportional);
    expect(composition.items[0].amountCents).toBe(106668);
    expect(composition.items.reduce((sum, item) => sum + item.amountCents, 0)).toBe(137600);
    expect(proportional).toEqual(original);
  });

  it("mantém administração explicitamente cobrada além do aluguel como item adicional", () => {
    const explicit = { ...rentalCharge, amount: 4448, line_items: rentalCharge.line_items!.map((item, index) => ({ ...item, ...(index === 0 ? { amount_cents: 320000 } : {}), ...(index === 1 ? { allocation_type: undefined } : {}) })) };
    const composition = getChargeComposition(explicit);
    expect(composition.rentalSplit).toBeUndefined();
    expect(composition.items.map((item) => item.amountCents)).toEqual([320000, 32000, 65000, 18000, 9800]);
    expect(composition.items.reduce((sum, item) => sum + item.amountCents, 0)).toBe(444800);
  });

  it.each([
    { source_event_id: "manual-charge" },
    { type: "SALE" },
    { owner_id: "agency" },
    { line_items: rentalCharge.line_items!.map((item, index) => ({ ...item, ...(index === 1 ? { rule_version_id: "another-rule" } : {}) })) },
  ])("não infere desconto de administração para composição sem origem confiável: %j", (changes) => {
    const charge = { ...rentalCharge, ...changes };
    expect(getChargeComposition(charge).rentalSplit).toBeUndefined();
  });
});


it("contém o foco do recibo e o devolve ao fechar com Escape", async () => {
  function Harness() {
    const [open, setOpen] = useState(false);
    return <><button onClick={() => setOpen(true)}>Ver recibo</button>{open && <ChargeReceiptModal charge={rentalCharge} onClose={() => setOpen(false)} />}</>;
  }
  const user = userEvent.setup();
  render(<Harness />);
  const opener = screen.getByRole("button", { name: "Ver recibo" });
  await user.click(opener);
  const dialog = screen.getByRole("dialog", { name: "Recibo de Cobrança" });
  for (let i = 0; i < 8; i++) {
    await user.tab();
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
  }
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(opener).toHaveFocus();
});

describe("identificação da imobiliária no recibo", () => {
  it("imprime o nome da imobiliária informado, sem marca fixa de mock", () => {
    render(<ChargeReceiptModal charge={rentalCharge} tenantName="Exemplo Imóveis" onClose={vi.fn()} />);

    const receipt = screen.getByRole("document");
    expect(within(receipt).getByText("Exemplo Imóveis")).toBeInTheDocument();
    expect(within(receipt).getByText(/Emitido por Exemplo Imóveis em/)).toBeInTheDocument();
    expect(within(receipt).queryByText(/Toca Imobiliária/i)).not.toBeInTheDocument();
  });

  it("cai para um rótulo neutro quando não há empresa no contexto", () => {
    render(<ChargeReceiptModal charge={rentalCharge} onClose={vi.fn()} />);

    const receipt = screen.getByRole("document");
    expect(within(receipt).getByText("Imobiliária")).toBeInTheDocument();
    expect(within(receipt).queryByText(/Toca Imobiliária/i)).not.toBeInTheDocument();
  });
});

describe("exportação do recibo", () => {
  it("imprime o recibo num documento isolado ao clicar em Salvar como PDF", async () => {
    const { printElement } = await import("../utils/printDocument");
    const spy = vi.spyOn(printElementModule, "printElement").mockImplementation(() => {});
    const user = userEvent.setup();
    render(<ChargeReceiptModal charge={rentalCharge} onClose={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Salvar como PDF" }));

    expect(spy).toHaveBeenCalledTimes(1);
    // O nó impresso é o corpo do recibo já renderizado (não `window.print()`,
    // que saía em branco por causa do posicionamento dentro do dialog).
    const node = spy.mock.calls[0][0] as HTMLElement | null;
    expect(node).not.toBeNull();
    // O nó é o CORPO do recibo (toolbar/título ficam fora da impressão).
    expect(node!.textContent).toContain("Aluguel");
    spy.mockRestore();
  });
});

describe("recibo oficial em PDF", () => {
  it("baixa o PDF do backend com o nome do recibo quando a cobrança está quitada", async () => {
    const blob = new Blob(["%PDF-1.4 recibo"], { type: "application/pdf" });
    const download = vi.spyOn(chargeService, "downloadReceipt").mockResolvedValue(blob);
    const createUrl = vi.fn(() => "blob:recibo");
    vi.stubGlobal("URL", { createObjectURL: createUrl, revokeObjectURL: vi.fn() });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const printSpy = vi.spyOn(printElementModule, "printElement").mockImplementation(() => {});

    const user = userEvent.setup();
    render(<ChargeReceiptModal charge={{ ...rentalCharge, status: "PAID" }} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Salvar como PDF" }));

    await waitFor(() => expect(download).toHaveBeenCalledWith("charge-real-rent"));
    expect(click).toHaveBeenCalledTimes(1);
    // Cobrança quitada usa o recibo do backend — não abre a impressão.
    expect(printSpy).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
    download.mockRestore();
    click.mockRestore();
    printSpy.mockRestore();
  });

  it("cai para a impressão térmica quando o download do PDF falha", async () => {
    const download = vi.spyOn(chargeService, "downloadReceipt").mockRejectedValue(new Error("500"));
    const printSpy = vi.spyOn(printElementModule, "printElement").mockImplementation(() => {});

    const user = userEvent.setup();
    render(<ChargeReceiptModal charge={{ ...rentalCharge, status: "PAID" }} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Salvar como PDF" }));

    await waitFor(() => expect(printSpy).toHaveBeenCalled());
    download.mockRestore();
    printSpy.mockRestore();
  });
});

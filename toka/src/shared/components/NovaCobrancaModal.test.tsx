import { describe, it, expect, vi, beforeAll } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import type { ReactElement } from "react";

import { NovaCobrancaModal } from "./NovaCobrancaModal";

const renderInRouter = (ui: ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>);

const CONTRACTS = [
  { id: "c1", property_title: "Apt 101", client: "Maria", property_id: "p1", customer_id: "t1" },
];
const PROPERTIES = [{ id: "p1", ownerId: "o1" }];

const baseInitial = {
  id: "ch1",
  type: "RENT",
  contract_id: "c1",
  amount: 1500,
  due_date: "2026-07-10T00:00:00.000Z",
  reference_month: "2026-07",
  description: "Aluguel julho",
};

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

describe("NovaCobrancaModal (edit)", () => {
  it("preserva o payload de edição (valor mascarado, due_date ISO, sem line_items)", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderInRouter(
      <NovaCobrancaModal
        mode="edit"
        initial={baseInitial}
        contracts={CONTRACTS}
        properties={PROPERTIES}
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    expect((screen.getByLabelText(/valor/i) as HTMLInputElement).value).toMatch(/1\.500,00/);

    await user.click(screen.getByRole("button", { name: /salvar alterações/i }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [payload, id] = onSave.mock.calls[0];
    expect(id).toBe("ch1");
    expect(payload.amount).toBe(1500);
    expect(payload.due_date).toMatch(/^2026-07-10T/);
    expect(payload.type).toBe("RENT");
    expect(payload.line_items).toBeUndefined();
  });

  it("usa a soma da composição (line_items) como valor", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderInRouter(
      <NovaCobrancaModal
        mode="edit"
        initial={baseInitial}
        contracts={CONTRACTS}
        properties={PROPERTIES}
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^item$/i }));
    fireEvent.change(screen.getByPlaceholderText(/Descrição \(ex\.: Condomínio\)/i), {
      target: { value: "Condomínio" },
    });
    fireEvent.change(screen.getByPlaceholderText("0,00"), { target: { value: "30000" } }); // R$ 300,00

    await user.click(screen.getByRole("button", { name: /salvar alterações/i }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [payload] = onSave.mock.calls[0];
    expect(payload.line_items).toEqual([{ description: "Condomínio", amount_cents: 30000 }]);
    expect(payload.amount).toBe(300);
  });

  it("preserva identidade, titularidade econômica e regra ao editar itens gerados", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderInRouter(
      <NovaCobrancaModal
        mode="edit"
        initial={{
          ...baseInitial,
          amount: 6000,
          line_items: [
            {
              id: "item-condominio",
              description: "Condomínio",
              amount_cents: 70000,
              source_type: "rental_bundled_item",
              source_id: "condominio",
              economic_owner_id: "owner-marisa",
              settlement_mode: "liquidacao_direta",
              rule_version_id: "rule-2026-08",
            },
          ],
        }}
        contracts={CONTRACTS}
        properties={PROPERTIES}
        onClose={vi.fn()}
        onSave={onSave}
      />,
    );

    await user.click(screen.getByRole("button", { name: /salvar alterações/i }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave.mock.calls[0][0].line_items).toEqual([
      {
        id: "item-condominio",
        description: "Condomínio",
        amount_cents: 70000,
        source_type: "rental_bundled_item",
        source_id: "condominio",
        economic_owner_id: "owner-marisa",
        settlement_mode: "liquidacao_direta",
        rule_version_id: "rule-2026-08",
      },
    ]);
  });
});

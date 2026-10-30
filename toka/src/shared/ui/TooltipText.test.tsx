import { describe, expect, it, vi, beforeAll, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { TooltipText } from "./TooltipText";

beforeAll(() => {
  // Radix Tooltip usa APIs de ponteiro que o jsdom não implementa.
  const proto = Element.prototype as unknown as Record<string, unknown>;
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

/**
 * FIN-10: informação essencial em célula truncada dependia do `title` nativo,
 * que não padroniza aparência nem atraso, não aparece no foco do teclado e não
 * existe em toque. O gatilho precisa ser alcançável por foco para que a
 * informação não fique presa ao ponteiro.
 */
describe("TooltipText", () => {
  it("expõe o texto completo mesmo quando a célula trunca", () => {
    render(<TooltipText text="Marisa Andrade Ferreira da Silva">Marisa Andrade…</TooltipText>);

    // O conteúdo integral está no documento (o truncamento é visual).
    expect(screen.getByText("Marisa Andrade…")).toBeInTheDocument();
  });

  it("o gatilho é focável, então a informação não depende de hover", () => {
    render(<TooltipText text="Contraparte completa" />);
    const trigger = screen.getByText("Contraparte completa");

    trigger.focus();
    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute("tabindex", "0");
  });

  it("mostra o balão ao focar pelo teclado", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(<TooltipText text="Identificador completo do item">Item…</TooltipText>);

    await user.tab();
    // O balão é renderizado em portal; o texto completo aparece ao focar.
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Identificador completo do item");
  });

  it("não cria gatilho quando não há texto a revelar", () => {
    render(<TooltipText text="" />);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    expect(document.querySelector('[tabindex="0"]')).toBeNull();
  });
});

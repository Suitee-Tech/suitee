import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { SearchableSelect } from "./searchable-select";

/**
 * No celular o seletor com busca abre como PAINEL fixo no topo da área visível,
 * e não como dropdown flutuante: com o teclado aberto o popover ancorado ao
 * campo fica sob a faixa coberta e as opções não podem ser tocadas.
 */
function stubViewport(matchesSmallScreen: boolean, keyboardInset = 0) {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: () => ({
      matches: matchesSmallScreen,
      media: "(max-width: 639px)",
      addEventListener: () => {},
      removeEventListener: () => {},
    }),
  });
  Object.defineProperty(window, "visualViewport", {
    configurable: true,
    value: {
      height: window.innerHeight - keyboardInset,
      offsetTop: 0,
      addEventListener: () => {},
      removeEventListener: () => {},
    },
  });
}

beforeAll(() => {
  // cmdk (usado pelo combobox) depende de ResizeObserver e scrollIntoView.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

const options = [
  { value: "a", label: "Contrato A" },
  { value: "b", label: "Contrato B" },
];

afterEach(() => {
  cleanup();
  // @ts-expect-error jsdom não implementa matchMedia/visualViewport.
  delete window.matchMedia;
  // @ts-expect-error idem para a viewport visual.
  delete window.visualViewport;
});

describe("SearchableSelect", () => {
  it("em tela pequena abre painel fixo limitado à área visível", () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    stubViewport(true, 360);
    render(<SearchableSelect value="" onValueChange={vi.fn()} options={options} placeholder="Contrato" />);

    fireEvent.click(screen.getByRole("combobox"));

    const sheet = document.querySelector('[data-slot="dialog-content"]') as HTMLElement | null;
    expect(sheet).not.toBeNull();
    // Área visível = 800 - 360 = 440; o painel reserva 16px de margem.
    expect(sheet!.style.maxHeight).toBe("424px");
    expect(screen.getByText("Contrato A")).toBeInTheDocument();
  });

  it("em tela grande mantém o dropdown ancorado ao campo", () => {
    stubViewport(false);
    render(<SearchableSelect value="" onValueChange={vi.fn()} options={options} placeholder="Contrato" />);

    fireEvent.click(screen.getByRole("combobox"));

    expect(document.querySelector('[data-slot="dialog-content"]')).toBeNull();
    expect(document.querySelector('[data-slot="popover-content"]')).not.toBeNull();
  });
});

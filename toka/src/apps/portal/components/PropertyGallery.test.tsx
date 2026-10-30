import { describe, expect, it, vi, beforeAll } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";

import { PropertyGallery } from "./PropertyGallery";

const IMAGES = [
  { url: "https://files.example/fachada.jpg", description: "Fachada" },
  { url: "https://files.example/piscina.jpg", description: "Piscina" },
  { url: "https://files.example/sala.jpg", description: "Sala de estar" },
];

beforeAll(() => {
  // Embla (carrossel) depende de ResizeObserver, scrollIntoView e matchMedia.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  (globalThis as Record<string, unknown>).IntersectionObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
    takeRecords() { return []; }
    root = null;
    rootMargin = "";
    thresholds = [];
  };
  if (typeof window.matchMedia !== "function") {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
  }
});

describe("galeria do imóvel no portal", () => {
  it("mostra contador, legenda da foto atual e miniaturas navegáveis", () => {
    render(<PropertyGallery images={IMAGES} alt="Casa Vitória Régia" className="h-56" />);

    expect(screen.getByText("1/3")).toBeInTheDocument();
    expect(screen.getByText("Fachada")).toBeInTheDocument();
    const first = screen.getByRole("button", { name: "Ver foto 1: Fachada" });
    const second = screen.getByRole("button", { name: "Ver foto 2: Piscina" });
    expect(first).toHaveAttribute("aria-current", "true");
    expect(second).toHaveAttribute("aria-current", "false");
  });

  it("abre a tela cheia pelo botão Ampliar, com contador, legenda e fechamento", () => {
    render(<PropertyGallery images={IMAGES} alt="Casa Vitória Régia" className="h-56" />);

    fireEvent.click(screen.getByRole("button", { name: /Ampliar/ }));

    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText("Fotos de Casa Vitória Régia")).toBeInTheDocument();
    expect(within(dialog).getByText("1/3")).toBeInTheDocument();
    expect(within(dialog).getByText("Fachada")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Fechar" })).toBeInTheDocument();
    // Na tela cheia há acesso direto a cada foto.
    expect(within(dialog).getByRole("button", { name: "Ir para foto 3" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Fechar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("abre a tela cheia com um toque na foto", () => {
    render(<PropertyGallery images={IMAGES} alt="Casa Vitória Régia" className="h-56" />);

    const surface = screen.getByRole("region").parentElement as HTMLElement;
    fireEvent.pointerDown(surface, { clientX: 40, clientY: 60 });
    fireEvent.pointerUp(surface, { clientX: 42, clientY: 61 });

    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("um arraste troca de foto e NÃO abre a tela cheia", () => {
    render(<PropertyGallery images={IMAGES} alt="Casa Vitória Régia" className="h-56" />);

    const surface = screen.getByRole("region").parentElement as HTMLElement;
    fireEvent.pointerDown(surface, { clientX: 40, clientY: 60 });
    fireEvent.pointerUp(surface, { clientX: 120, clientY: 62 });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("com uma única foto não há contador nem miniaturas", () => {
    render(<PropertyGallery images={[IMAGES[0]]} alt="Casa" className="h-56" />);

    expect(screen.queryByText("1/1")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Ver foto/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Ampliar/ })).toBeInTheDocument();
  });

  it("sem foto cadastrada mostra o estado de preparação", () => {
    render(<PropertyGallery images={[]} alt="Casa" className="h-56" />);

    expect(screen.getByText("Fotos em preparação")).toBeInTheDocument();
  });
});

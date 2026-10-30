import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useVisualViewportInset } from "./useVisualViewportInset";

function Probe() {
  const inset = useVisualViewportInset();
  return <span data-testid="inset">{inset}</span>;
}

type FakeVisualViewport = {
  height: number;
  offsetTop: number;
  emit: (event: "resize" | "scroll") => void;
};

function stubVisualViewport(initial: { height: number; offsetTop?: number }): FakeVisualViewport {
  const listeners = new Map<string, Set<() => void>>();
  const fake = {
    height: initial.height,
    offsetTop: initial.offsetTop ?? 0,
    addEventListener: (event: string, callback: () => void) => {
      const set = listeners.get(event) ?? new Set();
      set.add(callback);
      listeners.set(event, set);
    },
    removeEventListener: (event: string, callback: () => void) => {
      listeners.get(event)?.delete(callback);
    },
    emit: (event: "resize" | "scroll") => {
      listeners.get(event)?.forEach((callback) => callback());
    },
  };
  Object.defineProperty(window, "visualViewport", { value: fake, configurable: true });
  return fake as unknown as FakeVisualViewport;
}

const originalVisualViewport = Object.getOwnPropertyDescriptor(window, "visualViewport");

afterEach(() => {
  cleanup();
  if (originalVisualViewport) {
    Object.defineProperty(window, "visualViewport", originalVisualViewport);
  } else {
    // @ts-expect-error jsdom não expõe visualViewport; remove o stub do teste.
    delete window.visualViewport;
  }
});

describe("useVisualViewportInset", () => {
  it("mede a faixa coberta pelo teclado e reage ao resize", () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    const viewport = stubVisualViewport({ height: 800 });

    render(<Probe />);
    expect(screen.getByTestId("inset")).toHaveTextContent("0");

    // Teclado abre: a viewport visual encolhe ~400px.
    act(() => {
      viewport.height = 400;
      viewport.emit("resize");
    });
    expect(screen.getByTestId("inset")).toHaveTextContent("400");

    // Teclado fecha.
    act(() => {
      viewport.height = 800;
      viewport.emit("resize");
    });
    expect(screen.getByTestId("inset")).toHaveTextContent("0");
  });

  it("desconta o deslocamento quando o navegador rola até o campo focado", () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    const viewport = stubVisualViewport({ height: 500, offsetTop: 100 });

    render(<Probe />);
    // Área coberta = 800 - 500 - 100 = 200.
    expect(screen.getByTestId("inset")).toHaveTextContent("200");

    act(() => {
      viewport.offsetTop = 300;
      viewport.emit("scroll");
    });
    expect(screen.getByTestId("inset")).toHaveTextContent("0");
  });

  it("fica em zero quando o navegador não expõe visualViewport", () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    // @ts-expect-error cobre navegadores/SSR sem a API.
    delete window.visualViewport;

    render(<Probe />);
    expect(screen.getByTestId("inset")).toHaveTextContent("0");
  });
});

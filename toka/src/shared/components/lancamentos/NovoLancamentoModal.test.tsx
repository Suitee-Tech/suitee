import { beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import { NovoLancamentoModal } from "./NovoLancamentoModal";

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

describe("NovoLancamentoModal", () => {
  it("não oferece repasse manual e explica a origem canônica", () => {
    const onSelect = vi.fn();
    render(<NovoLancamentoModal onClose={vi.fn()} onSelect={onSelect} />);

    expect(screen.queryByRole("button", { name: /repasse mensal/i })).not.toBeInTheDocument();
    expect(screen.getByText(/repasses não são lançados manualmente/i)).toBeInTheDocument();
    expect(screen.getByText(/nasce da baixa da cobrança ou do recebimento vinculado/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /cobrança.*cobrança manual/i }));
    expect(onSelect).toHaveBeenCalledWith("charge");
  });
});

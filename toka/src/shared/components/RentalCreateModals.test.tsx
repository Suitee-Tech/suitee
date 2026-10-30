import { beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { NovaCobrancaModal } from "./RentalCreateModals";

vi.mock("../context/PropertiesContext", () => ({
  usePropertiesContext: () => ({ properties: [{ id: "p1", ownerId: "o1" }] }),
}));

vi.mock("../hooks/useContracts", () => ({
  useCreateChargeMutation: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

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

const CONTRACT = {
  id: "c1",
  property_id: "p1",
  customer_id: "t1",
  client: "Maria",
  property_title: "Apt 101",
  value: 1500,
} as any;

describe("NovaCobrancaModal — acessibilidade do formulário", () => {
  it("associa categoria, descrição e erro ao passo de dados", () => {
    render(
      <MemoryRouter>
        <NovaCobrancaModal contract={CONTRACT} onClose={vi.fn()} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("group", { name: /Categoria da Cobrança/ })).toBeInTheDocument();
    const description = screen.getByLabelText(/Descrição/);
    expect(description).toHaveAttribute("aria-required", "true");

    fireEvent.click(screen.getByRole("button", { name: /Próximo/ }));

    const error = screen.getByRole("alert");
    expect(error).toHaveTextContent("Descrição obrigatória");
    expect(description).toHaveAttribute("aria-invalid", "true");
    expect(description).toHaveAttribute("aria-describedby", error.id);
  });
});

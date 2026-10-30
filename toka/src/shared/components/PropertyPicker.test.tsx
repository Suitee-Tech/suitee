import { beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { PropertyPicker } from "./PropertyPicker";

const properties = [
  { id: "prop-1", title: "Casa", displayId: "123", address: { street: "Rua A", neighborhood: "Centro" } },
];

// cmdk (usado pelo combobox) depende de ResizeObserver e de scrollIntoView,
// que o jsdom não implementa.
beforeAll(() => {
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView ??= () => {};
});

describe("PropertyPicker", () => {
  it("oferece cadastrar novo imóvel no rodapé da lista", async () => {
    const onCreateNew = vi.fn();
    render(<PropertyPicker properties={properties} value="" onChange={() => {}} onCreateNew={onCreateNew} />);

    fireEvent.click(screen.getByRole("combobox"));
    const cta = await screen.findByText("Cadastrar novo imóvel");
    fireEvent.click(cta);

    expect(onCreateNew).toHaveBeenCalledTimes(1);
  });

  it("oferece o cadastro também quando a busca não encontra nada", async () => {
    const onCreateNew = vi.fn();
    render(<PropertyPicker properties={properties} value="" onChange={() => {}} onCreateNew={onCreateNew} />);

    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.change(await screen.findByPlaceholderText("Buscar imóvel..."), { target: { value: "apartamento na praia" } });

    expect(await screen.findByText("Nenhum imóvel encontrado.")).toBeInTheDocument();
    // CTA no estado vazio (o do rodapé fica oculto enquanto o filtro não casa).
    const ctas = await screen.findAllByRole("button", { name: /Cadastrar novo imóvel/ });
    expect(ctas.length).toBeGreaterThan(0);
    fireEvent.click(ctas[0]);
    expect(onCreateNew).toHaveBeenCalledTimes(1);
  });

  it("não mostra o CTA quando quem usa o seletor não pediu", async () => {
    render(<PropertyPicker properties={properties} value="" onChange={() => {}} />);
    fireEvent.click(screen.getByRole("combobox"));
    expect(screen.queryByText("Cadastrar novo imóvel")).not.toBeInTheDocument();
  });

  it("mantém a seleção de um imóvel existente", async () => {
    const onChange = vi.fn();
    render(<PropertyPicker properties={properties} value="" onChange={onChange} onCreateNew={() => {}} />);

    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByText("Casa"));

    expect(onChange).toHaveBeenCalledWith("prop-1");
  });
});

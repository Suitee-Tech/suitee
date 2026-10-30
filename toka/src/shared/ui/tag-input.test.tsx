import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { TagInput } from "./tag-input";

// Estado controlado de verdade: o componente recebe a lista e devolve a próxima,
// como no formulário do imóvel.
function Harness({ initial = [], maxItems }: { initial?: string[]; maxItems?: number }) {
  const [value, setValue] = useState(initial);
  return (
    <form onSubmit={(event) => event.preventDefault()}>
      <label htmlFor="amenities">Comodidades</label>
      <TagInput id="amenities" label="Comodidades" value={value} onChange={setValue} maxItems={maxItems} placeholder="Ex: Piscina" />
      <output data-testid="values">{value.join("|")}</output>
    </form>
  );
}

// O nome acessível vem do <label htmlFor>, como no formulário do imóvel.
const input = () => screen.getByRole("textbox", { name: "Comodidades" });

describe("TagInput", () => {
  it("confirma o item ao digitar a vírgula, sem engolir o separador", () => {
    render(<Harness />);
    fireEvent.change(input(), { target: { value: "Piscina," } });

    expect(screen.getByTestId("values")).toHaveTextContent("Piscina");
    // O item virou um chip removível (e o campo ficou livre para o próximo).
    expect(screen.getByRole("button", { name: "Remover Piscina" })).toBeInTheDocument();
    expect(input()).toHaveValue("");
  });

  it("permite cadastrar VÁRIOS itens digitando (o caso relatado)", () => {
    render(<Harness />);
    // "Piscina," -> academia + Enter -> "Salão de Festas," etc.
    fireEvent.change(input(), { target: { value: "Piscina," } });
    fireEvent.change(input(), { target: { value: "Academia" } });
    fireEvent.keyDown(input(), { key: "Enter" });
    fireEvent.change(input(), { target: { value: "Salão de Festas" } });
    fireEvent.keyDown(input(), { key: "Enter" });

    expect(screen.getByTestId("values")).toHaveTextContent("Piscina|Academia|Salão de Festas");
  });

  it("Enter confirma o item e não envia o formulário", () => {
    const onSubmit = vi.fn((event: React.FormEvent) => event.preventDefault());
    render(
      <form onSubmit={onSubmit}>
        <label htmlFor="only">Comodidades</label>
        <TagInput id="only" label="Comodidades" value={[]} onChange={() => {}} />
      </form>,
    );
    const field = screen.getByRole("textbox", { name: "Comodidades" });
    fireEvent.change(field, { target: { value: "Piscina" } });
    fireEvent.keyDown(field, { key: "Enter" });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("colar uma lista separada por vírgulas cria vários itens", () => {
    render(<Harness />);
    fireEvent.change(input(), { target: { value: "Piscina, Academia, Churrasqueira" } });

    expect(screen.getByTestId("values")).toHaveTextContent("Piscina|Academia|Churrasqueira");
  });

  it("ignora duplicata sem diferenciar maiúsculas e normaliza o texto", () => {
    render(<Harness initial={["Piscina"]} />);
    fireEvent.change(input(), { target: { value: "  piscina ,  Academia  " } });

    expect(screen.getByTestId("values")).toHaveTextContent("Piscina|Academia");
  });

  it("Backspace no campo vazio remove o último item", () => {
    render(<Harness initial={["Piscina", "Academia"]} />);
    fireEvent.keyDown(input(), { key: "Backspace" });

    expect(screen.getByTestId("values")).toHaveTextContent("Piscina");
    expect(screen.queryByText("Academia")).not.toBeInTheDocument();
  });

  it("remove um item pelo botão, com rótulo acessível", () => {
    render(<Harness initial={["Piscina", "Academia"]} />);
    fireEvent.click(screen.getByRole("button", { name: "Remover Piscina" }));

    expect(screen.getByTestId("values")).toHaveTextContent("Academia");
  });

  it("confirma o texto em digitação ao sair do campo (não perde o que foi digitado)", () => {
    render(<Harness />);
    fireEvent.change(input(), { target: { value: "Playground" } });
    fireEvent.blur(input());

    expect(screen.getByTestId("values")).toHaveTextContent("Playground");
  });

  it("avisa quando estoura o teto de itens e não grava além dele", () => {
    render(<Harness maxItems={2} />);
    fireEvent.change(input(), { target: { value: "Piscina" } });
    fireEvent.keyDown(input(), { key: "Enter" });
    fireEvent.change(input(), { target: { value: "Academia" } });
    fireEvent.keyDown(input(), { key: "Enter" });
    fireEvent.change(input(), { target: { value: "Sauna" } });
    fireEvent.keyDown(input(), { key: "Enter" });

    expect(screen.getByRole("alert")).toHaveTextContent("máximo de 2 itens");
    expect(screen.getByTestId("values")).toHaveTextContent("Piscina|Academia");
  });

  it("mostra o contador e o texto de apoio", () => {
    render(
      <TagInput
        label="Comodidades"
        value={["Piscina", "Academia"]}
        onChange={() => {}}
        hint="Confirme com Enter ou vírgula."
      />,
    );
    expect(screen.getByText("2 de 40")).toBeInTheDocument();
    expect(screen.getByText("Confirme com Enter ou vírgula.")).toBeInTheDocument();
  });
});

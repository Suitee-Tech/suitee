import * as React from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { MoneyInput } from "./money-input";

// Harness controlado: segura o estado em reais e re-renderiza, como um modal real faz.
function Harness({
  initial,
  fractionDigits,
  withSymbol,
}: {
  initial?: number;
  fractionDigits?: 0 | 2;
  withSymbol?: boolean;
}) {
  const [v, setV] = React.useState<number | undefined>(initial);
  return (
    <>
      <MoneyInput
        aria-label="valor"
        placeholder="0,00"
        value={v}
        onChange={setV}
        fractionDigits={fractionDigits}
        withSymbol={withSymbol}
      />
      <output data-testid="val">{v === undefined ? "undefined" : String(v)}</output>
    </>
  );
}

const val = () => screen.getByTestId("val").textContent;
const input = () => screen.getByLabelText("valor") as HTMLInputElement;
// Simula o navegador acrescentando um caractere no fim do campo (o gesto dominante).
const append = (el: HTMLInputElement, ch: string) => fireEvent.change(el, { target: { value: el.value + ch } });

describe("MoneyInput — transformação dígitos→reais (fractionDigits=2)", () => {
  it("trata cada keystroke como acumulador de centavos", () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="v" value={undefined} onChange={onChange} />);
    const el = screen.getByLabelText("v");

    fireEvent.change(el, { target: { value: "5" } });
    expect(onChange).toHaveBeenLastCalledWith(0.05); // 5 centavos
    fireEvent.change(el, { target: { value: "150" } });
    expect(onChange).toHaveBeenLastCalledWith(1.5);
    fireEvent.change(el, { target: { value: "150000" } });
    expect(onChange).toHaveBeenLastCalledWith(1500);
  });

  it("ignora não-dígitos e faz round-trip de um valor já formatado (com NBSP)", () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="v" value={undefined} onChange={onChange} withSymbol />);
    const el = screen.getByLabelText("v");

    // "R$ 1.234,56" com o NBSP do Intl entre símbolo e número.
    fireEvent.change(el, { target: { value: "R$ 1.234,56" } });
    expect(onChange).toHaveBeenLastCalledWith(1234.56);
  });

  it("campo vazio e lixo viram undefined", () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="v" value={123} onChange={onChange} />);
    const el = screen.getByLabelText("v");

    fireEvent.change(el, { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
    fireEvent.change(el, { target: { value: "abc" } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });
});

describe("MoneyInput — render a partir da prop (controlado/stateless)", () => {
  it("formata o value numérico em pt-BR", () => {
    const { rerender } = render(<MoneyInput aria-label="v" value={42} onChange={vi.fn()} />);
    expect((screen.getByLabelText("v") as HTMLInputElement).value).toBe("42,00");
    rerender(<MoneyInput aria-label="v" value={42.5} onChange={vi.fn()} />);
    expect((screen.getByLabelText("v") as HTMLInputElement).value).toBe("42,50");
  });

  it("undefined mostra string vazia (placeholder aparece)", () => {
    render(<MoneyInput aria-label="v" value={undefined} onChange={vi.fn()} placeholder="0,00" />);
    const el = screen.getByLabelText("v") as HTMLInputElement;
    expect(el.value).toBe("");
    expect(el.placeholder).toBe("0,00");
  });

  it("withSymbol prefixa R$ (assertivo tolerante a NBSP)", () => {
    render(<MoneyInput aria-label="v" value={1500} onChange={vi.fn()} withSymbol />);
    expect((screen.getByLabelText("v") as HTMLInputElement).value).toMatch(/^R\$\s1\.500,00$/);
  });
});

describe("MoneyInput — fractionDigits=0 (reais inteiros)", () => {
  it("cada dígito é um real, sem casas decimais", () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="v" value={undefined} onChange={onChange} fractionDigits={0} />);
    const el = screen.getByLabelText("v");
    fireEvent.change(el, { target: { value: "850000" } });
    expect(onChange).toHaveBeenLastCalledWith(850000);
  });

  it("formata sem decimais", () => {
    render(<MoneyInput aria-label="v" value={850000} onChange={vi.fn()} fractionDigits={0} />);
    expect((screen.getByLabelText("v") as HTMLInputElement).value).toBe("850.000");
  });
});

describe("MoneyInput — integração: acumulador + foco (regressão do bug)", () => {
  it("digitar 1·5·0·0·0·0 acumula até R$ 1.500,00 sem travar", () => {
    render(<Harness />);
    const el = input();
    for (const ch of "150000") append(el, ch);
    expect(el.value).toBe("1.500,00");
    expect(val()).toBe("1500");
  });

  it("é digitável até os centavos (zeros à esquerda)", () => {
    render(<Harness />);
    const el = input();
    append(el, "5"); // antes do fix, ficava preso em "5,00"
    expect(el.value).toBe("0,05");
    expect(val()).toBe("0.05");
  });

  it("backspace remove o último dígito (divide por 10)", () => {
    render(<Harness initial={1500} />);
    const el = input();
    expect(el.value).toBe("1.500,00");
    fireEvent.change(el, { target: { value: "1.500,0" } }); // apaga o último char visível
    expect(el.value).toBe("150,00");
    expect(val()).toBe("150");
  });

  it("limpar tudo volta a undefined e reexibe o placeholder", () => {
    render(<Harness initial={1500} />);
    const el = input();
    fireEvent.change(el, { target: { value: "" } });
    expect(el.value).toBe("");
    expect(val()).toBe("undefined");
  });

  it("digitação real (userEvent) acumula e mantém o foco", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const el = input();
    await user.click(el);
    await user.type(el, "150000");
    expect(el).toHaveFocus();
    expect(val()).toBe("1500");
    expect(el.value).toBe("1.500,00");
  });
});

describe("MoneyInput — readOnly", () => {
  it("não dispara onChange ao digitar", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<MoneyInput aria-label="v" value={100} onChange={onChange} readOnly />);
    await user.type(screen.getByLabelText("v"), "5");
    expect(onChange).not.toHaveBeenCalled();
  });
});

import * as React from "react";
import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { PercentInput } from "./percent-input";

// Harness controlado, como um form real: segura o número e re-renderiza.
function Harness({ initial }: { initial?: number }) {
  const [v, setV] = React.useState<number | undefined>(initial);
  return (
    <>
      <PercentInput aria-label="pct" value={v} onChange={setV} />
      <output data-testid="val">{v === undefined ? "undefined" : String(v)}</output>
    </>
  );
}
const val = () => screen.getByTestId("val").textContent;
const el = () => screen.getByLabelText("pct") as HTMLInputElement;

describe("PercentInput", () => {
  it("exibe o value externo formatado", () => {
    render(<Harness initial={50} />);
    expect(el().value).toBe("50");
  });

  it("digita inteiro", () => {
    render(<Harness />);
    fireEvent.change(el(), { target: { value: "6" } });
    expect(val()).toBe("6");
    expect(el().value).toBe("6");
  });

  it("permite decimal com vírgula sem a vírgula sumir ao digitar", () => {
    render(<Harness />);
    fireEvent.change(el(), { target: { value: "12," } });
    expect(el().value).toBe("12,");
    fireEvent.change(el(), { target: { value: "12,5" } });
    expect(el().value).toBe("12,5");
    expect(val()).toBe("12.5");
  });

  it("aceita ponto como separador decimal (normaliza p/ vírgula)", () => {
    render(<Harness />);
    fireEvent.change(el(), { target: { value: "12.5" } });
    expect(val()).toBe("12.5");
    expect(el().value).toBe("12,5");
  });

  it("campo vazio vira undefined", () => {
    render(<Harness initial={10} />);
    fireEvent.change(el(), { target: { value: "" } });
    expect(val()).toBe("undefined");
  });

  it("ignora letras", () => {
    render(<Harness />);
    fireEvent.change(el(), { target: { value: "ab" } });
    expect(val()).toBe("undefined");
    expect(el().value).toBe("");
  });
});

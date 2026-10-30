import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { CepInput } from "./cep-input";

const setup = (value = "") => {
  const onChange = vi.fn();
  render(<CepInput aria-label="cep" value={value} onChange={onChange} />);
  return { el: screen.getByLabelText("cep") as HTMLInputElement, onChange };
};

describe("CepInput", () => {
  it("formata 00000-000 a partir de value", () => {
    expect(setup("01310100").el.value).toBe("01310-100");
  });

  it("formata parcial sem o hífen até 5 dígitos", () => {
    expect(setup("013").el.value).toBe("013");
  });

  it("emite só os dígitos no onChange", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "01310-100" } });
    expect(onChange).toHaveBeenLastCalledWith("01310100");
  });

  it("limita a 8 dígitos", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "013101009999" } });
    expect(onChange).toHaveBeenLastCalledWith("01310100");
  });

  it("vazio mostra placeholder (value vazio)", () => {
    expect(setup("").el.value).toBe("");
  });
});

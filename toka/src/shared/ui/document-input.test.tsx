import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { DocumentInput, type DocumentMode } from "./document-input";

const setup = (value = "", mode: DocumentMode = "auto") => {
  const onChange = vi.fn();
  render(<DocumentInput aria-label="doc" mode={mode} value={value} onChange={onChange} />);
  return { el: screen.getByLabelText("doc") as HTMLInputElement, onChange };
};

describe("DocumentInput", () => {
  it("auto: aplica máscara de CPF até 11 dígitos", () => {
    expect(setup("12345678901").el.value).toBe("123.456.789-01");
  });

  it("auto: aplica máscara de CNPJ a partir de 12 dígitos", () => {
    expect(setup("12345678000199").el.value).toBe("12.345.678/0001-99");
  });

  it("emite os dígitos crus no onChange", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "123.456.789-01" } });
    expect(onChange).toHaveBeenLastCalledWith("12345678901");
  });

  it("mode=cnpj limita a 14 dígitos", () => {
    const { el, onChange } = setup("", "cnpj");
    fireEvent.change(el, { target: { value: "123456780001999999" } });
    expect(onChange).toHaveBeenLastCalledWith("12345678000199");
  });

  it("mode=cnpj formata CNPJ a partir de value", () => {
    expect(setup("12345678000199", "cnpj").el.value).toBe("12.345.678/0001-99");
  });

  it("mode=cpf limita a 11 dígitos", () => {
    const { el, onChange } = setup("", "cpf");
    fireEvent.change(el, { target: { value: "123456789012345" } });
    expect(onChange).toHaveBeenLastCalledWith("12345678901");
  });

  it("vazio mostra placeholder (value vazio)", () => {
    expect(setup("").el.value).toBe("");
  });
});

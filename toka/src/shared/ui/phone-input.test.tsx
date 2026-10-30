import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { PhoneInput } from "./phone-input";

const setup = (value = "") => {
  const onChange = vi.fn();
  render(<PhoneInput aria-label="tel" value={value} onChange={onChange} />);
  return { el: screen.getByLabelText("tel") as HTMLInputElement, onChange };
};

describe("PhoneInput", () => {
  it("formata celular (11 dígitos) a partir de value", () => {
    expect(setup("11999998888").el.value).toBe("(11) 99999-8888");
  });

  it("formata fixo (10 dígitos) a partir de value", () => {
    expect(setup("1133334444").el.value).toBe("(11) 3333-4444");
  });

  it("emite só os dígitos no onChange", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "(11) 99999-8888" } });
    expect(onChange).toHaveBeenLastCalledWith("11999998888");
  });

  it("tolera +55 colado", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "+55 11 99999-8888" } });
    expect(onChange).toHaveBeenLastCalledWith("11999998888");
  });

  it("ignora não-dígitos", () => {
    const { el, onChange } = setup("");
    fireEvent.change(el, { target: { value: "abc" } });
    expect(onChange).toHaveBeenLastCalledWith("");
  });

  it("vazio mostra placeholder (value vazio)", () => {
    expect(setup("").el.value).toBe("");
  });
});

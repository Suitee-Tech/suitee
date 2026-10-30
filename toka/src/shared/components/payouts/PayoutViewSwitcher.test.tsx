import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { PayoutViewSwitcher } from "./PayoutViewSwitcher";

describe("PayoutViewSwitcher", () => {
  it("marca a visão de consolidação como atual", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/payouts"]}>
        <PayoutViewSwitcher />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Padrão" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Listagem simples" })).not.toHaveAttribute("aria-current");
  });

  it("permite voltar da listagem simples para a padrão", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/payouts-lista"]}>
        <PayoutViewSwitcher />
      </MemoryRouter>,
    );

    const table = screen.getByRole("link", { name: "Padrão" });
    expect(table).toHaveAttribute("href", "/financeiro/payouts");
    expect(screen.getByRole("link", { name: "Listagem simples" })).toHaveAttribute("aria-current", "page");
  });
});

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { FinanceAreaNav } from "./FinanceAreaNav";

describe("FinanceAreaNav", () => {
  it("marca somente a área canônica atual", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/cobrancas"]}>
        <FinanceAreaNav />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Recebimentos" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Painel" })).not.toHaveAttribute("aria-current");
  });

  it("mantém a listagem simples dentro de Ordens de pagamento", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/payouts-lista"]}>
        <FinanceAreaNav />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Ordens de pagamento" })).toHaveAttribute("aria-current", "page");
  });

  it("trata a régua como um subfluxo de Recebimentos", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/regua"]}>
        <FinanceAreaNav />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Recebimentos" })).toHaveAttribute("aria-current", "page");
  });

  it("expõe Impostos retidos como área própria do financeiro", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/impostos"]}>
        <FinanceAreaNav />
      </MemoryRouter>,
    );

    const tab = screen.getByRole("link", { name: "Impostos retidos" });
    expect(tab).toHaveAttribute("href", "/financeiro/impostos");
    expect(tab).toHaveAttribute("aria-current", "page");
  });

  it("expõe Adiantamentos como área própria do financeiro", () => {
    render(
      <MemoryRouter initialEntries={["/financeiro/adiantamentos"]}>
        <FinanceAreaNav />
      </MemoryRouter>,
    );

    const tab = screen.getByRole("link", { name: "Adiantamentos" });
    expect(tab).toHaveAttribute("href", "/financeiro/adiantamentos");
    expect(tab).toHaveAttribute("aria-current", "page");
  });
});

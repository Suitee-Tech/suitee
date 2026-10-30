import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import { PageHeader } from "./page-header";

describe("PageHeader", () => {
  it("renderiza título, descrição e ações", () => {
    const onClick = vi.fn();
    render(
      <PageHeader
        title="Estoque de Imóveis"
        description="Busque e gerencie os imóveis do portfólio"
      >
        <button type="button" onClick={onClick}>Novo Cadastro</button>
      </PageHeader>,
    );

    expect(screen.getByRole("heading", { level: 1, name: "Estoque de Imóveis" })).toBeInTheDocument();
    expect(screen.getByText("Busque e gerencie os imóveis do portfólio")).toBeInTheDocument();
    screen.getByRole("button", { name: "Novo Cadastro" }).click();
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("usa h2 quando embutido e ignora ícone na árvore acessível", () => {
    render(
      <PageHeader as="h2" title="Visão geral" icon={<span data-testid="icon">★</span>}>
        <button type="button">Ação</button>
      </PageHeader>,
    );

    expect(screen.getByRole("heading", { level: 2, name: "Visão geral" })).toBeInTheDocument();
    expect(screen.getByTestId("icon").parentElement).toHaveAttribute("aria-hidden", "true");
  });
});

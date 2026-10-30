import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { MessageContent } from "./MessageContent";

describe("MessageContent", () => {
  it("renderiza tabela com caption e uma versão em cards para o mobile", () => {
    render(
      <MessageContent
        content={"| Nome | Perfil | Status |\n| --- | --- | --- |\n| Maria | admin | Ativo |"}
      />,
    );

    expect(screen.getByText("Tabela da resposta")).toBeInTheDocument();
    expect(screen.getAllByText("Nome").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Maria")).toHaveLength(2);
    expect(screen.getAllByText("Perfil").length).toBeGreaterThan(0);
    expect(document.querySelectorAll(".rounded-xl")).toHaveLength(1);
  });
});

describe("MessageContent links", () => {
  it("normaliza link de imóvel para a rota canônica", () => {
    render(
      <MemoryRouter>
        <MessageContent content={"[Abrir imóvel](/properties/abc-123)"} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Abrir imóvel" })).toHaveAttribute("href", "/imoveis/abc-123");
  });
});

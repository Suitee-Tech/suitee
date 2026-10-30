import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { JsonTable } from "./JsonTable";

const content = JSON.stringify([
  { title: "Repasse — Aluguel 2026-11", subtitle: "Líquido R$ 2.614,62", status: "PENDING", open_path: "/properties/abc" },
]);

describe("JsonTable", () => {
  it("localiza cabeçalhos, esconde paths internos e oferece ação", () => {
    render(<MemoryRouter><JsonTable content={content} /></MemoryRouter>);

    expect(screen.getAllByText("Título").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Pendente").length).toBeGreaterThan(0);
    expect(screen.queryByText(/\/transfers\//)).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Imóvel/ }).length).toBeGreaterThan(0);
  });
});

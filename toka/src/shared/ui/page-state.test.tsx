import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import { PageState } from "./page-state";

describe("PageState", () => {
  it("mostra skeleton com status acessível enquanto carrega", () => {
    render(<PageState loading><p>conteúdo</p></PageState>);
    expect(screen.getByRole("status")).toHaveTextContent("Carregando");
    expect(screen.queryByText("conteúdo")).not.toBeInTheDocument();
  });

  it("anuncia erro e oferece retry", () => {
    const retry = vi.fn();
    render(<PageState error="Falha ao carregar cobranças" onRetry={retry}><p>conteúdo</p></PageState>);

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Falha ao carregar cobranças");
    fireEvent.click(screen.getByRole("button", { name: /Tentar novamente/i }));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("mostra o vazio e libera o conteúdo quando pronto", () => {
    const { rerender } = render(<PageState empty emptyState={<p>Nenhuma cobrança</p>}><p>conteúdo</p></PageState>);
    expect(screen.getByText("Nenhuma cobrança")).toBeInTheDocument();

    rerender(<PageState><p>conteúdo</p></PageState>);
    expect(screen.getByText("conteúdo")).toBeInTheDocument();
  });
});

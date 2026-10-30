import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach } from "vitest";
import { describe, expect, it, vi } from "vitest";

import { VariableSidebar } from "./VariableSidebar";
import type { VariableDetectionState } from "../../hooks/useVariableDetection";

const detection: VariableDetectionState = {
  allFoundKeys: ["buyer_name"],
  identifiedVars: [{ key: "buyer_name", label: "Nome do Comprador", category: "Pessoas" }],
  unidentifiedKeys: [],
  isScanning: false,
};

const renderSidebar = (overrides: Partial<VariableDetectionState> = {}) =>
  render(
    <VariableSidebar
      detection={{ ...detection, ...overrides }}
      onTriggerScan={vi.fn()}
      onInsertVariable={vi.fn()}
      onMapVariable={vi.fn()}
      onRemapVariable={vi.fn()}
    />,
  );

describe("VariableSidebar", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("usa plural correto e não duplica a contagem no topo", () => {
    renderSidebar();
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("1 variável identificada");
    expect(status).not.toHaveTextContent("1 identificadas");
  });

  it("não mostra o construtor de data quando a busca não tem relação", { timeout: 30000 }, () => {
    renderSidebar();
    const search = screen.getByRole("textbox", { name: "Buscar variável" });
    fireEvent.change(search, { target: { value: "propr" } });
    expect(screen.queryByText("Calcular data ou vencimento")).not.toBeInTheDocument();
    expect(screen.getByText(/resultados nesta aba/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Limpar busca" }));
    expect(screen.getByText("Calcular data ou vencimento")).toBeInTheDocument();
  });

  it("destaca o trecho encontrado no código da variável", () => {
    renderSidebar();
    fireEvent.change(screen.getByRole("textbox", { name: "Buscar variável" }), { target: { value: "buyer" } });
    expect(document.querySelector("mark")).toHaveTextContent("buyer");
  });

  it("expõe abas com semântica e alterna para o documento", () => {
    renderSidebar();
    expect(screen.getByRole("tab", { name: "Sistema" })).toHaveAttribute("aria-selected", "true");
    fireEvent.click(screen.getByRole("tab", { name: /No documento/ }));
    expect(screen.getByRole("tab", { name: /No documento/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Identificadas (1)");
  });

  it("permite fixar e registra recentes ao copiar", { timeout: 20000 }, () => {
    renderSidebar();
    fireEvent.click(screen.getAllByRole("button", { name: "Copiar {{buyer}}" })[0]);
    expect(screen.getByText("Recentes")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Fixar {{buyer}}" })[0]);
    expect(screen.getByText("Fixadas")).toBeInTheDocument();
  });
});

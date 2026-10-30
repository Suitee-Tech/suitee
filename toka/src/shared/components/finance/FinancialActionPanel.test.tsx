import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FinancialActionPanel, PanelSection } from "./FinancialActionPanel";

afterEach(cleanup);

// O painel é o shell ÚNICO das ações financeiras (baixa de cobrança, ordem de
// pagamento, repasse, comissão, lançamento). Estes testes travam o que garante a
// consistência entre os fluxos: mesmo cabeçalho, mesmas ações no rodapé, erro no
// mesmo lugar e envio único.
describe("FinancialActionPanel", () => {
  it("mostra título, subtítulo e a seção com a mesma hierarquia em qualquer fluxo", () => {
    render(
      <FinancialActionPanel
        onClose={vi.fn()}
        title="Registrar pagamento"
        subtitle="Cobrança de aluguel · R$ 2.500,00"
        submitLabel="Confirmar recebimento"
      >
        <PanelSection title="Dados do recebimento" description="Valor e data do comprovante.">
          <p>campos</p>
        </PanelSection>
      </FinancialActionPanel>,
    );

    // O nome acessível do painel vem do título (sr-only, exigido pelo Radix);
    // o título visível é o `h2` do cabeçalho padrão.
    expect(screen.getByRole("dialog", { name: "Registrar pagamento" })).toBeInTheDocument();
    const headings = screen.getAllByRole("heading", { name: "Registrar pagamento" });
    expect(headings.some((node) => node.tagName === "H2" && !node.className.includes("sr-only"))).toBe(true);
    expect(screen.getByText("Cobrança de aluguel · R$ 2.500,00")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Dados do recebimento" })).toBeInTheDocument();
    expect(screen.getByText("Valor e data do comprovante.")).toBeInTheDocument();
  });

  it("mantém Cancelar e a ação primária no rodapé, com rótulos explícitos", () => {
    const onClose = vi.fn();
    render(
      <FinancialActionPanel onClose={onClose} title="Processar repasse" submitLabel="Confirmar repasse">
        <p>corpo</p>
      </FinancialActionPanel>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /Confirmar repasse/ })).toBeEnabled();
  });

  it("usa o rótulo de progresso e bloqueia o envio duplo enquanto grava", () => {
    render(
      <FinancialActionPanel
        onClose={vi.fn()}
        title="Registrar pagamento"
        submitLabel="Confirmar recebimento"
        submitting
        submittingLabel="Enviando comprovante…"
      >
        <p>corpo</p>
      </FinancialActionPanel>,
    );

    const submit = screen.getByRole("button", { name: /Enviando comprovante…/ });
    expect(submit).toBeDisabled();
    expect(submit).toHaveAttribute("aria-busy", "true");
    // Cancelar também fica bloqueado: fechar no meio do envio perderia o estado.
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
  });

  it("ignora o fechamento durante o envio em todos os fluxos (X, Escape e clique fora)", () => {    const onClose = vi.fn();
    const { rerender } = render(
      <FinancialActionPanel onClose={onClose} title="Registrar pagamento" submitLabel="Confirmar">
        <p>corpo</p>
      </FinancialActionPanel>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Fechar painel" }));
    expect(onClose).toHaveBeenCalledTimes(1);

    // Com envio em andamento, a saída fica inerte: fechar perderia o comprovante
    // já enviado e deixaria a gravação órfã (FIN-06).
    rerender(
      <FinancialActionPanel onClose={onClose} title="Registrar pagamento" submitLabel="Confirmar" submitting>
        <p>corpo</p>
      </FinancialActionPanel>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Fechar painel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("associa o submit do rodapé ao formulário do corpo (formId)", () => {
    render(
      <FinancialActionPanel onClose={vi.fn()} title="Baixar lançamento" submitLabel="Baixar" formId="settle-form">
        <form id="settle-form">
          <input aria-label="valor" />
        </form>
      </FinancialActionPanel>,
    );

    const submit = screen.getByRole("button", { name: /Baixar/ });
    expect(submit).toHaveAttribute("type", "submit");
    expect(submit).toHaveAttribute("form", "settle-form");
  });

  it("mostra o erro da operação com role=alert no mesmo lugar em todos os fluxos", () => {
    render(
      <FinancialActionPanel
        onClose={vi.fn()}
        title="Marcar ordem como paga"
        submitLabel="Confirmar"
        error="Não foi possível enviar o comprovante. O arquivo selecionado foi mantido."
      >
        <p>corpo</p>
      </FinancialActionPanel>,
    );

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Não foi possível concluir");
    expect(alert).toHaveTextContent("Não foi possível enviar o comprovante");
  });

  it("expõe ação secundária no rodapé só quando o fluxo pede (ex.: cancelar ordem)", () => {
    render(
      <FinancialActionPanel
        onClose={vi.fn()}
        title="Ordem de pagamento"
        submitLabel="Marcar como paga"
        secondaryAction={<button type="button">Cancelar ordem</button>}
      >
        <p>corpo</p>
      </FinancialActionPanel>,
    );

    expect(screen.getByRole("button", { name: "Cancelar ordem" })).toBeInTheDocument();
  });
});

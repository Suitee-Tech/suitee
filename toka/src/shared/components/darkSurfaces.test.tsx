import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Building2 } from "lucide-react";

import { ThemeProvider } from "../context/ThemeContext";
import { ReportKPICard } from "./reports/ReportKPICard";
import { ExpandableSidePanel } from "./ExpandableSidePanel";

// Superfícies "invertidas" no modo noturno: dois componentes usavam 90% da cor do
// TEXTO (`foreground`, quase branco) como FUNDO em modo escuro, enquanto o texto
// dentro deles continua sendo `text-foreground`/`text-muted-foreground` (claros).
// Resultado nos prints: o drawer de "Novo compromisso" e o de "Nova proposta"
// apareciam CLAROS com rótulos apagados, e os cards de Relatórios mostravam
// "R$ 0" branco sobre card branco.
//
// O contrato é o padrão de design: superfície é `bg-card`/`bg-background` (o token
// resolve claro e escuro). Estes testes travam isso na classe renderizada — um
// teste de fonte não pegaria a variante `dark:bg-foreground/90`.
function renderDark(ui: React.ReactNode) {
  // O resolvedMode do provider vem do tema persistido (mesma chave do app).
  window.localStorage.setItem("suitee-theme-settings", JSON.stringify({ theme: "dark" }));
  return render(<ThemeProvider>{ui}</ThemeProvider>);
}

describe("ReportKPICard no modo noturno", () => {
  it("usa a superfície do tema, não o fundo invertido", () => {
    const { container } = renderDark(
      <ReportKPICard
        label="Valor do Portfólio"
        value="R$ 0"
        change={0}
        changeLabel="vs. período anterior"
        icon={Building2}
        iconBg="bg-primary-50"
        iconColor="text-primary-600"
        sparkData={[45, 52, 48, 61, 55, 70, 68, 82]}
      />,
    );

    const card = container.firstElementChild as HTMLElement;
    expect(card.className).toContain("bg-card");
    expect(card.className).not.toContain("bg-foreground");

    // As barras do sparkline também não podem ser quase brancas sólidas.
    const bars = Array.from(card.querySelectorAll("div")).filter((el) => el.className.includes("rounded-sm"));
    expect(bars.length).toBeGreaterThan(0);
    for (const bar of bars) {
      expect(bar.className).not.toContain("bg-foreground/75");
    }
  });

  it("mantém valor e rótulo legíveis (tokens de texto)", () => {
    renderDark(
      <ReportKPICard
        label="Receita Total"
        value="R$ 0"
        changeLabel="vs. período anterior"
        icon={Building2}
        iconBg="bg-status-ok-soft"
        iconColor="text-status-ok"
      />,
    );

    expect(screen.getByText("R$ 0").className).toContain("text-foreground");
    expect(screen.getByText("Receita Total").className).toContain("text-muted-foreground");
  });
});

describe("ExpandableSidePanel no modo noturno", () => {
  it("o painel é superfície do tema, não o fundo invertido claro", () => {
    renderDark(
      <ExpandableSidePanel open onClose={vi.fn()} ariaLabel="Novo compromisso">
        <div>corpo do painel</div>
      </ExpandableSidePanel>,
    );

    const panel = document.querySelector('[role="dialog"]') as HTMLElement | null;
    expect(panel, "painel não renderizou").not.toBeNull();
    expect(panel!.className).toContain("bg-card");
    expect(panel!.className).not.toContain("bg-foreground/90");
  });
});

import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EmailPreferencesPanel, EmailPrefsServiceLike } from "./EmailPreferencesPanel";
import type { EmailPref } from "../services/email-prefs.service";

const CATEGORIES: EmailPref[] = [
  { category: "lead_assigned", label: "Novos leads atribuídos", enabled: true, locked: true },
  { category: "proposals", label: "Propostas e contrapropostas", enabled: true, locked: false },
  { category: "recommendations", label: "Sugestões do corretor", enabled: false, locked: false },
];

function makeService(overrides?: Partial<EmailPrefsServiceLike>): EmailPrefsServiceLike {
  return {
    list: vi.fn(async () => CATEGORIES),
    update: vi.fn(async (cats) => {
      const [first] = cats;
      return CATEGORIES.map((c) => (c.category === first.category ? { ...c, enabled: first.enabled } : c));
    }),
    ...overrides,
  };
}

describe("EmailPreferencesPanel", () => {
  it("lista as categorias com estado atual", async () => {
    render(<EmailPreferencesPanel service={makeService()} />);
    expect(await screen.findByText("Propostas e contrapropostas")).toBeInTheDocument();
    expect(screen.getByText("Sugestões do corretor")).toBeInTheDocument();
  });

  it("desabilita o toggle de categoria travada", async () => {
    render(<EmailPreferencesPanel service={makeService()} />);
    await screen.findByText("Novos leads atribuídos");
    const lockedSwitch = screen.getByRole("switch", { name: "Novos leads atribuídos" });
    expect(lockedSwitch).toBeDisabled();
  });

  it("salva o toggle e aplica a resposta do backend", async () => {
    const service = makeService();
    const user = userEvent.setup();
    render(<EmailPreferencesPanel service={service} />);
    await screen.findByText("Propostas e contrapropostas");

    await user.click(screen.getByRole("switch", { name: "Propostas e contrapropostas" }));
    await waitFor(() => {
      expect(service.update).toHaveBeenCalledWith([{ category: "proposals", enabled: false }]);
    });
  });

  it("reverte o toggle otimista quando o backend falha", async () => {
    const service = makeService({
      update: vi.fn(async () => {
        throw new Error("network");
      }),
    });
    const user = userEvent.setup();
    render(<EmailPreferencesPanel service={service} />);
    await screen.findByText("Propostas e contrapropostas");

    await user.click(screen.getByRole("switch", { name: "Propostas e contrapropostas" }));
    expect(await screen.findByText(/Não foi possível salvar/)).toBeInTheDocument();
    // Reverteu: volta a ligado.
    expect(screen.getByRole("switch", { name: "Propostas e contrapropostas" })).toBeChecked();
  });

  it("mostra erro quando a listagem falha", async () => {
    const service = makeService({
      list: vi.fn(async () => {
        throw new Error("network");
      }),
    });
    render(<EmailPreferencesPanel service={service} />);
    expect(await screen.findByText(/Não foi possível carregar/)).toBeInTheDocument();
  });
});

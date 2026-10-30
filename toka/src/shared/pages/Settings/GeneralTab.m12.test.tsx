import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { GeneralTab } from "./GeneralTab";

// P2/M12 — a aba "Geral & Sistema" anunciava 2FA que não existe.
//
// A revisão de permissões de 2026-09-19 registrou que `two_factor_enabled` era
// preferência sem efeito: o usuário ligava o toggle, a tela mostrava "2FA
// ativado — todos os usuários serão solicitados a configurar autenticação de dois
// fatores no próximo login" e nada acontecia (não existe enroll nem step-up).
//
// Contrato novo: nenhum controle que minta. A linha continua existindo para o
// usuário não procurar em vão, mas diz que o recurso não está disponível.
vi.mock("../../services/settings.service", () => ({
  settingsService: {
    getPreferences: vi.fn().mockResolvedValue({
      id: "p1",
      account_id: "acc-1",
      language: "pt-BR",
      timezone: "America/Sao_Paulo",
      date_format: "dd/MM/yyyy",
      session_timeout: 30,
      focus_mode: false,
      two_factor_enabled: true, // resposta antiga em cache: não pode virar "2FA ativado"
    }),
    getCompanySettings: vi.fn().mockResolvedValue({ currency: "BRL", autoSave: true, compactTables: false }),
    updatePreferences: vi.fn().mockResolvedValue(undefined),
    updateCompanySettings: vi.fn().mockResolvedValue(undefined),
    resetPreferences: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("GeneralTab (P2/M12)", () => {
  it("não oferece toggle de 2FA nem afirma que o 2FA está ativado", async () => {
    const { container } = render(<GeneralTab onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByText(/Autenticação em dois fatores/i)).toBeInTheDocument();
    });

    expect(screen.queryByText(/2FA ativado/i)).toBeNull();
    expect(screen.queryByText(/autenticacao de dois fatores no proximo login/i)).toBeNull();
    expect(screen.getByText(/Ainda não disponível nesta versão/i)).toBeInTheDocument();

    // A linha de 2FA não pode ter NENHUM controle interativo: era o papel do
    // toggle mentir. A FieldRow renderiza dois filhos — o bloco do rótulo e o
    // slot do controle —, então o slot é o último filho do container da linha.
    const label = screen.getByText(/Autenticação em dois fatores/i);
    const labelBox = label.parentElement!;
    const rowRoot = labelBox.parentElement!;
    const controlSlot = rowRoot.lastElementChild!;
    expect(controlSlot.querySelectorAll("button, input, select")).toHaveLength(0);
    expect(controlSlot.textContent).toContain("Não disponível");
    void container;
  });
});

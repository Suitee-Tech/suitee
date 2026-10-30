import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { TermsAcceptanceModal } from "./TermsAcceptanceModal";
import { CURRENT_TERMS_VERSION } from "../config/lgpd";

// O aceite do pacote legal precisa distinguir ACEITAR os Termos de Uso de
// CONFIRMAR CIÊNCIA da Política de Privacidade: a Política é aviso de
// transparência e não consentimento genérico (LGPD art. 8º, §4º).
const success = vi.fn();

vi.mock("sonner", () => ({ toast: { success: (...a: unknown[]) => success(...a), error: vi.fn() } }));

const settingsClient = {
  getPreferences: vi.fn(async () => ({})),
  patchPreferences: vi.fn(async () => undefined),
};

function renderModal(consentVersion: string | null) {
  settingsClient.getPreferences.mockResolvedValue(
    consentVersion === null ? {} : { consent: { terms_version: consentVersion } },
  );
  return render(
    <MemoryRouter>
      <TermsAcceptanceModal
        policyPath="/portal/privacy"
        termsPath="/portal/terms"
        settings={settingsClient}
      />
    </MemoryRouter>,
  );
}

describe("TermsAcceptanceModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("pede novo aceite quando a versão registrada é antiga", async () => {
    renderModal("1.0");

    expect(await screen.findByRole("button", { name: "Aceitar Termos e continuar" })).toBeInTheDocument();
    expect(screen.getByText(/leia o resumo e os dois documentos/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Termos de Uso/ })).toHaveAttribute("href", "/portal/terms");
    expect(screen.getByRole("link", { name: /Política de Privacidade/ })).toHaveAttribute("href", "/portal/privacy");
  });

  it("não pede aceite quando o usuário já aceitou a versão vigente", async () => {
    renderModal(CURRENT_TERMS_VERSION);

    await waitFor(() => expect(settingsClient.getPreferences).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Aceitar Termos e continuar" })).not.toBeInTheDocument();
  });

  it("registra o aceite dos Termos na versão vigente", async () => {
    const user = userEvent.setup();
    renderModal(null);

    await user.click(await screen.findByRole("button", { name: "Aceitar Termos e continuar" }));

    expect(settingsClient.patchPreferences).toHaveBeenCalledWith({
      consent: { terms_version: CURRENT_TERMS_VERSION },
    });
    expect(success).toHaveBeenCalledWith("Aceite dos Termos de Uso registrado.");
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Aceitar Termos e continuar" })).not.toBeInTheDocument(),
    );
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";

// Regressão: ao trocar de imobiliária, o `pickOne` do merge era aplicado a
// TODOS os campos e revertia cores/fontes para o valor anterior — o tema
// ficava o da empresa anterior. Este teste renderiza o provider duas vezes
// com empresas diferentes e garante que a cor primária acompanha a empresa.

const mocks = vi.hoisted(() => ({
  useOptionalAuth: vi.fn(),
  getAppearance: vi.fn(),
  getCompanySettings: vi.fn(),
}));

vi.mock("./AuthContext", () => ({
  useOptionalAuth: () => mocks.useOptionalAuth(),
}));

vi.mock("../services/settings.service", () => ({
  settingsService: {
    getAppearance: mocks.getAppearance,
    getCompanySettings: mocks.getCompanySettings,
  },
}));

import { ThemeProvider, useTheme } from "./ThemeContext";

function Probe() {
  const { theme } = useTheme();
  return <span data-testid="primary">{theme.primaryColor}</span>;
}

describe("ThemeProvider (troca de empresa)", () => {
  beforeEach(() => {
    window.localStorage.clear();
    mocks.useOptionalAuth.mockReset();
    mocks.getAppearance.mockReset();
    mocks.getCompanySettings.mockReset();
    mocks.getCompanySettings.mockResolvedValue({ logoPreview: null });
  });

  it("aplica a fonte da marca no body e o traço dos ícones no documento", async () => {
    mocks.useOptionalAuth.mockReturnValue({ user: { id: "u1", current_company_id: "empresa-a" } });
    mocks.getAppearance.mockResolvedValue({
      primaryColor: "#AA0000",
      theme: "light",
      fontFamily: "Poppins",
      iconStyle: "bold",
    });

    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );

    await waitFor(() => expect(document.body.style.fontFamily).toContain("Poppins"));
    const root = document.documentElement;
    expect(root.style.getPropertyValue("--font-sans")).toContain("Poppins");
    expect(root.style.getPropertyValue("--default-font-family")).toContain("Poppins");
    expect(root.style.getPropertyValue("--icon-stroke")).toBe("2.5");
    expect(root.style.getPropertyValue("--icon-linecap")).toBe("round");
    await waitFor(() => {
      const link = document.getElementById("suitee-tenant-font") as HTMLLinkElement | null;
      expect(link?.href).toContain("Poppins");
    });
  });

  it("normaliza fonte/estilo desconhecidos para os padrões seguros", async () => {
    mocks.useOptionalAuth.mockReturnValue({ user: { id: "u1", current_company_id: "empresa-a" } });
    mocks.getAppearance.mockResolvedValue({
      primaryColor: "#AA0000",
      theme: "light",
      fontFamily: "Comic Sans",
      iconStyle: "neon",
    });

    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );

    await waitFor(() => expect(document.body.style.fontFamily).toContain("Inter"));
    expect(document.documentElement.style.getPropertyValue("--icon-stroke")).toBe("2");
  });

  it("atualiza a cor primária quando o usuário troca de empresa", async () => {
    mocks.useOptionalAuth.mockReturnValue({ user: { id: "u1", current_company_id: "empresa-a" } });
    mocks.getAppearance.mockResolvedValue({ primaryColor: "#AA0000", theme: "light" });

    const view = render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() => expect(view.getByTestId("primary").textContent).toBe("#AA0000"));

    // Troca para a empresa B: o usuário muda e o provider deve re-buscar a
    // aparência da nova empresa e aplicar a nova cor (não a da empresa A).
    mocks.useOptionalAuth.mockReturnValue({ user: { id: "u1", current_company_id: "empresa-b" } });
    mocks.getAppearance.mockResolvedValue({ primaryColor: "#0000AA", theme: "light" });

    view.rerender(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    await waitFor(() => expect(view.getByTestId("primary").textContent).toBe("#0000AA"));
  });
});

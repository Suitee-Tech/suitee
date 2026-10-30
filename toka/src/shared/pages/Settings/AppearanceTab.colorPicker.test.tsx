import { createRef } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AppearanceTab } from "./AppearanceTab";
import type { TabRef } from "./_shared";

// Seletor de cores do white-label (Configurações > Aparencia).
//
// O quadrado de cor era só um `<div>`: a única forma de trocar a cor principal
// era digitar o hexadecimal ou escolher um dos presets — não havia seletor.
const getAppearance = vi.fn();

vi.mock("../../services/settings.service", () => ({
  settingsService: {
    getAppearance: (...args: unknown[]) => getAppearance(...args),
    updateAppearance: vi.fn(),
    uploadLoginBanner: vi.fn().mockResolvedValue({ banner_url: "" }),
  },
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const baseAppearance = {
  tenant_id: "tenant-1",
  primaryColor: "#1C905B",
  accentColor: "#10B981",
  sidebarStyle: "default" as const,
  theme: "light" as const,
  showLogoLogin: true,
  darkModeEnabled: true,
  customDomain: "",
  domainVerified: false,
  loginTagline: "Sua plataforma imobiliaria",
  loginSubtitle: "Gestao completa",
  loginBannerPreset: "equipe",
  loginBannerUrl: "",
  loginBannerSource: "",
  emailFooter: "",
  fontFamily: "Inter",
};

async function renderTab(overrides: Record<string, unknown> = {}) {
  getAppearance.mockResolvedValue({ ...baseAppearance, ...overrides });
  render(<AppearanceTab ref={createRef<TabRef>()} onChange={() => {}} />);
  await waitFor(() => expect(getAppearance).toHaveBeenCalled());
}

const primaryPicker = () => screen.getByLabelText("Cor principal") as HTMLInputElement;
const accentPicker = () => screen.getByLabelText("Cor de acento") as HTMLInputElement;

describe("AppearanceTab — seletor de cores", () => {
  beforeEach(() => {
    getAppearance.mockReset();
  });

  it("oferece um seletor de cor nativo para a cor principal e para a de acento", async () => {
    await renderTab();

    for (const picker of [primaryPicker(), accentPicker()]) {
      expect(picker).toHaveAttribute("type", "color");
    }
    // O `<input type="color">` só aceita `#rrggbb`.
    expect(primaryPicker()).toHaveValue("#1c905b");
    expect(accentPicker()).toHaveValue("#10b981");
  });

  it("aplica a cor escolhida no seletor e reflete no campo hexadecimal", async () => {
    await renderTab();

    fireEvent.change(primaryPicker(), { target: { value: "#ff8800" } });

    // O campo de texto é o mesmo rótulo que o operador já conhece.
    expect(screen.getByLabelText("Cor Principal (Primary)")).toHaveValue("#ff8800");
    expect(primaryPicker()).toHaveValue("#ff8800");
  });

  it("mantém o seletor na última cor válida enquanto o hexadecimal está incompleto", async () => {
    await renderTab();

    const hex = screen.getByLabelText("Cor Principal (Primary)");
    fireEvent.change(hex, { target: { value: "#3" } });

    // Digitação parcial: o quadrado para de piscar preto, mas continua um seletor.
    expect(primaryPicker()).toHaveValue("#1c905b");
    expect(hex).toHaveValue("#3");
  });

  it("normaliza um hexadecimal curto gravado no tenant", async () => {
    await renderTab({ primaryColor: "#abc" });

    expect(primaryPicker()).toHaveValue("#aabbcc");
  });
});

describe("AppearanceTab — ícone indicativo do seletor", () => {
  beforeEach(() => {
    getAppearance.mockReset();
  });

  it("mostra o ícone dentro da área clicável das duas cores", async () => {
    await renderTab();

    for (const picker of [primaryPicker(), accentPicker()]) {
      const area = picker.closest("label");
      expect(area).not.toBeNull();
      expect(within(area as HTMLElement).getByTestId("color-input-icon")).toBeInTheDocument();
      // O ícone é decorativo: quem carrega o nome acessível é o input de cor.
      expect(area?.querySelector('[aria-hidden="true"]')).not.toBeNull();
    }
  });

  it("adapta o ícone à cor de fundo do quadrado", async () => {
    await renderTab({ primaryColor: "#0d1b2a", accentColor: "#D1FAE5" });

    const corDoChip = (input: HTMLInputElement) => {
      const chip = input.closest("label")?.querySelector('[data-testid="color-input-icon"]')?.parentElement;
      return chip ? getComputedStyle(chip).color : null;
    };

    // Cor escura -> ícone claro; cor clara -> ícone escuro.
    expect(corDoChip(primaryPicker())).toBe("rgb(255, 255, 255)");
    expect(corDoChip(accentPicker())).toBe("rgb(30, 41, 59)");
  });
});

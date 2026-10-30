import { createRef } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AppearanceTab } from "./AppearanceTab";
import type { TabRef } from "./_shared";
import { LOGIN_BANNERS, resolveLoginBanner } from "../../loginBanners";

// Prévia da imagem da tela de login (Configurações > Aparência).
//
// Contexto do defeito: o upload gravava a URL no storage e o "Salvar" devolvia
// 400 porque a validação do backend montava a allowlist de hosts com
// `SUPABASE_URL` (host interno do container) enquanto a URL gravada usava o host
// público. Ao investigar, a tela também não mostrava a imagem enviada em lugar
// nenhum — o usuário só descobria o resultado depois de sair e abrir o login.
//
// Segundo defeito, corrigido junto: escolher um preset APAGAVA a imagem enviada
// (`loginBannerUrl: ""`), então voltar para ela exigia subir o arquivo de novo.
// Agora a aba mostra as duas na mesma lista e a troca é um clique.
const getAppearance = vi.fn();
const updateAppearance = vi.fn();

vi.mock("../../services/settings.service", () => ({
  settingsService: {
    getAppearance: (...args: unknown[]) => getAppearance(...args),
    updateAppearance: (...args: unknown[]) => updateAppearance(...args),
    uploadLoginBanner: vi.fn().mockResolvedValue({ banner_url: "https://cdn.exemplo.com/novo.webp" }),
  },
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const baseAppearance = {
  tenant_id: "tenant-1",
  primaryColor: "#2563EB",
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
  iconStyle: "regular",
};

const UPLOADED = "https://cdn.exemplo.com/toca-logos/banner.webp";
const labelOf = (id: string) => `Usar imagem: ${LOGIN_BANNERS.find((b) => b.id === id)!.label}`;
const presetSrc = (id: string) => LOGIN_BANNERS.find((b) => b.id === id)!.src;

async function renderTab(overrides: Record<string, unknown> = {}) {
  getAppearance.mockResolvedValue({ ...baseAppearance, ...overrides });
  const ref = createRef<TabRef>();
  render(<AppearanceTab ref={ref} onChange={() => {}} />);
  await screen.findByTestId("login-banner-tile-custom").catch(() => undefined);
  await waitFor(() => expect(getAppearance).toHaveBeenCalled());
  return ref;
}

describe("AppearanceTab — tipografia e ícones", () => {
  it("mostra a prévia da fonte e a opção de traço salva", async () => {
    await renderTab({ fontFamily: "Poppins", iconStyle: "bold" });

    const preview = await screen.findByTestId("font-preview");
    expect(preview.style.fontFamily).toContain("Poppins");
    expect(screen.getByText(/Poppins · fonte da sua marca/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Ícones Forte/i })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("AppearanceTab — prévia da imagem de login", () => {
  it("mostra a imagem enviada na prévia", async () => {
    await renderTab({ loginBannerUrl: UPLOADED });

    const preview = await screen.findByTestId("login-banner-preview");
    expect(preview).toHaveAttribute("src", UPLOADED);
    expect(screen.getByText(/Imagem enviada pela imobiliária/i)).toBeInTheDocument();
  });

  it("mostra o preset quando não há imagem enviada", async () => {
    await renderTab({ loginBannerPreset: "trm", loginBannerUrl: "" });

    const preview = await screen.findByTestId("login-banner-preview");
    // Mesma resolução da tela de login: se as duas divergirem, a prévia mente.
    expect(preview).toHaveAttribute("src", resolveLoginBanner("trm", ""));
    expect(preview.getAttribute("src")).toBeTruthy();
  });

  it("avisa quando a imagem não carrega, em vez de mostrar um painel vazio", async () => {
    await renderTab({ loginBannerUrl: UPLOADED });

    const preview = await screen.findByTestId("login-banner-preview");
    fireEvent.error(preview);

    await waitFor(() => {
      expect(screen.getByText(/Não foi possível carregar esta imagem/i)).toBeInTheDocument();
    });
    expect(screen.queryByTestId("login-banner-preview")).toBeNull();
  });
});

describe("AppearanceTab — troca entre a imagem enviada e os modelos", () => {
  it("oferece a imagem enviada como uma opção ao lado dos modelos", async () => {
    await renderTab({ loginBannerUrl: UPLOADED });

    const tile = await screen.findByTestId("login-banner-tile-custom");
    expect(tile).toHaveAttribute("aria-pressed", "true"); // é a que está em uso
    for (const banner of LOGIN_BANNERS) {
      expect(screen.getByLabelText(labelOf(banner.id))).toBeInTheDocument();
    }
  });

  it("troca para o modelo escolhido sem apagar a imagem enviada", async () => {
    await renderTab({ loginBannerUrl: UPLOADED });

    fireEvent.click(screen.getByLabelText(labelOf("trm")));

    await waitFor(() => {
      expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", presetSrc("trm"));
    });
    // O ponto do pedido: a imagem continua na lista, pronta para voltar.
    expect(screen.getByTestId("login-banner-tile-custom")).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText(/Imagem enviada guardada/i)).toBeInTheDocument();
  });

  it("volta para a imagem enviada com um clique", async () => {
    await renderTab({ loginBannerUrl: UPLOADED });

    fireEvent.click(screen.getByLabelText(labelOf("trm")));
    await waitFor(() =>
      expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", presetSrc("trm")),
    );

    fireEvent.click(screen.getByTestId("login-banner-tile-custom"));

    await waitFor(() => {
      expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", UPLOADED);
    });
    expect(screen.getByText(/Imagem enviada em uso/i)).toBeInTheDocument();
  });

  // A troca só vale se o "Salvar" mandar as DUAS imagens: é o payload que o
  // backend grava para permitir a troca no próximo acesso.
  it("salva as duas imagens, mudando só a fonte ativa", async () => {
    const ref = await renderTab({ loginBannerUrl: UPLOADED });

    fireEvent.click(screen.getByLabelText(labelOf("cliente")));
    await waitFor(() =>
      expect(screen.getByTestId("login-banner-tile-custom")).toHaveAttribute("aria-pressed", "false"),
    );

    await act(async () => {
      await ref.current!.save();
    });

    expect(updateAppearance).toHaveBeenCalledWith(
      expect.objectContaining({
        loginBannerUrl: UPLOADED,
        loginBannerSource: "preset",
        loginBannerPreset: "cliente",
      }),
    );
  });

  it("remove a imagem enviada só quando isso é pedido, e volta para o preset", async () => {
    const ref = await renderTab({ loginBannerUrl: UPLOADED, loginBannerPreset: "room" });

    fireEvent.click(screen.getByRole("button", { name: /Remover imagem/i }));

    await waitFor(() => {
      expect(screen.queryByTestId("login-banner-tile-custom")).toBeNull();
    });
    expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", presetSrc("room"));

    await act(async () => {
      await ref.current!.save();
    });
    expect(updateAppearance).toHaveBeenCalledWith(
      expect.objectContaining({ loginBannerUrl: "", loginBannerSource: "preset" }),
    );
  });

  // Registro gravado antes do campo `loginBannerSource`: a imagem enviada
  // continua sendo a que aparece, sem exigir resalvar a aba.
  it("trata a imagem enviada como ativa quando a fonte não está definida", async () => {
    await renderTab({ loginBannerUrl: UPLOADED, loginBannerSource: "" });

    expect(await screen.findByTestId("login-banner-tile-custom")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", UPLOADED);
  });

  // O preset escolhido sobrevive a ter uma imagem enviada ativa: ao trocar para
  // preset, ele é o que aparece — não o padrão do sistema.
  it("lembra o preset escolhido quando a imagem enviada está ativa", async () => {
    await renderTab({ loginBannerUrl: UPLOADED, loginBannerPreset: "pessoas", loginBannerSource: "custom" });

    fireEvent.click(screen.getByLabelText(labelOf("pessoas")));
    // Clicar no preset que já estava escolhido troca para ele (não é no-op).
    await waitFor(() => {
      expect(screen.getByTestId("login-banner-preview")).toHaveAttribute("src", presetSrc("pessoas"));
    });
  });
});


it("associa rótulos aos controles de aparência e anuncia a seleção", async () => {
  await renderTab();
  expect(screen.getByLabelText("Cor Principal (Primary)")).toHaveValue("#2563EB");
  expect(screen.getByLabelText("Cor de Acento (Accent)")).toHaveValue("#10B981");
  expect(screen.getByLabelText("Familia Tipografica")).toHaveValue("Inter");
  expect(screen.getByLabelText("Título do Login")).toHaveValue("Sua plataforma imobiliaria");
  expect(screen.getByLabelText("Subtítulo do Login")).toHaveValue("Gestao completa");
  expect(screen.getByLabelText("Rodape dos E-mails")).toBeInTheDocument();
  const compact = screen.getByRole("button", { name: /Compacta: Icones com tooltip/ });
  fireEvent.click(compact);
  expect(compact).toHaveAttribute("aria-pressed", "true");
});

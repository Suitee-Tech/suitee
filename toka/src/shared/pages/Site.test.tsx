import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Site } from "./Site";

const mocks = vi.hoisted(() => ({
  getOverview: vi.fn(),
  readiness: vi.fn(),
  save: vi.fn(),
  savePage: vi.fn(),
  leads: vi.fn(),
  reconcile: vi.fn(),
}));

vi.mock("../services/site.service", async () => {
  const actual = await vi.importActual<typeof import("../services/site.service")>("../services/site.service");
  return { ...actual, siteService: mocks };
});
vi.mock("../context/PermissionsContext", () => ({
  usePermissionsContext: () => ({ can: () => true }),
}));
vi.mock("../context/ThemeContext", () => ({
  useTheme: () => ({
    theme: {
      primaryColor: "#1C905B",
      accentColor: "#10b981",
      logoUrl: "https://files.example/logo.png",
      loginTagline: "Sua plataforma imobiliária",
      theme: "light",
    },
    resolvedMode: "light",
  }),
}));
vi.mock("../context/AuthContext", () => ({
  useAuth: () => ({
    companies: [
      {
        id: "tenant-1",
        name: "Imobiliária Exemplo",
        email: "contato@Exemplo.com.br",
        phone: "(11) 99999-0000",
        address: { street: "Rua A", number: "100", district: "Centro", city: "Atibaia", state: "SP", zip_code: "12900-000" },
      },
    ],
    user: { current_company_id: "tenant-1" },
  }),
}));
vi.mock("../services/settings.service", () => ({
  settingsService: {
    getCompanySettings: vi.fn().mockResolvedValue({ creci: "CRECI-SP 12345-J" }),
  },
}));

const overview = {
  site: {
    id: "site-1",
    tenant_id: "tenant-1",
    slug: "exemplo",
    name: "Imobiliária Exemplo",
    enabled: true,
    public_base_url: "https://exemplo.imoveis.example.com",
    branding: { primary_color: "#1C905B" },
    contact: { email: "contato@Exemplo.com.br", creci: "CRECI-SP 12345-J" },
  },
  pages: [
    {
      key: "home",
      title: "Encontre seu imóvel",
      subtitle: "",
      body: "",
      seo_title: "Imóveis em Atibaia",
      seo_description: "Casas e apartamentos selecionados em Atibaia.",
    },
    { key: "about", title: "Sobre nós", subtitle: "", body: "" },
    { key: "contact", title: "Fale conosco", subtitle: "", body: "" },
  ],
  stats: {
    views_7d: 12,
    views_30d: 40,
    leads_7d: 3,
    leads_30d: 8,
    published_listings: 4,
    total_listings: 5,
    unpublished_listings: 1,
  },
  host_suffix: "imoveis.example.com",
  public_scheme: "https",
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Site />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const overviewWithCompanyLogo = {
  ...overview,
  site: {
    ...overview.site,
    branding: { ...overview.site.branding, logo_url: "https://files.example/logo.png" },
  },
};

describe("Site (painel)", () => {
  beforeEach(() => {
    mocks.save.mockReset();
    mocks.save.mockResolvedValue(overviewWithCompanyLogo);
    mocks.readiness.mockReset();
    mocks.readiness.mockResolvedValue({
      ready: true,
      status: "ready",
      public_url: "https://exemplo.imoveis.example.com",
    });
  });

  it("mostra o host comercial e as métricas da vitrine", async () => {
    mocks.getOverview.mockResolvedValue(overview);
    mocks.leads.mockResolvedValue({ leads: [], total: 0 });
    renderPage();

    expect(await screen.findByRole("heading", { name: /site de imóveis/i })).toBeInTheDocument();
    expect(screen.getByText("https://exemplo.imoveis.example.com")).toBeInTheDocument();
    expect(screen.getByText("Visualizações (7d)")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("Leads (7d)")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(await screen.findByDisplayValue("https://files.example/logo.png")).toBeInTheDocument();
    expect(await screen.findByDisplayValue("Imóveis em Atibaia")).toBeInTheDocument();
    expect(await screen.findByDisplayValue("Casas e apartamentos selecionados em Atibaia.")).toBeInTheDocument();
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith(
        expect.objectContaining({
          branding: expect.objectContaining({ logo_url: "https://files.example/logo.png" }),
        }),
      ),
    );
  });

  it("não libera abrir/copiar o site enquanto o certificado valida", async () => {
    mocks.getOverview.mockResolvedValue(overview);
    mocks.leads.mockResolvedValue({ leads: [], total: 0 });
    mocks.readiness.mockResolvedValue({
      ready: false,
      status: "awaiting_certificate",
      public_url: "https://exemplo.imoveis.example.com",
    });
    renderPage();

    expect(await screen.findByText(/publicando seu site com segurança/i)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /abrir site/i })).not.toBeInTheDocument();
    const validating = screen.getByRole("button", { name: /validando certificado/i });
    expect(validating).toBeDisabled();
    expect(screen.getByRole("button", { name: /copiar link/i })).toBeDisabled();
  });

  it("libera o link somente quando o certificado está pronto", async () => {
    mocks.getOverview.mockResolvedValue(overview);
    mocks.leads.mockResolvedValue({ leads: [], total: 0 });
    renderPage();

    expect(await screen.findByText(/site publicado com https válido/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /abrir site/i })).toHaveAttribute(
      "href",
      "https://exemplo.imoveis.example.com",
    );
  });

  it("salva a marca editada e o toggle do site", async () => {
    mocks.getOverview.mockResolvedValue(overview);
    mocks.leads.mockResolvedValue({ leads: [], total: 0 });
    mocks.save.mockResolvedValue({ ...overview, site: { ...overview.site, enabled: false, slug: "Exemplo-novo" } });
    renderPage();

    const slug = await screen.findByLabelText(/marca do endereço/i);
    await userEvent.clear(slug);
    await userEvent.type(slug, "Exemplo-novo");
    await userEvent.click(screen.getByRole("switch", { name: /ativar site de imóveis/i }));
    await userEvent.click(screen.getByRole("button", { name: /salvar site/i }));

    await waitFor(() => expect(mocks.save).toHaveBeenCalled());
    expect(mocks.save).toHaveBeenCalledWith(
      expect.objectContaining({
        slug: "Exemplo-novo",
        enabled: false,
        name: "Imobiliária Exemplo",
        contact: expect.objectContaining({ creci: "CRECI-SP 12345-J" }),
      }),
    );
  });

  it("não deixa salvar com marca reservada", async () => {
    mocks.getOverview.mockResolvedValue(overview);
    mocks.leads.mockResolvedValue({ leads: [], total: 0 });
    renderPage();

    const slug = await screen.findByLabelText(/marca do endereço/i);
    await userEvent.clear(slug);
    await userEvent.type(slug, "admin");

    expect(await screen.findByRole("alert")).toHaveTextContent(/reservada/i);
    expect(screen.getByRole("button", { name: /salvar site/i })).toBeDisabled();
  });
});

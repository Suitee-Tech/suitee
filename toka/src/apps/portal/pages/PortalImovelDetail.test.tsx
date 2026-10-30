import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { PortalImovelDetail } from "./PortalImovelDetail";

// Regressão do "Imóvel não encontrado." no portal (2026-09-19).
//
// A página resolvia o imóvel APENAS pelo overview do lead (recomendação/match).
// Quem chega pela Busca — que lista imóveis por busca do negócio e entrega o
// objeto no `state` da navegação — via a mensagem de não encontrado mesmo com o
// imóvel existindo; recarregar a página (perdendo o `state`) tinha o mesmo
// efeito. Agora a resolução tem três caminhos: estado da navegação, busca do
// negócio ativo e, por último, o overview.
const mocks = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getDealProperty: vi.fn(),
  searchProperties: vi.fn(),
  requestLeadVisit: vi.fn(),
  updateLeadVisit: vi.fn(),
  openDirectChannel: vi.fn(),
  getSiteListingUrl: vi.fn().mockResolvedValue({ published: false }),
  activeDeal: { id: "deal-1" } as { id: string } | null,
}));

vi.mock("../services/portal.service", () => ({ portalService: mocks }));
vi.mock("../context/PortalDealsContext", () => ({
  usePortalDeals: () => ({ activeDeal: mocks.activeDeal, deals: [], activeDealId: mocks.activeDeal?.id ?? null }),
}));
vi.mock("../hooks/usePortalOverview", () => ({
  usePortalOverview: () => ({ data: mocks.getOverview(), isPending: false, isError: false, refetch: vi.fn() }),
}));
vi.mock("../hooks/usePortalWebSocket", () => ({ usePortalWebSocket: () => ({ onMessage: vi.fn(() => () => {}) }) }));
vi.mock("../../../shared/hooks/useDataChangeRefresh", () => ({ useDataChangeRefresh: vi.fn() }));
vi.mock("../components/AppointmentPickerDialog", () => ({
  AppointmentPickerDialog: ({ open, mode, onSubmit }: any) => open ? <div><span>{mode}</span><button onClick={() => onSubmit({ date: new Date("2099-10-21T00:00:00"), startTime: "16:00", endTime: "17:00", note: "" })}>Enviar horário</button></div> : null,
}));

const imovel = {
  id: "property-1",
  title: "Apartamento no Centro",
  neighborhood: "Centro",
  city: "Curitiba",
  state: "PR",
  price: 450000,
  listing_type: "sale",
  image_url: "https://files.example/capa.jpg",
  bedrooms: 3,
  suites: 1,
  bathrooms: 2,
  parking_spaces: 2,
  total_area_m2: 120,
  usable_area_m2: 100,
  amenities: ["piscina", "sala_de_estar"],
  description: "Apartamento reformado, sol da manhã.",
  images: [
    { url: "https://files.example/capa.jpg", description: "Fachada" },
    { url: "https://files.example/sala.jpg", description: "Sala de estar" },
    { url: "https://files.example/quarto.jpg", description: "Quarto" },
  ],
};

function page(state?: Record<string, unknown>) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[{ pathname: "/portal/imoveis/property-1", state }]}
      >
        <Routes>
          <Route path="/portal/imoveis/:id" element={<PortalImovelDetail />} />
          <Route path="/portal/buscar" element={<div>Busca do portal</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeAll(() => {
  // Embla (carrossel da galeria) depende de ResizeObserver e scrollIntoView.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  // Embla consulta matchMedia para as media queries do carrossel.
  if (typeof window.matchMedia !== "function") {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
  }
  (globalThis as Record<string, unknown>).IntersectionObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
    takeRecords() { return []; }
    root = null;
    rootMargin = "";
    thresholds = [];
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getOverview.mockReturnValue([]);
  mocks.getDealProperty.mockResolvedValue(null);
  mocks.searchProperties.mockResolvedValue([]);
  mocks.activeDeal = { id: "deal-1" };
});

describe("detalhe do imóvel no portal", () => {
  it("mostra o imóvel que veio no estado da navegação (Busca, Meus Imóveis, Dashboard)", async () => {
    render(page({ property: imovel, leadId: "lead-1", dealId: "deal-1" }));
    expect(await screen.findByText(imovel.title)).toBeInTheDocument();
    expect(screen.queryByText("Imóvel não encontrado.")).not.toBeInTheDocument();
  });

  it("recupera o imóvel pela busca do negócio quando a página é recarregada (sem estado)", async () => {
    mocks.searchProperties.mockResolvedValue([{ ...imovel }]);
    render(page());
    expect(await screen.findByText(imovel.title)).toBeInTheDocument();
    expect(mocks.searchProperties).toHaveBeenCalledWith("deal-1", {});
  });

  it("abre o imóvel da negociação pelo endpoint do negócio, mesmo fora das sugestões", async () => {
    // A busca do negócio devolve as SUGESTÕES ao cliente e pode não conter o
    // imóvel negociado; era por isso que o detalhe do imóvel da negociação caía
    // em "Imóvel não encontrado" (API 200, lista sem o imóvel).
    mocks.getDealProperty.mockResolvedValue({ ...imovel });
    mocks.searchProperties.mockResolvedValue([{ id: "outro-imovel", title: "Sugestão" }]);

    render(page());

    expect(await screen.findByText(imovel.title)).toBeInTheDocument();
    expect(mocks.getDealProperty).toHaveBeenCalledWith("deal-1");
    expect(screen.queryByText("Imóvel não encontrado.")).not.toBeInTheDocument();
  });

  it("segue para as sugestões quando o imóvel do negócio é outro", async () => {
    mocks.getDealProperty.mockResolvedValue({ id: "imovel-do-negocio", title: "Imóvel do negócio" });
    mocks.searchProperties.mockResolvedValue([{ ...imovel }]);

    render(page());

    expect(await screen.findByText(imovel.title)).toBeInTheDocument();
    expect(mocks.searchProperties).toHaveBeenCalledWith("deal-1", {});
  });

  it("mostra erro com nova tentativa quando a leitura falha — nunca 'não encontrado'", async () => {
    mocks.getDealProperty.mockRejectedValue(new Error("401"));
    mocks.searchProperties.mockRejectedValue(new Error("401"));

    render(page());

    expect(await screen.findByText("Não foi possível carregar este imóvel.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /tentar novamente/i })).toBeInTheDocument();
    expect(screen.queryByText("Imóvel não encontrado.")).not.toBeInTheDocument();
  });

  it("exibe a foto quando o dashboard entrega a recomendação crua no state", async () => {
    mocks.activeDeal = null;
    render(page({
      property: {
        id: "property-1",
        title: "Casa do dashboard",
        thumbnail_url: "https://files.example/capa.jpg",
        media_images: ["https://files.example/capa.jpg"],
        listing_type: "sale",
      },
      leadId: "lead-site",
    }));

    expect(await screen.findByText("Casa do dashboard")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Casa do dashboard" })).toBeInTheDocument();
    expect(screen.queryByText("Fotos em preparação")).not.toBeInTheDocument();
  });

  it("exibe a foto do imóvel recomendado mesmo sem estado de navegação", async () => {
    mocks.activeDeal = null;
    mocks.getOverview.mockReturnValue([{
      id: "lead-site",
      tenant_id: "tenant-1",
      name: "Cliente do site",
      has_preferences: true,
      role: "buyer",
      recommended_property_id: "property-1",
      recommended_property: {
        id: "property-1",
        title: "Casa do site",
        thumbnail_url: "https://files.example/capa.jpg",
        media_images: ["https://files.example/capa.jpg"],
        listing_type: "sale",
        sale_price: 4200000,
      },
      matched_properties: [],
    }]);

    render(page());

    expect(await screen.findByText("Casa do site")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Casa do site" })).toBeInTheDocument();
    expect(screen.queryByText("Fotos em preparação")).not.toBeInTheDocument();
  });

  it("apresenta galeria, características, comodidades e descrição do cadastro", async () => {
    mocks.getDealProperty.mockResolvedValue({ ...imovel });

    render(page());

    // Galeria com contador e miniaturas.
    expect(await screen.findByText("Apartamento no Centro")).toBeInTheDocument();
    expect(screen.getByText("1/3")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Ver foto 2: Sala de estar/ })).toBeInTheDocument();
    expect(screen.getByText("Fachada")).toBeInTheDocument();

    // Características e comodidades legíveis.
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("quartos")).toBeInTheDocument();
    // Rótulo no singular quando o cadastro traz uma suíte.
    expect(screen.getByText("suíte")).toBeInTheDocument();
    expect(screen.getByText("120 m²")).toBeInTheDocument();
    expect(screen.getByText("100 m²")).toBeInTheDocument();
    expect(screen.getByText("Piscina")).toBeInTheDocument();
    expect(screen.getByText("Sala de estar")).toBeInTheDocument();
    expect(screen.getByText("Apartamento reformado, sol da manhã.")).toBeInTheDocument();
  });

  it("não anuncia valor de venda/aluguel no detalhe do imóvel", async () => {
    // O cadastro TEM preço (R$ 450.000, venda), mas o detalhe do portal não
    // anuncia valor: quem conduz o valor é a negociação, não o anúncio.
    mocks.getDealProperty.mockResolvedValue({ ...imovel, price: 450000, sale_price: 450000, rent_price: 2500, listing_type: "sale" });

    render(page());

    expect(await screen.findByText("Apartamento no Centro")).toBeInTheDocument();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
    expect(screen.queryByText("Preço sob consulta")).not.toBeInTheDocument();
    expect(screen.queryByText(/VENDA/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Aluguel/i)).not.toBeInTheDocument();
  });

  it("sem endereço liberado, mostra o bairro e explica como liberar (privacidade)", async () => {
    mocks.getDealProperty.mockResolvedValue({ ...imovel, address: "" });

    render(page());

    expect(await screen.findByText("Centro, Curitiba, PR")).toBeInTheDocument();
    expect(screen.getByText("Endereço completo liberado após agendar a visita.")).toBeInTheDocument();
  });

  it("com endereço liberado (imóvel do negócio), mostra a rua e o número", async () => {
    mocks.getDealProperty.mockResolvedValue({ ...imovel, address: "Rua das Flores, 120" });

    render(page());

    expect(await screen.findByText("Rua das Flores, 120")).toBeInTheDocument();
  });

  it("usa o overview quando o imóvel é a recomendação do lead e não há negócio ativo", async () => {
    mocks.activeDeal = null;
    mocks.getOverview.mockReturnValue([
      { id: "lead-1", recommended_property_id: imovel.id, recommended_property: imovel, matched_properties: [] },
    ]);
    render(page());
    expect(await screen.findByText(imovel.title)).toBeInTheDocument();
  });

  it("não afirma 'não encontrado' quando o imóvel realmente não está em nenhuma fonte, e oferece a busca", async () => {
    mocks.activeDeal = null;
    render(page());
    expect(await screen.findByText("Imóvel não encontrado.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ver imóveis disponíveis" })).toBeInTheDocument();
  });

  it("reagenda a visita existente em vez de criar outra", async () => {
    mocks.activeDeal = null;
    mocks.getOverview.mockReturnValue([{
      id: "lead-1", recommended_property_id: imovel.id, recommended_property: imovel, matched_properties: [],
      visit_request: { property_id: imovel.id, date: "2099-10-20", time: "14:00", end_time: "15:00", status: "Confirmado" },
    }]);
    mocks.updateLeadVisit.mockResolvedValue({});
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Alterar horário" }));
    expect(screen.getByText("reschedule")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Enviar horário" }));
    await waitFor(() => expect(mocks.updateLeadVisit).toHaveBeenCalledWith("lead-1", expect.objectContaining({ date: "2099-10-21", time: "16:00", end_time: "17:00" })));
    expect(mocks.requestLeadVisit).not.toHaveBeenCalled();
  });
});

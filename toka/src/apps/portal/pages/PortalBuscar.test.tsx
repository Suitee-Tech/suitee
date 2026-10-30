import type { PropsWithChildren } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PortalPropertySummary } from "../services/portal.service";
import { PortalBuscar } from "./PortalBuscar";

const mocks = vi.hoisted(() => ({
  deal: { id: "deal-a", status: "NEGOTIATION" } as any,
  getOverview: vi.fn(),
  requestLeadVisit: vi.fn(),
  searchProperties: vi.fn(),
  listAvailableProperties: vi.fn(),
  createAppointment: vi.fn(),
  toggleFavorite: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("../services/portal.service", () => ({
  portalService: {
    searchProperties: mocks.searchProperties,
    listAvailableProperties: mocks.listAvailableProperties,
    getOverview: mocks.getOverview,
    requestLeadVisit: mocks.requestLeadVisit,
  },
}));

vi.mock("../context/PortalDealsContext", () => ({
  usePortalDeals: () => ({
    deals: mocks.deal ? [mocks.deal] : [],
    activeDeal: mocks.deal,
    activeDealId: mocks.deal?.id,
    isLoading: false,
    setActiveDealId: vi.fn(),
  }),
}));

vi.mock("../hooks/useDealMutations", () => ({
  useCreateDealAppointment: () => ({
    mutateAsync: mocks.createAppointment,
  }),
  useToggleDealFavorite: () => ({
    isPending: false,
    variables: undefined,
    mutateAsync: mocks.toggleFavorite,
  }),
}));

vi.mock("../hooks/useDealResources", () => ({
  usePortalDealFavorites: () => ({ data: [] }),
}));

vi.mock("../components/AppointmentPickerDialog", () => ({
  AppointmentPickerDialog: ({ open, onSubmit }: any) => open ? <button onClick={() => void onSubmit({ date: new Date("2099-10-20T00:00:00"), startTime: "14:00", endTime: "15:30", note: " Acesso à garagem " })}>Enviar horário da visita</button> : null,
}));

vi.mock("../components/DealSwitcher", () => ({
  DealSwitcher: () => null,
}));

vi.mock("../components/PropertyMediaCarousel", () => ({
  PropertyMediaCarousel: () => null,
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: mocks.toastError,
  },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function property(id: string, title: string): PortalPropertySummary {
  return {
    id,
    title,
    address: "Rua Exemplo, 10",
    neighborhood: "Centro",
    city: "São Paulo",
    state: "SP",
    price: 1_000_000,
    listing_type: "rent",
  };
}

describe("PortalBuscar request state", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    mocks.deal = { id: "deal-a", status: "NEGOTIATION" };
    mocks.getOverview.mockReset();
    mocks.requestLeadVisit.mockReset().mockResolvedValue({});
    mocks.searchProperties.mockReset();
    mocks.listAvailableProperties.mockReset().mockResolvedValue([]);
    mocks.createAppointment.mockReset();
    mocks.toggleFavorite.mockReset();
    mocks.toastError.mockReset();
  });

  function wrapper({ children }: PropsWithChildren) {
    return (
      <MemoryRouter>
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      </MemoryRouter>
    );
  }

  it("keeps the latest search visible when an older response finishes later", async () => {
    const initialRequest = deferred<PortalPropertySummary[]>();
    const latestRequest = deferred<PortalPropertySummary[]>();
    mocks.searchProperties.mockImplementation(
      (_dealId: string, params: { q?: string }) =>
        params.q === "novo" ? latestRequest.promise : initialRequest.promise,
    );

    render(<PortalBuscar />, { wrapper });

    expect(screen.queryByText("Nenhum imóvel")).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("searchbox", { name: "Buscar imóveis" }), {
      target: { value: "novo" },
    });

    await waitFor(() => expect(mocks.searchProperties).toHaveBeenCalledTimes(2));
    await act(async () => {
      latestRequest.resolve([property("property-new", "Imóvel Novo")]);
      await latestRequest.promise;
    });

    expect(await screen.findByText("Imóvel Novo")).toBeInTheDocument();

    await act(async () => {
      initialRequest.resolve([property("property-old", "Imóvel Antigo")]);
      await initialRequest.promise;
    });

    expect(screen.getByText("Imóvel Novo")).toBeInTheDocument();
    expect(screen.queryByText("Imóvel Antigo")).not.toBeInTheDocument();
  });
  // Vitrine do cliente sem negócio aberto (revisão de acesso a imóveis,
  // 2026-09-19): a busca era `deal-scoped`, então quem só tinha lead via apenas
  // a recomendação/match. Agora a tela usa `/client-portal/properties`.
  it("sem negócio, busca na vitrine das imobiliárias do cliente", async () => {
    mocks.deal = null;
    mocks.getOverview.mockResolvedValue([{ id: "lead-comprador", has_preferences: false, role: "buyer", matched_properties: [] }]);
    mocks.listAvailableProperties.mockResolvedValue([property("imovel-vitrine", "Casa da vitrine")]);
    render(<PortalBuscar />, { wrapper });

    await waitFor(() => expect(mocks.listAvailableProperties).toHaveBeenCalledWith({ listing_type: "sale", limit: 30 }));
    expect(await screen.findByText("Casa da vitrine")).toBeInTheDocument();
    expect(mocks.searchProperties).not.toHaveBeenCalled();
  });

  it("sem negócio, não repete na vitrine o imóvel que já veio como sugestão do lead", async () => {
    mocks.deal = null;
    mocks.getOverview.mockResolvedValue([
      {
        id: "lead-1",
        has_preferences: false,
        recommended_property_id: "imovel-1",
        recommended_property: { id: "imovel-1", title: "Sugerido pelo corretor", listing_type: "sale", sale_price: 500000 },
        matched_properties: [],
      },
    ]);
    mocks.listAvailableProperties.mockResolvedValue([
      property("imovel-1", "Sugerido pelo corretor"),
      property("imovel-2", "Outro imóvel disponível"),
    ]);
    render(<PortalBuscar />, { wrapper });

    expect(await screen.findByText("Outro imóvel disponível")).toBeInTheDocument();
    expect(screen.getAllByText("Sugerido pelo corretor")).toHaveLength(1);
  });

  // "Em explorar imóveis ainda não é possível voltar facilmente" (relato do
  // usuário): a aba Imóveis é destino da barra inferior, mas também é alcançada
  // por atalhos ("Ver imóveis disponíveis", card do lead) — o cabeçalho agora
  // tem o mesmo botão Voltar das outras telas.
  it("expõe o botão Voltar no cabeçalho", async () => {
    mocks.searchProperties.mockResolvedValue([]);
    render(<PortalBuscar />, { wrapper });
    expect(await screen.findByRole("button", { name: "Voltar" })).toBeInTheDocument();
  });

  it("sends the selected property, duration and note for a deal visit", async () => {
    mocks.searchProperties.mockResolvedValue([property("selected", "Imóvel escolhido")]);
    mocks.createAppointment.mockResolvedValue({});
    render(<PortalBuscar />, { wrapper });
    await screen.findByText("Imóvel escolhido");
    fireEvent.click(screen.getByRole("button", { name: "Agendar" }));
    fireEvent.click(screen.getByRole("button", { name: "Enviar horário da visita" }));
    await waitFor(() => expect(mocks.createAppointment).toHaveBeenCalledWith(expect.objectContaining({ property_id: "selected", time: "14:00", end_time: "15:30", description: expect.stringContaining("Acesso à garagem") })));
  });
  it("sends duration and note for a lead visit without a deal", async () => {
    mocks.deal = null;
    mocks.getOverview.mockResolvedValue([{ id: "lead", name: "Lucas", has_preferences: false, recommended_property_id: "selected", recommended_property: { id: "selected", title: "Imóvel escolhido", listing_type: "sale", sale_price: 1000000 }, matched_properties: [] }]);
    render(<PortalBuscar />, { wrapper });
    await screen.findByText("Imóvel escolhido");
    fireEvent.click(screen.getByRole("button", { name: "Agendar" }));
    fireEvent.click(screen.getByRole("button", { name: "Enviar horário da visita" }));
    await waitFor(() => expect(mocks.requestLeadVisit).toHaveBeenCalledWith("lead", { property_id: "selected", date: "2099-10-20", time: "14:00", end_time: "15:30", note: "Acesso à garagem" }));
  });

});

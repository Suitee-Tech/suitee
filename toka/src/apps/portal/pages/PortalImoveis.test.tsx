import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PortalImoveis } from "./PortalImoveis";

const mocks = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getMyRentals: vi.fn(),
  getSiteListingUrl: vi.fn(),
}));

vi.mock("../services/portal.service", () => ({ portalService: mocks }));
vi.mock("../context/PortalDealsContext", () => ({ usePortalDeals: () => ({ activeDeal: null }) }));

const lead = {
  id: "lead-site",
  tenant_id: "11111111-1111-4111-8111-111111111111",
  name: "Cliente do site",
  email: "cliente@example.com",
  has_preferences: true,
  role: "buyer",
  recommended_property: {
    id: "property-1",
    title: "Casa do site",
    neighborhood: "Centro",
    city: "Atibaia",
    state: "SP",
    listing_type: "sale",
    sale_price: 4200000,
    thumbnail_url: "https://cdn.example/casa.jpg",
    media_images: ["https://cdn.example/casa.jpg"],
  },
  matched_properties: [],
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <PortalImoveis />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("PortalImoveis", () => {
  beforeEach(() => {
    mocks.getOverview.mockReset();
    mocks.getMyRentals.mockReset();
    mocks.getSiteListingUrl.mockReset();
    mocks.getSiteListingUrl.mockResolvedValue({
      published: true,
      public_url: "https://Exemplo.imoveis.suitee.com.br/imoveis/casa-do-site",
      status: "published",
    });
  });

  it("mostra o imóvel que gerou o lead sem depender das locações", async () => {
    mocks.getOverview.mockResolvedValue([lead]);
    renderPage();

    expect(await screen.findByText("Casa do site")).toBeInTheDocument();
    expect(screen.getByText("Sugestão do corretor")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /ver anúncio no site/i })).toHaveAttribute(
      "href",
      "https://Exemplo.imoveis.suitee.com.br/imoveis/casa-do-site",
    );
    expect(mocks.getSiteListingUrl).toHaveBeenCalledWith(
      "property-1",
      "11111111-1111-4111-8111-111111111111",
    );
    expect(mocks.getMyRentals).not.toHaveBeenCalled();
  });
});

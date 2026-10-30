import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PortalSiteListingLink } from "./PortalSiteListingLink";

const mocks = vi.hoisted(() => ({
  getSiteListingUrl: vi.fn(),
}));

vi.mock("../services/portal.service", () => ({ portalService: mocks }));

function renderLink(tenantId?: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <PortalSiteListingLink propertyId="property-1" tenantId={tenantId} />
    </QueryClientProvider>,
  );
}

describe("PortalSiteListingLink", () => {
  beforeEach(() => {
    mocks.getSiteListingUrl.mockReset();
  });

  it("mostra o CTA quando o imóvel está publicado na vitrine do deal", async () => {
    mocks.getSiteListingUrl.mockResolvedValue({
      published: true,
      public_url: "https://exemplo.imoveis.example.com/imoveis/apartamento-centro",
      seo_slug: "apartamento-centro",
      status: "published",
    });
    renderLink("11111111-1111-4111-8111-111111111111");
    const link = await screen.findByRole("link", { name: /ver anúncio no site/i });
    expect(link).toHaveAttribute("href", "https://exemplo.imoveis.example.com/imoveis/apartamento-centro");
    expect(mocks.getSiteListingUrl).toHaveBeenCalledWith(
      "property-1",
      "11111111-1111-4111-8111-111111111111",
    );
  });

  it("não mostra CTA quando o anúncio não está publicado", async () => {
    mocks.getSiteListingUrl.mockResolvedValue({ published: false, status: "unpublished" });
    renderLink();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByRole("link", { name: /ver anúncio no site/i })).toBeNull();
  });
});

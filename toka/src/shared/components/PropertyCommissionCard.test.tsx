import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PropertyCommissionCard } from "./PropertyCommissionCard";

const api = vi.hoisted(() => ({
  getSettings: vi.fn(),
  getRules: vi.fn(),
}));

const permissions = vi.hoisted(() => ({
  allowed: new Set<string>(["finance:read"]),
  loading: false,
}));

vi.mock("../services/tenantFinancialSettings.service", () => ({
  tenantFinancialSettingsService: { get: api.getSettings },
}));

vi.mock("../services/commissionRules.service", () => ({
  commissionRulesService: { get: api.getRules },
}));

vi.mock("../context/PermissionsContext", () => ({
  usePermissionsContext: () => ({
    can: (permission: string) => permissions.allowed.has(permission),
    isLoading: permissions.loading,
  }),
}));

const settings = {
  tenant_id: "tenant-1",
  default_late_fee_pct: 2,
  default_monthly_interest_pct: 1,
  default_due_day: 10,
  default_commission_pct_rent: 100,
  default_commission_pct_sale: 5,
  default_currency: "BRL",
  payout_proof_required: true,
  admin_fee_pct: 10,
  default_lease_term_months: 30,
  default_inss_pct: 0,
  default_iss_pct: 0,
  updated_at: "2026-10-01T12:00:00Z",
};

const rule = (role: string, default_percentage: number) => ({
  tenant_id: "tenant-1",
  role,
  default_percentage,
});

function renderCard(
  listingType?: "sale" | "rent" | "exchange",
  availableFor?: ("sale" | "rent" | "exchange")[],
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <PropertyCommissionCard listingType={listingType} availableFor={availableFor} />
    </QueryClientProvider>,
  );
}

describe("PropertyCommissionCard", () => {
  beforeEach(() => {
    api.getSettings.mockReset();
    api.getRules.mockReset();
    permissions.allowed = new Set<string>(["finance:read"]);
    permissions.loading = false;
  });

  it("renders the commission configured by the agency, not a hardcoded split", async () => {
    api.getSettings.mockResolvedValue(settings);
    api.getRules.mockResolvedValue([
      rule("captador", 30),
      rule("co_captador", 0),
      rule("visitador", 10),
      rule("atendente", 0),
      rule("fechamento", 30),
      rule("imobiliaria", 30),
    ]);

    renderCard("sale");

    expect(await screen.findByText("Comissão (5% total)")).toBeInTheDocument();
    expect(screen.getByText("30% imobiliária / 70% corretor")).toBeInTheDocument();
    expect(screen.getByText("Padrão da imobiliária para venda.")).toBeInTheDocument();
  });

  it("uses the rental default and says the percentage is over the first rent", async () => {
    api.getSettings.mockResolvedValue({ ...settings, default_commission_pct_rent: 100 });
    api.getRules.mockResolvedValue([rule("captador", 0), rule("fechamento", 100)]);

    renderCard("rent");

    expect(await screen.findByText("Comissão (100% do 1º aluguel)")).toBeInTheDocument();
    expect(screen.getByText("100% corretor")).toBeInTheDocument();
    expect(screen.getByText("Padrão da imobiliária para locação.")).toBeInTheDocument();
  });

  it("labels the card for rent when the property is offered only for rent", async () => {
    // Regressão: listing_type "sale" (default do mapeamento) fazia um imóvel
    // só de locação exibir "Padrão da imobiliária para venda".
    api.getSettings.mockResolvedValue(settings);
    api.getRules.mockResolvedValue([rule("captador", 100)]);

    renderCard("sale", ["rent"]);

    expect(await screen.findByText("Comissão (100% do 1º aluguel)")).toBeInTheDocument();
    expect(screen.getByText("Padrão da imobiliária para locação.")).toBeInTheDocument();
    expect(screen.queryByText(/para venda/)).not.toBeInTheDocument();
  });

  it("explains an unconfigured distribution instead of showing 0% / 0%", async () => {
    api.getSettings.mockResolvedValue(settings);
    api.getRules.mockResolvedValue([rule("imobiliaria", 0)]);

    renderCard("sale");

    expect(await screen.findByText("Sem distribuição por papel")).toBeInTheDocument();
    expect(screen.queryByText(/0% imobiliária/)).not.toBeInTheDocument();
  });

  it("admits a read failure instead of assuming the default split", async () => {
    api.getSettings.mockRejectedValue(new Error("network down"));
    api.getRules.mockResolvedValue([]);

    renderCard("sale");

    // A leitura tenta uma vez antes de desistir (retry: 1 no hook).
    expect(
      await screen.findByText("Não foi possível carregar", {}, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/40% imobiliária/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /tentar novamente/i })).toBeInTheDocument();
  });

  it("does not request the settings for profiles without financial access", () => {
    permissions.allowed = new Set<string>();

    renderCard("sale");

    expect(screen.getByText("Definida pela imobiliária")).toBeInTheDocument();
    expect(api.getSettings).not.toHaveBeenCalled();
    expect(api.getRules).not.toHaveBeenCalled();
  });
});

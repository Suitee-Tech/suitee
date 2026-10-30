import { test, expect, type Page } from "@playwright/test";

const user = {
  id: "client-1",
  email: "cliente@example.test",
  firstname: "Diego",
  lastname: "Santos",
  role: "user",
  functional_role: "cliente",
  account_type: "individual",
  permission: "owner",
  current_company_id: "account-company",
};

const AGENCIES: Record<string, string> = {
  "tenant-alfa": "Imobiliária Alfa",
  "tenant-beta": "Imobiliária Beta",
};

function property(id: string, title: string, city: string) {
  return {
    id,
    title,
    neighborhood: "Centro",
    city,
    state: "SP",
    listing_type: "sale",
    sale_price: 700000,
    rent_price: 0,
    thumbnail_url: "",
    media_images: [],
  };
}

function lead(overrides: Record<string, unknown>) {
  const recommended = property(`property-${String(overrides.tenant_id)}`, String(overrides.title || "Imóvel"), "Atibaia");
  return {
    id: `lead-${String(overrides.tenant_id)}`,
    tenant_id: overrides.tenant_id,
    name: "Diego Santos",
    email: user.email,
    phone: "11999999999",
    role: "buyer",
    status: "CONTACTED",
    created_at: "2026-10-20T10:00:00Z",
    updated_at: "2026-10-25T10:00:00Z",
    last_interaction: "2026-10-25T10:00:00Z",
    has_preferences: true,
    recommended_property_id: recommended.id,
    recommended_property: recommended,
    broker_name: overrides.broker_name || "Corretor",
    matched_properties: [],
    visit_request: overrides.visit_request,
    ...overrides,
  };
}

function deal(overrides: Record<string, unknown> = {}) {
  return {
    id: "deal-1",
    display_id: "NEG-0001",
    type: "SALE",
    status: "NEGOTIATION",
    property_id: "property-deal-1",
    owner_id: "owner-1",
    customer_id: "lead-client",
    broker_id: "broker-1",
    value: 700000,
    commission: 35000,
    tenant_id: "tenant-alfa",
    created_at: "2026-10-20T10:00:00Z",
    updated_at: "2026-10-25T10:00:00Z",
    my_party_id: "party-1",
    my_role: "buyer",
    parties: [],
    proposals: [],
    checklist: [],
    ...overrides,
  };
}

async function seedPortal(page: Page, input: {
  leads?: unknown[];
  deals?: unknown[];
  overviewStatus?: number;
  siteName?: (tenantId: string) => string;
  properties?: unknown[];
}) {
  await page.addInitScript(() => window.localStorage.setItem("auth_has_session", "1"));
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/api/v1", "");
    let data: unknown = [];
    if (/^\/auth\/refresh/.test(path) || /^\/auth\/portal\/refresh/.test(path)) data = { access_token: "synthetic" };
    if (path === "/auth/me") data = user;
    if (path === "/auth/companies") data = [];
    if (path === "/me/permissions") data = { functional_role: "cliente", profile: "cliente", permissions: [] };
    if (path === "/settings/preferences") data = { last_company_id: null, consent: { terms_version: "2.0" } };
    if (path === "/client-portal/deals") data = input.deals ?? [];
    if (path === "/client-portal/properties") data = input.properties ?? [];
    if (path === "/client-portal/overview") {
      if (input.overviewStatus && input.overviewStatus >= 400) {
        await route.fulfill({ status: input.overviewStatus, json: { success: false, message: "overview unavailable" } });
        return;
      }
      data = input.leads ?? [];
    }
    if (path === "/sites/portal/site") {
      const tenantId = url.searchParams.get("tenant_id") || "";
      data = {
        name: input.siteName?.(tenantId) || AGENCIES[tenantId] || "Imobiliária",
        slug: tenantId,
        public_url: `https://${tenantId}.example.test`,
        branding: { primary_color: "#1C905B" },
      };
    }
    if (path === "/sites/portal/listing-url") data = { published: true, public_url: "https://site.example.test/imovel" };
    if (path === "/client-errors") { await route.fulfill({ status: 204 }); return; }
    if (/^\/client-portal\/deals\/[^/]+\/property$/.test(path)) {
      const dealId = path.split("/")[3];
      data = {
        id: dealId === "deal-2" ? "property-deal-2" : "property-deal-1",
        title: dealId === "deal-2" ? "Casa do Negócio 2" : "Casa do Negócio 1",
        address: "Rua das Flores, 100",
        neighborhood: "Centro",
        city: "Atibaia",
        state: "SP",
        listing_type: "sale",
        price: 700000,
        media_images: [],
        image_url: "",
      };
    }
    if (/^\/client-portal\/deals\/[^/]+\/recommendations$/.test(path)) data = [];
    if (/^\/client-portal\/deals\/[^/]+\/checklist$/.test(path)) data = [];
    if (/^\/client-portal\/deals\/[^/]+$/.test(path)) {
      const id = path.split("/").pop();
      data = (input.deals ?? []).find((d) => (d as { id: string }).id === id) ?? input.deals?.[0] ?? null;
    }
    if (route.request().method() !== "GET" && !/^\/auth\/(refresh|portal\/refresh|switch-)/.test(path)) {
      await route.fallback();
      return;
    }
    const list = Array.isArray(data);
    await route.fulfill({
      json: { success: true, statusCode: 200, data, meta: { total: list ? (data as unknown[]).length : 1 } },
    });
  });
}

test.describe("portal: contexto de relacionamento", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("lead-only: identifica imobiliária, etapa, múltiplos atendimentos e detalhe", async ({ page }) => {
    const leads = [
      lead({ tenant_id: "tenant-alfa", title: "Casa Alfa", broker_name: "Ana Alfa" }),
      lead({
        tenant_id: "tenant-beta",
        title: "Apartamento Beta",
        broker_name: "Beto Beta",
        status: "VISIT_SCHEDULED",
        visit_request: { date: "2026-10-28", time: "10:00", end_time: "11:00", status: "Confirmado" },
      }),
    ];
    await seedPortal(page, { leads });
    await page.goto("/portal");

    await expect(page.getByText("Olá Diego")).toBeVisible();
    await expect(page.getByText("Imobiliária Alfa")).toBeVisible();
    await expect(page.getByText("Em atendimento").first()).toBeVisible();
    await expect(page.getByText("Ver atendimento completo")).toBeVisible();

    await page.getByRole("button", { name: /Meus atendimentos:/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("heading", { name: "Meus atendimentos" })).toBeVisible();
    await expect(dialog.getByText("Casa Alfa")).toBeVisible();
    await expect(dialog.getByText("Apartamento Beta")).toBeVisible();
    await expect(dialog.getByText("Imobiliária Beta")).toBeVisible();

    await dialog.getByRole("button", { name: /Apartamento Beta/ }).click();
    await expect(page.getByText("Visita agendada").first()).toBeVisible();
    await expect(page.getByText("Apartamento Beta").first()).toBeVisible();

    await page.goto("/portal/leads");
    await expect(page.getByRole("heading", { name: "Meus atendimentos" })).toBeVisible();
    await expect(page.getByText("Imobiliária Alfa")).toBeVisible();
    await expect(page.getByText("Imobiliária Beta")).toBeVisible();
    await page.getByRole("link", { name: "Abrir atendimento" }).first().click();
    await expect(page).toHaveURL(/\/portal\/atendimentos\//);
    await expect(page.getByRole("heading", { name: "Atendimento", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Voltar" })).toHaveCount(1);
    await expect(page.getByText("Etapas do atendimento")).toBeVisible();
    await expect(page.getByText("Imobiliária Alfa")).toBeVisible();
  });

  test("negócio + atendimento: a home mantém os dois contextos", async ({ page }) => {
    const currentDeal = deal();
    const otherLead = lead({ tenant_id: "tenant-beta", title: "Casa Beta", broker_name: "Beto Beta" });
    await seedPortal(page, { leads: [otherLead], deals: [currentDeal] });
    await page.goto("/portal");

    await expect(page.getByText("Seus outros atendimentos")).toBeVisible();
    await expect(page.getByText("Casa Beta")).toBeVisible();
    await expect(page.getByRole("button", { name: /Minhas relações:/i })).toBeVisible();

    await page.getByRole("button", { name: /Minhas relações:/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Negócios em andamento")).toBeVisible();
    await expect(dialog.getByText("Atendimentos", { exact: true })).toBeVisible();
    await expect(dialog.getByText("Casa do Negócio 1")).toBeVisible();
    await expect(dialog.getByText("Casa Beta")).toBeVisible();
    await expect(dialog.getByText("Imobiliária Alfa")).toBeVisible();
    await expect(dialog.getByText("Imobiliária Beta")).toBeVisible();
  });

  test("dois negócios multiempresa são distinguíveis no seletor", async ({ page }) => {
    const first = deal({ id: "deal-1", display_id: "NEG-0001", tenant_id: "tenant-alfa", property_id: "property-deal-1" });
    const second = deal({
      id: "deal-2",
      display_id: "NEG-0002",
      tenant_id: "tenant-beta",
      property_id: "property-deal-2",
      status: "PROPOSAL",
    });
    await seedPortal(page, { leads: [], deals: [first, second] });
    await page.goto("/portal");

    await page.getByRole("button", { name: /Minhas relações:/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Casa do Negócio 1")).toBeVisible();
    await expect(dialog.getByText("Casa do Negócio 2")).toBeVisible();
    await expect(dialog.getByText("Imobiliária Alfa")).toBeVisible();
    await expect(dialog.getByText("Imobiliária Beta")).toBeVisible();
  });


  test("busca lead-only permite filtrar por tipo, cidade e preço", async ({ page }) => {
    const leads = [lead({ tenant_id: "tenant-alfa", title: "Casa Alfa" })];
    await seedPortal(page, {
      leads,
      properties: [
        { id: "p-house", title: "Casa Filtrada", property_type: "house", listing_type: "sale", price: 500000, city: "Atibaia", neighborhood: "Centro", state: "SP", media_images: [] },
        { id: "p-apt", title: "Apartamento Filtrado", property_type: "apartment", listing_type: "sale", price: 900000, city: "Campinas", neighborhood: "Cambuí", state: "SP", media_images: [] },
      ],
    });
    await page.goto("/portal/buscar");

    await expect(page.locator("#portal-filter-type")).toBeVisible();
    await page.locator("#portal-filter-type").selectOption("house");
    await expect(page.getByText("Casa Filtrada")).toBeVisible();
    await expect(page.getByText("Apartamento Filtrado")).toHaveCount(0);

    await page.locator("#portal-filter-type").selectOption("");
    await page.locator("#portal-filter-city").fill("Campinas");
    await expect(page.getByText("Apartamento Filtrado")).toBeVisible();
    await expect(page.getByText("Casa Filtrada")).toHaveCount(0);

    await page.locator("#portal-filter-city").fill("");
    await page.locator("#portal-filter-max").fill("600000");
    await expect(page.getByText("Casa Filtrada")).toBeVisible();
    await expect(page.getByText("Apartamento Filtrado")).toHaveCount(0);
  });

  test("estado vazio oferece próximo passo", async ({ page }) => {
    await seedPortal(page, { leads: [], deals: [] });
    await page.goto("/portal/leads");
    await expect(page.getByText("Nenhum atendimento ainda")).toBeVisible();
    await expect(page.getByRole("link", { name: "Explorar imóveis" })).toBeVisible();
  });
});

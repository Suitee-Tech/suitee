import { test, expect, type Page } from '@playwright/test';

// UX Writer por persona no portal: o mesmo negócio, quatro clientes diferentes.
// O texto não pode induzir o fluxo do outro lado — quem anuncia não "explora
// imóveis" nem "agenda visita ao próprio imóvel"; quem procura não lê "meus
// repasses" nem "minhas locações".

const user = {
  id: 'fake-user-id', email: 'cliente@example.test', firstname: 'Diego', lastname: 'Santos',
  role: 'user', functional_role: 'cliente', account_type: 'individual',
  permission: 'owner', current_company_id: 'company-1',
};

type DealOverrides = { type?: string; my_role?: string };

function deal(overrides: DealOverrides = {}) {
  return {
    id: 'deal-1', display_id: 'NEG-0001', type: 'SALE', status: 'NEGOTIATION',
    property_id: 'prop-casa', owner_id: 'lead-1', customer_id: 'lead-1',
    broker_id: 'broker-1', value: 1400000, commission: 70000, tenant_id: 'company-1',
    created_at: '2026-09-30T10:00:00Z', updated_at: '2026-09-30T10:00:00Z',
    my_party_id: 'party-1', parties: [], proposals: [], checklist: [],
    ...overrides,
  };
}

const PROPERTY = {
  id: 'prop-casa', title: 'Casa Vitória Régia', address: 'Rua Vitória Régia, 152',
  neighborhood: 'Vila Gardênia', city: 'Atibaia', state: 'SP',
  listing_type: 'sale', price: 1400000, bedrooms: 3, bathrooms: 3, total_area_m2: 300,
  image_url: '/e2e-fixtures/fachada.jpg', images: [{ url: '/e2e-fixtures/fachada.jpg', description: 'Fachada' }],
};

const RECOMMENDATION = {
  id: 'prop-rec', title: 'Sobrado no Centro', neighborhood: 'Centro', city: 'Atibaia', state: 'SP',
  listing_type: 'sale', price: 900000, bedrooms: 3, total_area_m2: 180,
  image_url: '/e2e-fixtures/sobrado.jpg', match_score: 88, match_reasons: { price: 1, location: 1, used_weights: { price: 1, location: 1 } },
};

async function seedPortal(page: Page, options: { currentDeal: ReturnType<typeof deal>; recommendations?: unknown[] }) {
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || path === '/auth/portal/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'synthetic' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [];
    if (path === '/me/permissions') data = { functional_role: 'cliente', profile: 'cliente', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/client-portal/deals') data = [options.currentDeal];
    if (path === `/client-portal/deals/${options.currentDeal.id}`) data = options.currentDeal;
    if (path === `/client-portal/deals/${options.currentDeal.id}/property`) data = PROPERTY;
    if (path === `/client-portal/deals/${options.currentDeal.id}/recommendations`) data = options.recommendations ?? [];
    if (path === '/client-portal/overview') data = [];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|portal\/refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    const list = Array.isArray(data);
    await route.fulfill({
      json: { success: true, statusCode: 200, data, meta: { total: list ? (data as unknown[]).length : 1 } },
    });
  });
}

test.describe('portal: texto por persona', () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test('comprador: fluxo de busca e proposta, sem vocabulário de locação', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'SALE', my_role: 'buyer' }), recommendations: [RECOMMENDATION] });
    await page.goto('/portal');

    await expect(page.getByText('Sua compra')).toBeVisible();
    await expect(page.getByText('Comprador', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Explorar imóveis' })).toBeVisible();
    await expect(page.getByText('Selecionados para você')).toBeVisible();

    // Ações do outro lado não aparecem para quem compra.
    await expect(page.getByText('Meus repasses')).toHaveCount(0);
    await expect(page.getByText('Meus recebimentos')).toHaveCount(0);
    await expect(page.getByText('Minhas locações')).toHaveCount(0);

    // Navegação leva à busca de imóveis.
    await expect(page.getByRole('link', { name: 'Imóveis', exact: true })).toBeVisible();
  });

  test('locatário: boletos e manutenção, sem repasses', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'RENT', my_role: 'tenant' }) });
    await page.goto('/portal');

    await expect(page.getByText('Sua locação')).toBeVisible();
    await expect(page.getByText('Locatário', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Meus boletos' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Manutenção e chamados' })).toBeVisible();

    await expect(page.getByText('Meus repasses')).toHaveCount(0);
    await expect(page.getByText('Meus recebimentos')).toHaveCount(0);
  });

  test('vendedor: acompanha a venda e recebe, não explora imóveis', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'SALE', my_role: 'owner' }), recommendations: [RECOMMENDATION] });
    await page.goto('/portal');

    await expect(page.getByText('Venda do seu imóvel')).toBeVisible();
    await expect(page.getByText('Vendedor', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Meus recebimentos' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Meus imóveis' }).first()).toBeVisible();

    // Nada de fluxo de quem procura.
    await expect(page.getByRole('link', { name: 'Explorar imóveis' })).toHaveCount(0);
    await expect(page.getByText('Selecionados para você')).toHaveCount(0);
    await expect(page.getByText('Minhas locações')).toHaveCount(0);

    // A aba de imóveis do rodapé aponta para a carteira dele.
    await expect(page.getByRole('link', { name: 'Meus imóveis' }).last()).toHaveAttribute('href', '/portal/imoveis');
  });

  test('locador: repasses, locações e manutenção — e nenhum "agendar visita" no próprio imóvel', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'RENT', my_role: 'owner' }) });
    await page.goto('/portal');

    await expect(page.getByText('Locação do seu imóvel')).toBeVisible();
    await expect(page.getByText('Locador', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Meus repasses' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Minhas locações' })).toBeVisible();

    await expect(page.getByRole('link', { name: 'Explorar imóveis' })).toHaveCount(0);
    await expect(page.getByText('Meus boletos')).toHaveCount(0);

    // Detalhe do imóvel: quem anuncia o imóvel não agenda visita a ele.
    await page.goto('/portal/imoveis/prop-casa');
    await expect(page.getByText('Casa Vitória Régia')).toBeVisible();
    await expect(page.getByText('As visitas ao seu imóvel são agendadas pela imobiliária com os interessados.')).toBeVisible();
    await expect(page.getByRole('button', { name: /Agendar visita/ })).toHaveCount(0);
  });

  test('comprador e locatário continuam agendando visita no detalhe do imóvel', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'SALE', my_role: 'buyer' }) });
    await page.goto('/portal/imoveis/prop-casa');

    await expect(page.getByText('Casa Vitória Régia')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Agendar visita' })).toBeVisible();
    await expect(page.getByText('As visitas ao seu imóvel são agendadas pela imobiliária com os interessados.')).toHaveCount(0);
  });

  // O detalhe do negócio repetia "Agendar visita" na "Próxima ação" para todo
  // mundo — inclusive para quem anuncia o imóvel.
  test('detalhe do negócio: comprador age, vendedor acompanha a visita da imobiliária', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'SALE', my_role: 'buyer' }) });
    await page.goto('/portal/deals/deal-1');

    await expect(page.getByText('Conheça o imóvel de perto')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Agendar visita' }).first()).toBeVisible();
  });

  test('detalhe do negócio do vendedor não oferece agendar visita', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'SALE', my_role: 'owner' }) });
    await page.goto('/portal/deals/deal-1');

    await expect(page.getByText('Visitas ao seu imóvel').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Acompanhar visitas' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Agendar visita/ })).toHaveCount(0);
  });

  test('detalhe da locação do locador não oferece agendar visita', async ({ page }) => {
    await seedPortal(page, { currentDeal: deal({ type: 'RENT', my_role: 'owner' }) });
    await page.goto('/portal/deals/deal-1');

    await expect(page.getByText('Visitas ao seu imóvel').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Acompanhar visitas' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Agendar visita/ })).toHaveCount(0);
  });
});

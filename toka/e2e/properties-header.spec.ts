import { test, expect, type Page } from '@playwright/test';

const USER = {
  id: 'fake-user-id',
  email: 'qa@example.test',
  firstname: 'Ana',
  lastname: 'QA',
  role: 'user',
  functional_role: 'admin',
  account_type: 'individual',
  permission: 'owner',
  current_company_id: 'company-1',
};

const COMPANY = {
  id: 'company-1',
  type: 'company',
  company: {
    id: 'company-1',
    name: 'Imobiliária QA',
    type: 'imobiliaria',
    is_active: true,
    owner_id: USER.id,
    members: [{ account_id: USER.id, profile: 'owner' }],
  },
};

async function boot(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('auth_has_session', '1');
    localStorage.setItem('current_company_id', 'company-1');
  });
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    const json = (data: unknown) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 200, data, meta: { total: Array.isArray(data) ? data.length : 1 } }),
      });
    if (path === '/auth/refresh' || path.startsWith('/auth/switch')) return json({ access_token: 'synthetic-token' });
    if (path === '/auth/me') return json(USER);
    if (path === '/auth/companies') return json([COMPANY]);
    if (path === '/me/permissions') return json({ functional_role: 'admin', profile: 'owner', permissions: [] });
    if (path === '/settings/preferences') return json({ consent: { terms_version: '2.0' } });
    if (path === '/company-settings/appearance') return json({ theme: 'light', primaryColor: '#1C905B', brandName: 'QA', darkModeEnabled: true });
    if (path === '/company-settings') return json({});
    if (path === '/properties') return json([]);
    return json([]);
  });
}

for (const width of [1024, 1280, 1440]) {
  test(`cabeçalho de imóveis preserva título e CTA em ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await boot(page);
    await page.goto('/imoveis');
    await expect(page.getByRole('heading', { name: 'Estoque de Imóveis' })).toBeVisible();

    const title = page.getByRole('heading', { name: 'Estoque de Imóveis' });
    const cta = page.getByRole('button', { name: 'Novo Cadastro' });
    await expect(cta).toBeVisible();

    const titleBox = await title.boundingBox();
    const ctaBox = await cta.boundingBox();
    expect(titleBox).not.toBeNull();
    expect(ctaBox).not.toBeNull();
    // Título em uma linha e CTA inteiro dentro da viewport — a regressão era
    // 3 linhas em 96px e o botão cortado na borda direita em 1280.
    expect(titleBox!.height).toBeLessThan(48);
    expect(ctaBox!.x + ctaBox!.width).toBeLessThanOrEqual(width);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
}

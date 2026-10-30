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
    return json([]);
  });
}

test.describe('Navegação acessível do admin', () => {
  test('cada rota atualiza o título do documento', async ({ page }) => {
    await boot(page);
    await page.goto('/leads');
    await expect(page).toHaveTitle('Leads (CRM) · suitee');

    await page.goto('/imoveis');
    await expect(page).toHaveTitle('Imóveis · suitee');

    await page.goto('/configuracoes');
    await expect(page).toHaveTitle('Configurações · suitee');
  });

  test('skip link é o primeiro foco e leva ao conteúdo', async ({ page }) => {
    await boot(page);
    await page.goto('/leads');
    await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();

    await page.keyboard.press('Tab');
    const firstFocus = await page.evaluate(() => document.activeElement?.textContent?.trim());
    expect(firstFocus).toBe('Ir para o conteúdo');

    await page.keyboard.press('Enter');
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe('admin-content');
  });

  test('drawer mobile prende o foco, fecha com Escape e devolve o foco', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await boot(page);
    await page.goto('/leads');
    await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();

    const trigger = page.getByRole('button', { name: 'Abrir menu de navegação' });
    await trigger.click();
    const menu = page.getByRole('dialog', { name: 'Menu de navegação' });
    await expect(menu).toBeVisible();
    await expect(page.getByRole('button', { name: 'Fechar menu' })).toBeFocused();

    await page.keyboard.press('Shift+Tab');
    expect(await page.evaluate(() => Boolean(document.activeElement?.closest('#admin-sidebar')))).toBe(true);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: 'Fechar menu' })).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
    expect(await page.locator('#admin-sidebar').getAttribute('inert')).not.toBeNull();
    expect(await page.locator('#admin-sidebar').getAttribute('aria-hidden')).toBe('true');
  });
});

test.describe('Menu financeiro', () => {
  test('recolhe e expande os 12 destinos sem perder o estado acessível', async ({ page }) => {
    await boot(page);
    await page.goto('/financeiro');

    const toggle = page.getByRole('button', { name: /Financeiro/ });
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByRole('link', { name: 'Lançamentos', exact: true })).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect(page.getByRole('link', { name: 'Lançamentos', exact: true })).toBeHidden();

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByRole('link', { name: 'Lançamentos', exact: true })).toBeVisible();
  });
});

test.describe('Ordens de pagamento — modos de visualização', () => {
  test('alterna entre padrão e listagem simples com estado atual visível', async ({ page }) => {
    await boot(page);
    await page.goto('/financeiro/payouts');

    const tableMode = page.getByRole('link', { name: 'Padrão' });
    const listMode = page.getByRole('link', { name: 'Listagem simples' });

    await expect(tableMode).toHaveAttribute('aria-current', 'page');
    await expect(listMode).not.toHaveAttribute('aria-current', 'page');

    await listMode.click();
    await expect(page).toHaveURL(/\/financeiro\/payouts-lista$/);
    await expect(listMode).toHaveAttribute('aria-current', 'page');
    await expect(tableMode).not.toHaveAttribute('aria-current', 'page');

    await tableMode.click();
    await expect(page).toHaveURL(/\/financeiro\/payouts$/);
    await expect(tableMode).toHaveAttribute('aria-current', 'page');
  });
});

test.describe('Central de Ajuda — tabelas', () => {
  test('tabela de variáveis rola dentro do artigo e não vaza no mobile', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await boot(page);
    await page.goto('/ajuda?t=variaveis-contrato');

    await expect(page.getByRole('heading', { name: 'Variáveis de Contrato' })).toBeVisible();
    const table = page.locator('table').first();
    await expect(table).toBeVisible();

    const overflow = await page.evaluate(() => ({
      doc: document.documentElement.scrollWidth,
      viewport: document.documentElement.clientWidth,
    }));
    expect(overflow.doc).toBeLessThanOrEqual(overflow.viewport + 1);

    const tableScroll = await table.evaluate((element) => {
      const wrapper = element.parentElement as HTMLElement;
      return { scrollWidth: wrapper.scrollWidth, clientWidth: wrapper.clientWidth };
    });
    expect(tableScroll.scrollWidth).toBeGreaterThan(tableScroll.clientWidth);
  });
});

test.describe('Suitee Intelligence — layout', () => {
  test('abre sem deslocar a página e mantém cabeçalho e composer', async ({ page }) => {
    await page.setViewportSize({ width: 1722, height: 962 });
    await boot(page);
    await page.goto('/roleta');
    await page.getByRole('button', { name: /Assistente de inteligência/ }).click();

    await expect(page.getByRole('heading', { name: 'Suitee Intelligence' })).toBeVisible();
    await expect(page.getByRole('textbox')).toBeVisible();
    const overflow = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth + 1);
    await page.getByRole('button', { name: 'Fechar inteligência' }).click();
    await expect(page.getByRole('heading', { name: 'Suitee Intelligence' })).toBeHidden();
  });

  test('no mobile o painel ocupa a tela sem overflow da página', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await boot(page);
    await page.goto('/roleta');
    await page.getByRole('button', { name: /Assistente de inteligência/ }).click();

    await expect(page.getByRole('heading', { name: 'Suitee Intelligence' })).toBeVisible();
    await expect(page.getByRole('textbox')).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(1);
  });
});

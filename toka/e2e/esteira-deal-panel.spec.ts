import { test, expect, type Page } from '@playwright/test';

// Painel lateral de um item da Esteira no celular.
//
// Relato com print no app real: os controles do cabeçalho ultrapassavam a
// largura da tela e o botão de FECHAR ficava fora da área visível — sem ele não
// há como sair do painel (o painel ocupa a tela inteira no mobile).
//
// Medição no navegador: qual controle passa da viewport, se o fechar está
// alcançável e se a linha de abas rola em vez de cortar.

const TEST_USER = {
  id: 'fake-user-id', email: 'maria@example.com', firstname: 'Caua', lastname: 'Teste',
  role: 'user', functional_role: 'admin', account_type: 'individual',
  permission: 'owner', current_company_id: 'exemplo-company-id',
};
const TEST_COMPANY = {
  id: 'exemplo-company-id', type: 'company', company: {
    id: 'exemplo-company-id', name: 'Exemplo Imóveis', type: 'imobiliaria', is_active: true,
    owner_id: TEST_USER.id, members: [{ account_id: TEST_USER.id, profile: 'owner' }],
  },
};
const LEAD = {
  id: 'lead-diego', name: 'Diego Santos', email: 'diego@example.test',
  phone: '11999998888', document: '12345678901', status: 'Em Atendimento', role: 'buyer',
};
const PROPERTY = { id: 'prop-casa', title: 'Casa Praça Santina', city: 'Atibaia', state: 'SP', sale_price: 1400000 };
const DEAL = {
  id: 'ea105a98-38ee-4877-8eed-ef84b9312b0d',
  display_id: 'NEG-0001',
  type: 'SALE',
  status: 'PROPOSAL',
  property_id: PROPERTY.id,
  owner_id: LEAD.id,
  customer_id: LEAD.id,
  lead_id: LEAD.id,
  broker_id: 'broker-1',
  broker_participations: [{ broker_id: 'broker-1', role: 'captador' }],
  value: 1400000,
  commission: 70000,
  tenant_id: TEST_COMPANY.id,
  created_at: '2026-09-30T10:00:00Z',
  updated_at: '2026-09-30T10:00:00Z',
  proposals: [],
  parties: [
    { id: 'party-buyer', deal_id: 'x', role: 'buyer', is_primary: true, name: LEAD.name },
    { id: 'party-owner', deal_id: 'x', role: 'owner', is_primary: true, name: 'Eduardo Fernandes' },
  ],
};

async function seedEsteira(page: Page) {
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'fake' };
    if (path === '/auth/me') data = TEST_USER;
    if (path === '/auth/companies') data = [TEST_COMPANY];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/deals' || path === '/deals/') data = [DEAL];
    if (path === `/deals/${DEAL.id}`) data = DEAL;
    if (path === '/leads') data = [LEAD];
    if (path === '/properties') data = [PROPERTY];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    const list = Array.isArray(data);
    await route.fulfill({
      json: {
        success: true, statusCode: 200, data,
        meta: { total: list ? (data as unknown[]).length : 1, page: 1, limit: url.searchParams.get('limit') ?? 100, totalPages: 1 },
      },
    });
  });
}

// Diagnóstico: quem passa da viewport, onde está o fechar e como se comporta a
// linha de abas.
const PROBE = () => {
  const panel = document.querySelector('[role="dialog"]') as HTMLElement | null;
  if (!panel) return null;
  const vw = window.innerWidth;

  const box = (el: Element | null) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height), right: Math.round(r.right) };
  };
  const label = (el: Element) =>
    (el.getAttribute('aria-label') || el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 28);

  const buttons = Array.from(panel.querySelectorAll('button')).map((el) => ({
    label: label(el),
    box: box(el),
    overflowsRight: (box(el)?.right ?? 0) > vw + 1,
    overflowsLeft: (box(el)?.x ?? 0) < -1,
  }));

  const close = panel.querySelector('button[aria-label="Fechar painel"]');
  const closeBox = box(close);
  const tabsRow = Array.from(panel.querySelectorAll('div')).find((el) => {
    const kids = Array.from(el.children);
    return kids.length >= 4 && kids.every((k) => k.tagName === 'BUTTON') && el.className.includes('rounded-xl');
  }) as HTMLElement | undefined;

  // Elementos cujo lado direito passa da viewport (candidatos ao transbordo).
  const overflowing = Array.from(panel.querySelectorAll('*'))
    .map((el) => ({ el, r: el.getBoundingClientRect() }))
    .filter(({ r }) => r.width > 0 && r.right > vw + 1)
    .slice(0, 8)
    .map(({ el, r }) => ({
      tag: `${el.tagName.toLowerCase()}${el.className ? '.' + el.className.toString().split(' ').slice(0, 3).join('.') : ''}`,
      text: (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 24),
      right: Math.round(r.right),
    }));

  return {
    viewportWidth: vw,
    panel: box(panel),
    close: { box: closeBox, reachable: !!closeBox && closeBox.right <= vw + 1 && closeBox.x >= -1 },
    buttonsOverflowing: buttons.filter((b) => b.overflowsRight || b.overflowsLeft),
    buttonCount: buttons.length,
    tabs: tabsRow
      ? { clientWidth: tabsRow.clientWidth, scrollWidth: tabsRow.scrollWidth, scrollable: tabsRow.scrollWidth > tabsRow.clientWidth + 1, box: box(tabsRow) }
      : null,
    overflowing,
  };
};

async function openDealPanel(page: Page) {
  await seedEsteira(page);
  await page.goto('/esteira');
  await expect(page.getByText('Diego Santos').first()).toBeVisible();
  await page.getByText('Diego Santos').first().click();
  await expect(page.locator('[role="dialog"]')).toBeVisible();
  await page.waitForTimeout(400);
}

for (const viewport of [
  { name: 'celular 390', width: 390, height: 844, mobile: true },
  { name: 'celular 320', width: 320, height: 720, mobile: true },
]) {
  test.describe(`Esteira — painel lateral em ${viewport.name}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height }, isMobile: viewport.mobile, hasTouch: viewport.mobile });

    test('o fechar fica alcançável, nada passa da viewport e o painel fecha', async ({ page }) => {
      await openDealPanel(page);

      const probe = await page.evaluate(PROBE);
      expect(probe).not.toBeNull();
      expect(
        probe!.close.reachable,
        `botão de fechar fora da área visível: ${JSON.stringify(probe!.close.box)} (viewport ${viewport.width}px)`,
      ).toBe(true);
      expect(probe!.buttonsOverflowing, 'controles do cabeçalho passando da viewport').toEqual([]);
      expect(probe!.overflowing, 'elementos passando da viewport dentro do painel').toEqual([]);

      // A linha de abas pode rolar, mas nunca cortar: se o conteúdo excede a
      // largura, o contêiner precisa permitir scroll horizontal.
      if (probe!.tabs && probe!.tabs.scrollWidth > probe!.tabs.clientWidth + 1) {
        const overflowX = await page
          .locator('[role="dialog"] div.scrollbar-none')
          .first()
          .evaluate((el) => getComputedStyle(el).overflowX);
        expect(['auto', 'scroll'], 'linha de abas cortando em vez de rolar').toContain(overflowX);
      }

      // Acessível de verdade: o controle fecha o painel.
      await page.locator('button[aria-label="Fechar painel"]').click();
      await expect(page.locator('[role="dialog"]')).toBeHidden();
    });
  });
}

test.describe('Esteira — painel lateral no desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('o cabeçalho segue em uma única linha, com os controles à direita', async ({ page }) => {
    await openDealPanel(page);

    const probe = await page.evaluate(PROBE);
    expect(probe).not.toBeNull();
    expect(probe!.close.reachable).toBe(true);
    expect(probe!.buttonsOverflowing).toEqual([]);

    // Informação e controles na MESMA linha (sem a quebra que só existe no
    // celular): os centros verticais coincidem.
    const centers = await page.evaluate(() => {
      const panel = document.querySelector('[role="dialog"]')!;
      const close = panel.querySelector('button[aria-label="Fechar painel"]')!.getBoundingClientRect();
      const title = Array.from(panel.querySelectorAll('h2, a')).find((el) => /Diego Santos/.test(el.textContent ?? ''));
      const titleRect = title!.getBoundingClientRect();
      return {
        closeCenter: close.y + close.height / 2,
        titleCenter: titleRect.y + titleRect.height / 2,
      };
    });
    expect(Math.abs(centers.closeCenter - centers.titleCenter)).toBeLessThanOrEqual(12);
  });
});

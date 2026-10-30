import { test, expect, type Page } from '@playwright/test';

// These tests exercise the rendered application against synthetic HTTP fixtures.
// Cross-service persistence is verified separately by the full-stack visit suite.
const user = {
  id: 'fake-user-id', email: 'qa@example.test', firstname: 'Cliente', lastname: 'QA',
  role: 'user', functional_role: 'admin', account_type: 'individual',
  permission: 'owner', current_company_id: 'company-1',
};
const company = { id: 'company-1', type: 'company', company: {
  id: 'company-1', name: 'Imobiliária QA', type: 'imobiliaria', is_active: true,
  owner_id: user.id, members: [{ account_id: user.id, profile: 'owner' }],
} };
const supportId = 'e205b59b-a353-4f95-a880-3c02e1cd6849';
const appointment = {
  id: 'portal-visit', title: 'Visita de teste', client: 'Cliente QA', type: 'Visita',
  date: '2099-10-20', time: '14:00', end_time: '15:00', status: 'Pendente',
  priority: 'Media', agent: 'Portal', customer_account_id: user.id, lead_id: 'lead-qa',
};
// Imóvel com rótulo longo (título + ID de exibição). É o caso do defeito: no
// seletor de imóvel o texto empurrava o botão de remover para fora do painel e
// o corpo rolável ganhava scroll horizontal no mobile.
const longProperty = {
  id: 'property-long',
  display_id: '712ddc85-7980-4d80-be24-d13659605885',
  title: 'Casa Vitória Régia - Condomínio Alto da Serra',
  description: '',
  address: { street: 'Rua das Acácias', neighborhood: 'Jardim Botânico', city: 'São Paulo', state_code: 'SP' },
  owners: [],
};

// Negócio SEM display_id: era exatamente o caso em que o dropdown mostrava o
// UUID cru em vez de algo identificável para um humano.
const longDeal = {
  id: '3fb8df23-d5b7-4c5c-a0ac-d0fe3459de87',
  type: 'SALE',
  status: 'CONTRACT',
  value: 1400000,
  property_id: 'property-long',
  customer_id: 'lead-qa',
  owner_id: 'owner-qa',
  broker_id: 'fake-user-id',
  created_at: '2026-10-01T03:42:03Z',
  updated_at: '2026-10-01T03:42:03Z',
};

const overviewLead = {
  id: 'lead-qa', name: 'Cliente QA', has_preferences: true,
  recommended_property_id: 'property-qa', recommended_property: {
    id: 'property-qa', title: 'Apartamento de teste', city: 'São Paulo', sale_price: 400000,
  }, matched_properties: [],
  visit_request: { date: appointment.date, time: appointment.time, end_time: appointment.end_time,
    property_id: 'property-qa', status: 'Confirmado' },
};

async function seed(page: Page) {
  const runtimeErrors: string[] = [];
  page.on('pageerror', error => runtimeErrors.push(error.message));
  // Mesma dica de sessão usada pelos outros specs: sem ela o bootstrap do app
  // não tenta retomar a sessão sintética e a tela de login aparece.
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || path === '/auth/portal/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'synthetic-qa-token' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [company];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/appointments') data = [appointment];
    if (path === '/properties') data = [longProperty];
    if (path === '/deals') data = [longDeal];
    if (path === '/leads') data = [{ id: 'lead-qa', name: 'João Neto', role: 'buyer', is_client: true }];
    if (path === '/client-portal/overview') data = [overviewLead];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    // Unknown reads get an explicit empty fixture; unknown writes remain denied
    // by the isolated API server so an accidental mutation fails global teardown.
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|portal\/refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    await route.fulfill({ json: { success: true, statusCode: 200, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
  });
  return runtimeErrors;
}

async function expectSupportToast(page: Page, viewport: { width: number; height: number }) {
  const message = page.getByText(new RegExp(`Código de suporte: ${supportId}`));
  await expect(message).toHaveCount(1);
  await expect(message).toBeVisible();
  const toast = page.locator('[data-sonner-toast]').filter({ has: message });
  // Wait for the entry animation and verify the complete feedback fits above
  // the mobile action footer/navigation, rather than accepting a clipped toast.
  await expect.poll(async () => {
    const box = await toast.boundingBox();
    return !!box && box.y >= 0 && box.y + box.height <= viewport.height - (viewport.width < 640 ? 64 : 0);
  }).toBe(true);
}

for (const viewport of [{ name: 'desktop', width: 1280, height: 900 }, { name: 'mobile', width: 390, height: 844 }]) {
  test.describe(viewport.name, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height }, locale: 'pt-BR' });

    test('agenda: teclado, foco contido, popover e Escape em camadas', async ({ page }) => {
      const errors = await seed(page);
      await page.goto('/agenda');
      const opener = page.getByRole('button', { name: 'Novo', exact: true });
      await opener.focus();
      await page.keyboard.press('Enter');
      const dialog = page.getByRole('dialog', { name: 'Novo compromisso' });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel('Título *', { exact: true })).toBeVisible();
      const save = dialog.getByRole('button', { name: 'Salvar', exact: true });
      await save.focus();
      await page.keyboard.press('Tab');
      await expect(dialog.getByRole('button', { name: 'Expandir painel' })).toBeFocused();
      const client = dialog.getByLabel('Cliente *', { exact: true });
      await client.focus();
      await page.keyboard.press('Enter');
      await expect(page.getByPlaceholder('Buscar lead/cliente...')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.getByPlaceholder('Buscar lead/cliente...')).toBeHidden();
      await expect(dialog).toBeVisible();
      await expect(client).toBeFocused();
      await page.keyboard.press('Control+Shift+E');
      await expect(dialog.getByRole('button', { name: 'Recolher painel' })).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Expandir painel' })).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(dialog).toBeHidden();
      await expect(opener).toBeFocused();
      expect(errors).toEqual([]);
    });

    test('agenda: seletor de imóvel com rótulo longo não gera overflow horizontal', async ({ page }) => {
      const errors = await seed(page);
      await page.goto('/agenda');
      await page.getByRole('button', { name: 'Novo', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Novo compromisso' });
      await expect(dialog).toBeVisible();

      const picker = dialog.getByRole('combobox', { name: 'Imóvel (opcional)' });
      await picker.click();
      await page.getByRole('option', { name: /Casa Vitória Régia/ }).click();
      await expect(picker).toContainText('Casa Vitória Régia');

      // Nada dentro do painel pode passar da borda direita: era exatamente o que
      // acontecia (o rótulo longo empurrava o botão de remover para fora).
      const overflowers = await dialog.evaluate((root) => {
        const limit = root.getBoundingClientRect().right;
        return Array.from(root.querySelectorAll('*'))
          .filter((el) => el.getBoundingClientRect().right > limit + 1)
          .map((el) => `${el.tagName.toLowerCase()}.${String(el.className).slice(0, 60)}`);
      });
      expect(overflowers).toEqual([]);

      // O corpo rolável do painel também não pode ter scroll horizontal.
      const scroller = dialog.locator('div[class*="overflow-y-auto"]').first();
      const horizontalOverflow = await scroller.evaluate((el) => el.scrollWidth - el.clientWidth);
      expect(horizontalOverflow).toBeLessThanOrEqual(0);

      // E o rótulo é truncado (ellipsis), não vazando do campo.
      const label = picker.locator('span').first();
      const box = await label.boundingBox();
      const dialogBox = await dialog.boundingBox();
      expect(box).not.toBeNull();
      expect(dialogBox).not.toBeNull();
      expect(box!.x + box!.width).toBeLessThanOrEqual(dialogBox!.x + dialogBox!.width + 1);
      expect(errors).toEqual([]);
    });

    test('agenda: dropdown de negócio identifica o negócio em vez de mostrar o UUID', async ({ page }) => {
      const errors = await seed(page);
      await page.goto('/agenda');
      await page.getByRole('button', { name: 'Novo', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Novo compromisso' });
      const dealField = dialog.getByLabel('Negócio (opcional)', { exact: true });
      await dealField.click();

      const option = page.getByRole('option').filter({ hasText: 'Casa Vitória Régia' });
      await expect(option).toBeVisible();
      await expect(option).toContainText('Venda');
      await expect(option).toContainText(/R\$\s*1\.400\.000/);
      await expect(option).toContainText('João Neto');
      // O identificador cru do negócio não é rótulo para humano.
      await expect(option).not.toContainText('3fb8df23');

      await option.click();
      await expect(dealField).toContainText('Casa Vitória Régia');
      await expect(dealField).not.toContainText('3fb8df23');
      expect(errors).toEqual([]);
    });

    test('agenda: erro 500 preserva edição e permite tentar novamente', async ({ page }) => {
      const errors = await seed(page);
      const writes: Record<string, unknown>[] = [];
      await page.route('**/api/v1/appointments/portal-visit', async route => {
        expect(route.request().method()).toBe('PUT');
        writes.push(route.request().postDataJSON());
        await route.fulfill(writes.length === 1
          ? { status: 500, headers: { 'X-Request-ID': supportId }, json: { message: 'Falha simulada' } }
          : { json: { data: { ...appointment, ...writes[1] } } });
      });
      await page.goto('/agenda?compromisso=portal-visit');
      const dialog = page.getByRole('dialog', { name: 'Editar compromisso' });
      const title = dialog.getByLabel('Título *', { exact: true });
      await title.fill('Título preservado após falha');
      const save = dialog.getByRole('button', { name: 'Salvar', exact: true });
      await save.click();
      await expectSupportToast(page, viewport);
      await expect(save).toBeEnabled();
      await expect(title).toHaveValue('Título preservado após falha');
      const box = await dialog.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width + 1);
      await page.screenshot({ path: `/private/tmp/toca-review-agenda-${viewport.name}.png` });
      await save.click();
      await expect(dialog).toBeHidden();
      expect(writes).toHaveLength(2);
      expect(writes[0]).toEqual(writes[1]);
      expect(writes[0].title).toBe('Título preservado após falha');
      expect(errors).toEqual([]);
    });

    test('portal: cancelar visita confirmada mostra contexto, mantém visita e recupera falha', async ({ page }) => {
      const errors = await seed(page);
      let cancellations = 0;
      await page.route('**/api/v1/client-portal/leads/lead-qa/visit', async route => {
        expect(route.request().method()).toBe('DELETE');
        cancellations++;
        await route.fulfill({ status: 500, headers: { 'X-Request-ID': supportId }, json: { message: 'Falha simulada' } });
      });
      await page.goto('/portal');
      const opener = page.getByRole('button', { name: 'Cancelar visita', exact: true });
      await opener.click();
      const dialog = page.getByRole('alertdialog', { name: 'Cancelar visita confirmada?' });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('Apartamento de teste', { exact: false })).toBeVisible();
      await expect(dialog.getByText(/20\/10\/2099.*14:00/)).toBeVisible();
      await expect(dialog.getByText(/agenda da imobiliária/)).toBeVisible();
      await dialog.getByRole('button', { name: 'Manter visita' }).click();
      await expect(dialog).toBeHidden();
      await expect(opener).toBeFocused();
      expect(cancellations).toBe(0);
      await opener.click();
      await dialog.getByRole('button', { name: 'Cancelar visita', exact: true }).click();
      await expectSupportToast(page, viewport);
      await expect(dialog).toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Cancelar visita', exact: true })).toBeEnabled();
      expect(cancellations).toBe(1);
      await page.screenshot({ path: `/private/tmp/toca-review-portal-${viewport.name}.png` });
      await dialog.getByRole('button', { name: 'Manter visita' }).click();
      expect(errors).toEqual([]);
    });
  });
}

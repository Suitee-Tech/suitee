import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

async function seed(page: Page, theme: 'light' | 'dark', primaryColor: string) {
  await page.addInitScript(() => localStorage.setItem('auth_has_session', '1'));
  const writes: string[] = [];
  let unavailable = true;
  let notifications = { channels: [
    { id: 'lead', label: 'Novo lead', category: 'Leads', email: false, whatsapp: true, inApp: true },
    { id: 'visit', label: 'Visita', category: 'Agenda', email: false, whatsapp: false, inApp: true },
  ], deliveryFreq: 'realtime', quietHoursStart: '', quietHoursEnd: '' };
  const user = { id: 'fake-user-id', email: 'qa@example.test', firstname: 'Ana', lastname: 'QA', role: 'user', functional_role: 'admin', permission: 'owner', account_type: 'individual', current_company_id: 'company-1' };
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    const method = route.request().method();
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (method === 'PUT' && path === '/company-settings/notifications') {
      notifications = route.request().postDataJSON(); writes.push(path);
      await route.fulfill({ json: { success: true, data: notifications } }); return;
    }
    if (method !== 'GET' && !/^\/auth\/(refresh|switch-)/.test(path)) {
      writes.push(path); await route.fallback(); return;
    }
    if (path === '/company-settings' && unavailable) {
      await route.fulfill({ status: 503, json: { message: 'Fixture indisponível' } }); return;
    }
    let data: unknown = [];
    if (/^\/auth\/(refresh|switch-)/.test(path)) data = { access_token: 'synthetic-qa-token' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [{ id: 'company-1', type: 'company', company: { id: 'company-1', name: 'Imobiliária QA', type: 'imobiliaria', is_active: true, owner_id: user.id, members: [{ account_id: user.id, profile: 'owner' }] } }];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, timezone: 'America/Manaus', consent: { terms_version: '2.0' } };
    if (path === '/company-settings/appearance') data = { theme, primaryColor, brandName: 'Suitee QA', darkModeEnabled: true };
    if (path === '/company-settings/notifications') data = notifications;
    if (path === '/company-settings') data = { currency: 'BRL', autoSave: false, compactTables: true };
    await route.fulfill({ json: { success: true, statusCode: 200, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
  });
  return { writes, notifications: () => notifications, recover: () => { unavailable = false; } };
}

for (const variant of [
  { width: 360, theme: 'light', primaryColor: '#1C905B' },
  { width: 768, theme: 'dark', primaryColor: '#1C905B' },
  { width: 1440, theme: 'light', primaryColor: '#7C3AED' },
] as const) {
  test(`configurações: erro, recuperação e confirmação acessível ${variant.width}-${variant.theme}`, async ({ page }) => {
    await page.setViewportSize({ width: variant.width, height: 900 });
    const fixture = await seed(page, variant.theme, variant.primaryColor);
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('/configuracoes');
    await page.getByRole('button', { name: 'Geral & Sistema', exact: true }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'Não foi possível carregar as configurações' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Salvar Alterações', exact: true })).toBeDisabled();
    fixture.recover();
    await page.getByRole('button', { name: 'Tentar novamente', exact: true }).click();
    await expect(page.getByRole('combobox', { name: 'Fuso horário' })).toHaveValue('America/Manaus');
    await expect(page.getByRole('switch', { name: 'Salvamento automático' })).not.toBeChecked();
    const opener = page.getByRole('button', { name: /Restaurar Padrão Resetar/ });
    await opener.focus(); await page.keyboard.press('Enter');
    const dialog = page.getByRole('alertdialog', { name: 'Restaurar configurações padrão' });
    await expect(dialog).toBeVisible();
    await dialog.evaluate(async element => { await Promise.all(element.getAnimations({ subtree: true }).map(animation => animation.finished)); });
    const result = await new AxeBuilder({ page }).include('[role="alertdialog"]').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    expect(result.violations).toEqual([]);
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(variant.width);
    const screenshotName = `settings-confirm-${variant.width}-${variant.theme}.png`;
    // Pixel baselines depend on the OS/browser. CI always runs axe + geometry;
    // opt in to image comparison on a platform with reviewed baselines.
    if (process.env.E2E_VISUAL === '1') await expect(dialog).toHaveScreenshot(screenshotName, { animations: 'disabled' });
    else await test.info().attach(screenshotName, { body: await dialog.screenshot({ animations: 'disabled' }), contentType: 'image/png' });
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden(); await expect(opener).toBeFocused();
    expect(fixture.writes).toEqual([]); expect(errors).toEqual([]);
  });
  test(`notificações: teclado, persistência e layout ${variant.width}-${variant.theme}`, async ({ page }) => {
    await page.setViewportSize({ width: variant.width, height: 900 });
    const fixture = await seed(page, variant.theme, variant.primaryColor);
    fixture.recover();
    await page.goto('/configuracoes');
    await page.getByRole('navigation', { name: 'Seções de configurações' }).getByRole('button', { name: 'Notificações', exact: true }).click();
    const region = page.getByRole('region', { name: 'Preferências de notificação' });
    const category = region.getByRole('button', { name: 'Leads (1 gatilhos)' });
    await category.focus(); await page.keyboard.press('Enter');
    await expect(category).toHaveAttribute('aria-expanded', 'false');
    await expect(region.getByRole('button', { name: 'Novo lead: E-mail' })).toBeHidden();
    await expect(region.getByRole('button', { name: 'Visita: E-mail' })).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(region.getByRole('button', { name: 'Novo lead: E-mail' })).toBeVisible();
    await region.getByRole('checkbox', { name: 'Todas as notificações por E-mail' }).click();
    await region.getByRole('switch', { name: 'Horário silencioso' }).click();
    await region.getByLabel('Início do horário silencioso').fill('21:30');
    await region.getByLabel('Fim do horário silencioso').fill('08:00');
    await page.getByRole('button', { name: 'Salvar Alterações' }).click();
    await expect(page.getByRole('button', { name: 'Salvar Alterações' })).toBeDisabled();
    expect(fixture.notifications().channels.every(channel => channel.email)).toBe(true);
    expect(fixture.notifications().quietHoursStart).toBe('21:30');
    await page.reload();
    await page.getByRole('navigation', { name: 'Seções de configurações' }).getByRole('button', { name: 'Notificações', exact: true }).click();
    await expect(region.getByRole('checkbox', { name: 'Todas as notificações por E-mail' })).toBeChecked();
    await expect(region.getByLabel('Início do horário silencioso')).toHaveValue('21:30');
    await expect(region.getByLabel('Fim do horário silencioso')).toHaveValue('08:00');
    await expect.poll(() => region.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    await region.evaluate(async element => { await Promise.all(element.getAnimations({ subtree: true }).map(animation => animation.finished)); });
    expect((await new AxeBuilder({ page }).include('[aria-label="Preferências de notificação"]').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()).violations).toEqual([]);
    expect(fixture.writes).toEqual(['/company-settings/notifications']);
  });

}

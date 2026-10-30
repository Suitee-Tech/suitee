import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

for (const surface of ['portal', 'admin'] as const) {
for (const width of [360, 1440]) {
  test(`${surface}: ações por teclado e exclusão confirmada ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.addInitScript(() => localStorage.setItem('auth_has_session', '1'));
    let deletes = 0;
    let removed = false;
    const message = { id: 'message-qa', channel_id: 'channel-qa', sender_id: 'client-qa', sender_name: 'Cliente QA', content: 'Mensagem para revisão', sent_at: '2026-09-26T12:00:00Z', read_by: ['client-qa'] };
    const channel = { id: 'channel-qa', name: 'Atendimento', type: 'direct', status: 'open', participants: [{ id: 'client-qa', name: 'Cliente QA' }, { id: 'broker-qa', name: 'Maria Corretora' }], updated_at: message.sent_at, last_message: message };
    await page.route('**/api/v1/**', async route => {
      const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
      const method = route.request().method();
      if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
      if (method === 'POST' && path === '/channels/channel-qa/read') { await route.fulfill({ json: { success: true } }); return; }
      if (method === 'DELETE' && path === '/channels/channel-qa/messages/message-qa') {
        deletes++;
        if (deletes === 1) { await route.fulfill({ status: 503, json: { message: 'Falha simulada' } }); return; }
        removed = true; await route.fulfill({ json: { success: true } }); return;
      }
      if (method === 'POST' && /^\/auth\/(refresh|switch-)/.test(path)) { await route.fulfill({ json: { success: true, data: { access_token: 'synthetic-qa-token' } } }); return; }
      if (method !== 'GET') { await route.fallback(); return; }
      let data: unknown = [];
      if (path === '/auth/me') data = { id: 'client-qa', firstname: 'Cliente', lastname: 'QA', email: 'cliente@example.test', role: surface === 'portal' ? 'client' : 'user', functional_role: 'admin', permission: 'owner', account_type: 'individual', current_company_id: 'company-1' };
      if (path === '/settings/preferences') data = { consent: { terms_version: '2.0' } };
      if (path === '/auth/companies') data = [{ id: 'company-1', type: 'company', company: { id: 'company-1', name: 'Imobiliária QA', type: 'imobiliaria', is_active: true, owner_id: 'client-qa', members: [{ account_id: 'client-qa', profile: 'owner' }] } }];
      if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: ['module:crm', 'messages:read'] };
      if (path === '/channels/mine' || path === '/channels') data = [channel];
      if (path === '/channels/channel-qa/messages') data = surface === 'portal' ? (removed ? [] : [message]) : { messages: removed ? [] : [message], total: removed ? 0 : 1 };
      await route.fulfill({ json: { success: true, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
    });
    await page.goto(`${surface === 'portal' ? '/portal' : ''}/mensagens?channel=channel-qa`);
    const opener = page.getByRole('button', { name: 'Ações da mensagem' });
    await expect(opener).toBeVisible();
    await opener.focus(); await page.keyboard.press('Enter');
    const actions = page.getByRole('dialog', { name: 'Ações da mensagem', exact: true });
    await expect(actions).toBeVisible();
    await actions.evaluate(async element => { await Promise.all(element.getAnimations({ subtree: true }).map(animation => animation.finished)); });
    expect((await new AxeBuilder({ page }).include('[role="dialog"]').withTags(['wcag2a', 'wcag2aa']).analyze()).violations).toEqual([]);
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press('Tab');
      expect(await actions.evaluate(element => element.contains(document.activeElement))).toBe(true);
    }
    await page.keyboard.press('Escape'); await expect(opener).toBeFocused();
    await opener.press('Enter');
    await actions.getByRole('button', { name: 'Apagar', exact: true }).click();
    const confirmation = page.getByRole('alertdialog', { name: 'Apagar mensagem' });
    await expect(confirmation).toBeVisible(); expect(deletes).toBe(0);
    await confirmation.evaluate(async element => { await Promise.all(element.getAnimations({ subtree: true }).map(animation => animation.finished)); });
    expect((await new AxeBuilder({ page }).include('[role="alertdialog"]').withTags(['wcag2a', 'wcag2aa']).analyze()).violations).toEqual([]);
    await confirmation.getByRole('button', { name: 'Cancelar' }).click();
    await expect(actions.getByRole('button', { name: 'Apagar', exact: true })).toBeFocused();
    expect(deletes).toBe(0);
    await actions.getByRole('button', { name: 'Apagar', exact: true }).click();
    await confirmation.getByRole('button', { name: 'Apagar', exact: true }).click();
    await expect(confirmation.getByRole('alert')).toContainText('Não foi possível concluir');
    expect(deletes).toBe(1);
    await confirmation.getByRole('button', { name: 'Apagar', exact: true }).click();
    await expect(confirmation).toBeHidden(); await expect(actions).toBeHidden();
    expect(deletes).toBe(2);
  });
}

}

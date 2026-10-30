import { test, expect, type Page } from '@playwright/test';

async function seed(page: Page) {
  await page.addInitScript(() => localStorage.setItem('auth_has_session', '1'));
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  const writes: Record<string, unknown>[] = [];
  const supplierWrites: Record<string, unknown>[] = [];
  const contract = { id: 'contract-qa', display_id: 'CTR-042', type: 'locacao', status: 'ACTIVE',
    property_id: 'property-qa', property_title: 'Apartamento dos Ipês', client: 'Vanessa Melo', customer_id: 'client-qa',
    tenant_id: 'company-1', content: '', signatories: [], start_date: '2026-09-01', created_at: '2026-09-01T12:00:00Z', updated_at: '2026-09-01T12:00:00Z' };
  const suppliers = [
    { id: 'inspector-qa', tenant_id: 'company-1', name: 'Ana Vistoriadora', service_type: 'Vistoriador', phone: '11999990000', kind: 'externo', active: true },
    { id: 'electrician-qa', tenant_id: 'company-1', name: 'Carlos Eletricista', service_type: 'Elétrica', phone: '11999991111', kind: 'externo', active: true },
  ];
  const inspections: Record<string, unknown>[] = [];
  let failCreate = true;
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    const method = route.request().method();
    let data: unknown = [];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (method === 'POST' && path === '/inspections') {
      if (failCreate) { failCreate = false; await route.fulfill({ status: 503, json: { message: 'Falha temporária' } }); return; }
      const body = route.request().postDataJSON(); writes.push(body);
      const item = { ...body, id: 'inspection-qa', status: 'Agendada' }; inspections.push(item);
      await route.fulfill({ json: { success: true, data: item } }); return;
    }
    if (method === 'PUT' && path === '/inspections/inspection-qa') {
      const item = inspections[0]; Object.assign(item, route.request().postDataJSON());
      await route.fulfill({ json: { success: true, data: item } }); return;
    }
    if (method === 'POST' && path === '/suppliers') {
      const body = route.request().postDataJSON();
      supplierWrites.push(body);
      const item = { ...body, id: 'new-inspector', tenant_id: 'company-1', active: true }; suppliers.push(item);
      await route.fulfill({ json: { success: true, data: item } }); return;
    }
    if (method !== 'GET' && !/^\/auth\/(refresh|switch-)/.test(path)) { await route.fallback(); return; }
    const user = { id: 'fake-user-id', email: 'qa@example.test', firstname: 'Ana', lastname: 'QA', role: 'user', functional_role: 'admin', permission: 'owner', account_type: 'individual', current_company_id: 'company-1' };
    if (/^\/auth\/(refresh|switch-)/.test(path)) data = { access_token: 'synthetic-qa-token' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [{ id: 'company-1', type: 'company', company: { id: 'company-1', name: 'Imobiliária QA', type: 'imobiliaria', is_active: true, owner_id: user.id, members: [{ account_id: user.id, profile: 'owner' }] } }];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { consent: { terms_version: '2.0' } };
    if (path === '/contracts') data = [contract];
    if (path === '/contracts/contract-qa') data = contract;
    if (path === '/suppliers') data = suppliers;
    if (path === '/suppliers/service-types') data = ['Elétrica', 'Vistoriador'];
    if (path === '/inspections') data = inspections;
    if (path === '/appointments') data = [{ id: 'inspection-calendar', title: 'Vistoria de entrada', date: '2099-10-20', time: '14:30', end_time: '15:30', type: 'Vistoria', priority: 'Media', status: 'Agendado', client: '', agent_id: 'fake-user-id', inspection_id: 'inspection-qa', inspection_contract_id: 'contract-qa', inspector_id: 'inspector-qa', inspector_name: 'Ana Vistoriadora' }];
    if (path === '/properties') data = [{ id: 'property-qa', title: 'Apartamento dos Ipês', type: 'Apartamento', status: 'Alugado', address: { city: 'São Paulo' } }];
    if (path === '/leads') data = [{ id: 'client-qa', name: 'Vanessa Melo', is_client: true }];
    await route.fulfill({ json: { success: true, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
  });
  return { writes, supplierWrites, errors };
}

for (const width of [390, 1440]) {
  test(`vistoria: cadastro compartilhado, seleção e recuperação ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const fixture = await seed(page);
    await page.goto('/contratos/contract-qa?aba=locacao&secao=vistorias');
    await page.getByRole('button', { name: 'Agendar', exact: true }).click();
    await page.getByLabel('Data e horário').fill('2099-10-20T14:30');
    await page.getByRole('combobox', { name: 'Vistoriador' }).click();
    await expect(page.getByRole('option', { name: /Ana Vistoriadora/ })).toBeVisible();
    await expect(page.getByRole('option', { name: /Carlos Eletricista/ })).toHaveCount(0);
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: 'Novo', exact: true }).click();
    const panel = page.getByRole('dialog', { name: 'Novo profissional' });
    await expect(panel.getByLabel('Tipo de serviço *')).toHaveValue('Vistoria');
    await panel.getByLabel('Nome *', { exact: true }).fill('Bia Vistoriadora');
    await panel.getByLabel('Telefone').fill('11988880000');
    await expect(panel.getByLabel('Telefone')).toHaveValue('(11) 98888-0000');
    await panel.getByLabel('CPF/CNPJ').fill('11111111111');
    await panel.getByLabel('E-mail', { exact: true }).fill('bia@invalido');
    await panel.getByRole('button', { name: 'Cadastrar profissional' }).click();
    await expect(panel.getByLabel('E-mail', { exact: true })).toBeFocused();
    await expect(panel.getByLabel('CPF/CNPJ')).toHaveAttribute('aria-invalid', 'true');
    expect(fixture.supplierWrites).toHaveLength(0);
    await panel.getByLabel('E-mail', { exact: true }).fill('Bia@Example.com');
    await panel.getByLabel('CPF/CNPJ').fill('52998224725');
    await expect(panel.getByLabel('CPF/CNPJ')).toHaveValue('529.982.247-25');
    await panel.getByRole('button', { name: 'Cadastrar profissional' }).click();
    await expect(panel).toBeHidden();
    expect(fixture.supplierWrites).toEqual([expect.objectContaining({ phone: '11988880000', email: 'bia@example.com', document: '52998224725' })]);
    await expect(page.getByRole('combobox', { name: 'Vistoriador' })).toContainText('Bia Vistoriadora');
    await page.getByRole('button', { name: 'Agendar vistoria', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Seus dados foram mantidos');
    await expect(page.getByLabel('Data e horário')).toHaveValue('2099-10-20T14:30');
    await page.getByRole('button', { name: 'Agendar vistoria', exact: true }).click();
    await expect(page.getByLabel('Data e horário')).toBeHidden();
    await page.getByRole('button', { name: 'Reagendar vistoria' }).click();
    await expect(page.getByLabel('Data e horário')).toHaveValue('2099-10-20T14:30');
    await page.getByLabel('Data e horário').fill('2099-10-21T15:00');
    await page.getByRole('button', { name: 'Salvar agendamento' }).click();
    await expect(page.getByLabel('Data e horário')).toBeHidden();
    await page.getByRole('button', { name: 'Cancelar vistoria', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancelar vistoria', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Reagendar vistoria' })).toHaveCount(0);
    expect(fixture.writes).toHaveLength(1);
    expect(fixture.writes[0]).toMatchObject({ contract_id: 'contract-qa', property_id: 'property-qa', inspector_id: 'new-inspector', inspector_name: 'Bia Vistoriadora' });
    expect(fixture.errors).toEqual([]);
  });

  test(`vistoria: seletor com busca permanece selecionável com o teclado aberto ${width}`, async ({ page }) => {
    test.skip(width !== 390, 'teclado virtual é um caso do celular');
    await page.setViewportSize({ width, height: 900 });
    const fixture = await seed(page);

    // Lista longa: com o teclado aberto a última opção só fica alcançável se a
    // lista couber na faixa visível e rolar dentro dela.
    const manySuppliers = Array.from({ length: 12 }, (_, index) => ({
      id: `inspector-${index + 1}`,
      tenant_id: 'company-1',
      name: `Profissional QA ${String(index + 1).padStart(2, '0')}`,
      service_type: 'Vistoriador',
      phone: '11999990000',
      kind: 'externo',
      active: true,
    }));
    await page.route('**/api/v1/suppliers**', (route) =>
      route.fulfill({ json: { success: true, data: manySuppliers, meta: { total: manySuppliers.length } } }));

    // Teclado virtual simulado: encolhe apenas a viewport VISUAL (como no iOS).
    await page.addInitScript(() => {
      const listeners = new Set<() => void>();
      const state = { height: window.innerHeight };
      const fake = {
        get height() { return state.height; },
        width: window.innerWidth,
        offsetTop: 0,
        offsetLeft: 0,
        scale: 1,
        addEventListener: (event: string, callback: () => void) => {
          if (event === 'resize') listeners.add(callback);
        },
        removeEventListener: (_event: string, callback: () => void) => { listeners.delete(callback); },
      };
      Object.defineProperty(window, 'visualViewport', { configurable: true, get: () => fake });
      (window as unknown as { __openKeyboard: (px: number) => void }).__openKeyboard = (px: number) => {
        state.height = window.innerHeight - px;
        listeners.forEach((callback) => callback());
      };
    });

    await page.goto('/contratos/contract-qa?aba=locacao&secao=vistorias');
    await page.getByRole('button', { name: 'Agendar', exact: true }).click();

    const keyboardInset = 360;
    await page.getByRole('combobox', { name: 'Vistoriador' }).click();
    await page.evaluate((px) => (window as unknown as { __openKeyboard: (px: number) => void }).__openKeyboard(px), keyboardInset);

    const picker = page.getByRole('dialog', { name: 'Selecione um profissional cadastrado…' });
    await expect(picker).toBeVisible();
    const visibleBottom = 900 - keyboardInset;

    // O seletor inteiro precisa ficar acima da faixa coberta pelo teclado.
    await expect.poll(async () => {
      const box = await picker.boundingBox();
      if (!box) return false;
      return box.y >= 0 && box.y + box.height <= visibleBottom + 1;
    }).toBe(true);

    // E a última opção continua alcançável (rolando dentro do painel).
    const lastOption = picker.getByRole('option').last();
    await expect(lastOption).toHaveText(/Profissional QA 12/);
    await lastOption.scrollIntoViewIfNeeded();
    const optionBox = await lastOption.boundingBox();
    expect(optionBox).not.toBeNull();
    expect(optionBox!.y + optionBox!.height).toBeLessThanOrEqual(visibleBottom + 1);

    await lastOption.click();
    await expect(page.getByRole('combobox', { name: 'Vistoriador' })).toContainText('Profissional QA 12');
    expect(fixture.errors).toEqual([]);
  });

  test(`manutenção: vínculo preservado e contrato legível ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const fixture = await seed(page);
    await page.goto('/manutencao?contrato=contract-qa');
    await expect(page.getByRole('button', { name: /CTR-042 · Vanessa Melo · Apartamento dos Ipês/ })).toBeVisible();
    await page.getByRole('button', { name: /CTR-042 · Vanessa Melo · Apartamento dos Ipês/ }).click();
    await page.getByPlaceholder('Buscar por cliente, imóvel ou código...').fill('Vanessa');
    await expect(page.getByRole('option', { name: /CTR-042 · Vanessa Melo · Apartamento dos Ipês/ })).toBeVisible();
    expect(fixture.errors).toEqual([]);
  });
}

for (const width of [390, 1440]) {
  test(`agenda: vistoriador e acesso ao contrato de origem ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const fixture = await seed(page);
    await page.goto('/agenda?apt=inspection-calendar');
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText('Vistoriador: Ana Vistoriadora')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Editar', exact: true })).toHaveCount(0);
    await dialog.getByRole('link', { name: 'Abrir contrato da vistoria' }).click();
    await expect(page.getByRole('heading', { name: 'Vistorias agendadas e concluídas' })).toBeVisible();
    expect(fixture.errors).toEqual([]);
  });
}

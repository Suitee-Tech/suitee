import { test, expect, Page, Route } from '@playwright/test';

// Configurações > Aparência: a escolha local precisa sobreviver à recarga.
//
// Relato que originou o teste: "a configuração de tema não está persistindo".
// Reproduzido assim — a aba aplica a mudança na hora (o app fica escuro e grava
// no localStorage), mas o envio ao servidor só acontece no "Salvar Alterações".
// O ThemeProvider relê a aparência do servidor a cada carga, então recarregar
// antes de salvar devolvia o valor do tenant e o tema revertia.
//
// Contrato: o rascunho local vence a leitura do servidor até ser salvo; salvar
// limpa o rascunho e aí o valor do tenant volta a mandar (é o que faz uma
// mudança do admin chegar aos usuários).
//
// O mock da aparência é COM ESTADO (GET devolve o que o PUT gravou) — com um
// mock estático este teste passaria sem provar persistência nenhuma.

const TEST_USER = {
  id: 'fake-user-id',
  email: 'maria@example.com',
  firstname: 'Maria',
  lastname: 'Teste',
  role: 'user',
  functional_role: 'admin',
  account_type: 'individual',
  permission: 'owner',
  current_company_id: 'company-1',
};

const TEST_COMPANY = {
  id: 'company-1',
  type: 'company',
  company: {
    id: 'company-1',
    name: 'Imobiliária Exemplo',
    legal_name: 'Imobiliária Exemplo LTDA',
    tax_id: '11222333000181',
    email: 'contato@mci.com.br',
    phone: '11999998888',
    type: 'imobiliaria',
    is_active: true,
    owner_id: TEST_USER.id,
    members: [{ account_id: TEST_USER.id, profile: 'owner' }],
  },
};

const APPEARANCE_INICIAL = {
  tenant_id: 'company-1',
  primaryColor: '#013529',
  accentColor: '#B3833D',
  sidebarStyle: 'default',
  theme: 'auto',
  fontFamily: 'Nunito',
  showLogoLogin: true,
  darkModeEnabled: true,
  customDomain: '',
  domainVerified: false,
  loginTagline: 'Sua plataforma imobiliaria',
  loginSubtitle: 'Gestao completa',
  loginBannerPreset: 'equipe',
  loginBannerUrl: '',
  loginBannerSource: 'preset',
  emailFooter: '',
};

interface Backend {
  appearance: typeof APPEARANCE_INICIAL;
  puts: Array<Record<string, unknown>>;
}

/** Backend mínimo com estado para a aparência (GET lê o que o PUT gravou). */
async function mockBackend(page: Page): Promise<Backend> {
  await page.addInitScript(() => {
    localStorage.setItem('auth_has_session', '1');
    localStorage.setItem('current_company_id', 'company-1');
  });
  const backend: Backend = { appearance: { ...APPEARANCE_INICIAL }, puts: [] };

  await page.route(/\/api\/v1\/auth\/refresh\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake' } }) }),
  );
  await page.route(/\/api\/v1\/auth\/me\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: TEST_USER }) }),
  );
  await page.route(/\/api\/v1\/auth\/companies\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: [TEST_COMPANY] }) }),
  );
  await page.route(/\/api\/v1\/auth\/switch-(context|company)\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-tenant' } }) }),
  );
  await page.route(/\/api\/v1\/settings\/preferences\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { last_company_id: null, consent: { terms_version: '2.0' } } }) }),
  );
  await page.route(/\/api\/v1\/me\/permissions\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: { functional_role: 'admin', profile: 'owner', permissions: ['settings:manage_appearance'] },
      }),
    }),
  );
  // Registrada por ÚLTIMO: as rotas do Playwright são avaliadas em LIFO, então
  // esta vence o catch-all abaixo.
  await page.route(/\/api\/v1\/.*/, async (route: Route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.includes('/company-settings/appearance')) {
      if (route.request().method() === 'PUT') {
        const payload = JSON.parse(route.request().postData() || '{}');
        backend.puts.push(payload);
        backend.appearance = { ...backend.appearance, ...payload };
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ message: 'appearance updated' }) });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 200, data: backend.appearance }),
      });
      return;
    }
    if (/\/auth\/(refresh|me|companies|switch-)/.test(path) || /settings\/preferences/.test(path) || /me\/permissions/.test(path)) {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [], total: 0, meta: { total: 0, limit: 100 } }),
    });
  });

  return backend;
}

async function abrirAparencia(page: Page) {
  await page.goto('/configuracoes');
  await page.locator('button:has-text("Aparência"), [role="tab"]:has-text("Aparência")').first().click();
  await expect(page.getByText('Paleta de Cores')).toBeVisible();
}

const modoEscuro = (page: Page) => page.evaluate(() => document.documentElement.classList.contains('dark'));

test.describe('Configurações > Aparência — a escolha local sobrevive à recarga', () => {
  test('trocar o tema e recarregar SEM salvar mantém o tema escolhido', async ({ page }) => {
    const backend = await mockBackend(page);
    await abrirAparencia(page);

    expect(await modoEscuro(page)).toBe(false);
    await page.locator('button:has(.lucide-moon)').first().click();
    await expect.poll(() => modoEscuro(page)).toBe(true);

    // O servidor continua com o valor antigo — o rascunho é que sustenta a escolha.
    expect(backend.puts).toHaveLength(0);
    expect(backend.appearance.theme).toBe('auto');

    await page.reload();
    await expect.poll(() => modoEscuro(page)).toBe(true);

    await abrirAparencia(page);
    await expect(page.getByTestId('appearance-pending')).toBeVisible();
  });

  test('salvar grava no servidor, limpa o aviso e mantém o tema', async ({ page }) => {
    const backend = await mockBackend(page);
    await abrirAparencia(page);

    await page.locator('button:has(.lucide-moon)').first().click();
    await expect.poll(() => modoEscuro(page)).toBe(true);

    await page.locator('button:has-text("Salvar")').first().click();
    await expect.poll(() => backend.puts.length).toBeGreaterThan(0);
    expect(backend.appearance.theme).toBe('dark');
    // O envio leva o estado inteiro da aba, não só o campo alterado.
    expect(backend.puts[0].primaryColor).toBe('#013529');

    await expect.poll(() => page.evaluate(() => window.localStorage.getItem('suitee-appearance-pending'))).toBeNull();

    await page.reload();
    await expect.poll(() => modoEscuro(page)).toBe(true);

    await abrirAparencia(page);
    await expect(page.getByTestId('appearance-pending')).toBeHidden();
  });

  test('alteração do servidor volta a valer quando não há rascunho', async ({ page }) => {
    const backend = await mockBackend(page);
    await abrirAparencia(page);

    // Admin troca a cor do tenant enquanto este navegador não tem rascunho.
    backend.appearance = { ...backend.appearance, primaryColor: '#7C3AED' };
    await page.reload();
    await abrirAparencia(page);

    await expect(page.locator('input[value="#7C3AED"]').first()).toBeVisible();
  });
});

import { test, expect, Page, type Route } from '@playwright/test';

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

// Bootstrap auth + permissions + CRM sem leads.
async function mockAuthBoot(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('auth_has_session', '1');
    localStorage.setItem('current_company_id', 'company-1');
  });
  await page.route(/\/api\/v1\/auth\/refresh\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-token' } }),
    }),
  );
  await page.route(/\/api\/v1\/auth\/me\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: TEST_USER }),
    }),
  );
  await page.route(/\/api\/v1\/auth\/companies\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [TEST_COMPANY] }),
    }),
  );
  await page.route(/\/api\/v1\/auth\/switch-(context|company)\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-tenant-scoped' } }),
    }),
  );
  await page.route(/\/api\/v1\/settings\/preferences\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: { last_company_id: null, consent: { terms_version: '2.0' } },
      }),
    }),
  );
  // Permissions endpoint usado pelo PermissionsContext (mas o `can` faz bypass pra owner).
  await page.route(/\/api\/v1\/me\/permissions\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { functional_role: 'admin', profile: 'owner', permissions: [] } }),
    }),
  );
}

async function mockLeads(page: Page, leads: any[], respond?: (route: Route) => Promise<void>) {
  await page.route(/\/api\/v1\/leads(\?|$).*/, (r) => {
    if (respond) return respond(r);
    return r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: leads,
        total: leads.length,
        page: 1,
        limit: 1000,
      }),
    });
  });
  // Roleta config (chamado quando user tem leads:configure_roulette).
  await page.route(/\/api\/v1\/leads\/roulette\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { enabled: false, active_brokers: [] } }),
    }),
  );
}

// Catch-all para qualquer outro endpoint /api/v1/* não mockado: retorna lista vazia.
async function mockCatchAll(page: Page) {
  await page.route(/\/api\/v1\/.*/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (
      /\/auth\/(refresh|me|companies|switch-)/.test(path) ||
      /\/settings\/preferences/.test(path) ||
      /\/me\/permissions/.test(path) ||
      /\/leads(\b|$|\/)/.test(path)
    ) {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [] }),
    });
  });
}

function buildLead(overrides: Partial<any> = {}) {
  return {
    id: `lead-${Math.random().toString(36).slice(2, 9)}`,
    name: 'Lead Padrão',
    email: 'lead@example.com',
    phone: '11999998888',
    source: 'Site',
    status: 'Novo',
    is_client: false,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    tenant_id: 'company-1',
    ...overrides,
  };
}

async function gotoLeads(page: Page, opts?: { leads?: any[]; leadsRespond?: (route: Route) => Promise<void> }) {
  await mockAuthBoot(page);
  await mockLeads(page, opts?.leads ?? [], opts?.leadsRespond);
  await mockCatchAll(page);
  await page.goto('/leads');
  await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();
}

test.describe('Leads (CRM)', () => {
  test.describe('Empty state', () => {
    test.beforeEach(async ({ page }) => {
      await gotoLeads(page);
    });

    // @header
    test('exibe título e subtítulo da página', async ({ page }) => {
      await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();
      await expect(page.getByText('Acompanhe e mova seus contatos pelo funil de vendas.')).toBeVisible();
    });

    // @header @acoes
    test('exibe botões Novo Lead e Iniciar Captação no header da página', async ({ page }) => {
      // Escopa pelo <main> para não capturar o botão "Novo Lead" global do layout (banner).
      const main = page.getByRole('main');
      await expect(main.getByRole('button', { name: 'Novo Lead', exact: true })).toBeVisible();
      await expect(main.getByRole('button', { name: 'Iniciar Captação' })).toBeVisible();
    });

    // @header @toggle
    test('toggle Lista / Quadro presente, com "Quadro" pré-selecionado por padrão', async ({ page }) => {
      const lista = page.getByRole('button', { name: 'Lista' });
      const quadro = page.getByRole('button', { name: 'Quadro' });
      await expect(lista).toBeVisible();
      await expect(quadro).toBeVisible();
      // Default é "board" (vem do ViewPreferencesContext). Ativo recebe `text-primary-700`.
      await expect(quadro).toHaveClass(/text-primary-700/);
      await expect(lista).not.toHaveClass(/text-primary-700/);
    });

    // @filtros @header
    test('filtros padrão visíveis', async ({ page }) => {
      await expect(page.getByPlaceholder('Buscar por nome, email, telefone ou imóvel...')).toBeVisible();
      await expect(page.locator('select').filter({ hasText: 'Todos os Status' })).toBeVisible();
      await expect(page.locator('select').filter({ hasText: 'Tipo de Lead' })).toBeVisible();
      await expect(page.locator('select').filter({ hasText: 'Leads e Clientes' })).toBeVisible();
    });

    // @filtros @status_options
    test('select de status lista todas as 9 opções', async ({ page }) => {
      const select = page.locator('select').filter({ hasText: 'Todos os Status' });
      const options = await select.locator('option').allTextContents();
      expect(options).toEqual([
        'Todos os Status',
        'Novo',
        'Em Atendimento',
        'Qualificado',
        'Visita Agendada',
        'Em Negociação',
        'Proposta Enviada',
        'Convertido',
        'Perdido',
      ]);
    });

    // @filtros @tipo_options
    test('select de tipo de lead lista as 4 opções', async ({ page }) => {
      const select = page.locator('select').filter({ hasText: 'Tipo de Lead' });
      const options = await select.locator('option').allTextContents();
      expect(options).toEqual(['Tipo de Lead', 'Captação', 'Compra e Venda', 'Locação']);
    });

    // @filtros @cliente_options
    test('select de leads/clientes lista 3 opções', async ({ page }) => {
      const select = page.locator('select').filter({ hasText: 'Leads e Clientes' });
      const options = await select.locator('option').allTextContents();
      expect(options).toEqual(['Leads e Clientes', 'Leads', 'Clientes']);
    });

    // @empty @sem_leads
    test('base vazia exibe estado vazio com CTA Novo Lead', async ({ page }) => {
      await expect(page.getByRole('heading', { name: 'Sua base de leads está vazia' })).toBeVisible();
      await expect(
        page.getByText('Use o botão Novo Lead no topo da página para adicionar seu primeiro lead e acompanhá-lo no funil de vendas.')
      ).toBeVisible();
      await expect(page.getByRole('button', { name: 'Novo Lead', exact: true })).toBeVisible();
    });

    // @novo_lead @empty_state
    test('clicar "Novo Lead" com a base vazia abre o painel', async ({ page }) => {
      await page.getByRole('main').getByRole('button', { name: 'Novo Lead', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Novo Lead' })).toBeVisible();
      await expect(page.getByText('Selecione o tipo de lead ou preencha os dados')).toBeVisible();
    });

    // @novo_lead @abrir
    test('clicar "Novo Lead" no header da página abre o painel', async ({ page }) => {
      await page.getByRole('main').getByRole('button', { name: 'Novo Lead', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Novo Lead' })).toBeVisible();
    });
  });

  test.describe('Com leads cadastrados', () => {
    test.beforeEach(async ({ page }) => {
      await gotoLeads(page, {
        leads: [
          buildLead({ id: 'lead-1', name: 'Moacir Matos', email: 'moacir@example.com', status: 'Novo' }),
          buildLead({ id: 'lead-2', name: 'Matha Golpista', email: 'matha@example.com', status: 'Em Atendimento' }),
          buildLead({ id: 'lead-3', name: 'Bernardo', email: 'bernardo@example.com', status: 'Qualificado' }),
        ],
      });
    });

    // @lista @cards
    test('modo Lista renderiza os leads cadastrados', async ({ page }) => {
      await expect(page.getByText('Moacir Matos')).toBeVisible();
      await expect(page.getByText('Matha Golpista')).toBeVisible();
      await expect(page.getByText('Bernardo')).toBeVisible();
    });

    // @toggle @lista_para_quadro
    test('alternar para "Quadro" muda o estado ativo do toggle', async ({ page }) => {
      const lista = page.getByRole('button', { name: 'Lista' });
      const quadro = page.getByRole('button', { name: 'Quadro' });
      await quadro.click();
      await expect(quadro).toHaveClass(/text-primary-700/);
      await expect(lista).not.toHaveClass(/text-primary-700/);
    });

    // @toggle @quadro_para_lista
    test('alternar de "Quadro" para "Lista" volta ao estado original', async ({ page }) => {
      const lista = page.getByRole('button', { name: 'Lista' });
      const quadro = page.getByRole('button', { name: 'Quadro' });
      await quadro.click();
      await expect(quadro).toHaveClass(/text-primary-700/);
      await lista.click();
      await expect(lista).toHaveClass(/text-primary-700/);
    });

    // @filtros @sem_resultado
    test('busca sem match exibe "Nenhum lead encontrado" + Limpar Filtros', async ({ page }) => {
      const search = page.getByPlaceholder('Buscar por nome, email, telefone ou imóvel...');
      await search.fill('texto_que_nao_existe_em_nenhum_lead_xyz_123');
      await expect(page.getByRole('heading', { name: 'Nenhum lead encontrado' })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Limpar Filtros' })).toBeVisible();
    });

    // @filtros @limpar
    test('Limpar Filtros reseta busca e selects', async ({ page }) => {
      const search = page.getByPlaceholder('Buscar por nome, email, telefone ou imóvel...');
      await search.fill('texto_inexistente_xyz');
      await page.getByRole('button', { name: 'Limpar Filtros' }).click();
      await expect(search).toHaveValue('');
      // Os leads voltam a aparecer
      await expect(page.getByText('Moacir Matos')).toBeVisible();
    });
  });
});

test.describe('Leads — estados de leitura', () => {
  test('não apresenta a base como vazia enquanto a leitura está em andamento', async ({ page }) => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await gotoLeads(page, {
      leadsRespond: async (route) => {
        await gate;
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, statusCode: 200, data: [], total: 0, page: 1, limit: 1000 }),
        });
      },
    });

    await expect(page.getByRole('status', { name: 'Carregando leads' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Sua base de leads está vazia' })).toHaveCount(0);

    release();
    await expect(page.getByRole('heading', { name: 'Sua base de leads está vazia' })).toBeVisible();
  });

  test('500 mostra erro com retry e não finge base vazia', async ({ page }) => {
    let fail = true;
    await gotoLeads(page, {
      leadsRespond: async (route) => {
        if (fail) {
          await route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: JSON.stringify({ success: false, message: 'Falha simulada' }),
          });
          return;
        }
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, statusCode: 200, data: [], total: 0, page: 1, limit: 1000 }),
        });
      },
    });

    await expect(page.getByRole('alert').filter({ hasText: 'A leitura dos leads falhou' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Sua base de leads está vazia' })).toHaveCount(0);

    fail = false;
    await page.getByRole('button', { name: 'Tentar novamente' }).click();
    await expect(page.getByRole('heading', { name: 'Sua base de leads está vazia' })).toBeVisible();
  });
});

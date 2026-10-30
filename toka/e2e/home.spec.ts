import { test, expect, Page } from '@playwright/test';

const TEST_USER = {
  id: 'fake-user-id',
  email: 'maria@example.com',
  firstname: 'Maria',
  lastname: 'Teste',
  role: 'user',
  functional_role: 'admin',
  account_type: 'individual',
  permission: 'owner',
  current_company_id: 'mci-company-id',
};

const TEST_COMPANY = {
  id: 'mci-company-id',
  type: 'company',
  company: {
    id: 'mci-company-id',
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

// Mocka o boot do AuthContext + sincronização de company.
async function mockAuthBoot(page: Page, options?: { companies?: any[] }) {
  const companies = options?.companies ?? [TEST_COMPANY];
  await page.route(/\/api\/v1\/auth\/refresh\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-token' } }),
    });
  });
  await page.route(/\/api\/v1\/auth\/me\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: { ...TEST_USER, current_company_id: companies.length > 0 ? companies[0].id : undefined },
      }),
    });
  });
  await page.route(/\/api\/v1\/auth\/companies\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: companies }),
    });
  });
  await page.route(/\/api\/v1\/auth\/switch-(context|company)\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-tenant-scoped' } }),
    });
  });
  // O catch-all de `mockEmptyCrm` responde `data: []` para tudo que não estiver
  // listado, mas `/me/permissions` NÃO é uma lista: o contrato é o objeto
  // `{ functional_role, profile, permissions }` (settings.service.getMyPermissions).
  // Mockar aqui mantém a forma correta — como já fazem leads/new-lead/
  // calendar-portal-ux — em vez de depender de um catch-all com o tipo errado.
  // (Isto NÃO era a causa da falha da agenda: aquela era a ausência de `meta`
  // em `/appointments`, logo abaixo.)
  await page.route(/\/api\/v1\/me\/permissions\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: { functional_role: 'admin', profile: 'owner', permissions: [] },
      }),
    });
  });
  await page.route(/\/api\/v1\/settings\/preferences\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: { last_company_id: null, consent: { terms_version: '2.0' } },
      }),
    });
  });
}

// Catch-all para qualquer outro endpoint /api/v1/* que a Dashboard ou CRMContext consome.
// Devolve sempre 200 com data vazia ([] para listas, 0 para stats).
async function mockEmptyCrm(page: Page) {
  await page.route(/\/api\/v1\/leads\/stats\b.*/, async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { total: 0, conversion_rate: 0 } }),
    });
  });
  await page.route(/\/api\/v1\/deals\/stats\b.*/, async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { monthly_sales: 0 } }),
    });
  });
  // Fallback genérico — qualquer outro endpoint /api/v1/* não mockado retorna lista/objeto vazio.
  await page.route(/\/api\/v1\/.*/, async (route) => {
    const url = new URL(route.request().url());
    // Já tratados acima; deixa o handler mais específico assumir
    const path = url.pathname;
    if (
      /\/auth\/(refresh|me|companies|switch-)/.test(path) ||
      /\/settings\/preferences/.test(path) ||
      /\/leads\/stats/.test(path) ||
      /\/deals\/stats/.test(path)
    ) {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [] }),
    });
  });
}

async function gotoHome(page: Page) {
  await mockAuthBoot(page);
  await mockEmptyCrm(page);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: `Olá, ${TEST_USER.firstname}` })).toBeVisible();
}

test.describe('Home (Dashboard)', () => {
  test.beforeEach(async ({ page }) => {
    await gotoHome(page);
  });

  // @header
  test('cabeçalho exibe saudação, empresa e botão Atender Novo Lead', async ({ page }) => {
    await expect(page.getByRole('heading', { name: `Olá, ${TEST_USER.firstname}` })).toBeVisible();
    await expect(page.getByText('Imobiliária Exemplo')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Atender Novo Lead' })).toBeVisible();
  });

  // @kpis
  test('exibe os 4 KPIs do resumo do dia', async ({ page }) => {
    await expect(page.getByText('Leads Ativos', { exact: true })).toBeVisible();
    await expect(page.getByText('Visitas Hoje', { exact: true })).toBeVisible();
    await expect(page.getByText('Propostas Pendentes', { exact: true })).toBeVisible();
    await expect(page.getByText('Comissões (Est.)', { exact: true })).toBeVisible();
  });

  // @agenda @empty
  test('Agenda do Dia vazia exibe estado vazio com CTA Agendar Compromisso', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /Agenda do Dia/ })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Sua agenda está livre' })).toBeVisible();
    await expect(
      page.getByText('Você não tem compromissos para hoje. Aproveite para organizar suas próximas atividades.')
    ).toBeVisible();
    await expect(page.getByRole('button', { name: 'Agendar Compromisso' })).toBeVisible();
  });

  // @leads @empty
  test('Leads Recentes vazio exibe estado vazio com CTA Adicionar Novo Lead', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /Leads Recentes/ })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Sem leads recentes' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Adicionar Novo Lead' })).toBeVisible();
  });

  // @metas
  test('card Metas do Mês mostra VGV Vendido e Taxa de Conversão zerados', async ({ page }) => {
    // Localiza o container do card Metas do Mês para evitar ambiguidade com KPIs
    const metasCard = page.locator('div', { has: page.getByRole('heading', { name: /Metas do Mês/ }) }).first();
    await expect(metasCard.getByRole('heading', { name: /Metas do Mês/ })).toBeVisible();
    await expect(metasCard.getByText('VGV Vendido')).toBeVisible();
    await expect(metasCard.getByText('Taxa de Conversão')).toBeVisible();
    // Valores zerados dentro do card
    await expect(metasCard.locator('span', { hasText: 'R$ 0' })).toBeVisible();
    await expect(metasCard.locator('span', { hasText: '0%' })).toBeVisible();
  });

  // @negociacoes @empty
  test('Negociações Ativas vazio exibe estado vazio com CTA Acessar Esteira Digital', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /Negociações Ativas/ })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Nenhuma negociação ativa' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Acessar Esteira Digital' })).toBeVisible();
  });

  // @navegacao @atender_lead
  test('Atender Novo Lead navega para /leads', async ({ page }) => {
    await page.getByRole('button', { name: 'Atender Novo Lead' }).click();
    await page.waitForURL(/\/leads/);
  });

  // @navegacao @agenda (empty state CTA)
  test('Agendar Compromisso (empty state) navega para /agenda', async ({ page }) => {
    await page.getByRole('button', { name: 'Agendar Compromisso' }).click();
    await page.waitForURL(/\/agenda/);
  });

  // @navegacao @ver_todas
  test('Ver todas (agenda) navega para /agenda', async ({ page }) => {
    await page.getByRole('button', { name: 'Ver todas' }).click();
    await page.waitForURL(/\/agenda/);
  });

  // @navegacao @adicionar_tarefa
  test('+ Adicionar Tarefa navega para /agenda', async ({ page }) => {
    await page.getByRole('button', { name: '+ Adicionar Tarefa' }).click();
    await page.waitForURL(/\/agenda/);
  });

  // @navegacao @ver_leads
  test('Ver todos os leads ativos navega para /leads', async ({ page }) => {
    await page.getByRole('button', { name: 'Ver todos os leads ativos' }).click();
    await page.waitForURL(/\/leads/);
  });

  // @navegacao @adicionar_lead
  test('Adicionar Novo Lead (empty state) navega para /leads', async ({ page }) => {
    await page.getByRole('button', { name: 'Adicionar Novo Lead' }).click();
    await page.waitForURL(/\/leads/);
  });

  // @navegacao @esteira
  test('Ir para Esteira Digital navega para /esteira', async ({ page }) => {
    await page.getByRole('button', { name: 'Ir para Esteira Digital' }).click();
    await page.waitForURL(/\/esteira/);
  });
});

// Tests for direct-link navigation from home items into detail pages (Phase: navigation).
test.describe('Home - Navegação direta para item', () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthBoot(page);
    await mockEmptyCrm(page);
  });

  // Helper que sobrescreve /appointments antes da navegação para /
  async function mockTodayAppointment(page: Page, apt: { id: string; title: string }) {
    // Dashboard compara com a data local. `toISOString()` vira o dia seguinte
    // depois das 21h no fuso de São Paulo e tornava este cenário dependente do horário.
    const now = new Date();
    const today = [
      now.getFullYear(),
      String(now.getMonth() + 1).padStart(2, '0'),
      String(now.getDate()).padStart(2, '0'),
    ].join('-');
    await page.route(/\/api\/v1\/appointments\b.*/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          statusCode: 200,
          data: [
            {
              id: apt.id,
              title: apt.title,
              date: today,
              time: '09:00',
              end_time: '10:00',
              type: 'Visita',
              client: 'Cliente Teste',
              status: 'Pendente',
            },
          ],
          // `collectPages` (shared/services/collectPages.ts) recusa uma listagem
          // que não informe `meta.total` nem `meta.limit` — sem isso a leitura
          // lança, o CalendarContext cai no catch e a agenda fica vazia, mesmo
          // com o 200 correto. A resposta sem `meta` era o fixture antigo,
          // anterior a esse endurecimento.
          meta: { total: 1, limit: 100 },
        }),
      });
    });
  }

  test('clicar em um item da agenda do dia navega para /agenda?apt=:id', async ({ page }) => {
    const APT_ID = 'apt-home-test-1';
    await mockTodayAppointment(page, { id: APT_ID, title: 'Visita Teste' });
    await page.goto('/');
    // Aguarda carregar
    await expect(page.getByRole('heading', { name: /Olá, /i })).toBeVisible();
    // Procura o card da tarefa (com role="button") e clica
    const taskCard = page.getByText('Visita Teste').first();
    await expect(taskCard).toBeVisible();
    await taskCard.click();
    // Calendar.tsx remove o query param após abrir o detalhe — então só asseguramos que mudamos para /agenda
    await page.waitForURL((url) => url.pathname === '/agenda');
    expect(page.url()).toContain('/agenda');
  });
});

test.describe('Home - Redirect sem empresa', () => {
  // @redirect @sem_empresa
  test('usuário sem empresa é redirecionado para /onboarding', async ({ page }) => {
    await mockAuthBoot(page, { companies: [] });
    await mockEmptyCrm(page);
    await page.goto('/');
    await page.waitForURL(/\/onboarding/);
    expect(page.url()).toContain('/onboarding');
  });
});

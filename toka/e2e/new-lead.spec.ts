import { test, expect, Page, Route, Request } from '@playwright/test';

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

const ALICE_BROKER = {
  id: 'broker-alice',
  name: 'Alice Corretora',
  role: 'corretor',
  email: 'alice@example.com',
  avatar: '',
  active: true,
  paused: false,
};

// Bootstrap auth + permissões.
async function mockAuthBoot(page: Page) {
  // A borda de sessão do app só tenta restaurar a sessão (`POST /auth/refresh`)
  // quando `hasSessionHint()` é verdadeiro, ou seja, quando já houve login nesta
  // origem. Sem este marcador o boot mostra o formulário de login e nenhum dos
  // mocks abaixo é usado — era o que deixava a suíte inteira deste arquivo
  // falhando em `gotoLeads`. É o mesmo `addInitScript` das outras suítes
  // determinísticas (settings-consistency, partnership-agreements, …).
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
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
    // consent.terms_version must match CURRENT_TERMS_VERSION (shared/config/lgpd.ts) so the
    // global TermsAcceptanceModal does not block the page during the lead-panel tests.
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { last_company_id: null, consent: { terms_version: '2.0' } } }) }),
  );
  await page.route(/\/api\/v1\/me\/permissions\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { functional_role: 'admin', profile: 'owner', permissions: [] } }) }),
  );
}

// Mocks de leads + brokers + roleta.
async function mockCrm(page: Page) {
  // /leads list (GET) inicialmente vazio
  await page.route(/\/api\/v1\/leads(\?|$).*/, async (route: Route) => {
    if (route.request().method() !== 'GET') {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [], total: 0, page: 1, limit: 1000 }),
    });
  });
  await page.route(/\/api\/v1\/leads\/roulette\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { enabled: true, active_brokers: [] } }) }),
  );
  // /brokers responde com `{ data: BrokerDTO[] }` no body (sem wrapper success/statusCode).
  // CRMContext.fetchBrokers usa `response.data?.data` que é o array direto.
  await page.route(/\/api\/v1\/brokers\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [ALICE_BROKER] }) }),
  );
}

// Catch-all para qualquer outro endpoint.
async function mockCatchAll(page: Page) {
  await page.route(/\/api\/v1\/.*/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (
      /\/auth\/(refresh|me|companies|switch-)/.test(path) ||
      /\/settings\/preferences/.test(path) ||
      /\/me\/permissions/.test(path) ||
      /\/leads(\b|$|\/)/.test(path) ||
      /\/brokers\b/.test(path)
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

/**
 * Captura a próxima request `POST /leads` para inspeção do payload, e responde com a
 * resposta especificada. O capturedRequest pode ser inspecionado após o await.
 */
type LeadCreateMock =
  | { kind: 'success'; respondWith?: any }
  | { kind: 'error'; status: number; message?: string }
  | { kind: 'network-failure' };

interface CapturedRequest {
  payload?: any;
  method?: string;
}

async function captureLeadCreate(page: Page, mock: LeadCreateMock): Promise<CapturedRequest> {
  const captured: CapturedRequest = {};
  await page.route(/\/api\/v1\/leads(\?|$).*/, async (route: Route, request: Request) => {
    if (request.method() !== 'POST') {
      await route.fallback();
      return;
    }
    captured.method = request.method();
    try {
      captured.payload = JSON.parse(request.postData() || '{}');
    } catch {
      captured.payload = request.postData();
    }
    if (mock.kind === 'success') {
      const body = mock.respondWith ?? {
        id: 'new-lead-id',
        name: captured.payload?.name,
        email: captured.payload?.email,
        phone: captured.payload?.phone,
        status: captured.payload?.status ?? 'NEW',
        source: captured.payload?.source ?? 'MANUAL',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        tenant_id: 'company-1',
        assigned_broker_id: captured.payload?.assigned_broker_id,
        role: captured.payload?.role,
      };
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 201, data: body }),
      });
    } else if (mock.kind === 'error') {
      await route.fulfill({
        status: mock.status,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, statusCode: mock.status, message: mock.message ?? 'erro' }),
      });
    } else {
      await route.abort('failed');
    }
  });
  return captured;
}

async function gotoLeads(page: Page) {
  await mockAuthBoot(page);
  await mockCrm(page);
  await mockCatchAll(page);
  await page.goto('/leads');
  await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();
}

async function openNewLeadPanel(page: Page) {
  await page.getByRole('main').getByRole('button', { name: 'Novo Lead', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeVisible();
}

// Default valid payload helper.
const VALID = {
  name: 'João Silva',
  email: 'joao@example.com',
  phoneDigits: '11987654321',
  phoneMasked: '(11) 98765-4321',
};

// Preenche o trio nome/email/telefone obrigatório.
async function fillRequiredFields(page: Page, opts: { name?: string; email?: string; phone?: string } = {}) {
  // O label do nome varia conforme o tipo de lead — usamos getByPlaceholder porque eles diferem.
  // Captação: "Ex: Maria da Silva (Proprietária)" / Outros: "Ex: João Silva"
  const nameInput = page.locator('input[placeholder*="Ex:"]').first();
  await nameInput.fill(opts.name ?? VALID.name);
  await page.getByPlaceholder('Ex: joao@email.com').fill(opts.email ?? VALID.email);
  await page.getByPlaceholder('Ex: (11) 99999-9999').fill(opts.phone ?? VALID.phoneDigits);
}

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Painel', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
  });

  test('exibe os três cards de tipo de lead', async ({ page }) => {
    await expect(page.getByText('Selecione o tipo de lead ou preencha os dados')).toBeVisible();
    await expect(page.getByRole('button', { name: /Captação de imóvel/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Compra e venda/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Locação/ })).toBeVisible();
  });

  test('Cancelar fecha o painel sem submeter', async ({ page }) => {
    let postCalled = false;
    await page.route(/\/api\/v1\/leads(\?|$).*/, async (route) => {
      if (route.request().method() === 'POST') postCalled = true;
      await route.fallback();
    });
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
    await fillRequiredFields(page);
    await page.getByRole('button', { name: 'Cancelar' }).click();
    await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeHidden();
    expect(postCalled).toBe(false);
  });
});

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Fluxo Captação', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
  });

  test('renderiza aviso de proprietário e labels específicos', async ({ page }) => {
    await expect(
      page.getByText('Este lead é um proprietário que deseja cadastrar um imóvel. Preencha os dados do proprietário abaixo.')
    ).toBeVisible();
    await expect(page.getByText('Nome do proprietário *')).toBeVisible();
    await expect(page.getByText('Imóvel para captação')).toBeVisible();
  });

  test('submit válido envia POST /leads com role=owner', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { name: 'Maria Proprietária' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();

    // Aguarda o toast de sucesso (vem do LeadsContext.addLead).
    await expect(page.getByText('Lead "Maria Proprietária" criado com sucesso!')).toBeVisible();

    expect(captured.method).toBe('POST');
    expect(captured.payload?.name).toBe('Maria Proprietária');
    expect(captured.payload?.role).toBe('owner');
    expect(captured.payload?.phone).toBe(VALID.phoneDigits); // strip de máscara
  });

  test('falha 500 mostra toast de erro e mantém o painel aberto', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'error', status: 500, message: 'Erro interno' });
    await fillRequiredFields(page, { name: 'Erro Servidor' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText(/Criar lead.*Erro interno/i)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeVisible();
  });
});

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Fluxo Compra e Venda', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Compra e venda/ }).click();
  });

  test('renderiza labels padrão sem aviso de proprietário', async ({ page }) => {
    await expect(page.getByText('Nome completo *')).toBeVisible();
    await expect(page.getByText('Imóvel de interesse')).toBeVisible();
    await expect(page.getByText('Orçamento (R$)')).toBeVisible();
    await expect(
      page.getByText('Este lead é um proprietário que deseja cadastrar um imóvel.')
    ).toBeHidden();
  });

  test('submit válido envia POST /leads com role=buyer', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { name: 'João Comprador' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Lead "João Comprador" criado com sucesso!')).toBeVisible();
    expect(captured.payload?.role).toBe('buyer');
    expect(captured.payload?.email).toBe(VALID.email);
  });

  test('atribuir corretor manualmente envia assigned_broker_id no payload', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { name: 'Manual Assignee' });
    // Abre o popover de corretor e seleciona Alice. O trigger é um combobox (tem busca),
    // então localizamos pelo elemento <button> como no seletor de tipo acima.
    await page.locator('button', { hasText: /Distribuir automaticamente|Alice Corretora/ }).first().click();
    await page.getByRole('option', { name: /Alice Corretora/ }).click();
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Lead "Manual Assignee" criado com sucesso!')).toBeVisible();
    expect(captured.payload?.assigned_broker_id).toBe(ALICE_BROKER.id);
  });

  test('roleta (atribuição automática) não envia assigned_broker_id', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { name: 'Auto Assignee' });
    // Não toca no select de corretor — fica em "Distribuir automaticamente (Roleta)"
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Lead "Auto Assignee" criado com sucesso!')).toBeVisible();
    // Quando agentId é "", o LeadsContext envia assigned_broker_id: undefined → JSON omitido
    expect(captured.payload?.assigned_broker_id).toBeUndefined();
  });

  test('erro 400 com mensagem específica é exibido no toast', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'error', status: 400, message: 'Email já cadastrado na imobiliária' });
    await fillRequiredFields(page);
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Criar lead: Email já cadastrado na imobiliária')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeVisible();
  });

  test('erro 409 (duplicata) é exibido como toast', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'error', status: 409, message: 'Lead duplicado' });
    await fillRequiredFields(page);
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Criar lead: Lead duplicado')).toBeVisible();
  });

  test('falha de rede mostra toast de erro com o contexto', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'network-failure' });
    await fillRequiredFields(page);
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText(/Criar lead:/i)).toBeVisible();
    // Painel permanece aberto (não fecha)
    await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeVisible();
  });
});

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Fluxo Locação', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Locação/ }).click();
  });

  test('renderiza label de orçamento mensal', async ({ page }) => {
    await expect(page.getByText('Orçamento mensal (R$)')).toBeVisible();
  });

  test('submit válido envia POST /leads com role=tenant', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { name: 'Bruno Inquilino' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Lead "Bruno Inquilino" criado com sucesso!')).toBeVisible();
    expect(captured.payload?.role).toBe('tenant');
  });
});

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Validações', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Compra e venda/ }).click();
  });

  test('nome com 1 caractere mostra "Nome muito curto (mínimo 2 caracteres)"', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'success' }); // não deve ser chamado
    const nameInput = page.locator('input[placeholder*="Ex:"]').first();
    await nameInput.fill('A');
    await page.getByPlaceholder('Ex: joao@email.com').fill(VALID.email);
    await page.getByPlaceholder('Ex: (11) 99999-9999').fill(VALID.phoneDigits);
    const nameField = page.getByLabel(/Nome completo/);
    await expect(nameField).toHaveAttribute('aria-required', 'true');
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Nome muito curto (mínimo 2 caracteres)')).toBeVisible();
    await expect(nameField).toHaveAttribute('aria-invalid', 'true');
    const describedBy = await nameField.getAttribute('aria-describedby');
    expect(describedBy).toBeTruthy();
    await expect(page.locator(`#${describedBy}`)).toHaveText('Nome muito curto (mínimo 2 caracteres)');
  });

  test('email inválido mostra "E-mail inválido"', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { email: 'x.exemplo.com' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  test('telefone com menos de 10 dígitos mostra erro', async ({ page }) => {
    await captureLeadCreate(page, { kind: 'success' });
    await fillRequiredFields(page, { phone: '1199' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText(/Telefone deve ter DDD/i)).toBeVisible();
  });
});

// ──────────────────────────────────────────────────────────────────────────
test.describe('Novo Lead — Navegação interna', () => {
  test.beforeEach(async ({ page }) => {
    await gotoLeads(page);
    await openNewLeadPanel(page);
  });

  test('Voltar reseta para a seleção de tipo de lead', async ({ page }) => {
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
    await expect(page.getByText('Nome do proprietário *')).toBeVisible();
    await page.getByRole('button', { name: 'Voltar' }).click();
    // Os três cards voltam a aparecer
    await expect(page.getByRole('button', { name: /Captação de imóvel/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Compra e venda/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /Locação/ })).toBeVisible();
  });

  test('Trocar de tipo via popover preserva nome e email digitados', async ({ page }) => {
    await page.getByRole('button', { name: /Compra e venda/ }).click();
    await fillRequiredFields(page);
    // Confirma que está no fluxo compra/venda
    await expect(page.getByText('Orçamento (R$)')).toBeVisible();
    // Abre o popover de tipo (botão com "Compra e venda" no header). Usamos o nome completo
    // da opção (label + descrição) para não colidir com o <option>Locação</option> do filtro
    // "Filtrar por tipo de lead" que fica na página atrás do painel.
    await page.locator('button', { hasText: /Compra e venda/ }).first().click();
    await page.getByRole('option', { name: /Locação Cliente interessado/ }).click();
    await expect(page.getByText('Orçamento mensal (R$)')).toBeVisible();
    // Nome e email permanecem
    await expect(page.locator('input[placeholder*="Ex:"]').first()).toHaveValue(VALID.name);
    await expect(page.getByPlaceholder('Ex: joao@email.com')).toHaveValue(VALID.email);
  });
});

// ──────────────────────────────────────────────────────────────────────────
// Mobile (390×844): no painel de Novo Lead o formulário é mais alto que a tela.
// As ações principais ("Criar Lead" / "Cancelar") ficavam inalcançáveis por
// duas somas de altura:
//   1. o conteúdo do painel era `h-full` — 100% do PAINEL, não do espaço que
//      sobra abaixo do header — então header + conteúdo passava da altura e o
//      excesso era recortado pelo `overflow-hidden` do painel (as ações eram o
//      último item do conteúdo rolável, junto com o resto do recorte);
//   2. `height: 100%` num elemento `position: fixed` mede a viewport GRANDE do
//      mobile (a faixa atrás da barra do navegador entra na conta).
//
// O invariante aqui é o que o usuário sente: com o painel aberto, o rodapé de
// ações está DENTRO da área visível e NÃO se move quando o formulário rola —
// a ação principal não pode depender do scroll do formulário.
test.describe('Novo Lead — painel no mobile', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  const VIEWPORT = { width: 390, height: 844 };

  async function openBuyerPanel(page: Page) {
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Compra e venda/ }).click();
  }

  function insideViewport(box: { y: number; height: number } | null, label: string) {
    expect(box, `${label} não foi renderizado`).not.toBeNull();
    const { y, height } = box!;
    expect(
      y >= 0 && y + height <= VIEWPORT.height + 1,
      `${label} fora da área visível (y=${Math.round(y)}, altura=${Math.round(height)}, viewport=${VIEWPORT.height})`,
    ).toBe(true);
  }

  test('as ações ficam na área visível e não se movem com o scroll do formulário', async ({ page }) => {
    await openBuyerPanel(page);

    const cancelar = page.getByRole('button', { name: 'Cancelar' });
    const criar = page.getByRole('button', { name: 'Criar Lead' });
    await expect(criar).toBeVisible();

    insideViewport(await criar.boundingBox(), '"Criar Lead"');
    insideViewport(await cancelar.boundingBox(), '"Cancelar"');

    // O painel em si não pode passar da viewport (é ele que recorta o conteúdo
    // que não couber).
    const panel = page.locator('[role="dialog"]').first();
    const panelBox = await panel.boundingBox();
    expect(panelBox, 'o painel não foi renderizado').not.toBeNull();
    expect(
      panelBox!.height,
      'o painel é mais alto que a viewport (o que sobra é recortado e inalcançável)',
    ).toBeLessThanOrEqual(VIEWPORT.height + 1);

    const before = await criar.boundingBox();

    // Rola a área do formulário até o fim: se as ações estivessem dentro dela
    // (como antes), o botão sairia do lugar — e no painel recortado, da tela.
    const scroller = page.locator('[role="dialog"] .overflow-y-auto').first();
    await scroller.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });

    const after = await criar.boundingBox();
    expect(after, '"Criar Lead" sumiu depois de rolar o formulário').not.toBeNull();
    expect(
      Math.abs(after!.y - before!.y) <= 1,
      `"Criar Lead" se moveu com o scroll do formulário (y ${Math.round(before!.y)} → ${Math.round(after!.y)}): a ação voltou para dentro da área rolável`,
    ).toBe(true);
    insideViewport(after, '"Criar Lead" (formulário rolado até o fim)');
  });

  test('a ação principal no mobile submete o lead', async ({ page }) => {
    const captured = await captureLeadCreate(page, { kind: 'success' });
    await openBuyerPanel(page);
    await fillRequiredFields(page, { name: 'Cliente Mobile' });
    await page.getByRole('button', { name: 'Criar Lead' }).click();
    await expect(page.getByText('Lead "Cliente Mobile" criado com sucesso!')).toBeVisible();
    expect(captured.method).toBe('POST');
    expect(captured.payload?.name).toBe('Cliente Mobile');
  });
});

//
// ──────────────────────────────────────────────────────────────────────────
// Modo noturno — seta dos <select> crus.
//
// Dois defeitos em sequência, ambos no CSS global:
//   1. `.dark select { background-image: url(<chevron>) }` sem `no-repeat`: o
//      SVG repetia por todo o campo e virava uma fileira de setas (o modo claro
//      não tem a regra, por isso só aparecia no noturno);
//   2. corrigido o repeat, ficou a soma com a seta NATIVA — duas setas no mesmo
//      campo, visível principalmente nos filtros de Leads.
//
// A regra foi removida. Esta é a prova no app real: com a folha de estilo da
// aplicação aplicada, nenhum <select> pinta background-image (a seta que se vê é
// a do controle nativo, que no noturno aparece clara porque `.dark select` fixa
// `color: var(--color-slate-100)`).
test.describe('Novo Lead — setas dos selects no modo noturno', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.localStorage.setItem('suitee-theme-settings', JSON.stringify({ theme: 'dark' }));
    });
    await gotoLeads(page);
    await openNewLeadPanel(page);
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
  });

  test('não soma uma seta pintada por CSS à seta do controle nativo', async ({ page }) => {
    await expect(page.locator('html')).toHaveClass(/dark/);

    // Inclui os filtros da barra de Leads (os <select> do print), que ficam
    // atrás do painel e são o caso mais visível do defeito.
    const selects = await page.locator('select').evaluateAll((elements) =>
      elements.map((element) => {
        const style = window.getComputedStyle(element);
        return {
          label: element.getAttribute('aria-label') ?? element.className.slice(0, 30),
          image: style.backgroundImage,
          appearance: style.appearance,
          color: style.color,
        };
      }),
    );

    // Guarda contra um seletor que não ache nada e o laço passar vazio.
    expect(selects.length).toBeGreaterThan(1);
    for (const style of selects) {
      // Quem mantém o controle nativo não pode ter seta pintada: seriam duas.
      if (style.appearance !== 'none') {
        expect(style.image, `select "${style.label}" tem seta pintada + seta nativa`).toBe('none');
      }
      // A seta nativa é desenhada com a cor do texto; clara aqui é o que a torna
      // visível no fundo escuro (é por isso que a seta por CSS era dispensável).
      expect(style.color, `select "${style.label}" sem cor clara`).not.toBe('rgb(0, 0, 0)');
    }
  });
});

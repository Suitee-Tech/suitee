import { test, expect, Page, Route } from '@playwright/test';

const TEST_USER = {
  id: 'fake-user-id',
  email: 'joao@example.com',
  firstname: 'João',
  lastname: 'Silva',
  role: 'user',
  // Conta recém-cadastrada: ainda NÃO tem vínculo, e é do CompanyMember que sai
  // o `functional_role` — o vínculo do dono é criado pelo próprio
  // `POST /api/v1/companies`. Um fixture com `'admin'` aqui descrevia um usuário
  // que não existe no onboarding e escondia o caminho do caso real.
  functional_role: '',
  account_type: 'individual',
};

const VALID = {
  name: 'Lumière Imóveis',
  legal_name: 'Lumière Negócios Imobiliários Ltda',
  // CNPJ válido com dígitos verificadores corretos (test fixture). 11.222.333/0001-81
  taxIdDigits: '11222333000181',
  taxIdMasked: '11.222.333/0001-81',
  phoneDigits: '11999998888',
  phoneMasked: '(11) 99999-8888',
  cepDigits: '01310100',
  cepMasked: '01310-100',
  street: 'Av Paulista',
  number: '1000',
  district: 'Bela Vista',
  city: 'São Paulo',
  state: 'SP',
};

// Mocks de autenticação para o onboarding (usuário autenticado, sem empresas).
async function mockAuth(page: Page) {
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
      body: JSON.stringify({ success: true, statusCode: 200, data: TEST_USER }),
    });
  });
  await page.route(/\/api\/v1\/auth\/companies\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: [] }),
    });
  });
}

type CompanyMock =
  | { kind: 'success' }
  | { kind: 'error'; status: number; message?: string }
  | { kind: 'delay'; ms: number }
  | { kind: 'network-failure' };

async function mockLogout(page: Page, opts?: { onCalled?: () => void }) {
  await page.route(/\/api\/v1\/auth\/logout-all\b.*/, async (route: Route) => {
    opts?.onCalled?.();
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200 }),
    });
  });
}

async function mockCreateCompany(page: Page, mock: CompanyMock) {
  await page.route(/\/api\/v1\/companies\b.*/, async (route: Route) => {
    if (mock.kind === 'success') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          statusCode: 201,
          data: {
            id: 'company-1',
            type: 'company',
            company: {
              id: 'c1',
              name: VALID.name,
              legal_name: VALID.legal_name,
              tax_id: VALID.taxIdDigits,
              email: TEST_USER.email,
              phone: VALID.phoneDigits,
              type: 'imobiliaria',
              is_active: true,
              members: [{ account_id: TEST_USER.id, profile: 'owner' }],
            },
          },
        }),
      });
    } else if (mock.kind === 'error') {
      await route.fulfill({
        status: mock.status,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, statusCode: mock.status, message: mock.message || 'erro' }),
      });
    } else if (mock.kind === 'delay') {
      await new Promise(r => setTimeout(r, mock.ms));
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 201, data: { id: 'c1', type: 'company', company: { id: 'c1', name: VALID.name, legal_name: VALID.legal_name, tax_id: VALID.taxIdDigits, email: TEST_USER.email, phone: VALID.phoneDigits, type: 'imobiliaria', is_active: true, members: [] } } }),
      });
    } else if (mock.kind === 'network-failure') {
      await route.abort('failed');
    }
  });
}

async function fillStep1(page: Page) {
  await page.getByLabel('Nome Fantasia').fill(VALID.name);
  await page.getByLabel('Razão Social').fill(VALID.legal_name);
  await page.getByLabel('CNPJ').fill(VALID.taxIdDigits);
  await page.getByLabel('Telefone Comercial').fill(VALID.phoneDigits);
}

async function fillStep2(page: Page) {
  await page.getByLabel('CEP').fill(VALID.cepDigits);
  await page.getByLabel('Rua / Logradouro').fill(VALID.street);
  await page.getByLabel('Número').fill(VALID.number);
  await page.getByLabel('Bairro').fill(VALID.district);
  await page.getByLabel('Cidade').fill(VALID.city);
  await selectState(page, VALID.state);
}

async function selectState(page: Page, state: string) {
  await page.getByRole('combobox', { name: 'Estado' }).click();
  await page.getByRole('option', { name: state, exact: true }).click();
}

async function gotoOnboarding(page: Page) {
  await mockAuth(page);
  await page.goto('/onboarding');
  await expect(page.getByRole('heading', { name: 'Bem-vindo(a) ao suitee!' })).toBeVisible();
}

test.describe('Onboarding - Cadastro da Imobiliária', () => {
  test.beforeEach(async ({ page }) => {
    await gotoOnboarding(page);
  });

  // @header @sair (fix do bug)
  test('botão Sair encerra a sessão e redireciona para /login', async ({ page }) => {
    let logoutCalled = false;
    await mockLogout(page, { onCalled: () => { logoutCalled = true; } });

    await page.getByRole('button', { name: 'Sair' }).click();
    await page.waitForURL(/\/login/);
    expect(page.url()).toContain('/login');
    expect(logoutCalled).toBe(true);
  });

  // @passo1 @header
  test('Passo 1 exibe título, subtítulo e campos', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Bem-vindo(a) ao suitee!' })).toBeVisible();
    await expect(page.getByText('Para começarmos, qual o nome da sua imobiliária?')).toBeVisible();
    await expect(page.getByLabel('Nome Fantasia')).toBeVisible();
    await expect(page.getByLabel('Razão Social')).toBeVisible();
    await expect(page.getByLabel('CNPJ')).toBeVisible();
    await expect(page.getByLabel('Telefone Comercial')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeVisible();
  });

  // @passo1 @validar @nome_fantasia
  test('Nome Fantasia inválido mostra erro ao perder foco', async ({ page }) => {
    const nome = page.getByLabel('Nome Fantasia');
    await nome.fill('L');
    await nome.blur();
    await expect(page.getByText('Nome da empresa é obrigatório')).toBeVisible();
  });

  // @passo1 @validar @razao_social
  test('Razão Social inválida mostra erro ao perder foco', async ({ page }) => {
    const rs = page.getByLabel('Razão Social');
    await rs.fill('L');
    await rs.blur();
    await expect(page.getByText('Razão social é obrigatória')).toBeVisible();
  });

  // @passo1 @validar @cnpj_invalido
  test('CNPJ incompleto mostra "CNPJ inválido"', async ({ page }) => {
    const cnpj = page.getByLabel('CNPJ');
    await cnpj.fill('12345');
    await cnpj.blur();
    await expect(page.getByText('CNPJ inválido')).toBeVisible();
  });

  // @passo1 @mask @cnpj
  test('CNPJ aplica máscara 00.000.000/0000-00', async ({ page }) => {
    const cnpj = page.getByLabel('CNPJ');
    await cnpj.fill(VALID.taxIdDigits);
    await expect(cnpj).toHaveValue(VALID.taxIdMasked);
  });

  // @passo1 @validar @telefone_incompleto
  test('Telefone Comercial incompleto mostra "Telefone incompleto"', async ({ page }) => {
    const tel = page.getByLabel('Telefone Comercial');
    await tel.fill('1199');
    await tel.blur();
    await expect(page.getByText('Telefone incompleto')).toBeVisible();
  });

  // @passo1 @mask @telefone
  test('Telefone Comercial aplica máscara (XX) XXXXX-XXXX', async ({ page }) => {
    const tel = page.getByLabel('Telefone Comercial');
    await tel.fill(VALID.phoneDigits);
    await expect(tel).toHaveValue(VALID.phoneMasked);
  });

  // @passo1 @habilitar-botao
  test('Próximo Passo fica habilitado com Passo 1 válido', async ({ page }) => {
    await fillStep1(page);
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeEnabled();
  });

  // @passo1 @desabilitar-botao
  test('Próximo Passo fica desabilitado inicialmente', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeDisabled();
  });

  // @passo2 @header
  test('Passo 2 exibe título, subtítulo e campos de endereço', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await expect(page.getByRole('heading', { name: 'Endereço Comercial' })).toBeVisible();
    await expect(page.getByText('Onde fica a matriz da sua operação?')).toBeVisible();
    await expect(page.getByLabel('CEP')).toBeVisible();
    await expect(page.getByLabel('Rua / Logradouro')).toBeVisible();
    await expect(page.getByLabel('Número')).toBeVisible();
    await expect(page.getByLabel('Complemento')).toBeVisible();
    await expect(page.getByLabel('Bairro')).toBeVisible();
    await expect(page.getByLabel('Cidade')).toBeVisible();
    await expect(page.getByLabel('Estado')).toBeVisible();
  });

  // @passo2 @mask @cep
  test('CEP aplica máscara 00000-000', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const cep = page.getByLabel('CEP');
    await cep.fill(VALID.cepDigits);
    await expect(cep).toHaveValue(VALID.cepMasked);
  });

  // @passo2 @validar @cep_incompleto
  test('CEP incompleto mostra "CEP incompleto"', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const cep = page.getByLabel('CEP');
    await cep.fill('01310');
    await cep.blur();
    await expect(page.getByText('CEP incompleto')).toBeVisible();
  });

  // @passo2 @validar @rua_vazia
  test('Rua / Logradouro vazia mostra "Rua é obrigatória" ao perder foco', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const rua = page.getByLabel('Rua / Logradouro');
    await rua.focus();
    await rua.blur();
    await expect(page.getByText('Rua é obrigatória')).toBeVisible();
  });

  // @passo2 @validar @numero_vazio
  test('Número vazio mostra "Número é obrigatório" ao perder foco', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const numero = page.getByLabel('Número');
    await numero.focus();
    await numero.blur();
    await expect(page.getByText('Número é obrigatório')).toBeVisible();
  });

  // @passo2 @validar @bairro_vazio
  test('Bairro vazio mostra "Bairro é obrigatório" ao perder foco', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const bairro = page.getByLabel('Bairro');
    await bairro.focus();
    await bairro.blur();
    await expect(page.getByText('Bairro é obrigatório')).toBeVisible();
  });

  // @passo2 @validar @cidade_vazia
  test('Cidade vazia mostra "Cidade é obrigatória" ao perder foco', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const cidade = page.getByLabel('Cidade');
    await cidade.focus();
    await cidade.blur();
    await expect(page.getByText('Cidade é obrigatória')).toBeVisible();
  });

  // @passo2 @validar @estado_vazio
  test('Estado vazio mostra "UF inválida" ao perder foco', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const estado = page.getByLabel('Estado');
    await estado.focus();
    await estado.blur();
    await expect(page.getByText('UF inválida')).toBeVisible();
  });

  // @passo2 @opcoes @estado_lista
  test('select Estado lista UFs disponíveis com placeholder UF', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const estado = page.getByRole('combobox', { name: 'Estado' });
    await expect(estado).toHaveText('UF');
    await estado.click();
    const options = await page.getByRole('option').allTextContents();
    expect(options).toEqual(['SP', 'RJ', 'MG', 'RS', 'PR', 'SC']);
    // "UF" é apenas o placeholder e não aparece como opção selecionável.
    await expect(page.getByRole('option', { name: 'UF', exact: true })).toHaveCount(0);
  });

  // @passo2 @opcional @complemento
  test('Complemento é opcional — botão habilita sem preencher', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    // Preenche TUDO exceto Complemento
    await page.getByLabel('CEP').fill(VALID.cepDigits);
    await page.getByLabel('Rua / Logradouro').fill(VALID.street);
    await page.getByLabel('Número').fill(VALID.number);
    await page.getByLabel('Bairro').fill(VALID.district);
    await page.getByLabel('Cidade').fill(VALID.city);
    await selectState(page, VALID.state);
    await expect(page.getByLabel('Complemento')).toHaveValue('');
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeEnabled();
  });

  // @passo2 @habilitar-botao
  test('Próximo Passo do endereço fica habilitado com campos válidos', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeEnabled();
  });

  // @passo2 @desabilitar-botao
  test('Próximo Passo do endereço começa desabilitado', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await expect(page.getByRole('button', { name: 'Próximo Passo' })).toBeDisabled();
  });

  // @passo2 @navegacao @voltar
  test('voltar para o Passo 1 preserva dados', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await page.getByRole('button', { name: 'Voltar ao passo anterior' }).click();
    await expect(page.getByRole('heading', { name: 'Bem-vindo(a) ao suitee!' })).toBeVisible();
    await expect(page.getByLabel('Nome Fantasia')).toHaveValue(VALID.name);
    await expect(page.getByLabel('Razão Social')).toHaveValue(VALID.legal_name);
    await expect(page.getByLabel('CNPJ')).toHaveValue(VALID.taxIdMasked);
    await expect(page.getByLabel('Telefone Comercial')).toHaveValue(VALID.phoneMasked);
  });

  // @passo3 @header
  test('Passo 3 exibe opções de tamanho e botão Finalizar', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await expect(page.getByRole('heading', { name: 'Tamanho da Equipe' })).toBeVisible();
    await expect(page.getByText('Quantas pessoas usarão o sistema?')).toBeVisible();
    for (const size of ['1-10', '11-50', '51-200', '200+']) {
      await expect(page.getByRole('button', { name: size, exact: true })).toBeVisible();
    }
    await expect(page.getByText('Pronto para decolar!')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Finalizar e Acessar CRM' })).toBeVisible();
  });

  // @passo3 @validar @selecionar_tamanho
  test('selecionar uma opção de tamanho destaca o botão', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    const opt = page.getByRole('button', { name: '11-50', exact: true });
    await opt.click();
    // Classe "border-primary-600" só aparece quando a opção está selecionada.
    await expect(opt).toHaveClass(/border-primary-600/);
  });

  // @passo3 @opcoes @todas_opcoes
  for (const size of ['1-10', '11-50', '51-200', '200+']) {
    test(`opção de tamanho "${size}" pode ser selecionada`, async ({ page }) => {
      await fillStep1(page);
      await page.getByRole('button', { name: 'Próximo Passo' }).click();
      await fillStep2(page);
      await page.getByRole('button', { name: 'Próximo Passo' }).click();
      const opt = page.getByRole('button', { name: size, exact: true });
      await opt.click();
      await expect(opt).toHaveClass(/border-primary-600/);
    });
  }

  // @passo3 @mensagem @pronto
  test('mensagem "Pronto para decolar!" e descrição estão visíveis', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await expect(page.getByText('Pronto para decolar!')).toBeVisible();
    await expect(
      page.getByText('Ao finalizar, criaremos seu ambiente seguro e isolado para sua equipe.')
    ).toBeVisible();
  });

  // @passo3 @navegacao @voltar
  test('voltar do Passo 3 para o Passo 2 preserva o endereço', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    // Saímos do Passo 3 voltando ao Passo 2
    await page.getByRole('button', { name: 'Voltar ao passo anterior' }).click();
    await expect(page.getByRole('heading', { name: 'Endereço Comercial' })).toBeVisible();
    await expect(page.getByLabel('CEP')).toHaveValue(VALID.cepMasked);
    await expect(page.getByLabel('Rua / Logradouro')).toHaveValue(VALID.street);
    await expect(page.getByLabel('Número')).toHaveValue(VALID.number);
    await expect(page.getByLabel('Bairro')).toHaveValue(VALID.district);
    await expect(page.getByLabel('Cidade')).toHaveValue(VALID.city);
    await expect(page.getByRole('combobox', { name: 'Estado' })).toHaveText(VALID.state);
  });

  // @passo3 @submit @sucesso
  test('Finalizar com sucesso redireciona para "/"', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'success' });
    await page.getByRole('button', { name: 'Finalizar e Acessar CRM' }).click();
    await page.waitForURL((url) => url.pathname === '/');
  });

  // @passo3 @submit @loading
  test('Finalizar exibe spinner enquanto requisição corre', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'delay', ms: 800 });
    const submit = page.getByRole('button', { name: 'Finalizar e Acessar CRM' });
    await submit.click();
    await expect(submit).toBeDisabled();
    await expect(submit.locator('.animate-spin')).toBeVisible();
  });

  // @passo3 @submit @erro @cnpj_duplicado
  test('CNPJ duplicado (409) mostra mensagem específica', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'error', status: 409, message: 'company already exists' });
    await page.getByRole('button', { name: 'Finalizar e Acessar CRM' }).click();
    await expect(
      page.getByText('Já existe uma empresa cadastrada com esse CNPJ. Se essa empresa é sua, solicite acesso ao administrador.')
    ).toBeVisible();
  });

  // @passo3 @submit @erro @servidor
  test('erro 500 mostra mensagem amigável de servidor', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'error', status: 500, message: 'Ocorreu um erro interno.' });
    await page.getByRole('button', { name: 'Finalizar e Acessar CRM' }).click();
    await expect(
      page.getByText('Não conseguimos criar sua imobiliária no momento. Tente novamente em alguns instantes.')
    ).toBeVisible();
    // A mensagem técnica do backend não deve vazar.
    await expect(page.getByText('Ocorreu um erro interno.')).toBeHidden();
  });

  // @passo3 @submit @erro @permissao
  // Regressão: o backend recusou `POST /companies` com 403
  // "permission denied: insufficient permissions" para contas sem vínculo
  // (`authz.denied` na auditoria) e a tela mostrava esse texto cru, em inglês.
  // A causa raiz foi corrigida no backend; este teste trava o contrato da UI
  // para qualquer negação futura (ex.: papel `cliente`, que segue barrado).
  test('erro 403 mostra mensagem amigável e não vaza o texto do backend', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'error', status: 403, message: 'permission denied: insufficient permissions' });
    await page.getByRole('button', { name: 'Finalizar e Acessar CRM' }).click();
    await expect(
      page.getByText('Sua conta não tem permissão para criar a imobiliária. Fale com o administrador.')
    ).toBeVisible();
    await expect(page.getByText('permission denied: insufficient permissions')).toBeHidden();
  });

  // @passo3 @submit @erro @rede
  test('falha de rede mostra mensagem amigável', async ({ page }) => {
    await fillStep1(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();
    await fillStep2(page);
    await page.getByRole('button', { name: 'Próximo Passo' }).click();

    await mockCreateCompany(page, { kind: 'network-failure' });
    await page.getByRole('button', { name: 'Finalizar e Acessar CRM' }).click();
    await expect(
      page.getByText('Sem conexão com o servidor. Verifique sua internet e tente novamente.')
    ).toBeVisible();
  });
});

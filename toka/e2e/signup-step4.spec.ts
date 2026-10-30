import { test, expect, Page, Route } from '@playwright/test';

const TEST_EMAIL = 'joao@example.com';
const TEST_PHONE_DIGITS = '11999998888';
const TEST_PHONE_MASKED = '(11) 99999-8888';
const TEST_FIRSTNAME = 'João';
const TEST_LASTNAME = 'Silva';
const TEST_ROLE_VALUE = 'Administrador';
const TEST_PASSWORD = 'SenhaForte1!';

// Roteia o /auth/register conforme o caso de teste configurado pelo cenário.
type RegisterMock =
  | { kind: 'success' }
  | { kind: 'error'; status: number; message?: string }
  | { kind: 'delay'; ms: number }
  | { kind: 'network-failure' };

// Para sucesso, também precisamos mockar /auth/me e /auth/companies, que o AuthContext
// chama logo após o registro via fetchUserData.
async function mockPostRegisterCalls(page: Page) {
  await page.route(/\/api\/v1\/auth\/me\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: {
          id: 'fake-user-id',
          email: TEST_EMAIL,
          firstname: TEST_FIRSTNAME,
          lastname: TEST_LASTNAME,
          role: 'user',
          // Conta recém-cadastrada: ainda sem vínculo, portanto sem
          // `functional_role` — ele nasce no POST /api/v1/companies, junto com
          // o CompanyMember do dono.
          functional_role: '',
          account_type: 'individual',
        },
      }),
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

async function mockRegister(page: Page, mock: RegisterMock) {
  await page.route(/\/api\/v1\/auth\/register\b.*/, async (route: Route) => {
    if (mock.kind === 'success') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          message: 'User registered successfully',
          statusCode: 201,
          data: { access_token: 'fake-access-token' },
        }),
      });
    } else if (mock.kind === 'error') {
      await route.fulfill({
        status: mock.status,
        contentType: 'application/json',
        body: JSON.stringify({
          success: false,
          message: mock.message || 'erro',
          statusCode: mock.status,
        }),
      });
    } else if (mock.kind === 'delay') {
      await new Promise((r) => setTimeout(r, mock.ms));
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 201, data: { access_token: 'fake' } }),
      });
    } else if (mock.kind === 'network-failure') {
      await route.abort('failed');
    }
  });
}

async function gotoStep4(page: Page) {
  await page.goto('/login');
  await page.getByRole('button', { name: 'Cadastre-se' }).click();
  // Passo 1
  await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill(TEST_EMAIL);
  await page.getByPlaceholder('(00) 00000-0000').fill(TEST_PHONE_DIGITS);
  await page.getByRole('button', { name: /Continuar/i }).click();
  await expect(page.getByText('Passo 2 de 4')).toBeVisible();
  // Passo 2
  await page.getByLabel('Nome', { exact: true }).fill(TEST_FIRSTNAME);
  await page.getByLabel('Sobrenome', { exact: true }).fill(TEST_LASTNAME);
  await page.getByLabel('Cargo / Função').selectOption(TEST_ROLE_VALUE);
  await page.getByRole('button', { name: /Continuar/i }).click();
  await expect(page.getByText('Passo 3 de 4')).toBeVisible();
  // Passo 3
  await page.getByLabel('Senha', { exact: true }).fill(TEST_PASSWORD);
  await page.getByLabel('Confirme a Senha', { exact: true }).fill(TEST_PASSWORD);
  await page.getByRole('button', { name: /Continuar/i }).click();
  await expect(page.getByText('Passo 4 de 4')).toBeVisible();
}

test.describe('Cadastro - Passo 4 (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await gotoStep4(page);
  });

  test('exibe header, card de revisão e botão de submit', async ({ page }) => {
    await expect(page.getByText('Passo 4 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Revise seus dados' })).toBeVisible();
    await expect(page.getByText('Tudo certo? Confirme para criar sua conta.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Criar minha conta' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Voltar ao passo anterior' })).toBeVisible();
  });

  // @revisao @avatar
  test('avatar mostra a inicial do primeiro nome', async ({ page }) => {
    // Avatar é o círculo com inicial — usamos seu container conhecido (classe + texto único).
    const avatar = page.locator('.rounded-full', { hasText: TEST_FIRSTNAME.charAt(0) }).first();
    await expect(avatar).toBeVisible();
    await expect(avatar).toHaveText(TEST_FIRSTNAME.charAt(0));
  });

  // @revisao @dados (nome completo)
  test('card exibe nome completo', async ({ page }) => {
    await expect(page.getByText(`${TEST_FIRSTNAME} ${TEST_LASTNAME}`, { exact: true })).toBeVisible();
  });

  // @revisao @dados (cargo)
  test('card exibe o cargo selecionado', async ({ page }) => {
    await expect(page.getByText(TEST_ROLE_VALUE, { exact: true })).toBeVisible();
  });

  // @revisao @dados (email)
  test('card exibe o e-mail informado', async ({ page }) => {
    await expect(page.getByText(TEST_EMAIL, { exact: true })).toBeVisible();
  });

  // @revisao @dados (telefone formatado)
  test('card exibe telefone com máscara (XX) XXXXX-XXXX', async ({ page }) => {
    await expect(page.getByText(TEST_PHONE_MASKED, { exact: true })).toBeVisible();
  });

  // @revisao @termos
  test('aviso de termos de serviço está visível', async ({ page }) => {
    // Copy atual (commit 61d4ea9f): "Termos de Uso" + "Política de Privacidade".
    await expect(page.getByText(/Ao clicar em "Criar conta"/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Termos de Uso' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Política de Privacidade' })).toBeVisible();
  });

  // @submit @sucesso
  test('criar conta com sucesso redireciona para /onboarding', async ({ page }) => {
    await mockPostRegisterCalls(page);
    await mockRegister(page, { kind: 'success' });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await page.waitForURL(/\/onboarding/);
    expect(page.url()).toContain('/onboarding');
  });

  // @submit @loading
  test('botão fica desabilitado e exibe spinner durante a requisição', async ({ page }) => {
    await mockPostRegisterCalls(page);
    await mockRegister(page, { kind: 'delay', ms: 800 });
    const submit = page.getByRole('button', { name: 'Criar minha conta' });
    await submit.click();
    // Após o click, o conteúdo do botão muda para spinner e fica desabilitado.
    await expect(submit).toBeDisabled();
    await expect(submit.locator('.animate-spin')).toBeVisible();
  });

  // @submit @erro @conflito — 409 genérico não confirma qual dado já existe
  test('conflito 409 volta ao Passo 1 com mensagem genérica, sem apontar o campo', async ({ page }) => {
    // Mesmo que o backend devolva uma mensagem específica (legado), a borda
    // ignora e mostra o texto genérico — nada de confirmar qual dado existe.
    await mockRegister(page, { kind: 'error', status: 409, message: 'Este e-mail já está cadastrado.' });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await expect(page.getByText('Passo 1 de 4')).toBeVisible();
    await expect(page.getByText('Não foi possível concluir o cadastro com os dados informados.')).toBeVisible();
    await expect(page.getByText('Este e-mail já está cadastrado.')).toBeHidden();
    await expect(page.getByText('E-mail já está sendo utilizado')).toBeHidden();
    await expect(page.getByText('Telefone já está sendo utilizado')).toBeHidden();
    await expect(page.locator('p').filter({ hasText: 'Este e-mail já está cadastrado' })).toBeHidden();
    await expect(page.locator('p').filter({ hasText: 'Este telefone já está cadastrado' })).toBeHidden();
  });

  // @submit @erro @generico_400 — qualquer outro 400 mostra a mensagem do backend
  test('mensagem do backend é exibida em erro 400 genérico', async ({ page }) => {
    const backendMsg = 'Falha no cadastro. Verifique os dados informados ou tente fazer login.';
    await mockRegister(page, { kind: 'error', status: 400, message: backendMsg });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await expect(page.getByText(backendMsg)).toBeVisible();
  });

  // @submit @erro @servidor — 500
  test('erro 500 mostra mensagem amigável de servidor', async ({ page }) => {
    await mockRegister(page, { kind: 'error', status: 500, message: 'Ocorreu um erro interno.' });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await expect(
      page.getByText('Não conseguimos criar sua conta no momento. Tente novamente em alguns instantes.')
    ).toBeVisible();
    // O texto técnico do backend não deve aparecer ao usuário.
    await expect(page.getByText('Ocorreu um erro interno.')).toBeHidden();
  });

  // @submit @erro @servidor — 503
  test('erro 503 mostra a mesma mensagem amigável de servidor', async ({ page }) => {
    await mockRegister(page, { kind: 'error', status: 503 });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await expect(
      page.getByText('Não conseguimos criar sua conta no momento. Tente novamente em alguns instantes.')
    ).toBeVisible();
  });

  // @submit @erro @rede — falha de rede
  test('falha de rede mostra mensagem amigável de conexão', async ({ page }) => {
    await mockRegister(page, { kind: 'network-failure' });
    await page.getByRole('button', { name: 'Criar minha conta' }).click();
    await expect(
      page.getByText('Sem conexão com o servidor. Verifique sua internet e tente novamente.')
    ).toBeVisible();
  });

  // @navegacao @voltar
  test('voltar para o Passo 3 preserva senha e confirme a senha', async ({ page }) => {
    await page.getByRole('button', { name: 'Voltar ao passo anterior' }).click();
    await expect(page.getByText('Passo 3 de 4')).toBeVisible();
    await expect(page.getByLabel('Senha', { exact: true })).toHaveValue(TEST_PASSWORD);
    await expect(page.getByLabel('Confirme a Senha', { exact: true })).toHaveValue(TEST_PASSWORD);
  });
});

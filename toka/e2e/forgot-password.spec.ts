import { test, expect, Page, Route } from '@playwright/test';

type ForgotMock =
  | { kind: 'success' }
  | { kind: 'error'; status: number; message?: string }
  | { kind: 'delay'; ms: number }
  | { kind: 'network-failure' };

const VALID_EMAIL = 'ana@example.com';

async function mockForgot(page: Page, mock: ForgotMock) {
  await page.route(/\/api\/v1\/auth\/forgot-password\b.*/, async (route: Route) => {
    if (mock.kind === 'success') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          statusCode: 200,
          message: 'If the email exists, a reset link has been sent',
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
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, statusCode: 200 }),
      });
    } else if (mock.kind === 'network-failure') {
      await route.abort('failed');
    }
  });
}

async function gotoForgot(page: Page) {
  await page.goto('/login');
  await page.getByRole('button', { name: 'Esqueci minha senha' }).click();
  await expect(page.getByRole('heading', { name: 'Recuperar Senha' })).toBeVisible();
}

test.describe('Recuperar Senha (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await gotoForgot(page);
  });

  test('exibe título, subtítulo, campo e botões', async ({ page }) => {
    await expect(page.getByText('Informe seu e-mail e enviaremos um link para redefinir sua senha.')).toBeVisible();
    await expect(page.getByLabel('E-mail da Conta')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Enviar Link de Recuperação' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Voltar para o Login' })).toBeVisible();
  });

  // @input @validar @email_invalido
  test('email inválido mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    const email = page.getByLabel('E-mail da Conta');
    await email.fill('ana.example.com');
    await email.blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  // @input @validar @email_vazio
  test('email vazio mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    const email = page.getByLabel('E-mail da Conta');
    await email.focus();
    await email.blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  // @habilitar-botao
  test('Enviar fica habilitado com email válido', async ({ page }) => {
    await page.getByLabel('E-mail da Conta').fill(VALID_EMAIL);
    await expect(page.getByRole('button', { name: 'Enviar Link de Recuperação' })).toBeEnabled();
  });

  // @desabilitar-botao (vazio)
  test('Enviar começa desabilitado', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Enviar Link de Recuperação' })).toBeDisabled();
  });

  // @desabilitar-botao (email inválido)
  test('Enviar fica desabilitado com email inválido', async ({ page }) => {
    await page.getByLabel('E-mail da Conta').fill('ana.example.com');
    await expect(page.getByRole('button', { name: 'Enviar Link de Recuperação' })).toBeDisabled();
  });

  // @submit @sucesso
  test('envio com sucesso mostra confirmação com o e-mail informado', async ({ page }) => {
    await mockForgot(page, { kind: 'success' });
    await page.getByLabel('E-mail da Conta').fill(VALID_EMAIL);
    await page.getByRole('button', { name: 'Enviar Link de Recuperação' }).click();
    await expect(page.getByRole('heading', { name: 'E-mail enviado!' })).toBeVisible();
    await expect(page.getByText(VALID_EMAIL)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Voltar para o Login' })).toBeVisible();
  });

  // @submit @loading
  test('botão exibe spinner durante a requisição', async ({ page }) => {
    await mockForgot(page, { kind: 'delay', ms: 800 });
    await page.getByLabel('E-mail da Conta').fill(VALID_EMAIL);
    const submit = page.getByRole('button', { name: 'Enviar Link de Recuperação' });
    await submit.click();
    await expect(submit).toBeDisabled();
    await expect(submit.locator('.animate-spin')).toBeVisible();
  });

  // @submit @erro @servidor
  test('erro 500 mostra mensagem amigável', async ({ page }) => {
    await mockForgot(page, { kind: 'error', status: 500, message: 'Ocorreu um erro interno.' });
    await page.getByLabel('E-mail da Conta').fill(VALID_EMAIL);
    await page.getByRole('button', { name: 'Enviar Link de Recuperação' }).click();
    await expect(
      page.getByText('Não conseguimos enviar o e-mail agora. Tente novamente em alguns instantes.')
    ).toBeVisible();
    await expect(page.getByText('Ocorreu um erro interno.')).toBeHidden();
  });

  // @submit @erro @rede
  test('falha de rede mostra mensagem amigável', async ({ page }) => {
    await mockForgot(page, { kind: 'network-failure' });
    await page.getByLabel('E-mail da Conta').fill(VALID_EMAIL);
    await page.getByRole('button', { name: 'Enviar Link de Recuperação' }).click();
    await expect(
      page.getByText('Sem conexão com o servidor. Verifique sua internet e tente novamente.')
    ).toBeVisible();
  });

  // @navegacao @voltar
  test('Voltar para o Login retorna ao formulário de login', async ({ page }) => {
    await page.getByRole('button', { name: 'Voltar para o Login' }).click();
    await expect(page.getByRole('heading', { name: 'Bem-vindo de volta!' })).toBeVisible();
  });
});

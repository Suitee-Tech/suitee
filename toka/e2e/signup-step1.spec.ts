import { test, expect } from '@playwright/test';

test.describe('Cadastro - Passo 1 (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('button', { name: 'Cadastre-se' }).click();
  });

  test('exibe header e campos do Passo 1', async ({ page }) => {
    await expect(page.getByText('Passo 1 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Seus dados de contato' })).toBeVisible();
    await expect(page.getByText('Vamos começar com o básico para sua conta.')).toBeVisible();
    await expect(page.getByPlaceholder('nome@suaimobiliaria.com.br')).toBeVisible();
    await expect(page.getByPlaceholder('(00) 00000-0000')).toBeVisible();
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeVisible();
  });

  // @input @validar @email_valido + @telefone_valido
  test('email e telefone válidos avançam para o Passo 2', async ({ page }) => {
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill(`e2e-${Date.now()}@test.com`);
    await page.getByPlaceholder('(00) 00000-0000').fill('11999998888');
    await page.getByRole('button', { name: /Continuar/i }).click();
    await expect(page.getByText('Passo 2 de 4')).toBeVisible();
  });

  // O Passo 1 não consulta mais disponibilidade: um e-mail já cadastrado não
  // pode ser confirmado pela tela antes do submit (anti-enumeração).
  test('não consulta disponibilidade no Passo 1', async ({ page }) => {
    let availabilityChecked = false;
    await page.route(/\/api\/v1\/auth\/check-(email|phone)\b.*/, async (route) => {
      availabilityChecked = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, message: 'ok', statusCode: 200, data: { available: false } }),
      });
    });

    await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill('ja-existe@example.com');
    await page.getByPlaceholder('(00) 00000-0000').fill('11999998888');
    await page.waitForTimeout(600);

    expect(availabilityChecked).toBe(false);
    await expect(page.getByText('E-mail já está sendo utilizado')).toBeHidden();
    await expect(page.getByText('Telefone já está sendo utilizado')).toBeHidden();

    const button = page.getByRole('button', { name: /Continuar/i });
    await expect(button).toBeEnabled();
    await button.click();
    await expect(page.getByText('Passo 2 de 4')).toBeVisible();
  });

  // @input @validar @email_invalido
  test('email inválido mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill('email.exemplo.com');
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
    await expect(page.getByText('Passo 1 de 4')).toBeVisible();
  });

  // @input @validar @email_vazio
  // Spec esperava "Por favor, insira seu e-mail"; código mostra "E-mail inválido" (ver VALIDATION.md #7).
  test('email vazio mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').focus();
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  // @input @validar @telefone_invalido
  test('telefone curto mostra "Telefone inválido (mínimo 10 dígitos)" ao perder foco', async ({ page }) => {
    await page.getByPlaceholder('(00) 00000-0000').fill('1199');
    await page.getByPlaceholder('(00) 00000-0000').blur();
    await expect(page.getByText('Telefone inválido (mínimo 10 dígitos)')).toBeVisible();
    await expect(page.getByText('Passo 1 de 4')).toBeVisible();
  });

  // @input @validar @telefone_vazio
  // Spec esperava "Por favor, insira seu telefone"; código mostra "Telefone inválido (mínimo 10 dígitos)" (ver VALIDATION.md #7).
  test('telefone vazio mostra "Telefone inválido (mínimo 10 dígitos)" ao perder foco', async ({ page }) => {
    await page.getByPlaceholder('(00) 00000-0000').focus();
    await page.getByPlaceholder('(00) 00000-0000').blur();
    await expect(page.getByText('Telefone inválido (mínimo 10 dígitos)')).toBeVisible();
  });

  // @input @mask @telefone-masked
  test('telefone aplica máscara (XX) XXXXX-XXXX', async ({ page }) => {
    const phone = page.getByPlaceholder('(00) 00000-0000');
    await phone.fill('11999998888');
    await expect(phone).toHaveValue('(11) 99999-8888');
  });

  // @input @mask @email-masked
  // Email não tem máscara real (input type="email" puro). Verificamos que o valor é preservado sem transformação.
  test('email não é transformado por máscara', async ({ page }) => {
    const email = page.getByPlaceholder('nome@suaimobiliaria.com.br');
    await email.fill('user@example.com');
    await expect(email).toHaveValue('user@example.com');
  });

  // @habilitar-botao @continuar
  test('botão Continuar fica habilitado quando ambos os campos são válidos', async ({ page }) => {
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill(`disponivel-${Date.now()}@example.com`);
    await page.getByPlaceholder('(00) 00000-0000').fill('11999998888');
    const button = page.getByRole('button', { name: /Continuar/i });
    await expect(button).toBeEnabled();
  });

  // @desabilitar-botao @continuar
  test('botão Continuar fica desabilitado quando algum campo é inválido', async ({ page }) => {
    const button = page.getByRole('button', { name: /Continuar/i });

    // Estado inicial: tudo vazio → desabilitado
    await expect(button).toBeDisabled();

    // Telefone curto → ainda desabilitado mesmo com email válido
    await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill('ok@example.com');
    await page.getByPlaceholder('(00) 00000-0000').fill('11');
    await expect(button).toBeDisabled();
  });
});

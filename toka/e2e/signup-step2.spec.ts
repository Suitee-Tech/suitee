import { test, expect, Page } from '@playwright/test';

// Avança do Passo 1 ao Passo 2 com dados válidos.
async function gotoStep2(page: Page) {
  await page.goto('/login');
  await page.getByRole('button', { name: 'Cadastre-se' }).click();
  await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill(`step2-${Date.now()}@example.com`);
  await page.getByPlaceholder('(00) 00000-0000').fill('11999998888');
  const continuar = page.getByRole('button', { name: /Continuar/i });
  await expect(continuar).toBeEnabled();
  await continuar.click();
  await expect(page.getByText('Passo 2 de 4')).toBeVisible();
}

test.describe('Cadastro - Passo 2 (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await gotoStep2(page);
  });

  test('exibe header e campos do Passo 2', async ({ page }) => {
    await expect(page.getByText('Passo 2 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Identificação e Perfil' })).toBeVisible();
    await expect(page.getByText('Como devemos chamar você na plataforma?')).toBeVisible();
    await expect(page.getByText('Nome', { exact: true })).toBeVisible();
    await expect(page.getByText('Sobrenome', { exact: true })).toBeVisible();
    await expect(page.getByText('Cargo / Função')).toBeVisible();
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeVisible();
  });

  // @input @validar @nome_valido
  test('nome válido é aceito', async ({ page }) => {
    await page.getByLabel('Nome', { exact: true }).fill('João');
    await page.getByLabel('Nome', { exact: true }).blur();
    await expect(page.getByText('Nome deve ter pelo menos 2 caracteres')).toBeHidden();
  });

  // @input @validar @nome_invalido
  test('nome com 1 caractere mostra "Nome deve ter pelo menos 2 caracteres"', async ({ page }) => {
    const nome = page.getByLabel('Nome', { exact: true });
    await nome.fill('J');
    await nome.blur();
    await expect(page.getByText('Nome deve ter pelo menos 2 caracteres')).toBeVisible();
  });

  // @input @validar @nome_vazio
  test('nome vazio mostra "Nome deve ter pelo menos 2 caracteres" ao perder foco', async ({ page }) => {
    const nome = page.getByLabel('Nome', { exact: true });
    await nome.focus();
    await nome.blur();
    await expect(page.getByText('Nome deve ter pelo menos 2 caracteres')).toBeVisible();
  });

  // @input @validar @sobrenome_valido
  test('sobrenome válido é aceito', async ({ page }) => {
    await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');
    await page.getByLabel('Sobrenome', { exact: true }).blur();
    await expect(page.getByText('Sobrenome deve ter pelo menos 2 caracteres')).toBeHidden();
  });

  // @input @validar @sobrenome_invalido
  test('sobrenome com 1 caractere mostra "Sobrenome deve ter pelo menos 2 caracteres"', async ({ page }) => {
    const sobrenome = page.getByLabel('Sobrenome', { exact: true });
    await sobrenome.fill('S');
    await sobrenome.blur();
    await expect(page.getByText('Sobrenome deve ter pelo menos 2 caracteres')).toBeVisible();
  });

  // @input @validar @sobrenome_vazio
  test('sobrenome vazio mostra "Sobrenome deve ter pelo menos 2 caracteres" ao perder foco', async ({ page }) => {
    const sobrenome = page.getByLabel('Sobrenome', { exact: true });
    await sobrenome.focus();
    await sobrenome.blur();
    await expect(page.getByText('Sobrenome deve ter pelo menos 2 caracteres')).toBeVisible();
  });

  // @input @opcoes @cargo_opcoes
  test('lista as 4 opções de cargo disponíveis', async ({ page }) => {
    const cargo = page.getByLabel('Cargo / Função');
    const options = await cargo.locator('option').allTextContents();
    expect(options).toEqual([
      'Administrador da Imobiliária',
      'Gerente de Vendas / Locação',
      'Corretor / Captador',
      'Secretário / Administrativo',
    ]);
  });

  // @input @valor-padrao @cargo_default
  test('cargo padrão pré-selecionado é "Corretor / Captador"', async ({ page }) => {
    const cargo = page.getByLabel('Cargo / Função');
    await expect(cargo).toHaveValue('Corretor');
    const selected = await cargo.locator('option:checked').textContent();
    expect(selected?.trim()).toBe('Corretor / Captador');
  });

  // @input @validar @cargo_selecionavel
  test('selecionar um cargo diferente atualiza o valor', async ({ page }) => {
    const cargo = page.getByLabel('Cargo / Função');
    await cargo.selectOption({ label: 'Gerente de Vendas / Locação' });
    await expect(cargo).toHaveValue('Gerente');
  });

  // @habilitar-botao @continuar
  test('botão Continuar fica habilitado quando nome, sobrenome e cargo são válidos', async ({ page }) => {
    await page.getByLabel('Nome', { exact: true }).fill('João');
    await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');
    // Cargo já vem com default válido "Corretor".
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeEnabled();
  });

  // @desabilitar-botao @continuar (nome inválido)
  test('botão Continuar fica desabilitado quando nome tem só 1 caractere', async ({ page }) => {
    await page.getByLabel('Nome', { exact: true }).fill('J');
    await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @desabilitar-botao @continuar (sobrenome inválido)
  test('botão Continuar fica desabilitado quando sobrenome tem só 1 caractere', async ({ page }) => {
    await page.getByLabel('Nome', { exact: true }).fill('João');
    await page.getByLabel('Sobrenome', { exact: true }).fill('S');
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @desabilitar-botao @continuar (ambos vazios — estado inicial)
  test('botão Continuar começa desabilitado no Passo 2 (nome e sobrenome vazios)', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @navegacao @voltar
  test('voltar para o Passo 1 preserva email e telefone', async ({ page }) => {
    // Preenche dados no Passo 2 para garantir que o "voltar" também preserva eles
    await page.getByLabel('Nome', { exact: true }).fill('João');
    await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');

    const voltar = page.getByRole('button', { name: 'Voltar ao passo anterior' });
    await voltar.click();

    await expect(page.getByText('Passo 1 de 4')).toBeVisible();
    await expect(page.getByPlaceholder('nome@suaimobiliaria.com.br')).toHaveValue(/step2-\d+@example\.com/);
    await expect(page.getByPlaceholder('(00) 00000-0000')).toHaveValue('(11) 99999-8888');
  });

  // @navegacao @avancar
  test('avançar para o Passo 3 com todos os campos válidos', async ({ page }) => {
    await page.getByLabel('Nome', { exact: true }).fill('João');
    await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');
    await page.getByRole('button', { name: /Continuar/i }).click();
    await expect(page.getByText('Passo 3 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Crie sua Senha' })).toBeVisible();
  });
});

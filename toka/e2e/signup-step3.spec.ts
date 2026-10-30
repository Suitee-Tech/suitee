import { test, expect, Page } from '@playwright/test';

// Avança do Passo 1 ao Passo 3 com dados válidos.
async function gotoStep3(page: Page) {
  await page.goto('/login');
  await page.getByRole('button', { name: 'Cadastre-se' }).click();
  await page.getByPlaceholder('nome@suaimobiliaria.com.br').fill(`step3-${Date.now()}@example.com`);
  await page.getByPlaceholder('(00) 00000-0000').fill('11999998888');
  await page.getByRole('button', { name: /Continuar/i }).click();
  await expect(page.getByText('Passo 2 de 4')).toBeVisible();
  await page.getByLabel('Nome', { exact: true }).fill('João');
  await page.getByLabel('Sobrenome', { exact: true }).fill('Silva');
  await page.getByRole('button', { name: /Continuar/i }).click();
  await expect(page.getByText('Passo 3 de 4')).toBeVisible();
}

test.describe('Cadastro - Passo 3 (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await gotoStep3(page);
  });

  test('exibe header e campos do Passo 3', async ({ page }) => {
    await expect(page.getByText('Passo 3 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Crie sua Senha' })).toBeVisible();
    await expect(page.getByText('Escolha uma senha forte e segura (mín. 8 caracteres).')).toBeVisible();
    await expect(page.getByLabel('Senha', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Confirme a Senha', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Alternar visibilidade da senha' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeVisible();
  });

  // @input @validar @senha_valida
  test('senha forte é aceita', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('SenhaForte1!');
    await senha.blur();
    await expect(page.getByText('A senha deve ter pelo menos 8 caracteres')).toBeHidden();
    await expect(page.getByText('A senha deve conter pelo menos uma letra maiúscula')).toBeHidden();
  });

  // @input @validar @senha_curta
  test('senha com menos de 8 caracteres mostra "A senha deve ter pelo menos 8 caracteres"', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('Sf1!');
    await senha.blur();
    await expect(page.getByText('A senha deve ter pelo menos 8 caracteres')).toBeVisible();
  });

  // @input @validar @senha_sem_maiuscula
  test('senha sem letra maiúscula mostra mensagem específica', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('senhaforte1!');
    await senha.blur();
    await expect(page.getByText('A senha deve conter pelo menos uma letra maiúscula')).toBeVisible();
  });

  // @input @validar @senha_sem_minuscula
  test('senha sem letra minúscula mostra mensagem específica', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('SENHAFORTE1!');
    await senha.blur();
    await expect(page.getByText('A senha deve conter pelo menos uma letra minúscula')).toBeVisible();
  });

  // @input @validar @senha_sem_numero
  test('senha sem número mostra mensagem específica', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('SenhaForte!');
    await senha.blur();
    await expect(page.getByText('A senha deve conter pelo menos um número')).toBeVisible();
  });

  // @input @validar @senha_sem_especial
  test('senha sem caractere especial mostra mensagem específica', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('SenhaForte1');
    await senha.blur();
    await expect(page.getByText('A senha deve conter pelo menos um caractere especial')).toBeVisible();
  });

  // @input @validar @senha_vazia
  test('senha vazia mostra "A senha deve ter pelo menos 8 caracteres" ao perder foco', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.focus();
    await senha.blur();
    await expect(page.getByText('A senha deve ter pelo menos 8 caracteres')).toBeVisible();
  });

  // @input @validar @confirme_senha_igual
  test('confirme a senha igual à senha é aceita', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('SenhaForte1!');
    const confirm = page.getByLabel('Confirme a Senha', { exact: true });
    await confirm.fill('SenhaForte1!');
    await confirm.blur();
    await expect(page.getByText('As senhas não coincidem')).toBeHidden();
  });

  // @input @validar @confirme_senha_diferente
  test('confirme a senha diferente mostra "As senhas não coincidem"', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('SenhaForte1!');
    const confirm = page.getByLabel('Confirme a Senha', { exact: true });
    await confirm.fill('Outra123!');
    await confirm.blur();
    await expect(page.getByText('As senhas não coincidem')).toBeVisible();
  });

  // @input @visibilidade @toggle_senha
  test('botão de visibilidade alterna o type do campo senha', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('SenhaForte1!');
    await expect(senha).toHaveAttribute('type', 'password');

    const toggle = page.getByRole('button', { name: 'Alternar visibilidade da senha' });
    await toggle.click();
    await expect(senha).toHaveAttribute('type', 'text');

    await toggle.click();
    await expect(senha).toHaveAttribute('type', 'password');
  });

  // @habilitar-botao @continuar
  test('botão Continuar fica habilitado com senha forte e confirme igual', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('SenhaForte1!');
    await page.getByLabel('Confirme a Senha', { exact: true }).fill('SenhaForte1!');
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeEnabled();
  });

  // @desabilitar-botao @continuar (senha fraca)
  test('botão Continuar fica desabilitado quando senha não atende aos requisitos', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('fraca');
    await page.getByLabel('Confirme a Senha', { exact: true }).fill('fraca');
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @desabilitar-botao @continuar (senhas diferentes)
  test('botão Continuar fica desabilitado quando senhas não coincidem', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('SenhaForte1!');
    await page.getByLabel('Confirme a Senha', { exact: true }).fill('Outra123!');
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @desabilitar-botao @continuar (estado inicial)
  test('botão Continuar começa desabilitado no Passo 3 (campos vazios)', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Continuar/i })).toBeDisabled();
  });

  // @navegacao @voltar
  test('voltar para o Passo 2 preserva nome, sobrenome e cargo', async ({ page }) => {
    const voltar = page.getByRole('button', { name: 'Voltar ao passo anterior' });
    await voltar.click();

    await expect(page.getByText('Passo 2 de 4')).toBeVisible();
    await expect(page.getByLabel('Nome', { exact: true })).toHaveValue('João');
    await expect(page.getByLabel('Sobrenome', { exact: true })).toHaveValue('Silva');
    await expect(page.getByLabel('Cargo / Função')).toHaveValue('Corretor');
  });

  // @navegacao @avancar
  test('avançar para o Passo 4 com senha forte + confirme igual', async ({ page }) => {
    await page.getByLabel('Senha', { exact: true }).fill('SenhaForte1!');
    await page.getByLabel('Confirme a Senha', { exact: true }).fill('SenhaForte1!');
    await page.getByRole('button', { name: /Continuar/i }).click();
    await expect(page.getByText('Passo 4 de 4')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Revise seus dados' })).toBeVisible();
  });
});

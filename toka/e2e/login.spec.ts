import { test, expect, Page, Route } from '@playwright/test';

type LoginMock =
  | { kind: 'success' }
  | { kind: 'error'; status: number; message?: string }
  | { kind: 'delay'; ms: number }
  | { kind: 'network-failure' };

const VALID_EMAIL = 'ana@example.com';
const VALID_PASSWORD = 'minhasenha';

async function mockLogin(page: Page, mock: LoginMock) {
  await page.route(/\/api\/v1\/auth\/login\b.*/, async (route: Route) => {
    if (mock.kind === 'success') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          statusCode: 200,
          data: { access_token: 'fake-access-token' },
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
        body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake' } }),
      });
    } else if (mock.kind === 'network-failure') {
      await route.abort('failed');
    }
  });
}

// Mocks dos endpoints que o AuthContext.fetchUserData consome após o login.
async function mockPostLogin(page: Page, companies: any[] = []) {
  await page.route(/\/api\/v1\/auth\/me\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: {
          id: 'fake-user-id',
          email: VALID_EMAIL,
          firstname: 'Ana',
          lastname: 'Souza',
          role: 'user',
          functional_role: 'admin',
          account_type: 'individual',
          current_company_id: companies.length > 0 ? companies[0].id : undefined,
        },
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
  // /settings/preferences: usado para sincronizar a empresa selecionada
  await page.route(/\/api\/v1\/settings\/preferences\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { last_company_id: null } }),
    });
  });
  // /auth/switch-context: chamado automaticamente para vincular o token à empresa selecionada
  await page.route(/\/api\/v1\/auth\/switch-(context|company)\b.*/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, statusCode: 200, data: { access_token: 'fake-tenant-scoped-token' } }),
    });
  });
}

test.describe('Login (UI)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'Bem-vindo de volta!' })).toBeVisible();
  });

  test('exibe título, subtítulo, campos e link de senha', async ({ page }) => {
    await expect(page.getByText('Insira suas credenciais para acessar sua conta.')).toBeVisible();
    await expect(page.getByLabel('E-mail Corporativo')).toBeVisible();
    await expect(page.getByLabel('Senha', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Esqueci minha senha' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Entrar na conta' })).toBeVisible();
    await expect(page.getByText('Recebeu um convite?')).toBeVisible();
  });

  // @input @validar @email_invalido
  test('email inválido mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    const email = page.getByLabel('E-mail Corporativo');
    await email.fill('ana.example.com');
    await email.blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  // @input @validar @email_vazio
  test('email vazio mostra "E-mail inválido" ao perder foco', async ({ page }) => {
    const email = page.getByLabel('E-mail Corporativo');
    await email.focus();
    await email.blur();
    await expect(page.getByText('E-mail inválido')).toBeVisible();
  });

  // @input @visibilidade @toggle_senha
  test('botão de visibilidade alterna o type da senha', async ({ page }) => {
    const senha = page.getByLabel('Senha', { exact: true });
    await senha.fill('minhasenha');
    await expect(senha).toHaveAttribute('type', 'password');
    const toggle = page.getByRole('button', { name: 'Alternar visibilidade da senha' });
    await toggle.click();
    await expect(senha).toHaveAttribute('type', 'text');
    await toggle.click();
    await expect(senha).toHaveAttribute('type', 'password');
  });

  // @habilitar-botao
  test('Entrar fica habilitado com email e senha válidos', async ({ page }) => {
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    await expect(page.getByRole('button', { name: 'Entrar na conta' })).toBeEnabled();
  });

  // @desabilitar-botao (campos vazios)
  test('Entrar começa desabilitado', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Entrar na conta' })).toBeDisabled();
  });

  // @desabilitar-botao (email inválido)
  test('Entrar fica desabilitado quando email é inválido', async ({ page }) => {
    await page.getByLabel('E-mail Corporativo').fill('ana.example.com');
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    await expect(page.getByRole('button', { name: 'Entrar na conta' })).toBeDisabled();
  });

  // @submit @sucesso
  test('login com sucesso redireciona para /', async ({ page }) => {
    await mockLogin(page, { kind: 'success' });
    await mockPostLogin(page, [
      { id: 'c1', type: 'company', company: { id: 'c1', name: 'Imo X', tax_id: '123', email: VALID_EMAIL, type: 'imobiliaria', is_active: true, members: [{ account_id: 'fake-user-id', profile: 'owner' }] } },
    ]);
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    await page.getByRole('button', { name: 'Entrar na conta' }).click();
    await page.waitForURL((url) => url.pathname === '/');
  });

  // @submit @loading
  test('Entrar exibe spinner durante a requisição', async ({ page }) => {
    await mockLogin(page, { kind: 'delay', ms: 800 });
    await mockPostLogin(page);
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    const submit = page.getByRole('button', { name: 'Entrar na conta' });
    await submit.click();
    await expect(submit).toBeDisabled();
    await expect(submit.locator('.animate-spin')).toBeVisible();
  });

  // @submit @erro @credenciais
  test('credenciais inválidas (401) mostram "E-mail ou senha incorretos."', async ({ page }) => {
    await mockLogin(page, { kind: 'error', status: 401, message: 'Invalid login credentials' });
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill('senhaerrada');
    await page.getByRole('button', { name: 'Entrar na conta' }).click();
    await expect(page.getByText('E-mail ou senha incorretos.')).toBeVisible();
  });

  // @submit @erro @servidor
  test('erro 500 mostra mensagem amigável de servidor', async ({ page }) => {
    await mockLogin(page, { kind: 'error', status: 500, message: 'Ocorreu um erro interno.' });
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    await page.getByRole('button', { name: 'Entrar na conta' }).click();
    await expect(
      page.getByText('Não conseguimos te autenticar no momento. Tente novamente em alguns instantes.')
    ).toBeVisible();
    await expect(page.getByText('Ocorreu um erro interno.')).toBeHidden();
  });

  // @submit @erro @rede
  test('falha de rede mostra mensagem amigável de conexão', async ({ page }) => {
    await mockLogin(page, { kind: 'network-failure' });
    await page.getByLabel('E-mail Corporativo').fill(VALID_EMAIL);
    await page.getByLabel('Senha', { exact: true }).fill(VALID_PASSWORD);
    await page.getByRole('button', { name: 'Entrar na conta' }).click();
    await expect(
      page.getByText('Sem conexão com o servidor. Verifique sua internet e tente novamente.')
    ).toBeVisible();
  });

  // @navegacao @esqueci_senha
  test('Esqueci minha senha abre o formulário de recuperação', async ({ page }) => {
    await page.getByRole('button', { name: 'Esqueci minha senha' }).click();
    await expect(page.getByRole('heading', { name: 'Recuperar Senha' })).toBeVisible();
  });

  // @navegacao @cadastrar
  test('Cadastre-se abre o wizard de cadastro', async ({ page }) => {
    await page.getByRole('button', { name: 'Cadastre-se' }).click();
    await expect(page.getByRole('heading', { name: 'Seus dados de contato' })).toBeVisible();
  });
});

// White label do painel esquerdo (desktop). A tela de login é
// PRÉ-AUTENTICAÇÃO: não há chamada ao servidor para descobrir a imobiliária, o
// tema vem da última sessão gravada em localStorage pelo ThemeContext — o mesmo
// caminho das cores. A imobiliária edita isso em Configurações > Aparência
// (`settings:manage_appearance` = admin e marketing).
test.describe('Login - white label da ultima sessao', () => {
  // 1x1 transparente: uma URL de imagem própria sem sair para a rede. Uma URL
  // externa real viraria CONNECT no servidor de contenção e faria o teardown do
  // gate falhar — que é justamente o que estes testes querem evitar.
  const CUSTOM_BANNER = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';
  const THEME_KEY = 'suitee-theme-settings';

  async function seedTheme(page: Page, theme: Record<string, unknown>) {
    await page.addInitScript(
      ([key, value]) => window.localStorage.setItem(key, JSON.stringify(value)),
      [THEME_KEY, theme] as const,
    );
    await page.goto('/login');
  }

  // @marca @banner
  test('sem sessao anterior usa a imagem padrao da plataforma', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', /equipe/);
  });

  // @marca @banner
  test('preset escolhido pela imobiliaria substitui o padrao', async ({ page }) => {
    await seedTheme(page, { loginBannerPreset: 'cliente' });
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', /cliente/);
  });

  // @marca @banner @upload
  test('imagem enviada pela imobiliaria vence o preset', async ({ page }) => {
    await seedTheme(page, {
      loginBannerPreset: 'cliente',
      loginBannerUrl: CUSTOM_BANNER,
    });
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', CUSTOM_BANNER);
  });

  // Troca entre a imagem enviada e um modelo do sistema (Configurações >
  // Aparência). As DUAS ficam gravadas e `loginBannerSource` decide — antes,
  // escolher um preset apagava a imagem enviada.
  // @marca @banner @troca
  test('preset ativo com imagem guardada mostra o preset, nao a imagem', async ({ page }) => {
    await seedTheme(page, {
      loginBannerPreset: 'room',
      loginBannerUrl: CUSTOM_BANNER,
      loginBannerSource: 'preset',
    });
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', /room/);
  });

  // @marca @banner @troca
  test('voltar a fonte para a imagem enviada mostra a imagem de novo', async ({ page }) => {
    await seedTheme(page, {
      loginBannerPreset: 'room',
      loginBannerUrl: CUSTOM_BANNER,
      loginBannerSource: 'custom',
    });
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', CUSTOM_BANNER);
  });

  // Sessao gravada antes do campo existir: a imagem enviada continua vencendo,
  // como sempre foi.
  // @marca @banner @troca
  test('sem a fonte definida mantem a regra antiga', async ({ page }) => {
    await seedTheme(page, {
      loginBannerPreset: 'room',
      loginBannerUrl: CUSTOM_BANNER,
    });
    await expect(page.getByTestId('login-banner')).toHaveAttribute('src', CUSTOM_BANNER);
  });

  // @marca @textos
  test('titulo e subtitulo da imobiliaria aparecem no painel', async ({ page }) => {
    await seedTheme(page, {
      loginTagline: 'Imobiliaria Aurora',
      loginSubtitle: 'O imovel dos seus sonhos esta aqui',
    });
    await expect(page.getByRole('heading', { name: 'Imobiliaria Aurora' })).toBeVisible();
    await expect(page.getByText('O imovel dos seus sonhos esta aqui')).toBeVisible();
  });

  // @marca @textos
  test('sem configuracao mostra os textos padrao da plataforma', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'Sua plataforma imobiliária' })).toBeVisible();
    await expect(
      page.getByText('Gerencie leads, automatize sua esteira digital e feche mais negócios com inteligência de dados.'),
    ).toBeVisible();
  });
});

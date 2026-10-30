import { test, expect, Page, Route } from '@playwright/test';

// Contraste no MODO NOTURNO (WCAG 2.1 AA).
//
// Contexto: no modo noturno o app tinha textos pretos herdados do <body> (1.1:1),
// placeholder cinza-escuro (3.96:1), botão primário branco sobre o verde da marca
// (3.99:1), texto de destaque da marca sobre o card (3.82–4.27:1), a pílula
// "Expandir" cinza sobre cinza (1.74:1) e toasts CLAROS (o Toaster lia o tema do
// next-themes, que não é montado neste projeto, e seguia o tema do sistema
// operacional). Nada disso aparecia no modo claro.
//
// Este teste lê a cor computada de TUDO que tem texto nas telas de Leads e mede a
// razão de contraste contra o fundo efetivo (compondo alphas e ancestrais). É a
// única forma de garantir o requisito: nenhum teste de componente enxerga a cor
// final depois de tema + utilitários + camadas de CSS.

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

async function mockAuthBoot(page: Page) {
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
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { last_company_id: null, consent: { terms_version: '2.0' } } }) }),
  );
  await page.route(/\/api\/v1\/me\/permissions\b.*/, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      // Permissões de escrita das telas cobertas: sem elas os botões "Novo"/
      // "Nova Proposta" nem aparecem e a medição do drawer seria vazia.
      body: JSON.stringify({
        success: true,
        statusCode: 200,
        data: {
          functional_role: 'admin',
          profile: 'owner',
          permissions: ['finance:write', 'deals:write', 'crm:write', 'settings:manage_appearance'],
        },
      }),
    }),
  );
}

async function mockCrm(page: Page) {
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
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: { enabled: false, brokers: [] } }) }),
  );
  await page.route(/\/api\/v1\/brokers\b.*/, (r) =>
    r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, statusCode: 200, data: [ALICE_BROKER] }) }),
  );
}

// Catch-all para qualquer outro endpoint. Precisa devolver `fallback()` nos
// endpoints já mockados: o Playwright avalia as rotas em LIFO, então esta (a
// última registrada) seria chamada primeiro e venceria os mocks específicos.
async function mockCatchAll(page: Page) {
  await page.route(/\/api\/v1\/.*/, async (route) => {
    const path = new URL(route.request().url()).pathname;
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
      // `meta` entra junto porque os paginadores do app recusam uma listagem sem
      // total/limite (o `collectPages` lança) — sem ele as telas caem em estado
      // de erro e a medição de contraste mediria a tela de erro.
      body: JSON.stringify({ success: true, statusCode: 200, data: [], total: 0, page: 1, limit: 100, meta: { total: 0, page: 1, limit: 100 } }),
    });
  });
}

// Medição no browser: resolve qualquer cor (oklch/hsl/rgba) para sRGB via canvas,
// compõe o fundo efetivo subindo os ancestrais e calcula a razão WCAG.
const MEASURE = () => {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const ctx = canvas.getContext('2d')!;
  const cache = new Map<string, { r: number; g: number; b: number; a: number }>();

  const parse = (css: string) => {
    const cached = cache.get(css);
    if (cached) return cached;
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#000';
    ctx.fillStyle = css;
    ctx.fillRect(0, 0, 1, 1);
    const data = ctx.getImageData(0, 0, 1, 1).data;
    const value = { r: data[0], g: data[1], b: data[2], a: data[3] / 255 };
    cache.set(css, value);
    return value;
  };

  const over = (fg: { r: number; g: number; b: number; a: number }, bg: { r: number; g: number; b: number; a: number }) => {
    const a = fg.a + bg.a * (1 - fg.a);
    if (!a) return { r: 0, g: 0, b: 0, a: 0 };
    return {
      r: (fg.r * fg.a + bg.r * bg.a * (1 - fg.a)) / a,
      g: (fg.g * fg.a + bg.g * bg.a * (1 - fg.a)) / a,
      b: (fg.b * fg.a + bg.b * bg.a * (1 - fg.a)) / a,
      a,
    };
  };

  const effectiveBackground = (el: Element) => {
    const layers: { r: number; g: number; b: number; a: number }[] = [];
    let node: Element | null = el;
    while (node) {
      const background = parse(getComputedStyle(node).backgroundColor);
      if (background.a > 0) layers.push(background);
      if (background.a === 1) break;
      node = node.parentElement;
    }
    layers.push({ r: 255, g: 255, b: 255, a: 1 });
    let acc = layers[layers.length - 1];
    for (let i = layers.length - 2; i >= 0; i -= 1) acc = over(layers[i], acc);
    return acc;
  };

  const luminance = (color: { r: number; g: number; b: number }) => {
    const channel = (value: number) => {
      const s = value / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * channel(color.r) + 0.7152 * channel(color.g) + 0.0722 * channel(color.b);
  };

  const ratio = (a: { r: number; g: number; b: number }, b: { r: number; g: number; b: number }) => {
    const la = luminance(a);
    const lb = luminance(b);
    return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
  };

  const results: {
    kind: string;
    text: string;
    ratio: number;
    need: number;
    fg: string;
    bg: string;
    cls: string;
  }[] = [];

  const consider = (kind: string, el: Element, pseudo?: string) => {
    const rect = (el as HTMLElement).getBoundingClientRect();
    // Fora da tela, escondido por clip (sr-only) ou em animação de entrada.
    if (rect.width < 2 || rect.height < 2) return;
    if (!(el as HTMLElement).offsetParent && getComputedStyle(el).position !== 'fixed') return;
    // Controles inativos são isentos de AA (WCAG 1.4.3) e costumam usar opacity-50.
    if (el.closest('[disabled], [aria-disabled="true"], [data-disabled]')) return;
    if (el.closest('[aria-hidden="true"]')) return;
    // Overlays de fundo (scrim) não carregam texto legível.
    if (el.getAttribute('aria-hidden') === 'true') return;

    const style = getComputedStyle(el, pseudo as never);
    if (style.visibility === 'hidden' || Number(style.opacity) === 0) return;

    const fg = parse(style.color);
    if (fg.a === 0) return;

    const bg = effectiveBackground(el);
    const size = parseFloat(style.fontSize);
    const weight = Number(style.fontWeight) || 400;
    // WCAG: texto grande (>= 24px, ou >= 18.66px em negrito) exige 3:1.
    const need = size >= 24 || (size >= 18.66 && weight >= 700) ? 3 : 4.5;

    results.push({
      kind,
      text: ((el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 40) || el.getAttribute('aria-label') || el.tagName.toLowerCase()),
      ratio: Math.round(ratio(over(fg, bg), bg) * 100) / 100,
      need,
      fg: style.color,
      bg: `rgb(${Math.round(bg.r)},${Math.round(bg.g)},${Math.round(bg.b)})`,
      cls: (el.className || '').toString().slice(0, 80),
    });
  };

  document.querySelectorAll('button, a[role="button"]').forEach((el) => consider('button', el));
  document.querySelectorAll('input:not([type=hidden]), textarea').forEach((el) => {
    consider('input', el);
    consider('placeholder', el, '::placeholder');
  });
  document.querySelectorAll('select, option').forEach((el) => consider('select', el));
  document.querySelectorAll('[data-sonner-toast] *').forEach((el) => consider('toast', el));
  // Todo elemento com texto PRÓPRIO (filho de texto direto), não só as tags
  // usuais: `<strong>`, `<td>`, `<li>`, `<option>`, `<code>` também carregam
  // texto e ficavam fora da medição — foi o caso do aviso "Repasses não são
  // lançados manualmente." dentro de um `<strong>`.
  document.querySelectorAll('*').forEach((el) => {
    const hasOwnText = Array.from(el.childNodes).some(
      (node) => node.nodeType === 3 && (node.textContent ?? '').trim(),
    );
    if (hasOwnText) consider('text', el);
  });
  return results;
};

type Measurement = Awaited<ReturnType<typeof MEASURE>>;

function failures(rows: Measurement) {
  return rows
    .filter((row) => row.ratio < row.need)
    .map((row) => `${row.kind} "${row.text}" ${row.ratio}:1 (min ${row.need}) fg=${row.fg} bg=${row.bg} cls=${row.cls}`);
}

// Guarda contra medição vazia: além do mínimo de elementos, exige que os tipos
// que interessam (botão, campo, placeholder, select, texto) tenham sido vistos —
// senão um seletor quebrado faria o teste "passar" sem medir nada.
function expectNoContrastFailure(rows: Measurement, requiredKinds = ['button', 'input', 'placeholder', 'select', 'text']) {
  const kinds = new Set(rows.map((row) => row.kind));
  for (const kind of requiredKinds) {
    expect([...kinds], `medição sem elementos do tipo "${kind}"`).toContain(kind);
  }
  expect(rows.length).toBeGreaterThan(10);
  expect(failures(rows)).toEqual([]);
}

/** Abre uma tela autenticada já no modo noturno, com os mocks de sessão. */
async function gotoDark(page: Page, path: string, theme: Record<string, unknown> = { theme: 'dark' }) {
  // Tema pela mesma chave que o ThemeContext persiste no localStorage.
  await page.addInitScript((settings) => {
    window.localStorage.setItem('suitee-theme-settings', JSON.stringify(settings));
  }, theme);
  await mockAuthBoot(page);
  await mockCrm(page);
  await mockCatchAll(page);
  await page.goto(path);
  await expect(page.locator('html')).toHaveClass(/dark/);
}

async function gotoLeadsDark(page: Page, theme: Record<string, unknown> = { theme: 'dark' }) {
  await gotoDark(page, '/leads', theme);
  await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();
}

test.describe('Modo noturno — contraste (WCAG AA)', () => {
  test('lista de Leads: nenhum texto abaixo de AA', async ({ page }) => {
    await gotoLeadsDark(page);
    await expect(page.locator('html')).toHaveClass(/dark/);

    expectNoContrastFailure(await page.evaluate(MEASURE));
  });

  test('painel do Novo Lead: campos, botões e selects em AA', async ({ page }) => {
    await gotoLeadsDark(page);
    await page.getByRole('main').getByRole('button', { name: 'Novo Lead', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Novo Lead', exact: true })).toBeVisible();
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
    // Espera a animação de entrada do painel: durante ela as cores ainda interpolam.
    await page.waitForTimeout(500);

    expectNoContrastFailure(await page.evaluate(MEASURE));
  });

  test('dropdown do corretor: itens em AA', async ({ page }) => {
    await gotoLeadsDark(page);
    await page.getByRole('main').getByRole('button', { name: 'Novo Lead', exact: true }).click();
    await page.getByRole('button', { name: /Captação de imóvel/ }).click();
    await page.waitForTimeout(300);
    await page.locator('button[role="combobox"]').last().click();
    await page.waitForTimeout(400);

    expectNoContrastFailure(await page.evaluate(MEASURE));
  });

  test('tema claro não regride (a correção é do noturno)', async ({ page }) => {
    await mockAuthBoot(page);
    await mockCrm(page);
    await mockCatchAll(page);
    await page.goto('/leads');
    await expect(page.getByRole('heading', { name: 'Gestão de Leads' })).toBeVisible();

    const rows = await page.evaluate(MEASURE);
    expect(rows.length).toBeGreaterThan(25);
    // No tema claro o texto do <body> já era a cor do tema; aqui a rede é só
    // contra regressão óbvia (o objetivo do teste é o modo noturno).
    expect(failures(rows).filter((line) => /1\.\d\d?:1/.test(line))).toEqual([]);
  });

  // A cor de marca muda o par `--primary`/`--primary-text`. A marca padrão da
  // plataforma e a marca ESCURA (a que a imobiliária realmente usa) exercitam
  // caminhos opostos da escolha por contraste: a escura serve de fundo como está
  // e precisa de um tom mais claro para TEXTO; a padrão precisa de um tom mais
  // escuro para o texto branco do botão.
  test('marca escura da imobiliária: texto de destaque e botão em AA', async ({ page }) => {
    await gotoLeadsDark(page, { theme: 'dark', primaryColor: '#013529' });
    await expect(page.locator('html')).toHaveClass(/dark/);
    // Confirma que a marca escura chegou ao documento (senão o teste mediria o
    // verde padrão e passaria por engano).
    const primaryText = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--primary-text').trim(),
    );
    expect(primaryText).not.toBe('');

    expectNoContrastFailure(await page.evaluate(MEASURE));
  });

  // Drawers abertos por cima de cada tela. Estes painéis usavam uma superfície
  // "invertida" (90% da cor do TEXTO como fundo) e ficavam CLAROS no modo
  // noturno, com rótulo claro por cima: os prints de "Novo compromisso" e "Nova
  // proposta" mostravam exatamente isso. Agora são superfície do tema.
  test('drawer de Novo compromisso (Agenda) em AA', async ({ page }) => {
    await gotoDark(page, '/agenda');
    await page.getByRole('button', { name: 'Novo', exact: true }).first().click();
    await page.waitForTimeout(600);

    const rows = await page.evaluate(MEASURE);
    // O painel de verdade abriu? (guarda contra medir a tela de trás.)
    expect(rows.some((row) => /Visita ao imóvel|lead\/cliente/i.test(row.text))).toBe(true);
    expectNoContrastFailure(rows);
  });

  test('drawer de Nova proposta (Esteira) em AA', async ({ page }) => {
    await gotoDark(page, '/esteira');
    await page.getByRole('button', { name: /Nova Proposta/i }).first().click();
    await page.waitForTimeout(600);

    const rows = await page.evaluate(MEASURE);
    expect(rows.some((row) => /Nova proposta|Selecione um cliente/i.test(row.text))).toBe(true);
    expectNoContrastFailure(rows);
  });

  test('drawer de Novo Lançamento (Financeiro) em AA', async ({ page }) => {
    await gotoDark(page, '/financeiro/lancamentos');
    await page.getByRole('button', { name: /^Novo$/ }).first().click();
    await page.waitForTimeout(600);

    const rows = await page.evaluate(MEASURE);
    // O aviso "Repasses não são lançados manualmente" era um chip claro com
    // texto do tema; aqui ele precisa continuar legível.
    expect(rows.some((row) => row.text.includes('Repasses'))).toBe(true);
    expectNoContrastFailure(rows, ['button', 'text']);
  });
});

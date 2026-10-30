import { test, expect, type Page } from '@playwright/test';

// Painel "Alterar horário da visita" (portal do cliente).
//
// Defeitos relatados no app real:
//   1. as setas de navegação de mês ficavam desalinhadas: no layout padrão do
//      react-day-picker a <nav> é IRMÃ do mês e os botões usam `absolute
//      left-1/right-1`. Sem ancestral posicionado eles se ancoravam no diálogo
//      inteiro — medidos 111px abaixo da linha do mês, nas bordas do modal;
//   2. o calendário podia exibir um mês sem relação com a visita (visto
//      "julho 2028" com a visita em setembro): `defaultMonth` só vale na
//      montagem e a data chega depois;
//   3. contraste baixo no componente de horário: os chips não selecionados e os
//      campos de início/término tinham borda #e5e7eb sobre o card = 1,13:1
//      (WCAG 1.4.11 pede 3:1 para o limite de um controle).
//
// Tudo é medido no navegador: nenhum teste de componente enxerga a cor final
// depois de tema + utilitários + camadas de CSS, nem a posição real de um
// botão `absolute`.

const user = {
  id: 'fake-user-id', email: 'qa@example.test', firstname: 'Cliente', lastname: 'QA',
  role: 'user', functional_role: 'admin', account_type: 'individual',
  permission: 'owner', current_company_id: 'company-1',
};
const company = { id: 'company-1', type: 'company', company: {
  id: 'company-1', name: 'Imobiliária QA', type: 'imobiliaria', is_active: true,
  owner_id: user.id, members: [{ account_id: user.id, profile: 'owner' }],
} };
// Último dia do mês: além de fixar o mês esperado (se o calendário abrir em
// outro, o teste falha), permite checar que "Amanhã" atravessa para o mês
// seguinte levando a view junto.
const VISIT_DATE = '2099-10-31';

function overviewLead(date: string) {
  return {
    id: 'lead-qa', name: 'Cliente QA', has_preferences: false,
    recommended_property_id: 'property-qa', recommended_property: {
      id: 'property-qa', title: 'Apartamento de teste', city: 'São Paulo', sale_price: 400000,
    }, matched_properties: [],
    visit_request: {
      date, time: '14:00', end_time: '15:00',
      property_id: 'property-qa', status: 'Confirmado',
    },
  };
}

async function seed(page: Page, theme?: Record<string, unknown>, visitDate = VISIT_DATE) {
  if (theme) {
    await page.addInitScript((settings) => {
      window.localStorage.setItem('suitee-theme-settings', JSON.stringify(settings));
    }, theme);
  }
  const lead = overviewLead(visitDate);
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || path === '/auth/portal/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'synthetic-qa-token' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [company];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/client-portal/overview') data = [lead];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|portal\/refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    await route.fulfill({ json: { success: true, statusCode: 200, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
  });
}

async function openRescheduleDialog(page: Page, options: { theme?: Record<string, unknown>; visitDate?: string } = {}) {
  await seed(page, options.theme, options.visitDate);
  await page.goto('/portal');
  const opener = page.getByRole('button', { name: 'Alterar horário', exact: true });
  await expect(opener).toBeVisible();
  await opener.click();
  const dialog = page.getByRole('dialog', { name: 'Alterar horário da visita' });
  await expect(dialog).toBeVisible();
  // Espera a animação de entrada: durante ela as cores ainda interpolam.
  await page.waitForTimeout(400);
  await dialog.evaluate((el) => el.setAttribute('data-appointment-dialog', ''));
  return dialog;
}

// ── Medição no navegador ────────────────────────────────────────────────────
//
// Devolve duas listas: contraste de TEXTO (WCAG 1.4.3, 4.5:1 ou 3:1 para texto
// grande) e de LIMITE DE CONTROLE (WCAG 1.4.11, 3:1) — a segunda só para os
// controles do componente de horário, que é o alvo do relato.
const MEASURE = () => {
  const root = document.querySelector('[data-appointment-dialog]');
  if (!root) return null;

  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const ctx = canvas.getContext('2d')!;
  const cache = new Map<string, { r: number; g: number; b: number; a: number }>();
  const parse = (css: string) => {
    const hit = cache.get(css);
    if (hit) return hit;
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#000';
    ctx.fillStyle = css;
    ctx.fillRect(0, 0, 1, 1);
    const d = ctx.getImageData(0, 0, 1, 1).data;
    const value = { r: d[0], g: d[1], b: d[2], a: d[3] / 255 };
    cache.set(css, value);
    return value;
  };
  const over = (fg: any, bg: any) => {
    const a = fg.a + bg.a * (1 - fg.a);
    if (!a) return { r: 0, g: 0, b: 0, a: 0 };
    return {
      r: (fg.r * fg.a + bg.r * bg.a * (1 - fg.a)) / a,
      g: (fg.g * fg.a + bg.g * bg.a * (1 - fg.a)) / a,
      b: (fg.b * fg.a + bg.b * bg.a * (1 - fg.a)) / a,
      a,
    };
  };
  const effectiveBackground = (el: Element | null) => {
    const layers: any[] = [];
    let node: Element | null = el;
    while (node) {
      const bg = parse(getComputedStyle(node).backgroundColor);
      if (bg.a > 0) layers.push(bg);
      if (bg.a === 1) break;
      node = node.parentElement;
    }
    layers.push({ r: 255, g: 255, b: 255, a: 1 });
    let acc = layers[layers.length - 1];
    for (let i = layers.length - 2; i >= 0; i -= 1) acc = over(layers[i], acc);
    return acc;
  };
  const luminance = (color: any) => {
    const channel = (value: number) => {
      const s = value / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * channel(color.r) + 0.7152 * channel(color.g) + 0.0722 * channel(color.b);
  };
  const ratio = (a: any, b: any) => {
    const la = luminance(a);
    const lb = luminance(b);
    return Math.round(((Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)) * 100) / 100;
  };
  const visible = (el: Element) => {
    const rect = (el as HTMLElement).getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) return false;
    if (el.closest('[aria-hidden="true"]')) return false;
    const style = getComputedStyle(el);
    return style.visibility !== 'hidden' && Number(style.opacity) !== 0;
  };

  const text: any[] = [];
  const boundaries: any[] = [];

  const label = (el: Element) =>
    ((el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 32)
      || el.getAttribute('aria-label') || (el as HTMLInputElement).value || el.tagName.toLowerCase());

  // Texto: qualquer elemento com texto próprio dentro do diálogo.
  root.querySelectorAll('*').forEach((el) => {
    const hasOwnText = Array.from(el.childNodes).some((n) => n.nodeType === 3 && (n.textContent ?? '').trim());
    if (!hasOwnText || !visible(el)) return;
    if (el.closest('[disabled], [aria-disabled="true"], [data-disabled]')) return;
    const style = getComputedStyle(el);
    const fg = parse(style.color);
    if (!fg.a) return;
    const bg = effectiveBackground(el);
    const size = parseFloat(style.fontSize);
    const weight = Number(style.fontWeight) || 400;
    const need = size >= 24 || (size >= 18.66 && weight >= 700) ? 3 : 4.5;
    text.push({
      text: label(el),
      ratio: ratio(over(fg, bg), bg),
      need,
      fg: style.color,
      bg: `rgb(${Math.round(bg.r)},${Math.round(bg.g)},${Math.round(bg.b)})`,
    });
  });

  // Limites dos controles do componente de horário (chips + campos de hora).
  const timeControls = Array.from(root.querySelectorAll('button[aria-pressed], input[type="time"]'));
  timeControls.forEach((el) => {
    if (!visible(el)) return;
    const style = getComputedStyle(el);
    const parentBg = effectiveBackground(el.parentElement);
    const borderWidth = parseFloat(style.borderTopWidth) || 0;
    const borderColor = parse(style.borderTopColor);
    const fill = parse(style.backgroundColor);
    const boundary = borderWidth > 0 && borderColor.a > 0 ? over(borderColor, parentBg) : over(fill, parentBg);
    boundaries.push({
      text: label(el),
      kind: el.tagName.toLowerCase(),
      borderWidth,
      boundary: `rgb(${Math.round(boundary.r)},${Math.round(boundary.g)},${Math.round(boundary.b)})`,
      ratio: ratio(boundary, parentBg),
    });
  });

  return { text, boundaries };
};

type Measurement = NonNullable<Awaited<ReturnType<typeof MEASURE>>>;

function textFailures(rows: Measurement) {
  return rows.text
    .filter((row) => row.ratio < row.need)
    .map((row) => `texto "${row.text}" ${row.ratio}:1 (min ${row.need}) fg=${row.fg} bg=${row.bg}`);
}

function boundaryFailures(rows: Measurement) {
  return rows.boundaries
    .filter((row) => row.ratio < 3)
    .map((row) => `limite de "${row.text}" (${row.kind}) ${row.ratio}:1 (min 3) border=${row.boundary}`);
}

const MONTHS = [
  'janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho',
  'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro',
];

/** ISO local (sem deslocar por fuso, ao contrário de toISOString). */
function localISO(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

/** Legenda do mês exibido, no formato do date-fns pt-BR ("outubro 2099"). */
function captionFor(date: Date) {
  return new RegExp(`${MONTHS[date.getMonth()]} ${date.getFullYear()}`, 'i');
}

test.describe('portal: painel de alterar horário da visita', () => {
  test('abre no mês da visita e acompanha a data escolhida', async ({ page }) => {
    const dialog = await openRescheduleDialog(page);
    const caption = dialog.getByRole('status');
    const selectedDay = dialog.locator('[data-selected="true"]');

    // Abre no mês da VISITA — não no mês corrente nem em outro qualquer.
    await expect(caption).toHaveText(/outubro 2099/i);
    await expect(selectedDay).toHaveAttribute('data-day', VISIT_DATE);

    // "Amanhã" parte do dia real de hoje: a view do calendário precisa ir para o
    // mês da data escolhida (antes o calendário ficava num mês sem relação).
    const tomorrow = new Date();
    tomorrow.setDate(tomorrow.getDate() + 1);
    await dialog.getByRole('button', { name: 'Amanhã', exact: true }).click();
    await expect(caption).toHaveText(captionFor(tomorrow));
    await expect(selectedDay).toHaveAttribute('data-day', localISO(tomorrow));

    // "Hoje" volta para o dia corrente, com a mesma coerência.
    const today = new Date();
    await dialog.getByRole('button', { name: 'Hoje', exact: true }).click();
    await expect(caption).toHaveText(captionFor(today));
    await expect(selectedDay).toHaveAttribute('data-day', localISO(today));
  });

  test('as setas de mês ficam na linha da legenda, nas pontas do calendário', async ({ page }) => {
    const dialog = await openRescheduleDialog(page);
    const caption = dialog.getByRole('status');
    const previous = dialog.locator('button:has(.lucide-chevron-left)');
    const next = dialog.locator('button:has(.lucide-chevron-right)');
    const card = dialog.locator('div.rounded-2xl.border').first();

    const [captionBox, previousBox, nextBox, cardBox] = await Promise.all([
      caption.boundingBox(), previous.boundingBox(), next.boundingBox(), card.boundingBox(),
    ]);
    expect(captionBox && previousBox && nextBox && cardBox).toBeTruthy();

    const centerY = (box: { y: number; height: number }) => box.y + box.height / 2;
    expect(
      Math.abs(centerY(previousBox!) - centerY(captionBox!)),
      `seta anterior fora da linha do mês (${JSON.stringify(previousBox)} vs ${JSON.stringify(captionBox)})`,
    ).toBeLessThanOrEqual(2);
    expect(
      Math.abs(centerY(nextBox!) - centerY(captionBox!)),
      `próxima seta fora da linha do mês (${JSON.stringify(nextBox)} vs ${JSON.stringify(captionBox)})`,
    ).toBeLessThanOrEqual(2);

    // À esquerda e à direita da legenda, dentro do cartão do calendário.
    expect(previousBox!.x + previousBox!.width).toBeLessThanOrEqual(captionBox!.x + captionBox!.width / 2);
    expect(nextBox!.x).toBeGreaterThanOrEqual(captionBox!.x + captionBox!.width / 2);
    expect(previousBox!.x).toBeGreaterThanOrEqual(cardBox!.x);
    expect(nextBox!.x + nextBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width + 1);

    // Navegar continua funcionando pelas setas.
    await next.click();
    await expect(dialog.getByRole('status')).toHaveText(/novembro 2099/i);
  });

  for (const theme of [
    { name: 'claro', settings: undefined },
    { name: 'noturno', settings: { theme: 'dark' } },
  ]) {
    test(`contraste do painel no tema ${theme.name}`, async ({ page }) => {
      const dialog = await openRescheduleDialog(page, { theme: theme.settings });
      if (theme.settings) {
        await expect(page.locator('html')).toHaveClass(/dark/);
      }

      const rows = await page.evaluate(MEASURE);
      expect(rows).not.toBeNull();
      expect(textFailures(rows!), 'texto abaixo de WCAG AA no painel').toEqual([]);
      expect(boundaryFailures(rows!), 'limite de controle abaixo de 3:1 no componente de horário').toEqual([]);

      // Guarda contra medição vazia.
      expect(rows!.text.length).toBeGreaterThan(12);
      expect(rows!.boundaries.length).toBeGreaterThanOrEqual(6);
      await dialog.screenshot({ path: `/private/tmp/portal-appointment-${theme.name}.png` });
    });
  }
});

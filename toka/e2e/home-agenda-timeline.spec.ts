import { test, expect, type Page } from '@playwright/test';

// Home no celular: a linha do tempo da "Agenda do Dia" (marcadores + rail) e o
// cabeçalho de saudação da Home são `z-10`. Como os marcadores vêm DEPOIS no
// DOM, eles pintavam por cima do cabeçalho sticky quando a página rolava — o
// marcador do stepper aparecia solto sobre "Atender Novo Lead" (relato com
// print no app real).
//
// A medição é feita no navegador: a ordem de pintura só existe depois de
// layout + stacking contexts, nada disso é visível para um teste de componente.

const TEST_USER = {
  id: 'fake-user-id', email: 'maria@example.com', firstname: 'Caua', lastname: 'Teste',
  role: 'user', functional_role: 'admin', account_type: 'individual',
  permission: 'owner', current_company_id: 'mci-company-id',
};
const TEST_COMPANY = {
  id: 'mci-company-id', type: 'company', company: {
    id: 'mci-company-id', name: 'Exemplo Imóveis', legal_name: 'Exemplo Imóveis LTDA',
    tax_id: '11222333000181', email: 'contato@Exemplo.com.br', phone: '11999998888',
    type: 'imobiliaria', is_active: true, owner_id: TEST_USER.id,
    members: [{ account_id: TEST_USER.id, profile: 'owner' }],
  },
};

function localISODate(offsetDays = 0) {
  const date = new Date();
  date.setDate(date.getDate() + offsetDays);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

// Cinco compromissos de hoje: conteúdo suficiente para a Home rolar no celular.
function appointmentsFixture() {
  const today = localISODate();
  const titles = ['Visita Avaliacao', 'Visita Diego', 'Ligação proposta', 'Vistoria imóvel', 'Reunião de fechamento'];
  return titles.map((title, index) => ({
    id: `apt-${index}`,
    title,
    client: ['Eduardo Fernandes', 'Diego Santos', 'Marina Alves', 'Paulo Reis', 'Ana Paula'][index],
    date: today,
    time: `${String(8 + index).padStart(2, '0')}:00`,
    end_time: `${String(9 + index).padStart(2, '0')}:00`,
    status: index === 1 ? 'Concluido' : 'Confirmado',
    type: index % 2 === 0 ? 'Visita' : 'Ligacao',
    priority: 'Media',
    agent: 'Caua',
  }));
}

async function seedHome(page: Page) {
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'fake' };
    if (path === '/auth/me') data = TEST_USER;
    if (path === '/auth/companies') data = [TEST_COMPANY];
    if (path === '/me/permissions') data = { functional_role: 'admin', profile: 'owner', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/appointments') data = appointmentsFixture();
    if (path === '/leads/stats') data = { total: 0, conversion_rate: 0 };
    if (path === '/deals/stats') data = { monthly_sales: 0 };
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    await route.fulfill({ json: { success: true, statusCode: 200, data, meta: { total: Array.isArray(data) ? data.length : 1 } } });
  });
}

// Rola o contêiner real da Home (o Dashboard tem o próprio `overflow-auto`
// dentro de `main.overflow-auto`) e devolve quem está pintado no centro de cada
// marcador. Se o cabeçalho sticky cobrir o marcador, o topo do hit-test tem de
// ser o cabeçalho — nunca o próprio marcador.
const MEASURE = (scrollTop: number) => {
  // O scroller é o ancestral rolável REAL dos marcadores: a Home tem
  // `overflow-auto` dentro de `main.overflow-auto`, e a navegação lateral
  // também rola — pegar "o primeiro overflow-auto" rolava a barra errada.
  const anchor = document.querySelector('div.w-5.h-5.rounded-full.border-2');
  let scroller: HTMLElement | null = null;
  let node: HTMLElement | null = anchor?.parentElement ?? null;
  while (node) {
    const style = getComputedStyle(node);
    if (/(auto|scroll)/.test(style.overflowY) && node.scrollHeight > node.clientHeight + 40) {
      scroller = node;
      break;
    }
    node = node.parentElement;
  }
  if (scroller) scroller.scrollTop = scrollTop;

  const heading = document.querySelector('h1');
  const headerBlock = (heading?.closest('[class*="sticky"]') as HTMLElement | null) ?? null;
  const agendaHeading = Array.from(document.querySelectorAll('h3')).find((el) => /Agenda do Dia/.test(el.textContent ?? ''));
  const agendaCard = agendaHeading?.closest('div[class*="bg-card"]') as HTMLElement | null;
  const markers = Array.from(document.querySelectorAll('div.w-5.h-5.rounded-full.border-2')) as HTMLElement[];
  const cards = Array.from(document.querySelectorAll('div.flex-1.min-w-0.overflow-hidden')) as HTMLElement[];

  const box = (el: Element | null) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const headerBox = box(headerBlock);
  const intersects = (a: any, b: any) => !!a && !!b && a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;

  return {
    scrollTop: Math.round(scroller?.scrollTop ?? 0),
    scrollerFound: !!scroller,
    header: headerBox,
    agendaCard: box(agendaCard),
    markers: markers.map((marker) => {
      const r = marker.getBoundingClientRect();
      const cx = Math.round(r.x + r.width / 2);
      const cy = Math.round(r.y + r.height / 2);
      const hit = cy >= 0 && cy <= window.innerHeight ? document.elementFromPoint(cx, cy) : null;
      const markerBox = box(marker);
      return {
        box: markerBox,
        headerIntersects: intersects(markerBox, headerBox),
        // true = o marcador está por CIMA (o hit-test devolve ele mesmo).
        paintsAboveHeader: !!hit && (hit === marker || marker.contains(hit)),
        hitTag: hit ? `${hit.tagName.toLowerCase()}${hit.className ? '.' + hit.className.toString().split(' ').slice(0, 2).join('.') : ''}` : null,
      };
    }),
    cards: cards.map((card) => ({ box: box(card), headerIntersects: intersects(box(card), headerBox) })),
    viewport: { w: window.innerWidth, h: window.innerHeight },
  };
};

test.describe('Home no celular: linha do tempo da Agenda do Dia', () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test('os marcadores não pintam por cima do cabeçalho ao rolar', async ({ page }) => {
    await seedHome(page);
    await page.goto('/');
    await expect(page.getByRole('heading', { name: `Olá, ${TEST_USER.firstname}` })).toBeVisible();
    await expect(page.getByText('Visita Avaliacao')).toBeVisible();
    await expect(page.locator('div.w-5.h-5.rounded-full.border-2')).toHaveCount(5);

    let exercised = false;
    for (let offset = 0; offset <= 900; offset += 60) {
      const state = await page.evaluate(MEASURE, offset);
      const overlapping = state.markers.filter((marker) => marker.headerIntersects);
      if (overlapping.length === 0) continue;
      exercised = true;
      // Evidência do ponto exato da sobreposição (o cabeçalho tem de cobrir o
      // marcador, e não o contrário).
      await page.screenshot({ path: '/private/tmp/home-agenda-overlap.png' });
      expect(
        overlapping.filter((marker) => marker.paintsAboveHeader).map((marker) => ({ box: marker.box, hit: marker.hitTag })),
        `marcador da timeline pintado por cima do cabeçalho sticky (scrollTop=${state.scrollTop})`,
      ).toEqual([]);
    }

    expect(exercised, 'nenhum marcador passou sob o cabeçalho — o teste não exercitou a sobreposição').toBe(true);
  });
});

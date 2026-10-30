import { test, expect, type Page } from '@playwright/test';

// Regressão do portal do cliente: quando o cliente TEM negócio na esteira, a
// home tem de mostrar o ACOMPANHAMENTO do negócio (e o link para a tela do
// negócio). Era o que se perdia com a lista de negócios vazia: o backend lia
// `deal.deals` no pool de aplicação e a RLS cross-tenant zerava o resultado, o
// `activeDeal` nunca era definido e a home caía na jornada pré-negócio — sem
// propostas, sem documentos e sem os fluxos de aceitar proposta/enviar
// documento.
//
// O invariante é do frontend: com `GET /client-portal/deals` respondendo um
// negócio, a home NÃO pode mostrar a jornada pré-negócio e precisa oferecer o
// caminho para `/portal/deals/:id`.

const user = {
  id: 'fake-user-id', email: 'cliente@example.test', firstname: 'Diego', lastname: 'Santos',
  role: 'user', functional_role: 'cliente', account_type: 'individual',
  permission: 'owner', current_company_id: 'company-1',
};

const DEAL = {
  id: 'ea105a98-38ee-4877-8eed-ef84b9312b0d',
  display_id: 'NEG-0001',
  type: 'SALE',
  status: 'PROPOSAL',
  property_id: 'prop-casa',
  owner_id: 'lead-diego',
  customer_id: 'lead-diego',
  broker_id: 'broker-1',
  value: 1400000,
  commission: 70000,
  tenant_id: 'company-1',
  created_at: '2026-09-30T10:00:00Z',
  updated_at: '2026-09-30T10:00:00Z',
  my_party_id: 'party-buyer',
  my_role: 'buyer',
  parties: [],
  proposals: [{ id: 'proposal-1', status: 'SENT', value: 1400000, created_at: '2026-09-30T10:00:00Z' }],
  checklist: [],
};

async function seedPortal(page: Page, deals: unknown) {
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || path === '/auth/portal/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'synthetic' };
    if (path === '/auth/me') data = user;
    if (path === '/auth/companies') data = [];
    if (path === '/me/permissions') data = { functional_role: 'cliente', profile: 'cliente', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/client-portal/deals') data = deals;
    if (path === `/client-portal/deals/${DEAL.id}`) data = DEAL;
    if (path === '/client-portal/overview') data = [];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    if (route.request().method() !== 'GET' && !/^\/auth\/(refresh|portal\/refresh|switch-)/.test(path)) {
      await route.fallback(); return;
    }
    const list = Array.isArray(data);
    await route.fulfill({
      json: { success: true, statusCode: 200, data, meta: { total: list ? (data as unknown[]).length : 1 } },
    });
  });
}

// Imóvel da negociação: a busca do negócio devolve as SUGESTÕES e pode não
// conter o imóvel negociado (foi assim que o detalhe caiu em "Imóvel não
// encontrado" com a API respondendo 200).
const DEAL_PROPERTY = {
  id: 'prop-casa',
  title: 'Casa da Negociação',
  neighborhood: 'Centro',
  city: 'Curitiba',
  state: 'PR',
  listing_type: 'sale',
  sale_price: 1400000,
  rent_price: 0,
  media_images: [],
  thumbnail_url: '',
};

// Imóvel do negócio com o cadastro COMPLETO. Antes o detalhe lia nomes de campo
// da ops (`media_images`, `thumbnail_url`, `sale_price`) que o resumo do portal
// não devolve: a tela caía em placeholder, sem galeria, sem características e
// com "Preço sob consulta".
const RICH_PROPERTY = {
  id: 'prop-casa',
  title: 'Casa da Negociação',
  address: 'Rua Vitória Régia, 152',
  neighborhood: 'Vila Gardênia',
  city: 'Atibaia',
  state: 'SP',
  listing_type: 'sale',
  price: 1400000,
  bedrooms: 3,
  suites: 4,
  bathrooms: 3,
  parking_spaces: 8,
  total_area_m2: 300,
  usable_area_m2: 300,
  amenities: ['piscina'],
  description: 'Casa Vitória Régia',
  image_url: '/e2e-fixtures/fachada.jpg',
  images: [
    { url: '/e2e-fixtures/fachada.jpg', description: 'Fachada' },
    { url: '/e2e-fixtures/piscina.jpg', description: 'Piscina' },
    { url: '/e2e-fixtures/sala.jpg', description: 'Sala de estar' },
  ],
};

// A mesma casa na VITRINE (sugestões do atendimento): o backend mascara rua e
// número antes de responder — privacidade é valor pretendido, não só CSS.
const MASKED_PROPERTY = {
  ...RICH_PROPERTY,
  address: '',
};

test.describe('portal do cliente: acompanhamento do negócio', () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test('cliente com negócio vê o acompanhamento e chega na tela do negócio', async ({ page }) => {
    await seedPortal(page, [DEAL]);
    await page.goto('/portal');

    // NÃO é a jornada pré-negócio (busca de imóveis) — é o negócio em andamento,
    // com o vocabulário de quem compra (papel my_role=buyer do negócio).
    await expect(page.getByText('Sua compra')).toBeVisible();
    await expect(page.getByText('Sua jornada de compra')).toHaveCount(0);

    // A home do negócio: etapas da esteira + o caminho para a tela do negócio.
    await expect(page.getByText(/Etapa \d+ de \d+/)).toBeVisible();
    await expect(page.getByRole('link', { name: /Ver proposta/i }).first()).toBeVisible();

    const dealLink = page.locator(`a[href="/portal/deals/${DEAL.id}"]`).first();
    await expect(dealLink, 'a home do cliente não ofereceu o caminho para o negócio').toBeVisible();

    // O caminho abre a tela de acompanhamento (propostas/documentos).
    await dealLink.click();
    await expect(page).toHaveURL(new RegExp(`/portal/deals/${DEAL.id}`));
  });

  test('abre o imóvel da negociação mesmo quando ele não está nas sugestões', async ({ page }) => {
    await seedPortal(page, [DEAL]);
    await page.route('**/api/v1/client-portal/deals/*/properties', (route) =>
      route.fulfill({ json: { success: true, statusCode: 200, data: [{ id: 'outro', title: 'Sugestão' }], meta: { total: 1 } } }));
    await page.route('**/api/v1/client-portal/deals/*/property', (route) =>
      route.fulfill({ json: { success: true, statusCode: 200, data: DEAL_PROPERTY, meta: { total: 1 } } }));

    // Link direto/recarga: sem estado de navegação, a página depende da API.
    await page.goto('/portal/imoveis/prop-casa');

    await expect(page.getByText('Casa da Negociação')).toBeVisible();
    await expect(page.getByText('Imóvel não encontrado.')).toHaveCount(0);
  });

  test('sem negócio, a home mostra a jornada (não quebra)', async ({ page }) => {
    await seedPortal(page, []);
    await page.goto('/portal');

    // Sem negócio e sem papel de lead conhecido, o texto é neutro.
    await expect(page.getByText('Seu atendimento', { exact: true })).toBeVisible();
    await expect(page.locator(`a[href="/portal/deals/${DEAL.id}"]`)).toHaveCount(0);
  });

  test('detalhe do imóvel do negócio mostra galeria, características, comodidades e descrição', async ({ page }) => {
    await seedPortal(page, [DEAL]);
    await page.route('**/api/v1/client-portal/deals/*/property', (route) =>
      route.fulfill({ json: { success: true, statusCode: 200, data: RICH_PROPERTY, meta: { total: 1 } } }));

    await page.goto('/portal/imoveis/prop-casa');

    // Galeria com contador, legenda cadastrada e miniaturas navegáveis.
    await expect(page.getByText('1/3')).toBeVisible();
    await expect(page.getByText('Fachada')).toBeVisible();

    // A miniatura está FORA da caixa da foto: clicar nela é possível (antes o
    // `overflow-hidden` da caixa com altura fixa cortava a faixa inteira).
    await page.getByRole('button', { name: 'Ver foto 2: Piscina' }).click();
    await expect(page.getByText('2/3')).toBeVisible();
    await page.getByRole('button', { name: 'Ver foto 3: Sala de estar' }).click();
    await expect(page.getByText('3/3')).toBeVisible();
    // A legenda acompanha a foto atual ("Sala de estar" não é comodidade do
    // cadastro, então a asserção é inequívoca).
    await expect(page.getByText('Sala de estar')).toBeVisible();

    // Características e comodidades legíveis.
    await expect(page.getByText('quartos')).toBeVisible();
    await expect(page.getByText('suítes')).toBeVisible();
    await expect(page.getByText('vagas')).toBeVisible();
    await expect(page.getByText('Sobre o imóvel')).toBeVisible();
    await expect(page.getByText('Casa Vitória Régia')).toBeVisible();

    // Negócio do próprio cliente: o endereço completo é dele, então aparece.
    await expect(page.getByText('Rua Vitória Régia, 152')).toBeVisible();
    await expect(page.getByText('Endereço completo liberado após agendar a visita.')).toHaveCount(0);

    // O cadastro TEM preço (R$ 1.400.000): a tela do portal não o anuncia.
    await expect(page.getByText(/R\$/)).toHaveCount(0);
    await expect(page.getByText('Preço sob consulta')).toHaveCount(0);
  });

  test('a foto abre em tela cheia, com contador, legenda e fechamento', async ({ page }) => {
    await seedPortal(page, [DEAL]);
    await page.route('**/api/v1/client-portal/deals/*/property', (route) =>
      route.fulfill({ json: { success: true, statusCode: 200, data: RICH_PROPERTY, meta: { total: 1 } } }));

    await page.goto('/portal/imoveis/prop-casa');

    await page.getByRole('button', { name: /Ampliar/ }).click();

    const viewer = page.getByRole('dialog');
    await expect(viewer).toBeVisible();
    await expect(viewer.getByText('1/3')).toBeVisible();
    await expect(viewer.getByText('Fachada')).toBeVisible();
    await expect(viewer.getByRole('button', { name: 'Ir para foto 3' })).toBeVisible();

    // Tela cheia de verdade: a janela do visualizador cobre a viewport (antes
    // ela abria menor e a página aparecia em volta e por baixo).
    const box = await viewer.boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(390);
    expect(box?.height).toBeGreaterThanOrEqual(844);

    await viewer.getByRole('button', { name: 'Fechar' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('na vitrine o endereço exato não aparece: a tela explica como liberar', async ({ page }) => {
    await seedPortal(page, [DEAL]);
    await page.route('**/api/v1/client-portal/deals/*/properties', (route) =>
      route.fulfill({ json: { success: true, statusCode: 200, data: [MASKED_PROPERTY], meta: { total: 1 } } }));

    await page.goto('/portal/imoveis/prop-casa');

    await expect(page.getByText('Vila Gardênia, Atibaia, SP')).toBeVisible();
    await expect(page.getByText('Endereço completo liberado após agendar a visita.')).toBeVisible();
    await expect(page.getByText('Rua Vitória Régia, 152')).toHaveCount(0);
  });
});

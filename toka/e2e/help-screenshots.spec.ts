// help-screenshots.spec.ts — captura os prints da Central de Ajuda
// (toka/src/help/docs/img) renderizando as telas reais do app com fixtures
// fictícias (sem dados reais, sem rede externa). Executar:
//   E2E_SEED=suitee-ci-v1 npx playwright test --config=playwright.ci.config.ts help-screenshots.spec.ts
// Os PNGs são gravados direto na pasta img/ da fonte única da ajuda.
import { test, expect, type Page } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const IMG_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src/help/docs/img');

const ADMIN_USER = {
  id: 'fake-user-id',
  email: 'maria@example.com',
  firstname: 'Maria',
  lastname: 'Teste',
  role: 'user',
  functional_role: 'admin',
  account_type: 'individual',
  permission: 'owner',
  current_company_id: 'mci-company-id',
};

const now = '2026-10-03T10:00:00Z';

const lead = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `lead-${n}`,
  display_id: `LID-${String(n).padStart(4, '0')}`,
  name: ['Carla Menezes', 'Fábio Ramos', 'Gabriela Prado', 'Diego Santos', 'Eliane Costa', 'Bruno Teles'][n - 1] ?? `Lead ${n}`,
  email: `cliente${n}@example.com`,
  phone: `(11) 9${String(9000 + n)}-${String(1000 + n)}`,
  source: 'Site',
  status: 'Novo',
  role: 'buyer',
  interest_type: 'buy',
  preferred_location: 'Vila Mariana',
  interest_min_price: 400000,
  interest_max_price: 900000,
  is_client: false,
  broker_id: 'broker-1',
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const property = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `prop-${n}`,
  title: ['Apartamento Vila Mariana', 'Casa com piscina — Moema', 'Cobertura Jardins', 'Sobrado Centro', 'Kitnet Pinheiros', 'Terreno Granja Viana'][n - 1] ?? `Imóvel ${n}`,
  address: `Rua Exemplo, ${100 + n}`,
  neighborhood: 'Vila Mariana',
  city: 'São Paulo',
  state: 'SP',
  listing_type: n % 2 === 0 ? 'rent' : 'sale',
  price: 500000 + n * 120000,
  bedrooms: 2 + (n % 3),
  bathrooms: 1 + (n % 2),
  parking_spots: 1,
  total_area_m2: 60 + n * 12,
  status: ['available', 'captation', 'announced'][n % 3],
  image_url: '',
  images: [],
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const deal = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `deal-${n}`,
  display_id: `NEG-${String(n).padStart(4, '0')}`,
  type: n % 2 === 0 ? 'RENT' : 'SALE',
  status: ['NEGOTIATION', 'PROPOSAL', 'ANALYSIS', 'CONTRACT'][n % 4],
  property_id: `prop-${n}`,
  value: 600000 + n * 100000,
  commission: 36000 + n * 6000,
  broker_id: 'broker-1',
  parties: [],
  proposals: [],
  checklist: [],
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const charge = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `charge-${n}`,
  description: `Aluguel — ${['Carla Menezes', 'Fábio Ramos', 'Gabriela Prado', 'Diego Santos'][n - 1] ?? `Locação ${n}`}`,
  amount: 2800 + n * 350,
  due_date: '2026-10-05',
  status: ['pending', 'paid', 'overdue', 'pending'][n % 4],
  contract_id: `ctr-${n}`,
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

// Casos de inadimplência com a forma REAL do contrato: status do domínio,
// `actions[]` (timeline), `guarantee` (modalidade/valor/protocolo) e `judicial`
// (número CNJ, vara, advogado, valor da causa e audiência).
const delinquencyCollections = [
  {
    id: 'col-1',
    tenant_id: 'mci-company-id',
    contract_id: 'ctr-2',
    debtor_id: 'lead-3',
    debtor_name: 'Gabriela Prado',
    total_debt: 3800,
    days_overdue: 18,
    charge_ids: ['charge-3'],
    status: 'ACTIVE',
    notes: 'Contato telefônico sem retorno em 02/10.',
    actions: [
      { date: '2026-10-01T13:00:00Z', type: 'Ligação Telefônica', description: 'Sem resposta; recado na caixa postal.', user_id: 'member-1' },
      { date: '2026-09-28T11:30:00Z', type: 'E-mail', description: 'Cobrança amigável enviada com boleto atualizado.', user_id: 'member-1' },
    ],
    created_at: now,
    updated_at: now,
  },
  {
    id: 'col-2',
    tenant_id: 'mci-company-id',
    contract_id: 'ctr-4',
    debtor_id: 'lead-4',
    debtor_name: 'Diego Santos',
    total_debt: 7200,
    days_overdue: 47,
    charge_ids: ['charge-4'],
    status: 'GUARANTEE_ACTIVATED',
    guarantee: {
      type: 'SEGURO_FIANCA',
      amount: 7200,
      reason: 'Quatro parcelas de aluguel em atraso.',
      status: 'REQUESTED',
      insurer_name: 'Porto Seguro',
      insurer_protocol: 'PS-2026-0098123',
      requested_at: '2026-09-30T12:00:00Z',
      updated_at: now,
    },
    actions: [{ date: '2026-09-30T12:00:00Z', type: 'Notificação Extrajudicial', description: 'Acionamento da garantia protocolado.', user_id: 'member-1' }],
    created_at: now,
    updated_at: now,
  },
  {
    id: 'col-3',
    tenant_id: 'mci-company-id',
    contract_id: 'ctr-5',
    debtor_id: 'lead-5',
    debtor_name: 'Eliane Costa',
    total_debt: 12900,
    days_overdue: 96,
    charge_ids: ['charge-1'],
    status: 'JUDICIAL',
    judicial: {
      case_number: '1001234-56.2026.8.26.0100',
      court: '1ª Vara Cível de São Paulo',
      lawyer: { name: 'Marina Duarte', oab: '123456/SP', email: 'marina@duarteadv.com.br', phone: '(11) 98888-7777' },
      claim_value: 12900,
      status: 'EM_TRAMITE',
      next_hearing: '2026-11-12T14:00:00Z',
      notes: 'Audiência de conciliação designada.',
      created_at: now,
      updated_at: now,
    },
    actions: [{ date: '2026-09-22T10:00:00Z', type: 'Notificação Extrajudicial', description: 'Distribuição da ação de cobrança.', user_id: 'member-1' }],
    created_at: now,
    updated_at: now,
  },
];

const appointment = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `appt-${n}`,
  title: ['Visita — Apartamento Vila Mariana', 'Reunião com proprietário', 'Vistoria de saída', 'Assinatura de contrato'][n - 1] ?? `Compromisso ${n}`,
  type: ['visit', 'meeting', 'inspection', 'signature'][n % 4],
  starts_at: `2026-10-03T${String(8 + n).padStart(2, '0')}:00:00Z`,
  ends_at: `2026-10-03T${String(9 + n).padStart(2, '0')}:00:00Z`,
  lead_id: n % 2 === 0 ? `lead-${n}` : '',
  deal_id: '',
  property_id: '',
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const contract = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `ctr-${n}`,
  display_id: `CTR-${String(n).padStart(4, '0')}`,
  title: `Contrato de ${n % 2 === 0 ? 'locação' : 'compra e venda'} — ${['Carla Menezes', 'Fábio Ramos'][n % 2]}`,
  status: (['DRAFT', 'PENDING_SIGNATURE', 'SIGNED', 'ACTIVE'] as const)[n % 4],
  deal_id: `deal-${n}`,
  type: n % 2 === 0 ? 'locacao' : 'venda',
  content: `# Contrato de ${n % 2 === 0 ? 'Locação Residencial' : 'Compra e Venda'}\n\nPelo presente instrumento particular, as partes qualificadas ao final celebram o presente contrato, mediante as cláusulas e condições seguintes.\n\n## Cláusula 1ª — Do objeto\n\nO presente contrato tem por objeto o imóvel situado na Rua Exemplo, ${100 + n}, Vila Mariana, São Paulo/SP.\n\n## Cláusula 2ª — Do preço\n\nO valor total é de R$ ${(500000 + n * 100000).toLocaleString('pt-BR')},00, a ser pago na forma ajustada entre as partes.\n`,
  signatories: [
    { id: 'sig-1', name: 'Carla Menezes', email: 'carla@example.com', role: 'Locatária', signed_at: n % 3 === 0 ? now : undefined },
    { id: 'sig-2', name: 'Bruno Teles', email: 'bruno@mci.com.br', role: 'Imobiliária' },
  ],
  start_date: '2026-11-01',
  end_date: '2027-10-31',
  value: 500000 + n * 100000,
  commission_pct: 6,
  property_title: `Apartamento Vila Mariana ${n}`,
  client: ['Carla Menezes', 'Fábio Ramos'][n % 2],
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const rental = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `rental-${n}`,
  contract_id: `ctr-${n}`,
  property_id: `prop-${n}`,
  property_title: `Apartamento Vila Mariana ${n}`,
  tenant_name: ['Carla Menezes', 'Fábio Ramos'][n % 2],
  monthly_amount: 2800 + n * 300,
  status: 'active',
  start_date: '2026-01-10',
  end_date: '2027-01-09',
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const ticket = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `ticket-${n}`,
  subject: ['Boleto não chegou por e-mail', 'Agendar vistoria de saída', 'Segunda via do recibo', 'Dúvida sobre reajuste'][n - 1] ?? `Chamado ${n}`,
  status: ['open', 'in_progress', 'awaiting_info', 'resolved'][n % 4],
  priority: 'normal',
  channel: 'portal',
  requester_name: ['Gabriela Prado', 'Carla Menezes'][n % 2],
  created_at: now,
  updated_at: now,
  tenant_id: 'mci-company-id',
  ...overrides,
});

const member = (n: number, overrides: Record<string, unknown> = {}) => ({
  id: `member-${n}`,
  account_id: `member-${n}`,
  firstname: ['Bruno', 'Carla', 'Diego', 'Eliane'][n - 1] ?? `Membro ${n}`,
  lastname: ['Teles', 'Menezes', 'Santos', 'Costa'][n - 1] ?? '',
  email: `membro${n}@mci.com.br`,
  profile: ['owner', 'gestor', 'corretor', 'financeiro'][n - 1] ?? 'corretor',
  functional_role: ['admin', 'gestor', 'corretor', 'financeiro'][n - 1] ?? 'corretor',
  status: 'active',
  ...overrides,
});

// --- Fixtures do financeiro (relatórios) e do fiscal -------------------------
// As telas desreferenciam campos numéricos/arrays: devolver [] no lugar do
// objeto derruba a página no ErrorBoundary, então cada rota tem sua forma.
const financialKpis = {
  tenant_id: 'mci-company-id',
  generated_at: now,
  revenue_month_cents: 31800000,
  revenue_last_30_cents: 28900000,
  receivable_cents: 4520000,
  overdue_cents: 380000,
  overdue_count: 3,
  delinquency_pct: 2.4,
  average_ticket_cents: 980000,
  active_contract_count: 27,
  commission_paid_month_cents: 4200000,
  cash_balance_cents: 18500000,
  owner_payable_cents: 7300000,
  commission_payable_cents: 2100000,
  partner_payable_cents: 450000,
  royalty_payable_cents: 120000,
  tax_withheld_cents: 89000,
  other_payable_cents: 0,
  withdrawal_payable_cents: 150000,
  third_party_obligations_cents: 620000,
  other_commitments_cents: 0,
  committed_funds_cents: 9100000,
  agency_available_cash_cents: 9400000,
  coverage_gap_cents: 0,
  unexpected_payable_debit_cents: 0,
};

const financialDre = {
  tenant_id: 'mci-company-id',
  from: '2026-08-01',
  to: '2026-10-31',
  generated_at: now,
  revenue: [
    { category: 'Comissões de venda', count: 9, total_cents: 19800000 },
    { category: 'Taxa de administração', count: 18, total_cents: 12000000 },
  ],
  expense: [
    { category: 'Repasses e comissões', count: 22, total_cents: 8600000 },
    { category: 'Operacional', count: 6, total_cents: 1240000 },
  ],
  net_cents: 21960000,
  unclassified_receipts_cents: 0,
};

const financialCashflow = {
  tenant_id: 'mci-company-id',
  from: '2026-08-01',
  to: '2026-10-31',
  granularity: 'month',
  generated_at: now,
  entries: [
    { period: '2026-08', inflow_cents: 9800000, outflow_cents: 3200000, net_cents: 6600000 },
    { period: '2026-09', inflow_cents: 10400000, outflow_cents: 3600000, net_cents: 6800000 },
    { period: '2026-10', inflow_cents: 11600000, outflow_cents: 4100000, net_cents: 7500000 },
  ],
};

const financialSummary = {
  tenant_id: 'mci-company-id',
  generated_at: now,
  from: '2026-08-01',
  to: '2026-10-31',
  receivables: { by_status: [{ status: 'PENDING', count: 22, total_cents: 4520000 }], total_cents: 4520000 },
  withdrawals: { by_status: [{ status: 'PENDING', count: 4, total_cents: 7300000 }], total_cents: 7300000 },
  delinquency: { overdue_count: 3, overdue_total_cents: 380000, average_days_overdue: 12 },
};

const fiscalDeadlines = [
  { id: 'dl-1', type: 'DIMOB', title: 'DIMOB 2026', description: 'Declaração anual de operações imobiliárias.', due_date: '2027-02-28', status: 'UPCOMING' },
  { id: 'dl-2', type: 'INFORME', title: 'Informes de rendimentos 2026', description: 'Entrega aos proprietários até o fim de fevereiro.', due_date: '2027-02-28', status: 'DUE_SOON' },
];

const fiscalDimob = {
  id: 'dimob-2026',
  year: 2026,
  status: 'READY',
  total_rentals: 84,
  total_sales: 12,
  total_amount: 4600000,
  generated_at: now,
  created_at: now,
  tenant_id: 'mci-company-id',
};

const fiscalInformes = [
  { id: 'inf-1', year: 2026, owner_id: 'lead-1', owner_name: 'Fábio Ramos', total_gross: 96000, total_admin_fee: 9600, total_intermediation: 0, total_net: 86400, status: 'READY', created_at: now, tenant_id: 'mci-company-id' },
  { id: 'inf-2', year: 2026, owner_id: 'lead-2', owner_name: 'Carla Menezes', total_gross: 72000, total_admin_fee: 7200, total_intermediation: 0, total_net: 64800, status: 'READY', created_at: now, tenant_id: 'mci-company-id' },
];

const fiscalHistory = [
  { id: 'rep-1', type: 'DIMOB', name: 'DIMOB 2025', generated_at: '2026-02-20T10:00:00Z', status: 'READY' },
  { id: 'rep-2', type: 'INFORME', name: 'Informes 2025', generated_at: '2026-02-18T10:00:00Z', status: 'READY' },
];

// Um único handler roteado por caminho: fixture exato -> factory por padrão -> [].
function seedAdmin(page: Page, overrides: Record<string, unknown> = {}) {
  void overrides;
  const debugReq = process.env.PRINT_DEBUG_TEXT === '1';
  // Dica de sessão exigida pelo AuthContext desde o endurecimento de
  // 2026-09-21 (sem ela o boot nem tenta POST /auth/refresh).
  void page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  return page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace('/api/v1', '') || '/';
    if (debugReq) console.log(`[req] ${route.request().method()} ${path}`);
    const respond = (data: unknown, extra: Record<string, unknown> = {}) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          statusCode: 200,
          data,
          total: Array.isArray(data) ? data.length : undefined,
          page: 1,
          limit: 1000,
          meta: { total: Array.isArray(data) ? data.length : 1 },
          ...extra,
        }),
      });

    let data: unknown = [];
    if (path === '/auth/refresh') return respond({ access_token: 'fake-token' });
    if (path === '/auth/me') return respond(ADMIN_USER);
    if (path === '/auth/companies') return respond([{ id: 'mci-company-id', type: 'company', company: { id: 'mci-company-id', name: 'Imobiliária Exemplo', legal_name: 'Imobiliária Exemplo LTDA', tax_id: '11222333000181', email: 'contato@mci.com.br', phone: '11999998888', type: 'imobiliaria', is_active: true, owner_id: ADMIN_USER.id, members: [{ account_id: ADMIN_USER.id, profile: 'owner' }] } }]);
    if (/^\/auth\/switch-/.test(path)) return respond({ access_token: 'fake-tenant-scoped' });
    if (path === '/me/permissions') return respond({
      functional_role: 'admin',
      profile: 'owner',
      permissions: [
        'ai:use', 'module:admin', 'module:crm', 'module:finance', 'module:intelligence',
        'module:portfolio', 'module:rentals', 'leads:read', 'properties:read', 'deals:read',
        'contracts:read', 'contracts:write', 'documents:read', 'templates:read', 'finance:read', 'fiscal:read',
        'reports:read', 'tickets:read', 'rentals:read', 'rentals:inspect', 'charges:read', 'charges:write',
        'system:view_audit_trail', 'company:read_members',
      ],
    });
    // Sem envelope de assinatura: o serviço trata 404 como "nenhum".
    if (/^\/signature\/envelopes\/by-contract\//.test(path)) {
      return route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, statusCode: 404, error: 'envelope não encontrado' }),
      });
    }
    if (path === '/settings/preferences') return respond({ last_company_id: null, consent: { terms_version: '2.0' } });
    if (path === '/company-settings/appearance') return respond({ primary_color: '#7C3AED', logo_url: '', theme: 'light' });

    if (path === '/leads/stats') return respond({ total: 128, conversion_rate: 32 });
    if (path === '/deals/stats') return respond({ monthly_sales: 2450000, active: 14 });
    if (/^\/leads\/roulette/.test(path)) return respond({ enabled: true, active_brokers: [1, 2, 3], config: { distribution: 'round_robin', sla_tiers: [{ name: 'Padrão', minutes: 30 }] } });
    if (/^\/leads\/sla/.test(path)) return respond({ tiers: [], pools: [] });
    if (/^\/leads\/pools/.test(path)) return respond([]);
    if (/^\/leads\b/.test(path)) return respond([lead(1, { status: 'Novo' }), lead(2, { status: 'Contatado' }), lead(3, { status: 'Qualificado' }), lead(4, { status: 'Visita agendada' }), lead(5, { status: 'Proposta' }), lead(6, { status: 'Negociação' })]);

    if (/^\/properties\/prop-1$/.test(path)) return respond(property(1, { images: [{ url: '/e2e-fixtures/fachada.jpg', description: 'Fachada' }], image_url: '/e2e-fixtures/fachada.jpg' }));
    if (/^\/properties\/[^/]+\/media/.test(path)) return respond([{ url: '/e2e-fixtures/fachada.jpg', description: 'Fachada' }]);
    if (/^\/properties\b/.test(path)) return respond([property(1), property(2, { status: 'captation' }), property(3, { status: 'announced' }), property(4), property(5, { listing_type: 'rent' }), property(6, { listing_type: 'rent' })]);

    if (/^\/deals\/deal-1$/.test(path)) return respond(deal(1, { status: 'PROPOSAL', parties: [{ id: 'party-1', name: 'Carla Menezes', role: 'buyer' }] }));
    if (/^\/deals\b/.test(path)) return respond([deal(1, { status: 'NEGOTIATION' }), deal(2, { status: 'PROPOSAL' }), deal(3, { status: 'ANALYSIS' }), deal(4, { status: 'CONTRACT' })]);

    const positions = [
      ['/reports/financial/kpis', financialKpis],
      ['/reports/financial/dre', financialDre],
      ['/reports/financial/cashflow', financialCashflow],
      ['/reports/financial/summary', financialSummary],
      ['/reports/financial/chart-of-accounts', { version: '1', accounts: [] }],
      ['/reports/financial/reconciliation', { tenant_id: 'mci-company-id', generated_at: now, dry_run: true, summary: { total_findings: 0, errors: 0, warnings: 0, duplicates: 0, orphans: 0, excess_alloc: 0, sub_alloc: 0, consistency: 0 }, findings: [] }],
      ['/fiscal/calendar', fiscalDeadlines],
      ['/fiscal/informes', fiscalInformes],
      ['/fiscal/reports', fiscalHistory],
    ] as const;
    for (const [prefix, payload] of positions) {
      if (path.startsWith(prefix)) return respond(payload);
    }
    if (/^\/fiscal\/dimob\/\d+$/.test(path)) return respond(fiscalDimob);

    // Suitee IA: sem mcp_connected=true o painel fica em "Conectando…".
    if (path === '/ai/tools') return respond({
      mcp_connected: true,
      tools: [
        { name: 'search_leads', description: 'Buscar leads por nome, status ou responsável.' },
        { name: 'search_properties', description: 'Buscar imóveis por bairro, faixa de preço e status.' },
        { name: 'search_deals', description: 'Listar negócios da esteira por etapa e valor.' },
        { name: 'search_charges', description: 'Consultar cobranças em aberto e em atraso.' },
        { name: 'search_help', description: 'Buscar na Central de Ajuda como usar a plataforma.' },
        { name: 'get_help_topic', description: 'Ler o passo a passo completo de um tópico da ajuda.' },
      ],
    });
    if (path === '/ai/health') return respond({ status: 'ok', mcp_connected: true });
    if (path.startsWith('/ai/suggestions')) return respond({ suggestions: [
      { label: 'Leads da semana', prompt: 'Liste os leads criados nos últimos 7 dias.' },
      { label: 'Cobranças em atraso', prompt: 'Mostre as cobranças em atraso com valor e cliente.' },
      { label: 'Pipeline atual', prompt: 'Resuma o pipeline por etapa com quantidade e valor.' },
    ] });

    if (/^\/contracts\/[^/]+\/security-deposit-balance$/.test(path)) {
      const held = Number(url.searchParams.get('held') ?? 0);
      return respond({
        contract_id: path.split('/')[2],
        held,
        deductions_pending: 0,
        deductions_charged: 0,
        return_due: held,
        deductions_count: 0,
      });
    }
    if (/^\/contracts\/ctr-1$/.test(path)) return respond(contract(1, { status: 'PENDING_SIGNATURE' }));
    // Contrato individual (a tela de rescisão lê o contrato do caso de cobrança).
    if (/^\/contracts\/[^/]+$/.test(path)) {
      const id = path.split('/')[2];
      const number = Number(id.replace(/\D/g, '')) || 1;
      return respond(contract(number, { id, status: 'ACTIVE', security_deposit: 3000 }));
    }
    if (/^\/contracts\b/.test(path)) return respond([contract(1, { status: 'awaiting_signature' }), contract(2, { status: 'signed' }), contract(3, { status: 'active' }), contract(4, { status: 'draft' })]);

    if (/^\/rentals\/rental-1$/.test(path)) return respond(rental(1, { installments: [{ number: 1, amount: 3100, due_date: '2026-11-05', status: 'paid' }] }));
    if (/^\/rentals\/inspections/.test(path)) return respond([{ id: 'insp-1', type: 'exit', status: 'scheduled', property_title: 'Apartamento Vila Mariana 1', scheduled_at: '2026-10-20T09:00:00Z' }]);
    if (/^\/rentals\b/.test(path)) return respond([rental(1), rental(2), rental(3)]);

    if (/^\/appointments\b/.test(path)) return respond([appointment(1), appointment(2), appointment(3), appointment(4)]);

    // Mensagens: o serviço usa /channels (+ /channels/:id/messages).
    if (path === '/channels') return respond([
      { id: 'ch-1', tenant_id: 'mci-company-id', name: 'Carla Menezes', description: 'Lead — Vila Mariana', type: 'lead', status: 'active', participants: [], entity_id: 'lead-1', entity_type: 'lead', created_by: 'member-1', created_at: now, updated_at: now },
      { id: 'ch-2', tenant_id: 'mci-company-id', name: 'Suporte — Apto Vila Olímpia', description: 'Chamado do portal', type: 'support', status: 'active', participants: [], entity_id: 'ticket-1', entity_type: 'ticket', created_by: 'member-1', created_at: now, updated_at: now },
      { id: 'ch-3', tenant_id: 'mci-company-id', name: 'Equipe Comercial', description: 'Canal interno', type: 'group', status: 'active', participants: [], created_by: 'member-1', created_at: now, updated_at: now },
    ]);
    if (/^\/channels\/[^/]+\/messages/.test(path)) return respond({
      messages: [
        { id: 'msg-1', channel_id: 'ch-1', tenant_id: 'mci-company-id', sender_id: 'lead-1', sender_name: 'Carla Menezes', content: 'Olá! O apartamento da Vila Mariana ainda está disponível?', content_type: 'text', created_at: now },
        { id: 'msg-2', channel_id: 'ch-1', tenant_id: 'mci-company-id', sender_id: 'member-1', sender_name: 'Bruno Teles', content: 'Oi, Carla! Está sim. Posso agendar uma visita para quinta às 10h?', content_type: 'text', created_at: now },
        { id: 'msg-3', channel_id: 'ch-1', tenant_id: 'mci-company-id', sender_id: 'lead-1', sender_name: 'Carla Menezes', content: 'Perfeito, quinta às 10h funciona.', content_type: 'text', created_at: now },
      ],
      total: 3,
    });
    if (path === '/metrics') return respond({ channels: 3, unread: 2, messages_today: 12 });
    if (/^\/messages\/channels/.test(path)) return respond([{ id: 'ch-1', type: 'lead', name: 'Carla Menezes', unread: 2 }]);
    if (/^\/messages\b/.test(path)) return respond([{ id: 'msg-1', channel_id: 'ch-1', sender: 'Carla Menezes', content: 'Olá, o apartamento ainda está disponível?', created_at: now, status: 'delivered' }]);

    if (/^\/charges\/stats/.test(path)) return respond({ pending: 42, overdue: 7, monthly: 318000 });
    if (/^\/charges\b/.test(path)) return respond([charge(1), charge(2, { status: 'paid' }), charge(3, { status: 'overdue' }), charge(4)]);

    // Gestão de inadimplência: a tela lê os casos paginados de /collections
    // (status do domínio: ACTIVE | NEGOTIATED | GUARANTEE_ACTIVATED | JUDICIAL |
    // RESOLVED), o caso completo em /collections/:id e os KPIs de
    // /delinquency/dashboard — nenhum desses números é calculado no cliente.
    if (/^\/collections\/[^/]+$/.test(path)) {
      const id = path.split('/')[2];
      const found = delinquencyCollections.find((item) => item.id === id);
      return respond(found ?? delinquencyCollections[0]);
    }
    if (/^\/collections\b/.test(path)) {
      const status = url.searchParams.get('status');
      const filtered = status ? delinquencyCollections.filter((item) => item.status === status) : delinquencyCollections;
      return respond(filtered, {
        meta: { page: 1, limit: 20, total: filtered.length, totalPages: 1 },
      });
    }
    if (path === '/delinquency/dashboard') {
      return respond({
        stats: {
          total_overdue: 3,
          total_debt_amount: 23900,
          average_debt_amount: 7966.67,
          average_days_overdue: 53.7,
          active_collections: 1,
          recovery_rate: 18.4,
        },
        aging_report: [
          { label: '0-30', min_days: 0, max_days: 30, count: 1, total_debt: 3800 },
          { label: '31-60', min_days: 31, max_days: 60, count: 1, total_debt: 7200 },
          { label: '61-90', min_days: 61, max_days: 90, count: 0, total_debt: 0 },
          { label: '90+', min_days: 91, max_days: 0, count: 1, total_debt: 12900 },
        ],
        collection_stats: {
          active: 1,
          negotiated: 0,
          guarantee_activated: 1,
          judicial: 1,
          resolved: 0,
          total_debt: 23900,
          recovered_amount: 4400,
        },
      });
    }
    if (path === '/delinquency/aging') {
      return respond([{ label: '31-60', min_days: 31, max_days: 60, count: 1, total_debt: 7200 }]);
    }
    if (path === '/delinquency/stats') {
      return respond({ total_overdue: 3, total_debt_amount: 23900, active_collections: 1, recovery_rate: 18.4 });
    }
    if (path === '/delinquency/process') {
      return respond({
        charges_marked_overdue: 3,
        late_fees_calculated: 3,
        collections_created: 1,
        collections_updated: 2,
        processed_at: now,
      });
    }

    if (/^\/transactions\b/.test(path)) return respond([]);
    if (/^\/transfers\b/.test(path)) return respond([]);
    if (/^\/commissions\b/.test(path)) return respond([]);
    if (/^\/invoices\b/.test(path)) return respond([]);

    if (/^\/tickets\b/.test(path)) return respond([ticket(1), ticket(2, { status: 'in_progress' }), ticket(3, { status: 'awaiting_info' }), ticket(4, { status: 'resolved' })]);

    // Demais rotas do fiscal (ex.: histórico por ano) devolvem lista vazia —
    // nunca o objeto de DIMOB, que tem forma própria.
    if (/^\/fiscal\b/.test(path)) return respond([]);

    if (/^\/company\/members/.test(path) || /^\/companies\/members/.test(path) || /^\/company-members/.test(path)) return respond([member(1), member(2), member(3), member(4)]);
    if (/^\/companies\b/.test(path)) return respond([{ id: 'mci-company-id', name: 'Imobiliária Exemplo', members: [member(1), member(2), member(3), member(4)] }]);

    return respond(data);
  });
}

async function shot(page: Page, name: string, url: string, options: { mobile?: boolean; fullPage?: boolean; waitForText?: string } = {}) {
  await page.goto(url);
  if (options.waitForText) {
    try {
      await expect(page.getByText(options.waitForText).first()).toBeVisible({ timeout: 12_000 });
    } catch {
      console.warn(`[prints] texto de espera não encontrado em ${url}: ${options.waitForText} — capturando mesmo assim`);
    }
  }
  await page.waitForLoadState('networkidle').catch(() => undefined);
  await page.waitForTimeout(800);
  // Um print da Central de Ajuda nunca pode sair com o ErrorBoundary na tela:
  // falha o teste em vez de publicar uma imagem quebrada na documentação.
  const visibleText = String(await page.locator('body').innerText().catch(() => ''));
  const boundary = /Algo deu errado|Ocorreu um erro inesperado ao carregar|Página não encontrada/i.exec(visibleText);
  if (boundary) {
    throw new Error(`print ${name}: a tela mostrou estado de erro ("${boundary[0]}") em ${url}`);
  }
  if (process.env.PRINT_DEBUG_TEXT) {
    const text = String(await page.locator('body').innerText().catch(() => '(sem corpo)'));
    const hasError = /Algo deu errado|erro inesperado ao carregar|Página não encontrada/i.test(text);
    // Conteúdo principal = texto depois do chrome do shell (barra de topo).
    const marker = text.includes('Novo Lead') ? 'Novo Lead' : 'Menu';
    const main = text.slice(text.indexOf(marker) + marker.length).trim();
    console.log(`=== DEBUG ${url} ERR=${hasError ? 'SIM' : 'nao'} MAINLEN=${main.length} ===\n${text.slice(0, 1200)}\n...TAIL...\n${text.slice(-600)}\n=== FIM ===`);
  }
  const file = path.join(IMG_DIR, name);
  await page.screenshot({ path: file, fullPage: options.fullPage ?? true });
  if (!fs.existsSync(file)) throw new Error(`screenshot não gravado: ${file}`);
}

test.describe('prints da Central de Ajuda', () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test('dashboard.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'dashboard.png', '/', { waitForText: 'Leads Ativos' });
  });
  test('imoveis.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'imoveis.png', '/imoveis', { waitForText: 'Apartamento Vila Mariana' });
  });
  test('imovel-detalhe.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'imovel-detalhe.png', '/imoveis/prop-1', { waitForText: 'Apartamento Vila Mariana' });
  });
  test('esteira.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'esteira.png', '/esteira', { waitForText: 'NEG-0001' });
  });
  test('contratos.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'contratos.png', '/contratos', { waitForText: 'CTR-0001' });
  });
  test('contrato-detalhe.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'contrato-detalhe.png', '/contratos/ctr-1', { waitForText: 'CTR-0001' });
  });
  test('locacao.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'locacao.png', '/locacao/rental-1', { waitForText: 'Carla Menezes' });
  });
  test('vistoria.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'vistoria.png', '/manutencao', { waitForText: 'Vistoria' });
  });
  test('agenda.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'agenda.png', '/agenda', { waitForText: 'Visita' });
  });
  test('mensagens.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'mensagens.png', '/mensagens', { waitForText: 'Carla Menezes' });
  });
  test('financeiro.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'financeiro.png', '/financeiro', { waitForText: 'Cobranças' });
  });
  test('cobrancas.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'cobrancas.png', '/financeiro/cobrancas', { waitForText: 'Aluguel' });
  });
  test('inadimplencia.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'inadimplencia.png', '/inadimplencia', { waitForText: 'Total em Atraso' });
  });
  test('fiscal.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'fiscal.png', '/fiscal', { waitForText: 'DIMOB' });
  });
  test('chamados.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'chamados.png', '/chamados', { waitForText: 'Boleto' });
  });
  test('equipe.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'equipe.png', '/empresas', { waitForText: 'Imobiliária Exemplo' });
  });
  test('configuracoes.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'configuracoes.png', '/configuracoes', { waitForText: 'Perfil' });
  });
  test('roleta.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'roleta.png', '/roleta', { waitForText: 'Roleta' });
  });
  test('inteligencia.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'inteligencia.png', '/inteligencia', { waitForText: 'Suitee' });
  });
  test('ajuda.png', async ({ page }) => {
    await seedAdmin(page);
    await shot(page, 'ajuda.png', '/ajuda?t=financeiro', { waitForText: 'Central de Ajuda' });
  });

  test.describe('ajuda no celular', () => {
    test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    test('ajuda-mobile.png', async ({ page }) => {
      await seedAdmin(page);
      await shot(page, 'ajuda-mobile.png', '/ajuda?t=financeiro', {
        waitForText: 'Central de Ajuda',
        fullPage: false,
      });
    });
  });

  // Regressão do usuário: os links entre tópicos apontavam para arquivos .md e
  // quebravam dentro do SPA. Agora têm de navegar para /ajuda?t=<id>.
  test('links entre tópicos navegam no SPA', async ({ page }) => {
    await seedAdmin(page);
    await page.goto('/ajuda?t=leads');
    await expect(page.getByRole('heading', { name: 'CRM de Leads' })).toBeVisible({ timeout: 15_000 });
    const vejaTambem = page.locator('article').getByRole('link', { name: 'Roleta de Leads' }).first();
    await expect(vejaTambem).toHaveAttribute('href', '/ajuda?t=roleta');
    await vejaTambem.click();
    await expect(page).toHaveURL(/\/ajuda\?t=roleta/);
    await expect(page.getByRole('heading', { name: 'Roleta de Leads' })).toBeVisible({ timeout: 15_000 });

    // O índice usa a forma `topicos/NN-slug.md` e precisa do mesmo tratamento.
    await page.goto('/ajuda');
    const indice = page.locator('article').getByRole('link', { name: 'Leads', exact: true }).first();
    await expect(indice).toHaveAttribute('href', '/ajuda?t=leads');
  });

  // UX: o menu lateral não pode sumir ao rolar o conteúdo no desktop.
  test('desktop: menu lateral e busca seguem acessíveis ao rolar', async ({ page }) => {
    await seedAdmin(page);
    await page.goto('/ajuda?t=financeiro');
    await expect(page.getByRole('heading', { name: 'Financeiro', exact: true })).toBeVisible({ timeout: 15_000 });

    const scrollable = page.locator('main');
    await scrollable.evaluate((el) => el.scrollTo({ top: el.scrollHeight }));
    await page.waitForTimeout(500);

    await expect(page.getByLabel('Buscar na Central de Ajuda')).toBeVisible();
    const nav = page.getByRole('navigation', { name: 'Tópicos da Central de Ajuda' });
    await expect(nav.getByRole('button', { name: /Começando na Suitee/ })).toBeVisible();
  });

  // UX mobile: assuntos recolhidos, abrem em um toque e o conteúdo fica no topo.
  test('mobile: assuntos recolhem, abrem em um toque e trocam o tópico', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await seedAdmin(page);
    await page.goto('/ajuda?t=leads');
    await expect(page.getByRole('heading', { name: 'CRM de Leads' })).toBeVisible({ timeout: 15_000 });

    // Recolhido: a lista não ocupa a tela.
    await expect(page.getByRole('button', { name: 'Roleta de Leads' })).toBeHidden();
    await page.getByRole('button', { name: /Assuntos/ }).click();
    await page.getByRole('button', { name: 'Roleta de Leads' }).click();

    await expect(page).toHaveURL(/t=roleta/);
    await expect(page.getByRole('heading', { name: 'Roleta de Leads' })).toBeVisible({ timeout: 15_000 });
    // Depois de escolher, a lista fecha e o botão passa a mostrar o tópico atual.
    await expect(page.locator('#ajuda-topicos')).toBeHidden();
    await expect(page.getByRole('button', { name: /Roleta de Leads/ })).toHaveAttribute('aria-expanded', 'false');
  });

  // O preview da busca não pode exibir marcação crua de markdown.
  test('busca: preview em texto puro', async ({ page }) => {
    await seedAdmin(page);
    await page.goto('/ajuda');
    await page.getByLabel('Buscar na Central de Ajuda').fill('dimob');
    const nav = page.locator('#ajuda-topicos');
    await expect(nav.getByText('Fiscal e Relatórios').first()).toBeVisible({ timeout: 15_000 });
    const texto = await nav.innerText();
    for (const marca of ['##', '**', '](./', '](../', '```']) {
      expect(texto).not.toContain(marca);
    }
  });

  test.describe('portal', () => {
    test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    test('portal-dashboard.png', async ({ page }) => {
      await seedPortal(page);
      await shot(page, 'portal-dashboard.png', '/portal', { waitForText: 'Sua compra' });
    });
    test('portal-imoveis.png', async ({ page }) => {
      await seedPortal(page);
      await shot(page, 'portal-imoveis.png', '/portal/imoveis', { waitForText: 'Casa Vitória Régia' });
    });
  });
});

const PORTAL_USER = {
  id: 'fake-user-id', email: 'cliente@example.test', firstname: 'Gabriela', lastname: 'Prado',
  role: 'user', functional_role: 'cliente', account_type: 'individual',
  permission: 'owner', current_company_id: 'company-1',
};

const PORTAL_DEAL = {
  id: 'deal-1', display_id: 'NEG-0001', type: 'SALE', status: 'NEGOTIATION',
  property_id: 'prop-casa', owner_id: 'lead-1', customer_id: 'lead-1',
  broker_id: 'broker-1', value: 1400000, commission: 70000, tenant_id: 'company-1',
  created_at: now, updated_at: now,
  my_party_id: 'party-1', my_role: 'buyer', parties: [], proposals: [], checklist: [],
};

const PORTAL_PROPERTY = {
  id: 'prop-casa', title: 'Casa Vitória Régia', address: 'Rua Vitória Régia, 152',
  neighborhood: 'Vila Gardênia', city: 'Atibaia', state: 'SP',
  listing_type: 'sale', price: 1400000, bedrooms: 3, bathrooms: 3, total_area_m2: 300,
  image_url: '/e2e-fixtures/fachada.jpg', images: [{ url: '/e2e-fixtures/fachada.jpg', description: 'Fachada' }],
};

async function seedPortal(page: Page) {
  await page.addInitScript(() => window.localStorage.setItem('auth_has_session', '1'));
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    let data: unknown = [];
    if (path === '/auth/refresh' || path === '/auth/portal/refresh' || /^\/auth\/switch-/.test(path)) data = { access_token: 'synthetic' };
    if (path === '/auth/me') data = PORTAL_USER;
    if (path === '/auth/companies') data = [];
    if (path === '/me/permissions') data = { functional_role: 'cliente', profile: 'cliente', permissions: [] };
    if (path === '/settings/preferences') data = { last_company_id: null, consent: { terms_version: '2.0' } };
    if (path === '/client-portal/deals') data = [PORTAL_DEAL];
    if (path === `/client-portal/deals/${PORTAL_DEAL.id}`) data = PORTAL_DEAL;
    if (path === `/client-portal/deals/${PORTAL_DEAL.id}/property`) data = PORTAL_PROPERTY;
    if (path === `/client-portal/deals/${PORTAL_DEAL.id}/recommendations`) data = [];
    if (path === '/client-portal/overview') data = [{ id: 'lead-1', recommended_property: { ...PORTAL_PROPERTY, match_score: 92, match_reasons: { price: 1, location: 1, used_weights: { price: 1, location: 1 } } }, matched_properties: [] }];
    if (path === '/client-portal/properties') data = [PORTAL_PROPERTY];
    if (path === '/client-portal/search') data = [PORTAL_PROPERTY];
    if (path === '/client-errors') { await route.fulfill({ status: 204 }); return; }
    const list = Array.isArray(data);
    await route.fulfill({
      json: { success: true, statusCode: 200, data, meta: { total: list ? (data as unknown[]).length : 1 } },
    });
  });
}

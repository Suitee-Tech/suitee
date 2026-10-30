import { expect, test, type APIRequestContext } from '@playwright/test';
import { apiRequest, createTestUser, type TestUser } from './fixtures/api';

const API_URL = process.env.E2E_API_URL || 'http://localhost:8080/api/v1';
const mutableFinancialE2EEnabled =
  process.env.E2E_MUTABLE_FINANCIAL === '1' && process.env.E2E_ISOLATED_DATABASE === '1';

type ApiEnvelope<T> = { data?: T };

interface RentalChargeConfig {
  id: string;
  contract_id: string;
}

interface ChargePayment {
  id: string;
  amount_cents: number;
}

interface FinancialCharge {
  id: string;
  contract_id: string;
  total_amount: number;
  paid_amount?: number;
  status: string;
  payments?: ChargePayment[];
}

interface OwnerTransfer {
  id: string;
  contract_id: string;
  source_charge_id?: string;
  gross_rent: number;
  net_amount: number;
}

interface Transaction {
  id: string;
  contract_id: string;
  tenant_id: string;
}

interface CategoryBucket {
  category: string;
  total_cents: number;
}

interface ReconciliationReport {
  dry_run: boolean;
  findings: unknown[];
  summary: {
    total_findings: number;
    errors: number;
    warnings: number;
  };
}

async function responseData<T>(response: Awaited<ReturnType<typeof apiRequest>>): Promise<T> {
  const body = (await response.json()) as ApiEnvelope<T>;
  return body.data as T;
}

async function expectOk(
  response: Awaited<ReturnType<typeof apiRequest>>,
  operation: string,
): Promise<void> {
  if (!response.ok()) {
    throw new Error(`${operation}: HTTP ${response.status()} ${await response.text()}`);
  }
}

async function requestWithIdempotencyKey(
  request: APIRequestContext,
  path: string,
  token: string,
  idempotencyKey: string,
  data: Record<string, unknown>,
) {
  return request.post(`${API_URL}${path}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Idempotency-Key': idempotencyKey,
    },
    data,
  });
}

async function poll<T>(
  read: () => Promise<T>,
  ready: (value: T) => boolean,
  description: string,
  timeoutMs = 20_000,
): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  let latest: T;
  do {
    latest = await read();
    if (ready(latest)) return latest;
    await new Promise((resolve) => setTimeout(resolve, 500));
  } while (Date.now() < deadline);
  throw new Error(`Timed out waiting for ${description}; latest=${JSON.stringify(latest!)}`);
}

test.describe.serial('Contract signing -> cash -> transfer -> reports', () => {
  test.skip(
    !mutableFinancialE2EEnabled,
    'Mutable financial E2E requires E2E_MUTABLE_FINANCIAL=1 and E2E_ISOLATED_DATABASE=1',
  );

  let user = {} as TestUser;
  let propertyId = '';
  let tenantLeadId = '';
  let ownerLeadId = '';
  let dealId = '';
  let contractId = '';
  let configId = '';
  let chargeId = '';
  let chargeTotal = 0;
  let firstPayment = 0;

  test.beforeAll(async ({ request }) => {
    user = await createTestUser(request);

    const propertyResponse = await apiRequest(request, 'POST', '/properties', user.token, {
      title: `Financial E2E Property ${Date.now()}`,
      property_type: 'house',
      listing_type: 'rent',
      status: 'available',
      publication_status: 'draft',
      condition: 'new',
      address: {
        street: 'Praça da Sé',
        number: '100',
        neighborhood: 'Sé',
        city: 'São Paulo',
        state: 'São Paulo',
        state_code: 'SP',
        postal_code: '01001000',
        country: 'Brasil',
        country_code: 'BR',
      },
      pricing: {
        rent_price: {
          original_amount: 3000,
          current_amount: 3000,
          currency: 'BRL',
        },
      },
    });
    await expectOk(propertyResponse, 'create property');
    propertyId = (await responseData<{ id: string }>(propertyResponse)).id;

    const tenantLeadResponse = await apiRequest(request, 'POST', '/leads', user.token, {
      name: 'Financial E2E Tenant',
      email: `financial-tenant-${Date.now()}@test.invalid`,
      phone: '11999999991',
      source: 'WEBSITE',
      status: 'NEW',
      role: 'tenant',
    });
    await expectOk(tenantLeadResponse, 'create tenant lead');
    tenantLeadId = (await responseData<{ id: string }>(tenantLeadResponse)).id;

    const ownerLeadResponse = await apiRequest(request, 'POST', '/leads', user.token, {
      name: 'Financial E2E Owner',
      email: `financial-owner-${Date.now()}@test.invalid`,
      phone: '11999999992',
      source: 'REFERRAL',
      status: 'NEW',
      role: 'owner',
    });
    await expectOk(ownerLeadResponse, 'create owner lead');
    ownerLeadId = (await responseData<{ id: string }>(ownerLeadResponse)).id;

    const dealResponse = await apiRequest(request, 'POST', '/deals', user.token, {
      type: 'RENT',
      property_id: propertyId,
      customer_id: tenantLeadId,
      lead_id: tenantLeadId,
      owner_id: ownerLeadId,
      broker_id: user.accountId,
      value: 3000,
      broker_participations: [
        { broker_id: user.accountId, role: 'fechamento', percentage: 100 },
      ],
    });
    await expectOk(dealResponse, 'create deal');
    dealId = (await responseData<{ id: string }>(dealResponse)).id;
  });

  test.afterAll(async ({ request }) => {
    // Financial history is intentionally immutable. These API deletions clean
    // only mutable upstream fixtures; the required isolated database is then
    // discarded by the E2E harness, removing the exact tenant and its ledger.
    if (!user.token) return;
    if (contractId) await apiRequest(request, 'DELETE', `/contracts/${contractId}`, user.token);
    if (dealId) await apiRequest(request, 'DELETE', `/deals/${dealId}`, user.token);
    if (propertyId) await apiRequest(request, 'DELETE', `/properties/${propertyId}`, user.token);
  });

  test('materializes one rental configuration, but no owner transfer before cash', async ({ request }) => {
    const startDate = new Date();
    startDate.setUTCMonth(startDate.getUTCMonth() + 1, 1);
    startDate.setUTCHours(12, 0, 0, 0);
    const endDate = new Date(startDate);
    endDate.setUTCFullYear(endDate.getUTCFullYear() + 1);

    const contractResponse = await apiRequest(request, 'POST', '/contracts', user.token, {
      deal_id: dealId,
      type: 'locacao',
      content: 'Contrato isolado de consistência financeira E2E',
      start_date: startDate.toISOString(),
      end_date: endDate.toISOString(),
      value: 3000,
      signatories: [
        {
          id: user.accountId,
          name: 'Financial E2E Signer',
          email: user.email,
          role: 'locador',
        },
      ],
    });
    await expectOk(contractResponse, 'create contract');
    contractId = (await responseData<{ id: string }>(contractResponse)).id;

    const signResponse = await apiRequest(
      request,
      'POST',
      `/contracts/${contractId}/sign`,
      user.token,
      { signatory_id: user.accountId, signature: 'Financial E2E Signature' },
    );
    await expectOk(signResponse, 'sign contract');
    expect((await responseData<{ status: string }>(signResponse)).status).toBe('SIGNED');

    const config = await poll(
      async () => {
        const response = await apiRequest(
          request,
          'GET',
          `/rental-charge-configs/by-contract/${contractId}`,
          user.token,
        );
        await expectOk(response, 'load rental charge config');
        return responseData<RentalChargeConfig | null>(response);
      },
      (value) => value?.contract_id === contractId,
      'deal.contract.signed rental configuration',
    );
    configId = config!.id;

    const retryConfigResponse = await apiRequest(
      request,
      'GET',
      `/rental-charge-configs/by-contract/${contractId}`,
      user.token,
    );
    await expectOk(retryConfigResponse, 'reload rental charge config');
    expect((await responseData<RentalChargeConfig>(retryConfigResponse)).id).toBe(configId);

    const transfersResponse = await apiRequest(
      request,
      'GET',
      `/transfers?contract_id=${contractId}`,
      user.token,
    );
    await expectOk(transfersResponse, 'list transfers before payment');
    const transfers = await responseData<OwnerTransfer[]>(transfersResponse);
    expect(transfers.filter((item) => item.contract_id === contractId)).toHaveLength(0);

    const rentalsResponse = await apiRequest(
      request,
      'GET',
      `/rentals/${contractId}`,
      user.token,
    );
    await expectOk(rentalsResponse, 'load rental aggregate');
    expect((await responseData<{ contract_id: string }>(rentalsResponse)).contract_id).toBe(
      contractId,
    );
  });

  test('preserves partial receipts and releases exactly one cash-bounded transfer', async ({ request }) => {
    const generationResponse = await apiRequest(
      request,
      'POST',
      `/rental-charge-configs/${configId}/generate-charges`,
      user.token,
    );
    await expectOk(generationResponse, 'generate rental charges');
    expect((await responseData<{ charges_created: number }>(generationResponse)).charges_created).toBeGreaterThan(0);

    const chargesResponse = await apiRequest(
      request,
      'GET',
      `/charges?contract_id=${contractId}&limit=100`,
      user.token,
    );
    await expectOk(chargesResponse, 'list generated charges');
    const charges = await responseData<FinancialCharge[]>(chargesResponse);
    const firstCharge = charges.find((item) => item.contract_id === contractId);
    expect(firstCharge).toBeDefined();
    chargeId = firstCharge!.id;
    chargeTotal = firstCharge!.total_amount;
    firstPayment = Math.floor((chargeTotal * 100) / 2) / 100;
    expect(firstPayment).toBeGreaterThan(0);
    expect(firstPayment).toBeLessThan(chargeTotal);

    const partialKey = `financial-e2e:${contractId}:${chargeId}:partial-1`;
    const partialResponse = await requestWithIdempotencyKey(
      request,
      `/charges/${chargeId}/pay`,
      user.token,
      partialKey,
      { payment_method: 'PIX', paid_amount: firstPayment },
    );
    await expectOk(partialResponse, 'record first partial payment');
    const partial = await responseData<FinancialCharge>(partialResponse);
    expect(partial.status).toBe('PARTIALLY_PAID');
    expect(partial.paid_amount).toBe(firstPayment);
    expect(partial.payments).toHaveLength(1);

    const duplicateResponse = await requestWithIdempotencyKey(
      request,
      `/charges/${chargeId}/pay`,
      user.token,
      partialKey,
      { payment_method: 'PIX', paid_amount: firstPayment },
    );
    await expectOk(duplicateResponse, 'retry first partial payment');
    const duplicate = await responseData<FinancialCharge>(duplicateResponse);
    expect(duplicate.paid_amount).toBe(firstPayment);
    expect(duplicate.payments).toHaveLength(1);

    const beforeSettlementResponse = await apiRequest(
      request,
      'GET',
      `/transfers?contract_id=${contractId}`,
      user.token,
    );
    await expectOk(beforeSettlementResponse, 'list transfers after partial payment');
    expect(
      (await responseData<OwnerTransfer[]>(beforeSettlementResponse)).filter(
        (item) => item.source_charge_id === chargeId,
      ),
    ).toHaveLength(0);

    const from = new Date(Date.now() - 60_000).toISOString();
    const to = new Date(Date.now() + 60_000).toISOString();
    const partialDreResponse = await apiRequest(
      request,
      'GET',
      `/reports/financial/dre?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
      user.token,
    );
    await expectOk(partialDreResponse, 'load DRE after partial payment');
    const partialDre = await responseData<{ revenue: CategoryBucket[] }>(partialDreResponse);
    expect(partialDre.revenue.find((bucket) => bucket.category === 'RENT')?.total_cents).toBe(
      Math.round(firstPayment * 100),
    );

    const remaining = Math.round((chargeTotal - firstPayment) * 100) / 100;
    const settlementResponse = await requestWithIdempotencyKey(
      request,
      `/charges/${chargeId}/reconcile-manual`,
      user.token,
      `financial-e2e:${contractId}:${chargeId}:settlement`,
      {
        payment_method: 'TED',
        paid_amount: remaining,
        paid_at: new Date().toISOString(),
        proof_url: 'https://example.invalid/financial-e2e-proof',
        note: 'isolated financial consistency journey',
      },
    );
    await expectOk(settlementResponse, 'settle charge manually');
    const settled = await responseData<FinancialCharge>(settlementResponse);
    expect(settled.status).toBe('PAID');
    expect(settled.paid_amount).toBe(chargeTotal);
    expect(settled.payments).toHaveLength(2);

    const transfer = await poll(
      async () => {
        const response = await apiRequest(
          request,
          'GET',
          `/transfers?contract_id=${contractId}`,
          user.token,
        );
        await expectOk(response, 'poll charge-backed transfer');
        return (await responseData<OwnerTransfer[]>(response)).find(
          (item) => item.source_charge_id === chargeId,
        );
      },
      (value) => value !== undefined,
      'owner transfer backed by the settled charge',
    );
    expect(transfer!.gross_rent).toBe(chargeTotal);
    expect(transfer!.gross_rent).toBeLessThanOrEqual(settled.paid_amount!);
    expect(transfer!.net_amount).toBeGreaterThanOrEqual(0);

    const allTransfersResponse = await apiRequest(
      request,
      'GET',
      `/transfers?contract_id=${contractId}`,
      user.token,
    );
    await expectOk(allTransfersResponse, 'verify transfer idempotency');
    expect(
      (await responseData<OwnerTransfer[]>(allTransfersResponse)).filter(
        (item) => item.source_charge_id === chargeId,
      ),
    ).toHaveLength(1);
  });

  test('keeps ledger, reports and reconciliation tenant-consistent', async ({ request }) => {
    const transactionsResponse = await apiRequest(
      request,
      'GET',
      `/transactions?contract_id=${contractId}&limit=100`,
      user.token,
    );
    await expectOk(transactionsResponse, 'list contract transactions');
    const transactions = await responseData<Transaction[]>(transactionsResponse);
    expect(transactions.some((item) => item.contract_id === contractId)).toBeTruthy();
    expect(transactions.every((item) => item.tenant_id === user.companyId)).toBeTruthy();

    const from = new Date(Date.now() - 5 * 60_000).toISOString();
    const to = new Date(Date.now() + 5 * 60_000).toISOString();
    const dreResponse = await apiRequest(
      request,
      'GET',
      `/reports/financial/dre?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
      user.token,
    );
    await expectOk(dreResponse, 'load final DRE');
    const dre = await responseData<{ revenue: CategoryBucket[] }>(dreResponse);
    expect(dre.revenue.find((bucket) => bucket.category === 'RENT')?.total_cents).toBe(
      Math.round(chargeTotal * 100),
    );

    const reconciliationResponse = await apiRequest(
      request,
      'GET',
      '/reports/financial/reconciliation',
      user.token,
    );
    await expectOk(reconciliationResponse, 'run reconciliation');
    const reconciliation = await responseData<ReconciliationReport>(reconciliationResponse);
    expect(reconciliation.dry_run).toBe(true);
    expect(reconciliation.summary.total_findings).toBe(reconciliation.findings.length);
    expect(reconciliation.summary.errors + reconciliation.summary.warnings).toBe(
      reconciliation.summary.total_findings,
    );
    expect(reconciliation.summary.errors).toBe(0);
  });
});

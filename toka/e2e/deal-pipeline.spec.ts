import { test, expect } from '@playwright/test';
import { createTestUser, apiRequest, type TestUser } from './fixtures/api';

const API_URL = process.env.E2E_API_URL || 'http://localhost:8080/api/v1';

test.describe.serial('Deal Pipeline E2E', () => {
  let user: TestUser;
  let propertyId: string;
  let leadId: string;
  let dealId: string;

  test.beforeAll(async ({ request }) => {
    user = await createTestUser(request);
  });

  test('should create a property', async ({ request }) => {
    const res = await apiRequest(request, 'POST', '/properties', user.token, {
      title: 'E2E Test Apartment',
      property_type: 'apartment',
      listing_type: 'sale',
      status: 'available',
      publication_status: 'draft',
      condition: 'new',
      address: {
        street: 'Rua Teste',
        number: '123',
        neighborhood: 'Centro',
        city: 'São Paulo',
        state: 'SP',
        zip_code: '01001-000',
      },
      pricing: {
        sale_price: { current_amount: 500000 },
      },
      features: {
        bedrooms: 3,
        bathrooms: 2,
      },
      areas: {
        total_area_m2: 120,
      },
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    propertyId = body.data?.id;
    expect(propertyId).toBeDefined();
  });

  test('should create a lead', async ({ request }) => {
    const res = await apiRequest(request, 'POST', '/leads', user.token, {
      name: 'João Silva E2E',
      email: `lead-${Date.now()}@test.com`,
      phone: '11999999999',
      source: 'website',
      status: 'new',
      interest_type: 'purchase',
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    leadId = body.data?.id;
    expect(leadId).toBeDefined();
  });

  test('should create a deal linking property and lead', async ({ request }) => {
    const res = await apiRequest(request, 'POST', '/deals', user.token, {
      type: 'sale',
      property_id: propertyId,
      customer_id: leadId,
      lead_id: leadId,
      owner_id: user.accountId,
      broker_id: user.accountId,
      value: 500000,
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    dealId = body.data?.id;
    expect(dealId).toBeDefined();
    expect(body.data?.status).toBe('NEGOTIATION');
  });

  test('should add a proposal to the deal', async ({ request }) => {
    const res = await apiRequest(request, 'POST', `/deals/${dealId}/proposals`, user.token, {
      value: 480000,
      conditions: 'Payment in 30 days',
      payment_method: 'bank_transfer',
    });

    expect(res.ok()).toBeTruthy();
  });

  test('should advance deal status to PROPOSAL', async ({ request }) => {
    const res = await apiRequest(request, 'PATCH', `/deals/${dealId}/status`, user.token, {
      status: 'PROPOSAL',
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data?.status).toBe('PROPOSAL');
  });

  test('should get deal by ID with all details', async ({ request }) => {
    const res = await apiRequest(request, 'GET', `/deals/${dealId}`, user.token);

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data?.property_id).toBe(propertyId);
    expect(body.data?.customer_id).toBe(leadId);
  });

  test('should list deals with stats', async ({ request }) => {
    const statsRes = await apiRequest(request, 'GET', '/deals/stats', user.token);
    expect(statsRes.ok()).toBeTruthy();

    const listRes = await apiRequest(request, 'GET', '/deals', user.token);
    expect(listRes.ok()).toBeTruthy();
    const body = await listRes.json();
    expect(body.data?.length).toBeGreaterThanOrEqual(1);
  });
});

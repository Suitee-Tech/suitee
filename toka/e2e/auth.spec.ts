import { test, expect } from '@playwright/test';
import { generateValidCnpj } from './fixtures/api';

const API_URL = process.env.E2E_API_URL || 'http://localhost:8080/api/v1';

test.describe.serial('Authentication Flow', () => {
  const testEmail = `e2e-auth-${Date.now()}@test.com`;
  const testPassword = 'Test@123456';
  let accessToken: string;

  test('should register a new user', async ({ request }) => {
    const res = await request.post(`${API_URL}/auth/register`, {
      data: {
        firstname: 'E2E',
        lastname: 'Auth Test',
        email: testEmail,
        password: testPassword,
        confirm_password: testPassword,
        type: 'individual'
      },
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data).toBeDefined();
  });

  test('should login with registered user', async ({ request }) => {
    const res = await request.post(`${API_URL}/auth/login`, {
      data: {
        email: testEmail,
        password: testPassword,
      },
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    accessToken = body.data?.access_token || body.access_token;
    expect(accessToken).toBeDefined();
  });

  test('should get current user info', async ({ request }) => {
    const res = await request.get(`${API_URL}/auth/me`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data?.email || body.email).toBe(testEmail);
  });

  test('should create a company', async ({ request }) => {
    const res = await request.post(`${API_URL}/companies`, {
      headers: { Authorization: `Bearer ${accessToken}` },
      data: {
        name: 'E2E Test Imobiliaria',
        legal_name: 'E2E Test Imobiliaria LTDA',
        tax_id: generateValidCnpj(),
        email: `company-${Date.now()}@test.com`,
        phone: '11999999999',
        type: 'company',
        address: {
          street: 'Rua Teste',
          number: '100',
          district: 'Centro',
          city: 'São Paulo',
          state: 'SP',
          zip_code: '01001-000',
        },
      },
    });

    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data?.id).toBeDefined();
  });

  test('should reject unauthorized access', async ({ request }) => {
    const res = await request.get(`${API_URL}/auth/me`, {
      headers: { Authorization: 'Bearer invalid-token' },
    });

    expect(res.status()).toBe(401);
  });

  test('should reject login with wrong password', async ({ request }) => {
    const res = await request.post(`${API_URL}/auth/login`, {
      data: {
        email: testEmail,
        password: 'wrong-password',
      },
    });

    expect(res.ok()).toBeFalsy();
  });
});

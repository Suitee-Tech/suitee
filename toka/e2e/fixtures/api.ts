import { APIRequestContext } from '@playwright/test';

const API_URL = process.env.E2E_API_URL || 'http://localhost:8080/api/v1';

// Gera um CNPJ válido (com dígitos verificadores corretos) a partir de uma base de 12 dígitos.
// Usado para evitar que fixtures de teste sejam rejeitadas pelo validador `cnpj_valid`.
export function generateValidCnpj(seed?: string): string {
  const base12 = (seed ?? `${Date.now()}${Math.floor(Math.random() * 10000)}`)
    .replace(/\D/g, '')
    .padStart(12, '0')
    .slice(0, 12);
  const calc = (s: string, weights: number[]): number => {
    let sum = 0;
    for (let i = 0; i < s.length; i++) sum += parseInt(s[i], 10) * weights[i];
    const r = sum % 11;
    return r < 2 ? 0 : 11 - r;
  };
  const d1 = calc(base12, [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  const d2 = calc(base12 + d1, [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  return `${base12}${d1}${d2}`;
}

export interface TestUser {
  email: string;
  password: string;
  token: string;
  companyId: string;
  accountId: string;
}

export async function createTestUser(request: APIRequestContext): Promise<TestUser> {
  const email = `e2e-${Date.now()}@test.com`;
  const password = 'Test@123456';

  // Register
  const registerRes = await request.post(`${API_URL}/auth/register`, {
    data: {
      firstname: 'E2E',
      lastname: 'Test User',
      email,
      password,
      confirm_password: password,
      type: 'individual'
    },
  });

  if (!registerRes.ok()) {
    throw new Error(`Registration failed: ${await registerRes.text()}`);
  }

  // Login
  const loginRes = await request.post(`${API_URL}/auth/login`, {
    data: { email, password },
  });

  if (!loginRes.ok()) {
    throw new Error(`Login failed: ${await loginRes.text()}`);
  }

  const loginData = await loginRes.json();
  let token = loginData.data?.access_token || loginData.access_token;

  // Get account ID
  const meRes = await request.get(`${API_URL}/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!meRes.ok()) {
    throw new Error(`Load current account failed: HTTP ${meRes.status()} ${await meRes.text()}`);
  }
  const meData = await meRes.json();
  const accountId = meData.data?.id || '';
  if (!accountId) {
    throw new Error('Load current account failed: response did not include data.id');
  }

  // Create company
  const companyRes = await request.post(`${API_URL}/companies`, {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      name: `E2E Test Company ${Date.now()}`,
      legal_name: `E2E Test Company ${Date.now()} LTDA`,
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

  if (!companyRes.ok()) {
    throw new Error(`Company creation failed: HTTP ${companyRes.status()} ${await companyRes.text()}`);
  }
  const companyData = await companyRes.json();
  const companyId = companyData.data?.id || '';
  if (!companyId) {
    throw new Error('Company creation failed: response did not include data.id');
  }

  // Switch context to company to get tenant-scoped token
  const switchRes = await request.post(`${API_URL}/auth/switch-context`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { company_id: companyId },
  });
  if (!switchRes.ok()) {
    throw new Error(`Company context switch failed: HTTP ${switchRes.status()} ${await switchRes.text()}`);
  }
  const switchData = await switchRes.json();
  const switchedToken = switchData.data?.access_token || '';
  if (!switchedToken) {
    throw new Error('Company context switch failed: response did not include data.access_token');
  }
  token = switchedToken;

  return { email, password, token, companyId, accountId };
}

export async function loginUser(
  request: APIRequestContext,
  email: string,
  password: string
): Promise<string> {
  const res = await request.post(`${API_URL}/auth/login`, {
    data: { email, password },
  });

  if (!res.ok()) {
    throw new Error(`Login failed: ${await res.text()}`);
  }

  const data = await res.json();
  return data.data?.access_token || data.access_token;
}

export async function apiRequest(
  request: APIRequestContext,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE',
  path: string,
  token: string,
  data?: Record<string, unknown>
) {
  const options = {
    headers: { Authorization: `Bearer ${token}` },
    ...(data ? { data } : {}),
  };

  const res = await request[method.toLowerCase() as 'get' | 'post' | 'put' | 'patch' | 'delete'](
    `${API_URL}${path}`,
    options
  );

  return res;
}

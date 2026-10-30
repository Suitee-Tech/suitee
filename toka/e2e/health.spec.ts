import { test, expect } from '@playwright/test';

const GATEWAY_URL = process.env.E2E_API_URL?.replace('/api/v1', '') || 'http://localhost:8080';

const services = [
  { name: 'gateway', url: `${GATEWAY_URL}/health` },
];

test.describe('Service Health Checks', () => {
  for (const svc of services) {
    test(`${svc.name} should be healthy`, async ({ request }) => {
      const res = await request.get(svc.url);
      expect(res.ok()).toBeTruthy();
      const body = await res.json();
      expect(body.status).toBe('ok');
    });
  }
});

import { test, expect } from '@playwright/test';
import { apiRequest, createTestUser, type TestUser } from './fixtures/api';

async function waitForLeadAssigned(
  request: any,
  token: string,
  leadId: string,
  timeoutMs = 15000
): Promise<string> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const res = await apiRequest(request, 'GET', `/leads/${leadId}`, token);
    if (res.ok()) {
      const body = await res.json();
      const assigned = body.data?.assigned_broker_id || body.data?.AssignedBrokerID;
      if (assigned) return String(assigned);
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`Timeout waiting for lead assignment: ${leadId}`);
}

test.describe.serial('Roulette Pipeline E2E', () => {
  let user: TestUser;
  let brokerIds: string[] = [];

  test.beforeAll(async ({ request }) => {
    user = await createTestUser(request);

    const createBroker = async (name: string) => {
      const res = await apiRequest(request, 'POST', '/brokers', user.token, {
        name,
        role: 'corretor',
        active: true,
      });
      expect(res.ok()).toBeTruthy();
      const body = await res.json();
      return body.data?.id as string;
    };

    brokerIds = [
      await createBroker('E2E Broker 1'),
      await createBroker('E2E Broker 2'),
      await createBroker('E2E Broker 3'),
      await createBroker('E2E Broker 4'),
      await createBroker('E2E Broker 5'),
    ];

    const cfgRes = await apiRequest(request, 'PUT', '/leads/roulette', user.token, {
      enabled: true,
      active_brokers: brokerIds.slice(0, 2),
      stage_broker_pools: { NEW: brokerIds },
      distMethod: 'sequential',
      respectSpecialty: false,
      respectRegion: false,
      respectSchedule: false,
      autoCapLimit: false,
    });
    expect(cfgRes.ok()).toBeTruthy();
  });

  test('should distribute leads using stage pool (NEW) even when leads are created with lowercase status', async ({ request }) => {
    const assigned: string[] = [];
    for (let i = 0; i < 10; i++) {
      const res = await apiRequest(request, 'POST', '/leads', user.token, {
        name: `Roulette Lead ${i}`,
        email: `roulette-${Date.now()}-${i}@test.com`,
        phone: '11999999999',
        source: 'website',
        status: 'new',
      });

      expect(res.ok()).toBeTruthy();
      const body = await res.json();
      const leadId = body.data?.id as string;
      expect(leadId).toBeDefined();

      assigned.push(await waitForLeadAssigned(request, user.token, leadId));
    }

    const expected = brokerIds;
    for (let i = 0; i < assigned.length; i++) {
      const want = expected[i % expected.length];
      expect(assigned[i]).toBe(want);
    }
  });

  test('should not block distribution when respectSpecialty/respectRegion are enabled but brokers have no specialties/regions', async ({ request }) => {
    const cfgRes = await apiRequest(request, 'PUT', '/leads/roulette', user.token, {
      enabled: true,
      active_brokers: brokerIds.slice(0, 2),
      stage_broker_pools: { NEW: brokerIds },
      distMethod: 'sequential',
      respectSpecialty: true,
      respectRegion: true,
      respectSchedule: false,
      autoCapLimit: false,
    });
    expect(cfgRes.ok()).toBeTruthy();

    const assigned: string[] = [];
    for (let i = 0; i < 5; i++) {
      const res = await apiRequest(request, 'POST', '/leads', user.token, {
        name: `Roulette Buyer ${i}`,
        email: `roulette-buyer-${Date.now()}-${i}@test.com`,
        phone: '11999999999',
        source: 'website',
        status: 'new',
        role: 'buyer',
      });

      expect(res.ok()).toBeTruthy();
      const body = await res.json();
      const leadId = body.data?.id as string;
      expect(leadId).toBeDefined();

      assigned.push(await waitForLeadAssigned(request, user.token, leadId));
    }

    const expected = brokerIds;
    for (let i = 0; i < assigned.length; i++) {
      const want = expected[i % expected.length];
      expect(assigned[i]).toBe(want);
    }
  });
});

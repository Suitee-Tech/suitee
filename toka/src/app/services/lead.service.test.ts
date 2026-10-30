import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { AxiosError } from 'axios';

vi.mock('../../shared/services/api', () => ({
  default: { get: vi.fn() },
  unwrapApiData: (r: any) => r?.data?.data ?? r?.data,
}));

import api from '../../shared/services/api';
import { leadService, type ApiLead } from './lead.service';

const stubLead = (id: string): ApiLead => ({
  id,
  name: `lead-${id}`,
  email: `${id}@x.com`,
  phone: '+5511999999999',
  status: 'NEW',
  source: 'WEBSITE',
  created_at: '2026-05-17T00:00:00Z',
  updated_at: '2026-05-17T00:00:00Z',
} as ApiLead);

const apiGet = api.get as unknown as ReturnType<typeof vi.fn>;

const okResponse = (data: ApiLead[], total: number) => ({
  data: { data, total, page: 1, limit: data.length },
});

const cappedResponse = (cap: number) => {
  const err = new Error('Request failed with status code 400') as AxiosError;
  (err as any).isAxiosError = true;
  err.response = {
    status: 400,
    data: { message: `limit exceeds maximum of ${cap}`, success: false },
    headers: { 'x-max-limit': String(cap) },
    statusText: 'Bad Request',
    config: {} as any,
  };
  return err;
};

describe('leadService.getAll', () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  it('returns a single page when limit <= 100', async () => {
    apiGet.mockResolvedValueOnce(okResponse([stubLead('a'), stubLead('b')], 2));

    const res = await leadService.getAll({ limit: 50 });

    expect(apiGet).toHaveBeenCalledTimes(1);
    expect(apiGet.mock.calls[0][1].params).toMatchObject({ limit: 50, offset: 0 });
    expect(res.data).toHaveLength(2);
  });

  it('paginates transparently when caller asks for more than the API cap', async () => {
    // total=180, cap=100 → 2 page requests
    apiGet
      .mockResolvedValueOnce(okResponse(Array.from({ length: 100 }, (_, i) => stubLead(`p1-${i}`)), 180))
      .mockResolvedValueOnce(okResponse(Array.from({ length: 80 }, (_, i) => stubLead(`p2-${i}`)), 180));

    const res = await leadService.getAll({ limit: 1000 });

    expect(apiGet).toHaveBeenCalledTimes(2);
    expect(apiGet.mock.calls[0][1].params).toMatchObject({ limit: 100, offset: 0 });
    expect(apiGet.mock.calls[1][1].params).toMatchObject({ limit: 80, offset: 100 });
    expect(res.data).toHaveLength(180);
    expect(res.total).toBe(180);
  });

  it('stops paginating when the server returns an empty page', async () => {
    apiGet
      .mockResolvedValueOnce(okResponse(Array.from({ length: 100 }, (_, i) => stubLead(`p1-${i}`)), 1000))
      .mockResolvedValueOnce(okResponse([], 1000));

    const res = await leadService.getAll({ limit: 1000 });

    expect(res.data).toHaveLength(100);
    // Two HTTP calls: first that returned 100, second empty terminator.
    expect(apiGet).toHaveBeenCalledTimes(2);
  });

  it('retries with header-supplied cap on 400 X-Max-Limit', async () => {
    // Caller asks for 80 (under our local 100 cap), but backend has lowered
    // its own cap to 50 since the FE shipped. The service must read the
    // X-Max-Limit header and retry transparently.
    apiGet
      .mockRejectedValueOnce(cappedResponse(50))
      .mockResolvedValueOnce(okResponse([stubLead('x')], 1));

    const res = await leadService.getAll({ limit: 80 });

    expect(apiGet).toHaveBeenCalledTimes(2);
    expect(apiGet.mock.calls[0][1].params).toMatchObject({ limit: 80, offset: 0 });
    expect(apiGet.mock.calls[1][1].params).toMatchObject({ limit: 50, offset: 0 });
    expect(res.data).toHaveLength(1);
  });

  it('propagates non-cap 400 errors untouched', async () => {
    const fatal = new Error('Bad Request') as AxiosError;
    (fatal as any).isAxiosError = true;
    fatal.response = {
      status: 400,
      data: { message: 'invalid scope value' },
      headers: {},
      statusText: 'Bad Request',
      config: {} as any,
    };
    apiGet.mockRejectedValueOnce(fatal);

    await expect(leadService.getAll({ limit: 50 })).rejects.toThrow(/Bad Request/);
    expect(apiGet).toHaveBeenCalledTimes(1);
  });
});

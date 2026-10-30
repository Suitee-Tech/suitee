import { beforeEach, describe, expect, it, vi } from 'vitest';
import api from './api';
import { calendarService } from './calendar.service';
vi.mock('./api', () => ({ default: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }));
const raw = (id: number) => ({ id: String(id), title: 'Visita', date: '2099-10-20', time: '14:00', end_time: '15:00', type: 'Visita', client: 'Lucas', status: 'Pendente' });
describe('calendar API completeness and editing', () => {
 beforeEach(() => vi.clearAllMocks());
 it('collects every page with stable totals', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({ data: { data: Array.from({ length: 100 }, (_, i) => raw(i)), meta: { total: 101 } } } as never).mockResolvedValueOnce({ data: { data: [raw(100)], meta: { total: 101 } } } as never);
  expect((await calendarService.getAll()).data).toHaveLength(101);
  expect(api.get).toHaveBeenLastCalledWith('/appointments', { params: { limit: 100, offset: 100 }, signal: undefined });
 });
 it('rejects a shrinking total instead of publishing a partial list', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({ data: { data: Array.from({ length: 100 }, (_, i) => raw(i)), meta: { total: 200 } } } as never).mockResolvedValueOnce({ data: { data: [raw(199)], meta: { total: 101 } } } as never);
  await expect(calendarService.getAll()).rejects.toThrow('mudou durante');
 });
 it('cancels collection before starting the next page', async () => {
  const controller = new AbortController();
  vi.mocked(api.get).mockImplementationOnce(async () => { controller.abort(); return { data: { data: Array.from({ length: 100 }, (_, i) => raw(i)), meta: { total: 200 } } } as never; });
  await expect(calendarService.getAll(undefined, { signal: controller.signal })).rejects.toThrow('cancelada');
  expect(api.get).toHaveBeenCalledTimes(1);
 });
 it('sends selected deal, explicit property clear and the observed revision', async () => {
  vi.mocked(api.put).mockResolvedValue({ data: { data: raw(1) } } as never);
  await calendarService.update('1', { dealId: 'deal', propertyId: '', property: '', expectedUpdatedAt: '2026-09-17T10:00:00Z' });
  expect(api.put).toHaveBeenCalledWith('/appointments/1', { deal_id: 'deal', property_id: '', property: '', expected_updated_at: '2026-09-17T10:00:00Z' });
 });
 it('requests team scope explicitly and preserves privacy metadata', async () => {
  vi.mocked(api.get).mockResolvedValue({ data: { data: [{ ...raw(1), agent: 'Ana', agent_id: 'agent-1', visibility: 'private' }], meta: { total: 1 } } } as never);
  const result = await calendarService.getAll({ scope: 'team' });
  expect(api.get).toHaveBeenCalledWith('/appointments', { params: { scope: 'team', limit: 100, offset: 0 }, signal: undefined });
  expect(result.data[0]).toMatchObject({ agentId: 'agent-1', agentName: 'Ana', visibility: 'private' });
  vi.mocked(api.post).mockResolvedValue({ data: { data: raw(2) } } as never);
  await calendarService.create({ title: 'Médico', date: '2099-10-20', time: '14:00', endTime: '15:00', type: 'Outro', client: '', agent: 'Ana', visibility: 'private' });
  expect(api.post).toHaveBeenCalledWith('/appointments', expect.objectContaining({ visibility: 'private' }));
 });
});

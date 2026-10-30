import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('sonner', () => ({
  toast: { error: vi.fn() },
}));

import { toast } from 'sonner';
import { notifyApiError } from './errorToast';

const toastError = toast.error as unknown as ReturnType<typeof vi.fn>;

const apiError = (status: number, message?: string) => ({
  response: { status, data: message ? { message } : undefined, headers: {} },
  message: message ?? 'Request failed',
});

describe('notifyApiError', () => {
  beforeEach(() => {
    toastError.mockReset();
  });

  it('swallows 401 (axios refresh interceptor handles it)', () => {
    notifyApiError(apiError(401, 'unauthorized'), { context: 'X' });
    expect(toastError).not.toHaveBeenCalled();
  });

  it('translates the limit-exceeded message into pt-BR copy', () => {
    notifyApiError(apiError(400, 'limit exceeds maximum of 100'), { context: 'Carregar leads' });
    expect(toastError).toHaveBeenCalledOnce();
    const arg = toastError.mock.calls[0][0] as string;
    expect(arg).toContain('Carregar leads');
    expect(arg).toContain('paginando');
  });

  it('translates the invalid_phone message', () => {
    notifyApiError(apiError(400, 'invalid phone: invalid Brazilian phone number'), { context: 'Salvar lead' });
    const arg = toastError.mock.calls[0][0] as string;
    expect(arg).toContain('Telefone em formato inválido');
  });

  it('falls back to a status-specific message when there is no body message', () => {
    notifyApiError({ response: { status: 503, data: undefined, headers: {} } }, { context: 'Carregar leads' });
    const arg = toastError.mock.calls[0][0] as string;
    expect(arg).toMatch(/Servi[çc]o temporariamente indispon[íi]vel/i);
  });

  it('respects silent flag', () => {
    notifyApiError(apiError(500), { context: 'X', silent: true });
    expect(toastError).not.toHaveBeenCalled();
  });

  it('falls through to raw backend message when unknown', () => {
    notifyApiError(apiError(400, 'some custom validation error'), { context: 'Salvar lead' });
    const arg = toastError.mock.calls[0][0] as string;
    expect(arg).toContain('some custom validation error');
  });
});

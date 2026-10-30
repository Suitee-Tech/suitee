import { describe, expect, it, vi } from 'vitest';
import { CollectionAbortedError, collectPages } from './collectPages';

describe('complete financial lists', () => {
  it('includes older records beyond all former limits, including server-clamped pages', async () => {
    const records = Array.from({ length: 651 }, (_, id) => ({ id: String(id), amount: 10 }));
    const fetch = vi.fn(async ({ offset }: { offset: number }) => ({
      data: records.slice(offset, offset + 50), meta: { total: records.length, limit: 50 },
    }));
    const result = await collectPages(fetch);
    expect(result).toEqual(records);
    expect(result.reduce((total, item) => total + item.amount, 0)).toBe(6510);
    expect(fetch).toHaveBeenLastCalledWith({ limit: 100, offset: 650 });
  });

  it('rejects a later page failure instead of publishing a partial total', async () => {
    const fetch = vi.fn().mockResolvedValueOnce({ data: [{ id: 'first' }], total: 2 }).mockRejectedValueOnce(new Error('offline'));
    await expect(collectPages(fetch)).rejects.toThrow('offline');
  });

  it('detects a server ignoring offset and an empty incomplete page', async () => {
    await expect(collectPages(async () => ({ data: [{ id: 'same' }], total: 2 }))).rejects.toThrow('repetidos');
    const fetch = vi.fn().mockResolvedValueOnce({ data: [{ id: 'first' }], total: 2 }).mockResolvedValueOnce({ data: [], total: 2 });
    await expect(collectPages(fetch)).rejects.toThrow('lista completa');
  });

  it('does not mistake a changing total for a complete snapshot', async () => {
    const fetch = vi.fn().mockResolvedValueOnce({ data: [{ id: 'first' }], total: 2 }).mockResolvedValueOnce({ data: [{ id: 'second' }], total: 3 });
    await expect(collectPages(fetch)).rejects.toThrow('mudou');
  });

  // N-03: sem total E sem meta.limit não há como provar que a página veio
  // inteira. Antes o cliente comparava com o próprio limite pedido (100), então
  // um servidor clampando para 50 devolvia 50 registros como lista completa.
  it('recusa publicar sem total e sem limite informado pelo servidor', async () => {
    const fetch = vi.fn(async ({ offset }: { offset: number }) => ({
      // Servidor clampa em 50 e não reporta nem total nem limit.
      data: Array.from({ length: 50 }, (_, i) => ({ id: String(offset + i) })),
    }));
    await expect(collectPages(fetch)).rejects.toThrow(/não é possível garantir que a lista esteja completa/i);
  });

  it('encerra quando o servidor informa o limite e devolve página incompleta', async () => {
    const records = Array.from({ length: 30 }, (_, id) => ({ id: String(id) }));
    const fetch = vi.fn(async ({ offset }: { offset: number }) => ({
      data: records.slice(offset, offset + 50), meta: { limit: 50 },
    }));
    await expect(collectPages(fetch)).resolves.toEqual(records);
  });

  // FIN-12: sem sinal de cancelamento, uma leitura em andamento seguia
  // varrendo a carteira inteira depois que a tela já tinha saído.
  it('interrompe a coleta quando o chamador cancela', async () => {
    const controller = new AbortController();
    const fetch = vi.fn(async ({ offset }: { offset: number }) => {
      controller.abort();
      return { data: [{ id: String(offset) }], meta: { total: 500, limit: 1 } };
    });

    await expect(collectPages(fetch, { signal: controller.signal })).rejects.toBeInstanceOf(CollectionAbortedError);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('não inicia a coleta quando o sinal já está abortado', async () => {
    const controller = new AbortController();
    controller.abort();
    const fetch = vi.fn();

    await expect(collectPages(fetch, { signal: controller.signal })).rejects.toBeInstanceOf(CollectionAbortedError);
    expect(fetch).not.toHaveBeenCalled();
  });
});

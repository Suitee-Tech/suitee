import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useFinancialRead } from './useFinancialRead';

describe('selection-bound financial reads', () => {
  it('ignores a slow older result after changing filters', async () => {
    let resolveOld!: (value: string) => void;
    const old = () => new Promise<string>(resolve => { resolveOld = resolve; });
    const latest = vi.fn().mockResolvedValue('new period');
    const { result, rerender } = renderHook(({ key, load }) => useFinancialRead(key, load), { initialProps: { key: 'old', load: old } });
    rerender({ key: 'new', load: latest });
    await waitFor(() => expect(result.current.data).toBe('new period'));
    await act(async () => resolveOld('old period'));
    expect(result.current.data).toBe('new period');
  });

  it('hides the previous selection on failure and supports retry', async () => {
    const first = vi.fn().mockResolvedValue('old data');
    const next = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce('recovered');
    const { result, rerender } = renderHook(({ key, load }) => useFinancialRead(key, load), { initialProps: { key: 'old', load: first } });
    await waitFor(() => expect(result.current.data).toBe('old data'));
    rerender({ key: 'new', load: next });
    expect(result.current.data).toBeUndefined();
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.loading).toBe(false);
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.data).toBe('recovered'));
    expect(result.current.error).toBeUndefined();
  });
});

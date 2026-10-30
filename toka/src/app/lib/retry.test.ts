import { describe, expect, it, vi } from "vitest";
import { retry } from "./retry";

describe("retry", () => {
  it("retorna o valor na primeira tentativa quando fn resolve", async () => {
    const fn = vi.fn().mockResolvedValue("ok");
    const result = await retry(fn);
    expect(result).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("retenta erros de rede (sem response) até sucesso", async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce(new Error("network error"))
      .mockRejectedValueOnce(new Error("network error"))
      .mockResolvedValueOnce("ok");
    const result = await retry(fn, { attempts: 3, baseDelayMs: 1 });
    expect(result).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("retenta 5xx até sucesso", async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce({ response: { status: 500 } })
      .mockResolvedValueOnce("ok");
    const result = await retry(fn, { attempts: 3, baseDelayMs: 1 });
    expect(result).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("retenta 429 (rate limit)", async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce({ response: { status: 429 } })
      .mockResolvedValueOnce("ok");
    const result = await retry(fn, { attempts: 3, baseDelayMs: 1 });
    expect(result).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("NÃO retenta erro 4xx (request inválida)", async () => {
    const err = { response: { status: 400 } };
    const fn = vi.fn().mockRejectedValue(err);
    await expect(retry(fn, { attempts: 3, baseDelayMs: 1 })).rejects.toEqual(err);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("NÃO retenta 401 nem 403", async () => {
    const fn = vi.fn().mockRejectedValue({ response: { status: 401 } });
    await expect(retry(fn, { attempts: 3, baseDelayMs: 1 })).rejects.toBeDefined();
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("desiste após esgotar as tentativas e relança o último erro", async () => {
    const err = new Error("persistent failure");
    const fn = vi.fn().mockRejectedValue(err);
    await expect(retry(fn, { attempts: 2, baseDelayMs: 1 })).rejects.toEqual(err);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("chama onRetry antes de cada nova tentativa", async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce(new Error("blip"))
      .mockRejectedValueOnce(new Error("blip"))
      .mockResolvedValueOnce("ok");
    const onRetry = vi.fn();
    await retry(fn, { attempts: 3, baseDelayMs: 1, onRetry });
    expect(onRetry).toHaveBeenCalledTimes(2);
    expect(onRetry).toHaveBeenNthCalledWith(1, 1, expect.any(Error));
    expect(onRetry).toHaveBeenNthCalledWith(2, 2, expect.any(Error));
  });

  it("respeita shouldRetry customizado", async () => {
    const fn = vi.fn().mockRejectedValue(new Error("any"));
    const shouldRetry = vi.fn().mockReturnValue(false);
    await expect(retry(fn, { attempts: 3, baseDelayMs: 1, shouldRetry })).rejects.toBeDefined();
    expect(fn).toHaveBeenCalledTimes(1);
    expect(shouldRetry).toHaveBeenCalledTimes(1);
  });

  it("aplica backoff exponencial entre tentativas", async () => {
    // Timers falsos: verifica os atrasos do backoff de forma determinística,
    // sem depender do relógio real. A versão anterior media Date.now() e
    // quebrava na CI por flutuação de ~1ms (89ms vs. limite de 90ms).
    vi.useFakeTimers();
    try {
      const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
      const fn = vi
        .fn()
        .mockRejectedValueOnce(new Error("blip"))
        .mockRejectedValueOnce(new Error("blip"))
        .mockResolvedValueOnce("ok");

      const promise = retry(fn, { attempts: 3, baseDelayMs: 30 });
      await vi.runAllTimersAsync();

      await expect(promise).resolves.toBe("ok");
      expect(fn).toHaveBeenCalledTimes(3);
      // Backoff exponencial: 1ª espera = baseDelay (30ms), 2ª dobra para 60ms.
      const delays = setTimeoutSpy.mock.calls.map((call) => call[1]);
      expect(delays).toEqual([30, 60]);
    } finally {
      vi.useRealTimers();
    }
  });
});

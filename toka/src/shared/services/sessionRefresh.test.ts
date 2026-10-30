import { afterEach, describe, expect, it, vi } from "vitest";

import {
  isSessionRefreshActive,
  resetSessionRefreshForTests,
  runSessionRefresh,
  subscribeSessionRefresh,
} from "./sessionRefresh";

afterEach(() => {
  resetSessionRefreshForTests();
});

/**
 * O Supabase invalida a sessão ao ver REUSO de refresh token: se o boot do
 * AuthProvider, o interceptor do httpClient e a recuperação por foco trocarem o
 * token ao mesmo tempo, o segundo pedido derruba o usuário. Por isso a troca é
 * single-flight.
 */
describe("sessionRefresh", () => {
  it("executa a troca uma única vez para chamadas concorrentes", async () => {
    const work = vi.fn(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
      return "novo-token";
    });

    const [a, b, c] = await Promise.all([
      runSessionRefresh(work),
      runSessionRefresh(work),
      runSessionRefresh(work),
    ]);

    expect(a).toBe("novo-token");
    expect(b).toBe("novo-token");
    expect(c).toBe("novo-token");
    expect(work).toHaveBeenCalledTimes(1);
  });

  it("libera nova troca depois que a anterior termina", async () => {
    const work = vi.fn(async () => "token");
    await runSessionRefresh(work);
    await runSessionRefresh(work);
    expect(work).toHaveBeenCalledTimes(2);
  });

  it("avisa os interessados enquanto a renovação está em andamento", async () => {
    const states: boolean[] = [];
    subscribeSessionRefresh((active) => states.push(active));

    await runSessionRefresh(async () => {
      expect(isSessionRefreshActive()).toBe(true);
      return "token";
    });

    expect(states).toEqual([true, false]);
    expect(isSessionRefreshActive()).toBe(false);
  });

  it("propaga o erro e volta ao estado ocioso", async () => {
    await expect(runSessionRefresh(async () => {
      throw new Error("refresh recusado");
    })).rejects.toThrow("refresh recusado");

    expect(isSessionRefreshActive()).toBe(false);
  });
});

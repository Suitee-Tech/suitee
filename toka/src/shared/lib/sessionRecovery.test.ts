import { describe, expect, it } from "vitest";

import {
  isRecoverableStatus,
  needsSessionRecovery,
  recoveryRetryDelay,
  shouldCheckSessionOnWake,
  TOKEN_RENEW_SKEW_SECONDS,
  tokenExpiryMs,
} from "./sessionRecovery";

/** JWT de mentira: só o payload importa (o `exp` é lido sem validar assinatura). */
function fakeJwt(payload: Record<string, unknown>): string {
  const encode = (value: unknown) =>
    btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `${encode({ alg: "ES256", typ: "JWT" })}.${encode(payload)}.assinatura`;
}

describe("sessionRecovery", () => {
  it("lê o vencimento do JWT e decide se a sessão precisa ser renovada", () => {
    const expSeconds = 1_800_000_000; // 2027-01-15T08:00:00Z
    const token = fakeJwt({ exp: expSeconds });
    expect(tokenExpiryMs(token)).toBe(expSeconds * 1000);

    const beforeExpiry = expSeconds * 1000 - 10 * 60_000;
    expect(needsSessionRecovery(token, beforeExpiry)).toBe(false);

    // Dentro da margem: renova antes de a página disparar consultas.
    const withinSkew = expSeconds * 1000 - (TOKEN_RENEW_SKEW_SECONDS - 10) * 1000;
    expect(needsSessionRecovery(token, withinSkew)).toBe(true);

    // Vencido.
    expect(needsSessionRecovery(token, expSeconds * 1000 + 1)).toBe(true);
  });

  it("sem token em memória a sessão precisa ser restaurada", () => {
    expect(needsSessionRecovery(null)).toBe(true);
    expect(needsSessionRecovery(undefined)).toBe(true);
    expect(needsSessionRecovery("nao-e-jwt")).toBe(false);
  });

  it("decide quando vale checar a sessão ao voltar o foco", () => {
    const now = 10_000_000;
    expect(shouldCheckSessionOnWake(0, now)).toBe(true);
    expect(shouldCheckSessionOnWake(now - 5_000, now)).toBe(false);
    expect(shouldCheckSessionOnWake(now - 10 * 60_000, now)).toBe(true);
  });

  it("separa falha recuperável de erro real", () => {
    // Sessão renovável e indisponibilidade transitória.
    for (const status of [undefined, null, 0, 401, 408, 429, 500, 502, 503, 504]) {
      expect(isRecoverableStatus(status)).toBe(true);
    }
    // Erro real: insistir só atrasa a mensagem.
    for (const status of [400, 403, 404, 409, 422]) {
      expect(isRecoverableStatus(status)).toBe(false);
    }
  });

  it("faz backoff exponencial com teto", () => {
    expect(recoveryRetryDelay(0, 100)).toBe(100);
    expect(recoveryRetryDelay(1, 100)).toBe(200);
    expect(recoveryRetryDelay(2, 100)).toBe(400);
    expect(recoveryRetryDelay(20, 100)).toBeLessThanOrEqual(4_000);
  });
});

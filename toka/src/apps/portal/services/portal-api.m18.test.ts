import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPortalAccessToken, hasPortalSessionHint, setPortalAccessToken, shouldRefreshPortalRequest } from "./portal-api";

// P2/M18 — o access token do portal NÃO pode ir para o localStorage.
//
// A revisão de permissões de 2026-09-19 registrou `portal_access_token` no
// localStorage: bearer de `cliente` legível por XSS, extensão ou por quem
// compartilha o dispositivo. O refresh já é cookie HttpOnly, então o token vive
// em memória e a continuidade de sessão usa um hint booleano.
vi.mock("../../../lib/runtime-env", () => ({ getEnv: () => "http://localhost:8080/api/v1" }));
vi.mock("../../../shared/lib/errorDiagnostics", () => ({ reportClientError: vi.fn() }));
vi.mock("../../../shared/lib/dataSync", () => ({ publishApiMutation: vi.fn() }));

describe("portal-api token storage (P2/M18)", () => {
  beforeEach(() => {
    window.localStorage.clear();
    setPortalAccessToken(null);
    window.localStorage.clear();
  });

  it("mantém o access token em memória e não escreve em localStorage", () => {
    setPortalAccessToken("token-de-cliente");

    expect(getPortalAccessToken()).toBe("token-de-cliente");
    expect(window.localStorage.getItem("portal_access_token")).toBeNull();
    for (let i = 0; i < window.localStorage.length; i += 1) {
      const key = window.localStorage.key(i)!;
      expect(window.localStorage.getItem(key)).not.toBe("token-de-cliente");
    }
  });

  it("registra apenas o hint booleano de sessão", () => {
    setPortalAccessToken("token-de-cliente");

    expect(hasPortalSessionHint()).toBe(true);
    expect(window.localStorage.getItem("portal_has_session")).toBe("1");
    // O hint não é um segredo: não carrega o token.
    expect(window.localStorage.getItem("portal_has_session")).not.toContain("token");
  });

  it("limpa o hint quando a sessão termina", () => {
    setPortalAccessToken("token-de-cliente");
    setPortalAccessToken(null);

    expect(getPortalAccessToken()).toBeNull();
    expect(hasPortalSessionHint()).toBe(false);
    expect(window.localStorage.getItem("portal_has_session")).toBeNull();
  });

  it("não restaura token de execução anterior (recarga limpa a memória)", () => {
    setPortalAccessToken("token-de-cliente");
    // Simula recarga: a variável em memória desaparece, o hint permanece.
    const storage = window.localStorage;
    expect(storage.getItem("portal_has_session")).toBe("1");
    expect(storage.getItem("portal_access_token")).toBeNull();
  });
});

// A restauração de sessão NÃO pode depender do hint: sessão criada antes do
// hint existir (ou em outra aba) tem cookie válido e hint ausente. O gating por
// hint deixava o cliente na tela de login com o refresh válido no cookie.
describe("portal-api restauração de sessão (P2/M18 — refinamento)", () => {
  beforeEach(() => {
    window.localStorage.clear();
    setPortalAccessToken(null);
    window.localStorage.clear();
  });

  it("401 em rota protegida tenta o refresh mesmo sem hint", () => {
    expect(hasPortalSessionHint()).toBe(false);
    expect(
      shouldRefreshPortalRequest({
        error: { response: { status: 401 } },
        originalRequest: { url: "/client-portal/channels" },
      }),
    ).toBe(true);
  });

  it("não tenta refresh em endpoint de auth/OTP nem em retry", () => {
    expect(
      shouldRefreshPortalRequest({
        error: { response: { status: 401 } },
        originalRequest: { url: "/auth/otp/verify" },
      }),
    ).toBe(false);
    expect(
      shouldRefreshPortalRequest({
        error: { response: { status: 401 } },
        originalRequest: { url: "/client-portal/channels", _retry: true },
      }),
    ).toBe(false);
    expect(
      shouldRefreshPortalRequest({
        error: { response: { status: 500 } },
        originalRequest: { url: "/client-portal/channels" },
      }),
    ).toBe(false);
  });
});

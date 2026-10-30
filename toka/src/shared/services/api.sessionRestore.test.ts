import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  clearAuthRestoreFailure,
  hasRecentAuthRestoreFailure,
  markAuthRestoreFailed,
  setAccessToken,
} from "../services/api";

// Incidente de 2026-09-21 — marca de restauração de sessão.
//
// O bootstrap do AuthContext tentava `POST /auth/refresh` sempre que não havia
// token em memória. Depois de o perfil de acesso de um usuário ser trocado, o
// refresh passava mas a primeira chamada autenticada era recusada; o app limpava
// o estado e recarregava o /login, onde o refresh passava de novo… loop infinito
// com a tela presa em "Carregando...".
//
// A marca registra "acabamos de falhar em restaurar" e o boot não tenta de novo
// nesse intervalo (mostra o formulário).

beforeEach(() => {
  sessionStorage.clear();
});

afterEach(() => {
  sessionStorage.clear();
});

describe("marca de falha ao restaurar a sessão", () => {
  it("não existe em uma visita nova", () => {
    expect(hasRecentAuthRestoreFailure()).toBe(false);
  });

  it("passa a existir quando a restauração falha e some no login", () => {
    markAuthRestoreFailed();
    expect(hasRecentAuthRestoreFailure()).toBe(true);

    clearAuthRestoreFailure();
    expect(hasRecentAuthRestoreFailure()).toBe(false);
  });

  it("expira sozinha (não bloqueia o usuário para sempre)", () => {
    sessionStorage.setItem("auth_restore_failed_at", String(Date.now() - 60_000));
    expect(hasRecentAuthRestoreFailure()).toBe(false);
  });

  it("valor corrompido não trava a tela", () => {
    sessionStorage.setItem("auth_restore_failed_at", "não-é-número");
    expect(hasRecentAuthRestoreFailure()).toBe(false);
  });
});

// O ciclo só se sustenta se o boot tentar restaurar sem ter sessão para
// restaurar: `setAccessToken(null)` já limpa a dica, e o boot precisa respeitá-la.
describe("dica de sessão", () => {
  it("some ao limpar o token (logout, refresh falho)", async () => {
    const { hasSessionHint } = await import("../services/api");
    setAccessToken("token-abc");
    expect(hasSessionHint()).toBe(true);

    setAccessToken(null);
    expect(hasSessionHint()).toBe(false);
  });
});

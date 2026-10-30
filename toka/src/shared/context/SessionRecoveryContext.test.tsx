import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SessionRecoveryProvider, useSessionRecovery } from "./SessionRecoveryContext";
import { resetSessionRefreshForTests } from "../services/sessionRefresh";

const mocks = vi.hoisted(() => ({
  getAccessToken: vi.fn<() => string | null>(() => null),
  hasSessionHint: vi.fn(() => true),
  markAuthRestoreFailed: vi.fn(),
  refreshTokenRequest: vi.fn(async () => ({ access_token: "novo", refresh_token: "r" })),
  publishDataChange: vi.fn(),
}));

vi.mock("../services/api", () => ({
  getAccessToken: mocks.getAccessToken,
  hasSessionHint: mocks.hasSessionHint,
  markAuthRestoreFailed: mocks.markAuthRestoreFailed,
  setAccessToken: vi.fn(),
}));

vi.mock("../services/auth.service", () => ({
  refreshTokenRequest: mocks.refreshTokenRequest,
}));

vi.mock("../lib/dataSync", () => ({
  publishDataChange: mocks.publishDataChange,
}));

function Probe() {
  const { isRecovering, recover, lastRecoveredAt } = useSessionRecovery();
  return (
    <div>
      <output aria-label="estado">{isRecovering ? "recuperando" : "pronto"}</output>
      <output aria-label="recuperado">{String(lastRecoveredAt > 0)}</output>
      <button onClick={() => void recover()}>recuperar</button>
    </div>
  );
}

/** JWT com vencimento controlado — só o payload importa. */
function expiringToken(expiresInMs: number): string {
  const encode = (value: unknown) =>
    btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  const exp = Math.floor((Date.now() + expiresInMs) / 1000);
  return `${encode({ alg: "ES256" })}.${encode({ exp })}.assinatura`;
}

beforeEach(() => {
  resetSessionRefreshForTests();
  vi.clearAllMocks();
  // `clearAllMocks` limpa chamadas mas NÃO os return values: sem restaurar os
  // defaults aqui, um teste que zera a dica de sessão contaminaria os seguintes.
  mocks.getAccessToken.mockReturnValue(null);
  mocks.hasSessionHint.mockReturnValue(true);
  mocks.refreshTokenRequest.mockResolvedValue({ access_token: "novo", refresh_token: "r" });
});

afterEach(() => {
  resetSessionRefreshForTests();
});

describe("SessionRecoveryProvider", () => {
  it("renova a sessão quando o token já venceu", async () => {
    mocks.getAccessToken.mockReturnValue(expiringToken(-60_000));

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    expect(mocks.refreshTokenRequest).toHaveBeenCalledTimes(1);
    // Avisa o app para recarregar usuário/permissões com o token novo.
    expect(mocks.publishDataChange).toHaveBeenCalledWith(
      expect.objectContaining({ resources: expect.arrayContaining(["permissions", "users"]) }),
      { broadcast: false },
    );
    expect(screen.getByLabelText("recuperado")).toHaveTextContent("true");
    expect(screen.getByLabelText("estado")).toHaveTextContent("pronto");
  });

  it("não renova quando o token ainda está válido", async () => {
    mocks.getAccessToken.mockReturnValue(expiringToken(30 * 60_000));

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    expect(mocks.refreshTokenRequest).not.toHaveBeenCalled();
  });

  it("renova no boot sem token em memória (aba nova com sessão gravada)", async () => {
    mocks.getAccessToken.mockReturnValue(null);
    mocks.hasSessionHint.mockReturnValue(true);

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    expect(mocks.refreshTokenRequest).toHaveBeenCalled();
  });

  it("sem dica de sessão não tenta renovar (visita nova ou logout)", async () => {
    mocks.getAccessToken.mockReturnValue(null);
    mocks.hasSessionHint.mockReturnValue(false);

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    expect(mocks.refreshTokenRequest).not.toHaveBeenCalled();
  });

  it("marca falha de restauração quando o refresh é recusado", async () => {
    mocks.getAccessToken.mockReturnValue(expiringToken(-60_000));
    mocks.refreshTokenRequest.mockRejectedValue({ response: { status: 400 } });

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    await waitFor(() => expect(mocks.markAuthRestoreFailed).toHaveBeenCalled());
    expect(screen.getByLabelText("recuperado")).toHaveTextContent("false");
  });

  it("repete com backoff quando a falha é transitória", async () => {
    mocks.getAccessToken.mockReturnValue(expiringToken(-60_000));
    mocks.refreshTokenRequest
      .mockRejectedValueOnce({ response: { status: 503 } })
      .mockResolvedValueOnce({ access_token: "novo", refresh_token: "r" });

    render(
      <SessionRecoveryProvider>
        <Probe />
      </SessionRecoveryProvider>,
    );

    await act(async () => {
      await screen.getByRole("button", { name: "recuperar" }).click();
    });

    await waitFor(() => expect(mocks.refreshTokenRequest).toHaveBeenCalledTimes(2));
    expect(mocks.markAuthRestoreFailed).not.toHaveBeenCalled();
  });

  it("compartilha a renovação entre chamadas concorrentes", async () => {
    mocks.getAccessToken.mockReturnValue(expiringToken(-60_000));
    const { SessionRecoveryProvider: Provider } = await import("./SessionRecoveryContext");
    let recoverFn: (() => Promise<boolean>) | null = null;

    function Capture() {
      recoverFn = useSessionRecovery().recover;
      return null;
    }

    render(
      <Provider>
        <Capture />
      </Provider>,
    );

    await act(async () => {
      await Promise.all([recoverFn!(), recoverFn!(), recoverFn!()]);
    });

    expect(mocks.refreshTokenRequest).toHaveBeenCalledTimes(1);
  });
});

// T-03: o provider é genérico — o portal passa o PRÓPRIO ciclo de sessão
// (refresh, hint, token e sinal de atividade) e o fluxo de recuperação usa
// exatamente essas funções, sem tocar no ciclo do app admin.
describe("SessionRecoveryProvider com ciclo customizado (T-03, portal)", () => {
  const portalRefresh = vi.fn(async () => "portal-token-novo");
  const portalHint = vi.fn(() => true);
  const portalToken = vi.fn(() => null);
  let active = false;
  const portalSubscribe = vi.fn((listener: (a: boolean) => void) => {
    return () => {};
  });

  it("usa o refresh customizado em vez do refresh do admin", async () => {
    const { SessionRecoveryProvider: Provider } = await import("./SessionRecoveryContext");
    let recoverFn: (() => Promise<boolean>) | null = null;

    function Capture() {
      recoverFn = useSessionRecovery().recover;
      return null;
    }

    render(
      <Provider
        refreshAccessToken={portalRefresh}
        hasSessionHint={portalHint}
        getAccessToken={portalToken}
        isRefreshActive={() => active}
        subscribeRefresh={portalSubscribe}
      >
        <Capture />
      </Provider>,
    );

    let ok = false;
    await act(async () => {
      ok = await recoverFn!();
    });

    expect(ok).toBe(true);
    expect(portalRefresh).toHaveBeenCalledTimes(1);
    expect(portalHint).toHaveBeenCalled();
    expect(portalToken).toHaveBeenCalled();
    expect(mocks.refreshTokenRequest).not.toHaveBeenCalled();
    expect(portalSubscribe).toHaveBeenCalled();
  });

  it("sem hint do portal não tenta renovar", async () => {
    const { SessionRecoveryProvider: Provider } = await import("./SessionRecoveryContext");
    let recoverFn: (() => Promise<boolean>) | null = null;

    function Capture() {
      recoverFn = useSessionRecovery().recover;
      return null;
    }

    render(
      <Provider
        refreshAccessToken={portalRefresh}
        hasSessionHint={() => false}
        getAccessToken={portalToken}
        isRefreshActive={() => active}
        subscribeRefresh={portalSubscribe}
      >
        <Capture />
      </Provider>,
    );

    let ok = true;
    await act(async () => {
      ok = await recoverFn!();
    });

    expect(ok).toBe(false);
    expect(portalRefresh).not.toHaveBeenCalled();
  });
});

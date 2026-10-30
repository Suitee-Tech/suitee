import { beforeEach, describe, expect, it, vi } from "vitest";
import axios from "axios";

import { setPortalAccessToken } from "./portal-api";

vi.mock("../../../lib/runtime-env", () => ({ getEnv: () => "http://localhost:8080/api/v1" }));
vi.mock("../../../shared/lib/errorDiagnostics", () => ({ reportClientError: vi.fn() }));
vi.mock("../../../shared/lib/dataSync", () => ({ publishApiMutation: vi.fn() }));

vi.mock("axios", () => {
  const post = vi.fn();
  const create = vi.fn(() => ({
    interceptors: {
      request: { use: vi.fn() },
      response: { use: vi.fn() },
    },
  }));
  return { default: { post, create }, post, create };
});

// T-03 — o refresh do portal tem o próprio single-flight (hub separado do app
// admin) e grava o token em memória ao renovar. N chamadas concorrentes fazem
// UMA troca de refresh token.
describe("refreshPortalAccessToken (T-03)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (axios.post as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { data: { access_token: "portal-token" } },
    });
    setPortalAccessToken(null);
  });

  it("renova via /auth/portal/refresh e grava o token em memória", async () => {
    const { refreshPortalAccessToken, getPortalAccessToken } = await import("./portal-api");

    const token = await refreshPortalAccessToken();

    expect(token).toBe("portal-token");
    expect(getPortalAccessToken()).toBe("portal-token");
    expect(axios.post).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/auth/portal/refresh",
      {},
      { withCredentials: true },
    );
  });

  it("é single-flight: chamadas concorrentes compartilham a mesma troca", async () => {
    const { refreshPortalAccessToken } = await import("./portal-api");

    await Promise.all([refreshPortalAccessToken(), refreshPortalAccessToken(), refreshPortalAccessToken()]);

    expect(axios.post).toHaveBeenCalledTimes(1);
  });
});

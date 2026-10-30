import { afterEach, describe, expect, it, vi } from "vitest";
import { createHttpClient } from "./httpClient";
const options = { baseURL: "/api/v1", getAccessToken: () => null, setAccessToken: () => {}, shouldRefresh: () => false, refreshAccessToken: async () => "", onRefreshFailure: () => {} };
afterEach(() => vi.unstubAllGlobals());
describe("request ID transport", () => {
  it("preserves an existing ID regardless of header casing", async () => {
    const client = createHttpClient(options);
    const id = "11111111-1111-4111-8111-111111111111";
    await client.get("/appointments", { headers: { "x-request-id": id }, adapter: async (config) => {
      expect(config.headers.get("X-Request-ID")).toBe(id);
      return { status: 200, statusText: "OK", headers: {}, data: {}, config };
    } });
  });
  it("still sends requests on local HTTP origins without crypto.randomUUID", async () => {
    vi.stubGlobal("crypto", { getRandomValues: (bytes: Uint8Array) => bytes.fill(1) });
    const client = createHttpClient(options);
    await client.put("/appointments/id", { title: "typed form" }, { adapter: async (config) => {
      expect(config.headers.get("X-Request-ID")).toMatch(/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/);
      expect(JSON.parse(config.data).title).toBe("typed form");
      return { status: 200, statusText: "OK", headers: {}, data: {}, config };
    } });
  });
});

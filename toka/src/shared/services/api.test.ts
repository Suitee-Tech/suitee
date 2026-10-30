import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchPrivateContent, normalizeStorageUrl, setAccessToken, toGatewayUrl } from "./api";

type RuntimeEnvGlobal = typeof globalThis & {
  __TOCA_ENV__?: Record<string, string | undefined>;
};

const runtimeGlobal = globalThis as RuntimeEnvGlobal;
const originalRuntimeEnv = runtimeGlobal.__TOCA_ENV__;

beforeEach(() => {
  runtimeGlobal.__TOCA_ENV__ = {
    VITE_SUPABASE_URL: "https://project.supabase.co",
  };
});

afterEach(() => {
  runtimeGlobal.__TOCA_ENV__ = originalRuntimeEnv;
});

describe("normalizeStorageUrl", () => {
  it("rewrites host.docker.internal to the public Supabase origin and preserves the signed path", () => {
    const internalUrl =
      "http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/file.pdf?token=abc123#page=2";
    const expected =
      "https://project.supabase.co/storage/v1/object/sign/toca-documents/file.pdf?token=abc123#page=2";

    expect(normalizeStorageUrl(internalUrl)).toBe(expected);
    expect(toGatewayUrl(internalUrl)).toBe(expected);
  });

  it("does not send normalized local Supabase Storage URLs through the API gateway", () => {
    runtimeGlobal.__TOCA_ENV__ = {
      VITE_SUPABASE_URL: "http://localhost:54321",
    };
    const internalUrl =
      "http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/file.pdf?token=abc";
    const expected =
      "http://localhost:54321/storage/v1/object/sign/toca-documents/file.pdf?token=abc";

    expect(toGatewayUrl(internalUrl)).toBe(expected);
    expect(toGatewayUrl(internalUrl)).not.toContain("localhost:8080/api");
  });

  it("preserves a Supabase Storage URL that already uses the configured local public origin", () => {
    runtimeGlobal.__TOCA_ENV__ = {
      VITE_SUPABASE_URL: "http://localhost:54321",
    };
    const publicUrl =
      "http://localhost:54321/storage/v1/object/public/toca-avatars/avatar.png";

    expect(normalizeStorageUrl(publicUrl)).toBe(publicUrl);
    expect(toGatewayUrl(publicUrl)).toBe(publicUrl);
  });

  it("resolves a relative Storage URL against the public Supabase origin", () => {
    const relativeUrl =
      "/storage/v1/object/sign/toca-documents/file.pdf?token=abc#page=2";
    const expected =
      "https://project.supabase.co/storage/v1/object/sign/toca-documents/file.pdf?token=abc#page=2";

    expect(normalizeStorageUrl(relativeUrl)).toBe(expected);
    expect(toGatewayUrl(relativeUrl)).toBe(expected);
  });

  it("rebases loopback Storage origins that do not match the configured public origin", () => {
    const loopbackUrl =
      "http://127.0.0.1:54321/storage/v1/object/public/toca-avatars/avatar.png";

    expect(normalizeStorageUrl(loopbackUrl)).toBe(
      "https://project.supabase.co/storage/v1/object/public/toca-avatars/avatar.png",
    );
  });

  it("rejects raw authenticated Storage routes that cannot be rendered without headers", () => {
    const privateUrl =
      "https://project.supabase.co/storage/v1/object/authenticated/toca-documents/file.pdf";

    expect(normalizeStorageUrl(privateUrl)).toBe("");
    expect(toGatewayUrl(privateUrl)).toBe("");
  });

  it("does not rewrite non-Storage URLs from host.docker.internal", () => {
    const internalApiUrl = "http://host.docker.internal:8086/api/v1/documents/document-1";

    expect(normalizeStorageUrl(internalApiUrl)).toBe(internalApiUrl);
  });

  it.each([
    "https://another-project.supabase.co/storage/v1/object/sign/toca-documents/file.pdf?token=abc",
    "https://host.docker.internal.example.com/storage/v1/object/sign/toca-documents/file.pdf?token=abc",
  ])("does not rewrite a non-matching Storage URL: %s", (url) => {
    expect(normalizeStorageUrl(url)).toBe(url);
  });

  it("leaves the internal URL untouched when the public Supabase URL is invalid", () => {
    const internalUrl =
      "http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/file.pdf?token=abc";

    runtimeGlobal.__TOCA_ENV__ = { VITE_SUPABASE_URL: "not a URL" };
    expect(normalizeStorageUrl(internalUrl)).toBe(internalUrl);
  });
});

// O conteúdo privado (anexo do chat, comprovante, documento) é buscado por
// fetch cru com `credentials: "same-origin"`: as rotas da API podem responder
// 302 para uma URL assinada do Supabase Storage, e credenciais no
// redirecionamento cross-origin são bloqueadas pelo CORS do Storage
// (Access-Control-Allow-Origin: *). O token/tenant vão por header.
describe("fetchPrivateContent", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    setAccessToken(null);
    localStorage.clear();
  });

  it("URL tokenizada é buscada como requisição simples (sem header, sem preflight)", async () => {
    setAccessToken("access-token");
    const blob = new Blob(["png"], { type: "image/png" });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key === "content-type" ? "image/png" : null) },
      blob: async () => blob,
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchPrivateContent("/api/v1/channels/c1/messages/attachments/a.png/download?token=t");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/channels/c1/messages/attachments/a.png/download?token=t",
      { credentials: "same-origin", headers: {} },
    );
    expect(result.blob).toBe(blob);
    expect(result.contentType).toBe("image/png");
  });

  it("sem token na URL, autentica pelo header Authorization", async () => {
    setAccessToken("access-token");
    const blob = new Blob(["pdf"], { type: "application/pdf" });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key === "content-type" ? "application/pdf" : null) },
      blob: async () => blob,
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchPrivateContent("/api/v1/proofs/content/abc");

    expect(fetchMock).toHaveBeenCalledWith("/api/v1/proofs/content/abc", {
      credentials: "same-origin",
      headers: { Authorization: "Bearer access-token" },
    });
    expect(result.blob).toBe(blob);
    expect(result.contentType).toBe("application/pdf");
  });

  it("falha alto quando a rota responde erro", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 403 }));

    await expect(fetchPrivateContent("/api/v1/proofs/content/abc")).rejects.toThrow(/403/);
  });
});

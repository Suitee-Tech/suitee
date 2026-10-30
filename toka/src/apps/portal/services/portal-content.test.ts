import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchPortalContent, toPortalOwnedFileUrl } from "./portal-content";

// O cliente do portal abre arquivos que ele mesmo enviou. Dois defeitos
// impediam isso: a rota do arquivo do checklist era a de operador (gate
// `deals:read`) e a busca autenticada usava o cliente do painel.
describe("toPortalOwnedFileUrl", () => {
  it("reescreve a rota de operador do checklist para a rota do portal", () => {
    expect(
      toPortalOwnedFileUrl("/api/v1/deals/ea105a98-38ee-4877-8eed-ef84b9312b0d/checklist/c80c9317-e882-4e89-b02d-185e61bc28c6/download"),
    ).toBe(
      "/api/v1/client-portal/deals/ea105a98-38ee-4877-8eed-ef84b9312b0d/checklist/c80c9317-e882-4e89-b02d-185e61bc28c6/download",
    );
  });

  it("preserva query string e outras rotas", () => {
    expect(
      toPortalOwnedFileUrl("/api/v1/deals/deal-1/checklist/item-1/download?disposition=inline"),
    ).toBe("/api/v1/client-portal/deals/deal-1/checklist/item-1/download?disposition=inline");
    expect(toPortalOwnedFileUrl("/api/v1/channels/c1/messages/attachments/a.png/download")).toBe(
      "/api/v1/channels/c1/messages/attachments/a.png/download",
    );
    expect(toPortalOwnedFileUrl("https://cdn.example.com/x.png")).toBe("https://cdn.example.com/x.png");
  });

  it("devolve vazio para entrada vazia", () => {
    expect(toPortalOwnedFileUrl(null)).toBe("");
    expect(toPortalOwnedFileUrl("   ")).toBe("");
  });
});

describe("fetchPortalContent", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("busca a URL já autenticada e devolve blob + content type", async () => {
    const blob = new Blob(["img"], { type: "image/png" });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key === "content-type" ? "image/png" : null) },
      blob: async () => blob,
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchPortalContent("/api/v1/channels/c1/messages/attachments/a.png/download?token=t");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/channels/c1/messages/attachments/a.png/download?token=t",
      // `same-origin`: a perna cross-origin (URL assinada do Storage, após o
      // 302 da rota) precisa sair SEM credenciais, senão o CORS do Storage
      // (wildcard `*`) bloqueia o download.
      { credentials: "same-origin" },
    );
    expect(result.blob).toBe(blob);
    expect(result.contentType).toBe("image/png");
  });

  it("falha alto quando a rota responde erro (o visualizador mostra o estado de erro)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 403 }));

    await expect(fetchPortalContent("/api/v1/deals/deal-1/checklist/item-1/download")).rejects.toThrow(
      /403/,
    );
  });
});

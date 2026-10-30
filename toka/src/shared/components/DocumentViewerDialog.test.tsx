import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchPrivateContentMock = vi.fn();

vi.mock("../services/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../services/api")>();
  return {
    ...actual,
    fetchPrivateContent: (...args: Parameters<typeof actual.fetchPrivateContent>) => fetchPrivateContentMock(...args),
  };
});

vi.mock("./ExpandableSidePanel", () => ({
  ExpandableSidePanel: ({
    open,
    children,
  }: {
    open: boolean;
    children: ReactNode;
  }) => (open ? <div>{children}</div> : null),
}));

import { DocumentViewerDialog } from "./DocumentViewerDialog";

type RuntimeEnvGlobal = typeof globalThis & {
  __TOCA_ENV__?: Record<string, string | undefined>;
};

const runtimeGlobal = globalThis as RuntimeEnvGlobal;
const originalRuntimeEnv = runtimeGlobal.__TOCA_ENV__;

beforeEach(() => {
  runtimeGlobal.__TOCA_ENV__ = {
    VITE_SUPABASE_URL: "http://localhost:54321",
  };
  fetchPrivateContentMock.mockReset();
  fetchPrivateContentMock.mockResolvedValue({
    blob: new Blob([new Uint8Array([0x25, 0x50, 0x44, 0x46])], { type: "application/pdf" }),
    contentType: "application/pdf",
  });
  // jsdom não implementa createObjectURL/revokeObjectURL.
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    writable: true,
    value: vi.fn(() => "blob:viewer-conteudo-privado"),
  });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, writable: true, value: vi.fn() });
});

afterEach(() => {
  cleanup();
  runtimeGlobal.__TOCA_ENV__ = originalRuntimeEnv;
});

describe("DocumentViewerDialog media URLs", () => {
  it("normalizes Supabase image URLs inside the central viewer", () => {
    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        url="http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/image.png?token=abc"
        title="Imagem assinada"
        contentType="image/png"
      />,
    );

    expect(screen.getByRole("img", { name: "Imagem assinada" })).toHaveAttribute(
      "src",
      "http://localhost:54321/storage/v1/object/sign/toca-documents/image.png?token=abc",
    );
  });

  it("does not render a raw authenticated Storage URL", () => {
    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        url="http://localhost:54321/storage/v1/object/authenticated/toca-documents/private.png"
        title="Imagem privada"
        contentType="image/png"
      />,
    );

    expect(
      screen.queryByRole("img", { name: "Imagem privada" }),
    ).not.toBeInTheDocument();
  });

  // Comprovante financeiro e arquivo da esteira são servidos por rota
  // autenticada. O react-pdf buscava a URL direto, sem o header Authorization, e
  // recebia 401 — o documento aparecia na lista e não abria. Agora a busca passa
  // pelo cliente HTTP (token + tenant) e o viewer recebe um blob URL.
  it("busca conteúdo privado da API com a sessão e renderiza pelo blob", async () => {
    localStorage.setItem("current_company_id", "tenant-1");
    const objectUrl = "blob:viewer-conteudo-privado";

    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        url={`${window.location.origin}/api/v1/proofs/content/abc`}
        title="Comprovante de pagamento"
        contentType="image/png"
      />,
    );

    // A URL é normalizada para o origin do gateway antes da busca (o viewer
    // reescreve host interno para a borda pública).
    await waitFor(() =>
      expect(fetchPrivateContentMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/proofs/content/abc")),
    );
    // O visualizador não pode voltar a apontar para a rota privada sem token:
    // renderiza o blob autenticado.
    await waitFor(() => expect(screen.getByRole("img", { name: "Comprovante de pagamento" })).toHaveAttribute("src", objectUrl));
    localStorage.removeItem("current_company_id");
  });

  // Falha na busca autenticada precisa virar mensagem, não tela vazia (o 401
  // silencioso era justamente o sintoma).
  it("mostra erro quando o conteúdo privado não pode ser baixado", async () => {
    fetchPrivateContentMock.mockRejectedValueOnce(new Error("Request failed with status code 401"));

    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        url={`${window.location.origin}/api/v1/proofs/content/negado`}
        title="Comprovante negado"
        contentType="image/png"
      />,
    );

    await waitFor(() => expect(screen.getByText("Não foi possível abrir este documento")).toBeInTheDocument());
  });

  // Storage público/assinado já é portável pelo navegador: não pode passar pelo
  // cliente autenticado (nem gastar uma chamada de API por visualização).
  it("não busca via cliente autenticado o conteúdo público do Storage", () => {
    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        url="http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/comprovante.pdf?token=abc"
        title="Comprovante assinado"
        contentType="application/pdf"
      />,
    );

    expect(fetchPrivateContentMock).not.toHaveBeenCalled();
  });

  it("não envia credenciais nem tenant a um domínio externo", async () => {
    localStorage.setItem("current_company_id", "tenant-1");
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(new Uint8Array([0x25, 0x50, 0x44, 0x46]), {
        status: 200,
        headers: { "content-type": "application/pdf" },
      }),
    );

    render(
      <DocumentViewerDialog
        open
        onOpenChange={vi.fn()}
        // Sem extensão reconhecível: é o caso em que o viewer precisa detectar
        // o formato por conta própria. Com sufixo `.pdf` ele resolveria pelo
        // nome e nem chegaria a buscar o arquivo.
        url="https://drive.google.com/file/d/abc123/view"
        title="Comprovante externo"
      />,
    );

    // Localiza a chamada do próprio comprovante: outras partes da árvore também
    // usam fetch, e assumir a primeira tornaria o teste dependente de ordem.
    await waitFor(() =>
      expect(fetchSpy.mock.calls.some(([target]) => String(target).includes("drive.google.com"))).toBe(true),
    );
    const externalCall = fetchSpy.mock.calls.find(([target]) => String(target).includes("drive.google.com"))!;
    const init = externalCall[1] as RequestInit;
    // Vazar cookie/tenant para terceiro seria um problema, e o provedor não usa.
    expect(init.credentials).toBe("omit");
    expect((init.headers as Record<string, string>)["X-Tenant-Id"]).toBeUndefined();
    localStorage.removeItem("current_company_id");
  });
});

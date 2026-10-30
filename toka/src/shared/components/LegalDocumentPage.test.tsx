import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LegalDocumentPage } from "./LegalDocumentPage";

// Os dois documentos legais se citam e o MESMO markdown é servido no painel e no
// portal. Um link absoluto (`/privacy`) levava quem lê pelo portal para a página
// do painel — outra casca, com "Voltar" para o painel.
const markdown = [
  "# Termos de Uso",
  "",
  "Veja a [Política de Privacidade](/privacy) e os [termos da DeepSeek](https://cdn.deepseek.com/terms.html).",
].join("\n");

function renderPage(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LegalDocumentPage title="Termos de Uso" documentUrl="/legal/termos-de-uso.md" version="1.0" />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({ ok: true, text: async () => markdown })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LegalDocumentPage", () => {
  it("renderiza o markdown do arquivo publicado", async () => {
    renderPage("/terms");

    expect(await screen.findByRole("heading", { name: "Termos de Uso" })).toBeInTheDocument();
    expect(screen.getByText("Versão 1.0")).toBeInTheDocument();
  });

  it("mantém a citação entre documentos no painel", async () => {
    renderPage("/terms");

    const link = await screen.findByRole("link", { name: "Política de Privacidade" });
    expect(link).toHaveAttribute("href", "/privacy");
  });

  it("traduz a citação entre documentos quando a leitura é pelo portal", async () => {
    renderPage("/portal/terms");

    const link = await screen.findByRole("link", { name: "Política de Privacidade" });
    expect(link).toHaveAttribute("href", "/portal/privacy");
  });

  it("abre link externo em outra aba", async () => {
    renderPage("/terms");

    const externo = await screen.findByRole("link", { name: "termos da DeepSeek" });
    expect(externo).toHaveAttribute("href", "https://cdn.deepseek.com/terms.html");
    expect(externo).toHaveAttribute("target", "_blank");
  });

  it("avisa quando o documento não carrega", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: false, status: 404, text: async () => "" })),
    );
    renderPage("/terms");

    expect(await screen.findByText("Não foi possível carregar este documento.")).toBeInTheDocument();
    expect(screen.getByText(/HTTP 404/)).toBeInTheDocument();
  });
});

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Invariante das páginas internas do portal.
 *
 * O relato do usuário foi: "algumas páginas internas do portal não têm header,
 * menu inferior e botão de voltar, dificultando o retorno; e o visual não é
 * consistente". Isso acontecia porque cada tela montava o próprio cabeçalho e o
 * próprio contêiner — seis delas sem cabeçalho nenhum e sem caminho de volta.
 *
 * Este teste fixa o padrão: toda página interna usa o contêiner compartilhado
 * (`PortalPage`) e o cabeçalho compartilhado (`PortalPageHeader`), e não
 * reintroduz cor fixa (`#fafafa`) nem altura que esconda a barra inferior
 * (`h-dvh`, `min-h-screen`).
 *
 * Exceções: `PortalLogin` (fora do layout autenticado) e o `PrivacyPage`.
 */
const PAGES_DIR = join(__dirname, "..", "pages");

const EXEMPT = new Set(["PortalLogin.tsx"]);

/**
 * Páginas que dispensam o botão de voltar, com motivo.
 *
 * Só a raiz: é o destino inicial da navegação, não existe "anterior" dentro do
 * app, e o cabeçalho dela é a saudação da jornada (não um título de tela).
 *
 * As demais telas — inclusive as de destino da barra inferior, como Imóveis e
 * Mensagens — precisam de voltar: o retorno à tela anterior foi pedido
 * explicitamente pelo usuário ("em explorar imóveis e em chat ainda não é
 * possível voltar facilmente").
 */
const EXEMPT_BACK = new Set(["PortalDashboard.tsx"]);

function internalPages(): string[] {
  return readdirSync(PAGES_DIR)
    .filter((name) => /^Portal.*\.tsx$/.test(name) && !name.endsWith(".test.tsx") && !EXEMPT.has(name))
    .sort();
}

describe("shell das páginas internas do portal", () => {
  const pages = internalPages();

  it("cobre as páginas internas esperadas", () => {
    // Guarda contra um refactor que renomeie a pasta e deixe o teste vazio.
    expect(pages.length).toBeGreaterThanOrEqual(15);
  });

  it.each(pages)("%s usa o contêiner compartilhado (PortalPage)", (page) => {
    const source = readFileSync(join(PAGES_DIR, page), "utf8");
    expect(source).toContain('from "../shell/PortalPage"');
    expect(source).toMatch(/<PortalPage[\s>]/);
  });

  it.each(pages)("%s tem cabeçalho com caminho de volta acessível", (page) => {
    if (EXEMPT_BACK.has(page)) {
      // Raiz do portal: sem "anterior" dentro do app.
      const source = readFileSync(join(PAGES_DIR, page), "utf8");
      expect(source).toMatch(/<header[\s>]/);
      return;
    }
    const source = readFileSync(join(PAGES_DIR, page), "utf8");
    const usesSharedHeader = source.includes('from "../shell/PortalPageHeader"');
    const hasCustomHeaderWithBack = /<header[\s\S]{0,900}?aria-label="Voltar"/.test(source);
    // Nenhuma tela pode DESLIGAR o voltar do cabeçalho compartilhado: quem
    // precisa de cabeçalho próprio (chat, saudação) prova o botão acessível.
    expect(source).not.toContain("showBack={false}");
    expect(usesSharedHeader || hasCustomHeaderWithBack).toBe(true);
  });

  it.each(pages)("%s não usa cor fixa nem altura que cubra a barra inferior", (page) => {
    const source = readFileSync(join(PAGES_DIR, page), "utf8");
    expect(source).not.toContain("#fafafa");
    // `h-dvh`/`min-h-screen` num contêiner de página estouram a altura útil e
    // deixam o rodapé atrás da barra de navegação (o `PortalPage` já desconta).
    expect(source).not.toMatch(/className="[^"]*\b(h-dvh|min-h-screen)\b/);
  });

  it("o cabeçalho compartilhado expõe um botão de voltar acessível", () => {
    const header = readFileSync(join(__dirname, "PortalPageHeader.tsx"), "utf8");
    expect(header).toContain('aria-label="Voltar"');
    expect(header).toContain("usePortalBack");
  });

  it("voltar prioriza o histórico e só então o destino padrão", () => {
    const hook = readFileSync(join(__dirname, "usePortalBack.ts"), "utf8");
    expect(hook).toContain("navigate(-1)");
    expect(hook).toContain("navigate(fallbackTo)");
  });
});

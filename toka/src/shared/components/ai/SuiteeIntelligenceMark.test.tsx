import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SuiteeIntelligenceMark } from "./SuiteeIntelligenceMark";

// A marca do produto existia em quatro desenhos diferentes (navbar, cabeçalho do
// chat, avatar do assistente e indicador "digitando"). Este teste fixa o
// contrato único: mesma moldura, glifo e estados.
describe("SuiteeIntelligenceMark", () => {
  it("desenha o brilho da marca por padrão", () => {
    render(<SuiteeIntelligenceMark />);

    const marca = screen.getByTestId("suitee-intelligence-mark");
    expect(marca.querySelector(".lucide-sparkles")).not.toBeNull();
    expect(marca.className).toContain("h-8");
  });

  it("troca o brilho pela coroa no perfil sem acesso, sem mudar a moldura", () => {
    render(<SuiteeIntelligenceMark locked />);

    const marca = screen.getByTestId("suitee-intelligence-mark");
    expect(marca.querySelector(".lucide-crown")).not.toBeNull();
    expect(marca.querySelector(".lucide-sparkles")).toBeNull();
    expect(marca.className).toContain("rounded-full");
  });

  it("respeita os três tamanhos usados no produto", () => {
    const { unmount: u1 } = render(<SuiteeIntelligenceMark size="sm" />);
    expect(screen.getByTestId("suitee-intelligence-mark").className).toContain("h-7");
    u1();
    const { unmount: u2 } = render(<SuiteeIntelligenceMark size="lg" />);
    expect(screen.getByTestId("suitee-intelligence-mark").className).toContain("h-9");
    u2();
  });

  it("no tom de destaque usa a cor primária sólida e um halo permanente", () => {
    render(<SuiteeIntelligenceMark tone="prominent" />);

    const marca = screen.getByTestId("suitee-intelligence-mark");
    // Sólido na primária (não o gradiente suave do chat) — é o que dá destaque
    // na barra, ao lado de botões de ícone comuns.
    expect(marca.className).toContain("bg-primary");
    expect(marca.className).not.toContain("from-primary/10");
    const halo = marca.querySelector('[class*="blur-md"]');
    expect(halo).not.toBeNull();
  });

  it("é decorativa: o nome acessível fica no botão que a contém", () => {
    render(<SuiteeIntelligenceMark />);

    expect(screen.getByTestId("suitee-intelligence-mark")).toHaveAttribute("aria-hidden", "true");
  });

  it("só anima quando o movimento é seguro e no estado certo", () => {
    const { unmount } = render(<SuiteeIntelligenceMark pulse="hover" />);
    const comHover = screen.getByTestId("suitee-intelligence-mark").querySelector('[class*="animate-ai-pulse"]');
    // Anel aparece no hover do container, com `motion-safe`.
    expect(comHover?.className).toContain("group-hover:opacity-100");
    expect(comHover?.className).toContain("motion-safe:group-hover:animate-ai-pulse");
    unmount();

    render(<SuiteeIntelligenceMark live pulse="always" shimmer />);
    const sempre = screen.getByTestId("suitee-intelligence-mark").querySelector('[class*="animate-ai-pulse"]');
    expect(sempre?.className).toContain("motion-safe:animate-ai-pulse");
    expect(screen.getByTestId("suitee-intelligence-mark").querySelector('[class*="animate-ai-core"]')).not.toBeNull();
  });
});

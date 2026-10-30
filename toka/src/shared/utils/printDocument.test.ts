import { afterEach, describe, expect, it, vi } from "vitest";
import { printElement, printHtml } from "./printDocument";

function lastPrintIframe(): HTMLIFrameElement | null {
  const frames = document.querySelectorAll<HTMLIFrameElement>("iframe[data-print-document]");
  return frames.length ? frames[frames.length - 1] : null;
}

describe("printDocument", () => {
  afterEach(() => {
    document.querySelectorAll("iframe[data-print-document]").forEach((frame) => frame.remove());
    vi.restoreAllMocks();
  });

  it("imprime HTML num iframe isolado com charset UTF-8", () => {
    // jsdom não implementa print(): só garantimos que a chamada não quebra.
    printHtml('<!DOCTYPE html><html lang="pt-BR"><head><meta charset="utf-8"></head><body>Recibo — Cobrança</body></html>');

    const iframe = lastPrintIframe();
    expect(iframe).not.toBeNull();
    const doc = iframe!.contentDocument!;
    expect(doc.querySelector('meta[charset="utf-8"]')).not.toBeNull();
    expect(doc.body.textContent).toContain("Recibo — Cobrança");
  });

  it("imprime um elemento reutilizando o CSS do app e a largura térmica", () => {
    const node = document.createElement("div");
    node.innerHTML = "<p>Aluguel + Encargos</p>";
    document.body.appendChild(node);

    printElement(node);

    const iframe = lastPrintIframe();
    expect(iframe).not.toBeNull();
    const doc = iframe!.contentDocument!;
    // O corpo do recibo vai dentro do wrapper térmico (80mm) — o print CSS
    // antigo reposicionava o nó dentro do dialog e saía em branco.
    expect(doc.querySelector(".print-body")).not.toBeNull();
    expect(doc.querySelector(".print-body")!.innerHTML).toContain("Aluguel + Encargos");
    expect(doc.documentElement.innerHTML).toContain("@page");

    node.remove();
  });

  it("não cria iframe quando o elemento é nulo", () => {
    printElement(null);
    expect(lastPrintIframe()).toBeNull();
  });
});

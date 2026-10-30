/**
 * Impressão confiável de documentos (recibo, relatório) sem depender de CSS
 * `@media print` frágil nem de `window.open`.
 *
 * Motivos:
 *  - CSS de impressão que esconde `body *` e reposiciona um nó dentro de um
 *    dialog (position: fixed + overflow/transform) imprime página em branco em
 *    vários navegadores — foi o defeito do "Salvar como PDF" do recibo;
 *  - `window.open` fora do gesto do usuário (dentro de setTimeout/await) é
 *    bloqueado pelo popup blocker e a exportação "não faz nada".
 *
 * O documento é montado num iframe oculto (isolado do CSS/DOM do app) e
 * impresso ali; o usuário escolhe "Salvar como PDF" na caixa nativa.
 */

const DEFAULT_PAGE_CSS = "@page { margin: 0; size: 80mm auto; }";

function collectDocumentStyles(): string {
  return Array.from(document.querySelectorAll('link[rel="stylesheet"], style'))
    .map((el) => el.outerHTML)
    .join("");
}

function printIframe(html: string): void {
  const iframe = document.createElement("iframe");
  iframe.setAttribute("aria-hidden", "true");
  iframe.setAttribute("data-print-document", "true");
  iframe.style.position = "fixed";
  iframe.style.right = "0";
  iframe.style.bottom = "0";
  iframe.style.width = "0";
  iframe.style.height = "0";
  iframe.style.border = "0";
  document.body.appendChild(iframe);

  const doc = iframe.contentDocument;
  const win = iframe.contentWindow;
  if (!doc || !win) {
    iframe.remove();
    return;
  }

  doc.open();
  doc.write(html);
  doc.close();

  const cleanup = () => {
    window.setTimeout(() => iframe.remove(), 1000);
  };
  win.onafterprint = cleanup;
  // Fallback: alguns navegadores não disparam onafterprint no iframe.
  window.setTimeout(cleanup, 60_000);

  win.focus();
  win.print();
}

/** Imprime um documento HTML completo (o `html` deve trazer <head>/charset). */
export function printHtml(html: string): void {
  printIframe(html);
}

/**
 * Imprime um elemento já renderizado reutilizando o CSS do app (tailwind) e
 * forçando a largura térmica do recibo.
 */
export function printElement(
  node: HTMLElement | null,
  options?: { pageCss?: string; extraCss?: string; bodyClass?: string },
): void {
  if (!node) return;
  const pageCss = options?.pageCss ?? DEFAULT_PAGE_CSS;
  const html = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<title>Impressão</title>
${collectDocumentStyles()}
<style>
  ${pageCss}
  html, body { background: #fff; margin: 0; padding: 0; }
  .print-body { width: 80mm; max-width: 80mm; margin: 0 auto; padding: 6mm 4mm; }
  .print-body .receipt-printable { overflow: visible !important; max-height: none !important; padding: 0 !important; }
  ${options?.extraCss ?? ""}
</style>
</head>
<body class="${options?.bodyClass ?? ""}">
<div class="print-body">${node.innerHTML}</div>
</body>
</html>`;
  printIframe(html);
}

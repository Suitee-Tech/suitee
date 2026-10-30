/**
 * Normaliza caminhos devolvidos por ferramentas/modelos da Suitee Intelligence.
 *
 * O modelo às vezes usa nomes de recurso em inglês ou rotas legadas
 * (`/properties/:id`, `/transfers/:id`). A navegação interna precisa apontar
 * para as rotas canônicas em português; links externos continuam intactos.
 */
const CANONICAL_PREFIXES: Array<[RegExp, string]> = [
  [/^\/properties(?=\/|$)/, "/imoveis"],
  [/^\/property(?=\/|$)/, "/imoveis"],
  [/^\/transfers\/[^/?#]+/, "/financeiro/repasses"],
  [/^\/transfers(?=\/|$)/, "/financeiro/repasses"],
  [/^\/transfer\/[^/?#]+/, "/financeiro/repasses"],
  [/^\/transfer(?=\/|$)/, "/financeiro/repasses"],
  [/^\/financeiro\/transfers(?=\/|$)/, "/financeiro/repasses"],
  [/^\/finance\/transfers(?=\/|$)/, "/financeiro/repasses"],
  [/^\/finance\/payouts(?=\/|$)/, "/financeiro/payouts"],
  [/^\/finance\/(?:charges|invoices)(?=\/|$)/, "/financeiro/cobrancas"],
  [/^\/finance\/commissions(?=\/|$)/, "/financeiro/comissoes"],
  [/^\/finance(?=\/|$)/, "/financeiro"],
  [/^\/deals\/([^/?#]+)/, "/esteira/deals/$1"],
  [/^\/deals(?=\/|$)/, "/esteira"],
  [/^\/deal\/([^/?#]+)/, "/esteira/deals/$1"],
  [/^\/contracts(?=\/|$)/, "/contratos"],
  [/^\/documents(?=\/|$)/, "/documentos"],
  [/^\/messages(?=\/|$)/, "/mensagens"],
  [/^\/calendar(?=\/|$)/, "/agenda"],
  [/^\/appointments(?=\/|$)/, "/agenda"],
  [/^\/charges(?=\/|$)/, "/financeiro/cobrancas"],
  [/^\/invoices(?=\/|$)/, "/financeiro/cobrancas"],
  [/^\/commissions(?=\/|$)/, "/financeiro/comissoes"],
  [/^\/taxes(?=\/|$)/, "/financeiro/impostos"],
  [/^\/reports(?=\/|$)/, "/relatorios"],
  [/^\/partners(?=\/|$)/, "/parceiros"],
  [/^\/rentals(?=\/|$)/, "/locacao"],
  [/^\/maintenance(?=\/|$)/, "/manutencao"],
  [/^\/tickets(?=\/|$)/, "/chamados"],
  [/^\/settings(?=\/|$)/, "/configuracoes"],
  [/^\/help(?=\/|$)/, "/ajuda"],
  [/^\/pipeline(?=\/|$)/, "/esteira"],
];

function isSuiteeHost(hostname: string): boolean {
  return hostname === "suitee.com.br" || hostname.endsWith(".suitee.com.br");
}

export function normalizeAiPath(input: string | null | undefined): string {
  if (!input) return "";

  let path = input.trim();
  if (!path) return "";

  if (/^https?:\/\//i.test(path)) {
    try {
      const url = new URL(path);
      if (!isSuiteeHost(url.hostname)) return path;
      path = `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return input;
    }
  }

  if (!path.startsWith("/")) return path;

  const match = path.match(/^([^?#]*)(\?[^#]*)?(#.*)?$/);
  let pathname = match?.[1] ?? path;
  const suffix = `${match?.[2] ?? ""}${match?.[3] ?? ""}`;

  for (const [pattern, replacement] of CANONICAL_PREFIXES) {
    if (pattern.test(pathname)) {
      pathname = pathname.replace(pattern, replacement);
      break;
    }
  }

  return `${pathname.replace(/\/{2,}/g, "/").replace(/\/$/, "") || "/"}${suffix}`;
}

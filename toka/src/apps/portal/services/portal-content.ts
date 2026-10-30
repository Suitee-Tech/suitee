/**
 * Acesso do CLIENTE ao próprio conteúdo no portal.
 *
 * Dois defeitos faziam as imagens/documentos que o cliente carrega no portal
 * falharem ao abrir:
 *
 * 1. O visualizador buscava a rota autenticada com o cliente do PAINEL
 *    (operadores). O token do portal não vale ali e a resposta era 401/403 —
 *    daí "o arquivo não pôde ser baixado desta rota autenticada". O portal
 *    injeta `fetchPortalContent`, que usa a própria URL (ela já carrega o token
 *    do portal na query, o mesmo que a tag <img> do chat usa com sucesso).
 *
 * 2. O arquivo do checklist era salvo com a rota de OPERADOR
 *    (`/api/v1/deals/:id/checklist/:itemId/download`, que exige `deals:read`) e
 *    o cliente não passa nesse gate. `toPortalOwnedFileUrl` reescreve para a
 *    rota do portal (`/api/v1/client-portal/...`), autorizada pela identidade
 *    do cliente no negócio.
 */

const OPS_CHECKLIST_DOWNLOAD = /^\/api\/v1\/deals\/([^/]+)\/checklist\/([^/]+)\/download/;
const PORTAL_CHECKLIST_DOWNLOAD = "/api/v1/client-portal/deals/$1/checklist/$2/download";

/** Reescreve caminhos de arquivo do negócio para a rota do portal. */
export function toPortalOwnedFileUrl(fileUrl: string | null | undefined): string {
  const raw = String(fileUrl ?? "").trim();
  if (!raw) return "";
  return raw.replace(OPS_CHECKLIST_DOWNLOAD, PORTAL_CHECKLIST_DOWNLOAD);
}

/**
 * Busca autenticada do conteúdo pelo cliente do portal. A URL resolvida pelos
 * chamadores já traz o token na query, então um fetch simples do mesmo host
 * autentica — exatamente como a prévia do chat faz.
 */
export async function fetchPortalContent(
  url: string,
): Promise<{ blob: Blob; contentType: string | null }> {
  // `same-origin` (e não `include`): a rota da API pode redirecionar para uma
  // URL assinada do Storage, e credenciais no redirecionamento cross-origin são
  // bloqueadas pelo CORS do Storage (wildcard `*`).
  const response = await fetch(url, { credentials: "same-origin" });
  if (!response.ok) {
    throw new Error(`portal content request failed with status ${response.status}`);
  }
  const blob = await response.blob();
  return {
    blob,
    contentType: response.headers.get("content-type") || blob.type || null,
  };
}

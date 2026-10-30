// Recuperação de chunk obsoleto (stale chunk) após um deploy.
//
// Quando um build novo entra no ar, um `index.html` que já está carregado no
// navegador continua apontando para nomes de chunk cujo hash não existe mais no
// servidor. A rota lazy (esteira, financeiro, leads, ...) então falha com
// "Failed to fetch dynamically imported module" / 404 em `/assets/*.js`, e o
// usuário fica sem a página até dar um hard refresh manual.
//
// A recuperação é recarregar a página para pegar o build novo. A guarda é um
// COOLDOWN (e não um flag "uma vez por sessão"): uma aba do iOS restaurada do
// back/forward cache podia manter o flag antigo e nunca mais recarregar. O
// cooldown curto continua impedindo loop infinito quando o chunk realmente não
// existe no deploy.
const RELOAD_AT_KEY = "viteChunkReloadAt";
export const STALE_CHUNK_COOLDOWN_MS = 10_000;

/** Mensagens que caracterizam falha de carregamento de chunk (não erro de app). */
export const STALE_CHUNK_ERROR_RE =
  /dynamically imported module|error loading dynamically imported module|Importing a module script failed|Failed to fetch dynamically imported module|Loading chunk \d+ failed|ChunkLoadError/i;

export function isStaleChunkError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? "");
  return STALE_CHUNK_ERROR_RE.test(message);
}

// Costura fina para o teste observar a recarga (jsdom não permite espionar
// `window.location.reload`, que é non-configurable).
export const navigation = {
  reload: () => window.location.reload(),
};

/** Recarrega a página uma vez por janela de cooldown para pegar o build novo. */
export function reloadForStaleChunk(reason: string, detail?: unknown): void {
  let last = 0;
  try {
    last = Number(sessionStorage.getItem(RELOAD_AT_KEY) ?? 0) || 0;
  } catch {
    /* private mode / storage disabled */
  }
  const now = Date.now();
  if (now - last < STALE_CHUNK_COOLDOWN_MS) {
    console.error(`[stale-chunk] reload recente (<${STALE_CHUNK_COOLDOWN_MS}ms); desistindo`, reason, detail);
    return;
  }
  try {
    sessionStorage.setItem(RELOAD_AT_KEY, String(now));
  } catch {
    /* private mode / storage disabled */
  }
  console.warn(`[stale-chunk] ${reason} — recarregando para pegar o build novo`, detail);
  navigation.reload();
}

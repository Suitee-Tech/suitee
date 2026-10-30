// Rascunho local da aba Aparência (Configurações > Aparência).
//
// O problema que isto resolve: a aba aplica as mudanças NA HORA no tema (o app
// fica escuro, a cor muda) e grava tudo no `localStorage`, mas o valor do tenant
// só é enviado ao servidor no "Salvar Alterações". Como o ThemeProvider relê a
// aparência do servidor a cada carga, um reload antes de salvar devolvia o valor
// antigo: a configuração "não persistia".
//
// Contrato escolhido: a escolha local vale até ser salva. O rascunho fica aqui,
// o ThemeProvider o aplica por cima do valor do servidor e a aba se marca como
// "alterações não salvas". Salvar limpa o rascunho — a partir daí o valor do
// tenant volta a mandar (é o que faz uma mudança do admin chegar aos usuários).
const PENDING_APPEARANCE_KEY = "suitee-appearance-pending";

/** Campos de `AppearanceSettings` ainda não enviados ao servidor. */
export type PendingAppearance = Record<string, unknown>;

export function readPendingAppearance(): PendingAppearance | null {
  try {
    const raw = localStorage.getItem(PENDING_APPEARANCE_KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    return parsed as PendingAppearance;
  } catch {
    // localStorage indisponível (modo privado) ou JSON corrompido: sem rascunho.
    return null;
  }
}

export function writePendingAppearance(value: PendingAppearance): void {
  try {
    localStorage.setItem(PENDING_APPEARANCE_KEY, JSON.stringify(value));
  } catch {
    // Sem persistência local o app segue com o valor em memória.
  }
}

export function clearPendingAppearance(): void {
  try {
    localStorage.removeItem(PENDING_APPEARANCE_KEY);
  } catch {
    // idem
  }
}

export function hasPendingAppearance(): boolean {
  return readPendingAppearance() !== null;
}

/**
 * Combina o valor que está em memória com a leitura do servidor, deixando o
 * rascunho local por último: o que o usuário acabou de escolher neste navegador
 * não é sobrescrito pela leitura do servidor (era isso que fazia a troca de tema
 * "não persistir" depois de uma recarga sem salvar).
 *
 * `pickOne` resolve campos que precisam de normalização (o tema, por exemplo,
 * só aceita light/dark/auto) — recebe o candidato já com o rascunho aplicado e o
 * valor anterior.
 */
export function mergeServerAppearance<T extends object>(
  prev: T,
  fromServer: Partial<T>,
  pending: Partial<T> | null,
  pickOne?: (candidate: unknown, previous: unknown) => unknown,
): T {
  const merged: T = { ...prev, ...fromServer, ...(pending ?? {}) };
  if (pickOne) {
    for (const key of Object.keys(fromServer) as (keyof T)[]) {
      const candidate = (pending?.[key] ?? fromServer[key]) as unknown;
      (merged as Record<string, unknown>)[key as string] = pickOne(candidate, prev[key]);
    }
  }
  return merged;
}

export { PENDING_APPEARANCE_KEY };

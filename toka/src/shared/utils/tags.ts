/**
 * Listas de etiquetas (comodidades, tags) — espelho de `pkg/validation/tags.go`.
 *
 * O formulário precisa normalizar e limitar do MESMO jeito que a API, senão o
 * operador cadastra e o servidor recusa (ou o inverso: o servidor aceita lixo
 * que o formulário deixou passar).
 */

export const MAX_TAG_ITEMS = 40;
export const MAX_TAG_RUNES = 60;

/** Limpa um item: sem espaços nas pontas, sem caractere de controle, espaços internos colapsados. */
export function normalizeTag(tag: string): string {
  return (tag ?? "")
    // Caracteres de controle (inclui \n, \r, \t) não têm lugar em um item.
    .replace(/[\u0000-\u001F\u007F]/g, " ")
    .trim()
    .replace(/\s+/g, " ");
}

/** Normaliza, descarta vazios e remove duplicatas sem diferenciar maiúsculas, preservando a ordem. */
export function normalizeTags(tags: string[]): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const raw of tags) {
    const tag = normalizeTag(raw);
    if (!tag) continue;
    const key = tag.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(tag);
  }
  return out;
}

/** Motivo pelo qual a lista seria recusada pela API; `null` quando está válida. */
export function tagListError(tags: string[], label = "Comodidades"): string | null {
  const normalized = normalizeTags(tags);
  if (normalized.length > MAX_TAG_ITEMS) {
    return `${label}: máximo de ${MAX_TAG_ITEMS} itens (recebido: ${normalized.length})`;
  }
  const tooLong = normalized.find((tag) => [...tag].length > MAX_TAG_RUNES);
  if (tooLong) {
    return `${label}: cada item pode ter até ${MAX_TAG_RUNES} caracteres (recebido: "${tooLong.slice(0, 30)}…")`;
  }
  return null;
}

/**
 * Quebra o texto digitado/colado em itens. A vírgula é o separador combinado com
 * o operador (o rótulo diz "separadas por vírgula"), então colar uma lista cria
 * vários itens de uma vez — e digitar a vírgula confirma o item em vez de virá-lo
 * parte do texto.
 */
export function splitTagDraft(draft: string): string[] {
  return (draft ?? "")
    .split(/[,\n;]/)
    .map(normalizeTag)
    .filter(Boolean);
}

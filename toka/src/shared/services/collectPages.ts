export interface ListPage<T> {
  data: T[];
  total?: number;
  meta?: { total?: number; limit?: number };
}

/** Erro de cancelamento, quando o chamador aborta a coleta em andamento. */
export class CollectionAbortedError extends Error {
  constructor() {
    super('Coleta cancelada.');
    this.name = 'CollectionAbortedError';
  }
}

/**
 * Loads the entire result before publishing a financial total. A failed or
 * unstable page rejects the whole read rather than silently dropping records.
 *
 * Ponto cego conhecido e não detectável aqui: uma substituição concorrente que
 * preserve a contagem (remove um registro e insere outro entre duas páginas)
 * passa por todos os guardas — total estável, ids únicos, comprimento correto —
 * e produz um snapshot bem formado porém incorreto. Detectar isso exigiria
 * cursor estável ou marca d'água de versão no servidor, não uma checagem no
 * cliente. Enquanto isso não existir, a garantia é "nenhuma página foi
 * perdida", não "a lista é um retrato instantâneo".
 */
export async function collectPages<T extends { id: string }>(
  fetchPage: (paging: { limit: number; offset: number }) => Promise<ListPage<T>>,
  options: { signal?: AbortSignal } = {},
): Promise<T[]> {
  const limit = 100;
  const result: T[] = [];
  const seen = new Set<string>();
  let expectedTotal: number | undefined;
  for (let pageNumber = 0; pageNumber < 10000; pageNumber++) {
    // Cancelar entre páginas evita seguir buscando uma carteira inteira depois
    // que a tela que pediu a leitura já saiu.
    if (options.signal?.aborted) throw new CollectionAbortedError();
    const page = await fetchPage({ limit, offset: result.length });
    if (options.signal?.aborted) throw new CollectionAbortedError();
    if (!Array.isArray(page.data)) throw new Error('Resposta de listagem inválida. Tente carregar novamente.');
    const total = page.meta?.total ?? page.total;
    if (total !== undefined) {
      if (!Number.isInteger(total) || total < 0 || (expectedTotal !== undefined && total !== expectedTotal)) {
        throw new Error('A lista mudou durante a consulta. Tente carregar novamente.');
      }
      expectedTotal = total;
    }
    for (const item of page.data) {
      if (!item.id || seen.has(item.id)) throw new Error('A paginação retornou registros repetidos. Tente carregar novamente.');
      seen.add(item.id);
      result.push(item);
    }
    if (expectedTotal !== undefined) {
      if (result.length === expectedTotal) return result;
      if (result.length > expectedTotal || page.data.length === 0) {
        throw new Error('Não foi possível carregar a lista completa. Tente novamente.');
      }
    } else {
      // Sem total, a única prova de que o servidor devolveu a página inteira é
      // o limite efetivo que ele mesmo informa. Assumir o limite pedido (100)
      // era o defeito: um servidor que clampa para 50 sem reportar
      // `meta.limit` devolvia 50 itens < 100 e a função devolvia a lista como
      // se estivesse completa, sem erro — truncamento silencioso em cima do
      // qual totais financeiros eram publicados.
      const effectiveLimit = page.meta?.limit;
      if (effectiveLimit === undefined) {
        throw new Error(
          'A listagem não informou o total nem o tamanho da página; não é possível garantir que a lista esteja completa. Tente novamente.',
        );
      }
      if (page.data.length < effectiveLimit) return result;
    }
  }
  throw new Error('A lista excedeu o limite de consulta. Refine os filtros.');
}

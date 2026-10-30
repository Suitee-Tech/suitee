import { useCallback, useEffect, useState } from 'react';
import {
  collectionService,
  type Collection,
  type CollectionStatus,
} from '../services/collection.service';
import { chargeService, type Charge } from '../services/charge.service';
import { unwrapApiData } from '../services/api';
import { CollectionAbortedError, collectPages } from '../services/collectPages';
import { delinquencyService, type DelinquencyDashboard } from '../services/delinquency.service';
import { serverMessage } from '../lib/serverMessage';
import { useRequestScopeGuard } from './useRequestScopeGuard';

const OVERVIEW_FALLBACK = 'Não foi possível carregar os indicadores de inadimplência.';
const PAGE_FALLBACK = 'Não foi possível carregar os casos de inadimplência.';
const OVERDUE_CHARGES_FALLBACK = 'Não foi possível carregar as cobranças vencidas.';
const OPEN_CASES_FALLBACK = 'Não foi possível conferir quais cobranças já têm caso aberto.';

export const EMPTY_DASHBOARD: DelinquencyDashboard = {
  stats: null,
  aging_report: null,
  collection_stats: null,
};

/**
 * Indicadores de inadimplência vindos do SERVIDOR (`GET /delinquency/dashboard`:
 * stats + aging_report + collection_stats). Nenhum KPI é derivado da lista
 * paginada nem do termo de busca — por isso o número não muda quando o usuário
 * filtra a tabela.
 */
export function useDelinquencyOverview(enabled = true) {
  const beginScopedRequest = useRequestScopeGuard('delinquency-overview');
  const [dashboard, setDashboard] = useState<DelinquencyDashboard>(EMPTY_DASHBOARD);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const isCurrent = beginScopedRequest();
    setLoading(true);
    setError(null);
    void delinquencyService.getDashboard().then(
      (data) => {
        if (!isCurrent()) return;
        setDashboard(data ?? EMPTY_DASHBOARD);
        setLoading(false);
      },
      (err) => {
        if (!isCurrent()) return;
        setError(serverMessage(err, OVERVIEW_FALLBACK));
        setLoading(false);
      },
    );
  }, [beginScopedRequest, enabled, attempt]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  return { dashboard, loading, error, reload };
}

export type CollectionsPageState = {
  items: Collection[];
  total: number;
  totalPages: number;
  loading: boolean;
  error: string | null;
  reload: () => void;
  replace: (collection: Collection) => void;
};

/**
 * Uma página real de casos (`GET /collections?limit&offset&status`), com o
 * `total` e o `totalPages` informados pelo servidor. O filtro de status é
 * aplicado NO SERVIDOR — é o que faz as abas Cobrança/Garantias/Judicial
 * filtrarem de verdade em vez de só trocarem o rótulo.
 */
export function useCollectionsPage(params: {
  status?: CollectionStatus;
  page: number;
  pageSize: number;
  enabled?: boolean;
}): CollectionsPageState {
  const { status, page, pageSize, enabled = true } = params;
  const beginScopedRequest = useRequestScopeGuard(`${status ?? 'ALL'}:${page}:${pageSize}`);
  const [items, setItems] = useState<Collection[]>([]);
  const [meta, setMeta] = useState({ total: 0, totalPages: 1 });
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const isCurrent = beginScopedRequest();
    setLoading(true);
    setError(null);
    void collectionService
      .listPage({ status, limit: pageSize, offset: (page - 1) * pageSize })
      .then(
        (result) => {
          if (!isCurrent()) return;
          setItems(result.items);
          // A página exibida é a SOLICITADA (`offset`), não o eco do servidor:
          // misturar as duas fazia "Próxima" pular páginas.
          setMeta({ total: result.total, totalPages: result.totalPages });
          setLoading(false);
        },
        (err) => {
          if (!isCurrent()) return;
          setError(serverMessage(err, PAGE_FALLBACK));
          setItems([]);
          setLoading(false);
        },
      );
  }, [attempt, beginScopedRequest, enabled, page, pageSize, status]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  /** Substitui o caso pelo estado que o SERVIDOR devolveu após uma mutação. */
  const replace = useCallback((collection: Collection) => {
    setItems((prev) => prev.map((item) => (item.id === collection.id ? collection : item)));
  }, []);

  return {
    items,
    total: meta.total,
    totalPages: meta.totalPages,
    loading,
    error,
    reload,
    replace,
  };
}

/**
 * Caso completo (`GET /collections/:id`), com `actions[]`, `guarantee`,
 * `judicial` e `charge_ids` — é a fonte do painel de detalhe, não a linha
 * resumida da tabela.
 */
export function useCollectionCase(id: string | null, enabled = true) {
  const beginScopedRequest = useRequestScopeGuard(id ?? '');
  const [collection, setCollection] = useState<Collection | null>(null);
  const [loading, setLoading] = useState(Boolean(id));
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!id || !enabled) {
      setCollection(null);
      setLoading(false);
      setError(null);
      return;
    }
    const isCurrent = beginScopedRequest();
    setLoading(true);
    setError(null);
    void collectionService.getById(id).then(
      (data) => {
        if (!isCurrent()) return;
        setCollection(data);
        setLoading(false);
      },
      (err) => {
        if (!isCurrent()) return;
        setError(serverMessage(err, 'Não foi possível carregar o caso de cobrança.'));
        setCollection(null);
        setLoading(false);
      },
    );
  }, [attempt, beginScopedRequest, enabled, id]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  return { collection, loading, error, reload, setCollection };
}

export type OverdueChargesState = {
  charges: Charge[];
  loading: boolean;
  error: string | null;
  reload: () => void;
};

/**
 * Cobranças vencidas (`GET /charges/overdue`, permissão `charges:read`) — a
 * fonte da faixa "sem caso aberto" da aba Inadimplentes.
 *
 * É o que dá efeito visível ao processamento: o caso de cobrança só é criado
 * depois do limiar de dias de atraso, então entre o vencimento e o limiar a
 * lista de casos fica vazia e o operador não via nada. O `days_overdue` que o
 * backend devolve em cada item é a régua exibida — o cliente não recalcula.
 */
export function useOverdueCharges(enabled = true): OverdueChargesState {
  const beginScopedRequest = useRequestScopeGuard('overdue-charges');
  const [charges, setCharges] = useState<Charge[]>([]);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const isCurrent = beginScopedRequest();
    setLoading(true);
    setError(null);
    void chargeService.getOverdue().then(
      (response) => {
        if (!isCurrent()) return;
        const data = unwrapApiData<Charge[]>(response);
        setCharges(Array.isArray(data) ? data : []);
        setLoading(false);
      },
      (err) => {
        if (!isCurrent()) return;
        setError(serverMessage(err, OVERDUE_CHARGES_FALLBACK));
        setCharges([]);
        setLoading(false);
      },
    );
  }, [attempt, beginScopedRequest, enabled]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  return { charges, loading, error, reload };
}

export type OpenCaseChargeIdsState = {
  /** IDs das cobranças que JÁ estão dentro de um caso não resolvido. */
  chargeIds: Set<string>;
  loading: boolean;
  error: string | null;
  reload: () => void;
};

/**
 * IDs de cobrança vinculados a caso ABERTO (`GET /collections`, paginado).
 *
 * A régua de "sem caso aberto" precisa da carteira inteira, não só da página
 * visível: um caso na página 2 esconderia a cobrança dele na faixa e o operador
 * veria uma cobrança que já tem caso. Caso RESOLVIDO não conta como aberto — se
 * uma cobrança dele continuar vencida, ela precisa aparecer.
 *
 * Só deve ser ligado quando há cobranças vencidas a conferir: sem elas não há
 * o que cruzar e a varredura seria tráfego à toa.
 */
export function useOpenCaseChargeIds(enabled = true): OpenCaseChargeIdsState {
  const beginScopedRequest = useRequestScopeGuard('open-case-charge-ids');
  const [chargeIds, setChargeIds] = useState<Set<string>>(() => new Set());
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const isCurrent = beginScopedRequest();
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void collectPages<Collection>(
      async ({ limit, offset }) => {
        const page = await collectionService.listPage({ limit, offset });
        return { data: page.items, total: page.total };
      },
      { signal: controller.signal },
    ).then(
      (collections) => {
        if (!isCurrent()) return;
        setChargeIds(
          new Set(
            collections
              .filter((item) => item.status !== 'RESOLVED')
              .flatMap((item) => item.charge_ids ?? []),
          ),
        );
        setLoading(false);
      },
      (err) => {
        if (!isCurrent() || err instanceof CollectionAbortedError) return;
        setError(serverMessage(err, OPEN_CASES_FALLBACK));
        setLoading(false);
      },
    );
    return () => controller.abort();
  }, [attempt, beginScopedRequest, enabled]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  return { chargeIds, loading, error, reload };
}

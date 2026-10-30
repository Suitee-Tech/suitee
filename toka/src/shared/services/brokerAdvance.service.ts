import api, { unwrapApiData } from './api';

/** `OPEN` = saldo a recuperar; `SETTLED` = quitado pelos abatimentos; `CANCELLED` = estornado sem abatimento. */
export type BrokerAdvanceStatus = 'OPEN' | 'SETTLED' | 'CANCELLED';

/** Cada abatimento aponta para o desconto (`kind: "ADVANCE"`) que consumiu o saldo. */
export interface BrokerAdvanceSettlement {
  commission_id: string;
  deduction_id: string;
  contract_id?: string;
  amount_cents: number;
  at: string;
}

export interface BrokerAdvanceHistoryEntry {
  at: string;
  action: string;
  actor?: string;
  note?: string;
}

export interface BrokerAdvance {
  id: string;
  tenant_id: string;
  broker_id: string;
  contract_id?: string;
  deal_id?: string;
  amount_cents: number;
  settled_cents: number;
  status: BrokerAdvanceStatus;
  description?: string;
  granted_at: string;
  /** Lançamento da concessão (débito a receber do corretor / crédito caixa). */
  journal_entry_id?: string;
  settlements?: BrokerAdvanceSettlement[];
  history?: BrokerAdvanceHistoryEntry[];
  created_at: string;
  updated_at: string;
}

/**
 * Saldo agregado por corretor, calculado no SERVIDOR (`meta.balances`). É
 * independente do filtro/paginação da lista: os cartões da tela precisam do
 * total do tenant, não da soma da página.
 */
export interface BrokerBalance {
  broker_id: string;
  open_count: number;
  outstanding_cents: number;
  settled_cents: number;
}

export interface BrokerAdvanceListFilter {
  broker_id?: string;
  /** Um status, vários, ou vazio para "todos". */
  status?: BrokerAdvanceStatus | BrokerAdvanceStatus[] | '';
  /** Somente adiantamentos com saldo em aberto. */
  open?: boolean;
  limit?: number;
}

export interface BrokerAdvanceListResult {
  items: BrokerAdvance[];
  balances: BrokerBalance[];
}

export interface CreateBrokerAdvanceInput {
  broker_id: string;
  /** Valor em reais — o backend converte para centavos. */
  amount: number;
  description?: string;
  /** Data do adiantamento no formato AAAA-MM-DD. */
  granted_at?: string;
}

/**
 * Saldo em aberto de UM adiantamento (`amount_cents - settled_cents`).
 * Negativo não existe no domínio: o backend satura em zero.
 */
export const advanceOutstandingCents = (advance: Pick<BrokerAdvance, 'amount_cents' | 'settled_cents'>): number =>
  Math.max((advance.amount_cents ?? 0) - (advance.settled_cents ?? 0), 0);

/** Soma o saldo em aberto de um retrato por corretor (`meta.balances`). */
export const totalOutstandingCents = (balances: BrokerBalance[] | undefined): number =>
  (balances ?? []).reduce((sum, entry) => sum + (entry.outstanding_cents ?? 0), 0);

/** Soma o já abatido de um retrato por corretor (`meta.balances`). */
export const totalSettledCents = (balances: BrokerBalance[] | undefined): number =>
  (balances ?? []).reduce((sum, entry) => sum + (entry.settled_cents ?? 0), 0);

/** Serializa listas como `a,b` — o backend aceita repetição ou vírgula. */
const listParam = (value: string | string[] | undefined): string | undefined => {
  if (value === undefined || value === null) return undefined;
  const joined = (Array.isArray(value) ? value : [value]).filter(Boolean).join(',');
  return joined === '' ? undefined : joined;
};

const parseList = (response: any): BrokerAdvanceListResult => {
  const body = response?.data ?? response;
  return {
    items: body?.data ?? [],
    balances: body?.meta?.balances ?? [],
  };
};

export const brokerAdvanceService = {
  /**
   * Lista os adiantamentos do tenant com o saldo por corretor. `meta.balances`
   * vem do servidor e NÃO respeita o filtro — é o retrato geral do tenant.
   */
  list: async (filter: BrokerAdvanceListFilter = {}): Promise<BrokerAdvanceListResult> => {
    const params = new URLSearchParams();
    if (filter.broker_id) params.append('broker_id', filter.broker_id);
    const status = listParam(filter.status);
    if (status) params.append('status', status);
    if (filter.open) params.append('open', 'true');
    if (filter.limit !== undefined) params.append('limit', String(filter.limit));
    const qs = params.toString();

    const response = await api.get<any>(`/broker-advances${qs ? `?${qs}` : ''}`);
    return parseList(response);
  },

  /** Registra o adiantamento. O caixa é debitado no servidor (valor a receber do corretor). */
  create: async (input: CreateBrokerAdvanceInput): Promise<BrokerAdvance> => {
    const response = await api.post<any>('/broker-advances', input);
    return unwrapApiData<BrokerAdvance>(response);
  },

  /**
   * Cancela o adiantamento. O backend responde 422 quando já existe abatimento
   * em comissão — desfazer os descontos é pré-requisito.
   */
  cancel: async (id: string, reason?: string): Promise<BrokerAdvance> => {
    const response = await api.post<any>(`/broker-advances/${id}/cancel`, { reason });
    return unwrapApiData<BrokerAdvance>(response);
  },

  /**
   * Saldo em aberto do corretor, somado a partir dos adiantamentos ABERTOS dele
   * (`open=true`). Usado pelo editor de descontos da comissão para avisar antes
   * de o backend recusar o lançamento que excede o saldo.
   *
   * Não dá para ler isto de `meta.balances`: esse retrato é do tenant inteiro e
   * traria o saldo de outros corretores junto.
   */
  outstandingForBroker: async (brokerId: string): Promise<{ outstanding_cents: number; open_count: number }> => {
    const { items } = await brokerAdvanceService.list({ broker_id: brokerId, open: true, limit: 100 });
    return {
      outstanding_cents: items.reduce((sum, advance) => sum + advanceOutstandingCents(advance), 0),
      open_count: items.length,
    };
  },
};

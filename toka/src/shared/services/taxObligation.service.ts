import api, { unwrapApiData } from './api';

/** Tipos de retenção aceitos — espelham `commission.DeductionKind`. */
export type TaxObligationKind = 'IRRF' | 'INSS' | 'ISS' | 'OUTROS';

/** `OPEN` = retido e ainda não recolhido; `CANCELLED` = sem itens (descontos removidos). */
export type TaxObligationStatus = 'OPEN' | 'PAID' | 'CANCELLED';

/**
 * Item da obrigação: aponta para o desconto da comissão que a originou. A
 * obrigação é DERIVADA — nenhum valor é digitado na tela de tributos.
 */
export interface TaxObligationItem {
  commission_id: string;
  deduction_id: string;
  contract_id?: string;
  broker_id?: string;
  amount_cents: number;
  /** Competência do lançamento do desconto (AAAA-MM). */
  competence: string;
  at: string;
}

export interface TaxObligationHistoryEntry {
  at: string;
  action: string;
  actor?: string;
  note?: string;
}

export interface TaxObligation {
  id: string;
  tenant_id: string;
  kind: TaxObligationKind;
  /** Competência (AAAA-MM) — o mês do lançamento do desconto. */
  competence: string;
  /** Acumulado da competência, em centavos. */
  amount_cents: number;
  status: TaxObligationStatus;
  /** Vencimento em RFC3339. */
  due_date: string;
  items?: TaxObligationItem[];
  paid_at?: string;
  paid_by?: string;
  proof_url?: string;
  proof_filename?: string;
  note?: string;
  /** Lançamento do recolhimento (débito 2.1.05 / crédito caixa). */
  journal_entry_id?: string;
  history?: TaxObligationHistoryEntry[];
  created_at: string;
  updated_at: string;
}

/** Retrato por tipo de tributo dentro do filtro corrente. */
export interface TaxObligationKindTotals {
  open_count: number;
  open_amount_cents: number;
  paid_amount_cents: number;
}

/**
 * Totais calculados no SERVIDOR para o filtro corrente, independentes da
 * paginação: a tela precisa do total a recolher, não da soma da página.
 */
export interface TaxObligationTotals {
  open_count: number;
  open_amount_cents: number;
  overdue_count: number;
  overdue_amount_cents: number;
  paid_count: number;
  paid_amount_cents: number;
  by_kind: Partial<Record<TaxObligationKind, TaxObligationKindTotals>>;
}

export interface TaxObligationListFilter {
  /** Um status, vários, ou vazio para "todos". */
  status?: TaxObligationStatus | TaxObligationStatus[] | '';
  kind?: TaxObligationKind | TaxObligationKind[] | '';
  /** Competência no formato AAAA-MM. */
  competence?: string;
  /** Somente vencidas. */
  overdue?: boolean;
  /** Vencimento até a data (AAAA-MM-DD). */
  due_before?: string;
  limit?: number;
  offset?: number;
}

export interface TaxObligationListResult {
  items: TaxObligation[];
  totals: TaxObligationTotals;
  /** Vencidas dentro da página devolvida (o resto está em `totals`). */
  overdue_in_page: number;
}

export interface SettleTaxObligationInput {
  /** Obrigatório: sem comprovante do recolhimento não há baixa. */
  proof_url: string;
  proof_filename?: string;
  note?: string;
}

/** Serializa listas como `a,b` — o backend aceita repetição ou vírgula. */
const listParam = (value: string | string[] | undefined): string | undefined => {
  if (value === undefined || value === null) return undefined;
  const joined = (Array.isArray(value) ? value : [value]).filter(Boolean).join(',');
  return joined === '' ? undefined : joined;
};

export const taxObligationService = {
  /**
   * Lista os tributos retidos a recolher. Os totais vêm do servidor e já
   * respeitam o filtro — não somar a página no cliente.
   */
  list: async (filter: TaxObligationListFilter = {}): Promise<TaxObligationListResult> => {
    const params = new URLSearchParams();
    const status = listParam(filter.status);
    const kind = listParam(filter.kind);
    if (status) params.append('status', status);
    if (kind) params.append('kind', kind);
    if (filter.competence) params.append('competence', filter.competence);
    if (filter.overdue) params.append('overdue', 'true');
    if (filter.due_before) params.append('due_before', filter.due_before);
    if (filter.limit !== undefined) params.append('limit', String(filter.limit));
    if (filter.offset !== undefined) params.append('offset', String(filter.offset));
    const qs = params.toString();

    const response = await api.get<any>(`/tax-obligations${qs ? `?${qs}` : ''}`);
    const body = response.data ?? response;
    return {
      items: body?.data ?? [],
      totals: body?.meta?.totals ?? {
        open_count: 0,
        open_amount_cents: 0,
        overdue_count: 0,
        overdue_amount_cents: 0,
        paid_count: 0,
        paid_amount_cents: 0,
        by_kind: {},
      },
      overdue_in_page: body?.meta?.overdue_in_page ?? 0,
    };
  },

  getById: async (id: string): Promise<TaxObligation> => {
    const response = await api.get<any>(`/tax-obligations/${id}`);
    return unwrapApiData<TaxObligation>(response);
  },

  /** Dá baixa no recolhimento. O comprovante é exigido pelo backend. */
  settle: async (id: string, input: SettleTaxObligationInput): Promise<TaxObligation> => {
    const response = await api.post<any>(`/tax-obligations/${id}/settle`, input);
    return unwrapApiData<TaxObligation>(response);
  },

  /**
   * Ferramenta de reparo: reconstrói as obrigações a partir dos descontos das
   * comissões. Sem `commissionId`, reprocessa as comissões recentes do tenant.
   */
  recompute: async (commissionId?: string): Promise<{ commissions_processed: number }> => {
    const response = await api.post<any>(
      '/tax-obligations/recompute',
      commissionId ? { commission_id: commissionId } : {},
    );
    const body = unwrapApiData<{ commissions_processed?: number }>(response);
    return { commissions_processed: body?.commissions_processed ?? 0 };
  },
};

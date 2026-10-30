import api, { unwrapApiData } from './api';

/**
 * KPIs de inadimplência calculados no SERVIDOR (`GET /delinquency/stats` e
 * `GET /delinquency/dashboard`). Espelha `charge.DelinquencyStats` do
 * toca-financial — a tela nunca soma a lista paginada para produzir KPI.
 */
export interface DelinquencyStats {
  total_overdue: number;
  total_debt_amount: number;
  average_debt_amount: number;
  average_days_overdue: number;
  active_collections: number;
  recovery_rate: number;
}

/** Faixa do relatório de aging — espelha `charge.AgingBucket`. */
export interface AgingBucket {
  label: string;
  min_days: number;
  max_days: number;
  count: number;
  total_debt: number;
}

/** Estatísticas por status do caso — espelha `collection.CollectionStats`. */
export interface CollectionStats {
  active: number;
  negotiated: number;
  guarantee_activated: number;
  judicial: number;
  resolved: number;
  total_debt: number;
  recovered_amount: number;
}

/**
 * Configuração EFETIVA da régua de cobrança que o backend aplicou, devolvida
 * junto com o dashboard (`GET /delinquency/dashboard`).
 *
 * Por que existe: o limiar de dias para o caso nascer automaticamente era um
 * número espelhado no cliente (`AUTO_CREATE_CASE_AFTER_DAYS = 5`) — quando o
 * tenant configura outro valor, a tela explicava a regra errada. Aqui vem o
 * valor realmente usado pelo servidor; `source` diz de onde ele saiu.
 */
export interface DelinquencyConfig {
  /** Dias de atraso a partir dos quais o caso de cobrança é criado sozinho. */
  auto_create_collection_after_days: number;
  /** Intervalo do processamento automático da régua, em minutos. */
  process_interval_minutes: number;
  /** Multa por atraso (%) aplicada pela régua. */
  late_fee_percent: number;
  /** Juros mensais (%) aplicados pela régua. */
  monthly_interest_percent: number;
  /** `tenant`: valor configurado pela imobiliária; `global`: padrão da plataforma. */
  source: 'tenant' | 'global';
}

export interface DelinquencyDashboard {
  stats: DelinquencyStats | null;
  aging_report: AgingBucket[] | null;
  collection_stats: CollectionStats | null;
  /**
   * Opcional: payload antigo (ou servidor anterior a este contrato) não traz o
   * bloco e a tela usa o fallback documentado — nunca um número "inventado".
   */
  config?: DelinquencyConfig | null;
}

/** Resultado de `POST /delinquency/process` — espelha `service.ProcessResult`. */
export interface DelinquencyProcessResult {
  charges_marked_overdue: number;
  /**
   * Cobranças cuja marcação de atraso foi DESFEITA por não estarem vencidas na
   * regra de dia civil (vence hoje não é atraso). Opcional: só o backend que já
   * corrigiu a régua devolve o campo.
   */
  charges_reverted?: number;
  late_fees_calculated: number;
  collections_created: number;
  collections_updated: number;
  processed_at: string;
}

class DelinquencyService {
  private readonly baseUrl = '/delinquency';

  async getDashboard(): Promise<DelinquencyDashboard> {
    const response = await api.get<{ data: DelinquencyDashboard }>(`${this.baseUrl}/dashboard`);
    return unwrapApiData<DelinquencyDashboard>(response.data) ?? { stats: null, aging_report: null, collection_stats: null };
  }

  async getAging(): Promise<AgingBucket[]> {
    const response = await api.get<{ data: AgingBucket[] }>(`${this.baseUrl}/aging`);
    return unwrapApiData<AgingBucket[]>(response.data) ?? [];
  }

  async getStats(): Promise<DelinquencyStats | null> {
    const response = await api.get<{ data: DelinquencyStats }>(`${this.baseUrl}/stats`);
    return unwrapApiData<DelinquencyStats>(response.data) ?? null;
  }

  /** Dispara o processamento manual da régua (exige `charges:write`). */
  async process(): Promise<DelinquencyProcessResult> {
    const response = await api.post<{ data: DelinquencyProcessResult }>(`${this.baseUrl}/process`);
    return unwrapApiData<DelinquencyProcessResult>(response.data);
  }
}

export const delinquencyService = new DelinquencyService();

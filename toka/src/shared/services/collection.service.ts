import api, { unwrapApiData } from './api';

/** Status do caso de cobrança — espelha `collection.Status` do toca-financial. */
export type CollectionStatus =
  | 'ACTIVE'
  | 'NEGOTIATED'
  | 'GUARANTEE_ACTIVATED'
  | 'JUDICIAL'
  | 'RESOLVED';

/** Andamento da ação judicial — espelha `collection.JudicialStatus`. */
export type JudicialStatus =
  | 'EM_TRAMITE'
  | 'ACORDO'
  | 'SENTENCA_PROCEDENTE'
  | 'SENTENCA_IMPROCEDENTE'
  | 'ARQUIVADO';

/** Modalidade da garantia acionada — espelha `collection.GuaranteeType`. */
export type GuaranteeType = 'SEGURO_FIANCA' | 'FIADOR' | 'CAUCAO';

/** Estágio do acionamento — espelha `collection.GuaranteeActivationStatus`. */
export type GuaranteeActivationStatus =
  | 'REQUESTED'
  | 'ACCEPTED'
  | 'REJECTED'
  | 'PAID'
  | 'CANCELLED';

/**
 * Estágio do acordo — espelha `collection.AgreementStatus`.
 * ACTIVE é o acordo em andamento; FULFILLED foi cumprido; BROKEN foi quebrado
 * (o caso volta para ACTIVE) e CANCELLED foi cancelado antes de produzir efeito.
 */
export type AgreementStatus = 'ACTIVE' | 'FULFILLED' | 'BROKEN' | 'CANCELLED';

/**
 * Desfecho do processo judicial — espelha `collection.JudicialStatus` menos o
 * andamento (`EM_TRAMITE`), que não é desfecho.
 */
export type JudicialOutcomeStatus = 'ACORDO' | 'SENTENCA_PROCEDENTE' | 'SENTENCA_IMPROCEDENTE' | 'ARQUIVADO';

export interface CollectionAction {
  date: string;
  type: string;
  description: string;
  user_id: string;
}

export interface JudicialLawyer {
  name: string;
  oab?: string;
  email?: string;
  phone?: string;
}

export interface JudicialProcess {
  case_number: string;
  court: string;
  lawyer: JudicialLawyer;
  claim_value: number;
  status: JudicialStatus;
  next_hearing?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface GuaranteeActivation {
  type: GuaranteeType;
  amount: number;
  reason?: string;
  status: GuaranteeActivationStatus;
  insurer_name?: string;
  insurer_protocol?: string;
  insurer_response?: string;
  requested_at: string;
  resolved_at?: string;
  updated_at: string;
  /** Indenização efetivamente recebida — informada na baixa por garantia (`PAID`). */
  settled_amount?: number;
  /** Protocolo/referência do recebimento da indenização. */
  settlement_reference?: string;
  /**
   * Dedução lançada na caução do contrato quando a garantia é CAUCAO
   * (`settled_amount` sai do depósito). Opcional: só existe nesse caso.
   */
  security_deposit_deduction_id?: string;
}

/** Custa/honorário do processo — vira despesa do contrato no desfecho. */
export interface JudicialCost {
  description: string;
  amount: number;
}

/**
 * Acordo do caso. O acordo CANCELA as cobranças originais (`original_charge_ids`)
 * e cria as parcelas como cobranças novas (`installment_charge_ids`).
 */
export interface CollectionAgreement {
  id: string;
  collection_id: string;
  status: AgreementStatus;
  original_total: number;
  total_amount: number;
  discount: number;
  down_payment: number;
  installments: number;
  interval_days: number;
  first_due_date: string;
  original_charge_ids: string[];
  installment_charge_ids: string[];
  created_at: string;
  created_by: string;
  notes?: string;
  broken_at?: string | null;
  break_reason?: string;
  approval_required: boolean;
  approved_by?: string;
}

export interface Collection {
  id: string;
  tenant_id: string;
  contract_id: string;
  debtor_id: string;
  debtor_name: string;
  total_debt: number;
  days_overdue: number;
  charge_ids: string[];
  status: CollectionStatus;
  actions: CollectionAction[];
  negotiated_amount?: number;
  negotiated_date?: string;
  notes?: string;
  judicial?: JudicialProcess;
  guarantee?: GuaranteeActivation;
  /** Acordo vigente/quebrado do caso, quando já existe (`GET /agreement`). */
  agreement?: CollectionAgreement;
  /** Dedução da caução gerada pela baixa por garantia (só para CAUCAO). */
  security_deposit_deduction_id?: string;
  created_at: string;
  updated_at: string;
}

/** Página de casos com o `meta` de paginação que o servidor devolve. */
export interface CollectionPage {
  items: Collection[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
}

export interface CreateCollectionDTO {
  contract_id: string;
  debtor_id: string;
  debtor_name: string;
  total_debt: number;
  days_overdue: number;
  charge_ids: string[];
  notes?: string;
}

export interface UpdateCollectionDTO {
  total_debt?: number;
  days_overdue?: number;
  status?: CollectionStatus;
  negotiated_amount?: number;
  negotiated_date?: string;
  notes?: string;
  debtor_name?: string;
}

/**
 * `user_id` é derivado do token no backend e NÃO é enviado pelo cliente:
 * o servidor usa a identidade do ator autenticado.
 */
export interface AddActionDTO {
  type: string;
  description: string;
}

/** Corpo de `POST /collections/:id/judicial` (`case_number` e `court` obrigatórios). */
export interface SetJudicialDTO {
  case_number: string;
  court: string;
  lawyer_name?: string;
  lawyer_oab?: string;
  lawyer_email?: string;
  lawyer_phone?: string;
  claim_value?: number;
  status?: JudicialStatus;
  next_hearing?: string;
  notes?: string;
}

/** Corpo de `POST /collections/:id/guarantee` (`amount` > 0). */
export interface ActivateGuaranteeDTO {
  type: GuaranteeType;
  amount: number;
  reason?: string;
}

/** Corpo de `PUT /collections/:id/guarantee/status`. */
export interface UpdateGuaranteeStatusDTO {
  status: GuaranteeActivationStatus;
  insurer_response?: string;
  /** Obrigatório quando `status: 'PAID'` — indenização efetivamente recebida (> 0). */
  settled_amount?: number;
  /** Protocolo/referência do recebimento (opcional). */
  settlement_reference?: string;
}

/** Corpo de `POST /collections/:id/agreement`. */
export interface CreateAgreementDTO {
  total_amount: number;
  discount?: number;
  down_payment?: number;
  installments: number;
  first_due_date: string;
  interval_days?: number;
  notes?: string;
}

/** Corpo de `POST /collections/:id/agreement/break`. */
export interface BreakAgreementDTO {
  reason: string;
  /** `true` faz o desconto concedido deixar de valer (volta ao valor original). */
  restore_discount?: boolean;
}

/** Corpo de `POST /collections/:id/agreement/cancel`. */
export interface CancelAgreementDTO {
  reason: string;
}

/** Corpo de `POST /collections/:id/judicial/outcome`. */
export interface RegisterJudicialOutcomeDTO {
  status: JudicialOutcomeStatus;
  decided_at: string;
  notes?: string;
  costs?: JudicialCost[];
}

type Envelope<T> = { data?: T; meta?: { page?: number; limit?: number; total?: number; totalPages?: number } };

const toCollectionPage = (envelope: Envelope<Collection[]>, fallbackLimit: number): CollectionPage => {
  const items = unwrapApiData<Collection[]>(envelope) ?? [];
  const meta = envelope?.meta;
  const limit = typeof meta?.limit === 'number' && meta.limit > 0 ? meta.limit : fallbackLimit;
  const total = typeof meta?.total === 'number' ? meta.total : items.length;
  const page = typeof meta?.page === 'number' && meta.page > 0 ? meta.page : 1;
  const totalPages =
    typeof meta?.totalPages === 'number' ? meta.totalPages : limit > 0 ? Math.max(1, Math.ceil(total / limit)) : 1;
  return { items, total, page, limit, totalPages };
};

class CollectionService {
  private readonly baseUrl = '/collections';

  /** Listagem paginada real (`limit`/`offset`/`status`), com total do servidor. */
  async listPage(params: { status?: CollectionStatus; limit: number; offset: number }): Promise<CollectionPage> {
    const response = await api.get<Envelope<Collection[]>>(this.baseUrl, { params });
    return toCollectionPage(response.data, params.limit);
  }

  async getById(id: string): Promise<Collection> {
    const response = await api.get<Envelope<Collection>>(`${this.baseUrl}/${id}`);
    return unwrapApiData<Collection>(response.data);
  }

  /**
   * Registra uma ação de cobrança. O backend devolve o caso atualizado em
   * `data`; quando `data` vem vazio (implementações antigas devolviam `null`),
   * o chamador relê o caso com `getById` — nunca se assume sucesso silencioso.
   */
  async addAction(id: string, data: AddActionDTO): Promise<Collection | null> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/actions`, data);
    return unwrapApiData<Collection>(response.data) ?? null;
  }

  /** Cria/atualiza o processo judicial e escala o caso para JUDICIAL. */
  async setJudicial(id: string, data: SetJudicialDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/judicial`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /** Aciona a garantia locatícia (SEGURO_FIANCA | FIADOR | CAUCAO). */
  async activateGuarantee(id: string, data: ActivateGuaranteeDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/guarantee`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /** Registra a resposta do garantidor/acionamento já aberto. */
  async updateGuaranteeStatus(id: string, data: UpdateGuaranteeStatusDTO): Promise<Collection> {
    const response = await api.put<Envelope<Collection>>(`${this.baseUrl}/${id}/guarantee/status`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /**
   * Cria o acordo do caso (`POST /collections/:id/agreement`). O servidor
   * cancela as cobranças originais e cria as parcelas como cobranças novas —
   * a tela avisa o operador disso ANTES de enviar, porque é irreversível.
   */
  async createAgreement(id: string, data: CreateAgreementDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/agreement`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /**
   * Acordo do caso (`GET /collections/:id/agreement`). O 404 do contrato
   * significa "não existe acordo" — o chamador recebe `null`, não uma exceção,
   * para poder mostrar o formulário de criação.
   */
  async getAgreement(id: string): Promise<CollectionAgreement | null> {
    try {
      const response = await api.get<Envelope<CollectionAgreement>>(`${this.baseUrl}/${id}/agreement`);
      return unwrapApiData<CollectionAgreement>(response.data) ?? null;
    } catch (err) {
      if ((err as { response?: { status?: number } })?.response?.status === 404) return null;
      throw err;
    }
  }

  /** Quebra o acordo com motivo; o caso volta para ACTIVE. */
  async breakAgreement(id: string, data: BreakAgreementDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/agreement/break`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /** Cancela o acordo com motivo. */
  async cancelAgreement(id: string, data: CancelAgreementDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/agreement/cancel`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /**
   * Registra o desfecho do processo judicial. O efeito sobre o caso depende do
   * status (ACORDO → NEGOTIATED; SENTENÇA PROCEDENTE → segue JUDICIAL;
   * IMPROCEDENTE/ARQUIVADO → volta para ACTIVE) e as `costs` viram despesa.
   */
  async registerJudicialOutcome(id: string, data: RegisterJudicialOutcomeDTO): Promise<Collection> {
    const response = await api.post<Envelope<Collection>>(`${this.baseUrl}/${id}/judicial/outcome`, data);
    return unwrapApiData<Collection>(response.data);
  }

  async update(id: string, data: UpdateCollectionDTO): Promise<Collection> {
    const response = await api.put<Envelope<Collection>>(`${this.baseUrl}/${id}`, data);
    return unwrapApiData<Collection>(response.data);
  }

  /** @deprecated use `listPage` — mantido para os consumidores legados. */
  async getAll(params?: { status?: string; limit?: number; offset?: number }) {
    const response = await api.get<{ data: Collection[]; meta: unknown }>(this.baseUrl, { params });
    return response.data;
  }

  /** @deprecated use `getById` (que devolve o caso já desembrulhado). */
  async getByIdEnvelope(id: string) {
    const response = await api.get<{ data: Collection }>(`${this.baseUrl}/${id}`);
    return response.data;
  }

  async create(data: CreateCollectionDTO) {
    const response = await api.post<{ data: Collection }>(this.baseUrl, data);
    return response.data;
  }

  async processDelinquency() {
    const response = await api.post<{ data: unknown }>('/delinquency/process');
    return response.data;
  }
}

export const collectionService = new CollectionService();

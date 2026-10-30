import axios from 'axios';
import api, { unwrapApiData } from './api';

/**
 * Default max page size accepted by the backend's `/leads` endpoint. Mirrors
 * MaxLeadListLimit in toca-crm/internal/api/v1/handlers/lead_handler.go.
 * Used as the fallback when the backend rejects a higher limit with 400 +
 * `X-Max-Limit` header.
 */
const LEADS_API_MAX_PAGE_SIZE = 100;

export type LeadStatus =
 | 'NEW'
 | 'PENDING_ACCEPTANCE'
 | 'CONTACTED'
 | 'QUALIFIED'
 | 'VISIT_SCHEDULED'
 | 'PROPOSAL'
 | 'NEGOTIATION'
 | 'CONVERTED'
 | 'LOST'
 | 'DISQUALIFIED';

export type LeadSource =
 | 'WEBSITE'
 | 'REFERRAL'
 | 'SOCIAL'
 | 'MANUAL'
 | 'CAMPAIGN'
 | 'OTHER';

/** Role of a lead/client in the real estate process */
export type ClientRole = 'owner' | 'buyer' | 'tenant' | 'guarantor';

export interface LeadPropertyInterest {
 type?: string;
 preferred_location?: string;
 price_range?: {
 min: number;
 max: number;
 };
 features?: {
 bedrooms?: number;
 bathrooms?: number;
 parking?: number;
 area_min?: number;
 area_max?: number;
 };
 notes?: string;
}

export interface LeadHistory {
 id: string;
 action: string;
 timestamp: string;
 user_id?: string;
 details?: string;
 previous_status?: LeadStatus;
 new_status?: LeadStatus;
}

export interface ApiLead {
 id: string;
 display_id?: string;
 name: string;
 email: string;
 phone: string;
 document?: string;
 source: LeadSource;
 status: LeadStatus;
 created_at: string;
 updated_at: string;
 last_interaction?: string;
 tenant_id: string;
 property_interest?: LeadPropertyInterest;
 property_ids?: string[];
 assigned_broker_id?: string;
 history?: LeadHistory[];
 avatar?: string;

 // Client segmentation — same ID throughout the lifecycle (lead → client)
 is_client?: boolean;
  /** Role after conversion: owner, buyer, tenant, guarantor */
  role?: ClientRole;
  account_id?: string;
  /** Contract that converted the lead to a client */
 contract_id?: string;

 // Captação lead fields
 /** Property being registered at captação */
 captacao_property_id?: string;
 /** Owner lead linked at captação */
 owner_lead_id?: string;

 qualification?: Record<string, any>;
}

export interface CreateLeadDTO {
 name: string;
 email: string;
 phone: string;
 source?: LeadSource;
 status?: LeadStatus;
 avatar?: string;
 property_interest?: LeadPropertyInterest;
 assigned_broker_id?: string;
 role?: ClientRole;
 is_client?: boolean;
 captacao_property_id?: string;
 owner_lead_id?: string;
 qualification?: Record<string, any>;
}

export interface UpdateLeadDTO {
 name?: string;
 email?: string;
 phone?: string;
 source?: LeadSource;
 status?: LeadStatus;
 property_interest?: LeadPropertyInterest;
 assigned_broker_id?: string;
 role?: ClientRole;
 captacao_property_id?: string;
 owner_lead_id?: string;
 qualification?: Record<string, any>;
}

export interface ConvertToClientDTO {
 role: ClientRole;
 contract_id?: string;
}

// Issue #415 — payload de GET /leads/:id/matched-properties.
// Espelha service.MatchedProperty + clients.PropertyDTO no backend.
export interface MatchedPropertyDTO {
 id: string;
 display_id?: string;
 title: string;
 property_type: string;
 listing_type: string;
 available_for?: string[];
 status?: string;
 street?: string;
 neighborhood?: string;
 city?: string;
 state?: string;
 sale_price?: number;
 rent_price?: number;
 condo_fee?: number;
 area_total?: number;
 bedrooms?: number;
 bathrooms?: number;
 parking_spaces?: number;
 thumbnail_url?: string;
}

export interface MatchBreakdown {
 business: number;
 price: number;
 location: number;
 area: number;
 bedrooms: number;
 used_weights: {
  business: number;
  price: number;
  location: number;
  area: number;
  bedrooms: number;
 };
}

export interface MatchedProperty {
 property: MatchedPropertyDTO;
 score: number;
 reasons: MatchBreakdown;
}

export interface LeadStats {
 total: number;
 by_status: Record<string, number>;
 conversion_rate: number;
}

export interface SLATier {
 id: string;
 name: string;
 color: string;
 timeLimitMin: number;
 maxRetries: number;
 actions: {
 redirectNext: boolean;
 autoPauseBroker: boolean;
 notifyManager: boolean;
 escalateToSupervisor: boolean;
 markAsUrgent: boolean;
 };
 appliesTo: string[];
}

export interface EscalationStep {
 id: string;
 afterMinutes: number;
 action: string;
 label: string;
}

export interface NotifChannel {
 id: string;
 name: string;
 icon: string;
 enabled: boolean;
 description: string;
}

export interface QualField {
 id: string;
 label: string;
 type: string;
 required: boolean;
 active: boolean;
 options: string[];
 placeholder: string;
 helpText: string;
 conditionalOn: string | null;
 conditionalValue: string | null;
}

export interface QualRoutingConfig {
 specialtyFieldIds: string[];
 regionFieldId: string | null;
}

export interface BusinessHours {
  timezone?: string;
  windows?: Record<string, { start: string; end: string }>;
}

export interface RouletteConfig {
 tenant_id: string;
 enabled: boolean;
 active_brokers: string[];
 last_index: number;
 updated_at: string;
 updatedBy?: string;

 // Esteira (por status do lead)
 stage_broker_pools?: Record<string, string[]>;

 distMethod?: string;
 respectSpecialty?: boolean;
 respectRegion?: boolean;
 respectSchedule?: boolean;
 autoCapLimit?: boolean;
 slaTiers?: SLATier[];
 escalation?: EscalationStep[];
 notifChannels?: NotifChannel[];

 qualFields?: QualField[];
 qualRouting?: QualRoutingConfig;

 globalBusinessHours?: BusinessHours;
 simulationSeed?: number;
 /**
  * Wave P1.4 — mensagens descrevendo configurações tecnicamente válidas
  * mas sem efeito útil (ex: weighted sem brokers com Weight > 0). O
  * backend retorna pelo GET /leads/roulette; o LeadRoulette.tsx exibe
  * como banner para o admin agir antes da roleta rodar "estranho".
  */
 warnings?: string[];
}

export interface UpdateRouletteConfigDTO {
 enabled?: boolean;
 assign_unassigned_leads?: boolean;
 active_brokers?: string[];

 distMethod?: string;
 respectSpecialty?: boolean;
 respectRegion?: boolean;
 respectSchedule?: boolean;
 autoCapLimit?: boolean;
 slaTiers?: SLATier[];
 escalation?: EscalationStep[];
 notifChannels?: NotifChannel[];

 qualFields?: QualField[];
 qualRouting?: QualRoutingConfig;

 globalBusinessHours?: BusinessHours;
 simulationSeed?: number;

 // Esteira (por status do lead)
 stage_broker_pools?: Record<string, string[]>;
}

export interface RouletteSimulationRequest {
 count: number;
 stage?: string;
 persist?: boolean;
 seed?: number;
}

export interface RouletteSimulationItem {
 n: number;
 broker_id: string;
}

export interface RouletteSimulationResult {
 tenant_id: string;
 stage: string;
 count: number;
 persist: boolean;
 seed?: number;
 pool_size: number;
 assignments: RouletteSimulationItem[];
}

export interface RouletteHistoryItem {
 id: string;
 tenant_id: string;
 lead_id: string;
 from_broker_id?: string;
 to_broker_id: string;
 stage: string;
 method: string;
 reason?: string;
 created_at: string;
}

export interface RouletteHistoryResponse {
 items: RouletteHistoryItem[];
 total: number;
 limit: number;
 offset: number;
}

export interface RouletteAuditEntry {
  id: string;
  actor_id: string;
  actor_email?: string;
  action: string;
  target_id?: string;
  diff: Record<string, any>;
  created_at: string;
}

export interface RouletteAuditResponse {
  items: RouletteAuditEntry[];
  total: number;
  limit: number;
  offset: number;
}

class LeadService {
 private readonly baseUrl = '/leads';

 /**
  * Lists leads with transparent server-side pagination.
  *
  * The backend caps `limit` at 100 (MaxLeadListLimit). When the caller asks
  * for more (e.g. limit=1000 used by LeadsContext to "load everything"), we
  * issue sequential page requests of size 100 and concatenate, stopping
  * either at the requested total or when the server runs out of rows.
  *
  * On a 400 with `X-Max-Limit` header (e.g. backend reduces the cap further
  * in the future), we transparently retry using that value rather than
  * surfacing the error to the caller — keeping the contract `{ data, total,
  * page, limit }` stable. Other 4xx/5xx propagate untouched.
  */
 async getAll(params?: { page?: number; limit?: number; offset?: number; status?: string; search?: string; role?: string; is_client?: boolean; scope?: 'broker' | 'tenant' }) {
   const requestedLimit = params?.limit ?? LEADS_API_MAX_PAGE_SIZE;
   const baseOffset = params?.offset ?? 0;
   const restParams = { ...params };
   delete restParams.limit;
   delete restParams.offset;

   const pageSize = Math.min(requestedLimit, LEADS_API_MAX_PAGE_SIZE);

   const fetchPage = async (limit: number, offset: number) => {
     try {
       const res = await api.get<{ data: ApiLead[]; total: number; page: number; limit: number }>(
         this.baseUrl,
         { params: { ...restParams, limit, offset } },
       );
       return res.data;
     } catch (err) {
       // Backend caps the limit and signals via X-Max-Limit. Retry once with
       // the server-supplied cap so a misconfigured caller doesn't break the
       // whole list view.
       if (axios.isAxiosError(err) && err.response?.status === 400) {
         const headerCap = Number(err.response.headers?.['x-max-limit']);
         if (Number.isFinite(headerCap) && headerCap > 0 && headerCap < limit) {
           const retry = await api.get<{ data: ApiLead[]; total: number; page: number; limit: number }>(
             this.baseUrl,
             { params: { ...restParams, limit: headerCap, offset } },
           );
           return retry.data;
         }
       }
       throw err;
     }
   };

   // Fast path: caller fits in a single page.
   if (requestedLimit <= LEADS_API_MAX_PAGE_SIZE) {
     return fetchPage(pageSize, baseOffset);
   }

   // Slow path: paginate up to the requested total.
   const first = await fetchPage(pageSize, baseOffset);
   const collected: ApiLead[] = [...first.data];
   const grandTotal = first.total ?? collected.length;
   const stop = Math.min(requestedLimit, grandTotal);

   let cursor = baseOffset + first.data.length;
   while (collected.length < stop && first.data.length > 0) {
     const remaining = stop - collected.length;
     const next = await fetchPage(Math.min(pageSize, remaining), cursor);
     if (next.data.length === 0) break;
     collected.push(...next.data);
     cursor += next.data.length;
   }

   return {
     data: collected,
     total: grandTotal,
     page: 1,
     limit: requestedLimit,
   };
 }

 async getById(id: string) {
 const response = await api.get(`${this.baseUrl}/${id}`);
 return unwrapApiData<ApiLead>(response);
 }

 async create(data: CreateLeadDTO) {
 const response = await api.post<ApiLead>(this.baseUrl, data);
 return response.data;
 }

 async update(id: string, data: UpdateLeadDTO) {
 const response = await api.patch<ApiLead>(`${this.baseUrl}/${id}`, data);
 return response.data;
 }

 async updateStatus(id: string, status: LeadStatus) {
 return this.update(id, { status });
 }

 async getStats(): Promise<LeadStats> {
  // O backend responde com o envelope `{ success, message, statusCode, data }`.
  // Antes isto devolvia `response.data` (o envelope inteiro) tipado como
  // LeadStats — o tipo mentia e o Dashboard lia `stats.conversion_rate`
  // (undefined) em vez de `stats.data.conversion_rate`, exibindo 0%.
  const response = await api.get<{ data: LeadStats }>(`${this.baseUrl}/stats`);
  return response.data.data;
 }

 async addNote(id: string, content: string) {
 const response = await api.post<ApiLead>(`${this.baseUrl}/${id}/notes`, { content });
 return response.data;
 }

 /**
 * Convert a lead to a client with a specified role.
 * The lead keeps its unique ID — no data duplication.
 * Lead → Client, Proposta → Contrato without data loss.
 */
 async convertToClient(id: string, data: ConvertToClientDTO) {
 const response = await api.post<ApiLead>(`${this.baseUrl}/${id}/convert`, data);
 return response.data;
 }

 /** Corretor recomenda um imóvel ao cliente (sugestão exibida no portal). */
 async setRecommendedProperty(id: string, propertyId: string) {
  return api.put(`${this.baseUrl}/${id}/recommended-property`, { property_id: propertyId });
 }

 /** Corretor confirma a visita solicitada pelo cliente. */
 async confirmLeadVisit(id: string) {
  return api.put(`${this.baseUrl}/${id}/visit/confirm`);
 }

 async inviteClient(id: string) {
 await api.post(`${this.baseUrl}/${id}/invite`);
 }

 /**
  * Desqualifica um lead sem potencial com um motivo obrigatório (#429).
  * O backend marca o status como DISQUALIFIED, registra o motivo no
  * histórico do lead e emite o audit lead.disqualified.
  */
 async disqualifyLead(id: string, reason: string) {
 const response = await api.post<{ data: ApiLead }>(`${this.baseUrl}/${id}/disqualify`, { reason });
 return response.data?.data;
 }

 // ── Roulette assignment lifecycle (#425/#426/#428) ─────────────────────────

 // ── Match Properties / Cruzamento Inteligente (#415) ──────────────────────

 /**
  * Retorna imóveis disponíveis que batem com as preferências do lead,
  * rankeados por score (faixa de preço, localização, área, quartos).
  * Issue #415. Backend cuida do ranking + cut-off de matches fracos —
  * caller só renderiza a lista ordenada.
  *
  * Erros relevantes (status HTTP):
  *   - 404 lead não encontrado
  *   - 422 lead sem `property_interest` preenchido — UI deve sugerir
  *     completar o cadastro antes do match fazer sentido
  *   - 500 erro de lookup interno (CORE_SERVICE_URL ausente, etc.)
  */
 async getMatchedProperties(leadId: string, limit = 10): Promise<MatchedProperty[]> {
  const response = await api.get<{ data: MatchedProperty[] }>(`${this.baseUrl}/${leadId}/matched-properties`, {
   params: { limit },
  });
  return response.data?.data || [];
 }

 /**
  * Aceita a atribuição de um lead pela roleta — corretor confirma que vai
  * trabalhar o lead, o SLA de aceite (5min) é parado e o de primeiro
  * contato (1h) inicia. Backend pega o broker_id do JWT; o caller só
  * precisa passar o lead.id. Issue #425/#428.
  *
  * Erros HTTP relevantes:
  *   - 403 lead não está atribuído a este broker (ou broker_id faltando no JWT)
  *   - 404 lead não encontrado
  *   - 409 status do lead não permite aceitação (provavelmente já saiu da
  *     fila PENDING_ACCEPTANCE — outro corretor pegou, expirou, etc.)
  */
 async acceptAssignment(id: string) {
  const response = await api.post<{ data: ApiLead }>(`${this.baseUrl}/${id}/accept-assignment`, {});
  return response.data?.data;
 }

 /**
  * Rejeita a atribuição de um lead pela roleta. Reason é obrigatório
  * (min 2, max 500 chars) — usado pelo admin para auditar o
  * comportamento da roleta. Lead reentra na fila e o próximo corretor
  * do pool recebe. Issue #426/#428.
  */
 async rejectAssignment(id: string, reason: string) {
  const response = await api.post<{ data: ApiLead }>(`${this.baseUrl}/${id}/reject-assignment`, {
   reason,
  });
  return response.data?.data;
 }

 // ── Roulette config ────────────────────────────────────────────────────────

 async getRouletteConfig() {
 const response = await api.get(`${this.baseUrl}/roulette`);
 return unwrapApiData<RouletteConfig>(response);
 }

 async getRouletteUnassignedStatus() {
 const response = await api.get(`${this.baseUrl}/roulette/unassigned`);
 return unwrapApiData<{ has_unassigned: boolean; count: number }>(response);
 }

 async updateRouletteConfig(data: UpdateRouletteConfigDTO) {
 const response = await api.put(`${this.baseUrl}/roulette`, data);
 return unwrapApiData<RouletteConfig>(response);
 }

 async simulateRoulette(data: RouletteSimulationRequest) {
 const response = await api.post(`${this.baseUrl}/roulette/simulate`, data);
 return unwrapApiData<RouletteSimulationResult>(response);
 }

 async getRouletteHistory(params?: { limit?: number; offset?: number }) {
 const response = await api.get(`${this.baseUrl}/roulette/history`, { params });
 return unwrapApiData<RouletteHistoryResponse>(response);
 }

 async getRouletteAudit(params?: { limit?: number; offset?: number; action?: string; target_id?: string; from?: string; to?: string }) {
  const response = await api.get(`${this.baseUrl}/roulette/audit`, { params });
  return unwrapApiData<RouletteAuditResponse>(response);
 }
}

export const leadService = new LeadService();

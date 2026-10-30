import api from './api';
import type { AnalysisCriterion } from '../documents/types';

export type DealStatus =
  | 'NEGOTIATION'
  | 'PROPOSAL'
  | 'ANALYSIS'
  | 'CONTRACT'
  | 'CONVERTED'
  | 'CANCELLED';

export type DealType = 'SALE' | 'RENT';

/** Role of a broker in a deal (captador, visitador, fechamento) */
export type BrokerParticipationRole = 'captador' | 'co_captador' | 'visitador' | 'atendente' | 'fechamento' | 'imobiliaria';

/** Tracks a broker's participation in a deal for history and commission split */
export interface BrokerParticipation {
  broker_id: string;
  role: BrokerParticipationRole;
  commission?: number;
  percentage?: number;
}

export interface Proposal {
  id: string;
  value: number;
  counter_value?: number;
  payment_method: string;
  installments?: number;
  down_payment?: number;
  status: 'DRAFT' | 'SENT' | 'ACCEPTED' | 'REJECTED' | 'COUNTER_OFFER';
  created_at: string;
  created_by: string;
  notes?: string;
  accepted_by?: Array<{
    party_id: string;
    role: PartyRole;
    accepted_at: string;
  }>;
}

export type DueDiligenceCriterionStatus =
  | 'pending'
  | 'in_review'
  | 'approved'
  | 'rejected'
  | 'waived';

export interface DueDiligenceCriterionReview {
  status: DueDiligenceCriterionStatus;
  reviewed_by?: string;
  reviewed_at?: string;
  notes?: string;
}

export interface DueDiligence {
  score_serasa?: number;
  cnd_status: string;
  legal_opinion?: string;
  monthly_income?: number;
  criteria?: Partial<Record<AnalysisCriterion, DueDiligenceCriterionReview>>;
  status: string;
  updated_at: string;
}

export type PartyKind = 'person' | 'company';
export type PartyRole =
  | 'tenant'
  | 'buyer'
  | 'owner'
  | 'guarantor'
  | 'witness'
  | 'spouse'
  | 'attorney'
  | 'legal_representative'
  | 'guardian'
  | 'curator';

export type RepresentationType = 'attorney' | 'legal_representative' | 'guardian' | 'curator';

export interface PartyRepresentationDTO {
  type: RepresentationType;
  representative_party_id: string;
  requires_principal_signature: boolean;
  allow_representative_proposal: boolean;
}

/** Origem de um campo da party: identificado pela IA (ocr/ai), digitado (manual) ou confirmado pelo operador (validated). */
export type ProvenanceSource = 'ocr' | 'ai' | 'manual' | 'client' | 'validated';

export interface FieldProvenanceDTO {
  source: ProvenanceSource;
  confidence?: number;
  document_id?: string;
  validated_by?: string;
  validated_at?: string;
  updated_at: string;
}

export interface ApplyExtractionFieldDTO {
  name: string;
  target?: string;
  value: string;
  confidence?: number;
}

export interface PartyDTO {
  id: string;
  lead_id?: string;
  account_id?: string;
  name: string;
  email: string;
  phone?: string;
  document?: string;
  birth_date?: string;
  custom_fields?: Record<string, string>;
  /** Origem/confiança por campo (IA, manual, validado). Alimenta o indicador de origem na UI. */
  field_provenance?: Record<string, FieldProvenanceDTO>;
  kind?: PartyKind;
  role: PartyRole;
  spouse_of_party_id?: string;
  no_spouse_declared?: boolean;
  representation?: PartyRepresentationDTO;
  share_pct?: number;
  is_primary: boolean;
  /** When undefined, falls back to is_primary (backend semantic). */
  signature_required?: boolean;
  sign_status?: 'PENDING' | 'SIGNED' | 'REJECTED';
  signed_at?: string;
  signature_url?: string;
  due_diligence?: DueDiligence;
  created_at: string;
  updated_at: string;
}

export interface Signer {
  id: string;
  name: string;
  email: string;
  role: string;
  status: 'PENDING' | 'SIGNED' | 'REJECTED';
  signed_at?: string;
  signature_url?: string;
  /** When undefined, defaults to true (backend semantic). */
  is_required?: boolean;
  // ── Forensic signing metadata (Phase 3/S8) ──
  // Populated by the toca-signature subscriber when Clicksign returns
  // the data. Surfaced in the audit panel and exportable for legal
  // hand-offs. Always optional — older signers and email-token methods
  // won't have all fields.
  signed_ip?: string;
  /** Mirrors clicksign.AuthMethod: email|whatsapp|sms|selfie|icp_brasil|selfie+document. */
  auth_method_used?: string;
  /** ICP-Brasil X.509 certificate serial (uppercase hex). Empty for non-ICP methods. */
  certificate_serial?: string;
  /** Clicksign per-signer widget event log (page views, signature acceptance). */
  widget_event_log_url?: string;
  /** Clicksign proof-of-authentication artifact (selfie+document, cert snapshot, …). */
  auth_evidence_url?: string;
}

export type ChecklistItemStatusDTO = 'PENDING' | 'UPLOADED' | 'APPROVED' | 'REJECTED';
export type ChecklistItemCategoryDTO = 'pessoal' | 'financeiro' | 'imovel' | 'juridico' | 'outros';

export interface ChecklistRequestedFieldDTO {
  key: string;
  label: string;
  type: string;
  required: boolean;
}

export interface ChecklistFormSubmissionDTO {
  values: Record<string, string>;
  requested_fields?: ChecklistRequestedFieldDTO[];
  submitted_by: string;
  submitted_at: string;
}

export interface ChecklistItemDTO {
  storage_key: string;
  id: string;
  name: string;
  category: ChecklistItemCategoryDTO;
  party_id?: string;
  status: ChecklistItemStatusDTO;
  required: boolean;
  description?: string;
  /** "upload" (anexo) | "form" (apenas preenchimento de formulário, sem anexo). */
  request_mode?: "upload" | "form";
  requested_fields?: ChecklistRequestedFieldDTO[];
  rejection_reason?: string;
  file_url?: string;
  file_name?: string;
  content_type?: string;
  file_size?: number;
  uploaded_by?: string;
  uploaded_at?: string;
  reviewed_at?: string;
  solicited: boolean;
  solicited_at?: string;
  /** Snapshot do formulário no momento do envio pelo cliente. */
  form_submission?: ChecklistFormSubmissionDTO;
}

export interface InitChecklistItemDTO {
  name: string;
  category: ChecklistItemCategoryDTO;
  required: boolean;
  description?: string;
  request_mode?: "upload" | "form";
  requested_fields?: ChecklistRequestedFieldDTO[];
}

export interface InitChecklistDTO {
  items: InitChecklistItemDTO[];
}

export interface AddChecklistItemDTO {
  name: string;
  category: ChecklistItemCategoryDTO;
  party_id?: string;
  required: boolean;
  description?: string;
  request_mode?: "upload" | "form";
  requested_fields?: ChecklistRequestedFieldDTO[];
}

export interface UpdateChecklistItemStatusDTO {
  status: ChecklistItemStatusDTO;
  rejection_reason?: string;
}

export interface DealHistory {
  id: string;
  deal_id: string;
  status: DealStatus;
  timestamp: string;
  user_id: string;
  notes?: string;
}

export interface DealDTO {
  id: string;
  display_id?: string;
  type: DealType;
  status: DealStatus;
  property_id: string;
  owner_id: string;
  /** Imóvel que o corretor recomenda ao cliente (Sugestão do seu corretor). */
  recommended_property_id?: string;
  /** Lead/Client ID — buyer or tenant */
  customer_id: string;
  /** Explicit lead reference (preserved through lead→client conversion) */
  lead_id?: string;
  broker_id: string;
  /** Broker participations: captador, visitador, fechamento with commission split */
  broker_participations?: BrokerParticipation[];
  value: number;
  commission: number;
  created_at: string;
  updated_at: string;
  tenant_id: string;
  proposals?: Proposal[];
  due_diligence?: DueDiligence;
  parties?: PartyDTO[];
  signers?: Signer[];
  checklist?: ChecklistItemDTO[];
  history?: DealHistory[];
  my_party_id?: string;
  my_role?: PartyRole;
  /**
   * Derived checklist progress populated by the backend on GET /deals/:id
   * (Fase 2/W1 — see `deal.ChecklistStats` in toca-deal). Optional because
   * older list endpoints don't populate it.
   */
  checklist_stats?: ChecklistStatsDTO;
}

export interface ChecklistStatsDTO {
  total: number;
  required: number;
  approved: number;
  required_approved: number;
  pending: number;
  uploaded: number;
  rejected: number;
  required_complete: boolean;
}

// ── Checklist Templates (Phase 4/W4) ──
// Mirrors deal.ChecklistTemplate in toca-deal. Backend-served set
// replaces the hardcoded DEFAULT_CHECKLIST_TEMPLATES in data.ts — the
// FE keeps a local fallback for resilience.
export interface ChecklistTemplateDTO {
  id: string;
  label: string;
  items: {
    name: string;
    category: ChecklistItemCategoryDTO;
    required: boolean;
    description?: string;
    request_mode?: "upload" | "form";
    requested_fields?: ChecklistRequestedFieldDTO[];
  }[];
}

export interface CreatePartyDTO {
  lead_id?: string;
  account_id?: string;
  name?: string;
  email?: string;
  phone?: string;
  document?: string;
  kind?: PartyKind;
  role: PartyRole;
  spouse_of_party_id?: string;
  no_spouse_declared?: boolean;
  representation?: PartyRepresentationDTO;
  share_pct?: number;
  is_primary?: boolean;
}

export interface CreateDealDTO {
  type: DealType;
  property_id: string;
  owner_id: string;
  /** Lead/Client ID — buyer or tenant */
  customer_id: string;
  /** Explicit lead ID if different from customer_id */
  lead_id?: string;
  broker_id: string;
  value: number;
  customer_name?: string;
  customer_email?: string;
  customer_account_id?: string;
  customer_role?: string;
  /** Owner profile hints — let the backend populate Party.Name/Email/etc. without
   *  relying on the CRM service-to-service lookup, which can fail silently. */
  owner_name?: string;
  owner_email?: string;
  owner_phone?: string;
  owner_document?: string;
  owner_account_id?: string;
  parties?: CreatePartyDTO[];
  /** Initial commission split inherited from the property capture. */
  broker_participations?: BrokerParticipation[];
}

export interface UpdateDealStatusDTO {
  status: DealStatus;
  notes?: string;
}

export interface AddProposalDTO {
  value: number;
  payment_method: string;
  installments?: number;
  down_payment?: number;
  notes?: string;
}

export interface UpdateDueDiligenceDTO {
  score_serasa?: number;
  cnd_status?: string;
  legal_opinion?: string;
  monthly_income?: number;
  status?: string;
}

export interface UpdatePartyDTO {
  role?: PartyRole;
  no_spouse_declared?: boolean;
  representation?: PartyRepresentationDTO | null;
  signature_required?: boolean;
  /** Campos identificadores — completados pelo operador no painel de partes
   *  da Esteira. O documento (CPF/CNPJ) é exigido pelo gate de Contrato. */
  name?: string;
  email?: string;
  phone?: string;
  document?: string;
  birth_date?: string;
  custom_fields?: Record<string, string>;
  removed_custom_fields?: string[];
}

export interface AddSignerDTO {
  name: string;
  email: string;
  role: string;
  is_required?: boolean;
}

export interface UpdateSignerStatusDTO {
  status: 'PENDING' | 'SIGNED' | 'REJECTED';
  signature_url?: string;
}

export interface UpdateSignerDTO {
  is_required?: boolean;
  name?: string;
  email?: string;
  role?: string;
}

export interface AddBrokerParticipationDTO {
  broker_id: string;
  role: BrokerParticipationRole;
  commission?: number;
  percentage?: number;
}

export interface DealStats {
  active_proposals: number;
  monthly_sales: number;
}

/**
 * Server-side audit log entry from `audit.events`. Mirrors `pkg/audit.LogEntry`.
 * `before`/`after` carry the field snapshots written by `audit.Snapshot` and
 * are typically a partial of the entity — never the full aggregate, never PII.
 */
export interface DealAuditLogEntry {
  id: string;
  tenant_id: string;
  actor_id?: string;
  actor_type: string;
  action: string;
  entity_type: string;
  entity_id: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at: string;
}

class DealService {
  private readonly baseUrl = '/deals';

  async getAll(limit = 100, offset = 0, signal?: AbortSignal) {
    return api.get<{ data: DealDTO[], meta: { total: number; page: number; limit: number; totalPages: number } }>(`${this.baseUrl}?limit=${limit}&offset=${offset}`, { signal });
  }

  /**
   * Filtered listing — backend accepts `customer_id`, `property_id`,
   * `broker_id`, `status`, `type`, `search` query params (added in Phase 1).
   * Used by LeadDetails > Negócios and PropertyDetails > Negócios sections
   * (Phase 4) to fetch deals related to a single entity without dragging
   * the full deal pipeline.
   */
  async list(params?: { customer_id?: string; property_id?: string; broker_id?: string; status?: string; type?: string; search?: string; limit?: number; offset?: number }) {
    return api.get<{ data: DealDTO[], meta: any }>(this.baseUrl, { params });
  }

  async getById(id: string) {
    return api.get<{ data: DealDTO }>(`${this.baseUrl}/${id}`);
  }

  /** Corretor agenda uma visita para o cliente (persistida no calendário). */
  async createAppointment(id: string, req: { date: string; time: string; end_time: string; type?: string; title?: string; description?: string }) {
    return api.post<{ data: unknown }>(`${this.baseUrl}/${id}/appointments`, {
      date: req.date, time: req.time, end_time: req.end_time,
      type: req.type || "Visita", title: req.title || "Visita", description: req.description,
    });
  }

  /** Corretor recomenda um imóvel ao cliente (sugestão exibida no portal). */
  async setRecommendedProperty(id: string, propertyId: string) {
    return api.put<{ data: unknown }>(`${this.baseUrl}/${id}/recommended-property`, { property_id: propertyId });
  }

  /**
   * Persistent audit trail for a deal. Reads `audit.events` server-side
   * (5-year retention, RLS by tenant). Replaces the in-memory log the
   * Esteira's AuditTrailPanel previously held in CRMContext.
   */
  async getAuditLog(id: string, limit = 100) {
    return api.get<{ data: DealAuditLogEntry[] }>(`${this.baseUrl}/${id}/audit-log`, {
      params: { limit },
    });
  }

  /**
   * Idempotency-Key identifica a INTENÇÃO de criação: reenvio do mesmo
   * formulário (duplo clique, retry de rede) devolve o negócio já criado em
   * vez de duplicar — o backend responde com o registro existente.
   */
  async create(data: CreateDealDTO, idempotencyKey?: string) {
    return api.post<{ data: DealDTO }>(this.baseUrl, data, {
      headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : undefined,
    });
  }

  async updateStatus(id: string, data: UpdateDealStatusDTO) {
    return api.patch<{ data: DealDTO }>(`${this.baseUrl}/${id}/status`, data);
  }

  /**
   * Force-resync the Deal.Signers projection from the contract's
   * signatories. Recovery hatch invoked by the Esteira "Reconciliar"
   * button when the signer badges drift from the envelope reality —
   * runs the same auto-heal MoveDealStatus would run, without waiting
   * for an event redelivery. Idempotent.
   */
  async reconcileSigners(id: string) {
    return api.post<{ data: { deal_id: string; tenant_id: string } }>(`${this.baseUrl}/${id}/reconcile-signers`);
  }

  async delete(id: string) {
    return api.delete(`${this.baseUrl}/${id}`);
  }

  async getStats() {
    return api.get<{ data: DealStats }>(`${this.baseUrl}/stats`);
  }

  async addProposal(id: string, data: AddProposalDTO) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/proposals`, data);
  }

  async updateDueDiligence(id: string, data: UpdateDueDiligenceDTO) {
    return api.put<{ data: DealDTO }>(`${this.baseUrl}/${id}/due-diligence`, data);
  }

  async listParties(id: string) {
    return api.get<{ data: PartyDTO[] }>(`${this.baseUrl}/${id}/parties`);
  }

  async addParty(id: string, data: CreatePartyDTO) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties`, data);
  }

  async updateParty(id: string, partyId: string, data: UpdatePartyDTO) {
    return api.patch<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties/${partyId}`, data);
  }

  async updatePartyRole(id: string, partyId: string, data: { role: PartyRole }) {
    return this.updateParty(id, partyId, data);
  }

  async setPartyPrimary(id: string, partyId: string) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties/${partyId}/set-primary`, {});
  }

  async removeParty(id: string, partyId: string) {
    return api.delete<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties/${partyId}`);
  }

  async updatePartyDueDiligence(id: string, partyId: string, data: UpdateDueDiligenceDTO) {
    return api.patch<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties/${partyId}/due-diligence`, data);
  }

  async updatePartyDueDiligenceCriterion(
    id: string,
    partyId: string,
    criterion: AnalysisCriterion,
    data: { status: DueDiligenceCriterionStatus; notes?: string },
  ) {
    return api.patch<{ data: DealDTO }>(
      `${this.baseUrl}/${id}/parties/${partyId}/due-diligence/criteria/${criterion}`,
      data,
    );
  }

  /** Aplica/valida na party os campos identificados pela IA (origem = validated). */
  async applyExtractedFields(
    id: string,
    partyId: string,
    payload: { document_id?: string; fields: ApplyExtractionFieldDTO[] },
  ) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/parties/${partyId}/apply-extraction`, payload);
  }

  async addSigner(id: string, data: AddSignerDTO) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/signers`, data);
  }

  async updateSignerStatus(id: string, signerId: string, data: UpdateSignerStatusDTO) {
    return api.patch<{ data: DealDTO }>(`${this.baseUrl}/${id}/signers/${signerId}`, data);
  }

  async updateSigner(id: string, signerId: string, data: UpdateSignerDTO) {
    return api.put<{ data: DealDTO }>(`${this.baseUrl}/${id}/signers/${signerId}`, data);
  }

  async removeSigner(id: string, signerId: string) {
    return api.delete(`${this.baseUrl}/${id}/signers/${signerId}`);
  }

  async inviteSigner(id: string, signerId: string) {
    return api.post(`${this.baseUrl}/${id}/signers/${signerId}/invite`, {});
  }

  /**
  * Add a broker with a role (captador, visitador, fechamento) to a deal.
  * Used to track participation for history and commission split.
  */
  async addBrokerParticipation(id: string, data: AddBrokerParticipationDTO) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/broker-participations`, data);
  }

  /**
   * Replace the entire broker_participations array on a deal — used by the
   * commission distribution UI (issue #420) when an admin/gestor ajusta a
   * distribuição. The backend valida soma == 100% (±0.01) e devolve 422; se
   * o deal já estiver CONVERTED/CANCELLED devolve 409 (locked). Caller
   * should surface those errors to the user with a toast — see the
   * CommissionSplitCard which already does this.
   */
  async updateBrokerParticipations(id: string, participations: BrokerParticipation[], commissionPct?: number) {
    return api.put<{ data: DealDTO }>(`${this.baseUrl}/${id}/broker-participations`, {
      commission: commissionPct,
      participations,
    });
  }

  /**
   * Aceita uma contraproposta enviada pelo cliente — endpoint admin/operador
   * (gated por `deals:approve` no backend). Issue #421 / #424.
   *
   * Atualiza Deal.Value para `proposal.counter_value`, marca a proposta
   * como ACCEPTED e publica o evento `deal.counter_offer_accepted` para
   * que toca-comm, toca-financial, etc. reajam.
   *
   * Erros relevantes (status HTTP):
   *   - 400 proposal_id ausente
   *   - 403 sem permissão `deals:approve`
   *   - 404 deal/proposta não encontrada
   *   - 409 proposta não está em COUNTER_OFFER (ex.: já aceita)
   */
  async acceptCounterOffer(id: string, proposalId: string) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/accept-counter-offer`, {
      proposal_id: proposalId,
    });
  }

  // ── Checklist operations ──

  async initChecklist(id: string, data: InitChecklistDTO) {
    return api.put<{ data: DealDTO }>(`${this.baseUrl}/${id}/checklist`, data);
  }

  async addChecklistItem(id: string, data: AddChecklistItemDTO) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/checklist`, data);
  }

  async removeChecklistItem(id: string, itemId: string) {
    return api.delete<{ data: DealDTO }>(`${this.baseUrl}/${id}/checklist/${itemId}`);
  }

  async updateChecklistItemStatus(id: string, itemId: string, data: UpdateChecklistItemStatusDTO) {
    return api.patch<{ data: DealDTO }>(`${this.baseUrl}/${id}/checklist/${itemId}`, data);
  }

  /** Reconcilia no painel os documentos enviados pelo cliente na esteira. */
  async backfillChecklistDocuments() {
    return api.post<{ data: unknown }>(`${this.baseUrl}/backfill-checklist-documents`, {});
  }

  async solicitChecklistItem(
    id: string,
    itemId: string,
    partyId?: string,
    requestedFields?: ChecklistRequestedFieldDTO[],
    requestMode?: "upload" | "form",
  ) {
    return api.post<{ data: DealDTO }>(`${this.baseUrl}/${id}/checklist/${itemId}/solicit`, {
      ...(partyId ? { party_id: partyId } : {}),
      ...(requestedFields && requestedFields.length > 0 ? { requested_fields: requestedFields } : {}),
      ...(requestMode ? { request_mode: requestMode } : {}),
    });
  }

  // ── Portal (public, no auth) ──

  async getPortalDeal(token: string) {
    return api.get<{ data: DealDTO }>(`/portal/${token}`);
  }

  async getPortalChecklist(token: string) {
    return api.get<{ data: ChecklistItemDTO[] }>(`/portal/${token}/checklist`);
  }

  async portalUploadItem(token: string, itemId: string) {
    return api.patch<{ data: DealDTO }>(`/portal/${token}/checklist/${itemId}/upload`, {});
  }

  /**
   * Fetch the canonical checklist templates from the backend. Cache for
   * the lifetime of the page since templates change rarely and the
   * Esteira tab opens repeatedly. Callers should treat a rejection as
   * "use the local fallback" rather than blocking the UI.
   */
  async listChecklistTemplates() {
    const res = await api.get<{ data: ChecklistTemplateDTO[] }>(`/checklist-templates`);
    return res.data?.data ?? [];
  }
}

export const dealService = new DealService();

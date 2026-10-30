import portalApi, { unwrapPortalData } from './portal-api';
import type { DealDTO } from '../../../shared/services/deal.service';

export interface PortalAuthResponse {
  access_token: string;
}

export interface PortalUser {
  id: string;
  email?: string;
  phone?: string;
  name: string;
  firstname?: string;
  lastname?: string;
  fullname?: string;
  role: string;
  account_role?: string;
  functional_role?: string;
  account_type?: string;
  avatar_url?: string;
  address?: {
    street: string;
    number: string;
    complement?: string;
    district: string;
    city: string;
    state: string;
    zip_code: string;
  };
  notifications?: {
    email: boolean;
    sms: boolean;
    push: boolean;
  };
  banking?: PortalBankingInfo;
}

export type PortalPixKeyType = "cpf" | "cnpj" | "email" | "phone" | "evp";
export type PortalBankAccountType = "checking" | "savings";

export interface PortalBankingInfo {
  bank_code?: string;
  bank_name?: string;
  agency?: string;
  agency_digit?: string;
  account?: string;
  account_digit?: string;
  account_type?: PortalBankAccountType;
  pix_key_type?: PortalPixKeyType;
  pix_key?: string;
  holder_name?: string;
  holder_document?: string;
}

export interface PortalPartyFieldsUpdateRequest {
  name?: string;
  document?: string;
  birth_date?: string;
  phone?: string;
  custom_fields?: Record<string, string>;
}

const FUNCTIONAL_ROLE_LABELS: Record<string, string> = {
  cliente: "Cliente",
  corretor: "Corretor",
  admin: "Administrador",
  gestor: "Gestor",
  staff: "Equipe",
  proprietario: "Proprietário",
  inquilino: "Inquilino",
  comprador: "Comprador",
};

function normalizePortalUser(raw: any): PortalUser {
  const normalizeName = (value: string) => value.trim().replace(/\s+/g, " ");
  const fullname = normalizeName(raw?.fullname ?? "");
  let firstname = normalizeName(raw?.firstname ?? "") || fullname;
  let lastname = normalizeName(raw?.lastname ?? "");
  // Older passwordless client accounts stored the full name in firstname.
  // Preserve explicitly separated compound names; split only the legacy shape.
  if (!lastname) {
    const parts = firstname.split(" ");
    firstname = parts[0];
    lastname = parts.slice(1).join(" ");
  } else if (firstname.endsWith(` ${lastname}`)) {
    firstname = firstname.slice(0, -(lastname.length + 1));
  }
  const composed = [firstname, lastname].filter(Boolean).join(" ").trim();
  const name = fullname || composed;
  const functional = (raw?.functional_role ?? "").trim();
  const accountRole = (raw?.role ?? "").trim();
  const role = functional ? (FUNCTIONAL_ROLE_LABELS[functional] ?? functional) : accountRole;
  return {
    id: raw?.id,
    email: raw?.email || undefined,
    phone: raw?.phone || undefined,
    name,
    firstname: firstname || undefined,
    lastname: lastname || undefined,
    fullname: fullname || undefined,
    role,
    account_role: accountRole || undefined,
    functional_role: functional || undefined,
    account_type: raw?.account_type || undefined,
    avatar_url: raw?.avatar_url || undefined,
    address: raw?.address || undefined,
    notifications: raw?.notifications || undefined,
    banking: raw?.banking || undefined,
  };
}

export interface PortalPropertySummary {
  id: string;
  title: string;
  address: string;
  neighborhood: string;
  city: string;
  state: string;
  image_url?: string;
  price: number;
  bedrooms?: number;
  bathrooms?: number;
  parking_spaces?: number;
  total_area_m2?: number;
  listing_type?: string;
  property_type?: string;
  /** Detalhes que qualificam a decisão (nunca identificação exata: rua/número
   *  saem mascarados das leituras de vitrine e só aparecem no imóvel do
   *  negócio, onde o cliente já é parte). */
  suites?: number;
  usable_area_m2?: number;
  amenities?: string[];
  description?: string;
  images?: { url: string; description?: string }[];
  /** Compatibilidade calculada a partir das preferências do lead do negócio. */
  match_score?: number;
  match_reasons?: {
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
  };
}

export type PortalFavoriteIDs = string[];

export interface PortalCharge {
  id: string;
  description: string;
  due_date: string;
  /** Valor original do período, sem encargos. */
  amount: number;
  /** Multa por atraso somada ao total. Opcional: payload antigo não trazia. */
  late_fee?: number;
  /** Juros pro rata die somados ao total. Opcional: payload antigo não trazia. */
  interest?: number;
  /** Desconto abatido do total. Opcional: payload antigo não trazia. */
  discount?: number;
  /** Valor original + multa + juros − desconto. */
  total_amount: number;
  /**
   * Dias corridos de atraso (0 quando a cobrança está em dia). Opcional: a
   * origem interna não serializa o campo, então payload antigo pode não trazer
   * — a tela deriva do vencimento nesse caso.
   */
  days_overdue?: number;
  status: string;
  paid_at?: string | null;
  reference_month?: string;
  /** URL de pagamento (PIX/boleto) fornecida pelo PSP quando configurada. */
  payment_url?: string;
}

export interface PortalMaintenanceTicket {
  id: string;
  type: string;
  category: string;
  status: string;
  priority: string;
  description: string;
  attachments?: string[];
  property_title?: string;
  created_at: string;
  completed_at?: string | null;
}

export interface PortalTicketComment {
  id: string;
  author_id: string;
  author_name?: string;
  author_type: "client" | "operator";
  content: string;
  attachments?: string[];
  created_at: string;
}

export interface PortalTicket {
  id: string;
  tenant_id: string;
  deal_id?: string;
  property_id?: string;
  type: string;
  category: string;
  subject: string;
  description?: string;
  status: string;
  priority: string;
  requester_account_id: string;
  requester_name?: string;
  requester_email?: string;
  assigned_to_id?: string;
  assigned_to_name?: string;
  comments?: PortalTicketComment[];
  created_at: string;
  updated_at: string;
  resolved_at?: string | null;
  closed_at?: string | null;
}

export interface CreateTicketRequest {
  type?: string;
  category?: string;
  subject: string;
  description: string;
  priority?: string;
}

export interface PortalAttachmentUploadURL {
  upload_url: string;
  bucket: string;
  storage_key: string;
  expires_at: string;
  content_type?: string;
  file_name: string;
  file_size: number;
}

export interface PortalAttachmentUploadComplete {
  storage_key: string;
}

export interface PortalLead {
  id: string;
  status: string;
  property_title?: string;
  preferred_location?: string;
}

export interface CreateMaintenanceTicketRequest {
  type?: string;
  category?: string;
  priority?: string;
  description: string;
  attachments?: string[];
}

export interface PortalAppointment {
  can_manage?: boolean;
  updated_at?: string;
  id: string;
  title: string;
  date: string;
  time: string;
  end_time: string;
  type: string;
  status: string;
  description?: string;
  property?: string;
  created_at: string;
}

export interface CreateAppointmentRequest {
  title?: string;
  date: string;
  time: string;
  end_time: string;
  type?: string;
  description?: string;
  property_id?: string;
}

export interface PortalVisitRequest {
  property_id?: string;
  appointment_id?: string;
  date?: string;
  time?: string;
  end_time?: string;
  note?: string;
  status?: string;
}

export interface RequestLeadVisit {
  property_id?: string;
  date: string;
  time: string;
  end_time?: string;
  note?: string;
}

export interface PropertySearchParams {
  q?: string;
  listing_type?: string;
  min_bedrooms?: number;
  min_price?: number;
  max_price?: number;
  limit?: number;
  offset?: number;
}

export interface PortalSignerView {
  signer_id: string;
  name: string;
  role: string;
  status: string;
  signed_at?: string;
  request_signature_key?: string;
}

export interface PortalSignatureView {
  has_envelope: boolean;
  contract_id?: string;
  envelope_id?: string;
  envelope_status?: string;
  environment?: string;
  deadline?: string;
  signed_document_url?: string;
  my_signer?: PortalSignerView;
  other_signers: PortalSignerView[];
  signed_count: number;
  total_count: number;
}

export type PortalMyRentalRole = 'tenant' | 'owner' | 'both' | '';

export interface PortalRental {
  id: string;
  tenant_id: string;
  deal_id: string;
  contract_id: string;
  property_id: string;
  tenant_lead_id: string;
  owner_lead_id: string;
  broker_id?: string;
  start_date: string;
  end_date: string;
  monthly_value_cents: number;
  status: 'ACTIVE' | 'ENDED' | 'RESCINDED';
  created_at: string;
  updated_at: string;
}

export interface PortalRentalDTO {
  rental: PortalRental;
  my_role: PortalMyRentalRole;
}

export type PortalTransferStatus = "PENDING" | "APPROVED" | "PROCESSED" | "FAILED" | "CANCELLED";

export interface PortalTransfer {
  id: string;
  tenant_id: string;
  contract_id: string;
  owner_id: string;
  property_id: string;
  reference_month: string;
  gross_rent: number;
  admin_fee_percent: number;
  admin_fee_amount: number;
  maintenance_deductions: number;
  other_deductions: number;
  net_amount: number;
  /** Composição econômica congelada no repasse (inclui a intermediação do mês). */
  economic_breakdown?: {
    flow: string;
    gross_cents: number;
    owner_payable_cents: number;
    admin_fee_cents?: number;
    intermediation_revenue_cents?: number;
    commission_funded_cents?: number;
  };
  status: PortalTransferStatus;
  processed_at?: string | null;
  approved_at?: string | null;
  bank_account?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface PortalTransferListParams {
  status?: PortalTransferStatus | "";
  year?: number;
  limit?: number;
  offset?: number;
}

export interface PortalInforme {
  id: string;
  tenant_id: string;
  year: number;
  owner_id: string;
  owner_name: string;
  total_gross: number;
  total_admin_fee: number;
  // T-05: linha de intermediação imobiliária do informe anual (comissão
  // retida dos repasses). 0 em informes gerados antes da linha existir.
  total_intermediation: number;
  total_net: number;
  /** Separação por fluxo (T-06): locação (renda) x venda (repasse de capital). */
  rental_gross?: number;
  rental_admin_fee?: number;
  rental_intermediation?: number;
  rental_net?: number;
  rental_count?: number;
  sale_gross?: number;
  sale_intermediation?: number;
  sale_net?: number;
  sale_count?: number;
  status: string;
  pdf_url?: string;
  generated_at?: string | null;
  created_at: string;
}

export interface PortalMatchedProperty {
  id: string;
  title: string;
  sale_price?: number;
  rent_price?: number;
  neighborhood?: string;
  city?: string;
  state?: string;
  score?: number;
  thumbnail_url?: string;
  media_images?: string[];
  // O backend sempre mandou estes campos (`clients.PropertyDTO`); o tipo do
  // portal é que os omitia — sem `listing_type` a tela não tinha como dizer se
  // o valor exibido era venda ou aluguel.
  listing_type?: string;
  available_for?: string[];
  status?: string;
  property_type?: string;
}

export interface PortalOverviewLead {
  id: string;
  display_id?: string;
  tenant_id?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
  last_interaction?: string;
  name: string;
  email?: string;
  phone?: string;
  interest?: unknown;
  /** Papel do cliente no atendimento: buyer | tenant | owner | guarantor. */
  role?: string;
  has_preferences: boolean;
  recommended_property_id?: string;
  recommended_property?: PortalMatchedProperty;
  broker_name?: string;
  broker_email?: string;
  broker_avatar?: string;
  visit_request?: PortalVisitRequest;
  matched_properties: Array<{ property: PortalMatchedProperty; score: number }>;
}

export const portalService = {
  requestOtp: async (contact: string, method: 'email' | 'whatsapp' = 'email'): Promise<void> => {
    await portalApi.post('/auth/otp/request', { contact, method });
  },

  verifyOtp: async (contact: string, code: string): Promise<PortalAuthResponse> => {
    const response = await portalApi.post('/auth/otp/verify', { contact, code });
    return unwrapPortalData<PortalAuthResponse>(response);
  },

  logout: async (): Promise<void> => {
    await portalApi.post('/auth/logout');
  },

  getCurrentUser: async (options?: { silent?: boolean }): Promise<PortalUser> => {
    const response = await portalApi.get('/auth/me', { _silent: options?.silent ?? false } as any);
    return normalizePortalUser(unwrapPortalData<any>(response));
  },

  getDeals: async (options?: { signal?: AbortSignal }) => {
    const response = await portalApi.get('/client-portal/deals', { signal: options?.signal });
    return unwrapPortalData(response);
  },
  getOverview: async (): Promise<PortalOverviewLead[]> => {
    const response = await portalApi.get('/client-portal/overview');
    return unwrapPortalData<PortalOverviewLead[]>(response);
  },
  requestLeadVisit: async (leadId: string, req: RequestLeadVisit) => {
    const response = await portalApi.post(`/client-portal/leads/${leadId}/visit-request`, req);
    return unwrapPortalData(response);
  },
  updateLeadVisit: async (leadId: string, req: Omit<RequestLeadVisit, "property_id">) => {
    const response = await portalApi.put(`/client-portal/leads/${leadId}/visit`, req);
    return unwrapPortalData(response);
  },
  cancelLeadVisit: async (leadId: string) => {
    const response = await portalApi.delete(`/client-portal/leads/${leadId}/visit`);
    return unwrapPortalData(response);
  },
  openDealChannel: async (dealId: string): Promise<{ id: string }> => {
    const response = await portalApi.get(`/client-portal/deals/${encodeURIComponent(dealId)}/channel`);
    return unwrapPortalData<{ id: string }>(response);
  },
  openDirectChannel: async (leadId: string): Promise<{ channel_id?: string }> => {
    const response = await portalApi.post(`/client-portal/leads/${leadId}/channel`);
    return unwrapPortalData<{ channel_id?: string }>(response);
  },
  // Preferências do cliente no portal — usa o portalApi (token do PORTAL),
  // sem tocar na API/sessão do painel (auth separada por contexto).
  getPreferences: async (): Promise<Record<string, unknown>> => {
    const response = await portalApi.get('/settings/preferences');
    return unwrapPortalData<Record<string, unknown>>(response);
  },
  patchPreferences: async (data: Record<string, unknown>) => {
    await portalApi.patch('/settings/preferences', data);
  },

  getMyRentals: async (): Promise<PortalRentalDTO[]> => {
    const response = await portalApi.get('/client-portal/rentals');
    return unwrapPortalData<PortalRentalDTO[]>(response);
  },

  getDealById: async (id: string, options?: { signal?: AbortSignal }) => {
    const response = await portalApi.get(`/client-portal/deals/${id}`, { signal: options?.signal });
    return unwrapPortalData(response);
  },

  /**
   * URL pública do anúncio na vitrine da imobiliária do deal (ADR 0006).
   * O JWT do cliente carrega o tenant da própria conta, então o tenant da
   * imobiliária vem do negócio. O backend só responde URL de anúncio já
   * publicado.
   */
  getSite: async (
    tenantId?: string,
  ): Promise<import('../../../shared/services/site.service').PortalSiteInfo> => {
    const response = await portalApi.get('/sites/portal/site', {
      params: tenantId ? { tenant_id: tenantId } : undefined,
    });
    return unwrapPortalData(response);
  },

  getSiteListingUrl: async (
    propertyId: string,
    tenantId?: string,
  ): Promise<import('../../../shared/services/site.service').ListingSiteUrl> => {
    const response = await portalApi.get('/sites/portal/listing-url', {
      params: { property_id: propertyId, ...(tenantId ? { tenant_id: tenantId } : {}) },
    });
    return unwrapPortalData(response);
  },

  getDealProperty: async (id: string, options?: { signal?: AbortSignal }): Promise<PortalPropertySummary> => {
    const response = await portalApi.get(`/client-portal/deals/${id}/property`, { signal: options?.signal });
    return unwrapPortalData<PortalPropertySummary>(response);
  },

  getDealSignature: async (id: string): Promise<PortalSignatureView | null> => {
    try {
      const response = await portalApi.get(`/client-portal/deals/${id}/signature`);
      return unwrapPortalData<PortalSignatureView>(response);
    } catch (err: any) {
      if (err?.response?.status === 404) return null;
      throw err;
    }
  },

  getDealRecommendations: async (id: string, options?: { signal?: AbortSignal }): Promise<PortalPropertySummary[]> => {
    const response = await portalApi.get(`/client-portal/deals/${id}/recommendations`, { signal: options?.signal });
    return unwrapPortalData<PortalPropertySummary[]>(response);
  },

  getDealChecklist: async (id: string, options?: { signal?: AbortSignal }) => {
    const response = await portalApi.get(`/client-portal/deals/${id}/checklist`, { signal: options?.signal });
    return unwrapPortalData(response);
  },

  updateChecklistPartyFields: async (dealId: string, itemId: string, req: PortalPartyFieldsUpdateRequest): Promise<DealDTO> => {
    const response = await portalApi.patch(`/client-portal/deals/${dealId}/checklist/${itemId}/party-fields`, req);
    return unwrapPortalData<DealDTO>(response);
  },

  getDealAppointments: async (id: string, options?: { signal?: AbortSignal }) => {
    const response = await portalApi.get(`/client-portal/deals/${id}/appointments`, { signal: options?.signal });
    return unwrapPortalData<PortalAppointment[]>(response);
  },

  createDealAppointment: async (id: string, req: CreateAppointmentRequest): Promise<PortalAppointment> => {
    const response = await portalApi.post(`/client-portal/deals/${id}/appointments`, req);
    return unwrapPortalData<PortalAppointment>(response);
  },

  updateDealAppointment: async (id: string, appointmentId: string, req: {
    date: string; time: string; end_time: string; description?: string; expected_updated_at: string;
  }): Promise<PortalAppointment> => {
    const response = await portalApi.put(`/client-portal/deals/${id}/appointments/${appointmentId}`, req);
    return unwrapPortalData<PortalAppointment>(response);
  },

  cancelDealAppointment: async (id: string, appointmentId: string, expectedUpdatedAt: string): Promise<PortalAppointment> => {
    const response = await portalApi.delete(`/client-portal/deals/${id}/appointments/${appointmentId}`, {
      data: { expected_updated_at: expectedUpdatedAt },
    });
    return unwrapPortalData<PortalAppointment>(response);
  },

  /**
   * Baixa o `.ics` de um compromisso marcado com o token do PORTAL.
   *
   * É o que o botão "Adicionar ao calendário (.ics)" usa: a rota padrão do
   * componente (`calendarService`) é a da agenda interna e exige permissão de
   * operador — para o cliente ela responderia 403. Aqui a autorização é a do
   * portal (o negócio precisa ser dele). A resposta é `text/calendar`, sem
   * envelope JSON, então nada de `unwrapPortalData`: os bytes vêm no próprio
   * corpo. Compromisso ainda pendente ou já encerrado responde 409; o erro
   * sobe para o botão, que avisa sem quebrar a tela.
   */
  downloadDealAppointmentCalendar: async (dealId: string, appointmentId: string): Promise<Blob> => {
    const response = await portalApi.get(
      `/client-portal/deals/${dealId}/appointments/${appointmentId}/calendar.ics`,
      { responseType: 'blob' },
    );
    return response.data as Blob;
  },

  /**
   * Arquivo .ics da visita marcada do próprio cliente. Existe porque a visita
   * pedida antes do negócio não tem `deal_id`: a autorização é o lead dele.
   */
  downloadLeadVisitCalendar: async (leadId: string): Promise<Blob> => {
    const response = await portalApi.get(`/client-portal/leads/${leadId}/visit/calendar.ics`, { responseType: 'blob' });
    return response.data as Blob;
  },

  getDealCharges: async (id: string): Promise<PortalCharge[]> => {
    const response = await portalApi.get(`/client-portal/deals/${id}/charges`);
    return unwrapPortalData<PortalCharge[]>(response);
  },

  getMyTransfers: async (params?: PortalTransferListParams): Promise<PortalTransfer[]> => {
    const response = await portalApi.get("/client-portal/transfers", { params });
    return unwrapPortalData<PortalTransfer[]>(response);
  },

  getMyTransferReceiptUrl: (transferId: string): string => {
    return `/api/v1/client-portal/transfers/${transferId}/receipt`;
  },

  getMyInformes: async (year?: number): Promise<PortalInforme[]> => {
    const response = await portalApi.get("/client-portal/informes", {
      params: year ? { year } : undefined,
    });
    return unwrapPortalData<PortalInforme[]>(response);
  },

  getMyInformePdfUrl: (informeId: string): string => {
    return `/api/v1/client-portal/informes/${informeId}/pdf`;
  },

  getDealMaintenance: async (id: string): Promise<PortalMaintenanceTicket[]> => {
    const response = await portalApi.get(`/client-portal/deals/${id}/maintenance`);
    return unwrapPortalData<PortalMaintenanceTicket[]>(response);
  },

  createDealMaintenance: async (id: string, req: CreateMaintenanceTicketRequest): Promise<PortalMaintenanceTicket> => {
    const response = await portalApi.post(`/client-portal/deals/${id}/maintenance`, req);
    return unwrapPortalData<PortalMaintenanceTicket>(response);
  },

  getDealMaintenanceAttachmentUploadURL: async (
    id: string,
    req: { file_name: string; content_type?: string; file_size?: number },
  ): Promise<PortalAttachmentUploadURL> => {
    const response = await portalApi.post(`/client-portal/deals/${id}/maintenance/attachments/upload-url`, req);
    return unwrapPortalData<PortalAttachmentUploadURL>(response);
  },

  completeDealMaintenanceAttachmentUpload: async (
    id: string,
    req: { storage_key: string },
  ): Promise<PortalAttachmentUploadComplete> => {
    const response = await portalApi.post(`/client-portal/deals/${id}/maintenance/attachments/upload-complete`, req);
    return unwrapPortalData<PortalAttachmentUploadComplete>(response);
  },

  getDealTickets: async (dealId: string): Promise<PortalTicket[]> => {
    const response = await portalApi.get(`/client-portal/deals/${dealId}/tickets`);
    return unwrapPortalData<PortalTicket[]>(response);
  },

  createDealTicket: async (dealId: string, req: CreateTicketRequest): Promise<PortalTicket> => {
    const response = await portalApi.post(`/client-portal/deals/${dealId}/tickets`, req);
    return unwrapPortalData<PortalTicket>(response);
  },

  getTicketById: async (ticketId: string): Promise<PortalTicket> => {
    const response = await portalApi.get(`/client-portal/tickets/${ticketId}`);
    return unwrapPortalData<PortalTicket>(response);
  },

  addTicketComment: async (
    ticketId: string,
    content: string,
    attachments?: string[],
    clientRequestId?: string,
  ): Promise<PortalTicketComment> => {
    const body: Record<string, unknown> = { content, attachments };
    if (clientRequestId) {
      body.client_request_id = clientRequestId;
    }
    const response = await portalApi.post(`/client-portal/tickets/${ticketId}/comments`, body);
    return unwrapPortalData<PortalTicketComment>(response);
  },

  getTicketAttachmentUploadURL: async (
    ticketId: string,
    req: { file_name: string; content_type?: string; file_size?: number },
  ): Promise<PortalAttachmentUploadURL> => {
    const response = await portalApi.post(`/client-portal/tickets/${ticketId}/attachments/upload-url`, req);
    return unwrapPortalData<PortalAttachmentUploadURL>(response);
  },

  completeTicketAttachmentUpload: async (
    ticketId: string,
    req: { storage_key: string },
  ): Promise<PortalAttachmentUploadComplete> => {
    const response = await portalApi.post(`/client-portal/tickets/${ticketId}/attachments/upload-complete`, req);
    return unwrapPortalData<PortalAttachmentUploadComplete>(response);
  },

  getLeads: async (): Promise<PortalLead[]> => {
    const response = await portalApi.get(`/client-portal/leads`);
    return unwrapPortalData<PortalLead[]>(response);
  },

  searchProperties: async (dealId: string, params: PropertySearchParams = {}): Promise<PortalPropertySummary[]> => {
    const response = await portalApi.get(`/client-portal/deals/${dealId}/properties`, { params });
    return unwrapPortalData<PortalPropertySummary[]>(response);
  },

  /**
   * Vitrine de quem NÃO tem negócio aberto: imóveis disponíveis das
   * imobiliárias com quem o cliente se relaciona (tenants dos leads dele).
   * Mesmo recorte de disponibilidade e finalidade da busca do negócio — a
   * autorização é a posse dos leads, resolvida no servidor.
   */
  listAvailableProperties: async (
    params: { listing_type?: string; limit?: number } = {},
  ): Promise<PortalPropertySummary[]> => {
    const response = await portalApi.get('/client-portal/properties', { params });
    return unwrapPortalData<PortalPropertySummary[]>(response);
  },

  /**
   * Teto de arquivo do funil de upload, espelho de `virusscan.MaxUploadBytes`
   * no backend. O dropzone usa para recusar cedo, com a mesma mensagem que o
   * servidor devolveria (413).
   */
  maxUploadBytes: 25 * 1024 * 1024,

  uploadChecklistItem: async (dealId: string, itemId: string, file?: File | null) => {
    if (!file) {
      const form = new FormData();
      const response = await portalApi.post(`/client-portal/deals/${dealId}/checklist/${itemId}/upload`, form, {
        headers: { "Content-Type": "multipart/form-data" },
      });
      return unwrapPortalData(response);
    }

    try {
      const urlResp = await portalApi.post(`/client-portal/deals/${dealId}/checklist/${itemId}/upload-url`, {
        file_name: file.name,
        content_type: file.type || "application/octet-stream",
        file_size: file.size,
      });
      const urlData = unwrapPortalData<{ upload_url: string; storage_key: string }>(urlResp);

      const putResp = await fetch(urlData.upload_url, {
        method: "PUT",
        body: file,
        headers: { "Content-Type": file.type || "application/octet-stream" },
      });
      if (!putResp.ok) {
        throw new Error(`PUT failed: ${putResp.status}`);
      }

      const completeResp = await portalApi.post(`/client-portal/deals/${dealId}/checklist/${itemId}/upload-complete`, {
        storage_key: urlData.storage_key,
        file_name: file.name,
        content_type: file.type,
        file_size: file.size,
      });
      return unwrapPortalData(completeResp);
    } catch (e) {
      const form = new FormData();
      form.append("file", file);
      const response = await portalApi.post(`/client-portal/deals/${dealId}/checklist/${itemId}/upload`, form, {
        headers: { "Content-Type": "multipart/form-data" },
      });
      return unwrapPortalData(response);
    }
  },

  acceptProposal: async (dealId: string, proposalId: string) => {
    const response = await portalApi.post(`/client-portal/deals/${dealId}/proposals/${proposalId}/accept`);
    return unwrapPortalData(response);
  },

  rejectProposal: async (dealId: string, proposalId: string, reason?: string, counterValue?: number) => {
    const response = await portalApi.post(`/client-portal/deals/${dealId}/proposals/${proposalId}/reject`, { reason, counter_value: counterValue });
    return unwrapPortalData(response);
  },

  getDealFavorites: async (dealId: string): Promise<PortalFavoriteIDs> => {
    const response = await portalApi.get(`/client-portal/deals/${dealId}/favorites`);
    return unwrapPortalData<PortalFavoriteIDs>(response);
  },

  addDealFavorite: async (dealId: string, propertyId: string): Promise<void> => {
    await portalApi.post(`/client-portal/deals/${dealId}/favorites`, { property_id: propertyId });
  },

  removeDealFavorite: async (dealId: string, propertyId: string): Promise<void> => {
    await portalApi.delete(`/client-portal/deals/${dealId}/favorites/${propertyId}`);
  },
};

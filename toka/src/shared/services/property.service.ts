import api, { unwrapApiData } from './api';

export type PropertyStatus =
 | 'prospecting'
 | 'in_portfolio'
 | 'published'
 | 'attending'
 | 'negotiating'
 | 'proposal'
 | 'analysis'
 | 'contract'
 | 'rented'
 | 'sold'
 | 'administered'
 | 'rescission'
 | 'available'
 | 'inactive';

export type PropertyType = 'apartment' | 'house' | 'land' | 'commercial';
export type ListingType = 'sale' | 'rent' | 'exchange';
export type BrokerRole =
 | 'captador'
 | 'co_captador'
 | 'visitador'
 | 'atendente'
 | 'fechamento'
 | 'imobiliaria';

export interface Address {
 street: string;
 number: string;
 complement?: string;
 neighborhood: string;
 city: string;
 state: string;
 state_code?: string;
 stateCode?: string;
 postal_code?: string;
 postalCode?: string;
 country?: string;
 country_code?: string;
 countryCode?: string;
 formatted_address?: string;
 /** Coordenadas geográficas (string para compat com o wire do backend). */
 latitude?: string;
 longitude?: string;
}

/** Exclusividade da captação do imóvel. */
export type CaptureExclusivity = 'open' | 'exclusive';

export interface Money {
 current_amount: number;
 original_amount?: number;
 currency?: string;
}

export interface Pricing {
 sale_price?: Money;
 rent_price?: Money;
 condominium_fee?: Money;
 property_tax?: Money;
}

export interface Areas {
 total_area_m2: number;
 usable_area_m2: number;
 land_area_m2?: number;
 built_area_m2?: number;
}

export interface Features {
 bedrooms: number;
 bathrooms: number;
 suites: number;
 parking_spaces: number;
}

export interface PropertyImage {
 url: string;
 description?: string;
 order?: number;
 filename?: string;
}

export interface PropertyVideo {
 url: string;
 platform?: 'youtube' | 'vimeo' | 'other';
}

export interface Media {
 /**
  * O backend Go omite coleções vazias (`omitempty`). Imóveis legados sem
  * fotos podem, portanto, chegar como `media: {}`.
  */
 images?: PropertyImage[];
 videos?: PropertyVideo[];
 virtual_tours?: { url: string }[];
}

/** Owner reference — supports multiple owners with percentage */
export interface OwnerRef {
 id: string;
 type: string;
 /** Ownership share (0–100) */
 percentage?: number;
 since?: string;
}

export interface BrokerRef {
 id: string;
 creci?: string;
}

/** Capturing broker reference with role and capture date */
export interface CapturingBrokerRef {
 id: string;
 creci?: string;
 role: BrokerRole;
 commission_pct?: number;
 captured_at?: string;
}

/** Lead that expressed interest in the property */
export interface InterestedLeadRef {
 lead_id: string;
 interested_at: string;
 source?: string;
}

export interface PropertyDTO {
 id: string;
 display_id?: string;
 title: string;
 description?: string;
 status: PropertyStatus;
 publication_status?: 'draft' | 'published';
 condition?: 'new' | 'used' | 'under_construction';
 property_type: PropertyType;
 listing_type: ListingType;
 address: Address;
 pricing: Pricing;
 areas: Areas;
 features: Features;
 amenities?: string[];
 media?: Media;
 /** Legacy single owner (backward compat) */
 owner?: OwnerRef;
 /** Legacy single broker (backward compat) */
 broker?: BrokerRef;
 /** Multiple owners with percentage — single source of truth */
 owners?: OwnerRef[];
 /** Capturing brokers with roles — registered at captação */
 capturing_brokers?: CapturingBrokerRef[];
 /** Business types the property is available for (sale, rent, exchange) */
 available_for?: ListingType[];
 /** Reference to intermediation/administration/brokerage contract */
 intermediation_contract_id?: string;
 /** Leads interested in this property */
 interested_leads?: InterestedLeadRef[];
 /** Active deal/negotiation ID */
 active_deal_id?: string;
 /** Whether this property is in the partnership portfolio */
 partnership_portfolio?: boolean;
 /**
  * Exclusividade da captação. Um imóvel `exclusive` não pode estar em
  * `partnership_portfolio` (o backend rejeita com 400) — a UI bloqueia.
  */
 capture_exclusivity?: CaptureExclusivity;
 created_at?: string;
 updated_at?: string;
}

export interface PropertyFilter {
 q?: string;
 status?: string;
 type?: string;
 listing_type?: string;
 available_for?: ListingType;
 owner_id?: string;
 capturing_broker_id?: string;
 interested_lead_id?: string;
 min_price?: number;
 max_price?: number;
 min_bedrooms?: number;
 min_bathrooms?: number;
 min_area?: number;
 max_area?: number;
 partnership_portfolio?: boolean;
 limit?: number;
 offset?: number;
}

// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface CreatePropertyDTO extends Omit<PropertyDTO, 'id' | 'created_at' | 'updated_at'> {}
// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface UpdatePropertyDTO extends Partial<CreatePropertyDTO> {}

export interface AddInterestedLeadDTO {
 lead_id: string;
 source?: string;
}

export interface AddCapturingBrokerDTO {
 broker_id: string;
 creci?: string;
 role: BrokerRole;
 /**
  * UI-facing categorization (captacao | atendimento | visita). Optional on
  * the wire so older backends keep working; newer ones persist it to drive
  * commission breakdown reports.
  */
 brokerage_type?: 'captacao' | 'atendimento' | 'visita';
 /** Percentual de comissão (0–100) atribuído a este corretor. */
 commission_pct?: number;
}

export interface AddOwnerDTO {
 owner_id: string;
 owner_type: 'person' | 'company';
 percentage?: number;
}

class PropertyService {
 private readonly baseUrl = '/properties';

 async getAll(filter?: PropertyFilter) {
 const params = new URLSearchParams();
 if (filter) {
 Object.entries(filter).forEach(([key, value]) => {
 if (value !== undefined && value !== null && value !== '') {
 params.append(key, String(value));
 }
 });
 }
 return api.get<{ data: PropertyDTO[], meta: any }>(`${this.baseUrl}?${params.toString()}`);
 }

 async getById(id: string) {
 return api.get<{ data: PropertyDTO }>(`${this.baseUrl}/${id}`);
 }

 async create(data: CreatePropertyDTO) {
 return api.post<{ data: PropertyDTO }>(this.baseUrl, data);
 }

 async update(id: string, data: UpdatePropertyDTO) {
 return api.put<{ data: PropertyDTO }>(`${this.baseUrl}/${id}`, data);
 }

 async delete(id: string) {
 return api.delete(`${this.baseUrl}/${id}`);
 }

 /**
  * Phase 8.1 — backend agora retorna `{url, filename, property}`. O campo
  * `property` é a versão atualizada com a imagem já no `media.images[]` —
  * frontend pode usar como source-of-truth sem refetch. `property` pode
  * vir null se a etapa de persistência falhou (upload físico ainda OK).
  */
 async uploadImage(propertyId: string, file: File): Promise<{ url: string; filename: string; property?: PropertyDTO | null; persist_error?: string }> {
 const fd = new FormData();
 fd.append('file', file);
 const res = await api.post(`${this.baseUrl}/${propertyId}/media/images`, fd, {
 headers: { 'Content-Type': 'multipart/form-data' },
 });
 return unwrapApiData<{ url: string; filename: string; property?: PropertyDTO | null; persist_error?: string }>(res);
 }

 /**
  * Backend retorna 200 com `{property}` em vez de 204 quando a remoção
  * persiste com sucesso. Frontend pode atualizar lista local pela resposta.
  */
 async deleteImage(propertyId: string, filename: string): Promise<{ property?: PropertyDTO | null } | void> {
 const res = await api.delete(`${this.baseUrl}/${propertyId}/media/images`, { params: { filename } });
 if (res.status === 204) return;
 return unwrapApiData<{ property?: PropertyDTO | null }>(res);
 }

 // ── Cross-entity linking (single source of truth) ──────────────────────────

 /** Add a lead to the property's interest list */
 async addInterestedLead(propertyId: string, data: AddInterestedLeadDTO) {
 return api.post(`${this.baseUrl}/${propertyId}/interested-leads`, data);
 }

 /** Remove a lead from the property's interest list */
 async removeInterestedLead(propertyId: string, leadId: string) {
 return api.delete(`${this.baseUrl}/${propertyId}/interested-leads/${leadId}`);
 }

 /** Add a capturing broker (captador) to the property */
 async addCapturingBroker(propertyId: string, data: AddCapturingBrokerDTO) {
 return api.post(`${this.baseUrl}/${propertyId}/capturing-brokers`, data);
 }

 /** Add an owner to the property with optional ownership percentage */
 async addOwner(propertyId: string, data: AddOwnerDTO) {
 return api.post(`${this.baseUrl}/${propertyId}/owners`, data);
 }

 /** Link or unlink the active deal/negotiation to a property */
 async setActiveDeal(propertyId: string, dealId: string | null) {
 return api.patch(`${this.baseUrl}/${propertyId}/active-deal`, { deal_id: dealId ?? '' });
 }
}

export const propertyService = new PropertyService();

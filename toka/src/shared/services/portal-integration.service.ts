// Portal Integration service — mirrors the toca-portal backend that
// manages connections with external real-estate portals (ZAP, OLX,
// VivaReal, etc.). This is distinct from `portal.service.ts`, which
// handles the customer-facing portal (OTP login, my deals, etc.).
//
// Backend reference: toca-portal/internal/api/v1/routes/routes.go
//   POST   /api/v1/integrations
//   GET    /api/v1/integrations
//   GET    /api/v1/integrations/:id
//   PUT    /api/v1/integrations/:id
//   DELETE /api/v1/integrations/:id
//   POST   /api/v1/integrations/:id/sync
//   GET    /api/v1/integrations/:id/syncs
//   GET    /api/v1/integrations/:id/leads

import api from './api';

export type PortalType = 'zap_imoveis' | 'olx_grupo_zap' | 'vivareal';

export type IntegrationStatus = 'active' | 'disconnected' | 'error';

export interface FeedPropertyFilter {
  statuses?: string[];
  property_types?: string[];
  listing_types?: string[];
  min_price?: number;
  max_price?: number;
}

export interface PortalIntegration {
  id: string;
  tenant_id: string;
  portal: PortalType;
  status: IntegrationStatus;
  feed_url: string;
  webhook_url: string;
  sync_frequency: '6h' | '12h' | '24h';
  contact_name: string;
  contact_email: string;
  contact_phone: string;
  publication_type: 'STANDARD' | 'PREMIUM' | 'SUPER_PREMIUM';
  property_filter: FeedPropertyFilter;
  created_at: string;
  updated_at: string;
  last_sync_at?: string | null;
  last_sync_status?: string;
  error_message?: string;
  consecutive_fails: number;
}

export type PortalLeadStatus = 'received' | 'processed' | 'failed' | 'duplicate';

export interface PortalLead {
  id: string;
  tenant_id: string;
  integration_id: string;
  portal: PortalType;
  raw_payload: string;
  name: string;
  email: string;
  phone: string;
  message?: string;
  property_id?: string;
  portal_listing_id?: string;
  crm_lead_id?: string;
  status: PortalLeadStatus;
  received_at: string;
  processed_at?: string;
  error_message?: string;
}

export type SyncStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export interface PortalSync {
  id: string;
  integration_id: string;
  tenant_id: string;
  portal: PortalType;
  status: SyncStatus;
  started_at: string;
  completed_at?: string;
  properties_synced: number;
  properties_failed: number;
  error_details?: string[];
  feed_size_bytes: number;
}

export interface CreatePortalIntegrationRequest {
  portal: PortalType;
  contact_name: string;
  contact_email: string;
  contact_phone: string;
  sync_frequency: '6h' | '12h' | '24h';
  publication_type: 'STANDARD' | 'PREMIUM' | 'SUPER_PREMIUM';
  property_filter?: FeedPropertyFilter;
}

export interface UpdatePortalIntegrationRequest {
  contact_name?: string;
  contact_email?: string;
  contact_phone?: string;
  sync_frequency?: '6h' | '12h' | '24h';
  publication_type?: 'STANDARD' | 'PREMIUM' | 'SUPER_PREMIUM';
  property_filter?: FeedPropertyFilter;
}

export interface Paginated<T> {
  data: T[];
  total: number;
}

function unwrap<T>(body: unknown): T {
  if (body && typeof body === 'object' && 'data' in (body as Record<string, unknown>)) {
    return (body as { data: T }).data;
  }
  return body as T;
}

export const portalIntegrationService = {
  listIntegrations: async (): Promise<PortalIntegration[]> => {
    const { data } = await api.get('/integrations');
    return unwrap<PortalIntegration[]>(data) ?? [];
  },

  getIntegration: async (id: string): Promise<PortalIntegration> => {
    const { data } = await api.get(`/integrations/${id}`);
    return unwrap<PortalIntegration>(data);
  },

  createIntegration: async (req: CreatePortalIntegrationRequest): Promise<PortalIntegration> => {
    const { data } = await api.post('/integrations', req);
    return unwrap<PortalIntegration>(data);
  },

  updateIntegration: async (
    id: string,
    req: UpdatePortalIntegrationRequest,
  ): Promise<PortalIntegration> => {
    const { data } = await api.put(`/integrations/${id}`, req);
    return unwrap<PortalIntegration>(data);
  },

  deleteIntegration: async (id: string): Promise<void> => {
    await api.delete(`/integrations/${id}`);
  },

  triggerSync: async (id: string): Promise<PortalSync> => {
    const { data } = await api.post(`/integrations/${id}/sync`);
    return unwrap<PortalSync>(data);
  },

  getSyncHistory: async (id: string, limit = 10, offset = 0): Promise<Paginated<PortalSync>> => {
    const { data } = await api.get(`/integrations/${id}/syncs`, { params: { limit, offset } });
    const payload = unwrap<Paginated<PortalSync> | PortalSync[]>(data);
    if (Array.isArray(payload)) {
      return { data: payload, total: payload.length };
    }
    return payload ?? { data: [], total: 0 };
  },

  getLeadsByIntegration: async (
    id: string,
    limit = 10,
    offset = 0,
  ): Promise<Paginated<PortalLead>> => {
    const { data } = await api.get(`/integrations/${id}/leads`, { params: { limit, offset } });
    const payload = unwrap<Paginated<PortalLead> | PortalLead[]>(data);
    if (Array.isArray(payload)) {
      return { data: payload, total: payload.length };
    }
    return payload ?? { data: [], total: 0 };
  },
};

import api, { unwrapApiData } from "./api";

/** Registro da auditoria central — espelha `pkg/audit.LogEntry` (audit.events). */
export interface AuditTrailEntry {
  id: string;
  tenant_id?: string;
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

/** Filtros suportados por GET /audit-trail (toca-auth). Vazios = sem restrição. */
export interface AuditTrailFilters {
  tenant_id?: string;
  action?: string;
  action_prefix?: string;
  entity_type?: string;
  entity_id?: string;
  actor_id?: string;
  from?: string;
  to?: string;
  /** Cursor da próxima página: created_at do último item da página anterior. */
  created_before?: string;
  limit?: number;
}

export const auditService = {
  /**
   * Lista o trail central do sistema. `tenant_id` vazio = cross-tenant
   * (permitido apenas para quem tem `system:view_audit_trail`).
   */
  async listAuditTrail(filters: AuditTrailFilters = {}): Promise<AuditTrailEntry[]> {
    const params: Record<string, string | number> = {};
    for (const [key, value] of Object.entries(filters)) {
      if (value === undefined || value === null || value === "") continue;
      params[key] = value;
    }
    const res = await api.get("/audit-trail", { params });
    return unwrapApiData<AuditTrailEntry[]>(res) ?? [];
  },
};

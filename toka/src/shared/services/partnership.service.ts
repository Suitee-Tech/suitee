import api, { unwrapApiData } from "./api";
import type { PropertyDTO } from "./property.service";
import type { ApiLead } from "./lead.service";

export type PartnershipType = "franchise" | "partner";
export type PartnershipStatus = "active" | "pending" | "inactive" | "suspended";
export type PermissionScope = "shared_portfolio" | "leads";
export type AccessLevel = "view" | "create" | "edit" | "delete";
export type RoyaltyBase = "gross_commission" | "net_commission" | "revenue";

export interface CommissionModel {
  owner_company_pct: number;
  closing_company_pct: number;
}

export interface FinancialConfig {
  fee_payer?: "closing_pays_source" | "source_pays_closing";
  royalty_percentage: number;
  admin_fee: number;
  transaction_fee: number;
  commission_split?: string;
  royalty_base?: RoyaltyBase;
  commission_model?: CommissionModel;
}

export interface SharingConfig {
  share_owner_contact: boolean;
  share_lead_contact?: boolean;
  allow_co_brokering: boolean;
  shared_property_types: string[];
  shared_regions: string[];
}

export interface Permission {
  scope: PermissionScope;
  access_level: AccessLevel;
}

export interface AuditLog {
  action: string;
  user_id: string;
  timestamp: string;
  details?: string;
}

export interface SharingGrant {
  sharing_config: SharingConfig;
  permissions: Permission[];
}
export interface PartnerIdentity {
  id: string;
  name: string;
  tax_id: string;
  city: string;
  state: string;
  active: boolean;
}
export interface AgreementChange {
  expected_version: number;
  action:
    | "save"
    | "accept"
    | "reject"
    | "cancel"
    | "suspend"
    | "terminate"
    | "accept_financial"
    | "reject_financial";
  grant?: SharingGrant;
  financial_config?: FinancialConfig;
  reason?: string;
}
export function grantFrom(p: Partnership, owner: string): SharingGrant {
  const grant = p.grants
    ? p.grants[owner]
    : {
        sharing_config: { ...p.sharing_config, share_lead_contact: false },
        permissions: p.permissions,
      };
  return {
    sharing_config: {
      share_owner_contact: false,
      share_lead_contact: false,
      allow_co_brokering: false,
      shared_property_types: [],
      shared_regions: [],
      ...grant?.sharing_config,
    },
    permissions: grant?.permissions ?? [],
  };
}
export function effectiveFinancial(config: FinancialConfig): FinancialConfig {
  const structured = config.commission_model;
  const legacy = config.commission_split?.split("/").map(Number);
  const model =
    structured &&
    (structured.owner_company_pct !== 0 || structured.closing_company_pct !== 0)
      ? structured
      : legacy?.length === 2 && legacy.every(Number.isFinite)
        ? { owner_company_pct: legacy[0], closing_company_pct: legacy[1] }
        : { owner_company_pct: 50, closing_company_pct: 50 };
  return {
    ...config,
    royalty_base: config.royalty_base || "gross_commission",
    commission_model: model,
  };
}
export function counterpart(p: Partnership, companyId: string) {
  return p.requester_id === companyId ? p.target_id : p.requester_id;
}
export function canViewPortfolio(p: Partnership, companyId: string) {
  return (
    (companyId === p.requester_id || companyId === p.target_id) &&
    p.status === "active" &&
    grantFrom(p, counterpart(p, companyId)).permissions.some(
      (v) => v.scope === "shared_portfolio" && v.access_level === "view",
    )
  );
}

export interface Partnership {
  version: number;
  financial_version?: number;
  grants?: Record<string, SharingGrant>;
  companies?: Record<string, PartnerIdentity>;
  pending_financial?: {
    proposed_by: string;
    version: number;
    financial_config: FinancialConfig;
  };
  id: string;
  requester_id: string;
  target_id: string;
  type: PartnershipType;
  status: PartnershipStatus;
  financial_config: FinancialConfig;
  sharing_config?: SharingConfig;
  permissions: Permission[];
  audit_logs?: AuditLog[];
  created_at: string;
  updated_at: string;
  target_company_name?: string;
  target_company_logo?: string;
  target_company_location?: string;
}

export interface CreatePartnershipRequest {
  target_id: string;
  type: PartnershipType;
  financial_config: FinancialConfig;
  permissions: Permission[];
}

export interface UpdateFinancialConfigRequest {
  financial_config: FinancialConfig;
}

export interface UpdateSharingConfigRequest {
  sharing_config: SharingConfig;
}

export interface UpdatePermissionsRequest {
  permissions: Permission[];
}

export interface CompanyLookup {
  id: string;
  name: string;
  legal_name: string;
  tax_id: string;
  type: string;
  is_active: boolean;
  city: string;
  state: string;
}

export interface SharedProperty {
  property: PropertyDTO;
  partnership_id: string;
  source_company_id: string;
  source_company_name: string;
}

export interface SharedLead {
  lead: ApiLead;
  partnership_id: string;
  source_company_id: string;
  source_company_name: string;
}

class PartnershipService {
  async change(id: string, change: AgreementChange): Promise<Partnership> {
    return unwrapApiData(
      await api.put(`/partnerships/${id}/agreement`, change),
    );
  }
  async list(): Promise<Partnership[]> {
    const response = await api.get("/partnerships");
    return unwrapApiData(response);
  }

  async getById(id: string): Promise<Partnership> {
    const response = await api.get(`/partnerships/${id}`);
    return unwrapApiData(response);
  }

  async create(data: CreatePartnershipRequest): Promise<Partnership> {
    const response = await api.post("/partnerships", data);
    return unwrapApiData(response);
  }

  async updateFinancialConfig(
    id: string,
    config: FinancialConfig,
  ): Promise<void> {
    await api.put(`/partnerships/${id}/financial-config`, {
      financial_config: config,
    });
  }

  async updateSharingConfig(id: string, config: SharingConfig): Promise<void> {
    await api.put(`/partnerships/${id}/sharing-config`, {
      sharing_config: config,
    });
  }

  async updatePermissions(
    id: string,
    permissions: Permission[],
  ): Promise<void> {
    await api.put(`/partnerships/${id}/permissions`, { permissions });
  }

  async getSharedPortfolio(partnershipId: string): Promise<PropertyDTO[]> {
    const response = await api.get(
      `/partnerships/${partnershipId}/shared-portfolio`,
    );
    return unwrapApiData(response);
  }

  async getAllSharedProperties(): Promise<SharedProperty[]> {
    const response = await api.get("/partnerships/shared/properties");
    return unwrapApiData(response);
  }

  async getAllSharedLeads(): Promise<SharedLead[]> {
    const response = await api.get("/partnerships/shared/leads");
    return unwrapApiData(response);
  }

  async lookupByCnpj(cnpj: string): Promise<CompanyLookup> {
    const response = await api.get("/companies/lookup", {
      params: { tax_id: cnpj },
    });
    return unwrapApiData(response);
  }

  async accept(id: string): Promise<Partnership> {
    const response = await api.post(`/partnerships/${id}/accept`, {});
    return unwrapApiData(response);
  }

  async reject(id: string): Promise<Partnership> {
    const response = await api.post(`/partnerships/${id}/reject`, {});
    return unwrapApiData(response);
  }
}

export const partnershipService = new PartnershipService();

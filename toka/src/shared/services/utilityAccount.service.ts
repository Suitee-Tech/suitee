import api, { unwrapApiData } from "./api";

export type UtilityAccountType =
  | "AGUA"
  | "ENERGIA"
  | "GAS"
  | "CONDOMINIO"
  | "IPTU"
  | "INTERNET"
  | "SEGURO"
  | "OUTROS";

export type UtilityResponsibleParty = "TENANT" | "OWNER" | "AGENCY";
export type UtilityAccountStatus = "ACTIVE" | "INACTIVE";

export type UtilityBillingMode = "BUNDLED" | "VARIABLE_SEPARATE" | "INFORMATIONAL";

export interface UtilityAccount {
  id: string;
  tenant_id: string;
  contract_id: string;
  property_id: string;
  type: UtilityAccountType;
  provider: string;
  account_number: string;
  responsible_party: UtilityResponsibleParty;
  estimated_amount_cents: number;
  payment_method?: string;
  auto_pay_enabled: boolean;
  due_day?: number;
  last_payment_date?: string;
  next_due_date?: string;
  status: UtilityAccountStatus;
  notes?: string;
  billing_mode?: UtilityBillingMode;
  bundled_item_key?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateUtilityAccountDTO {
  contract_id: string;
  property_id: string;
  type: UtilityAccountType;
  provider: string;
  account_number?: string;
  responsible_party: UtilityResponsibleParty;
  estimated_amount: number;
  payment_method?: string;
  auto_pay_enabled?: boolean;
  due_day?: number;
  next_due_date?: string;
  notes?: string;
  billing_mode?: UtilityBillingMode;
  bundled_item_key?: string;
}

export interface UpdateUtilityAccountDTO {
  provider?: string;
  account_number?: string;
  responsible_party?: UtilityResponsibleParty;
  estimated_amount?: number;
  payment_method?: string;
  auto_pay_enabled?: boolean;
  due_day?: number;
  next_due_date?: string;
  last_payment_date?: string;
  status?: UtilityAccountStatus;
  notes?: string;
  billing_mode?: UtilityBillingMode;
  bundled_item_key?: string;
}

export const utilityAccountService = {
  list: async (contractId: string): Promise<UtilityAccount[]> => {
    const res = await api.get<any>("/utility-accounts", { params: { contract_id: contractId } });
    return unwrapApiData<UtilityAccount[]>(res) || [];
  },
  getById: async (id: string): Promise<UtilityAccount> => {
    const res = await api.get<any>(`/utility-accounts/${id}`);
    return unwrapApiData<UtilityAccount>(res);
  },
  create: async (data: CreateUtilityAccountDTO): Promise<UtilityAccount> => {
    const res = await api.post<any>("/utility-accounts", data);
    return unwrapApiData<UtilityAccount>(res);
  },
  update: async (id: string, data: UpdateUtilityAccountDTO): Promise<UtilityAccount> => {
    const res = await api.put<any>(`/utility-accounts/${id}`, data);
    return unwrapApiData<UtilityAccount>(res);
  },
  remove: async (id: string): Promise<void> => {
    await api.delete<any>(`/utility-accounts/${id}`);
  },
};

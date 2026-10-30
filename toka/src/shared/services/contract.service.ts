import api, { unwrapApiData } from "./api";
import type { Contract, CreateContractRequest, SendForSignatureRequest, SignContractRequest } from "../contracts/types";

export interface TerminateContractRequest {
  reason: string;
  early_termination_fee?: number;
  security_deposit_return?: number;
  security_deposit_deductions?: number;
  exit_inspection_id?: string;
  require_exit_inspection?: boolean;
}

export interface ReadjustContractRequest {
  new_value?: number;
  index_name?: string;
  index_value?: number;
  effective_from?: string;
  next_adjustment?: string;
  note?: string;
}

export interface AmendContractRequest {
  reason: string;
  start_date?: string;
  end_date?: string;
  value?: number;
  late_fee_pct?: number;
  monthly_interest_pct?: number;
  adjust_index?: string;
  auto_adjust?: boolean;
  next_adjustment?: string;
  security_deposit?: number;
  content?: string;
  signatories?: Contract["signatories"];
}

export interface SignedFileResponse {
  url: string;
  source: "minio" | "clicksign";
  status?: string;
}

export interface TerminationFeeBreakdown {
  fee_cents: number;
  monthly_rent_cents: number;
  total_months: number;
  months_elapsed: number;
  months_remaining: number;
  fraction_remaining: number;
  cap_applied: boolean;
  reason?: string;
}

export const contractService = {
  create: async (data: CreateContractRequest): Promise<Contract> => {
    const response = await api.post<any>("/contracts", data);
    return unwrapApiData<Contract>(response);
  },

  getById: async (id: string): Promise<Contract> => {
    const response = await api.get<any>(`/contracts/${id}`);
    return unwrapApiData<Contract>(response);
  },

  sign: async (id: string, data: SignContractRequest): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/sign`, data);
    return unwrapApiData<Contract>(response);
  },

  listByDeal: async (dealId: string): Promise<Contract[]> => {
    const response = await api.get<any>(`/deals/${dealId}/contracts`);
    return unwrapApiData<Contract[]>(response) || [];
  },

  getAll: async (): Promise<Contract[]> => {
    const response = await api.get<any>("/contracts");
    return unwrapApiData<Contract[]>(response) || [];
  },

  update: async (id: string, data: Partial<CreateContractRequest>): Promise<Contract> => {
    const response = await api.put<any>(`/contracts/${id}`, data);
    return unwrapApiData<Contract>(response);
  },

  sendForSignature: async (id: string, data: SendForSignatureRequest): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/send-for-signature`, data);
    return unwrapApiData<Contract>(response);
  },

  regenerate: async (id: string, data: {
    content: string;
    signatories: Contract["signatories"];
    expected_updated_at: string;
  }): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/regenerate`, data);
    return unwrapApiData<Contract>(response);
  },

  getSignedFileUrl: async (id: string): Promise<SignedFileResponse | null> => {
    try {
      const response = await api.get<any>(`/contracts/${id}/signed-file`);
      return unwrapApiData<SignedFileResponse>(response);
    } catch (err: any) {
      if (err?.response?.status === 404) return null;
      throw err;
    }
  },

  terminate: async (id: string, data: TerminateContractRequest): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/terminate`, data);
    return unwrapApiData<Contract>(response);
  },

  previewTerminationFee: async (id: string, date?: string): Promise<TerminationFeeBreakdown> => {
    const params = date ? { date } : undefined;
    const response = await api.get<any>(`/contracts/${id}/termination-fee-preview`, { params });
    return unwrapApiData<TerminationFeeBreakdown>(response);
  },

  readjust: async (id: string, data: ReadjustContractRequest): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/readjust`, data);
    return unwrapApiData<Contract>(response);
  },

  amend: async (id: string, data: AmendContractRequest): Promise<Contract> => {
    const response = await api.post<any>(`/contracts/${id}/amend`, data);
    return unwrapApiData<Contract>(response);
  },
};

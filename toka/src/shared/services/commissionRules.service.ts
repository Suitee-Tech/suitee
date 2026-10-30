import api from "./api";

export type CommissionRole = "captador" | "co_captador" | "visitador" | "atendente" | "fechamento" | "imobiliaria";

export interface CommissionRule {
  tenant_id: string;
  role: CommissionRole;
  default_percentage: number;
  updated_at?: string;
}

export interface UpdateCommissionRulesInput {
  rules: { role: CommissionRole; default_percentage: number }[];
}

class CommissionRulesService {
  private readonly baseUrl = "/commission-rules";

  async get(): Promise<CommissionRule[]> {
    const res = await api.get<{ data: CommissionRule[] }>(this.baseUrl);
    return res.data?.data || [];
  }

  async update(input: UpdateCommissionRulesInput): Promise<CommissionRule[]> {
    const res = await api.put<{ data: CommissionRule[] }>(this.baseUrl, input);
    return res.data?.data || [];
  }
}

export const commissionRulesService = new CommissionRulesService();

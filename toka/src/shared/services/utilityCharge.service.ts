import api, { unwrapApiData } from "./api";

export interface UtilityChargeInput {
  reference_month: string;
  amount_cents: number;
  due_date: string;
  description?: string;
}

export const utilityChargeService = {
  emit: async (utilityAccountId: string, input: UtilityChargeInput): Promise<any> => {
    const response = await api.post<any>(`/utility-accounts/${utilityAccountId}/charge`, input);
    return unwrapApiData(response);
  },
};

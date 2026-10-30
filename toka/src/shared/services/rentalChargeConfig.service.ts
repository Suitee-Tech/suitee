import api, { unwrapApiData } from "./api";
import type { BundledItem, CreateRentalChargeConfigDTO, RentalChargeConfig, UpdateRentalChargeConfigDTO } from "../rentals/types";

export type { BundledItem } from "../rentals/types";

export const rentalChargeConfigService = {
  create: async (data: CreateRentalChargeConfigDTO): Promise<{ config: RentalChargeConfig; charges_created: number }> => {
    const response = await api.post<any>("/rental-charge-configs", data);
    return unwrapApiData(response);
  },

  update: async (id: string, data: UpdateRentalChargeConfigDTO): Promise<RentalChargeConfig> => {
    const response = await api.put<any>(`/rental-charge-configs/${id}`, data);
    return unwrapApiData<RentalChargeConfig>(response);
  },

  getById: async (id: string): Promise<RentalChargeConfig> => {
    const response = await api.get<any>(`/rental-charge-configs/${id}`);
    return unwrapApiData<RentalChargeConfig>(response);
  },

  getByContractId: async (contractId: string): Promise<RentalChargeConfig | null> => {
    try {
      const response = await api.get<any>(`/rental-charge-configs/by-contract/${contractId}`);
      return unwrapApiData<RentalChargeConfig>(response);
    } catch (error: any) {
      if (error?.response?.status === 404) return null;
      throw error;
    }
  },

  generateCharges: async (id: string): Promise<{ charges_created: number }> => {
    const response = await api.post<any>(`/rental-charge-configs/${id}/generate-charges`);
    return unwrapApiData(response);
  },

  updateBundledItems: async (id: string, items: BundledItem[]): Promise<{ config: RentalChargeConfig; charges_created: number }> => {
    const response = await api.post<any>(`/rental-charge-configs/${id}/bundled-items`, { items });
    return unwrapApiData(response);
  },
};

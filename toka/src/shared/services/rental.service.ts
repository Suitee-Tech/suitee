import api from "./api";

export type RentalStatus = "ACTIVE" | "ENDED" | "RESCINDED";

export interface RentalDTO {
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
  status: RentalStatus;
  created_at: string;
  updated_at: string;
}

class RentalService {
  private readonly baseUrl = "/rentals";

  async list(params?: {
    lead_id?: string;
    role?: "tenant" | "owner";
    property_id?: string;
    deal_id?: string;
    status?: RentalStatus;
    limit?: number;
    offset?: number;
  }) {
    return api.get<{ data: RentalDTO[]; meta?: any }>(this.baseUrl, { params });
  }

  async getById(id: string) {
    return api.get<{ data: RentalDTO }>(`${this.baseUrl}/${id}`);
  }

  async getByDealId(dealId: string) {
    return api.get<{ data: RentalDTO }>(`${this.baseUrl}/by-deal/${dealId}`);
  }
}

export const rentalService = new RentalService();

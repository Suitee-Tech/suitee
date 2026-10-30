import api from "./api";

export interface BusinessHoursDTO {
  timezone?: string;
  windows?: Record<string, { start: string; end: string }>;
}

export interface BrokerDTO {
  id: string;
  name: string;
  role: string;
  email?: string;
  avatar?: string;
  active: boolean;
  max_active_leads?: number;
  specialties?: string[];
  regions?: string[];
  weight?: number;
  is_priority?: boolean;
  paused?: boolean;
  business_hours?: BusinessHoursDTO;
  tenant_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreateBrokerDTO {
  id?: string;
  name: string;
  role: string;
  email?: string;
  avatar?: string;
  active?: boolean;
  max_active_leads?: number;
  specialties?: string[];
  regions?: string[];
  weight?: number;
  is_priority?: boolean;
  paused?: boolean;
  business_hours?: BusinessHoursDTO;
}

export interface UpdateBrokerDTO {
  name?: string;
  role?: string;
  email?: string;
  avatar?: string;
  active?: boolean;
  max_active_leads?: number;
  specialties?: string[];
  regions?: string[];
  weight?: number;
  is_priority?: boolean;
  paused?: boolean;
  business_hours?: BusinessHoursDTO;
}

class BrokerService {
  private readonly baseUrl = "/brokers";

  async getAll() {
    return api.get<{ data: BrokerDTO[] }>(this.baseUrl);
  }

  async getById(id: string) {
    return api.get<{ data: BrokerDTO }>(`${this.baseUrl}/${id}`);
  }

  async getPerformance(id: string) {
    return api.get<{ data: { sales_volume: number; active_leads: number; conversion_rate: number } }>(
      `${this.baseUrl}/${id}/performance`,
    );
  }

  async create(data: CreateBrokerDTO) {
    return api.post<{ data: BrokerDTO }>(this.baseUrl, data);
  }

  async update(id: string, data: UpdateBrokerDTO) {
    return api.put<{ data: BrokerDTO }>(`${this.baseUrl}/${id}`, data);
  }

  async updateMyQualification(data: Pick<UpdateBrokerDTO, "specialties" | "regions" | "business_hours">) {
    return api.patch<{ data: BrokerDTO }>(`${this.baseUrl}/me/qualification`, data);
  }

  async delete(id: string) {
    return api.delete(`${this.baseUrl}/${id}`);
  }
}

export const brokerService = new BrokerService();

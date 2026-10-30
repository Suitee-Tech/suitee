import api from "./api";

export type PayerRole = "tenant" | "owner" | "shared" | "";

/**
 * Onde o custo entra no financeiro. `next_charge` soma na próxima cobrança em
 * aberto do contrato, `next_transfer` desconta no próximo repasse pendente do
 * proprietário e `standalone` emite uma cobrança avulsa — o caminho que
 * continua válido quando o contrato não tem mais cobrança nem repasse
 * programados (locação encerrada, venda já repassada).
 */
export type BillingTarget = "next_charge" | "next_transfer" | "standalone";

/**
 * Quem pagou o fornecedor do serviço. Define a natureza do valor: o custo é
 * sempre de terceiro — a imobiliária só adianta e recebe de volta. Quando quem
 * adianta é quem assume, o valor não passa pelo financeiro da imobiliária.
 */
export type AdvancedBy = "imobiliaria" | "proprietario" | "inquilino";

export interface MaintenanceTicket {
  id: string;
  tenant_id: string;
  type: string;
  ticket_type?: string;
  category: string;
  property_id: string;
  contract_id: string;
  rental_id?: string;
  requester_lead_id?: string;
  property_title: string;
  requester: string;
  profile: string;
  status: string;
  priority: string;
  description?: string;
  attachments?: string[];
  assigned_to?: string;
  /** Referência ao profissional/fornecedor cadastrado (reutilizável). */
  supplier_id?: string;
  instructions?: string;
  scheduled_at?: string;
  estimated_cost?: number;
  actual_cost?: number;
  payer_role?: PayerRole;
  billing_target?: BillingTarget;
  advanced_by?: AdvancedBy;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateMaintenanceTicketDTO {
  type: string;
  ticket_type?: string;
  category: string;
  property_id: string;
  contract_id?: string;
  rental_id?: string;
  requester_lead_id?: string;
  property_title: string;
  requester: string;
  profile: string;
  priority: string;
  description?: string;
  assigned_to?: string;
  supplier_id?: string;
  instructions?: string;
  scheduled_at?: string;
  estimated_cost?: number;
  payer_role?: PayerRole;
  billing_target?: BillingTarget;
  advanced_by?: AdvancedBy;
}

export interface UpdateMaintenanceTicketDTO extends Partial<CreateMaintenanceTicketDTO> {
  status?: string;
  actual_cost?: number;
}

class MaintenanceService {
  private readonly baseUrl = "/maintenance";

  async getAll(params?: { status?: string; propertyId?: string; rental_id?: string; requester_lead_id?: string }) {
    const response = await api.get<{ data: MaintenanceTicket[]; meta: any }>(this.baseUrl, { params });
    return response.data;
  }

  async getById(id: string) {
    const response = await api.get<{ data: MaintenanceTicket }>(`${this.baseUrl}/${id}`);
    return response.data;
  }

  async create(data: CreateMaintenanceTicketDTO) {
    const response = await api.post<{ data: MaintenanceTicket }>(this.baseUrl, data);
    return response.data;
  }

  async update(id: string, data: UpdateMaintenanceTicketDTO) {
    const response = await api.put<{ data: MaintenanceTicket }>(`${this.baseUrl}/${id}`, data);
    return response.data;
  }

  async delete(id: string) {
    return api.delete(`${this.baseUrl}/${id}`);
  }

  async changeStatus(id: string, status: string) {
    const response = await api.patch<{ data: MaintenanceTicket }>(`${this.baseUrl}/${id}/status`, { status });
    return response.data;
  }
}

export const maintenanceService = new MaintenanceService();

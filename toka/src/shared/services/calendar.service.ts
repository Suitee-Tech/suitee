import api from "./api";
import { collectPages } from "./collectPages";

interface ApiAppointmentRaw {
  inspection_id?: string;
  inspection_contract_id?: string;
  inspector_id?: string;
  inspector_name?: string;
  id: string;
  display_id?: string;
  title: string;
  date: string;
  time: string;
  end_time: string;
  type: string;
  priority?: string;
  client: string;
  client_id?: string;
  lead_id?: string;
  property?: string;
  property_id?: string;
  agent?: string;
  agent_id?: string;
  visibility?: "team" | "private";
  customer_account_id?: string;
  deal_id?: string;
  status: string;
  description?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ApiAppointment {
  inspectionId?: string;
  inspectionContractId?: string;
  inspectorId?: string;
  inspectorName?: string;
  id: string;
  displayId?: string;
  title: string;
  date: string;
  time: string;
  endTime: string;
  type: "Visita" | "Reunião" | "Reuniao" | "Vistoria" | "Outro" | "Ligacao" | "Assinatura" | "Lembrete" | "Entrega";
  client: string;
  clientId?: string;
  leadId?: string;
  property?: string;
  propertyId?: string;
  agent?: string;
  agentId?: string;
  agentName?: string;
  visibility?: "team" | "private";
  customerAccountId?: string;
  dealId?: string;
  requestedByClient?: boolean;
  priority?: string;
  status: "Agendado" | "Concluído" | "Concluido" | "Cancelado" | "Pendente" | "Confirmado" | "Reagendado" | "Em Andamento";
  description?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CreateAppointmentDTO {
  title: string;
  date: string;
  time: string;
  endTime: string;
  type: string;
  client: string;
  agent: string;
  agentId?: string;
  property?: string;
  propertyId?: string;
  leadId?: string;
  dealId?: string;
  expectedUpdatedAt?: string;
  priority?: string;
  description?: string;
  status?: string;
  visibility?: "team" | "private";
}

export interface UpdateAppointmentDTO {
  title?: string;
  date?: string;
  time?: string;
  endTime?: string;
  type?: string;
  client?: string;
  agent?: string;
  agentId?: string;
  property?: string;
  propertyId?: string;
  leadId?: string;
  dealId?: string;
  expectedUpdatedAt?: string;
  priority?: string;
  description?: string;
  status?: string;
  visibility?: "team" | "private";
}

function fromApi(raw: ApiAppointmentRaw): ApiAppointment {
  return {
    id: raw.id,
    inspectionId: raw.inspection_id,
    inspectionContractId: raw.inspection_contract_id,
    inspectorId: raw.inspector_id,
    inspectorName: raw.inspector_name,
    displayId: raw.display_id,
    title: raw.title,
    date: raw.date,
    time: raw.time,
    endTime: raw.end_time,
    type: raw.type as ApiAppointment["type"],
    priority: raw.priority,
    client: raw.client,
    clientId: raw.client_id,
    leadId: raw.lead_id,
    property: raw.property,
    propertyId: raw.property_id,
    agent: raw.agent,
    agentId: raw.agent_id,
    agentName: raw.agent,
    visibility: raw.visibility,
    customerAccountId: raw.customer_account_id,
    dealId: raw.deal_id,
    requestedByClient: Boolean(raw.customer_account_id || (raw.lead_id && /^Portal/i.test(raw.agent ?? ""))),
    status: raw.status as ApiAppointment["status"],
    description: raw.description,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  };
}

function toApiPayload(data: CreateAppointmentDTO | UpdateAppointmentDTO): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  if (data.title !== undefined) payload.title = data.title;
  if (data.date !== undefined) payload.date = data.date;
  if (data.time !== undefined) payload.time = data.time;
  if (data.endTime !== undefined) payload.end_time = data.endTime;
  if (data.type !== undefined) payload.type = data.type;
  if (data.priority !== undefined) payload.priority = data.priority;
  if (data.client !== undefined) payload.client = data.client;
  if (data.agent !== undefined) payload.agent = data.agent;
  if (data.agentId !== undefined) payload.agent_id = data.agentId;
  if (data.property !== undefined) payload.property = data.property;
  if (data.propertyId !== undefined) payload.property_id = data.propertyId;
  if (data.dealId !== undefined) payload.deal_id = data.dealId;
  if (data.expectedUpdatedAt !== undefined) payload.expected_updated_at = data.expectedUpdatedAt;
  if (data.leadId !== undefined) payload.lead_id = data.leadId;
  if (data.description !== undefined) payload.description = data.description;
  if ((data as UpdateAppointmentDTO).status !== undefined) payload.status = (data as UpdateAppointmentDTO).status;
  if (data.visibility !== undefined) payload.visibility = data.visibility;
  return payload;
}

class CalendarService {
  private readonly baseUrl = "/appointments";

  async getAll(params?: { startDate?: string; endDate?: string; agentId?: string; scope?: "mine" | "team" }, options: { signal?: AbortSignal } = {}) {
    const snakeParams: Record<string, string> = {};
    if (params?.startDate) snakeParams.start_date = params.startDate;
    if (params?.endDate) snakeParams.end_date = params.endDate;
    if (params?.agentId) snakeParams.agent_id = params.agentId;
    if (params?.scope === "team") snakeParams.scope = "team";
    const items = await collectPages<ApiAppointmentRaw>(async (paging) => {
      const response = await api.get<{ data: ApiAppointmentRaw[]; meta?: { total?: number; limit?: number } }>(this.baseUrl, {
        params: { ...snakeParams, ...paging }, signal: options.signal,
      });
      return response.data;
    }, options);
    return { data: items.map(fromApi), meta: { total: items.length } as { total?: number; limit?: number } };
  }

  async getById(id: string) {
    const response = await api.get<{ data: ApiAppointmentRaw }>(`${this.baseUrl}/${id}`);
    return { data: fromApi(response.data.data) };
  }

  async create(data: CreateAppointmentDTO) {
    const response = await api.post<{ data: ApiAppointmentRaw }>(this.baseUrl, toApiPayload(data));
    return { data: fromApi(response.data.data) };
  }

  async update(id: string, data: UpdateAppointmentDTO) {
    const response = await api.put<{ data: ApiAppointmentRaw }>(`${this.baseUrl}/${id}`, toApiPayload(data));
    return { data: fromApi(response.data.data) };
  }

  async delete(id: string) {
    return api.delete(`${this.baseUrl}/${id}`);
  }
  /**
   * Arquivo .ics de um compromisso marcado. O download é autenticado (o link
   * direto não levaria o token da sessão), por isso vem como blob.
   */
  async downloadCalendarFile(appointmentId: string): Promise<Blob> {
    const res = await api.get(`/appointments/${encodeURIComponent(appointmentId)}/calendar.ics`, { responseType: "blob" });
    return res.data as Blob;
  }
}

export const calendarService = new CalendarService();

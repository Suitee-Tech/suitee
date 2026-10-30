import api from "./api";

export interface TicketComment {
  id: string;
  author_id: string;
  author_name?: string;
  author_type: "client" | "operator";
  content: string;
  attachments?: string[];
  created_at: string;
}

export interface Ticket {
  id: string;
  tenant_id: string;
  deal_id?: string;
  property_id?: string;
  type: string;
  category: string;
  subject: string;
  description?: string;
  status: string;
  priority: string;
  requester_account_id: string;
  requester_name?: string;
  requester_email?: string;
  assigned_to_id?: string;
  assigned_to_name?: string;
  comments?: TicketComment[];
  created_at: string;
  updated_at: string;
  resolved_at?: string | null;
  closed_at?: string | null;
}

export interface CreateTicketDTO {
  deal_id?: string;
  property_id?: string;
  type: string;
  category: string;
  subject: string;
  description: string;
  priority: string;
  requester_account_id: string;
  requester_name?: string;
  requester_email?: string;
}

export const ticketService = {
  list: async (params?: {
    status?: string;
    type?: string;
    assignee_id?: string;
    requester_account_id?: string;
    deal_id?: string;
    limit?: number;
    offset?: number;
  }) => {
    return api.get<{ data: Ticket[]; meta?: { total: number } }>("/tickets", { params });
  },
  getById: async (id: string) => {
    return api.get<{ data: Ticket }>(`/tickets/${id}`);
  },
  create: async (data: CreateTicketDTO) => {
    return api.post<{ data: Ticket }>("/tickets", data);
  },
  assign: async (id: string, assigneeId: string, assigneeName?: string) => {
    return api.patch<{ data: Ticket }>(`/tickets/${id}/assign`, { assignee_id: assigneeId, assignee_name: assigneeName });
  },
  addComment: async (id: string, data: { author_id: string; author_name?: string; author_type: "client" | "operator"; content: string }) => {
    return api.post<{ data: TicketComment }>(`/tickets/${id}/comments`, data);
  },
  updateStatus: async (id: string, status: string) => {
    return api.patch<{ data: Ticket }>(`/tickets/${id}/status`, { status });
  },
};

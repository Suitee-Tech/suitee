import { getEnv } from "../../lib/runtime-env";
import type { Invoice } from "../finance/types";
import api from "./api";
import { collectPages } from "./collectPages";

const INVOICE_SERVICE_URL = getEnv("VITE_INVOICE_API_URL") || "/invoices";

const newInvoiceCommandKey = (prefix: string): string => {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}:${crypto.randomUUID()}`;
  }
  return `${prefix}:${Date.now()}-${Math.random().toString(16).slice(2)}`;
};

export const invoiceService = {
  list: async (status?: string, reference_id?: string): Promise<Invoice[]> => {
    const params: Record<string, string> = {};
    if (status) params.status = status;
    if (reference_id) params.reference_id = reference_id;
    return collectPages(async paging => {
      const response = await api.get<{ data: Invoice[]; meta?: { total?: number; limit?: number } }>(INVOICE_SERVICE_URL, { params: { ...params, ...paging } });
      return response.data;
    });
  },

  getById: async (id: string): Promise<Invoice> => {
    const response = await api.get<{ data: Invoice }>(`${INVOICE_SERVICE_URL}/${id}`);
    return response.data.data;
  },

  create: async (data: {
    reference_id: string;
    charge_id: string;
    payer_id: string;
    amount: number;
    due_date: string;
    type: string;
  }, idempotencyKey = newInvoiceCommandKey("invoice-create")): Promise<Invoice> => {
    const response = await api.post<{ data: Invoice }>(INVOICE_SERVICE_URL, data, {
      headers: { "Idempotency-Key": idempotencyKey },
    });
    return response.data.data;
  },

  pay: async (id: string): Promise<Invoice> => {
    const response = await api.post<{ data: Invoice }>(`${INVOICE_SERVICE_URL}/${id}/pay`, undefined, {
      headers: { "Idempotency-Key": newInvoiceCommandKey("invoice-payment") },
    });
    return response.data.data;
  },
};

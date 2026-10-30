export enum InvoiceStatus {
  DRAFT = "DRAFT",
  ISSUED = "ISSUED",
  PAID = "PAID",
  OVERDUE = "OVERDUE",
  CANCELLED = "CANCELLED",
}

export enum InvoiceType {
  RENT = "RENT",
  SALE = "SALE",
  FEE = "FEE",
  OTHER = "OTHER",
}

export interface Invoice {
  id: string;
  reference_id: string;
  charge_id?: string;
  payer_id: string;
  amount: number;
  currency: string;
  due_date: string;
  status: InvoiceStatus;
  type: InvoiceType;
  created_at: string;
  updated_at: string;
  paid_at?: string;
  tenant_id: string;
}

import { describe, expect, it } from "vitest";

import { InvoiceStatus, InvoiceType, type Invoice } from "../../finance/types";
import { formatCurrency, mapInvoiceToTransaction } from "./presentation";

const invoice: Invoice = {
 id: "invoice-12345678",
 reference_id: "contract-1",
 charge_id: "charge-1",
 payer_id: "payer-1",
 amount: 1250,
 currency: "BRL",
 due_date: "2026-08-10T00:00:00Z",
 status: InvoiceStatus.PAID,
 type: InvoiceType.RENT,
 created_at: "2026-07-17T12:00:00Z",
 updated_at: "2026-07-17T12:00:00Z",
 paid_at: "2026-07-18T12:00:00Z",
 tenant_id: "tenant-1",
};

describe("mapInvoiceToTransaction", () => {
 it("preserves identifiers, dates, amount and paid status", () => {
  expect(mapInvoiceToTransaction(invoice)).toMatchObject({
   id: invoice.id,
   contractId: invoice.reference_id,
   type: "receita",
   value: invoice.amount,
   date: "2026-07-17",
   dueDate: "2026-08-10",
   paymentDate: "2026-07-18",
   status: "Pago",
   category: InvoiceType.RENT,
   recordSource: "invoice",
   linkedChargeID: "charge-1",
  });
 });

 it("maps overdue invoices and optional payment dates", () => {
  const result = mapInvoiceToTransaction({ ...invoice, status: InvoiceStatus.OVERDUE, paid_at: undefined });

  expect(result.status).toBe("Em Atraso");
  expect(result.paymentDate).toBeNull();
 });
});

describe("formatCurrency", () => {
 it("formats absolute BRL values and keeps the empty fallback", () => {
  expect(formatCurrency(-1250)).toContain("1.250,00");
  expect(formatCurrency(Number.NaN)).toBe("R$ 0,00");
  expect(formatCurrency(undefined)).toBe("R$ 0,00");
 });
});

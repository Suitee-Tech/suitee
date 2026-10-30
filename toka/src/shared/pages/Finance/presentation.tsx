import { CheckCircle2, Clock, CreditCard } from "lucide-react";

import { InvoiceStatus, type Invoice } from "../../finance/types";
import { normalizeStatus, statusBadgeVariant } from "../../lib/lancamentos/status";
import type { Transaction } from "../../types/transaction";

export function formatCurrency(value: number | null | undefined) {
 if (value == null || Number.isNaN(value)) return "R$ 0,00";
 return new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
 }).format(Math.abs(value));
}

export function StatusBadge({ status, isVia }: { status: string; isVia?: string }) {
 const canonical = normalizeStatus("charge", status);
 const variant = canonical ? statusBadgeVariant(canonical) : null;
 const bg = variant?.bg ?? "bg-muted";
 const text = variant?.text ?? "text-muted-foreground";
 const border = variant?.border ?? "border-border";

 let icon = null;
 if (variant?.iconName === "clock") icon = <Clock className="w-3 h-3 mr-1" />;
 else if (variant?.iconName === "check") icon = <CheckCircle2 className="w-3 h-3 mr-1" />;

 return (
  <div className="flex items-center gap-2">
   <span className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-semibold border ${bg} ${text} ${border}`}>
    {icon}
    {status}
   </span>
   {isVia && (
    <span className="inline-flex items-center px-2 py-1 rounded-md text-xs font-semibold bg-primary-50 text-primary-700 border border-primary-100">
     <CreditCard className="w-3 h-3 mr-1" />
     {isVia}
    </span>
   )}
  </div>
 );
}

export function mapInvoiceToTransaction(inv: Invoice): Transaction {
 let status: Transaction["status"] = "Pendente";
 if (inv.status === InvoiceStatus.PAID) status = "Pago";
 if (inv.status === InvoiceStatus.OVERDUE) status = "Em Atraso";

 return {
  id: inv.id,
  contractId: inv.reference_id,
  type: "receita",
  description: `Fatura ${inv.type} - ${inv.id.substring(0, 8)}`,
  value: inv.amount,
  date: inv.created_at ? inv.created_at.split("T")[0] : "",
  status,
  dueDate: inv.due_date ? inv.due_date.split("T")[0] : undefined,
 paymentDate: inv.paid_at ? inv.paid_at.split("T")[0] : null,
 category: inv.type,
  recordSource: "invoice",
  linkedChargeID: inv.charge_id,
 };
}

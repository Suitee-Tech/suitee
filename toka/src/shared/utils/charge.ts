import type { Charge } from "../services/charge.service";

export type ChargeDisplayStatus = "Pendente" | "Pago" | "Parcial" | "Em Atraso" | "Cancelada" | "Reversada";

export const mapChargeStatus = (status: string): ChargeDisplayStatus => {
  const s = (status || "").toUpperCase();
  if (s === "PAID") return "Pago";
  if (s === "PARTIALLY_PAID") return "Parcial";
  if (s === "OVERDUE") return "Em Atraso";
  if (s === "CANCELLED") return "Cancelada";
  if (s === "REVERSED") return "Reversada";
  return "Pendente";
};

export type ChargeRawStatus = "PENDING" | "PAID" | "PARTIALLY_PAID" | "OVERDUE" | "CANCELLED" | "REVERSED";

export const normalizeChargeStatus = (status: string): ChargeRawStatus => {
  const s = (status || "").toUpperCase();
  if (s === "PAID") return "PAID";
  if (s === "PARTIALLY_PAID") return "PARTIALLY_PAID";
  if (s === "OVERDUE") return "OVERDUE";
  if (s === "CANCELLED") return "CANCELLED";
  if (s === "REVERSED") return "REVERSED";
  return "PENDING";
};

export type ChargeAction = "pay" | "edit" | "cancel" | "notify" | "receipt";

export const availableChargeActions = (status: string): ChargeAction[] => {
  const s = normalizeChargeStatus(status);
  switch (s) {
    case "PENDING":
    case "OVERDUE":
      return ["pay", "edit", "cancel", "notify", "receipt"];
    case "PARTIALLY_PAID":
      // Parcial ainda tem saldo em aberto: permite completar a baixa e ver o
      // recibo (sem editar/cancelar a composição original).
      return ["pay", "receipt"];
    case "PAID":
      return ["receipt"];
    case "CANCELLED":
    case "REVERSED":
      // Terminais (doc §20): sem ação de pagamento/edição/cancelamento.
      return [];
    default:
      return [];
  }
};

export const canEditCharge = (charge: Pick<Charge, "status">): boolean => {
  const s = normalizeChargeStatus(charge.status);
  return s === "PENDING" || s === "OVERDUE";
};

export const isChargeTerminal = (charge: Pick<Charge, "status">): boolean => {
  const s = normalizeChargeStatus(charge.status);
  return s === "PAID" || s === "CANCELLED" || s === "REVERSED";
};

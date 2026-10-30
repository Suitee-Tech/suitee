import type { Contract } from "./types";

/** IDs continuam sendo valores da relação, nunca o conteúdo apresentado ao operador. */
export function contractLabel(
  contract: Pick<Contract, "display_id" | "type" | "client" | "property_title" | "start_date">,
  clientName?: string,
  propertyTitle?: string,
): string {
  const date = contract.start_date?.slice(0, 10);
  const reference = contract.display_id || (date ? `Contrato de ${date.split("-").reverse().join("/")}` : "Contrato");
  return [reference, clientName || contract.client || "Cliente não informado", propertyTitle || contract.property_title || "Imóvel não informado"].join(" · ");
}

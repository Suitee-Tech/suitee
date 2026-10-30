import type { PartyRole } from "../services/deal.service";

/** Rótulos em PT-BR para os papéis de uma parte do negócio. */
export const PARTY_ROLE_LABELS: Record<PartyRole, string> = {
  tenant: "Inquilino",
  buyer: "Comprador",
  owner: "Proprietário",
  guarantor: "Fiador/Avalista",
  witness: "Testemunha",
  spouse: "Cônjuge",
  attorney: "Procurador",
  legal_representative: "Representante legal",
  guardian: "Tutor",
  curator: "Curador",
};

/** Rótulo do papel, com fallback para o próprio valor cru. */
export function partyRoleLabel(role: string): string {
  return PARTY_ROLE_LABELS[role as PartyRole] ?? role;
}

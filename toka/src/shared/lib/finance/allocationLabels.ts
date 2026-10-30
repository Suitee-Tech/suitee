// Rótulos (pt-BR) para os códigos de classificação econômica usados na
// esteira/financeiro (allocation_type / classificação gerencial). Centraliza a
// tradução para que os mesmos códigos nunca apareçam em inglês na UI.
export const ALLOCATION_TYPE_LABEL: Record<string, string> = {
  accounts_receivable: "Contas a receber",
  revenue: "Receita da imobiliária",
  owner_payable: "Repasse ao proprietário",
  commission_payable: "Comissão a pagar",
  partner_payable: "Repasse ao parceiro",
  royalty_payable: "Royalty a pagar",
  tax_withheld: "Tributo retido",
  receivable: "Custo de terceiro a receber",
};

/**
 * Retorna o rótulo pt-BR de um código de classificação econômica. Se o código
 * for desconhecido, devolve o próprio código (fallback) para não quebrar a UI.
 */
export function allocationTypeLabel(code: string): string {
  return ALLOCATION_TYPE_LABEL[code] ?? code;
}

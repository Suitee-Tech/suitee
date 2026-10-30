/**
 * UX writing contextual de cobranças: venda e locação têm naturezas
 * financeiras diferentes e não devem compartilhar o mesmo vocabulário.
 *
 * Venda:
 * - a intermediação total é receita da imobiliária;
 * - a comissão do corretor é uma obrigação/despesa de intermediação;
 * - o repasse é ao vendedor.
 *
 * Locação:
 * - a taxa de administração é receita recorrente;
 * - a comissão do corretor é uma obrigação de intermediação;
 * - o repasse é ao proprietário.
 */
export type ChargeContextLabels = {
  isSale: boolean;
  buyer: string;
  seller: string;
  original: string;
  discounts: string;
  total: string;
  paid: string;
  agencyFee: string;
  brokerFee: string;
  ownerTransfer: string;
  partnerTransfer: string;
  taxWithheld: string;
  other: string;
  outstanding: string;
  nature: string;
};

export function getChargeContextLabels(type?: string | null): ChargeContextLabels {
  const isSale = type === "SALE";
  return {
    isSale,
    buyer: isSale ? "Comprador" : "Inquilino",
    seller: isSale ? "Vendedor" : "Proprietário",
    original: "Valor Original",
    discounts: "Descontos Aplicados",
    total: "Total da Cobrança",
    paid: "Valor Recebido",
    agencyFee: isSale ? "Total de Intermediação da Venda" : "Taxa de Administração (Locação)",
    brokerFee: isSale ? "Comissão do Corretor (Venda)" : "Comissão do Corretor (Locação)",
    ownerTransfer: isSale ? "Repasse ao Vendedor" : "Repasse ao Proprietário",
    partnerTransfer: "Repasse ao Parceiro",
    taxWithheld: "Tributo Retido",
    other: "Outros Destinos",
    outstanding: "Saldo a Receber",
    nature: isSale
      ? "Venda: a intermediação total é receita da imobiliária; a comissão do corretor e o repasse ao vendedor são obrigações."
      : "Locação: a taxa de administração é receita recorrente; a comissão do corretor e o repasse ao proprietário são obrigações.",
  };
}

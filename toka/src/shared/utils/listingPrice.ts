import { formatCurrencyBRL0 } from "./format";

/**
 * Descrição do valor de um imóvel, por finalidade.
 *
 * Revisão de acesso a imóveis (2026-09-19): cada tela descrevia o preço do seu
 * jeito — uma usava `sale_price`, outra `sale_price || rent_price`, uma
 * formatava com centavos e outra sem, e nenhuma dizia SE o número era venda ou
 * aluguel. O resultado era número certo com rótulo errado (aluguel apresentado
 * como preço de venda), e "R$ 2.500" sem indicar que é mensal.
 *
 * Regra única, daqui em diante:
 *
 *  - finalidade define a ROTULAGEM ("Venda" / "Aluguel") e a periodicidade
 *    ("/mês" só em aluguel);
 *  - valor ausente ou zero é **"Preço sob consulta"** — nunca o preço da outra
 *    finalidade, nunca "R$ 0".
 */
export type ListingType = "sale" | "rent" | string | null | undefined;

export function isRentListing(listingType: ListingType): boolean {
  return String(listingType ?? "").toLowerCase() === "rent";
}

/** "Venda" | "Aluguel" — a finalidade do valor exibido. */
export function listingBusinessLabel(listingType: ListingType): string {
  return isRentListing(listingType) ? "Aluguel" : "Venda";
}

/** Preço do imóvel com rótulo da finalidade, ou "Preço sob consulta". */
export function describeListingPrice(
  price: number | null | undefined,
  listingType: ListingType,
): { hasPrice: boolean; amount: string; label: string; period: string } {
  const value = typeof price === "number" && Number.isFinite(price) ? price : 0;
  const rent = isRentListing(listingType);
  if (value <= 0) {
    return { hasPrice: false, amount: "Preço sob consulta", label: "", period: "" };
  }
  return {
    hasPrice: true,
    amount: formatCurrencyBRL0(value),
    label: rent ? "Aluguel" : "Venda",
    period: rent ? "/mês" : "",
  };
}

import { getStageMeta } from "../../deals/dealStages";
import type { DealStatus } from "../../services/deal.service";
import { formatCurrencyBRL0 } from "../../utils/format";

/**
 * Rótulo humano para um negócio em listas de seleção.
 *
 * Contexto do defeito: o dropdown de negócio exibia `display_id || id` — e na
 * ausência de `display_id` (o caso comum) o operador via um UUID, impossível de
 * identificar. O rótulo passa a ser montado com o que identifica o negócio para
 * uma pessoa: imóvel, cliente, tipo, valor e etapa.
 */

export type DealOptionLabelDeal = {
  id: string;
  display_id?: string | null;
  type?: string | null;
  status?: string | null;
  value?: number | null;
  property_id?: string | null;
  customer_id?: string | null;
  lead_id?: string | null;
  parties?: Array<{ role?: string | null; name?: string | null }> | null;
};

export interface DealOptionLabelInput {
  deal: DealOptionLabelDeal;
  /** Título do imóvel do negócio (lookup do painel, quando disponível). */
  propertyTitle?: string | null;
  /** Nome do cliente/comprador/locatário (lookup do painel, quando disponível). */
  customerName?: string | null;
  /** Nome do proprietário/vendedor (lookup do painel, quando disponível). */
  ownerName?: string | null;
}

export interface DealOptionLabel {
  /** Identificação principal: imóvel, cliente ou um código curto. */
  title: string;
  /** Complemento: tipo, valor, cliente e etapa. */
  detail: string;
  /** Texto único da linha (título + complemento). */
  full: string;
  /** Texto usado pela busca (sem acentos, minúsculo). */
  searchText: string;
}

const CUSTOMER_ROLES = ["buyer", "tenant", "locatario", "locatário", "inquilino", "comprador"];
const OWNER_ROLES = ["owner", "proprietario", "proprietário", "vendedor"];

const TYPE_LABELS: Record<string, string> = {
  SALE: "Venda",
  RENT: "Locação",
  sale: "Venda",
  rent: "Locação",
  venda: "Venda",
  locacao: "Locação",
  locação: "Locação",
};

export function dealTypeLabel(type?: string | null): string {
  const raw = String(type ?? "").trim();
  if (!raw) return "Negócio";
  return TYPE_LABELS[raw] ?? raw.charAt(0).toUpperCase() + raw.slice(1).toLowerCase();
}

export function dealStatusLabel(status?: string | null): string {
  const raw = String(status ?? "").trim();
  if (!raw) return "";
  const meta = getStageMeta(raw as DealStatus);
  return meta?.label ?? raw;
}

const normalize = (value: string): string =>
  value
    // O formatador de moeda usa espaço não separável entre "R$" e o valor; a
    // busca precisa do espaço comum para "r$ 900.000" casar.
    .replace(/\u00a0/g, " ")
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();

const nameFromParties = (deal: DealOptionLabelDeal, roles: string[]): string => {
  const parties = Array.isArray(deal.parties) ? deal.parties : [];
  for (const role of roles) {
    const match = parties.find((party) => normalize(String(party?.role ?? "")) === normalize(role));
    const name = String(match?.name ?? "").trim();
    if (name) return name;
  }
  return "";
};

/** Código curto e legível quando não há nenhuma identificação semântica. */
const shortCode = (deal: DealOptionLabelDeal): string => {
  const displayId = String(deal.display_id ?? "").trim();
  if (displayId) return displayId;
  const id = String(deal.id ?? "").trim();
  return id ? `#${id.slice(0, 8)}` : "Negócio";
};

export function dealOptionLabel({
  deal,
  propertyTitle,
  customerName,
  ownerName,
}: DealOptionLabelInput): DealOptionLabel {
  const title =
    String(propertyTitle ?? "").trim() ||
    String(customerName ?? "").trim() ||
    nameFromParties(deal, CUSTOMER_ROLES) ||
    String(ownerName ?? "").trim() ||
    nameFromParties(deal, OWNER_ROLES) ||
    shortCode(deal);

  const client =
    String(customerName ?? "").trim() ||
    nameFromParties(deal, CUSTOMER_ROLES) ||
    String(ownerName ?? "").trim();

  const detailParts = [
    dealTypeLabel(deal.type),
    client && client !== title ? client : "",
    typeof deal.value === "number" && deal.value > 0 ? formatCurrencyBRL0(deal.value) : "",
    dealStatusLabel(deal.status),
  ].filter((part) => part.length > 0);

  const detail = detailParts.join(" · ");
  const full = detail ? `${title} · ${detail}` : title;

  return {
    title,
    detail,
    full,
    searchText: normalize(`${full} ${shortCode(deal)}`),
  };
}

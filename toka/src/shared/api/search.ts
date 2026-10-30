import api from "../services/api";
import type { ApiLead } from "../services/lead.service";
import type { BrokerDTO } from "../services/broker.service";
import type { DealDTO, DealStatus, DealType } from "../services/deal.service";
import type { PropertyDTO } from "../services/property.service";

export type SearchEntity = "lead" | "property" | "broker" | "deal";

export type SearchResult = {
  entity: SearchEntity;
  id: string;
  title: string;
  subtitle?: string;
  badge?: string;
  route: string;
};

const PER_ENTITY_LIMIT = 8;

const formatBRL = (value: number | undefined | null): string => {
  if (typeof value !== "number" || !Number.isFinite(value)) return "";
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format(value);
};

const swallowAccessErrors = (err: unknown): SearchResult[] => {
  const status = (err as { response?: { status?: number } })?.response?.status;
  if (status === 401 || status === 403 || status === 404) return [];
  throw err;
};

export async function searchLeads(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await api.get<{ data: ApiLead[]; total?: number }>("/leads", {
      params: { search: q, limit: PER_ENTITY_LIMIT },
      signal,
    });
    const items = (res.data?.data ?? []) as ApiLead[];
    return items.map((lead) => ({
      entity: "lead" as const,
      id: lead.id,
      title: lead.name || lead.email || lead.id,
      subtitle: [lead.email, lead.phone].filter(Boolean).join(" · ") || undefined,
      badge: lead.status,
      route: `/leads/${lead.id}`,
    }));
  } catch (err) {
    return swallowAccessErrors(err);
  }
}

export async function searchProperties(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await api.get<{ data: PropertyDTO[] }>("/properties", {
      params: { q, limit: PER_ENTITY_LIMIT },
      signal,
    });
    const items = (res.data?.data ?? []) as PropertyDTO[];
    return items.map((prop) => {
      const addr = prop.address;
      const subtitleParts = addr ? [addr.street, addr.neighborhood, addr.city].filter(Boolean) : [];
      const price = prop.pricing?.sale_price?.current_amount ?? prop.pricing?.rent_price?.current_amount;
      return {
        entity: "property" as const,
        id: prop.id,
        title: (prop as PropertyDTO & { title?: string }).title || addr?.street || prop.id,
        subtitle: subtitleParts.join(", ") || undefined,
        badge: formatBRL(price) || prop.status,
        route: `/imoveis/${prop.id}`,
      };
    });
  } catch (err) {
    return swallowAccessErrors(err);
  }
}

export async function searchBrokers(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await api.get<{ data: BrokerDTO[] }>("/brokers", {
      params: { search: q, limit: PER_ENTITY_LIMIT },
      signal,
    });
    const items = (res.data?.data ?? []) as BrokerDTO[];
    return items.map((broker) => ({
      entity: "broker" as const,
      id: broker.id,
      title: broker.name || broker.email || broker.id,
      subtitle: broker.email || undefined,
      badge: broker.role,
      route: "/roleta",
    }));
  } catch (err) {
    return swallowAccessErrors(err);
  }
}

const dealStatusLabel = (status: DealStatus | undefined): string => {
  switch (status) {
    case "NEGOTIATION": return "Negociação";
    case "PROPOSAL": return "Proposta";
    case "ANALYSIS": return "Análise";
    case "CONTRACT": return "Contrato";
    case "CONVERTED": return "Convertido";
    case "CANCELLED": return "Cancelado";
    default: return status ?? "";
  }
};

const dealTypeLabel = (type: DealType | undefined): string => {
  if (type === "SALE") return "Venda";
  if (type === "RENT") return "Locação";
  return type ?? "";
};

export async function searchDeals(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await api.get<{ data: DealDTO[] }>("/deals", {
      params: { search: q, limit: PER_ENTITY_LIMIT },
      signal,
    });
    const items = (res.data?.data ?? []) as DealDTO[];
    return items.map((deal) => {
      const typeLabel = dealTypeLabel(deal.type);
      const valueLabel = formatBRL(deal.value);
      const idLabel = deal.display_id || deal.id.slice(0, 8);
      const title = typeLabel ? `${typeLabel} #${idLabel}` : `Negócio #${idLabel}`;
      return {
        entity: "deal" as const,
        id: deal.id,
        title,
        subtitle: valueLabel || undefined,
        badge: dealStatusLabel(deal.status),
        route: `/esteira/deals/${deal.id}`,
      };
    });
  } catch (err) {
    return swallowAccessErrors(err);
  }
}

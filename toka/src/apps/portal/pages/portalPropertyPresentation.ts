import type {
  PortalMatchedProperty,
  PortalOverviewLead,
  PortalPropertySummary,
} from "../services/portal.service";

export type PortalPropertyView = {
  id: string;
  title: string;
  address?: string;
  neighborhood?: string;
  city?: string;
  state?: string;
  imageUrl?: string;
  images: Array<{ url: string }>;
  price: number;
  listingType?: "rent" | "sale";
  propertyType?: string;
  bedrooms?: number;
  bathrooms?: number;
  parkingSpaces?: number;
  totalAreaM2?: number;
  matchScore?: number;
  matchReasons?: PortalPropertySummary["match_reasons"];
};

export type LeadPropertyEntry = {
  property: PortalPropertyView;
  leadId: string;
  tenantId?: string;
  recommended: boolean;
};

const positiveNumber = (value: unknown): number | undefined =>
  typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;

export function summaryToPropertyView(property: PortalPropertySummary): PortalPropertyView {
  const raw = property as PortalPropertySummary & {
    media?: { images?: Array<{ url?: string }> };
  };
  const images = (raw.media?.images ?? [])
    .filter((item): item is { url: string } => Boolean(item?.url))
    .map((item) => ({ url: item.url }));

  return {
    id: property.id,
    title: property.title?.trim() || "Imóvel sem título",
    address: property.address?.trim() || undefined,
    neighborhood: property.neighborhood?.trim() || undefined,
    city: property.city?.trim() || undefined,
    state: property.state?.trim() || undefined,
    imageUrl: property.image_url,
    images,
    price: positiveNumber(property.price) ?? 0,
    listingType: property.listing_type === "rent" || property.listing_type === "sale"
      ? property.listing_type
      : undefined,
    propertyType: property.property_type,
    bedrooms: positiveNumber(property.bedrooms),
    bathrooms: positiveNumber(property.bathrooms),
    parkingSpaces: positiveNumber(property.parking_spaces),
    totalAreaM2: positiveNumber(property.total_area_m2),
    matchScore: positiveNumber(property.match_score),
    matchReasons: property.match_reasons,
  };
}

export function matchedToPropertyView(
  property: PortalMatchedProperty,
  score?: number,
): PortalPropertyView {
  // A finalidade do CADASTRO manda: valor e rótulo vêm da mesma finalidade.
  // Antes a tela inferia pelo campo que tinha valor (`sale_price ?? rent_price`),
  // então um imóvel de locação com preço de venda residual aparecia como venda.
  const declared = property.listing_type === "rent" || property.listing_type === "sale" ? property.listing_type : undefined;
  const inferred = positiveNumber(property.sale_price) != null ? "sale" : "rent";
  const listingType = declared ?? inferred;
  const price = (listingType === "rent" ? positiveNumber(property.rent_price) : positiveNumber(property.sale_price)) ?? 0;
  const images = (property.media_images ?? []).filter(Boolean).map((url) => ({ url }));

  return {
    id: property.id,
    title: property.title?.trim() || "Imóvel sem título",
    neighborhood: property.neighborhood?.trim() || undefined,
    city: property.city?.trim() || undefined,
    state: property.state?.trim() || undefined,
    imageUrl: property.thumbnail_url,
    images,
    price,
    listingType,
    propertyType: property.property_type,
    matchScore: positiveNumber(score) ?? positiveNumber(property.score),
  };
}

export function collectLeadProperties(overview: PortalOverviewLead[]): LeadPropertyEntry[] {
  const entries = new Map<string, LeadPropertyEntry>();

  for (const lead of overview) {
    if (lead.recommended_property?.id) {
      entries.set(lead.recommended_property.id, {
        property: matchedToPropertyView(lead.recommended_property, lead.recommended_property.score ?? 100),
        leadId: lead.id,
        tenantId: lead.tenant_id,
        recommended: true,
      });
    }

    for (const match of lead.matched_properties ?? []) {
      if (!match.property?.id) continue;
      const existing = entries.get(match.property.id);
      const matchScore = positiveNumber(match.score) ?? 0;
      if (!existing || matchScore > (existing.property.matchScore ?? 0)) {
        entries.set(match.property.id, {
          property: matchedToPropertyView(match.property, match.score),
          leadId: lead.id,
          tenantId: lead.tenant_id,
          recommended: existing?.recommended ?? false,
        });
      }
    }
  }

  return Array.from(entries.values()).sort((a, b) => {
    if (a.recommended !== b.recommended) return a.recommended ? -1 : 1;
    return (b.property.matchScore ?? 0) - (a.property.matchScore ?? 0);
  });
}

export function filterLeadProperties(entries: LeadPropertyEntry[], query: string): LeadPropertyEntry[] {
  const normalized = query.trim().toLocaleLowerCase("pt-BR");
  if (!normalized) return entries;

  return entries.filter(({ property }) =>
    [property.title, property.address, property.neighborhood, property.city, property.state]
      .filter(Boolean)
      .some((value) => value!.toLocaleLowerCase("pt-BR").includes(normalized)),
  );
}

export function propertyLocation(property: PortalPropertyView): string {
  return [property.neighborhood, property.city, property.state].filter(Boolean).join(" · ");
}

export function propertyAddress(property: PortalPropertyView): string {
  const address = property.address?.replace(/^\s*,\s*|\s*,\s*$/g, "").trim();
  if (address) return address;
  return propertyLocation(property);
}

export function propertyMatchLabel(score?: number): string {
  if (typeof score !== "number") return "Selecionado para você";
  if (score >= 80) return "Excelente combinação";
  if (score >= 60) return "Boa combinação";
  return "Pode combinar com você";
}

export function propertyMatchReason(property: PortalPropertyView): string {
  const reasons = property.matchReasons;
  if (!reasons) return "Selecionado a partir das preferências compartilhadas com a imobiliária";

  const candidates = [
    { value: reasons.business ?? 0, weight: reasons.used_weights?.business ?? 0, label: "tipo de negócio" },
    { value: reasons.price ?? 0, weight: reasons.used_weights?.price ?? 0, label: "faixa de preço" },
    { value: reasons.location ?? 0, weight: reasons.used_weights?.location ?? 0, label: "localização" },
    { value: reasons.area ?? 0, weight: reasons.used_weights?.area ?? 0, label: "área" },
    { value: reasons.bedrooms ?? 0, weight: reasons.used_weights?.bedrooms ?? 0, label: "quantidade de quartos" },
  ]
    .filter((item) => item.weight > 0)
    .sort((a, b) => b.value / b.weight - a.value / a.weight);

  return candidates[0]
    ? `Este imóvel se destaca pela ${candidates[0].label}`
    : "Selecionado a partir das preferências compartilhadas com a imobiliária";
}

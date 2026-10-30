import type { Property } from "../../data";
import type { PropertyDTO, PropertyStatus as BackendPropertyStatus } from "../../services/property.service";
import { getMediaUrl } from "../../media";

export const propertyStatusFromBackend: Record<string, Property["status"]> = {
  prospecting: "Em Captação",
  in_portfolio: "Em Portfólio",
  published: "Anunciado",
  attending: "Em Atendimento",
  negotiating: "Em Negociação",
  proposal: "Em Proposta",
  analysis: "Em Análise",
  contract: "Em Contrato",
  rented: "Alugado",
  sold: "Vendido",
  administered: "Em Administração",
  rescission: "Em Rescisão",
  available: "Disponível",
  inactive: "Inativo",
};

export const propertyStatusToBackend: Record<Property["status"], BackendPropertyStatus> = {
  "Em Captação": "prospecting",
  "Em Portfólio": "in_portfolio",
  Anunciado: "published",
  "Em Atendimento": "attending",
  "Em Negociação": "negotiating",
  "Em Proposta": "proposal",
  "Em Análise": "analysis",
  "Em Contrato": "contract",
  Alugado: "rented",
  Vendido: "sold",
  "Em Administração": "administered",
  "Em Rescisão": "rescission",
  Disponível: "available",
  Inativo: "inactive",
};

export const extractPropertyImageFilename = (url?: string): string | undefined => {
  if (!url) return;
  try {
    const u = new URL(url, window.location.origin);
    const parts = u.pathname.split("/").filter(Boolean);
    const idx = parts.indexOf("toca-property-images");
    if (idx === -1) return;
    const rest = parts.slice(idx + 1).join("/");
    return rest || undefined;
  } catch {
    return;
  }
};

const propertyTypeLabelFromBackend: Record<string, Property["type"]> = {
  apartment: "Apartamento",
  house: "Casa",
  land: "Terreno",
  commercial: "Comercial",
};

export const mapPropertyDtoToProperty = (dto: PropertyDTO): Property => {
  // Go serializa slices vazios com `omitempty`. Assim, um imóvel sem fotos
  // pode chegar como `media: {}` (principalmente registros antigos). Trate o
  // wire como não confiável para que um único item legado não derrube toda a
  // listagem em `dtos.map(mapPropertyDtoToProperty)`.
  const mediaImages = Array.isArray(dto.media?.images) ? dto.media.images : [];
  const mediaVideos = Array.isArray(dto.media?.videos) ? dto.media.videos : [];
  const images = mediaImages
    .filter((img) => img && typeof img.url === "string")
    .map((img, idx) => {
      const url = getMediaUrl(img.url);
      return {
        url,
        description: img.description,
        order: img.order ?? idx,
        filename: img.filename ?? extractPropertyImageFilename(url),
      };
    })
    .filter((img) => Boolean(img.url));

  const address = dto.address || ({} as PropertyDTO["address"]);
  const pricing = dto.pricing || ({} as PropertyDTO["pricing"]);
  const areas = dto.areas || ({} as PropertyDTO["areas"]);
  const features = dto.features || ({} as PropertyDTO["features"]);

  return {
    id: dto.id,
    displayId: dto.display_id,
    title: dto.title,
    description: dto.description,
    address: {
      street: address.street || "",
      number: address.number || "",
      complement: address.complement || "",
      neighborhood: address.neighborhood || "",
      city: address.city || "",
      state: address.state || "",
      stateCode: address.state_code || address.stateCode || "",
      postalCode: address.postalCode || address.postal_code || "",
      country: address.country || "Brasil",
      countryCode: address.country_code || address.countryCode || "BR",
      latitude: address.latitude,
      longitude: address.longitude,
    },
    price:
      dto.listing_type === "sale"
        ? pricing.sale_price?.current_amount || 0
        : pricing.rent_price?.current_amount || 0,
    salePrice: pricing.sale_price?.current_amount || 0,
    rentPrice: pricing.rent_price?.current_amount || 0,
    condominiumFee: pricing.condominium_fee?.current_amount || 0,
    propertyTax: pricing.property_tax?.current_amount || 0,
    type: propertyTypeLabelFromBackend[String(dto.property_type || "").toLowerCase()] || "Apartamento",
    status: propertyStatusFromBackend[dto.status] || "Disponível",
    beds: features.bedrooms || 0,
    baths: features.bathrooms || 0,
    suites: features.suites || 0,
    parkingSpaces: features.parking_spaces || 0,
    area: areas.total_area_m2 || 0,
    amenities: Array.isArray(dto.amenities) ? dto.amenities : [],
    image: images[0]?.url || "",
    images,
    videos: mediaVideos,
    // Legacy single-owner is a Go zero-value struct ({ id: "" }) when unused.
    // Coerce the empty string to undefined so the `ownerId || owners[0].id`
    // chains downstream fall through to the real `owners[]` entry instead of
    // short-circuiting on "" (which `??` would not catch).
    ownerId: dto.owner?.id || undefined,
    owners: Array.isArray(dto.owners)
      ? dto.owners.map((o) => ({ id: o.id, percentage: o.percentage }))
      : undefined,
    capturingBrokers: Array.isArray(dto.capturing_brokers)
      ? dto.capturing_brokers.map((b) => ({
          id: b.id,
          role: b.role,
          creci: b.creci,
          commissionPct: b.commission_pct,
        }))
      : undefined,
    intermediationContractId: dto.intermediation_contract_id,
    availableFor: Array.isArray(dto.available_for) ? dto.available_for : undefined,
    interestedLeads: Array.isArray(dto.interested_leads)
      ? dto.interested_leads.map((l) => ({ id: l.lead_id, date: l.interested_at, source: l.source }))
      : undefined,
    activeDealId: dto.active_deal_id,
    partnershipPortfolio: dto.partnership_portfolio,
    captureExclusivity: dto.capture_exclusivity || "open",
    publicationStatus: dto.publication_status || "draft",
    listingType: dto.listing_type || "sale",
    condition: dto.condition || "used",
  };
};

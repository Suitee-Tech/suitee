export const portalKeys = {
  all: () => ["portal"] as const,
  overview: () => ["portal", "lead-overview"] as const,
  deals: () => ["portal", "deals"] as const,
  deal: (dealId: string) => ["portal", "deal", dealId] as const,
  dealProperty: (dealId: string) => ["portal", "deal", dealId, "property"] as const,
  dealRecommendations: (dealId: string) => ["portal", "deal", dealId, "recommendations"] as const,
  dealChecklist: (dealId: string) => ["portal", "deal", dealId, "checklist"] as const,
  dealCharges: (dealId: string) => ["portal", "deal", dealId, "charges"] as const,
  dealTickets: (dealId: string) => ["portal", "deal", dealId, "tickets"] as const,
  dealMaintenance: (dealId: string) => ["portal", "deal", dealId, "maintenance"] as const,
  dealAppointments: (dealId: string) => ["portal", "deal", dealId, "appointments"] as const,
  dealFavorites: (dealId: string) => ["portal", "deal", dealId, "favorites"] as const,
  dealPropertySearch: (dealId: string, listingType: "rent" | "sale", query: string) =>
    ["portal", "deal", dealId, "properties", listingType, query] as const,
  // Imóvel aberto por link direto/recarga: resolvido pela busca do negócio
  // ativo, não pelo overview (que só tem recomendação/match do lead).
  detailProperty: (propertyId: string) => ["portal", "property", propertyId] as const,
  ticket: (ticketId: string) => ["portal", "ticket", ticketId] as const,
  notifications: () => ["portal", "notifications"] as const,
  leads: () => ["portal", "leads"] as const,
} as const;

export type PortalQueryKey = ReturnType<(typeof portalKeys)[keyof typeof portalKeys]>;

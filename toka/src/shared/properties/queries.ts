import { unwrapApiData } from "../services/api";
import { propertyService, type PropertyDTO, type PropertyFilter } from "../services/property.service";

export type { PropertyDTO, PropertyFilter } from "../services/property.service";

export const propertiesListQueryKey = (filter: PropertyFilter = {}, tenantId?: string | null) =>
  ["properties", tenantId ?? "no-tenant", filter] as const;
export const propertyQueryKey = (id: string, tenantId?: string | null) =>
  ["property", tenantId ?? "no-tenant", id] as const;
export const propertyFallbackQueryKey = (id: string, tenantId?: string | null) =>
  ["property-fallback", tenantId ?? "no-tenant", id] as const;

export const fetchPropertiesList = async (filter: PropertyFilter = {}): Promise<PropertyDTO[]> => {
  const res = await propertyService.getAll(filter);
  return unwrapApiData<PropertyDTO[]>(res);
};

export const fetchProperty = async (id: string): Promise<PropertyDTO> => {
  const res = await propertyService.getById(id);
  return unwrapApiData<PropertyDTO>(res);
};

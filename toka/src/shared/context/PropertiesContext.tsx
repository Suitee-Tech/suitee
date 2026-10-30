import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { Property, SharedFrom } from "../data";
import { formatPropertyAddress } from "../data";
import { propertyService, type CreatePropertyDTO, type PropertyDTO, type UpdatePropertyDTO } from "../services/property.service";
import { partnershipService, type SharedProperty } from "../services/partnership.service";
import { unwrapApiData } from "../services/api";
import { notifyApiError } from "../services/errorToast";
import { mapPropertyDtoToProperty, propertyStatusToBackend } from "../lib/properties/mappers";
import { resolvePropertyDetailSnapshot } from "../lib/properties/detailState";
import {
  fetchPropertiesList,
  propertiesListQueryKey,
  propertyFallbackQueryKey,
  propertyQueryKey,
} from "../properties/queries";
import { coerceMoneyBR } from "../utils/format";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";

type PropertiesContextValue = {
  properties: Property[];
  setProperties: React.Dispatch<React.SetStateAction<Property[]>>;
  fetchProperties: () => Promise<void>;
  addProperty: (property: Omit<Property, "id">) => Promise<Property | null>;
  updateProperty: (id: string, updates: Partial<Property>) => Promise<Property | null>;
  updatePropertyStatus: (id: string, status: Property["status"]) => Promise<boolean>;
};

const PropertiesContext = createContext<PropertiesContextValue | null>(null);

const companyNamePromiseCache = new Map<string, Promise<string>>();

async function resolveCompanyName(companyId: string, resolver?: (companyId: string) => Promise<string>): Promise<string> {
  if (!companyId || !resolver) return "Empresa parceira";
  const cached = companyNamePromiseCache.get(companyId);
  if (cached) return cached;

  const promise = resolver(companyId)
    .then((name) => name || "Empresa parceira")
    .catch(() => {
      companyNamePromiseCache.delete(companyId);
      return "Empresa parceira";
    });

  companyNamePromiseCache.set(companyId, promise);
  return promise;
}

async function mapSharedPropertiesToProperties(
  shared: SharedProperty[],
  resolver?: (companyId: string) => Promise<string>,
): Promise<Property[]> {
  if (!Array.isArray(shared) || shared.length === 0) return [];

  const uniqueCompanyIds = Array.from(new Set(shared.filter((s) => !s.source_company_name?.trim()).map((s) => s.source_company_id).filter(Boolean)));
  const nameById = new Map<string, string>();
  await Promise.all(
    uniqueCompanyIds.map(async (id) => {
      nameById.set(id, await resolveCompanyName(id, resolver));
    }),
  );

  return shared.map((item) => {
    const companyName =
      item.source_company_name?.trim() || nameById.get(item.source_company_id) || "Empresa parceira";
    const sharedFrom: SharedFrom = {
      partnershipId: item.partnership_id,
      companyId: item.source_company_id,
      companyName,
    };
    return { ...mapPropertyDtoToProperty(item.property), sharedFrom };
  });
}

function mergeOwnedAndShared<T extends { id: string }>(owned: T[], shared: T[]): T[] {
  if (shared.length === 0) return owned;
  const ownedIds = new Set(owned.map((item) => item.id));
  const extras = shared.filter((item) => !ownedIds.has(item.id));
  return [...owned, ...extras];
}

export function reconcilePropertyQueryCaches(
  queryClient: QueryClient,
  tenantId: string | undefined,
  dto: PropertyDTO,
  property: Property,
) {
  queryClient.setQueryData(propertyQueryKey(property.id, tenantId), dto);
  queryClient.setQueryData(propertyFallbackQueryKey(property.id, tenantId), property);
  queryClient.setQueriesData<PropertyDTO[]>(
    { queryKey: ["properties", tenantId ?? "no-tenant"] },
    (current) => {
      if (!Array.isArray(current)) return current;
      return current.map((item) => (item.id === property.id ? dto : item));
    },
  );
}

export function PropertiesProvider({
  children,
  isAuthenticated,
  companyId,
  permissionsLoading,
  shouldFetch,
  canReadSharedContent,
  resolveSharedCompanyName,
  onPropertyCreated,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  permissionsLoading: boolean;
  shouldFetch: boolean;
  canReadSharedContent: boolean;
  resolveSharedCompanyName?: (companyId: string) => Promise<string>;
  onPropertyCreated?: (created: Property, draft: Omit<Property, "id">) => Promise<void>;
}) {
  const queryClient = useQueryClient();

  const [properties, setProperties] = useState<Property[]>([]);
  const propertyMutationGenerationRef = useRef(new Map<string, number>());
  const propertyMutationRef = useRef<Promise<unknown> | null>(null);
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);

  const fetchProperties = useCallback(async () => {
    // Wait for any in-flight property mutation to settle before refetching.
    const pendingMutation = propertyMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }

    const isCurrent = beginScopedRequest();
    const dtos = await queryClient.fetchQuery({
      queryKey: propertiesListQueryKey({ limit: 1000, offset: 0 }, companyId),
      queryFn: () => fetchPropertiesList({ limit: 1000, offset: 0 }),
      staleTime: 0,
    });
    const mapped = Array.isArray(dtos) ? dtos.map(mapPropertyDtoToProperty) : [];

    if (canReadSharedContent) {
      try {
        const shared = await partnershipService.getAllSharedProperties();
        const sharedMapped = await mapSharedPropertiesToProperties(shared, resolveSharedCompanyName);
        if (!isCurrent()) return;
        setProperties(mergeOwnedAndShared(mapped, sharedMapped));
        return;
      } catch (e) {
        if (!isCurrent()) return;
        notifyApiError(e, { context: "Carregar imóveis compartilhados", silent: true });
      }
    }
    if (!isCurrent()) return;
    setProperties(mapped);
  }, [beginScopedRequest, canReadSharedContent, companyId, queryClient, resolveSharedCompanyName]);

  useDataChangeRefresh(["properties", "partnerships"], fetchProperties, {
    enabled: isAuthenticated && Boolean(companyId) && !permissionsLoading && shouldFetch,
  });

  const previousCompanyId = useRef<string | undefined>(companyId);
  useEffect(() => {
    const prev = previousCompanyId.current;
    if (prev && companyId && prev !== companyId) {
      setProperties([]);
      queryClient.removeQueries({ queryKey: ["properties"] });
      queryClient.removeQueries({ queryKey: ["property"] });
    }
    previousCompanyId.current = companyId;
  }, [companyId, queryClient]);

  useEffect(() => {
    if (isAuthenticated) return;
    setProperties([]);
    queryClient.removeQueries({ queryKey: ["properties"] });
    queryClient.removeQueries({ queryKey: ["property"] });
  }, [isAuthenticated, queryClient]);

  useEffect(() => {
    if (!isAuthenticated || !companyId || permissionsLoading) return;
    if (shouldFetch) {
      fetchProperties().catch((e) => notifyApiError(e, { context: "Carregar imóveis" }));
    }
  }, [companyId, fetchProperties, isAuthenticated, permissionsLoading, shouldFetch]);

  const propertyTypeMap: Record<string, string> = {
    Apartamento: "apartment",
    Casa: "house",
    Terreno: "land",
    Comercial: "commercial",
    Cobertura: "apartment",
    Studio: "apartment",
    Loft: "apartment",
    "Sítio/Fazenda": "land",
    apartment: "apartment",
    house: "house",
    land: "land",
    commercial: "commercial",
    Apartment: "apartment",
    House: "house",
    Land: "land",
    Commercial: "commercial",
  };

  const addProperty = useCallback(
    async (property: Omit<Property, "id">) => {
      const isCurrent = beginScopedRequest();
      const tempId = `P-TEMP-${Date.now()}`;
      const newProperty: Property = { ...property, id: tempId };

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      propertyMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });

      setProperties((prev) => [newProperty, ...prev]);

      const mutationPromise = (async () => {
        try {
          const backendType = propertyTypeMap[property.type] || "apartment";
          const price = coerceMoneyBR((property as any).price);
          const condominiumFee = coerceMoneyBR((property as any).condominiumFee);
          const propertyTax = coerceMoneyBR((property as any).propertyTax);

          const dto: CreatePropertyDTO = {
            title: property.title,
            description: property.description || property.title,
            status: propertyStatusToBackend[property.status] || "prospecting",
            property_type: backendType as any,
            listing_type: "sale",
            condition: "used",
            publication_status:
              property.publicationStatus ??
              (property.status === "Anunciado" ? "published" : "draft"),
            address: {
              street: property.address.street,
              number: property.address.number,
              complement: property.address.complement || "",
              neighborhood: property.address.neighborhood,
              city: property.address.city,
              state: property.address.state,
              stateCode: property.address.stateCode,
              postalCode: property.address.postalCode,
              country: property.address.country || "Brasil",
              countryCode: property.address.countryCode || "BR",
              formatted_address: formatPropertyAddress(property.address),
              latitude: property.address.latitude || undefined,
              longitude: property.address.longitude || undefined,
            } as any,
            pricing: {
              sale_price: {
                current_amount: price,
                original_amount: price,
                currency: "BRL",
              },
              condominium_fee:
                condominiumFee > 0 ? { current_amount: condominiumFee, original_amount: condominiumFee, currency: "BRL" } : undefined,
              property_tax:
                propertyTax > 0 ? { current_amount: propertyTax, original_amount: propertyTax, currency: "BRL" } : undefined,
            },
            areas: { total_area_m2: property.area, usable_area_m2: property.area },
            features: {
              bedrooms: property.beds,
              bathrooms: property.baths,
              suites: property.suites || 0,
              parking_spaces: property.parkingSpaces || 0,
            },
            amenities: property.amenities || [],
            media: {
              images: (() => {
                const imgs =
                  (property.images || [])
                    .filter((img) => img.url && !img.url.startsWith("blob:"))
                    .map((img, idx) => ({ url: img.url, description: img.description, order: img.order ?? idx })) || [];
                if (imgs.length > 0) return imgs;
                // Nunca persiste blob:/vazio como capa — isso gravava uma
                // "imagem vazia" (URL de blob inválida) no imóvel.
                const cover = property.image || "";
                if (cover && !cover.startsWith("blob:") && cover.trim() !== "") {
                  return [{ url: cover, order: 0 }];
                }
                return [];
              })(),
              videos: (property.videos || []).filter((v) => v.url),
            },
            owners: property.owners?.map((o) => ({ id: o.id, type: "person", percentage: o.percentage })),
            capturing_brokers: property.capturingBrokers?.map((b) => ({
              id: b.id,
              role: b.role as any,
              creci: b.creci,
              commission_pct: b.commissionPct,
            })),
            intermediation_contract_id: property.intermediationContractId,
            available_for: property.availableFor as any,
            partnership_portfolio: property.partnershipPortfolio,
            capture_exclusivity: property.captureExclusivity || "open",
          };

          const response = await propertyService.create(dto);
          if (!isCurrent()) return null;
          const responseData = unwrapApiData<PropertyDTO>(response);
          const createdProp = mapPropertyDtoToProperty(responseData);

          setProperties((prev) => prev.map((p) => (p.id === tempId ? createdProp : p)));

          if (onPropertyCreated) {
            try {
              await onPropertyCreated(createdProp, property);
            } catch {
            }
          }

          if (!isCurrent()) return null;
          toast.success("Imóvel criado com sucesso!");
          return createdProp;
        } catch {
          if (!isCurrent()) return null;
          setProperties((prev) => prev.filter((p) => p.id !== tempId));
          toast.error("Erro ao criar imóvel. Verifique os dados.");
          return null;
        }
      })();

      propertyMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (propertyMutationRef.current === mutationPromise) {
          propertyMutationRef.current = null;
        }
      });

      return mutationPromise;
    },
    [beginScopedRequest, onPropertyCreated],
  );

  const updateProperty = useCallback(
    async (id: string, updates: Partial<Property>) => {
      const isCurrent = beginScopedRequest();
      const mutationGeneration =
        (propertyMutationGenerationRef.current.get(id) ?? 0) + 1;
      propertyMutationGenerationRef.current.set(id, mutationGeneration);
      const fallbackKey = propertyFallbackQueryKey(id, companyId);
      const previousFallback = queryClient.getQueryData<Property>(fallbackKey);
      const contextProperty = properties.find((p) => p.id === id);
      const oldProp =
        resolvePropertyDetailSnapshot(contextProperty, previousFallback) ??
        contextProperty ??
        previousFallback;
      if (!oldProp) {
        notifyApiError(new Error("Property not found"), { context: "Atualizar imóvel" });
        return null;
      }

      const optimisticProperty = { ...oldProp, ...updates };

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      propertyMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });

      setProperties((prev) => {
        const exists = prev.some((property) => property.id === id);
        return exists
          ? prev.map((property) => (property.id === id ? optimisticProperty : property))
          : [optimisticProperty, ...prev];
      });
      queryClient.setQueryData(fallbackKey, optimisticProperty);
      const optimisticFallback = queryClient.getQueryData<Property>(fallbackKey);

      const mutationPromise = (async () => {
        try {
          const salePrice = coerceMoneyBR((updates as any).price ?? (oldProp as any).price);
          const condoPrice = coerceMoneyBR((updates as any).condominiumFee ?? (oldProp as any).condominiumFee);
          const taxPrice = coerceMoneyBR((updates as any).propertyTax ?? (oldProp as any).propertyTax);

          const mediaImages =
            updates.images !== undefined
              ? updates.images
                  .filter((img) => img.url && !img.url.startsWith("blob:"))
                  .map((img, idx) => ({ url: img.url, description: img.description, order: img.order ?? idx }))
              : oldProp.images !== undefined
                ? oldProp.images
                    .filter((img) => img.url && !img.url.startsWith("blob:"))
                    .map((img, idx) => ({ url: img.url, description: img.description, order: img.order ?? idx }))
                : undefined;

          const mediaVideos =
            updates.videos !== undefined
              ? updates.videos.filter((v) => v.url)
              : oldProp.videos !== undefined
                ? oldProp.videos.filter((v) => v.url)
                : undefined;

          const owners =
            updates.owners !== undefined
              ? updates.owners.map((o) => ({ id: o.id, type: "person", percentage: o.percentage }))
              : oldProp.owners !== undefined
                ? oldProp.owners.map((o) => ({ id: o.id, type: "person", percentage: o.percentage }))
                : undefined;

          const capturing_brokers =
            updates.capturingBrokers !== undefined
              ? updates.capturingBrokers.map((b) => ({
                  id: b.id,
                  role: b.role as any,
                  creci: b.creci,
                  commission_pct: b.commissionPct,
                }))
              : oldProp.capturingBrokers !== undefined
                ? oldProp.capturingBrokers.map((b) => ({
                    id: b.id,
                    role: b.role as any,
                    creci: b.creci,
                    commission_pct: b.commissionPct,
                  }))
                : undefined;

          const nextStatus = updates.status !== undefined ? updates.status : oldProp.status;
          const statusChangedToAnnounced =
            updates.status === "Anunciado" && oldProp.status !== "Anunciado";
          const nextPublicationStatus =
            updates.publicationStatus !== undefined
              ? updates.publicationStatus
              : statusChangedToAnnounced
                ? "published"
                : oldProp.publicationStatus || "draft";
          const dto: UpdatePropertyDTO = {
            title: updates.title !== undefined ? updates.title : oldProp.title,
            description: updates.description !== undefined ? updates.description : oldProp.description || oldProp.title,
            status: propertyStatusToBackend[nextStatus] || "prospecting",
            publication_status: nextPublicationStatus,
            property_type: (propertyTypeMap[updates.type !== undefined ? updates.type : oldProp.type] || "apartment") as any,
            listing_type: updates.listingType !== undefined ? updates.listingType : oldProp.listingType || "sale",
            condition: updates.condition !== undefined ? updates.condition : oldProp.condition || "used",
            address: updates.address
              ? ({
                  street: updates.address.street,
                  number: updates.address.number,
                  complement: updates.address.complement || "",
                  neighborhood: updates.address.neighborhood,
                  city: updates.address.city,
                  state: updates.address.state,
                  stateCode: updates.address.stateCode,
                  postalCode: updates.address.postalCode,
                  country: updates.address.country || "Brasil",
                  countryCode: updates.address.countryCode || "BR",
                  formatted_address: formatPropertyAddress(updates.address),
                  latitude: updates.address.latitude || undefined,
                  longitude: updates.address.longitude || undefined,
                } as any)
              : ({
                  street: oldProp.address.street,
                  number: oldProp.address.number,
                  complement: oldProp.address.complement || "",
                  neighborhood: oldProp.address.neighborhood,
                  city: oldProp.address.city,
                  state: oldProp.address.state,
                  stateCode: oldProp.address.stateCode,
                  postalCode: oldProp.address.postalCode,
                  country: oldProp.address.country || "Brasil",
                  countryCode: oldProp.address.countryCode || "BR",
                  latitude: oldProp.address.latitude || undefined,
                  longitude: oldProp.address.longitude || undefined,
                } as any),
            pricing: {
              sale_price: { current_amount: salePrice, original_amount: salePrice, currency: "BRL" },
              condominium_fee:
                updates.condominiumFee !== undefined || oldProp.condominiumFee !== undefined
                  ? { current_amount: condoPrice, original_amount: condoPrice, currency: "BRL" }
                  : undefined,
              property_tax:
                updates.propertyTax !== undefined || oldProp.propertyTax !== undefined
                  ? { current_amount: taxPrice, original_amount: taxPrice, currency: "BRL" }
                  : undefined,
            },
            areas: {
              total_area_m2: updates.area !== undefined ? updates.area : oldProp.area,
              usable_area_m2: updates.area !== undefined ? updates.area : oldProp.area,
            },
            features: {
              bedrooms: updates.beds !== undefined ? updates.beds : oldProp.beds,
              bathrooms: updates.baths !== undefined ? updates.baths : oldProp.baths,
              suites: updates.suites !== undefined ? updates.suites : oldProp.suites || 0,
              parking_spaces: updates.parkingSpaces !== undefined ? updates.parkingSpaces : oldProp.parkingSpaces || 0,
            } as any,
            amenities: updates.amenities !== undefined ? updates.amenities : oldProp.amenities,
            media: mediaImages !== undefined ? ({ images: mediaImages, videos: mediaVideos } as any) : undefined,
            owners: owners as any,
            capturing_brokers: capturing_brokers as any,
            intermediation_contract_id:
              updates.intermediationContractId !== undefined ? updates.intermediationContractId : oldProp.intermediationContractId,
            available_for: updates.availableFor !== undefined ? (updates.availableFor as any) : (oldProp.availableFor as any),
            partnership_portfolio: updates.partnershipPortfolio !== undefined ? updates.partnershipPortfolio : oldProp.partnershipPortfolio,
            capture_exclusivity: updates.captureExclusivity !== undefined ? updates.captureExclusivity : oldProp.captureExclusivity,
          };

          const response = await propertyService.update(id, dto);
          const responseData = unwrapApiData<PropertyDTO>(response);
          if (!responseData?.id) {
            throw new Error("A API não retornou o imóvel atualizado.");
          }

          const mappedResponse = mapPropertyDtoToProperty(responseData);
          const updatedProperty: Property = {
            ...mappedResponse,
            displayId: mappedResponse.displayId ?? oldProp.displayId,
            sharedFrom: oldProp.sharedFrom,
          };

          if (
            !isCurrent() ||
            propertyMutationGenerationRef.current.get(id) !== mutationGeneration
          ) {
            return null;
          }
          setProperties((prev) => {
            const exists = prev.some((property) => property.id === id);
            return exists
              ? prev.map((property) => (property.id === id ? updatedProperty : property))
              : [updatedProperty, ...prev];
          });
          reconcilePropertyQueryCaches(queryClient, companyId, responseData, updatedProperty);
          return updatedProperty;
        } catch (err) {
          if (!isCurrent()) return null;
          if (propertyMutationGenerationRef.current.get(id) !== mutationGeneration) {
            return null;
          }
          setProperties((prev) => {
            const current = prev.find((property) => property.id === id);
            if (current !== optimisticProperty) return prev;
            if (!contextProperty) return prev.filter((property) => property.id !== id);
            return prev.map((property) => (property.id === id ? contextProperty : property));
          });
          if (queryClient.getQueryData(fallbackKey) === optimisticFallback) {
            if (previousFallback === undefined) {
              queryClient.removeQueries({ queryKey: fallbackKey, exact: true });
            } else {
              queryClient.setQueryData(fallbackKey, previousFallback);
            }
          }
          notifyApiError(err, { context: "Atualizar imóvel" });
          return null;
        }
      })();

      propertyMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (propertyMutationRef.current === mutationPromise) {
          propertyMutationRef.current = null;
        }
      });

      return mutationPromise;
    },
    [beginScopedRequest, companyId, properties, queryClient],
  );

  const updatePropertyStatus = useCallback(
    async (id: string, status: Property["status"]) => {
      // "Anunciado" é a transição de publicação comercial. Sem uma segunda
      // ação, o imóvel ficaria com status published e publication_status=draft
      // e nunca entraria na vitrine. O toggle "Publicar no site" permite
      // desfazer essa publicação sem voltar o status do imóvel.
      const updates: Partial<Property> = { status };
      if (status === "Anunciado") updates.publicationStatus = "published";
      return Boolean(await updateProperty(id, updates));
    },
    [updateProperty],
  );

  const value = useMemo<PropertiesContextValue>(
    () => ({ properties, setProperties, fetchProperties, addProperty, updateProperty, updatePropertyStatus }),
    [addProperty, fetchProperties, properties, updateProperty, updatePropertyStatus],
  );

  return <PropertiesContext.Provider value={value}>{children}</PropertiesContext.Provider>;
}

export function usePropertiesContext() {
  const ctx = useContext(PropertiesContext);
  if (!ctx) {
    throw new Error("usePropertiesContext must be used within PropertiesProvider");
  }
  return ctx;
}

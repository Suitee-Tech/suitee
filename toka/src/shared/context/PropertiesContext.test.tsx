import type { PropsWithChildren } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PropertyDTO } from "../services/property.service";
import {
  propertiesListQueryKey,
  propertyFallbackQueryKey,
  propertyQueryKey,
} from "../properties/queries";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";
import { PropertiesProvider, usePropertiesContext } from "./PropertiesContext";

const mocks = vi.hoisted(() => ({
  getAll: vi.fn(),
  update: vi.fn(),
  notifyApiError: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("../hooks/useDataChangeRefresh", () => ({
  useDataChangeRefresh: () => undefined,
}));

vi.mock("../services/partnership.service", () => ({
  partnershipService: {
    getAllSharedProperties: vi.fn().mockResolvedValue([]),
  },
}));

vi.mock("../services/property.service", () => ({
  propertyService: {
    getAll: mocks.getAll,
    update: mocks.update,
  },
}));

vi.mock("../services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: mocks.toastError,
  },
}));

function propertyDto(price: number): PropertyDTO {
  return {
    id: "property-1",
    display_id: "IMV-1",
    title: "Casa Vila Mariana",
    description: "Casa",
    status: "prospecting",
    publication_status: "draft",
    property_type: "house",
    listing_type: "sale",
    condition: "used",
    address: {
      street: "Rua Santa Cruz",
      number: "1041",
      neighborhood: "Vila Mariana",
      city: "São Paulo",
      state: "São Paulo",
      state_code: "SP",
      postal_code: "04121-002",
    },
    pricing: {
      sale_price: {
        current_amount: price,
        original_amount: price,
        currency: "BRL",
      },
    },
    areas: { total_area_m2: 80, usable_area_m2: 80 },
    features: { bedrooms: 2, bathrooms: 1, suites: 0, parking_spaces: 1 },
    amenities: [],
    media: { images: [], videos: [] },
    owners: [],
    capturing_brokers: [],
    available_for: ["sale"],
    capture_exclusivity: "open",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

describe("PropertiesProvider update reconciliation", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    mocks.getAll.mockReset().mockResolvedValue({
      data: { data: [propertyDto(4_000_000)] },
    });
    mocks.update.mockReset();
    mocks.notifyApiError.mockReset();
    mocks.toastError.mockReset();
  });

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <PropertiesProvider
          isAuthenticated
          companyId="tenant-a"
          permissionsLoading={false}
          shouldFetch
          canReadSharedContent={false}
        >
          {children}
        </PropertiesProvider>
      </QueryClientProvider>
    );
  }

  it("reconciles the server response into context, detail, fallback and list caches", async () => {
    const serverProperty = propertyDto(4_000_002);
    mocks.update.mockResolvedValueOnce({ data: { data: serverProperty } });

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(result.current.properties).toHaveLength(1));

    queryClient.setQueryData(propertyQueryKey("property-1", "tenant-a"), propertyDto(4_000_000));
    queryClient.setQueryData(
      propertyFallbackQueryKey("property-1", "tenant-a"),
      result.current.properties[0],
    );

    let updated = null;
    await act(async () => {
      updated = await result.current.updateProperty("property-1", { price: 4_000_001 });
    });

    expect(updated).toMatchObject({ id: "property-1", price: 4_000_002 });
    expect(result.current.properties[0].price).toBe(4_000_002);
    expect(
      queryClient.getQueryData<PropertyDTO>(propertyQueryKey("property-1", "tenant-a"))
        ?.pricing.sale_price?.current_amount,
    ).toBe(4_000_002);
    expect(
      queryClient.getQueryData<{ price: number }>(
        propertyFallbackQueryKey("property-1", "tenant-a"),
      )?.price,
    ).toBe(4_000_002);
    expect(
      queryClient.getQueryData<PropertyDTO[]>(
        propertiesListQueryKey({ limit: 1000, offset: 0 }, "tenant-a"),
      )?.[0].pricing.sale_price?.current_amount,
    ).toBe(4_000_002);
  });

  it("publica no site ao mudar o status para Anunciado", async () => {
    const published = { ...propertyDto(4_000_000), status: "published", publication_status: "published" };
    mocks.update.mockResolvedValueOnce({ data: { data: published } });

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(result.current.properties).toHaveLength(1));

    let updated = false;
    await act(async () => {
      updated = await result.current.updatePropertyStatus("property-1", "Anunciado");
    });

    expect(updated).toBe(true);
    expect(mocks.update).toHaveBeenCalledWith(
      "property-1",
      expect.objectContaining({ status: "published", publication_status: "published" }),
    );
    expect(result.current.properties[0].publicationStatus).toBe("published");
  });

  it("não republica no site ao editar um imóvel Anunciado que estava despublicado", async () => {
    const advertisedDraft = {
      ...propertyDto(4_000_000),
      status: "published",
      publication_status: "draft",
    };
    mocks.getAll.mockResolvedValueOnce({ data: { data: [advertisedDraft] } });
    mocks.update.mockResolvedValueOnce({
      data: { data: { ...advertisedDraft, title: "Casa atualizada" } },
    });

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(result.current.properties).toHaveLength(1));

    await act(async () => {
      await result.current.updateProperty("property-1", { title: "Casa atualizada" });
    });

    expect(mocks.update).toHaveBeenCalledWith(
      "property-1",
      expect.objectContaining({ publication_status: "draft" }),
    );
  });

  it("upserts a property that exists only in the hydrated fallback cache", async () => {
    mocks.getAll.mockResolvedValueOnce({ data: { data: [] } });
    const fallbackProperty = mapPropertyDtoToProperty(propertyDto(4_000_000));
    queryClient.setQueryData(
      propertyFallbackQueryKey("property-1", "tenant-a"),
      fallbackProperty,
    );
    mocks.update.mockResolvedValueOnce({
      data: { data: propertyDto(4_000_002) },
    });

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(mocks.getAll).toHaveBeenCalled());
    expect(result.current.properties).toEqual([]);

    let updated = null;
    await act(async () => {
      updated = await result.current.updateProperty("property-1", { price: 4_000_001 });
    });

    expect(updated).toMatchObject({ id: "property-1", price: 4_000_002 });
    expect(result.current.properties).toHaveLength(1);
    expect(result.current.properties[0]).toMatchObject({
      id: "property-1",
      price: 4_000_002,
    });
  });

  it("removes a fallback-only optimistic upsert and restores its cache when the update fails", async () => {
    mocks.getAll.mockResolvedValue({ data: { data: [] } });
    const fallbackProperty = mapPropertyDtoToProperty(propertyDto(4_000_000));
    const fallbackKey = propertyFallbackQueryKey("property-1", "tenant-a");
    queryClient.setQueryData(fallbackKey, fallbackProperty);
    const pendingUpdate = deferred<{ data: { data: PropertyDTO } }>();
    mocks.update.mockReturnValueOnce(pendingUpdate.promise);

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(mocks.getAll).toHaveBeenCalled());

    let updatePromise!: Promise<unknown>;
    act(() => {
      updatePromise = result.current.updateProperty("property-1", { price: 4_000_001 });
    });
    await waitFor(() => {
      expect(result.current.properties[0]?.price).toBe(4_000_001);
    });

    let updateResult: unknown;
    await act(async () => {
      pendingUpdate.reject(new Error("update failed"));
      updateResult = await updatePromise;
    });

    expect(updateResult).toBeNull();
    expect(result.current.properties).toEqual([]);
    expect(queryClient.getQueryData(fallbackKey)).toEqual(fallbackProperty);
    expect(mocks.notifyApiError).toHaveBeenCalled();
  });

  it("does not overwrite a newer external snapshot when an older update fails", async () => {
    const pendingUpdate = deferred<{ data: { data: PropertyDTO } }>();
    mocks.update.mockReturnValueOnce(pendingUpdate.promise);

    const { result } = renderHook(() => usePropertiesContext(), { wrapper });
    await waitFor(() => expect(result.current.properties).toHaveLength(1));

    const fallbackKey = propertyFallbackQueryKey("property-1", "tenant-a");
    queryClient.setQueryData(fallbackKey, result.current.properties[0]);

    let updatePromise!: Promise<unknown>;
    act(() => {
      updatePromise = result.current.updateProperty("property-1", { price: 4_000_001 });
    });
    await waitFor(() => {
      expect(result.current.properties[0]?.price).toBe(4_000_001);
    });

    const externalProperty = {
      ...result.current.properties[0],
      price: 4_000_003,
    };
    act(() => {
      result.current.setProperties((current) =>
        current.map((property) =>
          property.id === externalProperty.id ? externalProperty : property,
        ),
      );
      queryClient.setQueryData(fallbackKey, externalProperty);
    });

    await act(async () => {
      pendingUpdate.reject(new Error("older update failed"));
      await updatePromise;
    });

    expect(result.current.properties[0]).toBe(externalProperty);
    expect(queryClient.getQueryData(fallbackKey)).toEqual(externalProperty);
    expect(mocks.notifyApiError).toHaveBeenCalled();
  });
});
